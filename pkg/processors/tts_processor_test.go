package processors

import (
	"context"
	"testing"
	"time"

	"github.com/harunnryd/ranya/pkg/adapters/tts"
	"github.com/harunnryd/ranya/pkg/frames"
)

type mockTTS struct {
	flushCount int
	startCount int
	texts      []string
	out        chan frames.Frame
}

func (m *mockTTS) Name() string { return "mock_tts" }

func (m *mockTTS) Start(ctx context.Context) error {
	m.startCount++
	return nil
}

func (m *mockTTS) Close() error { return nil }

func (m *mockTTS) SendText(text string) error {
	m.texts = append(m.texts, text)
	return nil
}

func (m *mockTTS) Flush() {
	m.flushCount++
}

func (m *mockTTS) Results() <-chan frames.Frame { return m.out }

func TestTTSProcessorInterruptionFlush(t *testing.T) {
	mock := &mockTTS{out: make(chan frames.Frame, 1)}
	factory := func(callSID, streamID string) tts.StreamingTTS { return mock }
	proc := NewTTSProcessor(factory)

	meta := map[string]string{frames.MetaStreamID: "stream-1", frames.MetaSource: "llm"}
	text := frames.NewTextFrame("stream-1", time.Now().UnixNano(), "Halo", meta)
	if _, err := proc.Process(text); err != nil {
		t.Fatalf("process text: %v", err)
	}

	ctrl := frames.NewControlFrame("stream-1", time.Now().UnixNano(), frames.ControlStartInterruption, map[string]string{frames.MetaStreamID: "stream-1"})
	if _, err := proc.Process(ctrl); err != nil {
		t.Fatalf("process interruption: %v", err)
	}
	if mock.flushCount == 0 {
		t.Fatalf("expected flush to be called on interruption")
	}
}
