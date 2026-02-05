package turn

import (
	"sync"
	"testing"
	"time"

	"github.com/harunnryd/ranya/pkg/frames"
)

type captureEmitter struct {
	mu     sync.Mutex
	frames []frames.Frame
}

func (c *captureEmitter) Emit(frame frames.Frame) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.frames = append(c.frames, frame)
	return nil
}

func (c *captureEmitter) Count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.frames)
}

func TestStateMachineBargeInThreshold(t *testing.T) {
	emitter := &captureEmitter{}
	sm := newStateMachine(50*time.Millisecond, emitter)

	if err := sm.Transition(StateListening, "test listening"); err != nil {
		t.Fatalf("transition error: %v", err)
	}
	if err := sm.Transition(StateThinking, "test thinking"); err != nil {
		t.Fatalf("transition error: %v", err)
	}
	if err := sm.Transition(StateSpeaking, "test speaking"); err != nil {
		t.Fatalf("transition error: %v", err)
	}

	sm.OnSTTInput(20 * time.Millisecond)
	if emitter.Count() != 0 {
		t.Fatalf("expected no interruption below threshold")
	}

	sm.OnSTTInput(80 * time.Millisecond)
	if emitter.Count() != 1 {
		t.Fatalf("expected interruption emitted above threshold")
	}
	if sm.State() != StateListening {
		t.Fatalf("expected state LISTENING after barge-in, got %s", sm.State().String())
	}
}
