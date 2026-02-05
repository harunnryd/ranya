package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/harunnryd/ranya/pkg/adapters/stt"
	"github.com/harunnryd/ranya/pkg/adapters/tts"
	"github.com/harunnryd/ranya/pkg/configutil"
	"github.com/harunnryd/ranya/pkg/llm"
	"github.com/harunnryd/ranya/pkg/pipeline"
	"github.com/harunnryd/ranya/pkg/processors"
	"github.com/harunnryd/ranya/pkg/providers/deepgram"
	"github.com/harunnryd/ranya/pkg/providers/elevenlabs"
	"github.com/harunnryd/ranya/pkg/providers/mock"
	"github.com/harunnryd/ranya/pkg/providers/openai"
	"github.com/harunnryd/ranya/pkg/ranya"
	"github.com/harunnryd/ranya/pkg/resilience"
	"github.com/harunnryd/ranya/pkg/transports"
	mocktransport "github.com/harunnryd/ranya/pkg/transports/mock"
	twiliotransport "github.com/harunnryd/ranya/pkg/transports/twilio"
)

// LogConfig defines the configuration for structured logging
type LogConfig struct {
	Level  string // "DEBUG" or "INFO"
	Format string // "json" or "text"
}

type deepgramSettings struct {
	APIKey           string `mapstructure:"api_key"`
	Model            string `mapstructure:"model"`
	Language         string `mapstructure:"language"`
	SampleRate       int    `mapstructure:"sample_rate"`
	Encoding         string `mapstructure:"encoding"`
	Interim          *bool  `mapstructure:"interim"`
	VADEvents        *bool  `mapstructure:"vad_events"`
	EchoCancellation *bool  `mapstructure:"echo_cancellation"`
	UtteranceEndMS   *int   `mapstructure:"utterance_end_ms"`
}

type elevenlabsSettings struct {
	APIKey       string `mapstructure:"api_key"`
	VoiceID      string `mapstructure:"voice_id"`
	ModelID      string `mapstructure:"model_id"`
	OutputFormat string `mapstructure:"output_format"`
	SampleRate   int    `mapstructure:"sample_rate"`
}

type openAISettings struct {
	APIKey            string `mapstructure:"api_key"`
	Model             string `mapstructure:"model"`
	BaseURL           string `mapstructure:"base_url"`
	UseCircuitBreaker *bool  `mapstructure:"use_circuit_breaker"`
	CircuitThreshold  int    `mapstructure:"circuit_threshold"`
	CircuitCooldownMs int    `mapstructure:"circuit_cooldown_ms"`
}

type mockSTTSettings struct {
	Transcript        string `mapstructure:"transcript"`
	InterimTranscript string `mapstructure:"interim_transcript"`
	EmitInterim       *bool  `mapstructure:"emit_interim"`
	EmitVAD           *bool  `mapstructure:"emit_vad"`
	EmitUtteranceEnd  *bool  `mapstructure:"emit_utterance_end"`
}

type mockTTSSettings struct {
	EmitAudioReady *bool `mapstructure:"emit_audio_ready"`
	SampleRate     int   `mapstructure:"sample_rate"`
	Channels       int   `mapstructure:"channels"`
}

type mockLLMSettings struct {
	ResponseText string         `mapstructure:"response_text"`
	ToolCalls    []mockToolCall `mapstructure:"tool_calls"`
	HandoffAgent string         `mapstructure:"handoff_agent"`
	StreamChunks []string       `mapstructure:"stream_chunks"`
}

type mockToolCall struct {
	ID        string         `mapstructure:"id"`
	Name      string         `mapstructure:"name"`
	Arguments map[string]any `mapstructure:"arguments"`
}

type twilioSettings struct {
	AccountSID         string   `mapstructure:"account_sid"`
	AuthToken          string   `mapstructure:"auth_token"`
	PublicURL          string   `mapstructure:"public_url"`
	ServerAddr         string   `mapstructure:"server_addr"`
	VoicePath          string   `mapstructure:"voice_path"`
	WebsocketPath      string   `mapstructure:"ws_path"`
	TTSWebhookPath     string   `mapstructure:"tts_webhook_path"`
	StatusCallbackPath string   `mapstructure:"status_callback_path"`
	VoiceGreeting      string   `mapstructure:"voice_greeting"`
	AllowAnyOrigin     bool     `mapstructure:"allow_any_origin"`
	AllowedOrigins     []string `mapstructure:"allowed_origins"`
}

