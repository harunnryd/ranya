package turn

import (
	"github.com/harunnryd/ranya/pkg/frames"
)

type InterruptEmitter interface {
	Emit(frame frames.Frame) error
}

func NewFlushFrame(streamID string, pts int64) frames.ControlFrame {
	return frames.NewControlFrame(streamID, pts, frames.ControlFlush, nil)
}

func NewCancelFrame(streamID string, pts int64) frames.ControlFrame {
	return frames.NewControlFrame(streamID, pts, frames.ControlCancel, nil)
}

func NewInterruptFrame(streamID string, pts int64) frames.ControlFrame {
	return frames.NewControlFrame(streamID, pts, frames.ControlStartInterruption, nil)
}
