package processors

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/harunnryd/ranya/pkg/adapters/stt"
	"github.com/harunnryd/ranya/pkg/errorsx"
	"github.com/harunnryd/ranya/pkg/frames"
	"github.com/harunnryd/ranya/pkg/metrics"
	"github.com/harunnryd/ranya/pkg/pipeline"
	"github.com/harunnryd/ranya/pkg/redact"
	"github.com/harunnryd/ranya/pkg/resilience"
)

type STTProcessor struct {
	mu             sync.Mutex
	sessions       map[string]stt.StreamingSTT
	factory        func(callSID, streamID string) stt.StreamingSTT
	langFactories  map[string]func(callSID, streamID string) stt.StreamingSTT
	defaultLang    string
	codeSwitching  bool
	streamLang     map[string]string
	callStream     map[string]string
	streamCall     map[string]string
	replayCfg      STTReplayConfig
	replay         map[string]*audioReplayBuffer
	ctx            context.Context
	obs            metrics.Observer
	from           map[string]string
	trace          map[string]string
	retry          resilience.RetryPolicy
	breaker        *resilience.CircuitBreaker
	interimLogged  map[string]bool
	forwardInterim bool
	isQuestion     func(string) bool
	provider       string
	breakerOpen    bool
}

type STTReplayConfig struct {
	MaxChunks int
}

type audioChunk struct {
	data     []byte
	rate     int
	channels int
}

type audioReplayBuffer struct {
	maxChunks int
	chunks    []audioChunk
}

func newAudioReplayBuffer(maxChunks int) *audioReplayBuffer {
	if maxChunks <= 0 {
		maxChunks = 0
	}
	return &audioReplayBuffer{maxChunks: maxChunks}
}

func (b *audioReplayBuffer) Add(chunk audioChunk) {
	if b == nil || b.maxChunks <= 0 {
		return
	}
	b.chunks = append(b.chunks, chunk)
	if len(b.chunks) > b.maxChunks {
		b.chunks = b.chunks[len(b.chunks)-b.maxChunks:]
	}
}

func (b *audioReplayBuffer) Snapshot() []audioChunk {
	if b == nil || len(b.chunks) == 0 {
		return nil
	}
	out := make([]audioChunk, len(b.chunks))
	copy(out, b.chunks)
	return out
}

func NewSTTProcessor(factory func(callSID, streamID string) stt.StreamingSTT) *STTProcessor {
	return &STTProcessor{
		sessions:      make(map[string]stt.StreamingSTT),
		factory:       factory,
		langFactories: make(map[string]func(callSID, streamID string) stt.StreamingSTT),
		streamLang:    make(map[string]string),
		callStream:    make(map[string]string),
		streamCall:    make(map[string]string),
		replayCfg:     STTReplayConfig{MaxChunks: 50},
		replay:        make(map[string]*audioReplayBuffer),
		from:          make(map[string]string),
		trace:         make(map[string]string),
		retry:         resilience.NewRetryPolicy(2, 200*time.Millisecond),
		breaker:       resilience.NewCircuitBreaker(3, 30*time.Second),
		interimLogged: make(map[string]bool),
	}
}

// SetLanguageFactories configures per-language STT factories.
func (p *STTProcessor) SetLanguageFactories(factories map[string]func(callSID, streamID string) stt.StreamingSTT, defaultLang string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if factories == nil {
		factories = make(map[string]func(callSID, streamID string) stt.StreamingSTT)
	}
	p.langFactories = factories
	p.defaultLang = defaultLang
}

// SetCodeSwitching toggles whether language changes should keep the same STT session.
func (p *STTProcessor) SetCodeSwitching(enabled bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.codeSwitching = enabled
}

// SetReplayBuffer configures how many recent audio chunks to replay on reconnect.
func (p *STTProcessor) SetReplayBuffer(cfg STTReplayConfig) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if cfg.MaxChunks < 0 {
		cfg.MaxChunks = 0
	}
	p.replayCfg = cfg
	if cfg.MaxChunks == 0 {
		p.replay = make(map[string]*audioReplayBuffer)
	}
}

