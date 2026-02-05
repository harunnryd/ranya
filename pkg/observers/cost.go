package observers

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/harunnryd/ranya/pkg/metrics"
)

type CostSummary struct {
	TraceID       string  `json:"trace_id,omitempty"`
	StreamID      string  `json:"stream_id,omitempty"`
	STTAudioSec   float64 `json:"stt_audio_seconds"`
	TTSAudioSec   float64 `json:"tts_audio_seconds"`
	LLMTokenCount int     `json:"llm_tokens"`
	RecordedAtUTC string  `json:"recorded_at_utc"`
}

type CostObserver struct {
	dir   string
	mu    sync.Mutex
	stats map[string]*CostSummary
}

func NewCostObserver(dir string) *CostObserver {
	return &CostObserver{dir: dir, stats: make(map[string]*CostSummary)}
}

func (o *CostObserver) RecordEvent(ev metrics.MetricsEvent) {
	if strings.TrimSpace(o.dir) == "" {
		return
	}
	id := ""
	streamID := ""
	traceID := ""
	if ev.Tags != nil {
		streamID = ev.Tags["stream_id"]
		traceID = ev.Tags["trace_id"]
		if traceID != "" {
			id = traceID
		} else {
			id = streamID
		}
	}
	if id == "" {
		return
	}

	if ev.Name == "audio_in" || ev.Name == "audio_out" {
		sec := durationFromFields(ev.Fields)
		if sec <= 0 {
			return
		}
		o.mu.Lock()
		stat := o.stats[id]
		if stat == nil {
			stat = &CostSummary{TraceID: traceID, StreamID: streamID}
			o.stats[id] = stat
		}
		if ev.Name == "audio_in" {
			stat.STTAudioSec += sec
		} else {
			stat.TTSAudioSec += sec
		}
		o.mu.Unlock()
		return
	}

	if ev.Name == "llm_done" && ev.Fields != nil {
		if v, ok := ev.Fields["tokens"].(int); ok {
			o.mu.Lock()
			stat := o.stats[id]
			if stat == nil {
				stat = &CostSummary{TraceID: traceID, StreamID: streamID}
				o.stats[id] = stat
			}
			stat.LLMTokenCount += v
			o.mu.Unlock()
		}
	}
}

func (o *CostObserver) Close() error {
	if strings.TrimSpace(o.dir) == "" {
		return nil
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if err := os.MkdirAll(o.dir, 0o755); err != nil {
		return err
	}
	var errOut error
	for id, stat := range o.stats {
		stat.RecordedAtUTC = time.Now().UTC().Format(time.RFC3339)
		b, err := json.MarshalIndent(stat, "", "  ")
		if err != nil {
			errOut = errors.Join(errOut, err)
			continue
		}
		path := filepath.Join(o.dir, sanitizeID(id)+".cost.json")
		if err := os.WriteFile(path, b, 0o644); err != nil {
			errOut = errors.Join(errOut, err)
		}
	}
	return errOut
}

func durationFromFields(fields map[string]any) float64 {
	if fields == nil {
		return 0
	}
	payload, _ := fields["payload_b64"].(string)
	if payload == "" {
		return 0
	}
	raw, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		return 0
	}
	sampleRate := 0
	channels := 1
	if v, ok := fields["sample_rate"].(float64); ok {
		sampleRate = int(v)
	} else if v, ok := fields["sample_rate"].(int); ok {
		sampleRate = v
	}
	if v, ok := fields["channels"].(float64); ok {
		channels = int(v)
	} else if v, ok := fields["channels"].(int); ok {
		channels = v
	}
	if sampleRate <= 0 || channels <= 0 {
		return 0
	}
	return float64(len(raw)) / float64(sampleRate*channels)
}

var _ metrics.Observer = (*CostObserver)(nil)
