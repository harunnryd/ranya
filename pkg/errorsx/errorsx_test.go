package errorsx

import "testing"

func TestWrapAndReason(t *testing.T) {
	err := Wrap(assertErr{}, ReasonLLMGenerate)
	if Reason(err) != ReasonLLMGenerate {
		t.Fatalf("expected reason %s, got %s", ReasonLLMGenerate, Reason(err))
	}
	if !HasReason(err, ReasonLLMGenerate) {
		t.Fatalf("expected HasReason true")
	}
}

func TestWrapPreservesExistingReason(t *testing.T) {
	first := Wrap(assertErr{}, ReasonSTTSend)
	second := Wrap(first, ReasonLLMGenerate)
	if Reason(second) != ReasonSTTSend {
		t.Fatalf("expected reason preserved, got %s", Reason(second))
	}
}

type assertErr struct{}

func (assertErr) Error() string { return "boom" }
