package ranya

import (
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"

	"github.com/harunnryd/ranya/pkg/pipeline"
	"github.com/spf13/viper"
)

type Config struct {
	Pipeline      pipeline.Config       `mapstructure:"pipeline"`
	Engine        pipeline.EngineConfig `mapstructure:"engine"`
	Vendors       VendorsConfig         `mapstructure:"vendors"`
	Transports    TransportsConfig      `mapstructure:"transports"`
	STT           STTProcessingConfig   `mapstructure:"stt"`
	Turn          TurnConfig            `mapstructure:"turn"`
	Tools         ToolsConfig           `mapstructure:"tools"`
	Context       ContextConfig         `mapstructure:"context"`
	Summary       SummaryConfig         `mapstructure:"summary"`
	Recovery      RecoveryConfig        `mapstructure:"recovery"`
	Confirmation  ConfirmationConfig    `mapstructure:"confirmation"`
	Router        RouterConfig          `mapstructure:"router"`
	Environment   string                `mapstructure:"environment"`
	LogLevel      string                `mapstructure:"log_level"`
	LogFormat     string                `mapstructure:"log_format"`
	BasePrompt    string                `mapstructure:"base_prompt"`
	Languages     LanguageConfig        `mapstructure:"languages"`
	Observability ObservabilityConfig   `mapstructure:"observability"`
	Privacy       PrivacyConfig         `mapstructure:"privacy"`
	Agent         AgentProfileConfig    `mapstructure:"agent"`
	Vision        VisionConfig          `mapstructure:",squash"`
	Debug         DebugConfig           `mapstructure:",squash"`
}

type VendorConfig struct {
	Provider string         `mapstructure:"provider"`
	Settings map[string]any `mapstructure:"settings"`
}

type VendorsConfig struct {
	STT VendorConfig `mapstructure:"stt"`
	TTS VendorConfig `mapstructure:"tts"`
	LLM VendorConfig `mapstructure:"llm"`
}

type TransportsConfig struct {
	Provider string         `mapstructure:"provider"`
	Settings map[string]any `mapstructure:"settings"`
}

type VisionConfig struct {
	ImagePath string `mapstructure:"image_path"`
	ImageMime string `mapstructure:"image_mime"`
}

type LanguageOverrides struct {
	STT *VendorConfig `mapstructure:"stt"`
	TTS *VendorConfig `mapstructure:"tts"`
}

type LanguageConfig struct {
	Default       string                       `mapstructure:"default"`
	Prompts       map[string]string            `mapstructure:"prompts"`
	Overrides     map[string]LanguageOverrides `mapstructure:"overrides"`
	CodeSwitching bool                         `mapstructure:"code_switching"`
}

type STTProcessingConfig struct {
	ForwardInterim bool `mapstructure:"forward_interim"`
}

type TurnConfig struct {
	BargeInThresholdMS int                   `mapstructure:"barge_in_threshold_ms"`
	MinBargeInMS       int                   `mapstructure:"min_barge_in_ms"`
	EndOfTurnTimeoutMS int                   `mapstructure:"end_of_turn_timeout_ms"`
	SilenceReprompt    SilenceRepromptConfig `mapstructure:"silence_reprompt"`
}

type SilenceRepromptConfig struct {
	TimeoutMS        int               `mapstructure:"timeout_ms"`
	MaxAttempts      int               `mapstructure:"max_attempts"`
	PromptText       string            `mapstructure:"prompt_text"`
	PromptByLanguage map[string]string `mapstructure:"prompt_by_language"`
}

type ObservabilityConfig struct {
	ArtifactsDir  string `mapstructure:"artifacts_dir"`
	RecordAudio   bool   `mapstructure:"record_audio"`
	RetentionDays int    `mapstructure:"retention_days"`
}

type PrivacyConfig struct {
	RedactPII bool `mapstructure:"redact_pii"`
}

