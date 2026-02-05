package main

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/harunnryd/ranya/pkg/llm"
	"github.com/harunnryd/ranya/pkg/processors"
)

type LLMLanguageDetector struct {
	adapter llm.LLMAdapter
	timeout time.Duration
}

type languageDetectOutput struct {
	Language   string  `json:"language"`
	Confidence float64 `json:"confidence"`
}

func NewLLMLanguageDetector(adapter llm.LLMAdapter) *LLMLanguageDetector {
	return &LLMLanguageDetector{
		adapter: adapter,
		timeout: 2 * time.Second,
	}
}

func (d *LLMLanguageDetector) Detect(text string, meta map[string]string) (string, float64, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", 0, errors.New("empty text")
	}
	if d.adapter == nil {
		return "", 0, errors.New("missing llm adapter")
	}
	ctx, cancel := context.WithTimeout(context.Background(), d.timeout)
	defer cancel()
	input := llm.Context{
		Messages: []map[string]any{
			{"role": "system", "content": detectorPrompt()},
			{"role": "user", "content": text},
		},
	}
	resp, err := d.adapter.Generate(ctx, input)
	if err != nil {
		return "", 0, err
	}
	payload := cleanJSON(resp.Text)
	var out languageDetectOutput
	if err := json.Unmarshal([]byte(payload), &out); err != nil {
		return "", 0, err
	}
	lang := strings.ToLower(strings.TrimSpace(out.Language))
	if lang == "" {
		return "", 0, errors.New("empty language")
	}
	if out.Confidence <= 0 {
		out.Confidence = 0.4
	}
	return lang, out.Confidence, nil
}

func detectorPrompt() string {
	return strings.TrimSpace(`
Kamu adalah detektor bahasa untuk telephony. 
Keluarkan HANYA JSON valid:
{"language":"id|en|other","confidence":0.0-1.0}
Jika ragu, gunakan "other" dengan confidence rendah.
`)
}

var _ processors.LanguageDetector = (*LLMLanguageDetector)(nil)