// InitLogger initializes the global slog logger with the specified configuration
func InitLogger(config LogConfig) *slog.Logger {
	// Parse log level from configuration
	var level slog.Level
	switch strings.ToUpper(config.Level) {
	case "DEBUG":
		level = slog.LevelDebug
	case "INFO":
		level = slog.LevelInfo
	case "WARN", "WARNING":
		level = slog.LevelWarn
	case "ERROR":
		level = slog.LevelError
	default:
		// Default to INFO level if invalid or empty
		level = slog.LevelInfo
		slog.Warn("invalid log level specified, defaulting to INFO", "specified_level", config.Level)
	}

	// Create handler options with level filtering
	opts := &slog.HandlerOptions{
		Level: level,
	}

	// Create handler based on format
	var handler slog.Handler
	switch strings.ToLower(config.Format) {
	case "json":
		handler = slog.NewJSONHandler(os.Stdout, opts)
	case "text", "":
		// Default to text format
		handler = slog.NewTextHandler(os.Stdout, opts)
	default:
		// Default to text format if invalid
		handler = slog.NewTextHandler(os.Stdout, opts)
		slog.Warn("invalid log format specified, defaulting to text", "specified_format", config.Format)
	}

	// Create logger with the configured handler
	logger := slog.New(handler)

	// Set as default logger
	slog.SetDefault(logger)

	// Log initialization
	logger.Info("logger initialized", "level", level.String(), "format", config.Format)

	return logger
}

func validDeepgramEncoding(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "linear16", "mulaw":
		return true
	default:
		return false
	}
}

func applyAgentProfile(base string, profile ranya.AgentProfileConfig) string {
	base = strings.TrimSpace(base)
	var parts []string
	if base != "" {
		parts = append(parts, base)
	}
	if p := strings.TrimSpace(profile.Persona); p != "" {
		parts = append(parts, "Persona: "+p)
	}
	if s := strings.TrimSpace(profile.Style); s != "" {
		parts = append(parts, "Style: "+s)
	}
	return strings.Join(parts, "\n")
}

func recoveryConfigFromConfig(cfg ranya.RecoveryConfig, fallback map[string]string) processors.RecoveryConfig {
	prompts := cfg.PromptByLanguage
	if len(prompts) == 0 {
		prompts = fallback
	}
	out := processors.RecoveryConfig{
		PromptByLanguage: prompts,
		PromptText:       cfg.PromptText,
		MaxAttempts:      cfg.MaxAttempts,
		Phrases:          cfg.Phrases,
	}
	return out
}

func defaultRecoveryPrompts(overrides map[string]string) map[string]string {
	if len(overrides) > 0 {
		return overrides
	}
	return map[string]string{
		"id": "Maaf, saya belum menangkapnya. Bisa jelaskan ulang secara singkat?",
		"en": "Sorry, I didn't catch that. Could you repeat it briefly?",
	}
}