type AgentProfileConfig struct {
	Persona string `mapstructure:"persona"`
	Style   string `mapstructure:"style"`
}

type ToolsConfig struct {
	Concurrency       int  `mapstructure:"concurrency"`
	TimeoutMS         int  `mapstructure:"timeout_ms"`
	Retries           int  `mapstructure:"retries"`
	RetryBackoffMS    int  `mapstructure:"retry_backoff_ms"`
	SerializeByStream bool `mapstructure:"serialize_by_stream"`
}

type ContextConfig struct {
	MaxHistory int `mapstructure:"max_history"`
	MaxTokens  int `mapstructure:"max_tokens"`
}

type SummaryConfig struct {
	Enabled    bool `mapstructure:"enabled"`
	MaxEntries int  `mapstructure:"max_entries"`
	MaxChars   int  `mapstructure:"max_chars"`
}

type RecoveryConfig struct {
	MaxAttempts      int               `mapstructure:"max_attempts"`
	PromptText       string            `mapstructure:"prompt_text"`
	PromptByLanguage map[string]string `mapstructure:"prompt_by_language"`
	Phrases          []string          `mapstructure:"phrases"`
}

type ConfirmationConfig struct {
	Mode        string `mapstructure:"mode"`
	LLMFallback bool   `mapstructure:"llm_fallback"`
	TimeoutMS   int    `mapstructure:"timeout_ms"`
}

type RouterConfig struct {
	Mode     string `mapstructure:"mode"`
	MaxTurns int    `mapstructure:"max_turns"`
}

type DebugConfig struct {
	SimulateBadNet  bool `mapstructure:"simulate_bad_network"`
	SimulateHandoff bool `mapstructure:"simulate_handoff"`
}

