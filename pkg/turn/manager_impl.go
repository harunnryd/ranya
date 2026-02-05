package turn

import (
	"sync"
	"time"

	"github.com/harunnryd/ranya/pkg/frames"
)

type ManagerOptions struct {
	BargeInThreshold time.Duration
	MinBargeIn       time.Duration
}

type manager struct {
	mu              sync.RWMutex
	sm              *stateMachine
	strategy        Strategy
	emit            InterruptEmitter
	lastChange      time.Time
	userSpeechStart time.Time
	minBargeIn      time.Duration
	flushTimer      *time.Timer
}

func NewManager(strategy Strategy, emitter InterruptEmitter) Manager {
	return NewManagerWithOptions(strategy, emitter, ManagerOptions{})
}

func NewManagerWithOptions(strategy Strategy, emitter InterruptEmitter, opts ManagerOptions) Manager {
	sm := newStateMachine(opts.BargeInThreshold, emitter)
	minBargeIn := opts.MinBargeIn
	if minBargeIn <= 0 {
		minBargeIn = 300 * time.Millisecond
	}
	return &manager{
		sm:         sm,
		strategy:   strategy,
		emit:       emitter,
		lastChange: time.Now(),
		minBargeIn: minBargeIn,
	}
}

func (m *manager) State() State {
	return m.sm.State()
}

func (m *manager) setState(s State) {
	m.mu.Lock()
	m.lastChange = time.Now()
	m.mu.Unlock()

	// Use state machine for state transitions
	_ = m.sm.Transition(s, "manager state change")
}

func (m *manager) OnUserSpeechStart() {
	wasSpeaking := m.sm.State() == StateSpeaking
	m.setState(StateListening)
	m.mu.Lock()
	m.userSpeechStart = time.Now()
	if m.flushTimer != nil {
		m.flushTimer.Stop()
	}
	if wasSpeaking && m.strategy != nil && m.strategy.BargeInEnabled() {
		start := m.userSpeechStart
		m.flushTimer = time.AfterFunc(m.minBargeIn, func() {
			m.mu.Lock()
			active := m.sm.State() == StateListening && m.userSpeechStart.Equal(start)
			m.mu.Unlock()
			if active {
				m.emitFlush()
			}
		})
	}
	m.mu.Unlock()
}

func (m *manager) OnUserSpeechEnd() {
	m.setState(StateThinking)
	m.mu.Lock()
	if m.flushTimer != nil {
		m.flushTimer.Stop()
	}
	m.mu.Unlock()
}

func (m *manager) OnUserQuestion(text string) {
	// User question triggers transition to thinking state
	// First ensure we're in a valid state to transition to THINKING
	currentState := m.sm.State()
	if currentState == StateIdle {
		// Transition through LISTENING first
		_ = m.sm.Transition(StateListening, "user question - entering listening")
	}
	m.setState(StateThinking)
}

func (m *manager) OnAgentThinkStart() {
	// Agent thinking can happen from LISTENING or IDLE
	currentState := m.sm.State()
	if currentState == StateIdle {
		// Transition through LISTENING first
		_ = m.sm.Transition(StateListening, "agent think start - entering listening")
	}
	m.setState(StateThinking)
}

func (m *manager) OnAgentThinkEnd() {
}

func (m *manager) OnAgentSpeechStart() {
	m.setState(StateSpeaking)
}

func (m *manager) OnAgentSpeechEnd() {
	m.setState(StateIdle)
}

// OnAudioComplete notifies the state machine that playback is complete.
func (m *manager) OnAudioComplete() {
	m.sm.OnAudioComplete()
}

// OnSTTInput forwards STT input duration to the state machine for barge-in detection.
func (m *manager) OnSTTInput(duration time.Duration) {
	m.sm.OnSTTInput(duration)
}

func (m *manager) BargeInLatency() time.Duration {
	return time.Since(m.lastChange)
}

// AddListener registers a listener for state change events.
func (m *manager) AddListener(listener StateListener) {
	m.sm.AddListener(listener)
}

type AggressiveStrategy struct{}

func (AggressiveStrategy) Name() string         { return "aggressive" }
func (AggressiveStrategy) BargeInEnabled() bool { return true }

type PoliteStrategy struct{}

func (PoliteStrategy) Name() string         { return "polite" }
func (PoliteStrategy) BargeInEnabled() bool { return false }

func (m *manager) emitFlush() {
	m.mu.RLock()
	emit := m.emit
	m.mu.RUnlock()
	if emit != nil {
		meta := map[string]string{
			frames.MetaSource: "turn",
			frames.MetaReason: "barge_in",
		}
		_ = emit.Emit(frames.NewControlFrame("", time.Now().UnixNano(), frames.ControlFlush, meta))
		_ = emit.Emit(frames.NewControlFrame("", time.Now().UnixNano(), frames.ControlCancel, meta))
	}
}