// SetForwardInterim toggles emitting interim text frames downstream.
func (p *STTProcessor) SetForwardInterim(enabled bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.forwardInterim = enabled
}

func (p *STTProcessor) SetQuestionDetector(fn func(string) bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.isQuestion = fn
}

func (p *STTProcessor) Name() string { return "stt_processor" }

func (p *STTProcessor) SetObserver(obs metrics.Observer) { p.obs = obs }

func (p *STTProcessor) SetContext(ctx context.Context) {
	if ctx != nil {
		p.ctx = ctx
	}
}

func (p *STTProcessor) Process(f frames.Frame) ([]frames.Frame, error) {
	if f.Kind() == frames.KindSystem {
		sf := f.(frames.SystemFrame)
		meta := sf.Meta()
		streamID := meta[frames.MetaStreamID]
		if sf.Name() == "call_end" {
			if streamID == "" {
				streamID = p.streamForCall(meta[frames.MetaCallSID])
			}
			if streamID != "" {
				p.CloseStream(streamID)
			}
			return []frames.Frame{f}, nil
		}
		if lang := meta[frames.MetaGlobalLanguage]; streamID != "" && lang != "" {
			p.setLanguage(streamID, lang)
			p.mu.Lock()
			codeSwitching := p.codeSwitching
			p.mu.Unlock()
			if !codeSwitching && p.hasLangFactories() {
				p.CloseStream(streamID)
			}
		}
		return []frames.Frame{f}, nil
	}
	if f.Kind() != frames.KindAudio {
		return []frames.Frame{f}, nil
	}
	af := f.(frames.AudioFrame)
	meta := af.Meta()
	streamID := meta[frames.MetaStreamID]
	callSID := meta[frames.MetaCallSID]
	p.trackCallStream(callSID, streamID)
	p.addReplay(streamID, af)
	if v := meta[frames.MetaFromNumber]; v != "" {
		p.setFrom(streamID, v)
	}
	if v := meta[frames.MetaTraceID]; v != "" {
		p.setTrace(streamID, v)
	}

	if !p.breaker.Allow() {
		p.recordBreaker(metrics.EventBreakerDenied, streamID, p.getTrace(streamID))
		p.setBreakerOpen(true, streamID, p.getTrace(streamID))
		slog.Info("stt_circuit_open", "stream_id", streamID, "reason_code", string(errorsx.ReasonSTTCircuitOpen))
		frames.ReleaseAudioFrame(f)
		return []frames.Frame{frames.NewControlFrame(streamID, time.Now().UnixNano(), frames.ControlFallback, meta)}, nil
	}
	p.setBreakerOpen(false, streamID, p.getTrace(streamID))

	sttSession, err := p.getOrCreate(streamID, callSID)
	if err != nil {
		err = errorsx.Wrap(err, errorsx.ReasonSTTConnect)
		slog.Info("stt_session_error", "stream_id", streamID, "call_sid", callSID, "reason_code", string(errorsx.Reason(err)), "error", err.Error())
		p.recordRateLimit(err, streamID, p.getTrace(streamID))
		p.breaker.OnError(err)
		frames.ReleaseAudioFrame(f)
		return []frames.Frame{frames.NewControlFrame(streamID, time.Now().UnixNano(), frames.ControlFallback, meta)}, nil
	}
	p.setProviderFromSession(sttSession)
	p.record("stt_audio_in", streamID, p.getTrace(streamID))
	if err := sttSession.SendAudio(af); err != nil {
		err = errorsx.Wrap(err, errorsx.ReasonSTTSend)
		slog.Info("stt_send_error", "stream_id", streamID, "call_sid", callSID, "reason_code", string(errorsx.Reason(err)), "error", err.Error())
		replayed := false
		retryErr := p.retry.Do(func() error {
			p.CloseStream(streamID)
			sttSession, err = p.getOrCreate(streamID, callSID)
			if err != nil {
				return err
			}
			if !replayed {
				p.replayToSession(streamID, sttSession)
				replayed = true
			}
			return sttSession.SendAudio(af)
		})
		if retryErr != nil {
			retryErr = errorsx.Wrap(retryErr, errorsx.ReasonSTTRetry)
			slog.Info("stt_retry_error", "stream_id", streamID, "call_sid", callSID, "reason_code", string(errorsx.Reason(retryErr)), "error", retryErr.Error())
			p.recordRateLimit(retryErr, streamID, p.getTrace(streamID))
			p.breaker.OnError(retryErr)
			frames.ReleaseAudioFrame(f)
			return []frames.Frame{frames.NewControlFrame(streamID, time.Now().UnixNano(), frames.ControlFallback, meta)}, nil
		}
	}
	p.breaker.OnSuccess()
	frames.ReleaseAudioFrame(f)

	// Emit heartbeat to valid pipeline clock
	heartbeat := frames.NewSystemFrame(streamID, af.PTS(), "heartbeat", nil)
	out := []frames.Frame{heartbeat}

	res := p.drainFramesWithSignals(sttSession.Results(), streamID)
	out = append(out, res...)
	out = p.attachFrom(out, streamID)
	for _, e := range out {
		if e.Kind() == frames.KindText {
			p.record("stt_final", streamID, p.getTrace(streamID))
			break
		}
	}
	return out, nil
}

