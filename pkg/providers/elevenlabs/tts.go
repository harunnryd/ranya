package elevenlabs

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/harunnryd/ranya/pkg/adapters/tts"
	"github.com/harunnryd/ranya/pkg/frames"
	"github.com/harunnryd/ranya/pkg/resilience"
)

type Config struct {
	APIKey       string
	VoiceID      string
	ModelID      string
	OutputFormat string
	SampleRate   int
	StreamID     string
	CallSID      string
}

type ElevenLabsTTS struct {
	cfg     Config
	conn    *websocket.Conn
	out     chan frames.Frame
	writeCh chan ttsMessage
	ctx     context.Context
	cancel  context.CancelFunc
	mu      sync.Mutex
}

type ttsMessage struct {
	text  string
	flush bool
}

func New(cfg Config) *ElevenLabsTTS {
	if cfg.SampleRate == 0 {
		cfg.SampleRate = 44100
	}
	return &ElevenLabsTTS{
		cfg:     cfg,
		out:     make(chan frames.Frame, 256),
		writeCh: make(chan ttsMessage, 256),
	}
}

func (s *ElevenLabsTTS) Name() string { return "elevenlabs_tts" }

func (s *ElevenLabsTTS) Start(ctx context.Context) error {
	if s.cfg.APIKey == "" || s.cfg.VoiceID == "" {
		return errors.New("missing elevenlabs config")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	s.ctx, s.cancel = context.WithCancel(ctx)
	u, err := s.buildURL()
	if err != nil {
		return err
	}

	slog.Debug("connecting to ElevenLabs",
		slog.String("stream_id", s.cfg.StreamID),
		slog.String("output_format", s.cfg.OutputFormat))

	dialer := websocket.Dialer{Proxy: http.ProxyFromEnvironment}
	conn, resp, err := dialer.Dial(u, http.Header{
		"xi-api-key": []string{s.cfg.APIKey},
	})
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusTooManyRequests {
			slog.Error("ElevenLabs rate limit exceeded",
				slog.String("stream_id", s.cfg.StreamID),
				slog.String("status", resp.Status))
			return resilience.RateLimitError{Provider: "elevenlabs", Message: resp.Status}
		}
		slog.Error("failed to connect to ElevenLabs",
			slog.String("stream_id", s.cfg.StreamID),
			slog.String("error", err.Error()))
		return err
	}

	s.conn = conn
	slog.Info("connected to ElevenLabs",
		slog.String("stream_id", s.cfg.StreamID),
		slog.String("output_format", s.cfg.OutputFormat))

	_ = s.send(map[string]any{
		"text":                   " ",
		"try_trigger_generation": true,
		"voice_settings": map[string]any{
			"stability":        0.5,
			"similarity_boost": 0.8,
		},
		"generation_config": map[string]any{
			"chunk_length_schedule": []int{120, 160, 250, 290},
		},
	})
	go s.readLoop()
	go s.writeLoop()
	return nil
}

func (s *ElevenLabsTTS) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	slog.Info("tts close called",
		slog.String("stream_id", s.cfg.StreamID))
	if s.cancel != nil {
		s.cancel()
	}
	if s.conn != nil {
		_ = s.conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
		return s.conn.Close()
	}
	return nil
}

func (s *ElevenLabsTTS) SendText(text string) error {
	return s.SendTextWithOptions(text, false)
}

func (s *ElevenLabsTTS) Flush() {
	// Tell ElevenLabs to stop generating.
	_ = s.send(map[string]any{"text": " ", "flush": true})

	// Purge internal output channel to remove buffered audio frames.
	// This prevents "zombie audio" from being played after interruption.
	// Only drain what's currently there (non-blocking).
drainLoop:
	for {
		select {
		case <-s.out:
			// dropped
		default:
			break drainLoop
		}
	}
	slog.Info("tts channel purged",
		slog.String("stream_id", s.cfg.StreamID))
}

func (s *ElevenLabsTTS) Results() <-chan frames.Frame { return s.out }

