package resilience

import "time"

// RetryPolicy defines retry behavior for transient failures.
type RetryPolicy struct {
	MaxRetries int
	Backoff    time.Duration
}

func NewRetryPolicy(maxRetries int, backoff time.Duration) RetryPolicy {
	if maxRetries <= 0 {
		maxRetries = 2
	}
	if backoff <= 0 {
		backoff = 200 * time.Millisecond
	}
	return RetryPolicy{MaxRetries: maxRetries, Backoff: backoff}
}

func (r RetryPolicy) Do(fn func() error) error {
	var err error
	for i := 0; i <= r.MaxRetries; i++ {
		err = fn()
		if err == nil {
			return nil
		}
		if i == r.MaxRetries {
			return err
		}
		time.Sleep(r.Backoff)
	}
	return err
}
