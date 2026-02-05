package twilio

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/harunnryd/ranya/pkg/errorsx"
	"github.com/harunnryd/ranya/pkg/frames"
	"github.com/harunnryd/ranya/pkg/transports"
	"github.com/twilio/twilio-go"
	twilioclient "github.com/twilio/twilio-go/client"
	api "github.com/twilio/twilio-go/rest/api/v2010"
)

type Config struct {
	ServerAddr         string   `mapstructure:"server_addr"`
	PublicURL          string   `mapstructure:"public_url"`
	AuthToken          string   `mapstructure:"auth_token"`
	AccountSID         string   `mapstructure:"account_sid"`
	VoicePath          string   `mapstructure:"voice_path"`
	WebsocketPath      string   `mapstructure:"ws_path"`
	TTSWebhookPath     string   `mapstructure:"tts_webhook_path"`
	StatusCallbackPath string   `mapstructure:"status_callback_path"`
	VoiceGreeting      string   `mapstructure:"voice_greeting"`
	AllowAnyOrigin     bool     `mapstructure:"allow_any_origin"`
	AllowedOrigins     []string `mapstructure:"allowed_origins"`
}

func (c Config) withDefaults() Config {
	if c.ServerAddr == "" {
		c.ServerAddr = ":8080"
	}
	if c.VoicePath == "" {
		c.VoicePath = "/voice"
	}
	if c.WebsocketPath == "" {
		c.WebsocketPath = "/ws"
	}
	if c.TTSWebhookPath == "" {
		c.TTSWebhookPath = "/tts/webhook"
	}
	if c.StatusCallbackPath == "" {
		c.StatusCallbackPath = "/status"
	}
	if !c.AllowAnyOrigin && len(c.AllowedOrigins) == 0 {
		c.AllowAnyOrigin = true
	}
	return c
}

type Transport struct {
	cfg      Config
	server   *http.Server
	upgrader websocket.Upgrader
	recvCh   chan frames.Frame

	updateClient callUpdater

	mu          sync.Mutex
	sessions    map[string]*session
	callSIDs    map[string]string
	callStreams map[string]string
	traceIDs    map[string]string
	fromNumbers map[string]string

	draining atomic.Bool
}

type callUpdater interface {
	UpdateCall(sid string, params *api.UpdateCallParams) (*api.ApiV2010Call, error)
}

func New(cfg Config) *Transport {
	cfg = cfg.withDefaults()
	t := &Transport{
		cfg: cfg,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  4096,
			WriteBufferSize: 4096,
			CheckOrigin:     func(r *http.Request) bool { return true },
		},
		recvCh:      make(chan frames.Frame, 512),
		sessions:    make(map[string]*session),
		callSIDs:    make(map[string]string),
		callStreams: make(map[string]string),
		traceIDs:    make(map[string]string),
		fromNumbers: make(map[string]string),
	}
	t.upgrader.CheckOrigin = t.checkOrigin
	return t
}

func (t *Transport) Name() string { return "twilio" }

func (t *Transport) Recv() <-chan frames.Frame { return t.recvCh }

func (t *Transport) ReadyFields() map[string]any {
	return map[string]any{
		"webhook_url":         t.voiceWebhookURL(),
		"status_callback_url": t.statusCallbackURL(),
	}
}

func (t *Transport) Start(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	mux := http.NewServeMux()
	mux.HandleFunc(t.cfg.VoicePath, t.handleVoice)
	mux.Handle(t.cfg.WebsocketPath, t)
	mux.HandleFunc(t.cfg.TTSWebhookPath, t.handleTTSWebhook)
	mux.HandleFunc(t.cfg.StatusCallbackPath, t.handleStatusCallback)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	t.server = &http.Server{
		Addr:              t.cfg.ServerAddr,
		ReadHeaderTimeout: 5 * time.Second,
		Handler:           mux,
	}
	go func() {
		<-ctx.Done()
		_ = t.server.Close()
	}()
	go func() {
		if err := t.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("twilio_transport_server_error", "error", err.Error())
		}
	}()
	return nil
}

func (t *Transport) Stop() error {
	t.draining.Store(true)
	if t.server != nil {
		_ = t.server.Close()
	}
	t.mu.Lock()
	for _, sess := range t.sessions {
		_ = sess.close()
	}
	t.sessions = make(map[string]*session)
	t.mu.Unlock()
	close(t.recvCh)
	return nil
}

