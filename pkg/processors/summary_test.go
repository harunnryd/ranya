package processors

import (
	"testing"
	"time"

	"github.com/harunnryd/ranya/pkg/frames"
)

func TestSummaryProcessorEmitsSummaryOnCallEnd(t *testing.T) {
	proc := NewSummaryProcessor(SummaryConfig{MaxEntries: 4, MaxChars: 200})
	streamID := "stream-1"
	meta := map[string]string{frames.MetaStreamID: streamID, frames.MetaSource: "stt", frames.MetaIsFinal: "true"}
	_, _ = proc.Process(frames.NewTextFrame(streamID, time.Now().UnixNano(), "AC saya tidak dingin", meta))
	metaLLM := map[string]string{frames.MetaStreamID: streamID, frames.MetaSource: "llm"}
	_, _ = proc.Process(frames.NewTextFrame(streamID, time.Now().UnixNano(), "Baik, saya bantu cek.", metaLLM))

	out, err := proc.Process(frames.NewSystemFrame(streamID, time.Now().UnixNano(), "call_end", map[string]string{frames.MetaStreamID: streamID}))
	if err != nil {
		t.Fatalf("process error: %v", err)
	}
	if len(out) == 0 {
		t.Fatalf("expected summary frame")
	}
	found := false
	for _, f := range out {
		if f.Kind() == frames.KindSystem {
			sf := f.(frames.SystemFrame)
			if sf.Name() == "call_summary" {
				if sf.Meta()[frames.MetaCallSummary] == "" {
					t.Fatalf("summary meta empty")
				}
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("call_summary not emitted")
	}
}
