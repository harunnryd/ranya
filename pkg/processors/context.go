package processors

import (
	"encoding/base64"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/harunnryd/ranya/pkg/aggregators"
	"github.com/harunnryd/ranya/pkg/frames"
	"github.com/harunnryd/ranya/pkg/pipeline"
	"github.com/harunnryd/ranya/pkg/turn"
)

type ContextProcessor struct {
	aggConfig      aggregators.AggregatorConfig
	aggs           map[string]*aggregators.TextAggregator
	basePrompt     string
	injected       map[string]bool
	global         map[string]map[string]string
	globalHash     map[string]string
	DefaultCaption string

	// Speculative buffer integration
	buffer       *ContextBuffer
	turnManager  turn.Manager
	mu           sync.Mutex
	pendingFlush []frames.Frame
}

func NewContextProcessor(cfg aggregators.AggregatorConfig, basePrompt string) *ContextProcessor {
	return &ContextProcessor{
		aggConfig:      cfg,
		aggs:           make(map[string]*aggregators.TextAggregator),
		injected:       make(map[string]bool),
		basePrompt:     basePrompt,
		global:         make(map[string]map[string]string),
		globalHash:     make(map[string]string),
		DefaultCaption: "User image",
	}
}

// SetTurnManager sets the turn manager for event-driven flushing
// When set, the processor will use speculative buffering with event-driven flush
func (p *ContextProcessor) SetTurnManager(tm turn.Manager) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.turnManager = tm

	// Create buffer with flush handler that stores frames for later emission
	if tm != nil {
		p.buffer = NewContextBuffer(
			ContextBufferOptions{
				MaxBufferSize: 10000,
				StreamID:      "",
			},
			func(content string) error {
				// Create a text frame from the flushed content
				// We'll emit this during the next Process call
				p.mu.Lock()
				defer p.mu.Unlock()

				if content != "" {
					// Create frame with appropriate metadata
					meta := map[string]string{frames.MetaIsFinal: "true"}
					streamID := ""
					if p.buffer != nil {
						streamID = p.buffer.StreamID()
					}
					tf := frames.NewTextFrame(streamID, time.Now().UnixNano(), content, meta)
					p.pendingFlush = append(p.pendingFlush, tf)
				}
				return nil
			},
		)

		// Register buffer as state change listener.
		tm.AddListener(p.buffer)
	}
}

func (p *ContextProcessor) SetDefaultCaption(caption string) {
	p.DefaultCaption = caption
}

func (p *ContextProcessor) Name() string { return "context_processor" }

