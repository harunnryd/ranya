package observers

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/harunnryd/ranya/pkg/metrics"
	"github.com/harunnryd/ranya/pkg/redact"
)

// TimelineObserver writes a per-call timeline JSONL trace.
type TimelineObserver struct {
	dir   string
	mu    sync.Mutex
	files map[string]*os.File
}

// NewTimelineObserver creates a new timeline observer writing to dir.
func NewTimelineObserver(dir string) *TimelineObserver {
	return &TimelineObserver{dir: dir, files: make(map[string]*os.File)}
}

// RecordEvent implements metrics.Observer.
func (o *TimelineObserver) RecordEvent(ev metrics.MetricsEvent) {
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
	if id == "" || strings.TrimSpace(o.dir) == "" {
		return
	}
	name := mapEventName(ev)
	tags := copyTags(ev.Tags)
	fields := sanitizeFields(ev.Fields)
	entry := timelineEvent{
		Time:     ev.Time.UTC(),
		Event:    name,
		StreamID: streamID,
		TraceID:  traceID,
		Tags:     tags,
		Fields:   fields,
	}

	line, err := json.Marshal(entry)
	if err != nil {
		return
	}

	f := o.fileFor(id)
	if f == nil {
		return
	}
	_, _ = f.Write(append(line, '\n'))
}

// Close closes any open files.
func (o *TimelineObserver) Close() error {
	o.mu.Lock()
	defer o.mu.Unlock()
	var err error
	for _, f := range o.files {
		if f == nil {
			continue
		}
		if cerr := f.Close(); cerr != nil {
			err = errors.Join(err, cerr)
		}
	}
	o.files = make(map[string]*os.File)
	return err
}

type timelineEvent struct {
	Time     time.Time         `json:"time"`
	Event    string            `json:"event"`
	StreamID string            `json:"stream_id,omitempty"`
	TraceID  string            `json:"trace_id,omitempty"`
	Tags     map[string]string `json:"tags,omitempty"`
	Fields   map[string]any    `json:"fields,omitempty"`
}

func (o *TimelineObserver) fileFor(id string) *os.File {
	safe := sanitizeID(id)
	if safe == "" {
		return nil
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if f := o.files[safe]; f != nil {
		return f
	}
	if err := os.MkdirAll(o.dir, 0o755); err != nil {
		return nil
	}
	path := filepath.Join(o.dir, safe+".jsonl")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil
	}
	o.files[safe] = f
	return f
}

func mapEventName(ev metrics.MetricsEvent) string {
	if ev.Name == "frame_in" && ev.Tags != nil && ev.Tags["kind"] == "audio" {
		return "audio_in"
	}
	if ev.Name == "frame_out" && ev.Tags != nil && ev.Tags["kind"] == "audio" {
		return "audio_out"
	}
	return ev.Name
}

func sanitizeID(id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return ""
	}
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z':
			return r
		case r >= 'A' && r <= 'Z':
			return r
		case r >= '0' && r <= '9':
			return r
		case r == '-' || r == '_' || r == '.':
			return r
		default:
			return '_'
		}
	}, id)
}

func copyTags(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func sanitizeFields(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		if s, ok := v.(string); ok {
			if strings.Contains(k, "payload_b64") || strings.Contains(k, "audio_b64") {
				out[k] = s
			} else {
				out[k] = redact.Text(s)
			}
			continue
		}
		out[k] = v
	}
	return out
}

var _ metrics.Observer = (*TimelineObserver)(nil)
