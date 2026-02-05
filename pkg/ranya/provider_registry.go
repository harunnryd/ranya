package ranya

import (
	"fmt"
	"strings"

	"github.com/harunnryd/ranya/pkg/adapters/stt"
	"github.com/harunnryd/ranya/pkg/adapters/tts"
	"github.com/harunnryd/ranya/pkg/llm"
)

type STTFactoryBuilder func(cfg Config, traceID string) (func(callSID, streamID string) stt.StreamingSTT, error)
type TTSFactoryBuilder func(cfg Config) (func(callSID, streamID string) tts.StreamingTTS, error)
type LLMFactory func(cfg Config) (llm.LLMAdapter, error)

type ProviderRegistry struct {
	stt map[string]STTFactoryBuilder
	tts map[string]TTSFactoryBuilder
	llm map[string]LLMFactory
}

func NewProviderRegistry() *ProviderRegistry {
	return &ProviderRegistry{
		stt: make(map[string]STTFactoryBuilder),
		tts: make(map[string]TTSFactoryBuilder),
		llm: make(map[string]LLMFactory),
	}
}

func (r *ProviderRegistry) RegisterSTT(name string, factory STTFactoryBuilder) {
	r.stt[strings.ToLower(strings.TrimSpace(name))] = factory
}

func (r *ProviderRegistry) RegisterTTS(name string, factory TTSFactoryBuilder) {
	r.tts[strings.ToLower(strings.TrimSpace(name))] = factory
}

func (r *ProviderRegistry) RegisterLLM(name string, factory LLMFactory) {
	r.llm[strings.ToLower(strings.TrimSpace(name))] = factory
}

func (r *ProviderRegistry) BuildSTTFactory(provider string, cfg Config, traceID string) (func(callSID, streamID string) stt.StreamingSTT, error) {
	fn := r.stt[strings.ToLower(strings.TrimSpace(provider))]
	if fn == nil {
		return nil, fmt.Errorf("stt provider not registered: %s", provider)
	}
	return fn(cfg, traceID)
}

func (r *ProviderRegistry) BuildTTSFactory(provider string, cfg Config) (func(callSID, streamID string) tts.StreamingTTS, error) {
	fn := r.tts[strings.ToLower(strings.TrimSpace(provider))]
	if fn == nil {
		return nil, fmt.Errorf("tts provider not registered: %s", provider)
	}
	return fn(cfg)
}

func (r *ProviderRegistry) BuildLLM(provider string, cfg Config) (llm.LLMAdapter, error) {
	fn := r.llm[strings.ToLower(strings.TrimSpace(provider))]
	if fn == nil {
		return nil, fmt.Errorf("llm provider not registered: %s", provider)
	}
	return fn(cfg)
}
