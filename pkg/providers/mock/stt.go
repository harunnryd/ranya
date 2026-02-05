package mock

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/harunnryd/ranya/pkg/adapters/stt"
	"github.com/harunnryd/ranya/pkg/frames"
)

type STTConfig struct {
	StreamID          string
	CallSID           string
	TraceID           string
	Transcript        string
	InterimTranscript string
	EmitInterim       bool
	EmitVAD           bool
	EmitUtteranceEnd  bool
}

type StreamingSTT struct {
	cfg     STTConfig
	out     chan frames.Frame
	ctx     context.Context
	cancel  context.CancelFunc
	mu      sync.Mutex
	started bool
	emitted bool
}

func NewSTT(cfg STTConfig) *StreamingSTT {
	if cfg.Transcript == "" {
		cfg.Transcript = "mock transcript"
	}
	return &StreamingSTT{cfg: cfg, out: make(chan frames.Frame, 16)}
}

func (s *StreamingSTT) Name() string { return "mock_stt" }

func (s *StreamingSTT) Start(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	s.ctx, s.cancel = context.WithCancel(ctx)
	s.mu.Lock()
	s.started = true
	s.mu.Unlock()
	return nil
}

func (s *StreamingSTT) Close() error {
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

func (s *StreamingSTT) SendAudio(frame frames.AudioFrame) error {
	s.mu.Lock()
	if !s.started {
		s.mu.Unlock()
		return errors.New("not started")
	}
	if s.emitted {
		s.mu.Unlock()
		return nil
	}
	s.emitted = true
	s.mu.Unlock()

	traceID := s.cfg.TraceID
	if s.cfg.EmitVAD {
		startMeta := map[string]string{
			frames.MetaStreamID: s.cfg.StreamID,
			frames.MetaCallSID:  s.cfg.CallSID,
			frames.MetaSource:   "stt",
			frames.MetaReason:   "speech_started",
		}
		if traceID != "" {
			startMeta[frames.MetaTraceID] = traceID
		}
		s.out <- frames.NewControlFrame(s.cfg.StreamID, time.Now().UnixNano(), frames.ControlFlush, startMeta)
	}

	if s.cfg.EmitInterim {
		interim := s.cfg.InterimTranscript
		if interim == "" {
			interim = s.cfg.Transcript
		}
		meta := map[string]string{
			frames.MetaStreamID: s.cfg.StreamID,
			frames.MetaCallSID:  s.cfg.CallSID,
			frames.MetaSource:   "stt",
			frames.MetaIsFinal:  "false",
		}
		if traceID != "" {
			meta[frames.MetaTraceID] = traceID
		}
		s.out <- frames.NewTextFrame(s.cfg.StreamID, time.Now().UnixNano(), interim, meta)
	}

	finalMeta := map[string]string{
		frames.MetaStreamID: s.cfg.StreamID,
		frames.MetaCallSID:  s.cfg.CallSID,
		frames.MetaSource:   "stt",
		frames.MetaIsFinal:  "true",
	}
	if traceID != "" {
		finalMeta[frames.MetaTraceID] = traceID
	}
	s.out <- frames.NewTextFrame(s.cfg.StreamID, time.Now().UnixNano(), s.cfg.Transcript, finalMeta)

	flushMeta := map[string]string{
		frames.MetaStreamID: s.cfg.StreamID,
		frames.MetaCallSID:  s.cfg.CallSID,
		frames.MetaSource:   "stt",
		frames.MetaReason:   "speech_final",
	}
	if traceID != "" {
		flushMeta[frames.MetaTraceID] = traceID
	}
	s.out <- frames.NewControlFrame(s.cfg.StreamID, time.Now().UnixNano(), frames.ControlFlush, flushMeta)

	if s.cfg.EmitUtteranceEnd {
		endMeta := map[string]string{
			frames.MetaStreamID: s.cfg.StreamID,
			frames.MetaCallSID:  s.cfg.CallSID,
			frames.MetaSource:   "stt",
			frames.MetaReason:   "utterance_end",
		}
		if traceID != "" {
			endMeta[frames.MetaTraceID] = traceID
		}
		s.out <- frames.NewControlFrame(s.cfg.StreamID, time.Now().UnixNano(), frames.ControlFlush, endMeta)
	}
	return nil
}

func (s *StreamingSTT) Results() <-chan frames.Frame { return s.out }

var _ stt.StreamingSTT = (*StreamingSTT)(nil)