func LoadConfig(path string) (Config, error) {
	v := viper.New()
	v.SetConfigFile(path)
	v.SetDefault("pipeline.async", true)
	v.SetDefault("pipeline.stagebuffer", 128)
	v.SetDefault("pipeline.highcapacity", 256)
	v.SetDefault("pipeline.lowcapacity", 512)
	v.SetDefault("pipeline.fairnessratio", 3)
	v.SetDefault("pipeline.backpressure", "drop")
	v.SetDefault("engine.samplerate", 8000)
	v.SetDefault("engine.stt_replay_chunks", 50)
	v.SetDefault("stt.forward_interim", false)
	v.SetDefault("turn.barge_in_threshold_ms", 500)
	v.SetDefault("turn.min_barge_in_ms", 300)
	v.SetDefault("turn.end_of_turn_timeout_ms", 0)
	v.SetDefault("turn.silence_reprompt.timeout_ms", 0)
	v.SetDefault("turn.silence_reprompt.max_attempts", 0)
	v.SetDefault("turn.silence_reprompt.prompt_text", "")
	v.SetDefault("tools.concurrency", 4)
	v.SetDefault("tools.timeout_ms", 6000)
	v.SetDefault("tools.retries", 1)
	v.SetDefault("tools.retry_backoff_ms", 200)
	v.SetDefault("tools.serialize_by_stream", true)
	v.SetDefault("context.max_history", 12)
	v.SetDefault("context.max_tokens", 0)
	v.SetDefault("summary.enabled", false)
	v.SetDefault("summary.max_entries", 8)
	v.SetDefault("summary.max_chars", 600)
	v.SetDefault("confirmation.mode", "hybrid")
	v.SetDefault("confirmation.llm_fallback", false)
	v.SetDefault("confirmation.timeout_ms", 600)
	v.SetDefault("router.mode", "full")
	v.SetDefault("router.max_turns", 2)
	v.SetDefault("environment", "development")
	v.SetDefault("log_level", "info")
	v.SetDefault("log_format", "text")
	v.SetDefault("languages.default", "id")
	v.SetDefault("languages.code_switching", false)
	v.SetDefault("observability.artifacts_dir", "")
	v.SetDefault("observability.record_audio", false)
	v.SetDefault("observability.retention_days", 0)
	v.SetDefault("privacy.redact_pii", true)

	if err := v.ReadInConfig(); err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}

	var raw struct {
		Pipeline struct {
			Async         bool   `mapstructure:"async"`
			StageBuffer   int    `mapstructure:"stagebuffer"`
			HighCapacity  int    `mapstructure:"highcapacity"`
			LowCapacity   int    `mapstructure:"lowcapacity"`
			FairnessRatio int    `mapstructure:"fairnessratio"`
			Backpressure  string `mapstructure:"backpressure"`
		} `mapstructure:"pipeline"`
		Engine          pipeline.EngineConfig `mapstructure:"engine"`
		Vendors         VendorsConfig         `mapstructure:"vendors"`
		STT             STTProcessingConfig   `mapstructure:"stt"`
		Turn            TurnConfig            `mapstructure:"turn"`
		Tools           ToolsConfig           `mapstructure:"tools"`
		Context         ContextConfig         `mapstructure:"context"`
		Summary         SummaryConfig         `mapstructure:"summary"`
		Recovery        RecoveryConfig        `mapstructure:"recovery"`
		Confirmation    ConfirmationConfig    `mapstructure:"confirmation"`
		Router          RouterConfig          `mapstructure:"router"`
		Environment     string                `mapstructure:"environment"`
		LogLevel        string                `mapstructure:"log_level"`
		LogFormat       string                `mapstructure:"log_format"`
		Transports      TransportsConfig      `mapstructure:"transports"`
		BasePrompt      string                `mapstructure:"base_prompt"`
		Languages       LanguageConfig        `mapstructure:"languages"`
		Observability   ObservabilityConfig   `mapstructure:"observability"`
		Privacy         PrivacyConfig         `mapstructure:"privacy"`
		Agent           AgentProfileConfig    `mapstructure:"agent"`
		ImagePath       string                `mapstructure:"image_path"`
		ImageMime       string                `mapstructure:"image_mime"`
		SimulateBadNet  bool                  `mapstructure:"simulate_bad_network"`
		SimulateHandoff bool                  `mapstructure:"simulate_handoff"`
	}
	if err := v.Unmarshal(&raw); err != nil {
		return Config{}, fmt.Errorf("unmarshal: %w", err)
	}

	cfg := Config{
		Pipeline: pipeline.Config{
			Async:         raw.Pipeline.Async,
			StageBuffer:   raw.Pipeline.StageBuffer,
			HighCapacity:  raw.Pipeline.HighCapacity,
			LowCapacity:   raw.Pipeline.LowCapacity,
			FairnessRatio: raw.Pipeline.FairnessRatio,
			Backpressure:  parseBackpressure(raw.Pipeline.Backpressure),
		},
		Engine:        raw.Engine,
		Vendors:       raw.Vendors,
		STT:           raw.STT,
		Turn:          raw.Turn,
		Tools:         raw.Tools,
		Context:       raw.Context,
		Summary:       raw.Summary,
		Recovery:      raw.Recovery,
		Confirmation:  raw.Confirmation,
		Router:        raw.Router,
		Environment:   raw.Environment,
		LogLevel:      raw.LogLevel,
		LogFormat:     raw.LogFormat,
		Transports:    raw.Transports,
		BasePrompt:    raw.BasePrompt,
		Languages:     raw.Languages,
		Observability: raw.Observability,
		Privacy:       raw.Privacy,
		Agent:         raw.Agent,
		Vision: VisionConfig{
			ImagePath: raw.ImagePath,
			ImageMime: raw.ImageMime,
		},
		Debug: DebugConfig{
			SimulateBadNet:  raw.SimulateBadNet,
			SimulateHandoff: raw.SimulateHandoff,
		},
	}

	if cfg.Debug.SimulateBadNet {
		cfg.Pipeline.StageBuffer = 16
		cfg.Pipeline.HighCapacity = 64
		cfg.Pipeline.LowCapacity = 128
	}

	expandEnvStrings(&cfg)

	if err := cfg.Validate(); err != nil {
		return Config{}, fmt.Errorf("validate config: %w", err)
	}

	return cfg, nil
}

