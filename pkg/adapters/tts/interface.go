package tts

import (
	"context"

	"github.com/harunnryd/ranya/pkg/frames"
)

// StreamingTTS defines the contract for any TTS vendor implementation.
type StreamingTTS interface {
	// Name returns adapter name for logging/metrics.
	Name() string
	// Start initializes the TTS connection.
	Start(ctx context.Context) error
	// Close shuts down the TTS connection.
	Close() error
	// SendText sends text to be synthesized.
	SendText(text string) error
	// Flush stops current synthesis and clears buffers.
	Flush()
	// Results returns a channel of audio/control frames.
	Results() <-chan frames.Frame
}

// Config contains vendor-agnostic TTS configuration.
type Config struct {
	StreamID   string
	CallSID    string
	SampleRate int
	Channels   int
}
