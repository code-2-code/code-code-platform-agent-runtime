package agentsessionactions

import (
	"strings"

	agentsessionactionv1 "code-code.internal/go-contract/platform/agent_session_action/v1"
	"code-code.internal/platform-k8s/api/v1alpha1"
	"code-code.internal/platform-k8s/internal/platform/phaseconv"
	"code-code.internal/platform-k8s/internal/platform/protostate"
)

func actionStateFromResource(resource *v1alpha1.AgentSessionActionResource) (*agentsessionactionv1.AgentSessionActionState, error) {
	if resource == nil || resource.Spec.Action == nil {
		return nil, validation("action resource is invalid")
	}
	spec := cloneActionSpec(resource.Spec.Action)
	if spec.GetActionId() == "" {
		spec.ActionId = resource.Name
	}
	return &agentsessionactionv1.AgentSessionActionState{
		Generation: resource.Generation,
		Spec:       spec,
		Status: &agentsessionactionv1.AgentSessionActionStatus{
			ActionId:           spec.GetActionId(),
			Phase:              phaseconv.FromK8sActionPhase(resource.Status.Phase),
			ObservedGeneration: resource.Status.ObservedGeneration,
			Message:            resource.Status.Message,
			Run:                actionRunRef(resource.Status.RunID),
			CreatedAt:          protostate.Timestamp(createdAt(resource)),
			UpdatedAt:          protostate.Timestamp(resource.Status.UpdatedAt),
			FailureClass:       phaseconv.FromK8sActionFailureClass(resource.Status.FailureClass),
			RetryCount:         resource.Status.RetryCount,
			NextRetryAt:        protostate.Timestamp(resource.Status.NextRetryAt),
			View:               actionViewFromResource(resource),
			AttemptCount:       resource.Status.AttemptCount,
			CandidateIndex:     resource.Status.CandidateIndex,
		},
	}, nil
}

func actionRunRef(runID string) *agentsessionactionv1.AgentSessionActionRunRef {
	if strings.TrimSpace(runID) == "" {
		return nil
	}
	return &agentsessionactionv1.AgentSessionActionRunRef{RunId: strings.TrimSpace(runID)}
}