func (t *Transport) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if t.draining.Load() {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	conn, err := t.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	var callSID string
	var streamID string
	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			break
		}
		var evt TwilioEvent
		if err := json.Unmarshal(msg, &evt); err != nil {
			continue
		}
		switch evt.Event {
		case "start":
			if evt.Start == nil {
				continue
			}
			callSID = evt.Start.CallSID
			streamID = evt.Start.StreamID
			traceID := uuid.NewString()
			oldStream, oldSess := t.attach(streamID, callSID, traceID, evt.Start.From, conn)
			if oldSess != nil {
				_ = oldSess.close()
			}
			meta := map[string]string{
				frames.MetaStreamID:   streamID,
				frames.MetaCallSID:    callSID,
				frames.MetaTraceID:    traceID,
				frames.MetaFromNumber: evt.Start.From,
				frames.MetaSource:     "transport",
			}
			nonBlockingSend(t.recvCh, frames.NewSystemFrame(streamID, time.Now().UnixNano(), "call_start", meta))
			if oldStream != "" {
				reconnectMeta := map[string]string{
					frames.MetaStreamID:    streamID,
					frames.MetaCallSID:     callSID,
					frames.MetaTraceID:     traceID,
					frames.MetaOldStreamID: oldStream,
					frames.MetaSource:      "transport",
				}
				nonBlockingSend(t.recvCh, frames.NewSystemFrame(streamID, time.Now().UnixNano(), "call_reconnect", reconnectMeta))
			}
		case "media":
			if evt.Media == nil {
				continue
			}
			payload, err := base64.StdEncoding.DecodeString(evt.Media.Payload)
			if err != nil {
				continue
			}
			meta := t.metaForStream(streamID)
			meta[frames.MetaEncoding] = "mulaw"
			meta[frames.MetaCodec] = "ulaw"
			meta[frames.MetaFormat] = "ulaw_8000_1ch_8bit"
			af := frames.NewAudioFrame(streamID, time.Now().UnixNano(), payload, 8000, 1, meta)
			nonBlockingSend(t.recvCh, af)
		case "dtmf":
			if evt.DTMF == nil {
				continue
			}
			meta := t.metaForStream(streamID)
			meta[frames.MetaDTMFDigit] = evt.DTMF.Digit
			nonBlockingSend(t.recvCh, frames.NewControlFrame(streamID, time.Now().UnixNano(), frames.ControlDTMF, meta))
		case "stop":
			meta := t.metaForStream(streamID)
			reason := ""
			if evt.Stop != nil {
				reason = normalizeCallEndReason(evt.Stop.Reason)
			}
			if reason == "" {
				reason = "completed"
			}
			if reason != "" {
				meta[frames.MetaCallEndReason] = reason
			}
			nonBlockingSend(t.recvCh, frames.NewSystemFrame(streamID, time.Now().UnixNano(), "call_end", meta))
			t.detach(streamID)
			return
		}
	}
	if streamID != "" {
		meta := t.metaForStream(streamID)
		meta[frames.MetaCallEndReason] = normalizeCallEndReason("transport_closed")
		nonBlockingSend(t.recvCh, frames.NewSystemFrame(streamID, time.Now().UnixNano(), "call_end", meta))
		t.detach(streamID)
	}
}

func (t *Transport) Send(f frames.Frame) error {
	if f.Kind() == frames.KindControl {
		cf := f.(frames.ControlFrame)
		streamID := cf.Meta()[frames.MetaStreamID]
		switch cf.Code() {
		case frames.ControlFallback:
			return t.sendFallback(streamID)
		case frames.ControlFlush, frames.ControlCancel, frames.ControlStartInterruption:
			return t.clearBuffer(streamID)
		default:
			return nil
		}
	}
	if f.Kind() != frames.KindAudio {
		return nil
	}
	af := f.(frames.AudioFrame)
	streamID := af.Meta()[frames.MetaStreamID]
	sess := t.session(streamID)
	if sess == nil {
		return nil
	}
	payload := base64.StdEncoding.EncodeToString(af.RawPayload())
	msg := map[string]any{
		"event":     "media",
		"streamSid": streamID,
		"media": map[string]any{
			"payload": payload,
		},
	}
	return sess.enqueue(msg)
}

// Dial places an outbound call using Twilio REST API.
func (t *Transport) Dial(ctx context.Context, to, from, url string) (string, error) {
	dialer := NewDialer(t.cfg)
	return dialer.Dial(ctx, to, from, url)
}

