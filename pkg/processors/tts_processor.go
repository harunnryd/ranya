package processors

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/harunnryd/ranya/pkg/adapters/tts"
	"github.com/harunnryd/ranya/pkg/errorsx"
	"github.com/harunnryd/ranya/pkg/frames"
	"github.com/harunnryd/ranya/pkg/logging"
	"github.com/harunnryd/ranya/pkg/metrics"
	"github.com/harunnryd/ranya/pkg/pipeline"
	"github.com/harunnryd/ranya/pkg/redact"
	"github.com/harunnryd/ranya/pkg/resilience"
)

type TTSProcessor struct {
	mu            sync.Mutex
	sessions      map[string]tts.StreamingTTS
	factory       func(callSID, streamID string) tts.StreamingTTS
	langFactories map[string]func(callSID, streamID string) tts.StreamingTTS
	defaultLang   string
	ctx           context.Context
	obs           metrics.Observer
	first         map[string]bool
	trace         map[string]string
	callStream    map[string]string
	streamCall    map[string]string

	// Native parameters (optional)
	outputFormat string

	// Error handling
	breaker  *resilience.CircuitBreaker
	retry    resilience.RetryPolicy
	open     bool
	provider string

	// Structured logging
	logger *slog.Logger
}

type flushSender interface {
	SendTextWithOptions(text string, flush bool) error
}

func NewTTSProcessor(factory func(callSID, streamID string) tts.StreamingTTS) *TTSProcessor {
	return &TTSProcessor{
		sessions:      make(map[string]tts.StreamingTTS),
		factory:       factory,
		langFactories: make(map[string]func(callSID, streamID string) tts.StreamingTTS),
		first:         make(map[string]bool),
		trace:         make(map[string]string),
		callStream:    make(map[string]string),
		streamCall:    make(map[string]string),
		outputFormat:  "ulaw_8000",
		breaker:       resilience.NewCircuitBreaker(3, 30*time.Second),
		retry:         resilience.NewRetryPolicy(2, 200*time.Millisecond),
		logger:        logging.NewComponentLogger(slog.Default(), "tts_processor"),
	}
}

func (p *TTSProcessor) Name() string { return "tts_processor" }

func (p *TTSProcessor) SetObserver(obs metrics.Observer) { p.obs = obs }

func (p *TTSProcessor) SetContext(ctx context.Context) {
	if ctx != nil {
		p.ctx = ctx
	}
}

// SetLanguageFactories configures per-language TTS factories.
func (p *TTSProcessor) SetLanguageFactories(factories map[string]func(callSID, streamID string) tts.StreamingTTS, defaultLang string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if factories == nil {
		factories = make(map[string]func(callSID, streamID string) tts.StreamingTTS)
	}
	p.langFactories = factories
	p.defaultLang = defaultLang
}

// SetOutputFormat configures the output format for TTS logging/metrics.
func (p *TTSProcessor) SetOutputFormat(format string) {
	p.outputFormat = format
	p.logger.Info("tts output format configured",
		slog.String("output_format", format))
}

// SetLogger configures structured logging for the TTS processor.
func (p *TTSProcessor) SetLogger(logger *slog.Logger) {
	if logger != nil {
		p.logger = logging.NewComponentLogger(logger, "tts_processor")
	}
}

