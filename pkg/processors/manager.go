package processors

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/harunnryd/ranya/pkg/frames"
	"github.com/harunnryd/ranya/pkg/pipeline"
	"github.com/harunnryd/ranya/pkg/turn"
)

type TurnProcessor struct {
	mgr    turn.Manager
	emitCh chan frames.Frame
	lastID string

	silenceCfg      *SilenceRepromptConfig
	silenceTimer    *time.Timer
	repromptCount   int
	lastLanguage    string
	lastTraceID     string
	endOfTurnTTL    time.Duration
	endOfTurnTimer  *time.Timer
	endOfTurnStream string
	mu              sync.Mutex
}

type TurnProcessorConfig struct {
	BargeInThreshold time.Duration
	MinBargeIn       time.Duration
	EndOfTurnTimeout time.Duration
}

func NewTurnProcessor(strategy turn.Strategy) *TurnProcessor {
	return NewTurnProcessorWithConfig(strategy, TurnProcessorConfig{})
}

func NewTurnProcessorWithConfig(strategy turn.Strategy, cfg TurnProcessorConfig) *TurnProcessor {
	tp := &TurnProcessor{
		emitCh:       make(chan frames.Frame, 32),
		endOfTurnTTL: cfg.EndOfTurnTimeout,
	}
	emitter := &turnEmitter{out: tp.emitCh}
	tp.mgr = turn.NewManagerWithOptions(strategy, emitter, turn.ManagerOptions{
		BargeInThreshold: cfg.BargeInThreshold,
		MinBargeIn:       cfg.MinBargeIn,
	})
	return tp
}

type SilenceRepromptConfig struct {
	Timeout          time.Duration
	MaxAttempts      int
	PromptText       string
	PromptByLanguage map[string]string
}

func (p *TurnProcessor) SetSilenceReprompt(cfg *SilenceRepromptConfig) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.silenceCfg = cfg
	if cfg != nil && cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = 2
	}
	if cfg != nil && cfg.Timeout <= 0 {
		cfg.Timeout = 10 * time.Second
	}
	if cfg != nil && cfg.PromptText == "" {
		cfg.PromptText = "Halo, apakah Anda masih di line?"
	}
}

func (p *TurnProcessor) Name() string { return "turn_processor" }

func (p *TurnProcessor) Manager() turn.Manager { return p.mgr }

func (p *TurnProcessor) Process(f frames.Frame) ([]frames.Frame, error) {
	if traceID := f.Meta()[frames.MetaTraceID]; traceID != "" {
		p.mu.Lock()
		p.lastTraceID = traceID
		p.mu.Unlock()
	}
	if streamID := f.Meta()[frames.MetaStreamID]; streamID != "" {
		p.lastID = streamID
	}
	var out []frames.Frame
	out = append(out, p.drain()...)
	switch f.Kind() {
	case frames.KindControl:
		cf := f.(frames.ControlFrame)
		if cf.Code() == frames.ControlFlush {
			source := cf.Meta()[frames.MetaSource]
			if source == "stt" || source == "vad" || source == "audio_gate" {
				reason := cf.Meta()[frames.MetaReason]
				if isEndOfTurnReason(reason) {
					p.stopEndOfTurnTimer()
					p.mgr.OnUserSpeechEnd()
				} else {
					p.onUserSpeechStart(cf.Meta()[frames.MetaStreamID])
				}
			}
			p.resetSilenceTimer()
		}
		if cf.Code() == frames.ControlAudioReady {
			p.mgr.OnAudioComplete()
			p.startSilenceTimer()
		}
	case frames.KindText:
		tf := f.(frames.TextFrame)
		if lang := tf.Meta()[frames.MetaLanguage]; lang != "" {
			p.mu.Lock()
			p.lastLanguage = lang
			p.mu.Unlock()
		}
		if tf.Meta()[frames.MetaSource] == "stt" {
			p.resetSilenceTimer()
			if isFinal(tf.Meta()) {
				p.stopEndOfTurnTimer()
				p.mgr.OnUserSpeechEnd()
			} else {
				p.onUserSpeechStart(tf.Meta()[frames.MetaStreamID])
			}
		}
		if tf.Meta()[frames.MetaSource] == "llm" {
			p.mgr.OnAgentSpeechStart()
			p.resetSilenceTimer()
		}
	case frames.KindSystem:
		sf := f.(frames.SystemFrame)
		switch sf.Name() {
		case "thinking_start":
			p.mgr.OnAgentThinkStart()
		case "thinking_end":
			p.mgr.OnAgentThinkEnd()
		case "call_end":
			p.resetSilenceTimer()
			p.stopEndOfTurnTimer()
			p.mu.Lock()
			p.lastTraceID = ""
			p.mu.Unlock()
		}
	}
	out = append(out, f)
	out = append(out, p.drain()...)
	return out, nil
}

func (p *TurnProcessor) drain() []frames.Frame {
	var out []frames.Frame
	for {
		select {
		case f := <-p.emitCh:
			out = append(out, p.ensureStreamID(f))
		default:
			return out
		}
	}
}

