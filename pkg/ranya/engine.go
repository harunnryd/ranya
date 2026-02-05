package ranya

import (
	"context"
	"encoding/base64"
	"fmt"
	"log/slog"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/harunnryd/ranya/pkg/adapters/stt"
	"github.com/harunnryd/ranya/pkg/adapters/tts"
	"github.com/harunnryd/ranya/pkg/aggregators"
	"github.com/harunnryd/ranya/pkg/frames"
	"github.com/harunnryd/ranya/pkg/llm"
	"github.com/harunnryd/ranya/pkg/metrics"
	"github.com/harunnryd/ranya/pkg/observers"
	"github.com/harunnryd/ranya/pkg/pipeline"
	"github.com/harunnryd/ranya/pkg/processors"
	"github.com/harunnryd/ranya/pkg/redact"
	"github.com/harunnryd/ranya/pkg/runner"
	"github.com/harunnryd/ranya/pkg/transports"
	"github.com/harunnryd/ranya/pkg/turn"
)

type Engine struct {
	cfg       Config
	registry  *pipeline.SessionRegistry
	transport transports.Transport
	providers *ProviderRegistry
	runner    *pipeline.Runner
	asyncObs  *metrics.AsyncObserver
	ctx       context.Context
	cancel    context.CancelFunc

	// Customization
	tools  llm.ToolRegistry
	agents map[string]processors.AgentConfig
	router processors.RouterStrategy
}

type EngineOptions struct {
	Config    Config
	Providers *ProviderRegistry
	Transport transports.Transport
	Tools     llm.ToolRegistry
	Agents    map[string]processors.AgentConfig
	Router    processors.RouterStrategy
	Filler    pipeline.FrameProcessor
	// Optional hooks and extensions.
	PreProcessors      []pipeline.FrameProcessor
	BeforeContext      []pipeline.FrameProcessor
	BeforeLLM          []pipeline.FrameProcessor
	BeforeTTS          []pipeline.FrameProcessor
	PostProcessors     []pipeline.FrameProcessor
	QuestionDetector   func(string) bool
	ContextCaption     string
	LanguageDetector   processors.LanguageDetector
	LanguagePrompts    map[string]string
	LanguageMinConf    float64
	DefaultLanguage    string
	TTSFactoriesByLang map[string]func(callSID, streamID string) tts.StreamingTTS
	STTFactoriesByLang map[string]func(callSID, streamID string) stt.StreamingSTT
	ToolOptions        ToolDispatcherOptions
	SilenceReprompt    *processors.SilenceRepromptConfig
}

