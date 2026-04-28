package agentsessions

import (
	"context"
	"strings"

	platformv1alpha1 "code-code.internal/platform-k8s/api/v1alpha1"
	"google.golang.org/protobuf/proto"
)

func (r *Reconciler) syncProfileBackedSession(ctx context.Context, resource *platformv1alpha1.AgentSessionResource) (bool, error) {
	if resource == nil || resource.Spec.Session == nil || strings.TrimSpace(resource.Spec.Session.GetProfileId()) == "" {
		return false, nil
	}
	if r == nil || r.projector == nil {
		return false, validation("profile projector is unavailable")
	}
	projected, err := r.projector.Project(ctx, resource.Spec.Session)
	if err != nil {
		return false, err
	}
	if proto.Equal(projected, resource.Spec.Session) {
		return false, nil
	}
	sessionID := firstNonEmpty(resource.Spec.Session.GetSessionId(), resource.GetName())
	if _, err := r.sessions.Update(ctx, sessionID, projected); err != nil {
		return false, err
	}
	return true, nil
}
