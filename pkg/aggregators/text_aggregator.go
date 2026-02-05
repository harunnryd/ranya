package aggregators

import (
	"strings"
	"sync"
	"time"

	"github.com/harunnryd/ranya/pkg/frames"
	"github.com/harunnryd/ranya/pkg/pipeline"
)

type TextAggregator struct {
	mu          sync.Mutex
	cfg         AggregatorConfig
	sb          strings.Builder
	tokenCount  int
	firstPTS    int64
	streamID    string
	meta        map[string]string
	lastTokenAt time.Time
	history     []string
}

func NewTextAggregator(cfg AggregatorConfig) *TextAggregator {
	if cfg.MinLen <= 0 {
		cfg.MinLen = 8
	}
	if cfg.MaxTokens <= 0 {
		cfg.MaxTokens = 256
	}
	if cfg.MaxHistory <= 0 {
		cfg.MaxHistory = 10
	}
	if cfg.FlushTimeout <= 0 {
		cfg.FlushTimeout = 300 * time.Millisecond
	}
	return &TextAggregator{cfg: cfg}
}

func (a *TextAggregator) Configure(cfg AggregatorConfig) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if cfg.MinLen > 0 {
		a.cfg.MinLen = cfg.MinLen
	}
	if cfg.MaxTokens > 0 {
		a.cfg.MaxTokens = cfg.MaxTokens
	}
	if cfg.MaxHistory > 0 {
		a.cfg.MaxHistory = cfg.MaxHistory
	}
	if cfg.FlushTimeout > 0 {
		a.cfg.FlushTimeout = cfg.FlushTimeout
	}
	return nil
}

func (a *TextAggregator) AddToken(tok string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.sb.WriteString(tok)
	a.tokenCount++
	a.lastTokenAt = time.Now()
}

func (a *TextAggregator) Flush() string {
	f := a.FlushFrame()
	if f != nil {
		return f.Text()
	}
	return ""
}

func (a *TextAggregator) FlushFrame() *frames.TextFrame {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := strings.TrimSpace(a.sb.String())
	if out == "" {
		return nil
	}

	tf := frames.NewTextFrame(a.streamID, a.firstPTS, out, a.meta)

	a.sb.Reset()
	a.tokenCount = 0
	a.firstPTS = 0
	a.streamID = ""
	a.meta = nil
	a.appendHistory(out)

	return &tf
}

func (a *TextAggregator) Name() string { return "text_aggregator" }

func (a *TextAggregator) Process(f frames.Frame) ([]frames.Frame, error) {
	switch f.Kind() {
	case frames.KindText:
		tf := f.(frames.TextFrame)
		a.mu.Lock()
		if a.firstPTS == 0 {
			a.firstPTS = tf.PTS()
			a.streamID = tf.Meta()[frames.MetaStreamID]
			a.meta = tf.Meta()
		}
		a.sb.WriteString(tf.Text())
		a.tokenCount++
		a.lastTokenAt = time.Now()
		text := a.sb.String()
		isFinal := tf.Meta()[frames.MetaIsFinal] == "true"
		shouldFlush := eosDetected(text) || a.tokenCount >= a.cfg.MaxTokens || isFinal
		final := strings.TrimSpace(text)
		if shouldFlush && len(final) >= a.cfg.MinLen {
			out := frames.NewTextFrame(a.streamID, a.firstPTS, final, a.meta)
			a.sb.Reset()
			a.tokenCount = 0
			a.firstPTS = 0
			a.streamID = ""
			a.meta = nil
			a.appendHistory(final)
			a.mu.Unlock()
			return []frames.Frame{out}, nil
		}
		a.mu.Unlock()
		return nil, nil
	default:
		a.mu.Lock()
		text := strings.TrimSpace(a.sb.String())
		timeout := time.Since(a.lastTokenAt) > a.cfg.FlushTimeout && a.tokenCount > 0
		if timeout && len(text) >= a.cfg.MinLen {
			out := frames.NewTextFrame(a.streamID, a.firstPTS, text, a.meta)
			a.sb.Reset()
			a.tokenCount = 0
			a.firstPTS = 0
			a.streamID = ""
			a.meta = nil
			a.appendHistory(text)
			a.mu.Unlock()
			return []frames.Frame{out, f}, nil
		}
		a.mu.Unlock()
		return []frames.Frame{f}, nil
	}
}

func eosDetected(s string) bool {
	t := strings.TrimSpace(s)
	if len(t) == 0 {
		return false
	}
	if strings.HasSuffix(t, "...") {
		return len(t) >= 12
	}
	last := t[len(t)-1]
	return last == '.' || last == '!' || last == '?' || last == '\n'
}

var _ pipeline.FrameProcessor = (*TextAggregator)(nil)

func (a *TextAggregator) appendHistory(text string) {
	if a.cfg.MaxHistory <= 0 {
		return
	}
	a.history = append(a.history, text)
	if len(a.history) > a.cfg.MaxHistory {
		a.history = a.history[len(a.history)-a.cfg.MaxHistory:]
	}
}

func (a *TextAggregator) History() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]string, len(a.history))
	copy(out, a.history)
	return out
}