func NewEngine(opts EngineOptions) *Engine {
	cfg := opts.Config
	SetDefaultLogger(cfg.LogLevel)
	redact.SetEnabled(cfg.Privacy.RedactPII)

	slog.Info("ranya_init",
		"environment", cfg.Environment,
		"llm_provider", cfg.Vendors.LLM.Provider,
		"stt_provider", cfg.Vendors.STT.Provider,
		"tts_provider", cfg.Vendors.TTS.Provider,
		"transport", cfg.Transports.Provider,
	)

	// Logging Configuration
	pipeline.LogConfiguration(cfg.Engine)
	latencyObs := observers.NewLatencyObserver(slog.Default())
	logObs := observers.NewLoggerObserver(slog.Default())
	var timelineObs *observers.TimelineObserver
	var costObs *observers.CostObserver
	obsList := []metrics.Observer{latencyObs, logObs}
	if dir := strings.TrimSpace(cfg.Observability.ArtifactsDir); dir != "" {
		if cfg.Observability.RetentionDays > 0 {
			_, _ = observers.PurgeArtifacts(dir, time.Duration(cfg.Observability.RetentionDays)*24*time.Hour)
		}
		timelineObs = observers.NewTimelineObserver(dir)
		costObs = observers.NewCostObserver(dir)
		obsList = append(obsList, timelineObs, costObs)
	}
	multiObs := observers.NewMultiObserver(obsList...)
	asyncObs := metrics.NewAsyncObserver(multiObs, 2048)

	providers := opts.Providers
	if providers == nil {
		providers = NewProviderRegistry()
	}

	var sink func(frames.Frame)
	if opts.Transport != nil {
		sink = func(f frames.Frame) {
			if asyncObs != nil && f.Kind() == frames.KindAudio {
				af := f.(frames.AudioFrame)
				meta := f.Meta()
				fields := map[string]any{
					"sample_rate": af.Rate(),
					"channels":    af.Channels(),
				}
				if cfg.Observability.RecordAudio {
					fields["payload_b64"] = base64.StdEncoding.EncodeToString(af.RawPayload())
				}
				tags := map[string]string{
					"stream_id":        meta[frames.MetaStreamID],
					frames.MetaTraceID: meta[frames.MetaTraceID],
					frames.MetaCallSID: meta[frames.MetaCallSID],
					"component":        "transport",
				}
				asyncObs.RecordEvent(metrics.MetricsEvent{
					Name:   "audio_out",
					Time:   time.Now(),
					Tags:   tags,
					Fields: fields,
				})
			}
			_ = opts.Transport.Send(f)
		}
	}

	// Registry Factory
	registry := pipeline.NewSessionRegistry(func(ctx context.Context, callSID, streamID, traceID string) (pipeline.Orchestrator, error) {
		// Build STT processor.
		sttFactory, err := providers.BuildSTTFactory(cfg.Vendors.STT.Provider, cfg, traceID)
		if err != nil {
			return nil, err
		}
		sttProc := processors.NewSTTProcessor(sttFactory)
		sttFactories := opts.STTFactoriesByLang
		if len(sttFactories) == 0 {
			sttFactories = buildSTTLanguageFactories(cfg, providers, traceID)
		}
		if len(sttFactories) > 0 {
			sttProc.SetLanguageFactories(sttFactories, defaultLanguage(cfg, opts.DefaultLanguage))
		}
		sttProc.SetCodeSwitching(cfg.Languages.CodeSwitching)
		sttProc.SetForwardInterim(cfg.STT.ForwardInterim)
		if cfg.Engine.STTReplayChunks > 0 {
			sttProc.SetReplayBuffer(processors.STTReplayConfig{MaxChunks: cfg.Engine.STTReplayChunks})
		} else {
			sttProc.SetReplayBuffer(processors.STTReplayConfig{MaxChunks: 0})
		}
		if opts.QuestionDetector != nil {
			sttProc.SetQuestionDetector(opts.QuestionDetector)
		}
		sttProc.SetObserver(asyncObs)
		sttProc.SetContext(ctx)

		// Build LLM processor.
		llmAdapter, err := providers.BuildLLM(cfg.Vendors.LLM.Provider, cfg)
		if err != nil {
			return nil, err
		}

		var tools []llm.Tool
		if opts.Tools != nil {
			tools = opts.Tools.Tools()
		}

		llmProc := processors.NewLLMProcessor(llmAdapter, "", tools)
		if cfg.Context.MaxHistory > 0 || cfg.Context.MaxTokens > 0 {
			llmProc.SetMemoryLimits(cfg.Context.MaxHistory, cfg.Context.MaxTokens)
		}
		if cfg.Confirmation.LLMFallback {
			llmProc.SetConfirmationOptions(cfg.Confirmation.Mode, cfg.Confirmation.LLMFallback, time.Duration(cfg.Confirmation.TimeoutMS)*time.Millisecond)
		}
		if opts.Agents != nil {
			defaultAgent := "triage"
			if _, ok := opts.Agents["triage"]; !ok {
				for k := range opts.Agents {
					defaultAgent = k
					break
				}
			}
			llmProc.SetAgents(opts.Agents, defaultAgent)
		}
		llmProc.SetObserver(asyncObs)
		llmProc.SetContext(ctx)

		// Build TTS processor.
		ttsFactory, err := providers.BuildTTSFactory(cfg.Vendors.TTS.Provider, cfg)
		if err != nil {
			return nil, err
		}
		ttsProc := processors.NewTTSProcessor(ttsFactory)
		ttsFactories := opts.TTSFactoriesByLang
		if len(ttsFactories) == 0 {
			ttsFactories = buildTTSLanguageFactories(cfg, providers)
		}
		if len(ttsFactories) > 0 {
			ttsProc.SetLanguageFactories(ttsFactories, defaultLanguage(cfg, opts.DefaultLanguage))
		}
		ttsProc.SetObserver(asyncObs)
		ttsProc.SetContext(ctx)

		// 4. Dispatcher
		toolOpts := opts.ToolOptions
		if isZeroToolOptions(toolOpts) {
			toolOpts = toolOptionsFromConfig(cfg)
		}
		dispatcher := NewToolDispatcherWithOptions(opts.Tools, nil, toolOpts)

		// 5. Context / Aggregator
		maxHistory := 10
		if cfg.Context.MaxHistory > 0 {
			maxHistory = cfg.Context.MaxHistory
		}
		ctxProc := processors.NewContextProcessor(aggregators.AggregatorConfig{
			MinLen:       2,
			MaxTokens:    128,
			MaxHistory:   maxHistory,
			FlushTimeout: 400 * time.Millisecond,
		}, cfg.BasePrompt)
		if opts.ContextCaption != "" {
			ctxProc.SetDefaultCaption(opts.ContextCaption)
		}

		// Turn management (barge-in, interruption) + speculative buffering.
		turnCfg := processors.TurnProcessorConfig{
			BargeInThreshold: time.Duration(cfg.Turn.BargeInThresholdMS) * time.Millisecond,
			MinBargeIn:       time.Duration(cfg.Turn.MinBargeInMS) * time.Millisecond,
			EndOfTurnTimeout: time.Duration(cfg.Turn.EndOfTurnTimeoutMS) * time.Millisecond,
		}
		turnProc := processors.NewTurnProcessorWithConfig(turn.AggressiveStrategy{}, turnCfg)
		if opts.SilenceReprompt != nil {
			turnProc.SetSilenceReprompt(opts.SilenceReprompt)
		} else if reprompt := silenceRepromptFromConfig(cfg); reprompt != nil {
			turnProc.SetSilenceReprompt(reprompt)
		}
		ctxProc.SetTurnManager(turnProc.Manager())

		builder := pipeline.NewVoiceAgentBuilder()
		for _, p := range opts.PreProcessors {
			if p != nil {
				builder = builder.WithAcoustic(p)
			}
		}
		beforeTTS := append([]pipeline.FrameProcessor{}, opts.BeforeTTS...)
		if cfg.Summary.Enabled {
			summaryProc := processors.NewSummaryProcessor(processors.SummaryConfig{
				MaxEntries: cfg.Summary.MaxEntries,
				MaxChars:   cfg.Summary.MaxChars,
			})
			summaryProc.SetObserver(asyncObs)
			beforeTTS = append(beforeTTS, summaryProc)
		}
		builder = builder.WithSTT(sttProc).
			WithTurnManager(turnProc).
			WithProcessorList(opts.BeforeContext).
			WithContext(ctxProc).
			WithRouter(configureRouter(opts)).
			WithProcessorList(opts.BeforeLLM).
			WithLLM(llmProc).
			WithProcessor(dispatcher).
			WithProcessorList(beforeTTS).
			WithTTS(ttsProc)
		if opts.Filler != nil {
			builder = builder.WithFiller(opts.Filler)
		}
		for _, p := range opts.PostProcessors {
			if p != nil {
				builder = builder.WithSerializer(p)
			}
		}

		orch := builder.Build(cfg.Pipeline)
		orch.SetContext(ctx)
		orch.SetObserver(asyncObs)
		dispatcher.SetInput(orch.In())

		if sink != nil {
			orch.SetSink(sink)
		}

		go func() {
			<-ctx.Done()
			sttProc.CloseAll()
			ttsProc.CloseAll()
		}()

		return orch, nil
	})

	hooks := runner.Hooks{
		OnStart: func() {
			fields := []any{"message", "Ranya Engine Ready"}
			if rr, ok := opts.Transport.(transports.ReadyReporter); ok {
				for k, v := range rr.ReadyFields() {
					fields = append(fields, k, v)
				}
			}
			slog.Info("engine_ready", fields...)
		},
		OnStop: func() {
			if asyncObs != nil {
				asyncObs.Close()
			}
			if timelineObs != nil {
				_ = timelineObs.Close()
			}
			if costObs != nil {
				_ = costObs.Close()
			}
			slog.Info("shutdown", "goroutines", runtime.NumGoroutine(), "active_calls", registry.Count())
		},
	}

	drainer := pipeline.DrainerFunc(func() error {
		if opts.Transport != nil {
			_ = opts.Transport.Stop()
		}
		registry.SetDraining(true)
		registry.CloseAll()
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		_ = registry.WaitForEmpty(ctx, 200*time.Millisecond)
		return nil
	})

	lr := pipeline.NewDrainRunner(drainer, hooks, 30*time.Second)

	ctx, cancel := context.WithCancel(context.Background())

	return &Engine{
		cfg:       cfg,
		registry:  registry,
		transport: opts.Transport,
		providers: providers,
		runner:    lr,
		asyncObs:  asyncObs,
		ctx:       ctx,
		cancel:    cancel,
		tools:     opts.Tools,
		agents:    opts.Agents,
		router:    opts.Router,
	}
}

