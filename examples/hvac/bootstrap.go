package main

import (
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/harunnryd/ranya/pkg/frames"
	"github.com/harunnryd/ranya/pkg/pipeline"
	"github.com/harunnryd/ranya/pkg/ranya"
)

// HVACBootstrap injects initial context and optional image on call start.
type HVACBootstrap struct {
	cfg       ranya.Config
	image     []byte
	imageMIME string
	mu        sync.Mutex
	injected  map[string]bool
}

func NewHVACBootstrap(cfg ranya.Config) *HVACBootstrap {
	var image []byte
	var mime string
	if path := strings.TrimSpace(cfg.Vision.ImagePath); path != "" {
		raw, err := os.ReadFile(path)
		if err != nil {
			slog.Warn("hvac_image_load_failed", "path", path, "error", err)
		} else if len(raw) > 0 {
			image = raw
			mime = strings.TrimSpace(cfg.Vision.ImageMime)
			if mime == "" {
				mime = http.DetectContentType(raw)
			}
			if !strings.HasPrefix(mime, "image/") {
				slog.Warn("hvac_image_invalid_mime", "mime", mime)
				image = nil
				mime = ""
			}
		}
	}
	return &HVACBootstrap{
		cfg:       cfg,
		image:     image,
		imageMIME: mime,
		injected:  make(map[string]bool),
	}
}

func (b *HVACBootstrap) Name() string { return "hvac_bootstrap" }

func (b *HVACBootstrap) Process(f frames.Frame) ([]frames.Frame, error) {
	sf, ok := f.(frames.SystemFrame)
	if !ok {
		return []frames.Frame{f}, nil
	}
	switch sf.Name() {
	case "call_end":
		streamID := sf.Meta()[frames.MetaStreamID]
		if streamID != "" {
			b.mu.Lock()
			delete(b.injected, streamID)
			b.mu.Unlock()
		}
		return []frames.Frame{f}, nil
	case "call_start":
	default:
		return []frames.Frame{f}, nil
	}

	meta := sf.Meta()
	streamID := meta[frames.MetaStreamID]
	traceID := meta[frames.MetaTraceID]

	out := []frames.Frame{f}

	// Seed global context for shared state.
	global := map[string]string{
		frames.MetaStreamID:       streamID,
		"global_channel":          "voice",
		frames.MetaGlobalLanguage: defaultLanguage(b.cfg),
		"global_product":          "hvac",
	}
	if traceID != "" {
		global[frames.MetaTraceID] = traceID
	}
	if from := meta[frames.MetaFromNumber]; from != "" {
		global["global_customer_id"] = from
	}
	out = append(out, frames.NewSystemFrame(streamID, time.Now().UnixNano(), "global_bootstrap", global))

	// Optional: simulate handoff to validate routing.
	if b.cfg.Debug.SimulateHandoff && streamID != "" {
		handoffMeta := map[string]string{
			frames.MetaStreamID:     streamID,
			frames.MetaHandoffAgent: "technical",
			frames.MetaAgent:        "triage",
		}
		if traceID != "" {
			handoffMeta[frames.MetaTraceID] = traceID
		}
		out = append(out, frames.NewControlFrame(streamID, time.Now().UnixNano(), frames.ControlHandoff, handoffMeta))
	}

	// Optional: inject label image to validate vision pipeline.
	if len(b.image) > 0 && streamID != "" && b.markInjected(streamID) {
		imgMeta := map[string]string{frames.MetaImageCaption: "Label AC"}
		if traceID != "" {
			imgMeta[frames.MetaTraceID] = traceID
		}
		out = append(out, frames.NewImageFrameFromPool(streamID, time.Now().UnixNano(), b.image, b.imageMIME, "", imgMeta))
	}

	return out, nil
}

func (b *HVACBootstrap) markInjected(streamID string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.injected[streamID] {
		return false
	}
	b.injected[streamID] = true
	return true
}

func defaultLanguage(cfg ranya.Config) string {
	if strings.TrimSpace(cfg.Languages.Default) != "" {
		return strings.TrimSpace(cfg.Languages.Default)
	}
	return "id"
}

var _ pipeline.FrameProcessor = (*HVACBootstrap)(nil)