func (p *TTSProcessor) trackCallStream(callSID, streamID string) {
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

func (p *TTSProcessor) Process(f frames.Frame) ([]frames.Frame, error) {
	streamID := f.Meta()[frames.MetaStreamID]
	if callSID := f.Meta()[frames.MetaCallSID]; callSID != "" {
		p.trackCallStream(callSID, streamID)
	}
	lang := f.Meta()[frames.MetaLanguage]
	if lang == "" {
		lang = p.defaultLang
	}
	var out []frames.Frame

	if f.Kind() == frames.KindSystem {
		sf := f.(frames.SystemFrame)
		if sf.Name() == "call_end" {
			if streamID == "" {
				streamID = p.streamForCall(sf.Meta()[frames.MetaCallSID])
			}
			if streamID != "" {
				p.CloseStream(streamID)
			}
			return []frames.Frame{f}, nil
		}
	}

	if f.Kind() == frames.KindControl {
		cf := f.(frames.ControlFrame)
		if cf.Code() == frames.ControlStartInterruption {
			p.withSessions(streamID, func(ttsSession tts.StreamingTTS) {
				ttsSession.Flush()
				p.logger.Info("tts interruption received",
					slog.String("stream_id", streamID))
			})
			out = append(out, f)
			return out, nil
		}
	}

	// Helper to drain TTS
	drain := func() {
		res := p.drainAll(streamID)
		if len(res) > 0 {
			p.logger.Debug("tts results drained",
				slog.String("stream_id", streamID),
				slog.Int("count", len(res)))
			p.recordFirst(streamID)
			out = append(out, res...)
		}
	}

	switch f.Kind() {
	case frames.KindControl:
		drain()
		cf := f.(frames.ControlFrame)
		if cf.Code() == frames.ControlFlush {
			p.withSessions(streamID, func(ttsSession tts.StreamingTTS) {
				ttsSession.Flush()
				p.logger.Info("tts flush signal received",
					slog.String("stream_id", streamID))
			})
		} else if cf.Code() == frames.ControlCancel {
			p.logger.Info("tts cancel signal received",
				slog.String("stream_id", streamID))
			p.CloseStream(streamID)
		} else if cf.Code() == frames.ControlFallback {
			p.logger.Info("tts fallback signal received",
				slog.String("stream_id", streamID))
			p.CloseStream(streamID)
		} else if cf.Code() == frames.ControlAudioReady {
			p.logger.Debug("tts webhook flush",
				slog.String("stream_id", cf.Meta()[frames.MetaStreamID]))
			drain()
		}
		out = append(out, f)
		return out, nil

	case frames.KindText:
		tf := f.(frames.TextFrame)
		meta := tf.Meta()
		callSID := meta[frames.MetaCallSID]
		if traceID := meta[frames.MetaTraceID]; traceID != "" {
			p.setTrace(streamID, traceID)
		}
		flushRequested := meta[frames.MetaTTSFlush] == "true"
		if strings.TrimSpace(tf.Text()) == "" {
			if flushRequested {
				p.withSessions(streamID, func(ttsSession tts.StreamingTTS) {
					if sender, ok := ttsSession.(flushSender); ok {
						_ = sender.SendTextWithOptions("", true)
					} else {
						ttsSession.Flush()
					}
					p.logger.Info("tts flush requested",
						slog.String("stream_id", streamID))
				})
			}
			return out, nil
		}

		// Check circuit breaker
		if !p.breaker.Allow() {
			p.recordBreaker(metrics.EventBreakerDenied, streamID)
			p.setBreakerOpen(true, streamID)
			p.logger.Warn("tts circuit breaker open",
				slog.String("stream_id", streamID),
				slog.String("reason_code", string(errorsx.ReasonTTSCircuitOpen)),
				slog.String("reason", "rate_limit_protection"))
			fallback := frames.NewControlFrame(streamID, time.Now().UnixNano(), frames.ControlFallback, meta)
			drain()
			out = append(out, fallback)
			return out, nil
		}
		p.setBreakerOpen(false, streamID)

		// Get or create TTS session
		ttsSession, err := p.getOrCreate(streamID, callSID, lang)
		if err != nil {
			err = errorsx.Wrap(err, errorsx.ReasonTTSConnect)
			p.logger.Error("tts connection failed",
				slog.String("stream_id", streamID),
				slog.String("reason_code", string(errorsx.Reason(err))),
				slog.String("error", err.Error()))
			p.recordRateLimit(err, streamID)
			p.breaker.OnError(err)
			fallback := frames.NewControlFrame(streamID, time.Now().UnixNano(), frames.ControlFallback, meta)
			drain()
			out = append(out, fallback)
			return out, nil
		}

		// Log TTS request (redacted)
		safeText := redact.Text(tf.Text())
		p.logger.Info("tts request",
			slog.String("stream_id", streamID),
			slog.String("text", clipTTSText(safeText)),
			slog.Int("text_length", len(tf.Text())),
			slog.String("output_format", p.outputFormat))

		// Send text (flush if requested and supported)
		if flushRequested {
			if sender, ok := ttsSession.(flushSender); ok {
				err = sender.SendTextWithOptions(tf.Text(), true)
			} else {
				err = ttsSession.SendText(tf.Text())
			}
		} else {
			err = ttsSession.SendText(tf.Text())
		}
		if err != nil {
			err = errorsx.Wrap(err, errorsx.ReasonTTSSend)
			p.logger.Error("tts send failed",
				slog.String("stream_id", streamID),
				slog.String("reason_code", string(errorsx.Reason(err))),
				slog.String("error", err.Error()))

			// Retry with exponential backoff
			retryErr := p.retry.Do(func() error {
				p.CloseStream(streamID)
				ttsSession, err = p.getOrCreate(streamID, callSID, lang)
				if err != nil {
					return err
				}
				return ttsSession.SendText(tf.Text())
			})
			if retryErr != nil {
				err = retryErr
			} else {
				err = nil
			}
		}

		if err != nil {
			err = errorsx.Wrap(err, errorsx.ReasonTTSRetry)
			p.logger.Error("tts send failed after retry",
				slog.String("stream_id", streamID),
				slog.String("reason_code", string(errorsx.Reason(err))),
				slog.String("error", err.Error()),
				slog.Int("max_retries", p.retry.MaxRetries))
			p.recordRateLimit(err, streamID)
			p.breaker.OnError(err)
			fallback := frames.NewControlFrame(streamID, time.Now().UnixNano(), frames.ControlFallback, meta)
			drain()
			out = append(out, fallback)
			return out, nil
		}

		p.breaker.OnSuccess()
		p.logger.Debug("tts request successful",
			slog.String("stream_id", streamID))
		if flushRequested {
			if sender, ok := ttsSession.(flushSender); !ok || sender == nil {
				ttsSession.Flush()
			}
			p.logger.Info("tts flush requested",
				slog.String("stream_id", streamID))
		}
		drain()
		return out, nil

	default:
		drain()
		out = append(out, f)
		return out, nil
	}
}

func (p *TTSProcessor) getOrCreate(streamID, callSID, lang string) (tts.StreamingTTS, error) {
	key := sessionKey(streamID, lang)
	factory := p.factoryForLang(lang)
	p.mu.Lock()
	defer p.mu.Unlock()
	if ttsSession, ok := p.sessions[key]; ok {
		return ttsSession, nil
	}

	p.logger.Debug("creating new TTS session",
		slog.String("stream_id", streamID),
		slog.String("call_sid", callSID))

	ttsSession := factory(callSID, streamID)
	if p.ctx == nil {
		p.ctx = context.Background()
	}
	if err := ttsSession.Start(p.ctx); err != nil {
		p.logger.Error("failed to start TTS session",
			slog.String("stream_id", streamID),
			slog.String("error", err.Error()))
		return nil, err
	}

	p.logger.Info("TTS session created",
		slog.String("stream_id", streamID),
		slog.String("output_format", p.outputFormat))

	p.sessions[key] = ttsSession
	if p.provider == "" {
		p.provider = ttsSession.Name()
	}
	return ttsSession, nil
}

func (p *TTSProcessor) CloseStream(streamID string) {
	// Guard: Ignore empty stream ID (Ghost Close fix)
	if streamID == "" {
		p.logger.Debug("tts close stream ignored - empty stream ID")
		return
	}
	p.logger.Debug("tts close stream called",
		slog.String("stream_id", streamID))
	p.mu.Lock()
	defer p.mu.Unlock()
	for key, ttsSession := range p.sessions {
		if strings.HasPrefix(key, streamID+"|") || key == streamID {
			_ = ttsSession.Close()
			delete(p.sessions, key)
		}
	}
	if callSID := p.streamCall[streamID]; callSID != "" {
		if p.callStream[callSID] == streamID {
			delete(p.callStream, callSID)
		}
		delete(p.streamCall, streamID)
	}
	delete(p.first, streamID)
	delete(p.trace, streamID)
}

func (p *TTSProcessor) streamForCall(callSID string) string {
	if callSID == "" {
		return ""
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.callStream[callSID]
}

func (p *TTSProcessor) CloseAll() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for id, ttsSession := range p.sessions {
		_ = ttsSession.Close()
		delete(p.sessions, id)
	}
	p.first = make(map[string]bool)
	p.trace = make(map[string]string)
	p.callStream = make(map[string]string)
	p.streamCall = make(map[string]string)
}

func drainTTS(ch <-chan frames.Frame) []frames.Frame {
	var out []frames.Frame
	for {
		select {
		case f, ok := <-ch:
			if !ok {
				return out
			}
			out = append(out, f)
		default:
			return out
		}
	}
}

var _ pipeline.FrameProcessor = (*TTSProcessor)(nil)

func sessionKey(streamID, lang string) string {
	if streamID == "" {
		return ""
	}
	if lang == "" {
		return streamID
	}
	return streamID + "|" + lang
}

func (p *TTSProcessor) factoryForLang(lang string) func(callSID, streamID string) tts.StreamingTTS {
	p.mu.Lock()
	defer p.mu.Unlock()
	if lang != "" && p.langFactories != nil {
		if factory, ok := p.langFactories[lang]; ok && factory != nil {
			return factory
		}
	}
	return p.factory
}

func (p *TTSProcessor) session(streamID, lang string) (tts.StreamingTTS, bool) {
	key := sessionKey(streamID, lang)
	p.mu.Lock()
	defer p.mu.Unlock()
	ttsSession, ok := p.sessions[key]
	return ttsSession, ok
}

func (p *TTSProcessor) withSessions(streamID string, fn func(tts.StreamingTTS)) {
	if streamID == "" {
		return
	}
	p.mu.Lock()
	var sessions []tts.StreamingTTS
	for key, sess := range p.sessions {
		if key == streamID || strings.HasPrefix(key, streamID+"|") {
			sessions = append(sessions, sess)
		}
	}
	p.mu.Unlock()
	for _, sess := range sessions {
		fn(sess)
	}
}

func (p *TTSProcessor) drainAll(streamID string) []frames.Frame {
	var out []frames.Frame
	p.withSessions(streamID, func(sess tts.StreamingTTS) {
		out = append(out, drainTTS(sess.Results())...)
	})
	return out
}

func (p *TTSProcessor) recordFirst(streamID string) {
	if p.obs == nil {
		return
	}
	traceID := p.getTrace(streamID)
	p.mu.Lock()
	if p.first[streamID] {
		p.mu.Unlock()
		return
	}
	p.first[streamID] = true
	p.mu.Unlock()
	p.obs.RecordEvent(metrics.MetricsEvent{
		Name: "tts_first_audio",
		Time: time.Now(),
		Tags: p.baseTags(streamID, traceID),
	})
}

func (p *TTSProcessor) setTrace(streamID, traceID string) {
	if traceID == "" {
		return
	}
	p.mu.Lock()
	p.trace[streamID] = traceID
	p.mu.Unlock()
}

func (p *TTSProcessor) getTrace(streamID string) string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.trace[streamID]
}

