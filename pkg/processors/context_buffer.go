package processors

import (
	"log/slog"
	"strings"
	"sync"

	"github.com/harunnryd/ranya/pkg/turn"
)

// ContextBuffer implements speculative buffering for context assembly.
type ContextBuffer struct {
	// Buffer state
	buffer      strings.Builder
	lastInterim string
	mu          sync.Mutex

	// Configuration
	maxBufferSize int
	streamID      string

	// Integration with downstream processors
	flushHandler func(content string) error
}

// ContextBufferOptions holds configuration for the context buffer.
type ContextBufferOptions struct {
	MaxBufferSize int
	StreamID      string
}

// NewContextBuffer creates a new context buffer.
func NewContextBuffer(config ContextBufferOptions, flushHandler func(string) error) *ContextBuffer {
	if config.MaxBufferSize <= 0 {
		config.MaxBufferSize = 10000 // Default: 10KB
	}

	return &ContextBuffer{
		maxBufferSize: config.MaxBufferSize,
		streamID:      config.StreamID,
		flushHandler:  flushHandler,
	}
}

// SetStreamID sets the stream ID for logging and emitted frames.
// It is safe to call multiple times; the latest non-empty value wins.
func (cp *ContextBuffer) SetStreamID(id string) {
	if id == "" {
		return
	}
	cp.mu.Lock()
	cp.streamID = id
	cp.mu.Unlock()
}

// StreamID returns the current stream ID for this buffer.
func (cp *ContextBuffer) StreamID() string {
	cp.mu.Lock()
	defer cp.mu.Unlock()
	return cp.streamID
}

// AddTranscript updates the buffer with transcript data.
// isFinal indicates whether this is a final transcript or interim result.
func (cp *ContextBuffer) AddTranscript(text string, isFinal bool) {
	cp.mu.Lock()
	defer cp.mu.Unlock()

	if isFinal {
		// Append final transcript to buffer
		cp.buffer.WriteString(text)
		cp.buffer.WriteString(" ")
		cp.lastInterim = ""

		// Check for buffer overflow
		if cp.buffer.Len() > cp.maxBufferSize {
			slog.Warn("context_buffer_overflow",
				"stream_id", cp.streamID,
				"buffer_size", cp.buffer.Len(),
				"max_size", cp.maxBufferSize)
			// Flush immediately on overflow
			cp.flushLocked()
		}
	} else {
		// Store interim result separately (don't commit to buffer yet)
		cp.lastInterim = text
	}
}

// OnStateChange implements the StateListener interface.
// Triggers buffer flush on LISTENING → THINKING transition.
func (cp *ContextBuffer) OnStateChange(event turn.StateChange) {
	// Flush buffer when transitioning from LISTENING to THINKING
	if event.FromState == turn.StateListening && event.ToState == turn.StateThinking {
		cp.Flush()
	}
}

// Flush flushes the buffered content (thread-safe).
func (cp *ContextBuffer) Flush() {
	cp.mu.Lock()
	defer cp.mu.Unlock()
	cp.flushLocked()
}

// flushLocked flushes the buffer (must be called with lock held).
func (cp *ContextBuffer) flushLocked() {
	content := cp.buffer.String()
	if content == "" && cp.lastInterim != "" {
		content = cp.lastInterim
		cp.lastInterim = ""
	}
	if len(content) == 0 {
		return
	}

	// Send content to downstream processor (LLM)
	if cp.flushHandler != nil {
		err := cp.flushHandler(content)
		if err != nil {
			slog.Error("context_buffer_flush_failed",
				"stream_id", cp.streamID,
				"error", err)
			// Keep buffer content for retry
			return
		}
	}

	// Clear buffer after successful flush
	cp.buffer.Reset()
	slog.Debug("context_buffer_flushed",
		"stream_id", cp.streamID,
		"content_length", len(content))
}

// Reset clears the buffer and interim state
func (cp *ContextBuffer) Reset() {
	cp.mu.Lock()
	defer cp.mu.Unlock()
	cp.buffer.Reset()
	cp.lastInterim = ""
}

// Verify interface implementation
var _ turn.StateListener = (*ContextBuffer)(nil)
