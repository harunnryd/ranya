package turn

import "time"

type State int

const (
	StateIdle State = iota
	StateListening
	StateThinking
	StateSpeaking
)

// String returns the string representation of a State
func (s State) String() string {
	switch s {
	case StateIdle:
		return "IDLE"
	case StateListening:
		return "LISTENING"
	case StateThinking:
		return "THINKING"
	case StateSpeaking:
		return "SPEAKING"
	default:
		return "UNKNOWN"
	}
}

type Strategy interface {
	Name() string
	BargeInEnabled() bool
}

type Manager interface {
	OnUserSpeechStart()
	OnUserSpeechEnd()
	OnUserQuestion(text string)
	OnAgentThinkStart()
	OnAgentThinkEnd()
	OnAgentSpeechStart()
	OnAgentSpeechEnd()
	OnAudioComplete()
	OnSTTInput(duration time.Duration)
	AddListener(listener StateListener)
	State() State
	BargeInLatency() time.Duration
}
