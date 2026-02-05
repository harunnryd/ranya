package main

import (
	"sync"

	"github.com/harunnryd/ranya/pkg/frames"
	"github.com/harunnryd/ranya/pkg/pipeline"
)

// BadNetworkSimulator drops a deterministic subset of inbound audio frames
// to emulate packet loss in debug mode.
type BadNetworkSimulator struct {
	enabled   bool
	dropEvery int
	mu        sync.Mutex
	counter   map[string]int
}

func NewBadNetworkSimulator(enabled bool) *BadNetworkSimulator {
	return &BadNetworkSimulator{
		enabled:   enabled,
		dropEvery: 7,
		counter:   make(map[string]int),
	}
}

func (b *BadNetworkSimulator) Name() string { return "bad_network_simulator" }

func (b *BadNetworkSimulator) Process(f frames.Frame) ([]frames.Frame, error) {
	if !b.enabled {
		return []frames.Frame{f}, nil
	}
	switch f.Kind() {
	case frames.KindSystem:
		sf := f.(frames.SystemFrame)
		if sf.Name() == "call_end" {
			streamID := sf.Meta()[frames.MetaStreamID]
			if streamID != "" {
				b.mu.Lock()
				delete(b.counter, streamID)
				b.mu.Unlock()
			}
		}
		return []frames.Frame{f}, nil
	case frames.KindAudio:
		af := f.(frames.AudioFrame)
		streamID := af.Meta()[frames.MetaStreamID]
		if streamID == "" {
			return []frames.Frame{f}, nil
		}
		b.mu.Lock()
		b.counter[streamID]++
		n := b.counter[streamID]
		b.mu.Unlock()
		if n%b.dropEvery == 0 {
			frames.ReleaseAudioFrame(f)
			return nil, nil
		}
	}
	return []frames.Frame{f}, nil
}

var _ pipeline.FrameProcessor = (*BadNetworkSimulator)(nil)
