package twilio

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/harunnryd/ranya/pkg/transports"
	"github.com/twilio/twilio-go"
	api "github.com/twilio/twilio-go/rest/api/v2010"
)

type callCreator interface {
	CreateCall(params *api.CreateCallParams) (*api.ApiV2010Call, error)
}

// Dialer provides outbound call creation via Twilio REST API.
type Dialer struct {
	cfg    Config
	client callCreator
}

// NewDialer creates a new Twilio dialer.
func NewDialer(cfg Config) *Dialer {
	return &Dialer{cfg: cfg.withDefaults()}
}

// Dial places an outbound call using Twilio.
func (d *Dialer) Dial(ctx context.Context, to, from, url string) (string, error) {
	return d.DialWithOptions(ctx, to, from, url, transports.DialOptions{})
}

// DialWithOptions places an outbound call using Twilio with optional settings.
func (d *Dialer) DialWithOptions(ctx context.Context, to, from, url string, opts transports.DialOptions) (string, error) {
	_ = ctx
	if to == "" || from == "" {
		return "", errors.New("to/from required")
	}
	if d.cfg.AccountSID == "" || d.cfg.AuthToken == "" {
		return "", errors.New("missing twilio credentials")
	}
	if url == "" {
		url = d.voiceWebhookURL()
	}
	client := d.client
	if client == nil {
		rest := twilio.NewRestClientWithParams(twilio.ClientParams{
			Username: d.cfg.AccountSID,
			Password: d.cfg.AuthToken,
		})
		client = rest.Api
	}
	params := &api.CreateCallParams{}
	params.SetTo(to)
	params.SetFrom(from)
	params.SetUrl(url)
	if strings.TrimSpace(opts.SendDigits) != "" {
		params.SetSendDigits(opts.SendDigits)
	}
	resp, err := client.CreateCall(params)
	if err != nil {
		return "", err
	}
	if resp == nil || resp.Sid == nil {
		return "", fmt.Errorf("missing call sid")
	}
	return *resp.Sid, nil
}

func (d *Dialer) voiceWebhookURL() string {
	if d.cfg.PublicURL != "" {
		return "https://" + normalizePublicURL(d.cfg.PublicURL) + d.cfg.VoicePath
	}
	addr := d.cfg.ServerAddr
	if addr == "" {
		addr = ":8080"
	}
	if addr[0] == ':' {
		addr = "localhost" + addr
	}
	return "http://" + addr + d.cfg.VoicePath
}