func (p *STTProcessor) getOrCreate(streamID, callSID string) (stt.StreamingSTT, error) {
	lang := p.getLanguage(streamID)
	factory := p.factoryForLang(lang)
	p.mu.Lock()
	defer p.mu.Unlock()
	if sttSession, ok := p.sessions[streamID]; ok {
		return sttSession, nil
	}
	sttSession := factory(callSID, streamID)
	if p.ctx == nil {
		p.ctx = context.Background()
	}
	if err := sttSession.Start(p.ctx); err != nil {
		return nil, err
	}
	p.sessions[streamID] = sttSession
	return sttSession, nil
}

func (p *STTProcessor) CloseStream(streamID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if sttSession, ok := p.sessions[streamID]; ok {
		_ = sttSession.Close()
		delete(p.sessions, streamID)
	}
	if callSID := p.streamCall[streamID]; callSID != "" {
		if p.callStream[callSID] == streamID {
			delete(p.callStream, callSID)
		}
		delete(p.streamCall, streamID)
	}
	delete(p.from, streamID)
	delete(p.trace, streamID)
	delete(p.streamLang, streamID)
	delete(p.replay, streamID)
}

func (p *STTProcessor) streamForCall(callSID string) string {
	if callSID == "" {
		return ""
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.callStream[callSID]
}

func (p *STTProcessor) CloseAll() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for id, sttSession := range p.sessions {
		_ = sttSession.Close()
		delete(p.sessions, id)
	}
	p.from = make(map[string]string)
	p.trace = make(map[string]string)
	p.streamLang = make(map[string]string)
	p.callStream = make(map[string]string)
	p.streamCall = make(map[string]string)
	p.replay = make(map[string]*audioReplayBuffer)
}

func (p *STTProcessor) drainFramesWithSignals(ch <-chan frames.Frame, streamID string) []frames.Frame {
	var out []frames.Frame
	for {
		select {
		case f, ok := <-ch:
			if !ok {
				return out
			}
			if f.Kind() == frames.KindText {
				tf := f.(frames.TextFrame)
				p.mu.Lock()
				isQ := p.isQuestion != nil && p.isQuestion(tf.Text())
				forwardInterim := p.forwardInterim
				p.mu.Unlock()
				if isQ {
					meta := map[string]string{frames.MetaSource: "stt", frames.MetaReason: "question"}
					if traceID := p.getTrace(streamID); traceID != "" {
						meta[frames.MetaTraceID] = traceID
					}
					out = append(out, frames.NewControlFrame(streamID, time.Now().UnixNano(), frames.ControlFlush, meta))
				}
				if tf.Meta()[frames.MetaIsFinal] != "true" {
					p.logInterim(streamID, tf.Text())
					if forwardInterim {
						out = append(out, tf)
					}
					continue
				}
				p.logFinal(streamID, tf.Text())
				out = append(out, tf)
				continue
			}
			out = append(out, f)
		default:
			return out
		}
	}
}