func clipTTSText(text string) string {
	text = strings.TrimSpace(text)
	if len(text) <= 120 {
		return text
	}
	return text[:120] + "..."
}

func (p *TTSProcessor) baseTags(streamID, traceID string) map[string]string {
	tags := map[string]string{frames.MetaStreamID: streamID, "component": "tts"}
	if traceID != "" {
		tags[frames.MetaTraceID] = traceID
	}
	if callSID := p.callSIDForStream(streamID); callSID != "" {
		tags[frames.MetaCallSID] = callSID
	}
	if p.provider != "" {
		tags["provider"] = p.provider
	}
	return tags
}

func (p *TTSProcessor) callSIDForStream(streamID string) string {
	if streamID == "" {
		return ""
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.streamCall[streamID]
}

func (p *TTSProcessor) recordBreaker(name, streamID string) {
	if p.obs == nil {
		return
	}
	traceID := p.getTrace(streamID)
	p.obs.RecordEvent(metrics.MetricsEvent{
		Name: name,
		Time: time.Now(),
		Tags: p.baseTags(streamID, traceID),
	})
}

func (p *TTSProcessor) recordRateLimit(err error, streamID string) {
	if err == nil {
		return
	}
	if resilience.IsRateLimit(err) {
		p.recordBreaker(metrics.EventRateLimit, streamID)
	}
}

func (p *TTSProcessor) setBreakerOpen(open bool, streamID string) {
	if p.open == open {
		return
	}
	p.open = open
	if open {
		p.recordBreaker(metrics.EventBreakerOpen, streamID)
		return
	}
	p.recordBreaker(metrics.EventBreakerClose, streamID)
}
