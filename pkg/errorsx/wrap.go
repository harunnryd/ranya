package errorsx

import "errors"

// ReasonedError wraps an error with a reason code.
type ReasonedError struct {
	Err    error
	Reason ReasonCode
}

func (e ReasonedError) Error() string {
	if e.Err == nil {
		return string(e.Reason)
	}
	return e.Err.Error()
}

func (e ReasonedError) Unwrap() error {
	return e.Err
}

// Wrap attaches a reason code to an error (no-op if err is nil or already reasoned).
func Wrap(err error, reason ReasonCode) error {
	if err == nil {
		return nil
	}
	var re ReasonedError
	if errors.As(err, &re) {
		return err
	}
	return ReasonedError{Err: err, Reason: reason}
}

// Reason extracts a reason code from an error, if present.
func Reason(err error) ReasonCode {
	if err == nil {
		return ReasonUnknown
	}
	var re ReasonedError
	if errors.As(err, &re) {
		return re.Reason
	}
	return ReasonUnknown
}

// HasReason returns true if err contains the given reason code.
func HasReason(err error, reason ReasonCode) bool {
	return Reason(err) == reason
}
