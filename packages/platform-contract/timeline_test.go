package platform

import (
	"testing"
	"time"
)

func TestValidateTimelineScopeRefAcceptsSessionScope(t *testing.T) {
	err := ValidateTimelineScopeRef(TimelineScopeRef{
		Scope:     TimelineScopeSession,
		SessionID: "session-1",
	})
	if err != nil {
		t.Fatalf("ValidateTimelineScopeRef() error = %v", err)
	}
}

func TestValidateTimelineScopeRefRejectsTurnScopeWithoutTurnID(t *testing.T) {
	err := ValidateTimelineScopeRef(TimelineScopeRef{
		Scope:     TimelineScopeTurn,
		SessionID: "session-1",
	})
	if err == nil {
		t.Fatal("ValidateTimelineScopeRef() expected error, got nil")
	}
}

func TestValidateStageIntervalAcceptsTerminalInterval(t *testing.T) {
	startedAt := time.Date(2026, 4, 14, 10, 0, 0, 0, time.UTC)
	endedAt := startedAt.Add(2 * time.Second)
	err := ValidateStageInterval(&StageInterval{
		ScopeRef: TimelineScopeRef{
			Scope:     TimelineScopeTurn,
			SessionID: "session-1",
			TurnID:    "turn-1",
		},
		Stage:     "EXECUTE",
		Subject:   "run",
		Action:    "execute",
		Status:    TimelineStageStatusSucceeded,
		StartedAt: startedAt,
		EndedAt:   &endedAt,
		Attributes: map[string]string{
			"run_id": "run-1",
		},
	})
	if err != nil {
		t.Fatalf("ValidateStageInterval() error = %v", err)
	}
}

func TestValidateStageIntervalRejectsActiveWithEndedAt(t *testing.T) {
	startedAt := time.Date(2026, 4, 14, 10, 0, 0, 0, time.UTC)
	endedAt := startedAt.Add(time.Second)
	err := ValidateStageInterval(&StageInterval{
		ScopeRef: TimelineScopeRef{
			Scope:     TimelineScopeSession,
			SessionID: "session-1",
		},
		Stage:     "PREPARE",
		Subject:   "endpoint",
		Action:    "refresh",
		Status:    TimelineStageStatusActive,
		StartedAt: startedAt,
		EndedAt:   &endedAt,
	})
	if err == nil {
		t.Fatal("ValidateStageInterval() expected error, got nil")
	}
}

func TestValidateTimelineEventAcceptsEvent(t *testing.T) {
	err := ValidateTimelineEvent(&TimelineEvent{
		ScopeRef: TimelineScopeRef{
			Scope:     TimelineScopeTurn,
			SessionID: "session-1",
			TurnID:    "turn-1",
		},
		EventType:  "RESULT_COMMITTED",
		Subject:    "message",
		Action:     "persist",
		OccurredAt: time.Date(2026, 4, 14, 10, 0, 0, 0, time.UTC),
		Attributes: map[string]string{
			"message_id": "message-1",
		},
	})
	if err != nil {
		t.Fatalf("ValidateTimelineEvent() error = %v", err)
	}
}

func TestValidateTimelineEventRejectsEmptyEventType(t *testing.T) {
	err := ValidateTimelineEvent(&TimelineEvent{
		ScopeRef: TimelineScopeRef{
			Scope:     TimelineScopeSession,
			SessionID: "session-1",
		},
		Subject:    "session",
		Action:     "reconcile",
		OccurredAt: time.Date(2026, 4, 14, 10, 0, 0, 0, time.UTC),
	})
	if err == nil {
		t.Fatal("ValidateTimelineEvent() expected error, got nil")
	}
}