func (p *ContextProcessor) Process(f frames.Frame) ([]frames.Frame, error) {
	// Check for pending flush frames first
	p.mu.Lock()
	pending := p.pendingFlush
	p.pendingFlush = nil
	p.mu.Unlock()

	var out []frames.Frame

	// Add any pending flush frames
	if len(pending) > 0 {
		// Process pending frames through the aggregator
		for _, pf := range pending {
			if pf.Kind() == frames.KindText {
				tf := pf.(frames.TextFrame)
				// Add context wrapping
				if sys := p.buildBasePrompt(tf.Meta()); sys != nil {
					out = append(out, *sys)
				}
				if sys := p.buildGlobalMessage(tf.Meta()); sys != nil {
					out = append(out, *sys)
				}
				// Process through aggregator
				agg := p.aggFor(tf.Meta()[frames.MetaStreamID])
				r, err := agg.Process(tf)
				if err != nil {
					return out, err
				}
				out = append(out, r...)
			}
		}
	}

	if f.Kind() == frames.KindSystem {
		sf := f.(frames.SystemFrame)
		p.updateGlobal(sf.Meta())
		if sf.Name() == "call_end" {
			p.clearScope(sf.Meta())
			p.clearAgg(sf.Meta()[frames.MetaStreamID])
		}
		if sys := p.buildBasePrompt(sf.Meta()); sys != nil {
			return append(out, *sys, f), nil
		}
		if sf.Name() == "call_start" {
			return append(out, f), nil
		}
	}

	if f.Kind() == frames.KindControl {
		cf := f.(frames.ControlFrame)
		if cf.Code() == frames.ControlDTMF {
			meta := cf.Meta()
			digit := meta[frames.MetaDTMFDigit]
			if digit != "" {
				text := "DTMF input: " + digit
				meta[frames.MetaSource] = "dtmf"
				meta[frames.MetaIsFinal] = "true"
				tf := frames.NewTextFrame(meta[frames.MetaStreamID], time.Now().UnixNano(), text, meta)
				if sys := p.buildBasePrompt(tf.Meta()); sys != nil {
					out = append(out, *sys)
				}
				if sys := p.buildGlobalMessage(tf.Meta()); sys != nil {
					out = append(out, *sys)
				}
				agg := p.aggFor(tf.Meta()[frames.MetaStreamID])
				r, err := agg.Process(tf)
				if err != nil {
					return out, err
				}
				out = append(out, r...)
				out = append(out, f)
				return out, nil
			}
		}
		if cf.Code() == frames.ControlFlush {
			// If using speculative buffer, trigger flush through turn manager
			// Otherwise use the direct aggregator flush
			if p.buffer != nil {
				p.buffer.Flush()
			} else {
				// Direct flush behavior
				agg := p.aggFor(cf.Meta()[frames.MetaStreamID])
				if tf := agg.FlushFrame(); tf != nil {
					if sys := p.buildBasePrompt(tf.Meta()); sys != nil {
						out = append(out, *sys)
					}
					if sys := p.buildGlobalMessage(tf.Meta()); sys != nil {
						out = append(out, *sys)
					}
					out = append(out, *tf)
				}
			}
			out = append(out, f)
			return out, nil
		}
	}

	if f.Kind() == frames.KindText {
		tf := f.(frames.TextFrame)

		// If using speculative buffer, update it instead of using aggregator directly
		if p.buffer != nil {
			if streamID := tf.Meta()[frames.MetaStreamID]; streamID != "" {
				p.buffer.SetStreamID(streamID)
			}
			isFinal := isFinal(tf.Meta())
			p.buffer.AddTranscript(tf.Text(), isFinal)

			// For interim results, don't emit anything
			if !isFinal {
				return out, nil
			}

			// For final results, still return them but don't flush yet
			// The flush will happen on state transition
			return out, nil
		}

		// Direct behavior: only process final transcripts
		if !isFinal(tf.Meta()) {
			return out, nil
		}

		if sys := p.buildBasePrompt(tf.Meta()); sys != nil {
			out = append(out, *sys)
		}
		if sys := p.buildGlobalMessage(tf.Meta()); sys != nil {
			out = append(out, *sys)
		}
		agg := p.aggFor(tf.Meta()[frames.MetaStreamID])
		r, err := agg.Process(tf)
		if err != nil {
			return out, err
		}
		out = append(out, r...)
		return out, nil
	}

	if f.Kind() == frames.KindImage {
		im := f.(frames.ImageFrame)
		meta := im.Meta()
		caption := meta[frames.MetaImageCaption]
		if caption == "" {
			caption = p.DefaultCaption
		}
		if im.URL() != "" {
			meta[frames.MetaImageURL] = im.URL()
		} else if len(im.RawPayload()) > 0 {
			mime := im.MIME()
			if mime == "" {
				mime = http.DetectContentType(im.RawPayload())
			}
			if strings.HasPrefix(mime, "image/") {
				meta[frames.MetaImageMIME] = mime
				meta[frames.MetaImageBase64] = base64.StdEncoding.EncodeToString(im.RawPayload())
			} else {
				slog.Warn("context_invalid_image_mime", "stream_id", meta[frames.MetaStreamID], "mime", mime)
				delete(meta, frames.MetaImageBase64)
			}
		}
		frames.ReleaseImageFrame(f)
		return append(out, frames.NewTextFrame(meta[frames.MetaStreamID], time.Now().UnixNano(), caption, meta)), nil
	}

	// For other frame types, pass through aggregator
	streamID := f.Meta()[frames.MetaStreamID]
	if streamID == "" {
		return append(out, f), nil
	}
	agg := p.aggFor(streamID)
	r, err := agg.Process(f)
	if err != nil {
		return out, err
	}
	out = append(out, r...)
	return out, nil
}

