package agentruns

import (
	"context"

	platformcontract "code-code.internal/platform-contract"
)

type fakeTimelineSink struct {
	events    []*platformcontract.TimelineEvent
	intervals []*platformcontract.StageInterval
}

func (f *fakeTimelineSink) RecordStageInterval(_ context.Context, interval *platformcontract.StageInterval) error {
	f.intervals = append(f.intervals, interval)
	return nil
}

func (f *fakeTimelineSink) RecordEvent(_ context.Context, event *platformcontract.TimelineEvent) error {
	f.events = append(f.events, event)
	return nil
}

func (*fakeTimelineSink) Close() {}
