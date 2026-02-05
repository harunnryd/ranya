package processors

import (
	"testing"
	"time"

	"github.com/harunnryd/ranya/pkg/frames"
	"github.com/harunnryd/ranya/pkg/llm"
	mockllm "github.com/harunnryd/ranya/pkg/providers/mock"
)

func TestLLMMemoryPruneByHistory(t *testing.T) {
	adapter := mockllm.NewLLMAdapter(mockllm.LLMConfig{ResponseText: "ok"})
	proc := NewLLMProcessor(adapter, "", []llm.Tool{})
	proc.SetMemoryLimits(3, 0)
	meta := map[string]string{frames.MetaStreamID: "stream-1", frames.MetaSource: "stt"}

	for i := 0; i < 2; i++ {
		input := frames.NewTextFrame("stream-1", time.Now().UnixNano(), "message", meta)
		if _, err := proc.Process(input); err != nil {
			t.Fatalf("process error: %v", err)
		}
	}
	scope := proc.scopeKey(meta, "stream-1")
	snap := proc.contextSnapshot(scope)
	count := 0
	for _, msg := range snap.Messages {
		if role, _ := msg["role"].(string); role != "system" {
			count++
		}
	}
	if count > 3 {
		t.Fatalf("expected pruned messages <= 3, got %d", count)
	}
}
