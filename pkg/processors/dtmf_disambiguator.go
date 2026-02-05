package processors

import (
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/harunnryd/ranya/pkg/frames"
	"github.com/harunnryd/ranya/pkg/pipeline"
)

type DTMFDisambiguatorConfig struct {
	Window      time.Duration
	PreferDTMF  bool
	MarkOnly    bool
	MetaKeyFlag string
}

// DTMFDisambiguator drops or marks spoken digit-only text when DTMF was received recently.
type DTMFDisambiguator struct {
	cfg    DTMFDisambiguatorConfig
	mu     sync.Mutex
	lastDT map[string]time.Time
}

var digitOnly = regexp.MustCompile(`^[0-9]+$`)

func NewDTMFDisambiguator(cfg DTMFDisambiguatorConfig) *DTMFDisambiguator {
	if cfg.Window <= 0 {
		cfg.Window = 2 * time.Second
	}
	if cfg.MetaKeyFlag == "" {
		cfg.MetaKeyFlag = frames.MetaDTMFPriority
	}
	return &DTMFDisambiguator{
		cfg:    cfg,
		lastDT: make(map[string]time.Time),
	}
}

func (d *DTMFDisambiguator) Name() string { return "dtmf_disambiguator" }

func (d *DTMFDisambiguator) Process(f frames.Frame) ([]frames.Frame, error) {
	switch f.Kind() {
	case frames.KindSystem:
		sf := f.(frames.SystemFrame)
		if sf.Name() == "call_end" {
			streamID := sf.Meta()[frames.MetaStreamID]
			if streamID != "" {
				d.mu.Lock()
				delete(d.lastDT, streamID)
				d.mu.Unlock()
			}
		}
	case frames.KindControl:
		cf := f.(frames.ControlFrame)
		if cf.Code() == frames.ControlDTMF {
			streamID := cf.Meta()[frames.MetaStreamID]
			if streamID != "" {
				d.mu.Lock()
				d.lastDT[streamID] = time.Now()
				d.mu.Unlock()
			}
		}
	case frames.KindText:
		tf := f.(frames.TextFrame)
		meta := tf.Meta()
		if meta[frames.MetaSource] != "stt" {
			return []frames.Frame{f}, nil
		}
		text := strings.TrimSpace(tf.Text())
		if text == "" || !digitOnly.MatchString(text) {
			return []frames.Frame{f}, nil
		}
		streamID := meta[frames.MetaStreamID]
		if streamID == "" {
			return []frames.Frame{f}, nil
		}
		d.mu.Lock()
		last, ok := d.lastDT[streamID]
		d.mu.Unlock()
		if !ok || time.Since(last) > d.cfg.Window {
			return []frames.Frame{f}, nil
		}
		meta[d.cfg.MetaKeyFlag] = "true"
		if d.cfg.MarkOnly || !d.cfg.PreferDTMF {
			return []frames.Frame{frames.NewTextFrame(streamID, tf.PTS(), tf.Text(), meta)}, nil
		}
		// Prefer DTMF: drop spoken digits to avoid duplication.
		return nil, nil
	}
	return []frames.Frame{f}, nil
}

var _ pipeline.FrameProcessor = (*DTMFDisambiguator)(nil)
