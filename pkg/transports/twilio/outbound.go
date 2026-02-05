package twilio

import (
	"errors"

	"github.com/twilio/twilio-go"
	api "github.com/twilio/twilio-go/rest/api/v2010"
)

type OutboundClient struct {
	accountSID string
	authToken  string
}

func NewOutboundClient(accountSID, authToken string) *OutboundClient {
	return &OutboundClient{accountSID: accountSID, authToken: authToken}
}

func (c *OutboundClient) MakeCall(to string, from string, webhookURL string) (string, error) {
	if c.accountSID == "" || c.authToken == "" {
		return "", errors.New("missing twilio credentials")
	}
	if to == "" || from == "" || webhookURL == "" {
		return "", errors.New("missing call params")
	}
	client := twilio.NewRestClientWithParams(twilio.ClientParams{
		Username: c.accountSID,
		Password: c.authToken,
	})
	params := &api.CreateCallParams{}
	params.SetTo(to)
	params.SetFrom(from)
	params.SetUrl(webhookURL)
	resp, err := client.Api.CreateCall(params)
	if err != nil {
		return "", err
	}
	if resp.Sid == nil {
		return "", errors.New("missing call sid")
	}
	return *resp.Sid, nil
}
