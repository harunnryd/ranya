package deepgram

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/harunnryd/ranya/pkg/adapters/stt"
	"github.com/harunnryd/ranya/pkg/frames"
	"github.com/harunnryd/ranya/pkg/logging"

	msginterfaces "github.com/deepgram/deepgram-go-sdk/v3/pkg/api/listen/v1/websocket/interfaces"
	interfaces "github.com/deepgram/deepgram-go-sdk/v3/pkg/client/interfaces"
	client "github.com/deepgram/deepgram-go-sdk/v3/pkg/client/listen"
)

// RetryPolicy defines retry behavior for transient failures
type RetryPolicy struct {
	MaxRetries int
	Backoff    time.Duration
}

func newRetryPolicy(maxRetries int, backoff time.Duration) RetryPolicy {
	if maxRetries <= 0 {
		maxRetries = 2
	}
	if backoff <= 0 {
		backoff = 200 * time.Millisecond
	}
	return RetryPolicy{MaxRetries: maxRetries, Backoff: backoff}
}

type DeepgramParams struct {
	EchoCancellation bool
	UtteranceEndMS   int
}

type Config struct {
	APIKey     string
	Model      string
	Language   string
	SampleRate int
	Encoding   string
	Interim    bool
	VADEvents  bool
	StreamID   string
	CallSID    string
	TraceID    string
	Params     DeepgramParams
}

type StreamingSTT struct {
	cfg Config
	// Correcting the type guess. If WSCallback is wrong, likely it is Client.
	dgClient   *client.WSCallback
	out        chan frames.Frame
	ctx        context.Context
	cancel     context.CancelFunc
	pipeReader *io.PipeReader
	pipeWriter *io.PipeWriter
	metaLogged bool
	logger     *slog.Logger

	// Native parameters
	vadEvents        bool
	echoCancellation bool
	utteranceEndMs   int

	// Retry policy for transient failures
	retryPolicy RetryPolicy
}

func New(cfg Config) *StreamingSTT {
	if cfg.SampleRate == 0 {
		cfg.SampleRate = 16000
	}

	// Initialize logger with component context
	baseLogger := slog.Default()
	logger := logging.NewComponentLogger(baseLogger, "deepgram_stt")

	return &StreamingSTT{
		cfg:              cfg,
		out:              make(chan frames.Frame, 256),
		logger:           logger,
		vadEvents:        cfg.VADEvents,
		echoCancellation: cfg.Params.EchoCancellation,
		utteranceEndMs:   cfg.Params.UtteranceEndMS,
		retryPolicy:      newRetryPolicy(3, 200*time.Millisecond),
	}
}

func (s *StreamingSTT) Name() string { return "deepgram_streaming" }

func (s *StreamingSTT) Start(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	s.ctx, s.cancel = context.WithCancel(ctx)

	// Create a pipe for streaming audio
	s.pipeReader, s.pipeWriter = io.Pipe()

	// Initialize Deepgram Client
	clientOptions := &interfaces.ClientOptions{
		EnableKeepAlive: true,
	}

	transcriptOptions := &interfaces.LiveTranscriptionOptions{
		Model:          s.cfg.Model,
		Language:       s.cfg.Language,
		Encoding:       s.cfg.Encoding,
		SampleRate:     s.cfg.SampleRate,
		InterimResults: s.cfg.Interim,
		VadEvents:      s.vadEvents,
		SmartFormat:    true,
	}

	// Apply native parameters
	if s.utteranceEndMs > 0 {
		transcriptOptions.UtteranceEndMs = fmt.Sprintf("%d", s.utteranceEndMs)
		s.logger.Info("configured native utterance detection",
			slog.Int("utterance_end_ms", s.utteranceEndMs),
			slog.String("stream_id", s.cfg.StreamID))
	}

	// Note: Echo cancellation is not directly supported in the Deepgram SDK v3.5.0
	// It may need to be handled through the Extra field or is not available in this version
	if s.echoCancellation {
		s.logger.Debug("echo_cancellation_requested",
			slog.String("stream_id", s.cfg.StreamID),
			slog.String("note", "not yet supported in SDK"))
	}

	s.logger.Info("initializing deepgram connection",
		slog.String("stream_id", s.cfg.StreamID),
		slog.String("call_sid", s.cfg.CallSID),
		slog.String("model", s.cfg.Model),
		slog.Bool("vad_events", s.vadEvents),
		slog.Int("sample_rate", s.cfg.SampleRate))

	cb := &callback{parent: s}

	// NewWSUsingCallback(ctx context.Context, apiKey string, cOptions *interfaces.ClientOptions, tOptions *interfaces.LiveTranscriptionOptions, callback msginterfaces.LiveMessageCallback)
	dgClient, err := client.NewWSUsingCallback(s.ctx, s.cfg.APIKey, clientOptions, transcriptOptions, cb)
	if err != nil {
		s.logger.Error("deepgram_client_create_error",
			slog.String("error", err.Error()),
			slog.String("stream_id", s.cfg.StreamID))
		return err
	}
	s.dgClient = dgClient

	if connected := s.dgClient.Connect(); !connected {
		s.logger.Error("deepgram_connect_failed",
			slog.String("stream_id", s.cfg.StreamID))
		return fmt.Errorf("deepgram connection failed")
	}

	s.logger.Info("deepgram_connected",
		slog.String("stream_id", s.cfg.StreamID),
		slog.String("call_sid", s.cfg.CallSID),
		slog.String("model", s.cfg.Model))

	// Start streaming
	go func() {
		if err := s.dgClient.Stream(s.pipeReader); err != nil && s.ctx.Err() == nil {
			s.logger.Error("deepgram_stream_error",
				slog.String("error", err.Error()),
				slog.String("stream_id", s.cfg.StreamID))
		}
	}()

	return nil
}