var _ pipeline.FrameProcessor = (*STTProcessor)(nil)

func (p *STTProcessor) record(name, streamID, traceID string) {
	if p.obs == nil {
		return
	}
	tags := map[string]string{frames.MetaStreamID: streamID, "component": "stt"}
	if traceID != "" {
		tags[frames.MetaTraceID] = traceID
	}
	if callSID := p.getCallSID(streamID); callSID != "" {
		tags[frames.MetaCallSID] = callSID
	}
	if p.provider != "" {
		tags["provider"] = p.provider
	}
	p.obs.RecordEvent(metrics.MetricsEvent{
		Name: name,
		Time: time.Now(),
		Tags: tags,
	})
}

func (p *STTProcessor) setLanguage(streamID, lang string) {
	if streamID == "" || lang == "" {
		return
	}
	p.mu.Lock()
	p.streamLang[streamID] = strings.ToLower(strings.TrimSpace(lang))
	p.mu.Unlock()
}

func (p *STTProcessor) trackCallStream(callSID, streamID string) {
	if callSID == "" || streamID == "" {
		return
	}
	p.mu.Lock()
	prev := p.callStream[callSID]
	if prev != "" && prev != streamID {
		p.mu.Unlock()
		p.CloseStream(prev)
		p.mu.Lock()
	}
	p.callStream[callSID] = streamID
	p.streamCall[streamID] = callSID
	p.mu.Unlock()
}

func (p *STTProcessor) getLanguage(streamID string) string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if lang := p.streamLang[streamID]; lang != "" {
		return lang
	}
	return p.defaultLang
}

func (p *STTProcessor) factoryForLang(lang string) func(callSID, streamID string) stt.StreamingSTT {
	p.mu.Lock()
	defer p.mu.Unlock()
	if lang != "" && p.langFactories != nil {
		if factory, ok := p.langFactories[lang]; ok && factory != nil {
			return factory
		}
	}
	return p.factory
}

func (p *STTProcessor) hasLangFactories() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.langFactories) > 0
}

func (p *STTProcessor) recordWithFields(name, streamID, traceID string, fields map[string]any) {
	if p.obs == nil {
		return
	}
	tags := map[string]string{frames.MetaStreamID: streamID, "component": "stt"}
	if traceID != "" {
		tags[frames.MetaTraceID] = traceID
	}
	if callSID := p.getCallSID(streamID); callSID != "" {
		tags[frames.MetaCallSID] = callSID
	}
	if p.provider != "" {
		tags["provider"] = p.provider
	}
	p.obs.RecordEvent(metrics.MetricsEvent{
		Name:   name,
		Time:   time.Now(),
		Tags:   tags,
		Fields: fields,
	})
}

func (p *STTProcessor) getCallSID(streamID string) string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.streamCall[streamID]
}

func (p *STTProcessor) addReplay(streamID string, af frames.AudioFrame) {
	if streamID == "" {
		return
	}
	p.mu.Lock()
	cfg := p.replayCfg
	buf := p.replay[streamID]
	if cfg.MaxChunks <= 0 {
		p.mu.Unlock()
		return
	}
	if buf == nil {
		buf = newAudioReplayBuffer(cfg.MaxChunks)
		p.replay[streamID] = buf
	}
	p.mu.Unlock()

	chunk := audioChunk{
		data:     append([]byte(nil), af.RawPayload()...),
		rate:     af.Rate(),
		channels: af.Channels(),
	}
	p.mu.Lock()
	buf.Add(chunk)
	p.mu.Unlock()
}

