package processors

import (
	"bytes"
	"encoding/base64"
	"os"
	"strings"
	"sync"

	"github.com/harunnryd/ranya/pkg/frames"
	"github.com/harunnryd/ranya/pkg/pipeline"
)

type FillerProcessor struct {
	mu     sync.Mutex
	active map[string]bool
	chunks [][]byte
}

func NewFillerProcessor(path string) *FillerProcessor {
	raw := loadFiller(path)
	if len(raw) < 160 {
		raw = bytes.Repeat([]byte{0xFF}, 160*5)
	}
	var chunks [][]byte
	for i := 0; i+160 <= len(raw); i += 160 {
		chunks = append(chunks, raw[i:i+160])
	}
	return &FillerProcessor{
		active: make(map[string]bool),
		chunks: chunks,
	}
}

func (p *FillerProcessor) Name() string { return "filler" }

func (p *FillerProcessor) Process(f frames.Frame) ([]frames.Frame, error) {
	switch f.Kind() {
	case frames.KindSystem:
		sf := f.(frames.SystemFrame)
		streamID := sf.Meta()[frames.MetaStreamID]
		if sf.Name() == "call_end" {
			p.clear(streamID)
			return []frames.Frame{f}, nil
		}
		if sf.Name() == "thinking_start" {
			return p.play(streamID, sf.Meta()), nil
		}
		if sf.Name() == "thinking_end" {
			p.clear(streamID)
			return []frames.Frame{f}, nil
		}
	case frames.KindControl:
		cf := f.(frames.ControlFrame)
		if cf.Code() == frames.ControlFlush || cf.Code() == frames.ControlCancel {
			p.clear(cf.Meta()[frames.MetaStreamID])
		}
	}
	return []frames.Frame{f}, nil
}

func (p *FillerProcessor) play(streamID string, meta map[string]string) []frames.Frame {
	p.mu.Lock()
	if p.active[streamID] {
		p.mu.Unlock()
		return nil
	}
	p.active[streamID] = true
	p.mu.Unlock()
	var out []frames.Frame
	for _, c := range p.chunks {
		frameMeta := map[string]string{"encoding": "mulaw"}
		for k, v := range meta {
			frameMeta[k] = v
		}
		out = append(out, frames.NewAudioFrameFromPool(streamID, 0, c, 8000, 1, frameMeta))
	}
	return out
}

func (p *FillerProcessor) clear(streamID string) {
	p.mu.Lock()
	delete(p.active, streamID)
	p.mu.Unlock()
}

func loadFiller(path string) []byte {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	if strings.HasSuffix(path, ".b64") {
		s := strings.TrimSpace(string(b))
		if s == "" {
			return nil
		}
		decoded, err := base64.StdEncoding.DecodeString(s)
		if err == nil && len(decoded) > 0 {
			return decoded
		}
	}
	return b
}

var _ pipeline.FrameProcessor = (*FillerProcessor)(nil)
