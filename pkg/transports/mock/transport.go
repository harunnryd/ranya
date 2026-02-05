package mock

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/harunnryd/ranya/pkg/frames"
)

// Transport is an in-memory transport for local testing and integration.
// It implements the transports.Transport interface without any network dependency.
type Transport struct {
	recvCh chan frames.Frame
	sentCh chan frames.Frame
	closed atomic.Bool
	mu     sync.Mutex
}

func New() *Transport {
	return &Transport{
		recvCh: make(chan frames.Frame, 256),
		sentCh: make(chan frames.Frame, 256),
	}
}

func (t *Transport) Name() string { return "mock" }

func (t *Transport) Start(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	go func() {
		<-ctx.Done()
		_ = t.Stop()
	}()
	return nil
}

func (t *Transport) Stop() error {
	if t.closed.CompareAndSwap(false, true) {
		t.mu.Lock()
		close(t.recvCh)
		close(t.sentCh)
		t.mu.Unlock()
	}
	return nil
}

func (t *Transport) Recv() <-chan frames.Frame { return t.recvCh }

func (t *Transport) Send(f frames.Frame) error {
	if t.closed.Load() {
		return nil
	}
	select {
	case t.sentCh <- f:
	default:
	}
	return nil
}

// Push injects an inbound frame into the transport.
func (t *Transport) Push(f frames.Frame) {
	if t.closed.Load() {
		return
	}
	select {
	case t.recvCh <- f:
	default:
	}
}

// Sent exposes outbound frames for inspection.
func (t *Transport) Sent() <-chan frames.Frame { return t.sentCh }
