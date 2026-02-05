package redact

import (
	"regexp"
	"strings"
	"sync/atomic"
)

var enabled atomic.Bool

var (
	emailRe = regexp.MustCompile(`(?i)[a-z0-9._%+\-]+@[a-z0-9.\-]+\.[a-z]{2,}`)
	phoneRe = regexp.MustCompile(`\b\+?\d[\d\s\-]{7,}\d\b`)
)

// SetEnabled toggles PII redaction.
func SetEnabled(v bool) {
	enabled.Store(v)
}

// Enabled returns true when redaction is active.
func Enabled() bool {
	return enabled.Load()
}

// Text redacts emails and phone numbers when enabled.
func Text(in string) string {
	if !enabled.Load() || strings.TrimSpace(in) == "" {
		return in
	}
	out := emailRe.ReplaceAllString(in, "[REDACTED_EMAIL]")
	out = phoneRe.ReplaceAllString(out, "[REDACTED_PHONE]")
	return out
}
