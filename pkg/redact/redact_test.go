package redact

import (
	"strings"
	"testing"
)

func TestRedactDisabled(t *testing.T) {
	SetEnabled(false)
	in := "email a@b.com and phone +62 812 3456 7890"
	if got := Text(in); got != in {
		t.Fatalf("expected no redaction, got %q", got)
	}
}

func TestRedactEnabled(t *testing.T) {
	SetEnabled(true)
	in := "email a@b.com and phone +62 812 3456 7890"
	got := Text(in)
	if got == in {
		t.Fatalf("expected redaction")
	}
	if want := "[REDACTED_EMAIL]"; !strings.Contains(got, want) {
		t.Fatalf("expected %q in output", want)
	}
	if want := "[REDACTED_PHONE]"; !strings.Contains(got, want) {
		t.Fatalf("expected %q in output", want)
	}
}