func isFinal(meta map[string]string) bool {
	v := strings.ToLower(meta[frames.MetaIsFinal])
	return v == "true" || v == "1" || v == "yes"
}

func (p *ContextProcessor) aggFor(streamID string) *aggregators.TextAggregator {
	if streamID == "" {
		streamID = "default"
	}
	p.mu.Lock()
	agg := p.aggs[streamID]
	if agg == nil {
		agg = aggregators.NewTextAggregator(p.aggConfig)
		p.aggs[streamID] = agg
	}
	p.mu.Unlock()
	return agg
}

func (p *ContextProcessor) clearAgg(streamID string) {
	if streamID == "" {
		return
	}
	p.mu.Lock()
	delete(p.aggs, streamID)
	p.mu.Unlock()
}

var _ pipeline.FrameProcessor = (*ContextProcessor)(nil)

func (p *ContextProcessor) buildBasePrompt(meta map[string]string) *frames.SystemFrame {
	if p.basePrompt == "" {
		return nil
	}
	streamID := meta[frames.MetaStreamID]
	scope := p.scopeKey(meta)
	if streamID == "" || scope == "" || p.injected[scope] {
		return nil
	}
	p.injected[scope] = true
	sysMeta := map[string]string{frames.MetaSystemMessage: p.basePrompt}
	if traceID := meta[frames.MetaTraceID]; traceID != "" {
		sysMeta[frames.MetaTraceID] = traceID
	}
	frame := frames.NewSystemFrame(streamID, time.Now().UnixNano(), "base_prompt", sysMeta)
	return &frame
}

func (p *ContextProcessor) updateGlobal(meta map[string]string) {
	scope := p.scopeKey(meta)
	if scope == "" {
		return
	}
	g := p.global[scope]
	if g == nil {
		g = make(map[string]string)
		p.global[scope] = g
	}
	for k, v := range meta {
		if strings.HasPrefix(k, frames.MetaGlobalPrefix) && v != "" {
			g[k[len(frames.MetaGlobalPrefix):]] = v
		}
	}
	if from := meta[frames.MetaFromNumber]; from != "" {
		g["customer_id"] = from
	}
}

func (p *ContextProcessor) buildGlobalMessage(meta map[string]string) *frames.SystemFrame {
	streamID := meta[frames.MetaStreamID]
	scope := p.scopeKey(meta)
	if streamID == "" || scope == "" {
		return nil
	}
	g := p.global[scope]
	if len(g) == 0 {
		return nil
	}
	hash := globalHash(g)
	if p.globalHash[scope] == hash {
		return nil
	}
	p.globalHash[scope] = hash
	systemMsg := "Shared context: " + hash
	sysMeta := map[string]string{frames.MetaSystemMessage: systemMsg}
	if traceID := meta[frames.MetaTraceID]; traceID != "" {
		sysMeta[frames.MetaTraceID] = traceID
	}
	frame := frames.NewSystemFrame(streamID, time.Now().UnixNano(), "global_context", sysMeta)
	return &frame
}

func globalHash(g map[string]string) string {
	if len(g) == 0 {
		return ""
	}
	keys := make([]string, 0, len(g))
	for k := range g {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for i, k := range keys {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(k)
		b.WriteString("=")
		b.WriteString(g[k])
	}
	return b.String()
}

func (p *ContextProcessor) scopeKey(meta map[string]string) string {
	if meta == nil {
		return ""
	}
	if callSID := meta[frames.MetaCallSID]; callSID != "" {
		return callSID
	}
	return meta[frames.MetaStreamID]
}

func (p *ContextProcessor) clearScope(meta map[string]string) {
	scope := p.scopeKey(meta)
	if scope == "" {
		return
	}
	p.mu.Lock()
	delete(p.injected, scope)
	delete(p.global, scope)
	delete(p.globalHash, scope)
	p.mu.Unlock()
}