func (p *STTProcessor) replayToSession(streamID string, sess stt.StreamingSTT) {
	if sess == nil || streamID == "" {
		return
	}
	p.mu.Lock()
	buf := p.replay[streamID]
	p.mu.Unlock()
	if buf == nil {
		return
	}
	chunks := buf.Snapshot()
	for _, chunk := range chunks {
		if len(chunk.data) == 0 {
			continue
		}
		af := frames.NewAudioFrame(streamID, time.Now().UnixNano(), chunk.data, chunk.rate, chunk.channels, nil)
		_ = sess.SendAudio(af)
	}
}

func (p *STTProcessor) recordBreaker(name, streamID, traceID string) {
	p.record(name, streamID, traceID)
}

func (p *STTProcessor) recordRateLimit(err error, streamID, traceID string) {
	if err == nil {
		return
	}
	if resilience.IsRateLimit(err) {
		p.record(metrics.EventRateLimit, streamID, traceID)
	}
}

func (p *STTProcessor) setProviderFromSession(sess stt.StreamingSTT) {
	if sess == nil || p.provider != "" {
		return
	}
	p.provider = sess.Name()
}

func (p *STTProcessor) setBreakerOpen(open bool, streamID, traceID string) {
	if p.breakerOpen == open {
		return
	}
	p.breakerOpen = open
	if open {
		p.recordBreaker(metrics.EventBreakerOpen, streamID, traceID)
		return
	}
	p.recordBreaker(metrics.EventBreakerClose, streamID, traceID)
}

func (p *STTProcessor) setFrom(streamID, from string) {
	p.mu.Lock()
	p.from[streamID] = from
	p.mu.Unlock()
}

func (p *STTProcessor) setTrace(streamID, traceID string) {
	p.mu.Lock()
	p.trace[streamID] = traceID
	p.mu.Unlock()
}

func (p *STTProcessor) getTrace(streamID string) string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.trace[streamID]
}

func (p *STTProcessor) attachFrom(framesIn []frames.Frame, streamID string) []frames.Frame {
	p.mu.Lock()
	from := p.from[streamID]
	traceID := p.trace[streamID]
	p.mu.Unlock()
	if from == "" {
		if traceID == "" {
			return framesIn
		}
	}
	out := make([]frames.Frame, 0, len(framesIn))
	for _, f := range framesIn {
		if f.Kind() != frames.KindText {
			out = append(out, f)
			continue
		}
		tf := f.(frames.TextFrame)
		meta := tf.Meta()
		if meta[frames.MetaFromNumber] == "" {
			meta[frames.MetaFromNumber] = from
		}
		if meta[frames.MetaTraceID] == "" && traceID != "" {
			meta[frames.MetaTraceID] = traceID
		}
		out = append(out, frames.NewTextFrame(streamID, tf.PTS(), tf.Text(), meta))
	}
	return out
}

func (p *STTProcessor) logInterim(streamID, text string) {
	p.mu.Lock()
	if p.interimLogged[streamID] {
		p.mu.Unlock()
		return
	}
	p.interimLogged[streamID] = true
	traceID := p.trace[streamID]
	p.mu.Unlock()
	safe := redact.Text(text)
	slog.Info("stt_interim", "stream_id", streamID, "trace_id", traceID, "text", clipText(safe))
}

func (p *STTProcessor) logFinal(streamID, text string) {
	traceID := p.getTrace(streamID)
	safe := redact.Text(text)
	slog.Info("stt_final", "stream_id", streamID, "trace_id", traceID, "text", clipText(safe))
	p.recordWithFields("stt_final_text", streamID, traceID, map[string]any{"text": safe})
}

func clipText(text string) string {
	text = strings.TrimSpace(text)
	if len(text) <= 120 {
		return text
	}
	return text[:120] + "..."
}
