package observers

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/harunnryd/ranya/pkg/metrics"
)

func TestTimelineObserverWritesJSONL(t *testing.T) {
	dir := t.TempDir()
	obs := NewTimelineObserver(dir)

	ev := metrics.MetricsEvent{
		Name: "frame_out",
		Time: time.Now(),
		Tags: map[string]string{
			"stream_id": "stream-1",
			"trace_id":  "trace-1",
			"kind":      "audio",
		},
	}
	obs.RecordEvent(ev)
	_ = obs.Close()

	path := filepath.Join(dir, "trace-1.jsonl")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if !strings.Contains(string(b), "audio_out") {
		t.Fatalf("expected audio_out event in file")
	}
}
