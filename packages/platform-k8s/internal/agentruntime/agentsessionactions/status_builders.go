package agentsessionactions

import (
	"strings"
	"time"

	platformv1alpha1 "code-code.internal/platform-k8s/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type actionStatusSpec struct {
	phase          platformv1alpha1.AgentSessionActionResourcePhase
	runID          string
	message        string
	failureClass   platformv1alpha1.AgentSessionActionResourceFailureClass
	retryCount     int32
	attemptCount   int32
	candidateIndex int32
	nextRetryAt    *metav1.Time
}

func invalidStatus(resource *platformv1alpha1.AgentSessionActionResource, now time.Time) *platformv1alpha1.AgentSessionActionResourceStatus {
	if resource == nil {
		return invalidActionStatus(now, "AgentSessionAction resource is required.")
	}
	if resource.Spec.Action == nil {
		return failedStatus(resource, now, "AgentSessionAction spec.action is required.")
	}
	if strings.TrimSpace(resource.Spec.Action.GetActionId()) == "" {
		return failedStatus(resource, now, "AgentSessionAction actionId is required.")
	}
	if strings.TrimSpace(resource.Spec.Action.GetSessionId()) == "" {
		return failedStatus(resource, now, "AgentSessionAction sessionId is required.")
	}
	return nil
}

func invalidActionStatus(now time.Time, message string) *platformv1alpha1.AgentSessionActionResourceStatus {
	return &platformv1alpha1.AgentSessionActionResourceStatus{
		CommonStatusFields: platformv1alpha1.CommonStatusFields{
			ObservedGeneration: 0,
		},
		Phase:        platformv1alpha1.AgentSessionActionResourcePhaseFailed,
		FailureClass: platformv1alpha1.AgentSessionActionResourceFailureClassPermanent,
		Message:      strings.TrimSpace(message),
		UpdatedAt:    timePtr(now),
	}
}

func pendingBlockedStatus(resource *platformv1alpha1.AgentSessionActionResource, now time.Time, message string) *platformv1alpha1.AgentSessionActionResourceStatus {
	return buildStatus(resource, now, actionStatusSpec{
		phase:          platformv1alpha1.AgentSessionActionResourcePhasePending,
		message:        sanitizeActionMessage(message),
		failureClass:   platformv1alpha1.AgentSessionActionResourceFailureClassBlocked,
		retryCount:     resource.Status.RetryCount,
		attemptCount:   resource.Status.AttemptCount,
		candidateIndex: resource.Status.CandidateIndex,
	})
}

func runningStatus(resource *platformv1alpha1.AgentSessionActionResource, now time.Time, runID string, message string, attemptCount int32, candidateIndex int32) *platformv1alpha1.AgentSessionActionResourceStatus {
	if strings.TrimSpace(message) == "" {
		message = "AgentSessionAction run is in progress."
	}
	return buildStatus(resource, now, actionStatusSpec{
		phase:          platformv1alpha1.AgentSessionActionResourcePhaseRunning,
		runID:          runID,
		message:        sanitizeActionMessage(message),
		retryCount:     resource.Status.RetryCount,
		attemptCount:   attemptCount,
		candidateIndex: candidateIndex,
	})
}

func terminalStatus(
	resource *platformv1alpha1.AgentSessionActionResource,
	now time.Time,
	phase platformv1alpha1.AgentSessionActionResourcePhase,
	message string,
	failureClass platformv1alpha1.AgentSessionActionResourceFailureClass,
) *platformv1alpha1.AgentSessionActionResourceStatus {
	return buildStatus(resource, now, actionStatusSpec{
		phase:          phase,
		runID:          resource.Status.RunID,
		message:        sanitizeActionMessage(message),
		failureClass:   failureClass,
		retryCount:     resource.Status.RetryCount,
		attemptCount:   resource.Status.AttemptCount,
		candidateIndex: resource.Status.CandidateIndex,
	})
}

func canceledStatus(resource *platformv1alpha1.AgentSessionActionResource, now time.Time, message string) *platformv1alpha1.AgentSessionActionResourceStatus {
	return terminalStatus(resource, now, platformv1alpha1.AgentSessionActionResourcePhaseCanceled, message, "")
}

func failedStatus(resource *platformv1alpha1.AgentSessionActionResource, now time.Time, message string) *platformv1alpha1.AgentSessionActionResourceStatus {
	return buildStatus(resource, now, actionStatusSpec{
		phase:          platformv1alpha1.AgentSessionActionResourcePhaseFailed,
		message:        sanitizeActionMessage(message),
		failureClass:   platformv1alpha1.AgentSessionActionResourceFailureClassPermanent,
		retryCount:     resource.Status.RetryCount,
		attemptCount:   resource.Status.AttemptCount,
		candidateIndex: resource.Status.CandidateIndex,
	})
}

func exhaustedRetryStatus(
	resource *platformv1alpha1.AgentSessionActionResource,
	now time.Time,
	message string,
	failureClass platformv1alpha1.AgentSessionActionResourceFailureClass,
) *platformv1alpha1.AgentSessionActionResourceStatus {
	return buildStatus(resource, now, actionStatusSpec{
		phase:          platformv1alpha1.AgentSessionActionResourcePhaseFailed,
		message:        exhaustedRetryMessage(message),
		failureClass:   failureClass,
		retryCount:     resource.Status.RetryCount,
		attemptCount:   resource.Status.AttemptCount,
		candidateIndex: resource.Status.CandidateIndex,
	})
}

func buildStatus(resource *platformv1alpha1.AgentSessionActionResource, now time.Time, spec actionStatusSpec) *platformv1alpha1.AgentSessionActionResourceStatus {
	createdAt := createdAt(resource)
	if createdAt == nil || createdAt.IsZero() {
		createdAt = timePtr(now)
	}
	return &platformv1alpha1.AgentSessionActionResourceStatus{
		CommonStatusFields: platformv1alpha1.CommonStatusFields{
			ObservedGeneration: resource.Generation,
		},
		Phase:          spec.phase,
		FailureClass:   spec.failureClass,
		Message:        strings.TrimSpace(spec.message),
		RetryCount:     spec.retryCount,
		AttemptCount:   spec.attemptCount,
		CandidateIndex: spec.candidateIndex,
		NextRetryAt:    spec.nextRetryAt,
		RunID:          strings.TrimSpace(spec.runID),
		CreatedAt:      createdAt,
		UpdatedAt:      timePtr(now),
	}
}