// DialWithOptions places an outbound call using Twilio REST API with options.
func (t *Transport) DialWithOptions(ctx context.Context, to, from, url string, opts transports.DialOptions) (string, error) {
	dialer := NewDialer(t.cfg)
	return dialer.DialWithOptions(ctx, to, from, url, opts)
}

// SendDTMF sends DTMF digits on an active call using Twilio REST API.
func (t *Transport) SendDTMF(ctx context.Context, callSID, digits string) error {
	_ = ctx
	if strings.TrimSpace(callSID) == "" {
		return errors.New("call sid required")
	}
	if strings.TrimSpace(digits) == "" {
		return errors.New("digits required")
	}
	if t.cfg.AccountSID == "" || t.cfg.AuthToken == "" {
		return errors.New("missing twilio credentials")
	}
	updater := t.updateClient
	if updater == nil {
		rest := twilio.NewRestClientWithParams(twilio.ClientParams{
			Username: t.cfg.AccountSID,
			Password: t.cfg.AuthToken,
		})
		updater = rest.Api
	}
	params := &api.UpdateCallParams{}
	params.SetTwiml(buildDTMFTwiml(digits))
	_, err := updater.UpdateCall(callSID, params)
	return err
}

func (t *Transport) handleVoice(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if t.cfg.AuthToken != "" && !t.validateTwilioRequest(r) {
		slog.Warn("twilio_invalid_signature", "reason_code", string(errorsx.ReasonTransportInvalidSignature))
		w.WriteHeader(http.StatusForbidden)
		return
	}
	wsURL := t.websocketURL(r)
	greeting := strings.TrimSpace(t.cfg.VoiceGreeting)
	if greeting != "" {
		greeting = xmlEscape(greeting)
	}
	var twiml string
	if greeting != "" {
		twiml = `<Response><Say>` + greeting + `</Say><Connect><Stream url="` + wsURL + `"/></Connect></Response>`
	} else {
		twiml = `<Response><Connect><Stream url="` + wsURL + `"/></Connect></Response>`
	}
	w.Header().Set("Content-Type", "text/xml")
	_, _ = w.Write([]byte(twiml))
}

func (t *Transport) handleTTSWebhook(w http.ResponseWriter, r *http.Request) {
	if t.cfg.AuthToken != "" && !t.validateTwilioRequest(r) {
		slog.Warn("twilio_tts_webhook_invalid_signature", "reason_code", string(errorsx.ReasonTransportInvalidSignature))
		w.WriteHeader(http.StatusForbidden)
		return
	}
	streamID := r.URL.Query().Get("stream_id")
	if streamID == "" {
		t.mu.Lock()
		if len(t.sessions) == 1 {
			for sID := range t.sessions {
				streamID = sID
				break
			}
		}
		t.mu.Unlock()
	}
	if streamID == "" {
		w.WriteHeader(http.StatusOK)
		return
	}
	meta := t.metaForStream(streamID)
	nonBlockingSend(t.recvCh, frames.NewControlFrame(streamID, time.Now().UnixNano(), frames.ControlAudioReady, meta))
	w.WriteHeader(http.StatusOK)
}

func (t *Transport) handleStatusCallback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if t.cfg.AuthToken != "" && !t.validateTwilioRequest(r) {
		slog.Warn("twilio_status_invalid_signature", "reason_code", string(errorsx.ReasonTransportInvalidSignature))
		w.WriteHeader(http.StatusForbidden)
		return
	}
	if err := r.ParseForm(); err != nil {
		w.WriteHeader(http.StatusOK)
		return
	}
	callSID := r.FormValue("CallSid")
	status := r.FormValue("CallStatus")
	reason := normalizeCallEndReason(status)
	if reason == "" || callSID == "" {
		w.WriteHeader(http.StatusOK)
		return
	}
	streamID := t.streamForCall(callSID)
	if streamID == "" {
		w.WriteHeader(http.StatusOK)
		return
	}
	meta := t.metaForStream(streamID)
	meta[frames.MetaCallEndReason] = reason
	nonBlockingSend(t.recvCh, frames.NewSystemFrame(streamID, time.Now().UnixNano(), "call_end", meta))
	t.detach(streamID)
	w.WriteHeader(http.StatusOK)
}

func (t *Transport) websocketURL(r *http.Request) string {
	if t.cfg.PublicURL != "" {
		return "wss://" + normalizePublicURL(t.cfg.PublicURL) + t.cfg.WebsocketPath
	}
	host := r.Host
	if host == "" {
		host = strings.TrimPrefix(t.cfg.ServerAddr, ":")
	}
	return "wss://" + host + t.cfg.WebsocketPath
}

