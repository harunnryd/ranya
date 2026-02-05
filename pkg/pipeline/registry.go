package pipeline

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

type Session struct {
	CallSID  string
	StreamID string
	TraceID  string
	Orch     Orchestrator
	Ctx      context.Context
	Cancel   context.CancelFunc
	Created  time.Time
}

type SessionFactory func(ctx context.Context, callSID, streamID, traceID string) (Orchestrator, error)

type SessionRegistry struct {
	sessions sync.Map
	count    atomic.Int64
	factory  SessionFactory
	draining atomic.Bool
}

func NewSessionRegistry(factory SessionFactory) *SessionRegistry {
	return &SessionRegistry{factory: factory}
}

func (r *SessionRegistry) GetOrCreate(callSID, streamID, traceID string) (*Session, bool, error) {
	if callSID == "" {
		return nil, false, nil
	}
	if v, ok := r.sessions.Load(callSID); ok {
		return v.(*Session), false, nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	orch, err := r.factory(ctx, callSID, streamID, traceID)
	if err != nil {
		cancel()
		return nil, false, err
	}
	if err := orch.Start(); err != nil {
		cancel()
		return nil, false, err
	}
	sess := &Session{
		CallSID:  callSID,
		StreamID: streamID,
		TraceID:  traceID,
		Orch:     orch,
		Ctx:      ctx,
		Cancel:   cancel,
		Created:  time.Now(),
	}
	actual, loaded := r.sessions.LoadOrStore(callSID, sess)
	if loaded {
		_ = orch.Stop()
		cancel()
		return actual.(*Session), false, nil
	}
	r.count.Add(1)
	return sess, true, nil
}

func (r *SessionRegistry) Get(callSID string) (*Session, bool) {
	if v, ok := r.sessions.Load(callSID); ok {
		return v.(*Session), true
	}
	return nil, false
}

func (r *SessionRegistry) Remove(callSID string) {
	if v, ok := r.sessions.LoadAndDelete(callSID); ok {
		sess := v.(*Session)
		if sess.Cancel != nil {
			sess.Cancel()
		}
		if sess.Orch != nil {
			_ = sess.Orch.Stop()
		}
		r.count.Add(-1)
	}
}

func (r *SessionRegistry) CloseAll() {
	r.sessions.Range(func(key, value any) bool {
		callSID, ok := key.(string)
		if ok {
			r.Remove(callSID)
		}
		return true
	})
}

func (r *SessionRegistry) Count() int64 {
	return r.count.Load()
}

func (r *SessionRegistry) SetDraining(v bool) {
	r.draining.Store(v)
}

func (r *SessionRegistry) Draining() bool {
	return r.draining.Load()
}

func (r *SessionRegistry) WaitForEmpty(ctx context.Context, interval time.Duration) bool {
	if interval <= 0 {
		interval = 200 * time.Millisecond
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if r.Count() == 0 {
			return true
		}
		select {
		case <-ctx.Done():
			return false
		case <-ticker.C:
		}
	}
}
