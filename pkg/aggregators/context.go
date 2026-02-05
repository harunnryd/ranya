package aggregators

import "time"

type AggregatorConfig struct {
	MinLen       int
	MaxTokens    int
	MaxHistory   int
	FlushTimeout time.Duration
}

type Aggregator interface {
	Configure(cfg AggregatorConfig) error
	AddToken(tok string)
	Flush() string
}
