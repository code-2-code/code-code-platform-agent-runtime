package agentsessionactions

import (
	"context"
	"strconv"
	"strings"
	"time"

	platformcontract "code-code.internal/platform-contract"
	platformv1alpha1 "code-code.internal/platform-k8s/api/v1alpha1"
)

func (r *Reconciler) recordTimelineTransitions(ctx context.Context, events []*platformcontract.TimelineEvent) {
	if r == nil || r.sink == nil {
		return
	}
	for _, event := range events {
		if err := r.sink.RecordEvent(ctx, event); err != nil {
			r.logger.Error("agentSessionAction timeline event record failed", "error", err, "eventType", event.EventType)
		}
	}
}

func actionTimelineTransitions(resource *platformv1alpha1.AgentSessionActionResource, previous *platformv1alpha1.AgentSessionActionResourceStatus, next *platformv1alpha1.AgentSessionActionResourceStatus) []*platformcontract.TimelineEvent {
	if resource == nil || resource.Spec.Action == nil || next == nil {
		return nil
	}
	scope := platformcontract.TimelineScopeRef{
		Scope:     platformcontract.TimelineScopeSession,
		SessionID: resource.Spec.Action.GetSessionId(),
	}
	attributes := map[string]string{
		"action_id": resource.Spec.Action.GetActionId(),
		"type":      strings.TrimSpace(resource.Spec.Action.GetType().String()),
		"phase":     string(next.Phase),
	}
	if failureClass := strings.TrimSpace(string(next.FailureClass)); failureClass != "" {
		attributes["failure_class"] = failureClass
	}
	if next.RetryCount > 0 {
		attributes["retry_count"] = strconv.FormatInt(int64(next.RetryCount), 10)
	}
	if next.AttemptCount > 0 {
		attributes["attempt_count"] = strconv.FormatInt(int64(next.AttemptCount), 10)
	}
	if next.CandidateIndex > 0 {
		attributes["candidate_index"] = strconv.FormatInt(int64(next.CandidateIndex), 10)
	}
	if runID := strings.TrimSpace(next.RunID); runID != "" {
		attributes["run_id"] = runID
	}
	if nextRetryAt := next.NextRetryAt; nextRetryAt != nil && !nextRetryAt.IsZero() {
		attributes["next_retry_at"] = nextRetryAt.UTC().Format(time.RFC3339)
	}
	events := make([]*platformcontract.TimelineEvent, 0, 2)
	if retryScheduled(previous, next) {
		events = append(events, &platformcontract.TimelineEvent{
			ScopeRef:   scope,
			EventType:  "RETRY_SCHEDULED",
			Subject:    "action",
			Action:     "retry",
			OccurredAt: next.UpdatedAt.UTC(),
			Attributes: cloneAttributes(attributes),
		})
	}
	if fallbackScheduled(previous, next) {
		events = append(events, &platformcontract.TimelineEvent{
			ScopeRef:   scope,
			EventType:  "FALLBACK_SCHEDULED",
			Subject:    "action",
			Action:     "fallback",
			OccurredAt: next.UpdatedAt.UTC(),
			Attributes: cloneAttributes(attributes),
		})
	}
	if next.Phase == platformv1alpha1.AgentSessionActionResourcePhaseRunning && (previous == nil || previous.Phase != platformv1alpha1.AgentSessionActionResourcePhaseRunning) {
		events = append(events, &platformcontract.TimelineEvent{
			ScopeRef:   scope,
			EventType:  "STARTED",
			Subject:    "action",
			Action:     "dispatch",
			OccurredAt: next.UpdatedAt.UTC(),
			Attributes: cloneAttributes(attributes),
		})
	}
	if isTerminalPhase(next.Phase) && (previous == nil || !isTerminalPhase(previous.Phase)) {
		action := "complete"
		if strings.TrimSpace(next.RunID) != "" {
			action = "observe_run"
		}
		events = append(events, &platformcontract.TimelineEvent{
			ScopeRef:   scope,
			EventType:  "FINISHED",
			Subject:    "action",
			Action:     action,
			OccurredAt: next.UpdatedAt.UTC(),
			Attributes: cloneAttributes(attributes),
		})
	}
	return events
}

func fallbackScheduled(previous *platformv1alpha1.AgentSessionActionResourceStatus, next *platformv1alpha1.AgentSessionActionResourceStatus) bool {
	if next == nil || next.Phase != platformv1alpha1.AgentSessionActionResourcePhasePending || next.CandidateIndex <= 0 {
		return false
	}
	if previous == nil {
		return true
	}
	return next.CandidateIndex > previous.CandidateIndex
}

func retryScheduled(previous *platformv1alpha1.AgentSessionActionResourceStatus, next *platformv1alpha1.AgentSessionActionResourceStatus) bool {
	if next == nil || next.FailureClass != platformv1alpha1.AgentSessionActionResourceFailureClassTransient || next.NextRetryAt == nil || next.NextRetryAt.IsZero() {
		return false
	}
	if previous == nil {
		return true
	}
	if previous.RetryCount != next.RetryCount {
		return true
	}
	if previous.NextRetryAt == nil || previous.NextRetryAt.IsZero() {
		return true
	}
	return !previous.NextRetryAt.Equal(next.NextRetryAt)
}

func cloneAttributes(attributes map[string]string) map[string]string {
	if len(attributes) == 0 {
		return nil
	}
	out := make(map[string]string, len(attributes))
	for key, value := range attributes {
		out[key] = value
	}
	return out
}
