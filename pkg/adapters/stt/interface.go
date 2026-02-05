package stt

import (
	"context"

	"github.com/harunnryd/ranya/pkg/frames"
)

// StreamingSTT defines the contract for any STT vendor implementation.
type StreamingSTT interface {
	// Name returns adapter name for logging/metrics.
	Name() string
	// Start initializes the STT connection.
	Start(ctx context.Context) error
	// Close shuts down the STT connection.
	Close() error
	// SendAudio sends audio frames to the STT service.
	SendAudio(frame frames.AudioFrame) error
	// Results returns a channel of transcription/control frames.
	Results() <-chan frames.Frame
}

// Config contains vendor-agnostic STT configuration.
type Config struct {
	StreamID   string
	CallSID    string
	TraceID    string
	SampleRate int
	Language   string
}
