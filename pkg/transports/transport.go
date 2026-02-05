package transports

import (
	"context"

	"github.com/harunnryd/ranya/pkg/frames"
)

// Transport defines a vendor-agnostic I/O boundary for audio/text/control frames.
// Implementations are responsible for their own network lifecycle.
type Transport interface {
	Name() string
	Start(ctx context.Context) error
	Stop() error
	Recv() <-chan frames.Frame
	Send(frames.Frame) error
}

// DTMFSender allows transports to send DTMF digits during an active call.
type DTMFSender interface {
	SendDTMF(ctx context.Context, callSID, digits string) error
}

// OutboundDialer allows transports to initiate outbound calls.
type OutboundDialer interface {
	Dial(ctx context.Context, to, from, url string) (callSID string, err error)
}

// DialOptions carries optional outbound dial settings.
type DialOptions struct {
	SendDigits string
}

// OutboundDialerWithOptions extends dialing with optional parameters.
type OutboundDialerWithOptions interface {
	DialWithOptions(ctx context.Context, to, from, url string, opts DialOptions) (callSID string, err error)
}

// ReadyReporter allows transports to expose readiness metadata (e.g., webhook URLs).
// Implementations are optional and used for informational logging only.
type ReadyReporter interface {
	ReadyFields() map[string]any
}