func configureRouter(opts EngineOptions) pipeline.FrameProcessor {
	rp := processors.NewRouterProcessor(opts.Router)
	rp.SetConfig(processors.RouterProcessorConfig{
		Mode:          opts.Config.Router.Mode,
		MaxTurns:      opts.Config.Router.MaxTurns,
		CodeSwitching: opts.Config.Languages.CodeSwitching,
	})
	if opts.LanguageDetector != nil {
		rp.SetLanguageDetector(opts.LanguageDetector, opts.LanguageMinConf)
	}
	if len(opts.LanguagePrompts) > 0 {
		rp.SetLanguagePrompts(opts.LanguagePrompts)
	}
	return rp
}

func defaultLanguage(cfg Config, override string) string {
	if strings.TrimSpace(override) != "" {
		return strings.TrimSpace(override)
	}
	if strings.TrimSpace(cfg.Languages.Default) != "" {
		return strings.TrimSpace(cfg.Languages.Default)
	}
	return "id"
}

func silenceRepromptFromConfig(cfg Config) *processors.SilenceRepromptConfig {
	sr := cfg.Turn.SilenceReprompt
	if sr.TimeoutMS == 0 && sr.MaxAttempts == 0 && sr.PromptText == "" && len(sr.PromptByLanguage) == 0 {
		return nil
	}
	timeout := time.Duration(sr.TimeoutMS) * time.Millisecond
	return &processors.SilenceRepromptConfig{
		Timeout:          timeout,
		MaxAttempts:      sr.MaxAttempts,
		PromptText:       sr.PromptText,
		PromptByLanguage: sr.PromptByLanguage,
	}
}

