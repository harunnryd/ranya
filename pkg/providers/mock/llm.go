package mock

import (
	"context"

	"github.com/harunnryd/ranya/pkg/llm"
)

type LLMAdapter struct {
	cfg LLMConfig
}

type LLMConfig struct {
	ResponseText string
	ToolCalls    []llm.ToolCall
	HandoffAgent string
	StreamChunks []string
}

func NewLLMAdapter(cfg LLMConfig) *LLMAdapter {
	if cfg.ResponseText == "" {
		cfg.ResponseText = "mock response"
	}
	return &LLMAdapter{cfg: cfg}
}

func (a *LLMAdapter) Name() string { return "mock_llm" }

func (a *LLMAdapter) Generate(ctx context.Context, input llm.Context) (llm.Response, error) {
	resp := llm.Response{
		Text:         a.cfg.ResponseText,
		ToolCalls:    a.cfg.ToolCalls,
		HandoffAgent: a.cfg.HandoffAgent,
	}
	return resp, nil
}

func (a *LLMAdapter) Stream(ctx context.Context, input llm.Context) (<-chan string, error) {
	out := make(chan string, 4)
	if len(a.cfg.StreamChunks) > 0 {
		for _, chunk := range a.cfg.StreamChunks {
			out <- chunk
		}
	} else {
		out <- a.cfg.ResponseText
	}
	close(out)
	return out, nil
}

func (a *LLMAdapter) MapTools(tools []llm.Tool) (any, error) {
	return nil, nil
}

func (a *LLMAdapter) ToProviderFormat(ctx llm.Context) (any, error) {
	return nil, nil
}

func (a *LLMAdapter) FromProviderFormat(raw any) (llm.Response, error) {
	return llm.Response{Text: a.cfg.ResponseText}, nil
}
