package metrics

import (
	"math"
	"sync/atomic"
)

type SamplingObserver struct {
	inner       Observer
	rate        float64
	sampleEvery uint64
	counter     uint64
}

func NewSamplingObserver(inner Observer, rate float64) *SamplingObserver {
	if rate > 1 {
		rate = 1
	}
	if rate < 0 {
		rate = 0
	}
	var every uint64
	if rate == 0 {
		every = 0
	} else if rate == 1 {
		every = 1
	} else {
		every = uint64(math.Round(1.0 / rate))
		if every == 0 {
			every = 1
		}
	}
	return &SamplingObserver{inner: inner, rate: rate, sampleEvery: every}
}

func (s *SamplingObserver) RecordEvent(ev MetricsEvent) {
	if s.rate == 0 {
		return
	}
	if s.sampleEvery <= 1 {
		s.inner.RecordEvent(ev)
		return
	}
	n := atomic.AddUint64(&s.counter, 1)
	if n%s.sampleEvery == 0 {
		s.inner.RecordEvent(ev)
	}
}
