package processors

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/harunnryd/ranya/pkg/frames"
	"github.com/harunnryd/ranya/pkg/pipeline"
)

type RouterStrategy interface {
	Route(text string, meta map[string]string) string
	ExtractGlobal(text string, meta map[string]string) map[string]string
}

type LanguageDetector interface {
	Detect(text string, meta map[string]string) (string, float64, error)
}

type RouterProcessorConfig struct {
	Mode          string
	MaxTurns      int
	CodeSwitching bool
}

type RouterProcessor struct {
	mu                sync.Mutex
	active            map[string]string
	langActive        map[string]string
	turnCount         map[string]int
	strategy          RouterStrategy
	langDetector      LanguageDetector
	langMinConfidence float64
	langPrompts       map[string]string
	mode              string
	maxTurns          int
	codeSwitching     bool
}

func NewRouterProcessor(strategy RouterStrategy) *RouterProcessor {
	return &RouterProcessor{
		active:            make(map[string]string),
		langActive:        make(map[string]string),
		turnCount:         make(map[string]int),
		strategy:          strategy,
		langMinConfidence: 0.5,
		mode:              "full",
		maxTurns:          2,
		codeSwitching:     true,
	}
}

func (p *RouterProcessor) Name() string { return "router" }

func (p *RouterProcessor) SetConfig(cfg RouterProcessorConfig) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.mode = normalizeRouterMode(cfg.Mode)
	if cfg.MaxTurns <= 0 {
		cfg.MaxTurns = 2
	}
	p.maxTurns = cfg.MaxTurns
	p.codeSwitching = cfg.CodeSwitching
}

func (p *RouterProcessor) SetLanguageDetector(detector LanguageDetector, minConfidence float64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.langDetector = detector
	if minConfidence > 0 {
		p.langMinConfidence = minConfidence
	}
}

func (p *RouterProcessor) SetLanguagePrompts(prompts map[string]string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.langPrompts = prompts
}

func (p *RouterProcessor) Process(f frames.Frame) ([]frames.Frame, error) {
	switch f.Kind() {
	case frames.KindControl:
		cf := f.(frames.ControlFrame)
		if cf.Code() == frames.ControlHandoff {
			agent := cf.Meta()["handoff_agent"]
			if agent != "" {
				p.setAgent(cf.Meta()[frames.MetaStreamID], agent)
				sys := map[string]string{frames.MetaGlobalAgent: agent, frames.MetaSystemMessage: "Handoff ke agent " + agent}
				if traceID := cf.Meta()[frames.MetaTraceID]; traceID != "" {
					sys[frames.MetaTraceID] = traceID
				}
				return []frames.Frame{frames.NewSystemFrame(cf.Meta()[frames.MetaStreamID], time.Now().UnixNano(), "global_update", sys), f}, nil
			}
		}
	case frames.KindSystem:
		sf := f.(frames.SystemFrame)
		if sf.Name() == "call_end" {
			p.resetStream(sf.Meta()[frames.MetaStreamID])
		}
	case frames.KindText:
		tf := f.(frames.TextFrame)
		meta := tf.Meta()
		streamID := meta[frames.MetaStreamID]
		if meta[frames.MetaSource] == "stt" && p.strategy != nil {
			var out []frames.Frame
			final := isFinal(meta)
			if final && (p.codeSwitching || !p.hasLanguage(streamID, meta)) {
				lang, conf := p.detectLanguage(tf.Text(), meta)
				if lang != "" && conf >= p.langMinConfidence {
					meta[frames.MetaLanguage] = lang
					meta[frames.MetaLanguageConfidence] = formatConfidence(conf)
					if p.updateLanguage(streamID, lang) {
						global := map[string]string{
							frames.MetaStreamID:       streamID,
							frames.MetaGlobalLanguage: lang,
						}
						if traceID := meta[frames.MetaTraceID]; traceID != "" {
							global[frames.MetaTraceID] = traceID
						}
						out = append(out, frames.NewSystemFrame(streamID, time.Now().UnixNano(), "global_update", global))
						if prompt := p.languagePrompt(lang); prompt != "" {
							sys := map[string]string{frames.MetaSystemMessage: prompt}
							if traceID := meta[frames.MetaTraceID]; traceID != "" {
								sys[frames.MetaTraceID] = traceID
							}
							out = append(out, frames.NewSystemFrame(streamID, time.Now().UnixNano(), "language_prompt", sys))
						}
					}
				}
			}

			if final && p.shouldRoute(streamID) {
				agent := p.strategy.Route(tf.Text(), meta)
				if agent != "" {
					p.setAgent(streamID, agent)
				}
				global := p.strategy.ExtractGlobal(tf.Text(), meta)
				if global != nil {
					out = append(out, frames.NewSystemFrame(streamID, time.Now().UnixNano(), "global_update", global))
				}
				p.incrementTurn(streamID)
			}

			if agent := p.getAgent(streamID); agent != "" {
				meta[frames.MetaAgent] = agent
			}
			out = append(out, frames.NewTextFrame(streamID, tf.PTS(), tf.Text(), meta))
			return out, nil
		}
		if agent := p.getAgent(streamID); agent != "" {
			meta[frames.MetaAgent] = agent
			return []frames.Frame{frames.NewTextFrame(streamID, tf.PTS(), tf.Text(), meta)}, nil
		}
	}
	return []frames.Frame{f}, nil
}

