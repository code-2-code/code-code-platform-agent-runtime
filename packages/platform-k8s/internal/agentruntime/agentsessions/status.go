package agentsessions

import (
	"context"
	"reflect"

	agentsessionv1 "code-code.internal/go-contract/platform/agent_session/v1"
	platformv1alpha1 "code-code.internal/platform-k8s/api/v1alpha1"
)

type SessionRepository interface {
	Get(context.Context, string) (*agentsessionv1.AgentSessionState, error)
	Update(context.Context, string, *agentsessionv1.AgentSessionSpec) (*agentsessionv1.AgentSessionState, error)
	UpdateStatus(context.Context, string, *agentsessionv1.AgentSessionStatus) (*agentsessionv1.AgentSessionState, error)
	ClaimActiveRun(context.Context, string, string) (*agentsessionv1.AgentSessionState, error)
	ReleaseActiveRun(context.Context, string, string) (bool, error)
}

func (r *Reconciler) updateStatus(ctx context.Context, resource *platformv1alpha1.AgentSessionResource, next *platformv1alpha1.AgentSessionResourceStatus) error {
	status := sessionStatusFromResourceStatus(resource, next)
	_, err := r.sessions.UpdateStatus(ctx, status.GetSessionId(), status)
	return err
}

func statusSemanticallyEqual(previous *platformv1alpha1.AgentSessionResourceStatus, next *platformv1alpha1.AgentSessionResourceStatus) bool {
	return reflect.DeepEqual(normalizedStatus(previous), normalizedStatus(next))
}

func normalizedStatus(status *platformv1alpha1.AgentSessionResourceStatus) *platformv1alpha1.AgentSessionResourceStatus {
	if status == nil {
		return nil
	}
	out := status.DeepCopy()
	out.UpdatedAt = nil
	return out
}
