package runevents

import (
	"context"

	runeventv1 "code-code.internal/go-contract/platform/run_event/v1"
)

type Request struct {
	SessionID     string
	RunID         string
	AfterSequence uint64
}

type StreamEvent struct {
	Delta  *runeventv1.RunDeltaEvent
	Result *runeventv1.RunResultEvent
}

type Reader interface {
	Stream(context.Context, Request, func(StreamEvent) error) error
	Close()
}
