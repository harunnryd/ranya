package processors

import (
	"strings"

	"github.com/harunnryd/ranya/pkg/frames"
	"github.com/harunnryd/ranya/pkg/pipeline"
)

type ResponseLimiterConfig struct {
	MaxChars      int
	MaxSentences  int
	SourceFilters map[string]bool
}

// ResponseLimiter enforces short-turn responses for telephony.
type ResponseLimiter struct {
	cfg ResponseLimiterConfig
}

func NewResponseLimiter(cfg ResponseLimiterConfig) *ResponseLimiter {
	if cfg.MaxChars <= 0 {
		cfg.MaxChars = 420
	}
	if cfg.MaxSentences <= 0 {
		cfg.MaxSentences = 3
	}
	if cfg.SourceFilters == nil {
		cfg.SourceFilters = map[string]bool{"llm": true, "system": true}
	}
	return &ResponseLimiter{cfg: cfg}
}

func (r *ResponseLimiter) Name() string { return "response_limiter" }

func (r *ResponseLimiter) Process(f frames.Frame) ([]frames.Frame, error) {
	if f.Kind() != frames.KindText {
		return []frames.Frame{f}, nil
	}
	tf := f.(frames.TextFrame)
	meta := tf.Meta()
	if !r.cfg.SourceFilters[meta[frames.MetaSource]] {
		return []frames.Frame{f}, nil
	}
	text := strings.TrimSpace(tf.Text())
	if text == "" {
		return []frames.Frame{f}, nil
	}
	truncated := truncateSentences(text, r.cfg.MaxSentences)
	if len(truncated) > r.cfg.MaxChars {
		truncated = truncated[:r.cfg.MaxChars]
		truncated = strings.TrimSpace(truncated)
	}
	if truncated != text {
		meta[frames.MetaShortTurnEnforced] = "true"
		return []frames.Frame{frames.NewTextFrame(meta[frames.MetaStreamID], tf.PTS(), truncated, meta)}, nil
	}
	return []frames.Frame{f}, nil
}

func truncateSentences(text string, maxSentences int) string {
	if maxSentences <= 0 {
		return text
	}
	var out strings.Builder
	count := 0
	for _, r := range text {
		out.WriteRune(r)
		if r == '.' || r == '!' || r == '?' {
			count++
			if count >= maxSentences {
				break
			}
		}
	}
	result := strings.TrimSpace(out.String())
	if result == "" {
		return text
	}
	return result
}

var _ pipeline.FrameProcessor = (*ResponseLimiter)(nil)
