package platform

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// TimelineScope identifies one stable timeline scope.
type TimelineScope string

const (
	TimelineScopeSession TimelineScope = "session"
	TimelineScopeTurn    TimelineScope = "turn"
)

// TimelineStageStatus identifies one stable stage completion status.
type TimelineStageStatus string

const (
	TimelineStageStatusActive    TimelineStageStatus = "ACTIVE"
	TimelineStageStatusSucceeded TimelineStageStatus = "SUCCEEDED"
	TimelineStageStatusFailed    TimelineStageStatus = "FAILED"
	TimelineStageStatusCanceled  TimelineStageStatus = "CANCELED"
	TimelineStageStatusSkipped   TimelineStageStatus = "SKIPPED"
)

// TimelineScopeRef identifies the business scope owning one timeline record.
type TimelineScopeRef struct {
	Scope     TimelineScope
	SessionID string
	TurnID    string
}

// StageInterval describes one duration-bearing timeline record.
type StageInterval struct {
	ScopeRef   TimelineScopeRef
	Stage      string
	Subject    string
	Action     string
	Status     TimelineStageStatus
	StartedAt  time.Time
	EndedAt    *time.Time
	Attributes map[string]string
}

// TimelineEvent describes one instant timeline record.
type TimelineEvent struct {
	ScopeRef   TimelineScopeRef
	EventType  string
	Subject    string
	Action     string
	OccurredAt time.Time
	Attributes map[string]string
}

// TimelinePublisher publishes timeline records to a realtime event bus.
type TimelinePublisher interface {
	PublishStageInterval(ctx context.Context, interval *StageInterval) error
	PublishEvent(ctx context.Context, event *TimelineEvent) error
}

// TimelineMetricsProjector projects timeline records to metrics.
type TimelineMetricsProjector interface {
	ObserveStageInterval(ctx context.Context, interval *StageInterval) error
	ObserveEvent(ctx context.Context, event *TimelineEvent) error
}

// ValidateTimelineScopeRef validates one scope reference.
func ValidateTimelineScopeRef(scope TimelineScopeRef) error {
	if strings.TrimSpace(scope.SessionID) == "" {
		return fmt.Errorf("platform: timeline session id is empty")
	}
	switch scope.Scope {
	case TimelineScopeSession:
		if strings.TrimSpace(scope.TurnID) != "" {
			return fmt.Errorf("platform: timeline turn id must be empty for session scope")
		}
	case TimelineScopeTurn:
		if strings.TrimSpace(scope.TurnID) == "" {
			return fmt.Errorf("platform: timeline turn id is empty for turn scope")
		}
	default:
		return fmt.Errorf("platform: timeline scope %q is invalid", scope.Scope)
	}
	return nil
}

// ValidateStageInterval validates one stage interval record.
func ValidateStageInterval(interval *StageInterval) error {
	if interval == nil {
		return fmt.Errorf("platform: stage interval is nil")
	}
	if err := ValidateTimelineScopeRef(interval.ScopeRef); err != nil {
		return err
	}
	if strings.TrimSpace(interval.Stage) == "" {
		return fmt.Errorf("platform: stage interval stage is empty")
	}
	if strings.TrimSpace(interval.Subject) == "" {
		return fmt.Errorf("platform: stage interval subject is empty")
	}
	if strings.TrimSpace(interval.Action) == "" {
		return fmt.Errorf("platform: stage interval action is empty")
	}
	if interval.StartedAt.IsZero() {
		return fmt.Errorf("platform: stage interval startedAt is zero")
	}
	switch interval.Status {
	case TimelineStageStatusActive:
		if interval.EndedAt != nil {
			return fmt.Errorf("platform: active stage interval must not set endedAt")
		}
	case TimelineStageStatusSucceeded, TimelineStageStatusFailed, TimelineStageStatusCanceled, TimelineStageStatusSkipped:
		if interval.EndedAt == nil {
			return fmt.Errorf("platform: terminal stage interval endedAt is required")
		}
		if interval.EndedAt.Before(interval.StartedAt) {
			return fmt.Errorf("platform: stage interval endedAt is before startedAt")
		}
	default:
		return fmt.Errorf("platform: stage interval status %q is invalid", interval.Status)
	}
	for key := range interval.Attributes {
		if strings.TrimSpace(key) == "" {
			return fmt.Errorf("platform: stage interval attribute key is empty")
		}
	}
	return nil
}

// ValidateTimelineEvent validates one instant timeline record.
func ValidateTimelineEvent(event *TimelineEvent) error {
	if event == nil {
		return fmt.Errorf("platform: timeline event is nil")
	}
	if err := ValidateTimelineScopeRef(event.ScopeRef); err != nil {
		return err
	}
	if strings.TrimSpace(event.EventType) == "" {
		return fmt.Errorf("platform: timeline event type is empty")
	}
	if strings.TrimSpace(event.Subject) == "" {
		return fmt.Errorf("platform: timeline event subject is empty")
	}
	if strings.TrimSpace(event.Action) == "" {
		return fmt.Errorf("platform: timeline event action is empty")
	}
	if event.OccurredAt.IsZero() {
		return fmt.Errorf("platform: timeline event occurredAt is zero")
	}
	for key := range event.Attributes {
		if strings.TrimSpace(key) == "" {
			return fmt.Errorf("platform: timeline event attribute key is empty")
		}
	}
	return nil
}
