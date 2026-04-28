package agentsessionactions

import (
	"strings"
	"time"

	platformv1alpha1 "code-code.internal/platform-k8s/api/v1alpha1"
	ctrl "sigs.k8s.io/controller-runtime"
)

func pendingRetryWindow(resource *platformv1alpha1.AgentSessionActionResource, now time.Time) (*platformv1alpha1.AgentSessionActionResourceStatus, ctrl.Result, bool) {
	if resource == nil ||
		resource.Status.Phase != platformv1alpha1.AgentSessionActionResourcePhasePending ||
		resource.Status.FailureClass != platformv1alpha1.AgentSessionActionResourceFailureClassTransient ||
		resource.Status.NextRetryAt == nil {
		return nil, ctrl.Result{}, false
	}
	until := resource.Status.NextRetryAt.Time.Sub(now)
	if until <= 0 {
		return nil, ctrl.Result{}, false
	}
	return buildStatus(resource, now, actionStatusSpec{
		phase:          platformv1alpha1.AgentSessionActionResourcePhasePending,
		message:        resource.Status.Message,
		failureClass:   platformv1alpha1.AgentSessionActionResourceFailureClassTransient,
		retryCount:     resource.Status.RetryCount,
		attemptCount:   resource.Status.AttemptCount,
		candidateIndex: resource.Status.CandidateIndex,
		nextRetryAt:    resource.Status.NextRetryAt,
	}), ctrl.Result{RequeueAfter: until}, true
}

func scheduleRetryStatus(
	resource *platformv1alpha1.AgentSessionActionResource,
	now time.Time,
	message string,
	exhaustedClass platformv1alpha1.AgentSessionActionResourceFailureClass,
	policy RetryPolicy,
) (*platformv1alpha1.AgentSessionActionResourceStatus, ctrl.Result) {
	retryCount := resource.Status.RetryCount + 1
	if retryCount > policy.MaxRetries {
		resourceCopy := resource.DeepCopy()
		resourceCopy.Status.RetryCount = policy.MaxRetries
		return exhaustedRetryStatus(resourceCopy, now, message, exhaustedClass), ctrl.Result{}
	}
	delay := retryDelay(retryCount, policy)
	return buildStatus(resource, now, actionStatusSpec{
		phase:          platformv1alpha1.AgentSessionActionResourcePhasePending,
		message:        automaticRetryMessage(message),
		failureClass:   platformv1alpha1.AgentSessionActionResourceFailureClassTransient,
		retryCount:     retryCount,
		attemptCount:   resource.Status.AttemptCount,
		candidateIndex: resource.Status.CandidateIndex,
		nextRetryAt:    timePtr(now.Add(delay)),
	}), ctrl.Result{RequeueAfter: delay}
}

func scheduleFallbackStatus(resource *platformv1alpha1.AgentSessionActionResource, now time.Time, message string, nextCandidateIndex int32) *platformv1alpha1.AgentSessionActionResourceStatus {
	return buildStatus(resource, now, actionStatusSpec{
		phase:          platformv1alpha1.AgentSessionActionResourcePhasePending,
		message:        automaticFallbackMessage(message),
		failureClass:   platformv1alpha1.AgentSessionActionResourceFailureClassTransient,
		retryCount:     0,
		attemptCount:   resource.Status.AttemptCount,
		candidateIndex: nextCandidateIndex,
	})
}

func retryBudgetAvailable(resource *platformv1alpha1.AgentSessionActionResource, policy RetryPolicy) bool {
	if resource == nil {
		return false
	}
	return resource.Status.RetryCount < policy.MaxRetries
}

func retryDelay(retryCount int32, policy RetryPolicy) time.Duration {
	if retryCount <= 0 {
		return policy.BaseBackoff
	}
	delay := policy.BaseBackoff
	for attempt := int32(1); attempt < retryCount; attempt++ {
		delay *= 2
		if delay >= policy.MaxBackoff {
			return policy.MaxBackoff
		}
	}
	if delay > policy.MaxBackoff {
		return policy.MaxBackoff
	}
	return delay
}

func automaticRetryMessage(message string) string {
	message = sanitizeActionMessage(message)
	if message == "" {
		return "AgentSessionAction automatic retry is scheduled."
	}
	return "AgentSessionAction automatic retry is scheduled: " + message
}

func exhaustedRetryMessage(message string) string {
	message = sanitizeActionMessage(message)
	if message == "" {
		return "AgentSessionAction automatic retry budget was exhausted."
	}
	return "AgentSessionAction automatic retry budget was exhausted: " + message
}

func automaticFallbackMessage(message string) string {
	message = sanitizeActionMessage(message)
	if message == "" {
		return "AgentSessionAction automatic fallback is scheduled."
	}
	return "AgentSessionAction automatic fallback is scheduled: " + message
}

func sanitizeActionMessage(message string) string {
	message = strings.TrimSpace(message)
	for _, prefix := range []string{
		"platformk8s/agentsessionactions: ",
		"platformk8s/agentsessions: ",
		"platformk8s/agentruns: ",
		"platformk8s/agentexecution: ",
	} {
		message = strings.TrimPrefix(message, prefix)
	}
	return strings.TrimSpace(message)
}