func (s *ElevenLabsTTS) SendTextWithOptions(text string, flush bool) error {
	if s.conn == nil {
		return errors.New("not connected")
	}
	text = strings.TrimSpace(text)
	if text == "" && !flush {
		return nil
	}
	if text != "" && !strings.HasSuffix(text, " ") {
		text += " "
	}
	select {
	case s.writeCh <- ttsMessage{text: text, flush: flush}:
	default:
	}
	return nil
}

func (s *ElevenLabsTTS) buildURL() (string, error) {
	base := "wss://api.elevenlabs.io/v1/text-to-speech/" + s.cfg.VoiceID + "/stream-input"
	q := url.Values{}
	if s.cfg.ModelID != "" {
		q.Set("model_id", s.cfg.ModelID)
	}
	if s.cfg.OutputFormat != "" {
		q.Set("output_format", s.cfg.OutputFormat)
	}
	q.Set("optimize_streaming_latency", "4")
	return base + "?" + q.Encode(), nil
}

func (s *ElevenLabsTTS) writeLoop() {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return
		case msg := <-s.writeCh:
			payload := map[string]any{"text": msg.text}
			if msg.flush {
				payload["flush"] = true
			}
			_ = s.send(payload)
		case <-ticker.C:
			// Keep-alive: send empty text to prevent 20s timeout
			_ = s.send(map[string]any{"text": " "})
		}
	}
}

func (s *ElevenLabsTTS) readLoop() {
	for {
		select {
		case <-s.ctx.Done():
			slog.Info("tts read loop exit",
				slog.String("stream_id", s.cfg.StreamID),
				slog.String("reason", "context_cancelled"))
			return
		default:
			_, data, err := s.conn.ReadMessage()
			if err != nil {
				slog.Error("tts read loop error",
					slog.String("stream_id", s.cfg.StreamID),
					slog.String("error", err.Error()))
				return
			}
			s.handleMessage(data)
		}
	}
}

func (s *ElevenLabsTTS) handleMessage(data []byte) {
	var msg map[string]any
	if err := json.Unmarshal(data, &msg); err != nil {
		slog.Warn("tts websocket raw data", "data", string(data))
		return
	}
	audio, ok := msg["audio"].(string)
	if !ok {
		if a, ok := msg["audio_base_64"].(string); ok {
			audio = a
		} else if a, ok := msg["audio_base64"].(string); ok {
			audio = a
		} else {
			// Check if it's an alignment or other event, otherwise log error
			if _, isAlign := msg["alignment"]; !isAlign {
				slog.Debug("tts websocket message", "payload", msg)
			}
			return
		}
	}
	raw, err := base64.StdEncoding.DecodeString(audio)
	if err != nil {
		slog.Error("tts audio decode error", "error", err)
		return
	}

	// Log audio chunk received
	slog.Debug("tts audio chunk received",
		slog.String("stream_id", s.cfg.StreamID),
		slog.Int("size_bytes", len(raw)))

	// Create metadata with native format information
	meta := map[string]string{
		frames.MetaStreamID: s.cfg.StreamID,
		frames.MetaCallSID:  s.cfg.CallSID,
		frames.MetaSource:   "elevenlabs",
	}

	// Mark encoding type based on ElevenLabs output format
	// Native format: ulaw_8000 requires no transcoding
	if strings.Contains(s.cfg.OutputFormat, "ulaw") {
		meta[frames.MetaEncoding] = "mulaw"
		meta[frames.MetaCodec] = "ulaw"
		meta["sample_rate"] = "8000"
		meta["channels"] = "1"
	}

	// Create AudioFrame with native format metadata
	f := frames.NewAudioFrame(s.cfg.StreamID, time.Now().UnixNano(), raw, s.cfg.SampleRate, 1, meta)

	select {
	case s.out <- f:
		slog.Debug("tts audio frame emitted",
			slog.String("stream_id", s.cfg.StreamID),
			slog.Int("size_bytes", len(raw)),
			slog.String("codec", meta[frames.MetaCodec]))
	default:
		slog.Warn("tts output buffer full",
			slog.String("stream_id", s.cfg.StreamID))
	}
}

func (s *ElevenLabsTTS) send(payload map[string]any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return s.conn.WriteMessage(websocket.TextMessage, b)
}

var _ tts.StreamingTTS = (*ElevenLabsTTS)(nil)