func (t *Transport) voiceWebhookURL() string {
	if t.cfg.PublicURL != "" {
		return "https://" + normalizePublicURL(t.cfg.PublicURL) + t.cfg.VoicePath
	}
	addr := t.cfg.ServerAddr
	if addr == "" {
		addr = ":8080"
	}
	if strings.HasPrefix(addr, ":") {
		addr = "localhost" + addr
	}
	return "http://" + addr + t.cfg.VoicePath
}

func (t *Transport) statusCallbackURL() string {
	if t.cfg.PublicURL != "" {
		return "https://" + normalizePublicURL(t.cfg.PublicURL) + t.cfg.StatusCallbackPath
	}
	addr := t.cfg.ServerAddr
	if addr == "" {
		addr = ":8080"
	}
	if strings.HasPrefix(addr, ":") {
		addr = "localhost" + addr
	}
	return "http://" + addr + t.cfg.StatusCallbackPath
}

func (t *Transport) attach(streamID, callSID, traceID, from string, conn *websocket.Conn) (string, *session) {
	sess := &session{
		conn:   conn,
		sendCh: make(chan []byte, 256),
	}
	var oldStream string
	var oldSess *session
	t.mu.Lock()
	if callSID != "" {
		if existing := t.callStreams[callSID]; existing != "" && existing != streamID {
			oldStream = existing
			oldSess = t.sessions[existing]
			delete(t.sessions, existing)
			delete(t.callSIDs, existing)
			delete(t.traceIDs, existing)
			delete(t.fromNumbers, existing)
		}
		t.callStreams[callSID] = streamID
	}
	t.sessions[streamID] = sess
	t.callSIDs[streamID] = callSID
	t.traceIDs[streamID] = traceID
	if from != "" {
		t.fromNumbers[streamID] = from
	}
	t.mu.Unlock()
	go sess.loop()
	return oldStream, oldSess
}

func (t *Transport) detach(streamID string) {
	t.mu.Lock()
	sess := t.sessions[streamID]
	callSID := t.callSIDs[streamID]
	delete(t.sessions, streamID)
	delete(t.callSIDs, streamID)
	delete(t.traceIDs, streamID)
	delete(t.fromNumbers, streamID)
	if callSID != "" && t.callStreams[callSID] == streamID {
		delete(t.callStreams, callSID)
	}
	t.mu.Unlock()
	if sess != nil {
		_ = sess.close()
	}
}

func (t *Transport) session(streamID string) *session {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.sessions[streamID]
}

func (t *Transport) streamForCall(callSID string) string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.callStreams[callSID]
}

func (t *Transport) metaForStream(streamID string) map[string]string {
	t.mu.Lock()
	defer t.mu.Unlock()
	meta := map[string]string{frames.MetaStreamID: streamID}
	if v := t.callSIDs[streamID]; v != "" {
		meta[frames.MetaCallSID] = v
	}
	if v := t.traceIDs[streamID]; v != "" {
		meta[frames.MetaTraceID] = v
	}
	if v := t.fromNumbers[streamID]; v != "" {
		meta[frames.MetaFromNumber] = v
	}
	return meta
}

func (t *Transport) clearBuffer(streamID string) error {
	sess := t.session(streamID)
	if sess == nil {
		return nil
	}
	msg := map[string]any{
		"event":     "clear",
		"streamSid": streamID,
	}
	return sess.enqueue(msg)
}

func (t *Transport) sendFallback(streamID string) error {
	sess := t.session(streamID)
	if sess == nil {
		return nil
	}
	for _, chunk := range fallbackMuLawFrames() {
		payload := base64.StdEncoding.EncodeToString(chunk)
		msg := map[string]any{
			"event":     "media",
			"streamSid": streamID,
			"media": map[string]any{
				"payload": payload,
			},
		}
		_ = sess.enqueue(msg)
	}
	return nil
}

func (t *Transport) validateTwilioRequest(r *http.Request) bool {
	signature := r.Header.Get("X-Twilio-Signature")
	if signature == "" {
		return false
	}
	if t.cfg.AuthToken == "" {
		return false
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return false
	}
	_ = r.Body.Close()
	r.Body = io.NopCloser(bytes.NewReader(body))

	validator := twilioclient.NewRequestValidator(t.cfg.AuthToken)
	return validator.ValidateBody(t.requestURL(r), body, signature)
}

