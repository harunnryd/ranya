package priority

import (
	"sync/atomic"
	"time"
)

type Stats struct {
	HighPush int64
	LowPush  int64
	HighPop  int64
	LowPop   int64
}

type Queue interface {
	TryPushHigh(f any) bool
	TryPushLow(f any) bool
	Pop() (any, bool)
	Stats() Stats
}

type PriorityQueue struct {
	high     chan any
	low      chan any
	fairness int
	highPush int64
	lowPush  int64
	highPop  int64
	lowPop   int64
}

func New(highCap, lowCap, fairness int) *PriorityQueue {
	if fairness <= 0 {
		fairness = 3
	}
	return &PriorityQueue{
		high:     make(chan any, highCap),
		low:      make(chan any, lowCap),
		fairness: fairness,
	}
}

func (q *PriorityQueue) TryPushHigh(f any) bool {
	select {
	case q.high <- f:
		atomic.AddInt64(&q.highPush, 1)
		return true
	default:
		return false
	}
}

func (q *PriorityQueue) TryPushLow(f any) bool {
	select {
	case q.low <- f:
		atomic.AddInt64(&q.lowPush, 1)
		return true
	default:
		return false
	}
}

func (q *PriorityQueue) Pop() (any, bool) {
	for {
		select {
		case f := <-q.high:
			atomic.AddInt64(&q.highPop, 1)
			return f, true
		default:
		}
		if q.fairness > 0 {
			select {
			case f := <-q.low:
				atomic.AddInt64(&q.lowPop, 1)
				return f, true
			default:
			}
		}
		time.Sleep(time.Millisecond)
	}
}

func (q *PriorityQueue) Stats() Stats {
	return Stats{
		HighPush: atomic.LoadInt64(&q.highPush),
		LowPush:  atomic.LoadInt64(&q.lowPush),
		HighPop:  atomic.LoadInt64(&q.highPop),
		LowPop:   atomic.LoadInt64(&q.lowPop),
	}
}
