package agentsessions

import (
	"context"
	"fmt"
	"strings"

	agentsessionactionv1 "code-code.internal/go-contract/platform/agent_session_action/v1"
	platformcontract "code-code.internal/platform-contract"
	platformv1alpha1 "code-code.internal/platform-k8s/api/v1alpha1"
	"code-code.internal/platform-k8s/internal/agentruntime/agentresourceconfig"
	"code-code.internal/platform-k8s/internal/platform/resourceops"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (r *Reconciler) ensureReloadSubjectActions(ctx context.Context, resource *platformv1alpha1.AgentSessionResource, evaluation readinessEvaluation, now metav1.Time) error {
	actions := buildReloadSubjectActions(resource, evaluation)
	if len(actions) == 0 {
		return nil
	}
	for _, action := range actions {
		if err := resourceops.CreateResource(ctx, r.client, action, r.namespace, action.Name); err != nil {
			if apierrors.IsAlreadyExists(err) || apierrors.IsConflict(err) {
				continue
			}
			return err
		}
		r.recordTimelineEvents(ctx, []*platformcontract.TimelineEvent{{
			ScopeRef: platformcontract.TimelineScopeRef{
				Scope:     platformcontract.TimelineScopeSession,
				SessionID: resource.Spec.Session.GetSessionId(),
			},
			EventType:  "SUBMITTED",
			Subject:    "action",
			Action:     "create",
			OccurredAt: now.UTC(),
			Attributes: map[string]string{
				"action_id": action.Name,
				"type":      agentsessionactionv1.AgentSessionActionType_AGENT_SESSION_ACTION_TYPE_RELOAD_SUBJECT.String(),
				"subject":   action.Spec.Action.GetInputSnapshot().GetReloadSubject().GetSubject().String(),
			},
		}})
	}
	return nil
}

func buildReloadSubjectActions(resource *platformv1alpha1.AgentSessionResource, evaluation readinessEvaluation) []*platformv1alpha1.AgentSessionActionResource {
	if resource == nil || resource.Spec.Session == nil {
		return nil
	}
	if !evaluation.workspaceReady || !evaluation.warmStateReady || !evaluation.runtimeReady {
		return nil
	}
	if strings.TrimSpace(resource.Spec.Session.GetResourceConfig().GetSnapshotId()) == "" {
		return nil
	}
	subjects := agentresourceconfig.PendingSubjects(resource.Spec.Session.GetResourceConfig(), realizedResourceConfigRevisions(resource))
	if len(subjects) == 0 {
		return nil
	}
	sessionID := strings.TrimSpace(resource.Spec.Session.GetSessionId())
	if sessionID == "" {
		return nil
	}
	actions := make([]*platformv1alpha1.AgentSessionActionResource, 0, len(subjects))
	for _, subject := range subjects {
		action := buildReloadSubjectAction(resource, subject)
		if action == nil {
			continue
		}
		actions = append(actions, action)
	}
	if len(actions) == 0 {
		return nil
	}
	return actions
}

func buildReloadSubjectAction(resource *platformv1alpha1.AgentSessionResource, subject agentsessionactionv1.AgentSessionActionSubject) *platformv1alpha1.AgentSessionActionResource {
	if resource == nil || resource.Spec.Session == nil {
		return nil
	}
	sessionID := strings.TrimSpace(resource.Spec.Session.GetSessionId())
	snapshotID := strings.TrimSpace(resource.Spec.Session.GetResourceConfig().GetSnapshotId())
	snapshot := agentresourceconfig.Snapshot(resource.Spec.Session.GetResourceConfig(), subject)
	if sessionID == "" || snapshotID == "" || snapshot == nil {
		return nil
	}
	actionID := fmt.Sprintf("%s-%s-reload-g%d", sessionID, agentresourceconfig.SubjectSlug(subject), resource.Generation)
	return &platformv1alpha1.AgentSessionActionResource{
		TypeMeta: metav1.TypeMeta{
			APIVersion: platformv1alpha1.GroupVersion.String(),
			Kind:       platformv1alpha1.KindAgentSessionActionResource,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      actionID,
			Namespace: resource.Namespace,
			Labels:    reloadSubjectActionLabels(sessionID, subject),
		},
		Spec: platformv1alpha1.AgentSessionActionResourceSpec{
			Action: &agentsessionactionv1.AgentSessionActionSpec{
				ActionId:  actionID,
				SessionId: sessionID,
				Type:      agentsessionactionv1.AgentSessionActionType_AGENT_SESSION_ACTION_TYPE_RELOAD_SUBJECT,
				InputSnapshot: &agentsessionactionv1.AgentSessionActionInputSnapshot{
					Snapshot: &agentsessionactionv1.AgentSessionActionInputSnapshot_ReloadSubject{
						ReloadSubject: &agentsessionactionv1.AgentSessionReloadSubjectSnapshot{
							SessionGeneration: resource.Generation,
							Subject:           subject,
							SnapshotId:        snapshotID,
							SubjectRevision:   snapshot.SubjectRevision,
							ResourceConfig:    snapshot.ResourceConfig,
						},
					},
				},
			},
		},
	}
}

func reloadSubjectActionLabels(sessionID string, subject agentsessionactionv1.AgentSessionActionSubject) map[string]string {
	labels := map[string]string{
		"agentsessionaction.code-code.internal/session-id": strings.TrimSpace(sessionID),
		"agentsessionaction.code-code.internal/type":       strings.TrimSpace(agentsessionactionv1.AgentSessionActionType_AGENT_SESSION_ACTION_TYPE_RELOAD_SUBJECT.String()),
	}
	if subject != agentsessionactionv1.AgentSessionActionSubject_AGENT_SESSION_ACTION_SUBJECT_UNSPECIFIED {
		labels["agentsessionaction.code-code.internal/subject"] = strings.TrimSpace(subject.String())
	}
	return labels
}
