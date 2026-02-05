package processors

import (
	"strings"
	"sync"
	"time"

	"github.com/harunnryd/ranya/pkg/frames"
	"github.com/harunnryd/ranya/pkg/metrics"
	"github.com/harunnryd/ranya/pkg/pipeline"
)

type SummaryConfig struct {
	MaxEntries int
	MaxChars   int
}

type SummaryProcessor struct {
	cfg         SummaryConfig
	mu          sync.Mutex
	entries     map[string][]summaryEntry
	lastLang    map[string]string
	lastTraceID map[string]string
	lastCallSID map[string]string
	obs         metrics.Observer
}

type summaryEntry struct {
	role string
	text string
}

func NewSummaryProcessor(cfg SummaryConfig) *SummaryProcessor {
	if cfg.MaxEntries <= 0 {
		cfg.MaxEntries = 8
	}
	if cfg.MaxChars <= 0 {
		cfg.MaxChars = 600
	}
	return &SummaryProcessor{
		cfg:         cfg,
		entries:     make(map[string][]summaryEntry),
		lastLang:    make(map[string]string),
		lastTraceID: make(map[string]string),
		lastCallSID: make(map[string]string),
	}
}

func (p *SummaryProcessor) Name() string { return "summary_processor" }

func (p *SummaryProcessor) SetObserver(obs metrics.Observer) { p.obs = obs }

func (p *SummaryProcessor) Process(f frames.Frame) ([]frames.Frame, error) {
	streamID := f.Meta()[frames.MetaStreamID]
	if streamID == "" {
		return []frames.Frame{f}, nil
	}
	if traceID := f.Meta()[frames.MetaTraceID]; traceID != "" {
		p.mu.Lock()
		p.lastTraceID[streamID] = traceID
		p.mu.Unlock()
	}
	if callSID := f.Meta()[frames.MetaCallSID]; callSID != "" {
		p.mu.Lock()
		p.lastCallSID[streamID] = callSID
		p.mu.Unlock()
	}
	if lang := strings.ToLower(strings.TrimSpace(f.Meta()[frames.MetaLanguage])); lang != "" {
		p.mu.Lock()
		p.lastLang[streamID] = lang
		p.mu.Unlock()
	}

	switch f.Kind() {
	case frames.KindText:
		tf := f.(frames.TextFrame)
		meta := tf.Meta()
		if meta[frames.MetaSource] == "stt" && !isFinal(meta) {
			return []frames.Frame{f}, nil
		}
		role := "user"
		if meta[frames.MetaSource] == "llm" {
			role = "agent"
		}
		p.append(streamID, role, tf.Text())
	case frames.KindSystem:
		sf := f.(frames.SystemFrame)
		if sf.Name() == "call_end" {
			summary := p.buildSummary(streamID)
			meta := map[string]string{
				frames.MetaStreamID:    streamID,
				frames.MetaCallSummary: summary,
			}
			if traceID := p.getTrace(streamID); traceID != "" {
				meta[frames.MetaTraceID] = traceID
			}
			if callSID := p.getCallSID(streamID); callSID != "" {
				meta[frames.MetaCallSID] = callSID
			}
			if lang := p.getLang(streamID); lang != "" {
				meta[frames.MetaLanguage] = lang
			}
			p.recordSummary(streamID, summary)
			p.clear(streamID)
			return []frames.Frame{frames.NewSystemFrame(streamID, time.Now().UnixNano(), "call_summary", meta), f}, nil
		}
	}
	return []frames.Frame{f}, nil
}

func (p *SummaryProcessor) append(streamID, role, text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	entries := append(p.entries[streamID], summaryEntry{role: role, text: text})
	if len(entries) > p.cfg.MaxEntries {
		entries = entries[len(entries)-p.cfg.MaxEntries:]
	}
	p.entries[streamID] = entries
}

func (p *SummaryProcessor) buildSummary(streamID string) string {
	p.mu.Lock()
	entries := p.entries[streamID]
	lang := p.lastLang[streamID]
	p.mu.Unlock()
	if len(entries) == 0 {
		return defaultSummary(lang)
	}
	lastUser := ""
	lastAgent := ""
	for i := len(entries) - 1; i >= 0; i-- {
		if lastUser == "" && entries[i].role == "user" {
			lastUser = entries[i].text
		}
		if lastAgent == "" && entries[i].role == "agent" {
			lastAgent = entries[i].text
		}
		if lastUser != "" && lastAgent != "" {
			break
		}
	}
	summary := composeSummary(lang, lastUser, lastAgent)
	if len(summary) > p.cfg.MaxChars {
		summary = summary[:p.cfg.MaxChars]
	}
	return summary
}

func composeSummary(lang, lastUser, lastAgent string) string {
	if isEnglish(lang) {
		return "Summary: User said \"" + clipSummary(lastUser) + "\". Agent responded \"" + clipSummary(lastAgent) + "\"."
	}
	return "Ringkasan: User mengatakan \"" + clipSummary(lastUser) + "\". Agent merespons \"" + clipSummary(lastAgent) + "\"."
}

func defaultSummary(lang string) string {
	if isEnglish(lang) {
		return "Summary: call ended."
	}
	return "Ringkasan: panggilan selesai."
}

func clipSummary(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return "-"
	}
	if len(text) <= 120 {
		return text
	}
	return text[:120] + "..."
}

func (p *SummaryProcessor) recordSummary(streamID, summary string) {
	if p.obs == nil {
		return
	}
	tags := map[string]string{frames.MetaStreamID: streamID, "component": "summary"}
	if traceID := p.getTrace(streamID); traceID != "" {
		tags[frames.MetaTraceID] = traceID
	}
	if callSID := p.getCallSID(streamID); callSID != "" {
		tags[frames.MetaCallSID] = callSID
	}
	p.obs.RecordEvent(metrics.MetricsEvent{
		Name:   "call_summary",
		Time:   time.Now(),
		Tags:   tags,
		Fields: map[string]any{"summary": summary},
	})
}

func (p *SummaryProcessor) clear(streamID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.entries, streamID)
	delete(p.lastLang, streamID)
	delete(p.lastTraceID, streamID)
	delete(p.lastCallSID, streamID)
}

func (p *SummaryProcessor) getTrace(streamID string) string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.lastTraceID[streamID]
}

func (p *SummaryProcessor) getCallSID(streamID string) string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.lastCallSID[streamID]
}

func (p *SummaryProcessor) getLang(streamID string) string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.lastLang[streamID]
}

var _ pipeline.FrameProcessor = (*SummaryProcessor)(nil)
