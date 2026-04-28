package agentsessions

import (
	"context"
	"strings"

	platformv1alpha1 "code-code.internal/platform-k8s/api/v1alpha1"
	"code-code.internal/platform-k8s/internal/platform/resourceops"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func (m *CarrierManager) ensureCurrentPVCs(ctx context.Context, session *platformv1alpha1.AgentSessionResource) error {
	if workspaceID := strings.TrimSpace(session.Spec.Session.GetWorkspaceRef().GetWorkspaceId()); workspaceID != "" {
		if err := m.ensureCarrierPVC(ctx, session, carrierKindWorkspace, workspaceID); err != nil {
			return err
		}
	}
	if homeStateID := strings.TrimSpace(session.Spec.Session.GetHomeStateRef().GetHomeStateId()); homeStateID != "" {
		if err := m.ensureCarrierPVC(ctx, session, carrierKindHomeState, homeStateID); err != nil {
			return err
		}
	}
	return nil
}

func (m *CarrierManager) ensureCarrierPVC(ctx context.Context, session *platformv1alpha1.AgentSessionResource, kind carrierKind, carrierID string) error {
	next := buildCarrierPVC(session, m.runtimeNamespace, kind, carrierID)
	key := ctrlclient.ObjectKey{Namespace: m.runtimeNamespace, Name: next.GetName()}
	current := &corev1.PersistentVolumeClaim{}
	if err := m.client.Get(ctx, key, current); err != nil {
		if apierrors.IsNotFound(err) {
			return resourceops.CreateResource(ctx, m.client, next, m.runtimeNamespace, next.GetName())
		}
		return err
	}
	return nil
}
