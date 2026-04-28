package agentsessionactions

import (
	"context"
	"strings"
	"time"

	agentsessionactionv1 "code-code.internal/go-contract/platform/agent_session_action/v1"
	platformv1alpha1 "code-code.internal/platform-k8s/api/v1alpha1"
	"code-code.internal/platform-k8s/internal/agentruntime/agentsessions"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"
)

const runningRequeueAfter = 30 * time.Second

func (r *Reconciler) deriveStatus(ctx context.Context, resource *platformv1alpha1.AgentSessionActionResource) (*platformv1alpha1.AgentSessionActionResourceStatus, ctrl.Result, error) {
	now := r.now().UTC()
	if invalid := invalidStatus(resource, now); invalid != nil {
		return invalid, ctrl.Result{}, nil
	}
	if isTerminalPhase(resource.Status.Phase) {
		return resource.Status.DeepCopy(), ctrl.Result{}, nil
	}
	if resource.Spec.Action.GetStopRequested() {
		return r.deriveStoppedStatus(ctx, resource, now)
	}
	queueOwnerID, err := r.queueOwnerID(ctx, resource.Spec.Action.GetSessionId())
	if err != nil {
		return nil, ctrl.Result{}, err
	}
	if queueOwnerID != resource.Name {
		return pendingBlockedStatus(resource, now, "AgentSessionAction is waiting for prior actions."), ctrl.Result{}, nil
	}
	if waiting, result, ok := pendingRetryWindow(resource, now); ok {
		return waiting, result, nil
	}
	switch resource.Spec.Action.GetType() {
	case agentsessionactionv1.AgentSessionActionType_AGENT_SESSION_ACTION_TYPE_RUN_TURN:
		return r.deriveRunTurnStatus(ctx, resource, now)
	case agentsessionactionv1.AgentSessionActionType_AGENT_SESSION_ACTION_TYPE_RESET_WARM_STATE:
		return r.deriveResetWarmStateStatus(ctx, resource, now)
	case agentsessionactionv1.AgentSessionActionType_AGENT_SESSION_ACTION_TYPE_RELOAD_SUBJECT:
		return r.deriveReloadSubjectStatus(ctx, resource, now)
	default:
		return failedStatus(resource, now, "AgentSessionAction type is not supported yet."), ctrl.Result{}, nil
	}
}

func (r *Reconciler) queueOwnerID(ctx context.Context, sessionID string) (string, error) {
	items, err := listSessionActions(ctx, r.store, sessionID)
	if err != nil {
		return "", err
	}
	return queueOwnerID(items), nil
}

func (r *Reconciler) loadSession(ctx context.Context, sessionID string) (*platformv1alpha1.AgentSessionResource, error) {
	state, err := r.sessions.Get(ctx, strings.TrimSpace(sessionID))
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, apierrors.NewNotFound(schema.GroupResource{Group: platformv1alpha1.GroupName, Resource: "sessions"}, strings.TrimSpace(sessionID))
		}
		return nil, err
	}
	return agentsessions.ResourceFromState(state, r.namespace)
}

func isTerminalPhase(phase platformv1alpha1.AgentSessionActionResourcePhase) bool {
	switch phase {
	case platformv1alpha1.AgentSessionActionResourcePhaseSucceeded,
		platformv1alpha1.AgentSessionActionResourcePhaseFailed,
		platformv1alpha1.AgentSessionActionResourcePhaseCanceled:
		return true
	default:
		return false
	}
}