func (s *StreamingSTT) Close() error {
	s.logger.Info("closing deepgram connection",
		slog.String("stream_id", s.cfg.StreamID))

	if s.cancel != nil {
		s.cancel()
	}
	if s.pipeWriter != nil {
		_ = s.pipeWriter.Close()
	}
	if s.dgClient != nil {
		s.dgClient.Stop()
	}
	return nil
}

func (s *StreamingSTT) SendAudio(frame frames.AudioFrame) error {
	if s.pipeWriter == nil {
		return fmt.Errorf("not started")
	}

	s.logger.Debug("forwarding audio to deepgram",
		slog.Int("size_bytes", len(frame.RawPayload())),
		slog.String("stream_id", s.cfg.StreamID))

	_, err := s.pipeWriter.Write(frame.RawPayload())
	if err != nil {
		s.logger.Error("failed to send audio to deepgram",
			slog.String("error", err.Error()),
			slog.String("stream_id", s.cfg.StreamID))
	}
	return err
}

func (s *StreamingSTT) Results() <-chan frames.Frame { return s.out }

// --- Callback Implementation ---

type callback struct {
	parent *StreamingSTT
}

func (c *callback) Open(or *msginterfaces.OpenResponse) error {
	c.parent.logger.Info("deepgram_connection_opened",
		slog.String("stream_id", c.parent.cfg.StreamID))
	return nil
}

func (c *callback) Message(mr *msginterfaces.MessageResponse) error {
	// Handle transcript
	if len(mr.Channel.Alternatives) == 0 {
		return nil
	}
	alt := mr.Channel.Alternatives[0]
	transcript := alt.Transcript
	if transcript == "" {
		return nil
	}

	isFinal := mr.IsFinal || mr.SpeechFinal

	meta := map[string]string{
		frames.MetaStreamID: c.parent.cfg.StreamID,
		frames.MetaCallSID:  c.parent.cfg.CallSID,
		frames.MetaSource:   "stt",
	}
	if c.parent.cfg.TraceID != "" {
		meta[frames.MetaTraceID] = c.parent.cfg.TraceID
	}
	if isFinal {
		meta[frames.MetaIsFinal] = "true"
	} else {
		meta[frames.MetaIsFinal] = "false"
	}

	c.parent.logger.Debug("transcript_received",
		slog.String("stream_id", c.parent.cfg.StreamID),
		slog.String("transcript", transcript),
		slog.Bool("is_final", isFinal))

	f := frames.NewTextFrame(c.parent.cfg.StreamID, time.Now().UnixNano(), transcript, meta)

	select {
	case c.parent.out <- f:
	default:
		c.parent.logger.Warn("deepgram_out_channel_full",
			slog.String("stream_id", c.parent.cfg.StreamID))
	}

	if isFinal {
		// Emit control flush for speech final
		flushMeta := map[string]string{
			frames.MetaStreamID: c.parent.cfg.StreamID,
			frames.MetaCallSID:  c.parent.cfg.CallSID,
			frames.MetaSource:   "stt",
			frames.MetaReason:   "speech_final",
		}
		if c.parent.cfg.TraceID != "" {
			flushMeta[frames.MetaTraceID] = c.parent.cfg.TraceID
		}

		c.parent.logger.Info("emitting_speech_final_flush",
			slog.String("stream_id", c.parent.cfg.StreamID))

		ff := frames.NewControlFrame(c.parent.cfg.StreamID, time.Now().UnixNano(), frames.ControlFlush, flushMeta)
		select {
		case c.parent.out <- ff:
		default:
		}
	}
	return nil
}

