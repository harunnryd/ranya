package turn

import (
	"sync"
	"time"
)

// StateChange represents a state transition event.
type StateChange struct {
	FromState State
	ToState   State
	Timestamp time.Time
	Reason    string
}

// StateListener observes turn state changes.
type StateListener interface {
	OnStateChange(event StateChange)
}

// stateMachine implements the finite state machine for turn management.
type stateMachine struct {
	currentState State
	mu           sync.RWMutex

	// Configuration
	bargeInThreshold time.Duration

	// State tracking
	speakingStartTime  time.Time
	listeningStartTime time.Time

	// Event emission
	stateChangeListeners []StateListener

	// Interrupt emitter for sending control frames
	emitter InterruptEmitter
}

// newStateMachine creates a state machine for turn management.
func newStateMachine(bargeInThreshold time.Duration, emitter InterruptEmitter) *stateMachine {
	if bargeInThreshold <= 0 {
		bargeInThreshold = 500 * time.Millisecond
	}
	return &stateMachine{
		currentState:     StateIdle,
		bargeInThreshold: bargeInThreshold,
		emitter:          emitter,
	}
}

// State returns the current state.
func (tm *stateMachine) State() State {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	return tm.currentState
}

// transitionValid checks if a state transition is valid (must be called with lock held).
func (tm *stateMachine) transitionValid(from, to State) bool {
	// Define valid state transitions
	validTransitions := map[State][]State{
		StateIdle:      {StateListening},
		StateListening: {StateThinking, StateIdle},
		StateThinking:  {StateSpeaking, StateListening, StateIdle},
		StateSpeaking:  {StateListening, StateIdle},
	}

	allowedStates, exists := validTransitions[from]
	if !exists {
		return false
	}

	for _, allowed := range allowedStates {
		if allowed == to {
			return true
		}
	}
	return false
}

// Transition moves to a new state with validation.
func (tm *stateMachine) Transition(state State, reason string) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if !tm.transitionValid(tm.currentState, state) {
		return &InvalidTransitionError{
			From: tm.currentState,
			To:   state,
		}
	}

	oldState := tm.currentState
	tm.currentState = state

	// Track state-specific timestamps
	switch state {
	case StateListening:
		tm.listeningStartTime = time.Now()
	case StateSpeaking:
		tm.speakingStartTime = time.Now()
	}

	// Emit state change event to listeners
	event := StateChange{
		FromState: oldState,
		ToState:   state,
		Timestamp: time.Now(),
		Reason:    reason,
	}

	// Notify listeners (release lock during notification to avoid deadlocks)
	listeners := make([]StateListener, len(tm.stateChangeListeners))
	copy(listeners, tm.stateChangeListeners)
	tm.mu.Unlock()

	for _, listener := range listeners {
		listener.OnStateChange(event)
	}

	tm.mu.Lock()
	return nil
}

// AddListener registers a listener for state change events.
func (tm *stateMachine) AddListener(listener StateListener) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.stateChangeListeners = append(tm.stateChangeListeners, listener)
}

// InvalidTransitionError represents an invalid state transition attempt
type InvalidTransitionError struct {
	From State
	To   State
}

func (e *InvalidTransitionError) Error() string {
	return "invalid state transition from " + e.From.String() + " to " + e.To.String()
}

// OnAudioComplete handles audio playback completion.
// Triggers SPEAKING → LISTENING transition.
func (tm *stateMachine) OnAudioComplete() {
	tm.mu.RLock()
	currentState := tm.currentState
	tm.mu.RUnlock()

	if currentState == StateSpeaking {
		_ = tm.Transition(StateListening, "audio playback complete")
	}
}

// OnSTTInput handles STT input and detects barge-in.
// When in SPEAKING state and duration exceeds threshold, sends ControlInterrupt.
func (tm *stateMachine) OnSTTInput(duration time.Duration) {
	tm.mu.RLock()
	currentState := tm.currentState
	threshold := tm.bargeInThreshold
	emitter := tm.emitter
	tm.mu.RUnlock()

	if currentState == StateSpeaking {
		if duration > threshold {
			// Barge-in detected - send control interrupt frame
			if emitter != nil {
				interruptFrame := NewInterruptFrame("", time.Now().UnixNano())
				_ = emitter.Emit(interruptFrame)
			}
			// Transition back to listening
			_ = tm.Transition(StateListening, "barge-in detected")
		}
	}
}
