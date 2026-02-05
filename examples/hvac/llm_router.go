package main

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/harunnryd/ranya/pkg/frames"
	"github.com/harunnryd/ranya/pkg/llm"
	"github.com/harunnryd/ranya/pkg/processors"
	"github.com/harunnryd/ranya/pkg/redact"
)

type LLMRouterConfig struct {
	MinConfidence float64
	Timeout       time.Duration
	CacheTTL      time.Duration
}

type LLMRouterStrategy struct {
	adapter  llm.LLMAdapter
	fallback processors.RouterStrategy
	cfg      LLMRouterConfig

	mu    sync.Mutex
	cache map[string]routerDecision
}

type routerDecision struct {
	text       string
	agent      string
	globals    map[string]string
	confidence float64
	ts         time.Time
}

type routerOutput struct {
	Agent           string  `json:"agent"`
	EquipmentType   string  `json:"equipment_type"`
	IssueSummary    string  `json:"issue_summary"`
	Location        string  `json:"location"`
	Urgency         string  `json:"urgency"`
	AppointmentTime string  `json:"appointment_time"`
	Confidence      float64 `json:"confidence"`
}

func NewLLMRouterStrategy(adapter llm.LLMAdapter, fallback processors.RouterStrategy, cfg LLMRouterConfig) *LLMRouterStrategy {
	if cfg.MinConfidence <= 0 {
		cfg.MinConfidence = 0.45
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 2 * time.Second
	}
	if cfg.CacheTTL <= 0 {
		cfg.CacheTTL = 5 * time.Second
	}
	return &LLMRouterStrategy{
		adapter:  adapter,
		fallback: fallback,
		cfg:      cfg,
		cache:    make(map[string]routerDecision),
	}
}

func (s *LLMRouterStrategy) Route(text string, meta map[string]string) string {
	decision, err := s.decide(text, meta)
	if err == nil && decision.agent != "" && decision.confidence >= s.cfg.MinConfidence {
		return decision.agent
	}
	if s.fallback != nil {
		return s.fallback.Route(text, meta)
	}
	return ""
}

func (s *LLMRouterStrategy) ExtractGlobal(text string, meta map[string]string) map[string]string {
	decision, err := s.decide(text, meta)
	if err == nil && decision.confidence >= s.cfg.MinConfidence {
		if len(decision.globals) > 0 {
			return decision.globals
		}
		return nil
	}
	if s.fallback != nil {
		return s.fallback.ExtractGlobal(text, meta)
	}
	return nil
}

func (s *LLMRouterStrategy) decide(text string, meta map[string]string) (routerDecision, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return routerDecision{}, errors.New("empty text")
	}
	if s.adapter == nil {
		return routerDecision{}, errors.New("missing llm adapter")
	}
	streamID := ""
	if meta != nil {
		streamID = meta[frames.MetaStreamID]
	}
	if streamID != "" {
		if cached, ok := s.getCached(streamID, text); ok {
			return cached, nil
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), s.cfg.Timeout)
	defer cancel()
	prompt := routerPrompt()
	input := llm.Context{
		Messages: []map[string]any{
			{"role": "system", "content": prompt},
			{"role": "user", "content": text},
		},
	}
	resp, err := s.adapter.Generate(ctx, input)
	if err != nil {
		slog.Warn("llm_router_error", "error", err, "text", redact.Text(text))
		return routerDecision{}, err
	}

	payload := cleanJSON(resp.Text)
	var out routerOutput
	if err := json.Unmarshal([]byte(payload), &out); err != nil {
		slog.Warn("llm_router_parse_error", "error", err, "payload", redact.Text(resp.Text))
		return routerDecision{}, err
	}

	agent := normalizeAgent(out.Agent)
	globals := buildGlobals(out, meta)
	decision := routerDecision{
		text:       text,
		agent:      agent,
		globals:    globals,
		confidence: clampConfidence(out.Confidence),
		ts:         time.Now(),
	}
	if streamID != "" {
		s.setCached(streamID, decision)
	}
	return decision, nil
}

func (s *LLMRouterStrategy) getCached(streamID, text string) (routerDecision, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if streamID == "" {
		return routerDecision{}, false
	}
	decision, ok := s.cache[streamID]
	if !ok {
		return routerDecision{}, false
	}
	if decision.text != text {
		return routerDecision{}, false
	}
	if time.Since(decision.ts) > s.cfg.CacheTTL {
		delete(s.cache, streamID)
		return routerDecision{}, false
	}
	return decision, true
}

func (s *LLMRouterStrategy) setCached(streamID string, decision routerDecision) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if streamID == "" {
		return
	}
	s.cache[streamID] = decision
}

func routerPrompt() string {
	return strings.TrimSpace(`
Kamu adalah routing engine untuk telephony HVAC.
Output HARUS hanya JSON valid tanpa tambahan teks.
Format:
{
  "agent": "triage|technical|billing|",
  "equipment_type": "",
  "issue_summary": "",
  "location": "",
  "urgency": "low|medium|high|",
  "appointment_time": "",
  "confidence": 0.0
}
Gunakan string kosong jika tidak yakin. Jika ragu, confidence rendah.
Bahasa bisa Indonesia atau Inggris.
`)
}

func normalizeAgent(agent string) string {
	a := strings.ToLower(strings.TrimSpace(agent))
	switch a {
	case "billing", "payment", "finance", "invoice", "cost", "pricing", "promo":
		return "billing"
	case "technical", "tech", "engineer", "diagnosis", "support":
		return "technical"
	case "triage", "scheduling", "schedule", "dispatch", "service", "booking":
		return "triage"
	default:
		return ""
	}
}

func clampConfidence(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func buildGlobals(out routerOutput, meta map[string]string) map[string]string {
	globals := map[string]string{}
	if meta != nil {
		if from := meta[frames.MetaFromNumber]; from != "" {
			globals[frames.MetaGlobalPrefix+"customer_id"] = from
		}
		if traceID := meta[frames.MetaTraceID]; traceID != "" {
			globals[frames.MetaTraceID] = traceID
		}
	}
	if v := strings.TrimSpace(out.EquipmentType); v != "" {
		globals[frames.MetaGlobalPrefix+"equipment_type"] = v
	}
	if v := strings.TrimSpace(out.IssueSummary); v != "" {
		globals[frames.MetaGlobalPrefix+"issue_summary"] = v
	}
	if v := strings.TrimSpace(out.Location); v != "" {
		globals[frames.MetaGlobalPrefix+"location"] = v
	}
	if v := strings.TrimSpace(out.Urgency); v != "" {
		globals[frames.MetaGlobalPrefix+"urgency"] = v
	}
	if v := strings.TrimSpace(out.AppointmentTime); v != "" {
		globals[frames.MetaGlobalPrefix+"appointment_time"] = v
	}
	if len(globals) == 0 {
		return nil
	}
	return globals
}

var _ processors.RouterStrategy = (*LLMRouterStrategy)(nil)
