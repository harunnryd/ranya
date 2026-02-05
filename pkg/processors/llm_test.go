package processors

import (
	"testing"
	"time"

	"github.com/harunnryd/ranya/pkg/frames"
	"github.com/harunnryd/ranya/pkg/llm"
	mockllm "github.com/harunnryd/ranya/pkg/providers/mock"
)

func TestLLMToolConfirmationPrompt(t *testing.T) {
	adapter := mockllm.NewLLMAdapter(mockllm.LLMConfig{
		ToolCalls: []llm.ToolCall{{
			ID:        "tool-1",
			Name:      "schedule_visit",
			Arguments: map[string]any{"location": "Jakarta"},
		}},
	})
	tools := []llm.Tool{{
		Name:                 "schedule_visit",
		RequiresConfirmation: true,
		ConfirmationPrompt:   "Confirm schedule?",
	}}
	proc := NewLLMProcessor(adapter, "", tools)
	meta := map[string]string{
		frames.MetaStreamID: "stream-1",
		frames.MetaSource:   "stt",
		frames.MetaLanguage: "en",
	}
	input := frames.NewTextFrame("stream-1", time.Now().UnixNano(), "Schedule a visit", meta)
	out, err := proc.Process(input)
	if err != nil {
		t.Fatalf("process error: %v", err)
	}
	if len(out) == 0 {
		t.Fatalf("expected output frames")
	}
	var sawPrompt bool
	var sawToolCall bool
	for _, f := range out {
		switch f.Kind() {
		case frames.KindSystem:
			sf := f.(frames.SystemFrame)
			if sf.Meta()[frames.MetaGreetingText] == "Confirm schedule?" {
				sawPrompt = true
			}
		case frames.KindControl:
			cf := f.(frames.ControlFrame)
			if cf.Code() == frames.ControlToolCall {
				sawToolCall = true
			}
		}
	}
	if !sawPrompt {
		t.Fatalf("expected confirmation prompt")
	}
	if sawToolCall {
		t.Fatalf("did not expect tool call before confirmation")
	}
}

func TestLLMToolConfirmationAcceptsYes(t *testing.T) {
	adapter := mockllm.NewLLMAdapter(mockllm.LLMConfig{
		ToolCalls: []llm.ToolCall{{
			ID:        "tool-1",
			Name:      "schedule_visit",
			Arguments: map[string]any{"location": "Jakarta"},
		}},
	})
	tools := []llm.Tool{{
		Name:                 "schedule_visit",
		RequiresConfirmation: true,
		ConfirmationPrompt:   "Confirm schedule?",
	}}
	proc := NewLLMProcessor(adapter, "", tools)
	meta := map[string]string{
		frames.MetaStreamID: "stream-1",
		frames.MetaSource:   "stt",
		frames.MetaLanguage: "en",
	}
	input := frames.NewTextFrame("stream-1", time.Now().UnixNano(), "Schedule a visit", meta)
	_, err := proc.Process(input)
	if err != nil {
		t.Fatalf("process error: %v", err)
	}

	confirm := frames.NewTextFrame("stream-1", time.Now().UnixNano(), "yes", meta)
	out, err := proc.Process(confirm)
	if err != nil {
		t.Fatalf("process confirm error: %v", err)
	}
	var sawToolCall bool
	for _, f := range out {
		if f.Kind() == frames.KindControl {
			cf := f.(frames.ControlFrame)
			if cf.Code() == frames.ControlToolCall {
				sawToolCall = true
			}
		}
	}
	if !sawToolCall {
		t.Fatalf("expected tool call after confirmation")
	}
}
