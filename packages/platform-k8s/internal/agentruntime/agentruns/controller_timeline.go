package agentruns

import (
	"strings"
	"time"

	platformcontract "code-code.internal/platform-contract"
	platformv1alpha1 "code-code.internal/platform-k8s/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type timelineTransitions struct {
	intervals []*platformcontract.StageInterval
	events    []*platformcontract.TimelineEvent
}

func runTimelineTransitions(resource *platformv1alpha1.AgentRunResource, previous *platformv1alpha1.AgentRunResourceStatus, next *platformv1alpha1.AgentRunResourceStatus, workflowState *WorkflowState) timelineTransitions {
	if resource == nil || resource.Spec.Run == nil || strings.TrimSpace(resource.Spec.Run.GetSessionId()) == "" || strings.TrimSpace(resource.Spec.Run.GetRunId()) == "" || next == nil {
		return timelineTransitions{}
	}
	scope := platformcontract.TimelineScopeRef{
		Scope:     platformcontract.TimelineScopeSession,
		SessionID: resource.Spec.Run.GetSessionId(),
	}
	attributes := map[string]string{
		"run_id": resource.Spec.Run.GetRunId(),
		"phase":  string(next.Phase),
	}
	if workloadID := strings.TrimSpace(next.WorkloadID); workloadID != "" {
		attributes["workload_id"] = workloadID
	}
	transitions := timelineTransitions{}
	if next.Phase == platformv1alpha1.AgentRunResourcePhaseScheduled && (previous == nil || previous.Phase != platformv1alpha1.AgentRunResourcePhaseScheduled) {
		transitions.events = append(transitions.events, &platformcontract.TimelineEvent{
			ScopeRef:   scope,
			EventType:  "SCHEDULED",
			Subject:    "run",
			Action:     "workflow",
			OccurredAt: eventTime(nil, next.UpdatedAt),
			Attributes: cloneAttributes(attributes),
		})
	}
	if next.Phase == platformv1alpha1.AgentRunResourcePhaseRunning && (previous == nil || previous.Phase != platformv1alpha1.AgentRunResourcePhaseRunning) {
		transitions.events = append(transitions.events, &platformcontract.TimelineEvent{
			ScopeRef:   scope,
			EventType:  "STARTED",
			Subject:    "run",
			Action:     "workflow",
			OccurredAt: eventTime(timeFromWorkflow(workflowState, true), next.UpdatedAt),
			Attributes: cloneAttributes(attributes),
		})
	}
	if isTerminalPhase(next.Phase) && (previous == nil || !isTerminalPhase(previous.Phase)) {
		transitions.events = append(transitions.events, &platformcontract.TimelineEvent{
			ScopeRef:   scope,
			EventType:  "FINISHED",
			Subject:    "run",
			Action:     "workflow",
			OccurredAt: eventTime(timeFromWorkflow(workflowState, false), next.UpdatedAt),
			Attributes: cloneAttributes(attributes),
		})
		if interval := executeStageInterval(scope, previous, next, workflowState, attributes); interval != nil {
			transitions.intervals = append(transitions.intervals, interval)
		}
	}
	return transitions
}

func executeStageInterval(scope platformcontract.TimelineScopeRef, previous *platformv1alpha1.AgentRunResourceStatus, next *platformv1alpha1.AgentRunResourceStatus, workflowState *WorkflowState, attributes map[string]string) *platformcontract.StageInterval {
	startedAt := timeFromWorkflow(workflowState, true)
	if startedAt == nil {
		startedAt = conditionTransitionTime(next, string(platformcontract.AgentRunConditionTypeWorkloadReady), string(platformcontract.AgentRunConditionReasonRunStarted))
	}
	if startedAt == nil {
		startedAt = conditionTransitionTime(previous, string(platformcontract.AgentRunConditionTypeWorkloadReady), string(platformcontract.AgentRunConditionReasonRunStarted))
	}
	endedAt := timeFromWorkflow(workflowState, false)
	if endedAt == nil {
		endedAt = timePtrValue(next.UpdatedAt)
	}
	if startedAt == nil || endedAt == nil || endedAt.Before(*startedAt) {
		return nil
	}
	return &platformcontract.StageInterval{
		ScopeRef:   scope,
		Stage:      "EXECUTE",
		Subject:    "run",
		Action:     "workflow",
		Status:     timelineStageStatusFor(next.Phase),
		StartedAt:  *startedAt,
		EndedAt:    endedAt,
		Attributes: cloneAttributes(attributes),
	}
}

func conditionTransitionTime(status *platformv1alpha1.AgentRunResourceStatus, conditionType string, reason string) *time.Time {
	if status == nil {
		return nil
	}
	for _, c := range status.Conditions {
		if c.Type != conditionType || c.Reason != reason {
			continue
		}
		if c.LastTransitionTime.IsZero() {
			continue
		}
		value := c.LastTransitionTime.UTC()
		return &value
	}
	return nil
}

func eventTime(workflowTime *time.Time, updatedAt *metav1.Time) time.Time {
	if workflowTime != nil {
		return workflowTime.UTC()
	}
	if updatedAt == nil {
		return time.Time{}
	}
	return updatedAt.UTC()
}

func timePtrValue(value *metav1.Time) *time.Time {
	if value == nil {
		return nil
	}
	out := value.UTC()
	return &out
}

func timeFromWorkflow(state *WorkflowState, started bool) *time.Time {
	if state == nil {
		return nil
	}
	if started {
		if state.StartedAt == nil {
			return nil
		}
		value := state.StartedAt.UTC()
		return &value
	}
	if state.FinishedAt == nil {
		return nil
	}
	value := state.FinishedAt.UTC()
	return &value
}

func timelineStageStatusFor(phase platformv1alpha1.AgentRunResourcePhase) platformcontract.TimelineStageStatus {
	switch phase {
	case platformv1alpha1.AgentRunResourcePhaseSucceeded:
		return platformcontract.TimelineStageStatusSucceeded
	case platformv1alpha1.AgentRunResourcePhaseCanceled:
		return platformcontract.TimelineStageStatusCanceled
	default:
		return platformcontract.TimelineStageStatusFailed
	}
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