func isZeroToolOptions(opts ToolDispatcherOptions) bool {
	return opts.Concurrency == 0 &&
		opts.Timeout == 0 &&
		opts.Retries == 0 &&
		opts.RetryBackoff == 0 &&
		!opts.SerializeByStream
}

func toolOptionsFromConfig(cfg Config) ToolDispatcherOptions {
	return ToolDispatcherOptions{
		Concurrency:       cfg.Tools.Concurrency,
		Timeout:           time.Duration(cfg.Tools.TimeoutMS) * time.Millisecond,
		Retries:           cfg.Tools.Retries,
		RetryBackoff:      time.Duration(cfg.Tools.RetryBackoffMS) * time.Millisecond,
		SerializeByStream: cfg.Tools.SerializeByStream,
	}
}

func buildSTTLanguageFactories(cfg Config, providers *ProviderRegistry, traceID string) map[string]func(callSID, streamID string) stt.StreamingSTT {
	if providers == nil || len(cfg.Languages.Overrides) == 0 {
		return nil
	}
	out := map[string]func(callSID, streamID string) stt.StreamingSTT{}
	for lang, overrides := range cfg.Languages.Overrides {
		if overrides.STT == nil {
			continue
		}
		merged := mergeVendorConfig(cfg.Vendors.STT, overrides.STT)
		cfgLang := cfg
		cfgLang.Vendors.STT = merged
		factory, err := providers.BuildSTTFactory(merged.Provider, cfgLang, traceID)
		if err != nil {
			slog.Warn("stt_language_factory_failed", "lang", lang, "error", err)
			continue
		}
		out[lang] = factory
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func buildTTSLanguageFactories(cfg Config, providers *ProviderRegistry) map[string]func(callSID, streamID string) tts.StreamingTTS {
	if providers == nil || len(cfg.Languages.Overrides) == 0 {
		return nil
	}
	out := map[string]func(callSID, streamID string) tts.StreamingTTS{}
	for lang, overrides := range cfg.Languages.Overrides {
		if overrides.TTS == nil {
			continue
		}
		merged := mergeVendorConfig(cfg.Vendors.TTS, overrides.TTS)
		cfgLang := cfg
		cfgLang.Vendors.TTS = merged
		factory, err := providers.BuildTTSFactory(merged.Provider, cfgLang)
		if err != nil {
			slog.Warn("tts_language_factory_failed", "lang", lang, "error", err)
			continue
		}
		out[lang] = factory
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func copySettings(in map[string]any) map[string]any {
	if in == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func mergeVendorConfig(base VendorConfig, override *VendorConfig) VendorConfig {
	out := VendorConfig{
		Provider: base.Provider,
		Settings: copySettings(base.Settings),
	}
	if override == nil {
		return out
	}
	if strings.TrimSpace(override.Provider) != "" {
		out.Provider = override.Provider
	}
	if override.Settings != nil {
		for k, v := range override.Settings {
			out.Settings[k] = v
		}
	}
	return out
}

func (e *Engine) Start(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if e.transport != nil {
		if err := e.transport.Start(ctx); err != nil {
			return err
		}
		go e.routeTransport(ctx)
	}
	go func() {
		_ = e.runner.Run(ctx)
	}()
	return nil
}

func (e *Engine) Stop() error {
	if e.cancel != nil {
		e.cancel()
	}
	return e.runner.Stop()
}

func (e *Engine) routeTransport(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case f, ok := <-e.transport.Recv():
			if !ok {
				return
			}
			meta := f.Meta()
			callSID := meta[frames.MetaCallSID]
			streamID := meta[frames.MetaStreamID]
			traceID := meta[frames.MetaTraceID]
			if callSID == "" || streamID == "" {
				continue
			}
			if e.asyncObs != nil && f.Kind() == frames.KindAudio {
				af := f.(frames.AudioFrame)
				fields := map[string]any{
					"sample_rate": af.Rate(),
					"channels":    af.Channels(),
				}
				if e.cfg.Observability.RecordAudio {
					fields["payload_b64"] = base64.StdEncoding.EncodeToString(af.RawPayload())
				}
				tags := map[string]string{
					frames.MetaStreamID: streamID,
					frames.MetaTraceID:  traceID,
					frames.MetaCallSID:  callSID,
					"component":         "transport",
				}
				e.asyncObs.RecordEvent(metrics.MetricsEvent{
					Name:   "audio_in",
					Time:   time.Now(),
					Tags:   tags,
					Fields: fields,
				})
			}
			if f.Kind() == frames.KindSystem {
				sf := f.(frames.SystemFrame)
				if sf.Name() == "call_end" {
					e.registry.Remove(callSID)
					continue
				}
			}
			sess, _, err := e.registry.GetOrCreate(callSID, streamID, traceID)
			if err != nil {
				continue
			}
			nonBlockingSend(sess.Orch.In(), f)
		}
	}
}

func nonBlockingSend(ch chan frames.Frame, f frames.Frame) {
	select {
	case ch <- f:
	default:
	}
}

func SetDefaultLogger(level string) {
	lvl := slog.LevelInfo
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: lvl})))
}

func (e *Engine) ProviderRegistry() *ProviderRegistry {
	return e.providers
}

func (e *Engine) Transport() transports.Transport {
	return e.transport
}

func (e *Engine) Config() Config {
	return e.cfg
}

func (e *Engine) Registry() *pipeline.SessionRegistry {
	return e.registry
}

func (e *Engine) Context() context.Context {
	if e.ctx == nil {
		return context.Background()
	}
	return e.ctx
}

func (e *Engine) Health() error {
	if e.transport == nil {
		return fmt.Errorf("missing transport")
	}
	return nil
}
