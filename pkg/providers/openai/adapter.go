package openai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/harunnryd/ranya/pkg/llm"
	"github.com/harunnryd/ranya/pkg/resilience"
)

type Adapter struct {
	APIKey  string
	Model   string
	BaseURL string
	Client  *http.Client
}

func NewAdapter(apiKey, model string) *Adapter {
	return &Adapter{
		APIKey:  apiKey,
		Model:   model,
		BaseURL: "https://api.openai.com/v1",
		Client:  &http.Client{Timeout: 60 * time.Second},
	}
}

func (a *Adapter) Name() string { return "openai" }

func (a *Adapter) MapTools(tools []llm.Tool) (any, error) {
	var out []map[string]any
	for _, t := range tools {
		out = append(out, map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        t.Name,
				"description": t.Description,
				"parameters":  t.Schema,
			},
		})
	}
	return out, nil
}

func (a *Adapter) ToProviderFormat(ctx llm.Context) (any, error) {
	return map[string]any{"messages": ctx.Messages}, nil
}

func (a *Adapter) FromProviderFormat(raw any) (llm.Response, error) {
	m, ok := raw.(map[string]any)
	if !ok {
		return llm.Response{}, errors.New("invalid response")
	}
	choices, _ := m["choices"].([]any)
	if len(choices) == 0 {
		return llm.Response{}, errors.New("no choices")
	}
	first, _ := choices[0].(map[string]any)
	msg, _ := first["message"].(map[string]any)
	content, _ := msg["content"].(string)
	handoff, cleaned := detectHandoff(content)
	resp := llm.Response{Text: cleaned, HandoffAgent: handoff}
	if reason, _ := first["finish_reason"].(string); reason != "" {
		resp.FinishReason = reason
	}
	if tc, ok := msg["tool_calls"].([]any); ok {
		for _, item := range tc {
			call, _ := item.(map[string]any)
			fn, _ := call["function"].(map[string]any)
			argsRaw, _ := fn["arguments"].(string)
			args := map[string]any{}
			_ = json.Unmarshal([]byte(argsRaw), &args)
			resp.ToolCalls = append(resp.ToolCalls, llm.ToolCall{
				ID:        stringValue(call["id"]),
				Name:      stringValue(fn["name"]),
				Arguments: args,
			})
		}
	}
	return resp, nil
}

func (a *Adapter) Generate(ctx context.Context, input llm.Context) (llm.Response, error) {
	body, err := a.buildRequest(input, false)
	if err != nil {
		return llm.Response{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.BaseURL+"/chat/completions", body)
	if err != nil {
		return llm.Response{}, err
	}
	a.applyHeaders(req)
	resp, err := a.client().Do(req)
	if err != nil {
		return llm.Response{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusTooManyRequests {
		body, _ := io.ReadAll(resp.Body)
		return llm.Response{}, resilience.RateLimitError{Provider: "openai", Message: string(body)}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return llm.Response{}, errors.New(string(body))
	}
	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return llm.Response{}, err
	}
	return a.FromProviderFormat(payload)
}

func (a *Adapter) Stream(ctx context.Context, input llm.Context) (<-chan string, error) {
	body, err := a.buildRequest(input, true)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.BaseURL+"/chat/completions", body)
	if err != nil {
		return nil, err
	}
	a.applyHeaders(req)
	resp, err := a.client().Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, resilience.RateLimitError{Provider: "openai", Message: string(body)}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, errors.New(string(body))
	}
	out := make(chan string, 128)
	go func() {
		defer resp.Body.Close()
		defer close(out)
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if !strings.HasPrefix(line, "data:") {
				continue
			}
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if data == "[DONE]" {
				return
			}
			var chunk map[string]any
			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				continue
			}
			choices, _ := chunk["choices"].([]any)
			if len(choices) == 0 {
				continue
			}
			first, _ := choices[0].(map[string]any)
			delta, _ := first["delta"].(map[string]any)
			if text, _ := delta["content"].(string); text != "" {
				select {
				case <-ctx.Done():
					return
				case out <- text:
				}
			}
		}
	}()
	return out, nil
}

func (a *Adapter) buildRequest(input llm.Context, stream bool) (*bytes.Buffer, error) {
	req := map[string]any{
		"model":    a.Model,
		"stream":   stream,
		"messages": normalizeMessages(input.Messages),
	}
	if len(input.Tools) > 0 {
		tools, err := a.MapTools(input.Tools)
		if err != nil {
			return nil, err
		}
		req["tools"] = tools
		req["tool_choice"] = "auto"
	}
	if stream {
		req["stream_options"] = map[string]any{"include_usage": true}
	}
	b, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	return bytes.NewBuffer(b), nil
}

func (a *Adapter) applyHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.APIKey)
}

func (a *Adapter) client() *http.Client {
	if a.Client != nil {
		return a.Client
	}
	return http.DefaultClient
}

func normalizeMessages(messages []map[string]any) []map[string]any {
	out := make([]map[string]any, 0, len(messages))
	out = append(out, messages...)
	return out
}

func detectHandoff(text string) (string, string) {
	parts := strings.Split(text, "#handoff=")
	if len(parts) < 2 {
		return "", text
	}
	rest := parts[1]
	fields := strings.Fields(rest)
	if len(fields) == 0 {
		return "", text
	}
	agent := strings.TrimSpace(fields[0])
	cleaned := strings.TrimSpace(parts[0])
	return agent, cleaned
}

func stringValue(v any) string {
	s, _ := v.(string)
	return s
}