func (t *Transport) requestURL(r *http.Request) string {
	if t.cfg.PublicURL != "" {
		base := strings.TrimRight(t.cfg.PublicURL, "/")
		return base + r.URL.RequestURI()
	}
	scheme := r.URL.Scheme
	if scheme == "" {
		if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
			scheme = proto
		} else {
			scheme = "https"
		}
	}
	host := r.Host
	if host == "" {
		host = strings.TrimPrefix(t.cfg.ServerAddr, ":")
	}
	return scheme + "://" + host + r.URL.RequestURI()
}

func (t *Transport) checkOrigin(r *http.Request) bool {
	if t.cfg.AllowAnyOrigin {
		return true
	}
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return true
	}
	origin = strings.TrimRight(origin, "/")
	originHost := strings.TrimPrefix(origin, "https://")
	originHost = strings.TrimPrefix(originHost, "http://")
	for _, allowed := range t.cfg.AllowedOrigins {
		a := strings.TrimSpace(allowed)
		if a == "" {
			continue
		}
		a = strings.TrimRight(a, "/")
		if strings.HasPrefix(a, "http://") || strings.HasPrefix(a, "https://") {
			if strings.EqualFold(a, origin) {
				return true
			}
			continue
		}
		if strings.EqualFold(a, originHost) {
			return true
		}
	}
	return false
}

func buildDTMFTwiml(digits string) string {
	escaped := xmlEscape(digits)
	return fmt.Sprintf(`<Response><Play digits="%s"/></Response>`, escaped)
}

func xmlEscape(in string) string {
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		"\"", "&quot;",
		"'", "&apos;",
	)
	return replacer.Replace(in)
}

func normalizeCallEndReason(raw string) string {
	r := strings.ToLower(strings.TrimSpace(raw))
	if r == "" {
		return ""
	}
	switch r {
	case "queued", "ringing", "in-progress", "inprogress":
		return ""
	case "completed", "call_ended", "call-ended", "completed_by_user", "hangup":
		return "completed"
	case "busy":
		return "busy"
	case "no_answer", "noanswer", "no-answer":
		return "no_answer"
	case "failed", "error", "canceled", "cancelled", "transport_closed":
		return "failed"
	default:
		return "unknown"
	}
}

type session struct {
	conn   *websocket.Conn
	sendCh chan []byte
	mu     sync.Mutex
	closed atomic.Bool
}

func (s *session) enqueue(msg map[string]any) error {
	b, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	select {
	case s.sendCh <- b:
	default:
	}
	return nil
}

func (s *session) loop() {
	for msg := range s.sendCh {
		_ = s.conn.WriteMessage(websocket.TextMessage, msg)
	}
}

func (s *session) close() error {
	if s.closed.CompareAndSwap(false, true) {
		close(s.sendCh)
	}
	return s.conn.Close()
}

type TwilioStart struct {
	CallSID  string `json:"callSid"`
	StreamID string `json:"streamSid"`
	From     string `json:"from"`
}

type TwilioMedia struct {
	Payload string `json:"payload"`
}

type TwilioDTMF struct {
	Digit string `json:"digit"`
}

type TwilioStop struct {
	Reason string `json:"reason"`
}

type TwilioEvent struct {
	Event string       `json:"event"`
	Start *TwilioStart `json:"start,omitempty"`
	Media *TwilioMedia `json:"media,omitempty"`
	DTMF  *TwilioDTMF  `json:"dtmf,omitempty"`
	Stop  *TwilioStop  `json:"stop,omitempty"`
}

func normalizePublicURL(v string) string {
	if v == "" {
		return ""
	}
	if len(v) >= 8 && v[:8] == "https://" {
		return v[8:]
	}
	if len(v) >= 7 && v[:7] == "http://" {
		return v[7:]
	}
	for len(v) > 0 && v[len(v)-1] == '/' {
		v = v[:len(v)-1]
	}
	return v
}

var fallbackMuLawOnce sync.Once
var fallbackMuLaw [][]byte

func fallbackMuLawFrames() [][]byte {
	fallbackMuLawOnce.Do(func() {
		silence := bytes.Repeat([]byte{0xFF}, 160*5)
		for i := 0; i < len(silence); i += 160 {
			fallbackMuLaw = append(fallbackMuLaw, silence[i:i+160])
		}
	})
	return fallbackMuLaw
}

func nonBlockingSend(ch chan frames.Frame, f frames.Frame) {
	select {
	case ch <- f:
	default:
	}
}
