package agentsessions

import (
	"context"
	"fmt"
	"strings"

	agentrunv1 "code-code.internal/go-contract/platform/agent_run/v1"
	agentsessionactionv1 "code-code.internal/go-contract/platform/agent_session_action/v1"
	platformv1alpha1 "code-code.internal/platform-k8s/api/v1alpha1"
	"code-code.internal/platform-k8s/internal/platform/resourceops"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type carrierObservation struct {
	workspaceReady   bool
	workspaceMessage string
	warmStateReady   bool
	warmStateMessage string
}

type CarrierManager struct {
	client           ctrlclient.Client
	controlNamespace string
	runtimeNamespace string
	actions          SessionActionReader
}

const (
	actionSessionIDLabelKey = "agentsessionaction.code-code.internal/session-id"
	runSessionIDLabelKey    = "agentrun.code-code.internal/session-id"
)

func NewCarrierManager(client ctrlclient.Client, namespace string, runtimeNamespace ...string) (*CarrierManager, error) {
	if client == nil {
		return nil, validation("carrier manager client is nil")
	}
	namespace = strings.TrimSpace(namespace)
	if namespace == "" {
		return nil, validation("carrier manager namespace is empty")
	}
	runNamespace := namespace
	if len(runtimeNamespace) > 0 && strings.TrimSpace(runtimeNamespace[0]) != "" {
		runNamespace = strings.TrimSpace(runtimeNamespace[0])
	}
	return &CarrierManager{client: client, controlNamespace: namespace, runtimeNamespace: runNamespace}, nil
}

func (m *CarrierManager) Ensure(ctx context.Context, session *platformv1alpha1.AgentSessionResource) error {
	if session == nil || session.Spec.Session == nil {
		return nil
	}
	if err := m.ensureCurrentPVCs(ctx, session); err != nil {
		return err
	}
	return m.cleanupStalePVCs(ctx, session)
}

func (m *CarrierManager) Observe(ctx context.Context, session *platformv1alpha1.AgentSessionResource) (carrierObservation, error) {
	if session == nil || session.Spec.Session == nil {
		return carrierObservation{}, nil
	}
	workspaceReady, workspaceMessage, err := m.observeCarrier(ctx, session.GetName(), session.Spec.Session.GetWorkspaceRef().GetWorkspaceId(), carrierKindWorkspace, "Workspace reference is missing.")
	if err != nil {
		return carrierObservation{}, err
	}
	warmStateReady, warmStateMessage, err := m.observeCarrier(ctx, session.GetName(), session.Spec.Session.GetHomeStateRef().GetHomeStateId(), carrierKindHomeState, "Warm state reference is missing.")
	if err != nil {
		return carrierObservation{}, err
	}
	return carrierObservation{
		workspaceReady:   workspaceReady,
		workspaceMessage: workspaceMessage,
		warmStateReady:   warmStateReady,
		warmStateMessage: warmStateMessage,
	}, nil
}

func (m *CarrierManager) cleanupStalePVCs(ctx context.Context, session *platformv1alpha1.AgentSessionResource) error {
	keep, legacyRefs, err := m.referencedPVCNames(ctx, session)
	if err != nil || legacyRefs {
		return err
	}
	list := &corev1.PersistentVolumeClaimList{}
	if err := m.client.List(ctx, list,
		ctrlclient.InNamespace(m.runtimeNamespace),
		ctrlclient.MatchingLabels(map[string]string{
			carrierSessionIDLabel: strings.TrimSpace(session.GetName()),
			carrierManagedByLabel: carrierManagedByValue,
		}),
	); err != nil {
		return err
	}
	for i := range list.Items {
		if _, ok := keep[list.Items[i].GetName()]; ok {
			continue
		}
		if err := resourceops.DeleteResource(ctx, m.client, &corev1.PersistentVolumeClaim{}, m.runtimeNamespace, list.Items[i].GetName()); err != nil {
			return err
		}
	}
	return nil
}

func (m *CarrierManager) Cleanup(ctx context.Context, session *platformv1alpha1.AgentSessionResource) error {
	if session == nil {
		return nil
	}
	list := &corev1.PersistentVolumeClaimList{}
	if err := m.client.List(ctx, list,
		ctrlclient.InNamespace(m.runtimeNamespace),
		ctrlclient.MatchingLabels(map[string]string{
			carrierSessionIDLabel: strings.TrimSpace(session.GetName()),
			carrierManagedByLabel: carrierManagedByValue,
		}),
	); err != nil {
		return err
	}
	for i := range list.Items {
		if err := resourceops.DeleteResource(ctx, m.client, &corev1.PersistentVolumeClaim{}, m.runtimeNamespace, list.Items[i].GetName()); err != nil {
			return err
		}
	}
	return nil
}

func (m *CarrierManager) referencedPVCNames(ctx context.Context, session *platformv1alpha1.AgentSessionResource) (map[string]struct{}, bool, error) {
	keep := map[string]struct{}{}
	if workspaceID := strings.TrimSpace(session.Spec.Session.GetWorkspaceRef().GetWorkspaceId()); workspaceID != "" {
		keep[WorkspacePVCName(session.GetName(), workspaceID)] = struct{}{}
	}
	if homeStateID := strings.TrimSpace(session.Spec.Session.GetHomeStateRef().GetHomeStateId()); homeStateID != "" {
		keep[HomeStatePVCName(session.GetName(), homeStateID)] = struct{}{}
	}
	if m.actions != nil {
		actions, err := m.actions.ListBySession(ctx, strings.TrimSpace(session.GetName()))
		if err != nil {
			return nil, false, err
		}
		for i := range actions {
			if actions[i].Spec.Action == nil || strings.TrimSpace(actions[i].Spec.Action.GetSessionId()) != strings.TrimSpace(session.GetName()) {
				continue
			}
			if isTerminalActionResourcePhase(actions[i].Status.Phase) {
				continue
			}
			if actions[i].Spec.Action == nil || actions[i].Spec.Action.GetType() != agentsessionactionv1.AgentSessionActionType_AGENT_SESSION_ACTION_TYPE_RUN_TURN {
				continue
			}
			if legacy := addSnapshotPVCNames(keep, session.GetName(), actions[i].Spec.Action.GetInputSnapshot().GetRunTurn()); legacy {
				return keep, true, nil
			}
		}
	}
	runs := &platformv1alpha1.AgentRunResourceList{}
	if err := m.client.List(ctx, runs,
		ctrlclient.InNamespace(m.controlNamespace),
		ctrlclient.MatchingLabels(map[string]string{runSessionIDLabelKey: strings.TrimSpace(session.GetName())}),
	); err != nil {
		return nil, false, err
	}
	for i := range runs.Items {
		if runs.Items[i].Spec.Run == nil || strings.TrimSpace(runs.Items[i].Spec.Run.GetSessionId()) != strings.TrimSpace(session.GetName()) || isTerminalRunPhase(runs.Items[i].Status.Phase) {
			continue
		}
		if addSpecPVCNames(keep, runs.Items[i].Spec.Run, session.GetName()) {
			return keep, true, nil
		}
	}
	return keep, false, nil
}

func (m *CarrierManager) observeCarrier(ctx context.Context, sessionID string, carrierID string, kind carrierKind, missingMessage string) (bool, string, error) {
	carrierID = strings.TrimSpace(carrierID)
	if carrierID == "" {
		return false, missingMessage, nil
	}
	pvc := &corev1.PersistentVolumeClaim{}
	if err := m.client.Get(ctx, ctrlclient.ObjectKey{Namespace: m.runtimeNamespace, Name: carrierPVCName(sessionID, kind, carrierID)}, pvc); err != nil {
		if apierrors.IsNotFound(err) {
			return false, carrierPendingMessage(kind), nil
		}
		return false, "", err
	}
	if pvc.Status.Phase == corev1.ClaimLost {
		return false, carrierPendingMessage(kind), nil
	}
	return true, "", nil
}

func addSnapshotPVCNames(keep map[string]struct{}, sessionID string, snapshot *agentsessionactionv1.AgentSessionRunTurnSnapshot) bool {
	if snapshot == nil {
		return true
	}
	workspaceID := strings.TrimSpace(snapshot.GetWorkspaceId())
	homeStateID := strings.TrimSpace(snapshot.GetHomeStateId())
	if workspaceID == "" || homeStateID == "" {
		return true
	}
	keep[WorkspacePVCName(sessionID, workspaceID)] = struct{}{}
	keep[HomeStatePVCName(sessionID, homeStateID)] = struct{}{}
	return false
}

func addSpecPVCNames(keep map[string]struct{}, run *agentrunv1.AgentRunSpec, sessionID string) bool {
	workspaceID := strings.TrimSpace(run.GetWorkspaceId())
	homeStateID := strings.TrimSpace(run.GetHomeStateId())
	if workspaceID == "" || homeStateID == "" {
		return true
	}
	keep[WorkspacePVCName(sessionID, workspaceID)] = struct{}{}
	keep[HomeStatePVCName(sessionID, homeStateID)] = struct{}{}
	return false
}

func isTerminalActionResourcePhase(phase platformv1alpha1.AgentSessionActionResourcePhase) bool {
	switch phase {
	case platformv1alpha1.AgentSessionActionResourcePhaseSucceeded,
		platformv1alpha1.AgentSessionActionResourcePhaseFailed,
		platformv1alpha1.AgentSessionActionResourcePhaseCanceled:
		return true
	default:
		return false
	}
}

func carrierPendingMessage(kind carrierKind) string {
	switch kind {
	case carrierKindWorkspace:
		return "Workspace carrier is not ready."
	case carrierKindHomeState:
		return "Warm state carrier is not ready."
	default:
		return fmt.Sprintf("%s carrier is not ready.", kind)
	}
}