func main() {
	configPath := flag.String("config", "examples/hvac/config.local.yaml", "")
	dialTo := flag.String("dial_to", "", "destination number for outbound call")
	dialFrom := flag.String("dial_from", "", "caller ID for outbound call")
	dialURL := flag.String("dial_url", "", "override voice URL for outbound call")
	flag.Parse()
	cfg, err := ranya.LoadConfig(*configPath)
	if err != nil {
		panic(err)
	}
	cfg.BasePrompt = applyAgentProfile(cfg.BasePrompt, cfg.Agent)

	// Initialize structured logger
	logConfig := LogConfig{
		Level:  cfg.LogLevel,
		Format: cfg.LogFormat,
	}
	InitLogger(logConfig)

	// 1. Define Agents (HVAC Specific)
	agents := map[string]processors.AgentConfig{
		"triage": {
			Name: "triage",
			System: "Kamu Agent Triage HVAC. Tugas: kumpulkan lokasi, jenis AC, issue utama, urgensi, dan waktu yang diinginkan. " +
				"Gunakan tool schedule_visit, create_ticket, dan get_technician_eta jika butuh. " +
				"Jika butuh diagnosa teknis mendalam, akhiri jawaban dengan #handoff=technical. " +
				"Jika pertanyaan biaya/pembayaran dominan, akhiri jawaban dengan #handoff=billing.",
		},
		"technical": {
			Name: "technical",
			System: "Kamu Agent Technical HVAC. Fokus diagnosa (kompresor, freon, suara, tekanan, kode error). " +
				"Jawaban maksimal 4 kalimat lalu tawarkan kunjungan. " +
				"Jika perlu penjadwalan, akhiri jawaban dengan #handoff=triage. " +
				"Jika masuk ke biaya/promo, akhiri jawaban dengan #handoff=billing.",
		},
		"billing": {
			Name: "billing",
			System: "Kamu Agent Billing HVAC. Fokus biaya, estimasi, promo, invoice, dan pembayaran. " +
				"Gunakan tool estimate_service_cost dan send_payment_link jika perlu. " +
				"Jika user membutuhkan diagnosa teknis, akhiri jawaban dengan #handoff=technical. " +
				"Jika perlu penjadwalan, akhiri jawaban dengan #handoff=triage.",
		},
	}

	// 2. Define Tools (HVAC Specific)
	tools := NewHVACToolRegistry()

	// 3. Initialize providers
	providers := ranya.NewProviderRegistry()
	registerProviders(providers)

	// 4. LLM router + language detection
	llmAdapter, err := providers.BuildLLM(cfg.Vendors.LLM.Provider, cfg)
	if err != nil {
		slog.Error("router_llm_unavailable", "error", err)
		panic(err)
	}
	var router processors.RouterStrategy = NewLLMRouterStrategy(llmAdapter, nil, LLMRouterConfig{})
	slog.Info("router_mode", "mode", "llm_only")

	langDetector := NewLLMLanguageDetector(llmAdapter)
	langPrompts := map[string]string{}
	for lang, prompt := range cfg.Languages.Prompts {
		if strings.TrimSpace(prompt) != "" {
			langPrompts[lang] = prompt
		}
	}

	filler := processors.NewFillerProcessor("examples/hvac/assets/filler.ulaw")
	bootstrap := NewHVACBootstrap(cfg)
	var preProcessors []pipeline.FrameProcessor
	if cfg.Debug.SimulateBadNet {
		preProcessors = append(preProcessors, NewBadNetworkSimulator(true))
	}
	preProcessors = append(preProcessors, bootstrap)

	beforeContext := []pipeline.FrameProcessor{
		processors.NewTextNormalizer(processors.TextNormalizerConfig{
			Replacements: map[string]string{
				"compressor":      "kompresor",
				"freon leak":      "bocor freon",
				"aircon":          "ac",
				"air conditioner": "ac",
				"overheating":     "panas berlebih",
			},
		}),
		processors.NewDTMFDisambiguator(processors.DTMFDisambiguatorConfig{
			PreferDTMF: true,
		}),
	}
	recoveryPrompts := defaultRecoveryPrompts(cfg.Recovery.PromptByLanguage)
	beforeTTS := []pipeline.FrameProcessor{
		processors.NewRecoveryProcessor(recoveryConfigFromConfig(cfg.Recovery, recoveryPrompts)),
		processors.NewResponseLimiter(processors.ResponseLimiterConfig{
			MaxChars:     420,
			MaxSentences: 3,
		}),
	}

	// 5. Initialize Ranya Engine
	transport, err := buildTransport(cfg)
	if err != nil {
		panic(err)
	}
	opts := ranya.EngineOptions{
		Config:           cfg,
		Providers:        providers,
		Transport:        transport,
		Tools:            tools,
		Agents:           agents,
		Router:           router,
		Filler:           filler,
		PreProcessors:    preProcessors,
		BeforeContext:    beforeContext,
		BeforeTTS:        beforeTTS,
		ContextCaption:   "Label AC",
		LanguageDetector: langDetector,
		LanguagePrompts:  langPrompts,
		LanguageMinConf:  0.55,
		DefaultLanguage:  cfg.Languages.Default,
	}
	app := ranya.NewEngine(opts)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_ = app.Start(ctx)
	if *dialTo != "" && *dialFrom != "" {
		if dialer, ok := transport.(transports.OutboundDialer); ok {
			callSID, err := dialer.Dial(ctx, *dialTo, *dialFrom, *dialURL)
			if err != nil {
				slog.Error("outbound_dial_failed", "error", err)
			} else {
				slog.Info("outbound_dial_started", "call_sid", callSID)
			}
		} else {
			slog.Warn("transport_no_outbound_dialer")
		}
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	_ = app.Stop()
}

func registerProviders(reg *ranya.ProviderRegistry) {
	reg.RegisterSTT("deepgram", func(cfg ranya.Config, traceID string) (func(callSID, streamID string) stt.StreamingSTT, error) {
		if err := validateSettings("vendors.stt.settings", cfg.Vendors.STT.Settings, configutil.Schema{
			Required: []string{"api_key", "model"},
			Optional: []string{"language", "sample_rate", "encoding", "interim", "vad_events", "echo_cancellation", "utterance_end_ms"},
		}); err != nil {
			return nil, err
		}
		var settings deepgramSettings
		if err := configutil.DecodeSettings(cfg.Vendors.STT.Settings, &settings); err != nil {
			return nil, err
		}
		if err := configutil.RequireString(settings.APIKey, "vendors.stt.settings.api_key"); err != nil {
			return nil, err
		}
		if err := configutil.RequireString(settings.Model, "vendors.stt.settings.model"); err != nil {
			return nil, err
		}
		if settings.SampleRate == 0 {
			if cfg.Engine.SampleRate > 0 {
				settings.SampleRate = cfg.Engine.SampleRate
			} else {
				settings.SampleRate = 8000
			}
		}
		if settings.Language == "" {
			settings.Language = "id"
		}
		if settings.Encoding == "" {
			settings.Encoding = "mulaw"
		}
		if !validDeepgramEncoding(settings.Encoding) {
			return nil, fmt.Errorf("vendors.stt.settings.encoding must be one of [linear16, mulaw], got %s", settings.Encoding)
		}
		utteranceEnd := configutil.IntValue(settings.UtteranceEndMS, 1000)
		if utteranceEnd < 0 || utteranceEnd > 5000 {
			return nil, fmt.Errorf("vendors.stt.settings.utterance_end_ms must be between 0 and 5000, got %d", utteranceEnd)
		}
		interim := configutil.BoolValue(settings.Interim, true)
		vadEvents := configutil.BoolValue(settings.VADEvents, true)
		echoCancellation := configutil.BoolValue(settings.EchoCancellation, true)

		return func(callSID, streamID string) stt.StreamingSTT {
			return deepgram.New(deepgram.Config{
				APIKey:     settings.APIKey,
				Model:      settings.Model,
				Language:   settings.Language,
				SampleRate: settings.SampleRate,
				Encoding:   settings.Encoding,
				Interim:    interim,
				VADEvents:  vadEvents,
				StreamID:   streamID,
				CallSID:    callSID,
				TraceID:    traceID,
				Params: deepgram.DeepgramParams{
					EchoCancellation: echoCancellation,
					UtteranceEndMS:   utteranceEnd,
				},
			})
		}, nil
	})

	reg.RegisterSTT("mock", func(cfg ranya.Config, traceID string) (func(callSID, streamID string) stt.StreamingSTT, error) {
		if err := validateSettings("vendors.stt.settings", cfg.Vendors.STT.Settings, configutil.Schema{
			Optional: []string{"transcript", "interim_transcript", "emit_interim", "emit_vad", "emit_utterance_end"},
		}); err != nil {
			return nil, err
		}
		var settings mockSTTSettings
		if err := configutil.DecodeSettings(cfg.Vendors.STT.Settings, &settings); err != nil {
			return nil, err
		}
		emitInterim := configutil.BoolValue(settings.EmitInterim, false)
		emitVAD := configutil.BoolValue(settings.EmitVAD, false)
		emitUtteranceEnd := configutil.BoolValue(settings.EmitUtteranceEnd, false)
		return func(callSID, streamID string) stt.StreamingSTT {
			return mock.NewSTT(mock.STTConfig{
				StreamID:          streamID,
				CallSID:           callSID,
				TraceID:           traceID,
				Transcript:        settings.Transcript,
				InterimTranscript: settings.InterimTranscript,
				EmitInterim:       emitInterim,
				EmitVAD:           emitVAD,
				EmitUtteranceEnd:  emitUtteranceEnd,
			})
		}, nil
	})

	reg.RegisterTTS("elevenlabs", func(cfg ranya.Config) (func(callSID, streamID string) tts.StreamingTTS, error) {
		if err := validateSettings("vendors.tts.settings", cfg.Vendors.TTS.Settings, configutil.Schema{
			Required: []string{"api_key", "voice_id"},
			Optional: []string{"model_id", "output_format", "sample_rate"},
		}); err != nil {
			return nil, err
		}
		var settings elevenlabsSettings
		if err := configutil.DecodeSettings(cfg.Vendors.TTS.Settings, &settings); err != nil {
			return nil, err
		}
		if err := configutil.RequireString(settings.APIKey, "vendors.tts.settings.api_key"); err != nil {
			return nil, err
		}
		if err := configutil.RequireString(settings.VoiceID, "vendors.tts.settings.voice_id"); err != nil {
			return nil, err
		}
		if settings.OutputFormat == "" {
			settings.OutputFormat = "ulaw_8000"
		}
		if settings.SampleRate == 0 {
			if cfg.Engine.SampleRate > 0 {
				settings.SampleRate = cfg.Engine.SampleRate
			} else {
				settings.SampleRate = 8000
			}
		}
		return func(callSID, streamID string) tts.StreamingTTS {
			return elevenlabs.New(elevenlabs.Config{
				APIKey:       settings.APIKey,
				VoiceID:      settings.VoiceID,
				ModelID:      settings.ModelID,
				OutputFormat: settings.OutputFormat,
				SampleRate:   settings.SampleRate,
				StreamID:     streamID,
				CallSID:      callSID,
			})
		}, nil
	})

	reg.RegisterTTS("mock", func(cfg ranya.Config) (func(callSID, streamID string) tts.StreamingTTS, error) {
		if err := validateSettings("vendors.tts.settings", cfg.Vendors.TTS.Settings, configutil.Schema{
			Optional: []string{"emit_audio_ready", "sample_rate", "channels"},
		}); err != nil {
			return nil, err
		}
		var settings mockTTSSettings
		if err := configutil.DecodeSettings(cfg.Vendors.TTS.Settings, &settings); err != nil {
			return nil, err
		}
		sampleRate := settings.SampleRate
		if sampleRate == 0 {
			if cfg.Engine.SampleRate > 0 {
				sampleRate = cfg.Engine.SampleRate
			} else {
				sampleRate = 8000
			}
		}
		channels := settings.Channels
		if channels == 0 {
			channels = 1
		}
		emitAudioReady := configutil.BoolValue(settings.EmitAudioReady, false)
		return func(callSID, streamID string) tts.StreamingTTS {
			return mock.NewTTS(mock.TTSConfig{
				StreamID:       streamID,
				CallSID:        callSID,
				SampleRate:     sampleRate,
				Channels:       channels,
				EmitAudioReady: emitAudioReady,
			})
		}, nil
	})

	reg.RegisterLLM("openai", func(cfg ranya.Config) (llm.LLMAdapter, error) {
		if err := validateSettings("vendors.llm.settings", cfg.Vendors.LLM.Settings, configutil.Schema{
			Required: []string{"api_key", "model"},
			Optional: []string{"base_url", "use_circuit_breaker", "circuit_threshold", "circuit_cooldown_ms"},
		}); err != nil {
			return nil, err
		}
		var settings openAISettings
		if err := configutil.DecodeSettings(cfg.Vendors.LLM.Settings, &settings); err != nil {
			return nil, err
		}
		if err := configutil.RequireString(settings.APIKey, "vendors.llm.settings.api_key"); err != nil {
			return nil, err
		}
		if err := configutil.RequireString(settings.Model, "vendors.llm.settings.model"); err != nil {
			return nil, err
		}
		adapter := openai.NewAdapter(settings.APIKey, settings.Model)
		if settings.BaseURL != "" {
			adapter.BaseURL = settings.BaseURL
		}
		useBreaker := configutil.BoolValue(settings.UseCircuitBreaker, true)
		threshold := settings.CircuitThreshold
		if threshold == 0 {
			threshold = 3
		}
		cooldown := settings.CircuitCooldownMs
		if cooldown == 0 {
			cooldown = 30000
		}
		if useBreaker {
			breaker := resilience.NewCircuitBreaker(threshold, time.Duration(cooldown)*time.Millisecond)
			return llm.NewCircuitBreakerAdapter(adapter, breaker), nil
		}
		return adapter, nil
	})

	reg.RegisterLLM("mock", func(cfg ranya.Config) (llm.LLMAdapter, error) {
		if err := validateSettings("vendors.llm.settings", cfg.Vendors.LLM.Settings, configutil.Schema{
			Optional: []string{"response_text", "tool_calls", "handoff_agent", "stream_chunks"},
		}); err != nil {
			return nil, err
		}
		var settings mockLLMSettings
		if err := configutil.DecodeSettings(cfg.Vendors.LLM.Settings, &settings); err != nil {
			return nil, err
		}
		toolCalls := make([]llm.ToolCall, 0, len(settings.ToolCalls))
		for i, tc := range settings.ToolCalls {
			id := strings.TrimSpace(tc.ID)
			if id == "" {
				id = fmt.Sprintf("mock-tool-%d", i+1)
			}
			toolCalls = append(toolCalls, llm.ToolCall{
				ID:        id,
				Name:      tc.Name,
				Arguments: tc.Arguments,
			})
		}
		return mock.NewLLMAdapter(mock.LLMConfig{
			ResponseText: settings.ResponseText,
			ToolCalls:    toolCalls,
			HandoffAgent: settings.HandoffAgent,
			StreamChunks: settings.StreamChunks,
		}), nil
	})
}

func validateSettings(path string, input map[string]any, schema configutil.Schema) error {
	if err := configutil.ValidateSettings(input, schema); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}

func buildTransport(cfg ranya.Config) (transports.Transport, error) {
	switch strings.ToLower(strings.TrimSpace(cfg.Transports.Provider)) {
	case "twilio":
		if err := validateSettings("transports.settings", cfg.Transports.Settings, configutil.Schema{
			Required: []string{"account_sid", "auth_token"},
			Optional: []string{"public_url", "server_addr", "voice_path", "ws_path", "tts_webhook_path", "status_callback_path", "voice_greeting", "allow_any_origin", "allowed_origins"},
		}); err != nil {
			return nil, err
		}
		var settings twilioSettings
		if err := configutil.DecodeSettings(cfg.Transports.Settings, &settings); err != nil {
			return nil, err
		}
		if err := configutil.RequireString(settings.AccountSID, "transports.settings.account_sid"); err != nil {
			return nil, err
		}
		if err := configutil.RequireString(settings.AuthToken, "transports.settings.auth_token"); err != nil {
			return nil, err
		}
		return twiliotransport.New(twiliotransport.Config{
			AccountSID:         settings.AccountSID,
			AuthToken:          settings.AuthToken,
			PublicURL:          settings.PublicURL,
			ServerAddr:         settings.ServerAddr,
			VoicePath:          settings.VoicePath,
			WebsocketPath:      settings.WebsocketPath,
			TTSWebhookPath:     settings.TTSWebhookPath,
			StatusCallbackPath: settings.StatusCallbackPath,
			VoiceGreeting:      settings.VoiceGreeting,
			AllowAnyOrigin:     settings.AllowAnyOrigin,
			AllowedOrigins:     settings.AllowedOrigins,
		}), nil
	case "mock":
		return mocktransport.New(), nil
	default:
		return nil, fmt.Errorf("unsupported transport provider: %s", cfg.Transports.Provider)
	}
}
