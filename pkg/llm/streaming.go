package llm

import (
	"context"

	"github.com/harunnryd/ranya/pkg/frames"
)

func StreamTokensToFrames(ctx context.Context, streamID string, ptsBase int64, tokens <-chan string, out chan frames.Frame) {
	for {
		select {
		case <-ctx.Done():
			return
		case tok, ok := <-tokens:
			if !ok {
				return
			}
			tf := frames.NewTextFrame(streamID, ptsBase, tok, nil)
			select {
			case out <- tf:
			default:
			}
		}
	}
}
