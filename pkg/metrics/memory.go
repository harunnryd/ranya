package metrics

import "sync"

type MemoryObserver struct {
	mu     sync.Mutex
	Events []MetricsEvent
}

func NewMemoryObserver() *MemoryObserver {
	return &MemoryObserver{}
}

func (m *MemoryObserver) RecordEvent(ev MetricsEvent) {
	m.mu.Lock()
	m.Events = append(m.Events, ev)
	m.mu.Unlock()
}
