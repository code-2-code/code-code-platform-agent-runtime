package timeline

import (
	"context"
	"errors"
	"testing"
	"time"

	platformcontract "code-code.internal/platform-contract"
)

type fakePublisher struct {
	stageCount int
	eventCount int
	err        error
}

func (f *fakePublisher) PublishStageInterval(context.Context, *platformcontract.StageInterval) error {
	f.stageCount++
	return f.err
}

func (f *fakePublisher) PublishEvent(context.Context, *platformcontract.TimelineEvent) error {
	f.eventCount++
	return f.err
}

type fakeProjector struct {
	stageCount int
	eventCount int
	err        error
}

func (f *fakeProjector) ObserveStageInterval(context.Context, *platformcontract.StageInterval) error {
	f.stageCount++
	return f.err
}

func (f *fakeProjector) ObserveEvent(context.Context, *platformcontract.TimelineEvent) error {
	f.eventCount++
	return f.err
}

func TestNewSinkRejectsNilPublisher(t *testing.T) {
	_, err := newSink(nil, nil, nil)
	if err == nil {
		t.Fatal("newSink() expected error, got nil")
	}
}

func TestSinkPublishesEventAndSwallowsProjectorFailures(t *testing.T) {
	publisher := &fakePublisher{}
	projector := &fakeProjector{err: errors.New("project failed")}
	sink, err := newSink(publisher, projector, nil)
	if err != nil {
		t.Fatalf("newSink() error = %v", err)
	}

	err = sink.RecordEvent(context.Background(), &platformcontract.TimelineEvent{
		ScopeRef: platformcontract.TimelineScopeRef{
			Scope:     platformcontract.TimelineScopeSession,
			SessionID: "session-1",
		},
		EventType:  "CREATED",
		Subject:    "session",
		Action:     "reconcile",
		OccurredAt: time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("RecordEvent() error = %v", err)
	}
	if publisher.eventCount != 1 {
		t.Fatalf("publisher count = %d, want 1", publisher.eventCount)
	}
	if projector.eventCount != 1 {
		t.Fatalf("projector count = %d, want 1", projector.eventCount)
	}
}

func TestSinkReturnsPublisherFailures(t *testing.T) {
	publisher := &fakePublisher{err: errors.New("publish failed")}
	sink, err := newSink(publisher, nil, nil)
	if err != nil {
		t.Fatalf("newSink() error = %v", err)
	}

	err = sink.RecordStageInterval(context.Background(), &platformcontract.StageInterval{
		ScopeRef: platformcontract.TimelineScopeRef{
			Scope:     platformcontract.TimelineScopeSession,
			SessionID: "session-1",
		},
		Stage:     "EXECUTE",
		Subject:   "run",
		Action:    "workflow",
		Status:    platformcontract.TimelineStageStatusSucceeded,
		StartedAt: time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC),
		EndedAt:   timePtr(time.Date(2026, 4, 14, 12, 0, 1, 0, time.UTC)),
	})
	if err == nil {
		t.Fatal("RecordStageInterval() expected publish error, got nil")
	}
}

func timePtr(value time.Time) *time.Time {
	return &value
}