func (c *Config) Validate() error {
	if strings.TrimSpace(c.Transports.Provider) == "" {
		return fmt.Errorf("transports.provider is required")
	}
	if strings.TrimSpace(c.Vendors.STT.Provider) == "" {
		return fmt.Errorf("vendors.stt.provider is required")
	}
	if strings.TrimSpace(c.Vendors.TTS.Provider) == "" {
		return fmt.Errorf("vendors.tts.provider is required")
	}
	if strings.TrimSpace(c.Vendors.LLM.Provider) == "" {
		return fmt.Errorf("vendors.llm.provider is required")
	}

	return nil
}

func expandEnvStrings(cfg *Config) {
	expandValue(reflect.ValueOf(cfg))
	cfg.Vendors.STT.Settings = expandSettings(cfg.Vendors.STT.Settings)
	cfg.Vendors.TTS.Settings = expandSettings(cfg.Vendors.TTS.Settings)
	cfg.Vendors.LLM.Settings = expandSettings(cfg.Vendors.LLM.Settings)
	cfg.Transports.Settings = expandSettings(cfg.Transports.Settings)
	expandLanguageOverrides(cfg)
}

func expandLanguageOverrides(cfg *Config) {
	if cfg == nil {
		return
	}
	for key, overrides := range cfg.Languages.Overrides {
		if overrides.STT != nil {
			overrides.STT.Settings = expandSettings(overrides.STT.Settings)
		}
		if overrides.TTS != nil {
			overrides.TTS.Settings = expandSettings(overrides.TTS.Settings)
		}
		cfg.Languages.Overrides[key] = overrides
	}
}

func expandSettings(settings map[string]any) map[string]any {
	if settings == nil {
		return nil
	}
	for k, v := range settings {
		settings[k] = expandAny(v)
	}
	return settings
}

func expandAny(v any) any {
	switch val := v.(type) {
	case string:
		return os.ExpandEnv(val)
	case []any:
		for i := range val {
			val[i] = expandAny(val[i])
		}
		return val
	case map[string]any:
		for k, v := range val {
			val[k] = expandAny(v)
		}
		return val
	case map[any]any:
		out := make(map[string]any, len(val))
		for k, v := range val {
			ks, ok := k.(string)
			if !ok {
				continue
			}
			out[ks] = expandAny(v)
		}
		return out
	default:
		return v
	}
}

func expandValue(v reflect.Value) {
	if !v.IsValid() {
		return
	}
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return
		}
		expandValue(v.Elem())
		return
	}
	switch v.Kind() {
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			expandValue(v.Field(i))
		}
	case reflect.String:
		if v.CanSet() {
			v.SetString(os.ExpandEnv(v.String()))
		}
	case reflect.Slice, reflect.Array:
		for i := 0; i < v.Len(); i++ {
			expandValue(v.Index(i))
		}
	case reflect.Map:
		if v.Type().Key().Kind() == reflect.String && v.Type().Elem().Kind() == reflect.String {
			for _, key := range v.MapKeys() {
				val := v.MapIndex(key)
				expanded := os.ExpandEnv(val.String())
				v.SetMapIndex(key, reflect.ValueOf(expanded))
			}
		}
	}
}

func parseBackpressure(v string) pipeline.BackpressureMode {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "wait":
		return pipeline.BackpressureWait
	case "drop", "":
		return pipeline.BackpressureDrop
	default:
		if n, err := strconv.Atoi(v); err == nil {
			return pipeline.BackpressureMode(n)
		}
	}
	return pipeline.BackpressureDrop
}
