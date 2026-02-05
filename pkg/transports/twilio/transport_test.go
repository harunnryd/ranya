package twilio

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/harunnryd/ranya/pkg/frames"
	api "github.com/twilio/twilio-go/rest/api/v2010"
)

func TestSendStartInterruptionClearsBuffer(t *testing.T) {
	tr := New(Config{})
	sess := &session{sendCh: make(chan []byte, 1)}
	tr.mu.Lock()
	tr.sessions["stream-1"] = sess
	tr.mu.Unlock()

	cf := frames.NewControlFrame("stream-1", time.Now().UnixNano(), frames.ControlStartInterruption, map[string]string{})
	if err := tr.Send(cf); err != nil {
		t.Fatalf("send error: %v", err)
	}

	select {
	case msg := <-sess.sendCh:
		var payload map[string]any
		if err := json.Unmarshal(msg, &payload); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		evt, _ := payload["event"].(string)
		if evt != "clear" {
			t.Fatalf("expected clear event, got %q", evt)
		}
	default:
		t.Fatalf("expected clear event to be enqueued")
	}
}

func TestHandleVoiceSignatureValidation(t *testing.T) {
	cfg := Config{AuthToken: "token", PublicURL: "https://example.com", VoicePath: "/voice"}
	tr := New(cfg)

	form := url.Values{}
	form.Set("CallSid", "CA123")
	form.Set("From", "+123")
	body := form.Encode()

	req := httptest.NewRequest(http.MethodPost, "https://example.com/voice", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	params := map[string]string{"CallSid": "CA123", "From": "+123"}
	sig := computeSignature(cfg.AuthToken, tr.requestURL(req), params)
	req.Header.Set("X-Twilio-Signature", sig)

	w := httptest.NewRecorder()
	tr.handleVoice(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	reqInvalid := httptest.NewRequest(http.MethodPost, "https://example.com/voice", strings.NewReader(body))
	reqInvalid.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqInvalid.Header.Set("X-Twilio-Signature", "invalid")
	wInvalid := httptest.NewRecorder()
	tr.handleVoice(wInvalid, reqInvalid)
	if wInvalid.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", wInvalid.Code)
	}
}

func TestHandleTTSWebhookSignatureValidation(t *testing.T) {
	cfg := Config{AuthToken: "token", PublicURL: "https://example.com", TTSWebhookPath: "/tts/webhook"}
	tr := New(cfg)

	req := httptest.NewRequest(http.MethodPost, "https://example.com/tts/webhook?stream_id=stream-1", nil)
	sig := computeSignature(cfg.AuthToken, tr.requestURL(req), map[string]string{})
	req.Header.Set("X-Twilio-Signature", sig)

	w := httptest.NewRecorder()
	tr.handleTTSWebhook(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	reqInvalid := httptest.NewRequest(http.MethodPost, "https://example.com/tts/webhook?stream_id=stream-1", nil)
	reqInvalid.Header.Set("X-Twilio-Signature", "invalid")
	wInvalid := httptest.NewRecorder()
	tr.handleTTSWebhook(wInvalid, reqInvalid)
	if wInvalid.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", wInvalid.Code)
	}
}

type stubCallUpdater struct {
	lastSID   string
	lastTwiml string
	err       error
}

func (s *stubCallUpdater) UpdateCall(sid string, params *api.UpdateCallParams) (*api.ApiV2010Call, error) {
	s.lastSID = sid
	if params != nil && params.Twiml != nil {
		s.lastTwiml = *params.Twiml
	}
	if s.err != nil {
		return nil, s.err
	}
	return &api.ApiV2010Call{}, nil
}

func TestSendDTMF(t *testing.T) {
	tr := New(Config{AccountSID: "AC123", AuthToken: "token"})
	stub := &stubCallUpdater{}
	tr.updateClient = stub

	if err := tr.SendDTMF(context.Background(), "CA123", "W123#"); err != nil {
		t.Fatalf("SendDTMF error: %v", err)
	}
	if stub.lastSID != "CA123" {
		t.Fatalf("expected call sid CA123, got %q", stub.lastSID)
	}
	if !strings.Contains(stub.lastTwiml, `digits=\"W123#\"`) && !strings.Contains(stub.lastTwiml, `digits="W123#"`) {
		t.Fatalf("expected TwiML digits in request, got %q", stub.lastTwiml)
	}

	stub.err = errors.New("boom")
	if err := tr.SendDTMF(context.Background(), "CA123", "1"); err == nil {
		t.Fatalf("expected error on update failure")
	}
}

func TestHandleStatusCallbackMapping(t *testing.T) {
	cfg := Config{AuthToken: "token", PublicURL: "https://example.com", StatusCallbackPath: "/status"}
	tr := New(cfg)
	streamID := "stream-1"
	callSID := "CA123"

	tr.mu.Lock()
	tr.callStreams[callSID] = streamID
	tr.callSIDs[streamID] = callSID
	tr.mu.Unlock()

	form := url.Values{}
	form.Set("CallSid", callSID)
	form.Set("CallStatus", "completed")
	body := form.Encode()

	req := httptest.NewRequest(http.MethodPost, "https://example.com/status", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	params := map[string]string{"CallSid": callSID, "CallStatus": "completed"}
	sig := computeSignature(cfg.AuthToken, tr.requestURL(req), params)
	req.Header.Set("X-Twilio-Signature", sig)

	w := httptest.NewRecorder()
	tr.handleStatusCallback(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	select {
	case frame := <-tr.Recv():
		if frame.Kind() != frames.KindSystem {
			t.Fatalf("expected system frame, got %v", frame.Kind())
		}
		sys, ok := frame.(frames.SystemFrame)
		if !ok {
			t.Fatalf("expected SystemFrame, got %T", frame)
		}
		if sys.Name() != "call_end" {
			t.Fatalf("expected call_end event, got %q", sys.Name())
		}
		meta := sys.Meta()
		if meta[frames.MetaCallEndReason] != "completed" {
			t.Fatalf("expected call_end_reason completed, got %q", meta[frames.MetaCallEndReason])
		}
		if meta[frames.MetaCallSID] != callSID {
			t.Fatalf("expected call_sid %q, got %q", callSID, meta[frames.MetaCallSID])
		}
	case <-time.After(1 * time.Second):
		t.Fatalf("expected call_end frame")
	}
}

func computeSignature(authToken, url string, params map[string]string) string {
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	base := url
	for _, k := range keys {
		base += k + params[k]
	}
	mac := hmac.New(sha1.New, []byte(authToken))
	_, _ = mac.Write([]byte(base))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}