func (p *RouterProcessor) setAgent(streamID, agent string) {
	if streamID == "" || agent == "" {
		return
	}
	p.mu.Lock()
	p.active[streamID] = agent
	p.mu.Unlock()
}

func (p *RouterProcessor) getAgent(streamID string) string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.active[streamID]
}

func (p *RouterProcessor) hasLanguage(streamID string, meta map[string]string) bool {
	if meta != nil {
		if meta[frames.MetaLanguage] != "" || meta[frames.MetaGlobalLanguage] != "" {
			return true
		}
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.langActive[streamID] != ""
}

func (p *RouterProcessor) shouldRoute(streamID string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.strategy == nil {
		return false
	}
	mode := normalizeRouterMode(p.mode)
	switch mode {
	case "off":
		return false
	case "full":
		return true
	case "bootstrap":
		return p.turnCount[streamID] < p.maxTurns
	default:
		return true
	}
}

func (p *RouterProcessor) incrementTurn(streamID string) {
	if streamID == "" {
		return
	}
	p.mu.Lock()
	p.turnCount[streamID]++
	p.mu.Unlock()
}

func (p *RouterProcessor) detectLanguage(text string, meta map[string]string) (string, float64) {
	p.mu.Lock()
	detector := p.langDetector
	p.mu.Unlock()
	if detector == nil {
		return "", 0
	}
	lang, conf, err := detector.Detect(text, meta)
	if err != nil {
		return "", 0
	}
	return lang, conf
}

func (p *RouterProcessor) updateLanguage(streamID, lang string) bool {
	if streamID == "" || lang == "" {
		return false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.langActive[streamID] == lang {
		return false
	}
	p.langActive[streamID] = lang
	return true
}

func (p *RouterProcessor) languagePrompt(lang string) string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.langPrompts == nil {
		return ""
	}
	return p.langPrompts[lang]
}

func (p *RouterProcessor) resetStream(streamID string) {
	if streamID == "" {
		return
	}
	p.mu.Lock()
	delete(p.active, streamID)
	delete(p.langActive, streamID)
	delete(p.turnCount, streamID)
	p.mu.Unlock()
}

func normalizeRouterMode(mode string) string {
	mode = strings.ToLower(strings.TrimSpace(mode))
	switch mode {
	case "off", "disabled", "none":
		return "off"
	case "bootstrap", "hybrid", "warmup":
		return "bootstrap"
	case "full", "":
		return "full"
	default:
		return "full"
	}
}

func formatConfidence(v float64) string {
	if v < 0 {
		v = 0
	}
	if v > 1 {
		v = 1
	}
	return fmt.Sprintf("%.2f", v)
}

var _ pipeline.FrameProcessor = (*RouterProcessor)(nil)
