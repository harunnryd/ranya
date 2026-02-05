package mock

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/harunnryd/ranya/pkg/adapters/tts"
	"github.com/harunnryd/ranya/pkg/frames"
)

type TTSConfig struct {
	StreamID       string
	CallSID        string
	SampleRate     int
	Channels       int
	EmitAudioReady bool
}

type StreamingTTS struct {
	cfg     TTSConfig
	out     chan frames.Frame
	ctx     context.Context
	cancel  context.CancelFunc
	mu      sync.Mutex
	started bool
}

func NewTTS(cfg TTSConfig) *StreamingTTS {
	if cfg.SampleRate == 0 {
		cfg.SampleRate = 16000
	}
	if cfg.Channels == 0 {
		cfg.Channels = 1
	}
	return &StreamingTTS{
		cfg: cfg,
		out: make(chan frames.Frame, 16),
	}
}

func (s *StreamingTTS) Name() string { return "mock_tts" }

func (s *StreamingTTS) Start(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	s.ctx, s.cancel = context.WithCancel(ctx)
	s.mu.Lock()
	s.started = true
	s.mu.Unlock()
	return nil
}

func (s *StreamingTTS) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil {
		s.cancel()
	}
	if s.out != nil {
		close(s.out)
		s.out = nil
	}
	s.started = false
	return nil
}

func (s *StreamingTTS) SendText(text string) error {
	s.mu.Lock()
	started := s.started
	s.mu.Unlock()
	if !started {
		return errors.New("not started")
	}

	// Emit a deterministic silent audio frame.
	pcm := make([]byte, 320)
	meta := map[string]string{
		frames.MetaStreamID: s.cfg.StreamID,
		frames.MetaCallSID:  s.cfg.CallSID,
		frames.MetaSource:   "tts",
	}
	f := frames.NewAudioFrame(s.cfg.StreamID, time.Now().UnixNano(), pcm, s.cfg.SampleRate, s.cfg.Channels, meta)
	s.out <- f
	if s.cfg.EmitAudioReady {
		ready := frames.NewControlFrame(s.cfg.StreamID, time.Now().UnixNano(), frames.ControlAudioReady, map[string]string{
			frames.MetaSource: "tts",
		})
		s.out <- ready
	}
	return nil
}

func (s *StreamingTTS) Flush() {}

func (s *StreamingTTS) Results() <-chan frames.Frame { return s.out }

var _ tts.StreamingTTS = (*StreamingTTS)(nil)