func (p *TurnProcessor) ensureStreamID(f frames.Frame) frames.Frame {
	if p.lastID == "" {
		return f
	}
	meta := f.Meta()
	if meta[frames.MetaStreamID] != "" {
		return f
	}
	meta[frames.MetaStreamID] = p.lastID
	if meta[frames.MetaSource] == "" {
		meta[frames.MetaSource] = "turn"
	}
	switch f.Kind() {
	case frames.KindControl:
		cf := f.(frames.ControlFrame)
		return frames.NewControlFrame(p.lastID, cf.PTS(), cf.Code(), meta)
	case frames.KindSystem:
		sf := f.(frames.SystemFrame)
		return frames.NewSystemFrame(p.lastID, sf.PTS(), sf.Name(), meta)
	case frames.KindText:
		tf := f.(frames.TextFrame)
		return frames.NewTextFrame(p.lastID, tf.PTS(), tf.Text(), meta)
	default:
		return f
	}
}

var _ pipeline.FrameProcessor = (*TurnProcessor)(nil)

func (p *TurnProcessor) startSilenceTimer() {
	p.mu.Lock()
	cfg := p.silenceCfg
	if cfg == nil {
		p.mu.Unlock()
		return
	}
	if p.silenceTimer != nil {
		p.silenceTimer.Stop()
	}
	streamID := p.lastID
	timeout := cfg.Timeout
	p.silenceTimer = time.AfterFunc(timeout, func() {
		p.mu.Lock()
		defer p.mu.Unlock()
		if p.silenceCfg == nil {
			return
		}
		if streamID == "" {
			return
		}
		if p.repromptCount >= p.silenceCfg.MaxAttempts {
			return
		}
		p.repromptCount++
		prompt := p.silenceCfg.PromptText
		if p.silenceCfg.PromptByLanguage != nil {
			if lang := strings.ToLower(strings.TrimSpace(p.lastLanguage)); lang != "" {
				if candidate := p.silenceCfg.PromptByLanguage[lang]; candidate != "" {
					prompt = candidate
				}
			}
		}
		meta := map[string]string{
			frames.MetaStreamID:        streamID,
			frames.MetaGreetingText:    prompt,
			frames.MetaRepromptAttempt: fmt.Sprintf("%d", p.repromptCount),
		}
		if lang := strings.ToLower(strings.TrimSpace(p.lastLanguage)); lang != "" {
			meta[frames.MetaLanguage] = lang
		}
		if traceID := strings.TrimSpace(p.lastTraceID); traceID != "" {
			meta[frames.MetaTraceID] = traceID
		}
		sf := frames.NewSystemFrame(streamID, time.Now().UnixNano(), "reprompt", meta)
		// Emit greeting_text system frame; TurnProcessor will ensure stream ID.
		select {
		case p.emitCh <- sf:
		default:
		}
		// Schedule another reprompt if allowed.
		if p.repromptCount < p.silenceCfg.MaxAttempts {
			p.silenceTimer = time.AfterFunc(timeout, func() {
				p.startSilenceTimer()
			})
		}
	})
	p.mu.Unlock()
}

func (p *TurnProcessor) resetSilenceTimer() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.silenceTimer != nil {
		p.silenceTimer.Stop()
		p.silenceTimer = nil
	}
	p.repromptCount = 0
}

func (p *TurnProcessor) onUserSpeechStart(streamID string) {
	p.mgr.OnUserSpeechStart()
	p.startEndOfTurnTimer(streamID)
}

func (p *TurnProcessor) startEndOfTurnTimer(streamID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.endOfTurnTTL <= 0 || streamID == "" {
		return
	}
	if p.endOfTurnTimer != nil {
		p.endOfTurnTimer.Stop()
	}
	p.endOfTurnStream = streamID
	timeout := p.endOfTurnTTL
	p.endOfTurnTimer = time.AfterFunc(timeout, func() {
		p.mu.Lock()
		if p.endOfTurnStream != streamID {
			p.mu.Unlock()
			return
		}
		p.endOfTurnTimer = nil
		p.mu.Unlock()

		p.mgr.OnUserSpeechEnd()
		meta := map[string]string{
			frames.MetaStreamID: streamID,
			frames.MetaSource:   "turn",
			frames.MetaReason:   "speech_timeout",
		}
		p.mu.Lock()
		if traceID := strings.TrimSpace(p.lastTraceID); traceID != "" {
			meta[frames.MetaTraceID] = traceID
		}
		p.mu.Unlock()
		cf := frames.NewControlFrame(streamID, time.Now().UnixNano(), frames.ControlFlush, meta)
		select {
		case p.emitCh <- cf:
		default:
		}
	})
}

func (p *TurnProcessor) stopEndOfTurnTimer() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.endOfTurnTimer != nil {
		p.endOfTurnTimer.Stop()
		p.endOfTurnTimer = nil
	}
	p.endOfTurnStream = ""
}

func isEndOfTurnReason(reason string) bool {
	switch strings.ToLower(strings.TrimSpace(reason)) {
	case "utterance_end", "speech_final", "question", "speech_timeout":
		return true
	default:
		return false
	}
}

type turnEmitter struct {
	out chan frames.Frame
}

func (e *turnEmitter) Emit(frame frames.Frame) error {
	select {
	case e.out <- frame:
	default:
	}
	return nil
}
