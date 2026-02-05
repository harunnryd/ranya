package observers

import (
	"log/slog"
	"sync"
	"time"

	"github.com/harunnryd/ranya/pkg/metrics"
)

type LatencyObserver struct {
	mu     sync.Mutex
	traces map[string]*trace
	log    *slog.Logger
}

type trace struct {
	audioIn  time.Time
	sttFinal time.Time
	llmFirst time.Time
	ttsFirst time.Time
	llmDone  time.Time
	traceID  string
}

func NewLatencyObserver(log *slog.Logger) *LatencyObserver {
	if log == nil {
		log = slog.Default()
	}
	return &LatencyObserver{
		traces: make(map[string]*trace),
		log:    log,
	}
}

func (o *LatencyObserver) RecordEvent(ev metrics.MetricsEvent) {
	streamID := ""
	if ev.Tags != nil {
		streamID = ev.Tags["stream_id"]
	}
	if streamID == "" {
		return
	}
	o.mu.Lock()
	t := o.traces[streamID]
	if t == nil {
		t = &trace{}
		o.traces[streamID] = t
	}
	switch ev.Name {
	case "stt_audio_in":
		if t.audioIn.IsZero() {
			t.audioIn = ev.Time
		}
		if t.traceID == "" && ev.Tags != nil {
			t.traceID = ev.Tags["trace_id"]
		}
	case "stt_final":
		if t.sttFinal.IsZero() {
			t.sttFinal = ev.Time
		}
	case "llm_first_token":
		if t.llmFirst.IsZero() {
			t.llmFirst = ev.Time
		}
	case "tts_first_audio":
		if t.ttsFirst.IsZero() {
			t.ttsFirst = ev.Time
		}
	case "llm_done":
		t.llmDone = ev.Time
	}
	if !t.llmDone.IsZero() {
		o.logTTFBLocked(streamID, t)
		delete(o.traces, streamID)
	}
	o.mu.Unlock()
}

func (o *LatencyObserver) logTTFBLocked(streamID string, t *trace) {
	sttLatency := durationMs(t.audioIn, t.sttFinal)
	llmLatency := durationMs(t.sttFinal, t.llmFirst)
	ttsLatency := durationMs(t.llmFirst, t.ttsFirst)
	ttfb := durationMs(t.sttFinal, t.ttsFirst)
	o.log.Info("latency",
		"stream_id", streamID,
		"trace_id", t.traceID,
		"stt_ms", sttLatency,
		"llm_first_token_ms", llmLatency,
		"tts_first_audio_ms", ttsLatency,
		"ttfb_ms", ttfb,
	)
}

func durationMs(a, b time.Time) int64 {
	if a.IsZero() || b.IsZero() {
		return -1
	}
	return b.Sub(a).Milliseconds()
}
