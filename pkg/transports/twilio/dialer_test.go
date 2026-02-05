package twilio

import (
	"context"
	"testing"

	"github.com/harunnryd/ranya/pkg/transports"
	api "github.com/twilio/twilio-go/rest/api/v2010"
)

type stubCreator struct {
	last *api.CreateCallParams
	sid  string
	err  error
}

func (s *stubCreator) CreateCall(params *api.CreateCallParams) (*api.ApiV2010Call, error) {
	s.last = params
	if s.err != nil {
		return nil, s.err
	}
	return &api.ApiV2010Call{Sid: &s.sid}, nil
}

func TestDialerDialUsesDefaults(t *testing.T) {
	stub := &stubCreator{sid: "CA123"}
	cfg := Config{
		AccountSID: "AC1",
		AuthToken:  "token",
		PublicURL:  "https://example.com",
		VoicePath:  "/voice",
	}
	d := NewDialer(cfg)
	d.client = stub

	sid, err := d.Dial(context.Background(), "+100", "+200", "")
	if err != nil {
		t.Fatalf("dial error: %v", err)
	}
	if sid != "CA123" {
		t.Fatalf("expected sid CA123, got %s", sid)
	}
	if stub.last == nil || stub.last.To == nil || *stub.last.To != "+100" {
		t.Fatalf("expected To param")
	}
	if stub.last.From == nil || *stub.last.From != "+200" {
		t.Fatalf("expected From param")
	}
	if stub.last.Url == nil {
		t.Fatalf("expected Url param")
	}
}

func TestDialerDialUsesOverrideURL(t *testing.T) {
	stub := &stubCreator{sid: "CA999"}
	cfg := Config{AccountSID: "AC1", AuthToken: "token"}
	d := NewDialer(cfg)
	d.client = stub

	override := "https://override.example.com/voice"
	_, err := d.Dial(context.Background(), "+100", "+200", override)
	if err != nil {
		t.Fatalf("dial error: %v", err)
	}
	if stub.last == nil || stub.last.Url == nil || *stub.last.Url != override {
		t.Fatalf("expected override url")
	}
}

func TestDialerDialWithOptionsSendDigits(t *testing.T) {
	stub := &stubCreator{sid: "CA777"}
	cfg := Config{AccountSID: "AC1", AuthToken: "token"}
	d := NewDialer(cfg)
	d.client = stub

	_, err := d.DialWithOptions(context.Background(), "+100", "+200", "https://example.com/voice", transports.DialOptions{SendDigits: "W123#"})
	if err != nil {
		t.Fatalf("dial error: %v", err)
	}
	if stub.last == nil || stub.last.SendDigits == nil || *stub.last.SendDigits != "W123#" {
		t.Fatalf("expected SendDigits param")
	}
}