func (c *callback) Metadata(md *msginterfaces.MetadataResponse) error {
	if !c.parent.metaLogged {
		c.parent.metaLogged = true
		c.parent.logger.Info("deepgram_metadata_received",
			slog.String("stream_id", c.parent.cfg.StreamID),
			slog.String("request_id", md.RequestID))
	}
	return nil
}

func (c *callback) SpeechStarted(ssr *msginterfaces.SpeechStartedResponse) error {
	// Emit control flush immediately on speech start (native VAD event)
	meta := map[string]string{
		frames.MetaStreamID: c.parent.cfg.StreamID,
		frames.MetaCallSID:  c.parent.cfg.CallSID,
		frames.MetaSource:   "stt",
		frames.MetaReason:   "speech_started",
	}
	if c.parent.cfg.TraceID != "" {
		meta[frames.MetaTraceID] = c.parent.cfg.TraceID
	}

	c.parent.logger.Info("speech_started_event",
		slog.String("stream_id", c.parent.cfg.StreamID),
		slog.String("reason", "native_vad_detection"))

	f := frames.NewControlFrame(c.parent.cfg.StreamID, time.Now().UnixNano(), frames.ControlFlush, meta)

	select {
	case c.parent.out <- f:
		c.parent.logger.Debug("speech_started_flush_emitted",
			slog.String("stream_id", c.parent.cfg.StreamID))
	default:
		c.parent.logger.Warn("failed_to_emit_speech_started_flush",
			slog.String("stream_id", c.parent.cfg.StreamID),
			slog.String("reason", "channel_full"))
	}
	return nil
}

func (c *callback) UtteranceEnd(ur *msginterfaces.UtteranceEndResponse) error {
	// Emit control flush for utterance end (native VAD event)
	meta := map[string]string{
		frames.MetaStreamID: c.parent.cfg.StreamID,
		frames.MetaCallSID:  c.parent.cfg.CallSID,
		frames.MetaSource:   "stt",
		frames.MetaReason:   "utterance_end",
	}
	if c.parent.cfg.TraceID != "" {
		meta[frames.MetaTraceID] = c.parent.cfg.TraceID
	}

	c.parent.logger.Info("utterance_end_event",
		slog.String("stream_id", c.parent.cfg.StreamID),
		slog.Int("utterance_end_ms", c.parent.utteranceEndMs))

	f := frames.NewControlFrame(c.parent.cfg.StreamID, time.Now().UnixNano(), frames.ControlFlush, meta)

	select {
	case c.parent.out <- f:
		c.parent.logger.Debug("utterance_end_flush_emitted",
			slog.String("stream_id", c.parent.cfg.StreamID))
	default:
		c.parent.logger.Warn("failed_to_emit_utterance_end_flush",
			slog.String("stream_id", c.parent.cfg.StreamID),
			slog.String("reason", "channel_full"))
	}
	return nil
}

func (c *callback) Close(cr *msginterfaces.CloseResponse) error {
	c.parent.logger.Info("deepgram_connection_closed",
		slog.String("stream_id", c.parent.cfg.StreamID))
	return nil
}

func (c *callback) Error(er *msginterfaces.ErrorResponse) error {
	c.parent.logger.Error("deepgram_error",
		slog.String("stream_id", c.parent.cfg.StreamID),
		slog.String("error_code", er.ErrCode),
		slog.String("error_message", er.ErrMsg))
	return nil
}

func (c *callback) UnhandledEvent(byData []byte) error {
	c.parent.logger.Debug("deepgram_unhandled_event",
		slog.String("stream_id", c.parent.cfg.StreamID),
		slog.String("data", string(byData)))
	return nil
}

var _ stt.StreamingSTT = (*StreamingSTT)(nil)
