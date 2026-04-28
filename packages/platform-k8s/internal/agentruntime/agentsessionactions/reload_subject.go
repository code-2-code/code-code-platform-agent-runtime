package agentsessionactions

import (
	"context"
	"strings"
	"time"

	agentsessionv1 "code-code.internal/go-contract/platform/agent_session/v1"
	agentsessionactionv1 "code-code.internal/go-contract/platform/agent_session_action/v1"
	platformv1alpha1 "code-code.internal/platform-k8s/api/v1alpha1"
	"code-code.internal/platform-k8s/internal/agentruntime/agentresourceconfig"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
)

func (r *Reconciler) deriveReloadSubjectStatus(ctx context.Context, resource *platformv1alpha1.AgentSessionActionResource, now time.Time) (*platformv1alpha1.AgentSessionActionResourceStatus, ctrl.Result, error) {
	snapshot := resource.Spec.Action.GetInputSnapshot().GetReloadSubject()
	if snapshot == nil {
		return failedStatus(resource, now, "AgentSessionAction reload_subject requires input_snapshot.reload_subject."), ctrl.Result{}, nil
	}
	session, err := r.loadSession(ctx, resource.Spec.Action.GetSessionId())
	if err != nil {
		if apierrors.IsNotFound(err) {
			return failedStatus(resource, now, "AgentSession referenced session no longer exists."), ctrl.Result{}, nil
		}
		status, result := scheduleRetryStatus(resource, now, err.Error(), platformv1alpha1.AgentSessionActionResourceFailureClassPermanent, r.retryPolicy)
		return status, result, nil
	}
	if reloadSubjectSuperseded(session, snapshot) {
		return terminalStatus(resource, now, platformv1alpha1.AgentSessionActionResourcePhaseCanceled, canceledReloadMessage(snapshot), ""), ctrl.Result{}, nil
	}
	if reloadSubjectApplied(session, snapshot) && (snapshot.GetSubject() != agentsessionactionv1.AgentSessionActionSubject_AGENT_SESSION_ACTION_SUBJECT_RESOURCE_CONFIG || resource.Status.Phase == platformv1alpha1.AgentSessionActionResourcePhaseRunning) {
		return terminalStatus(resource, now, platformv1alpha1.AgentSessionActionResourcePhaseSucceeded, completedReloadMessage(snapshot), ""), ctrl.Result{}, nil
	}
	if resource.Status.Phase != platformv1alpha1.AgentSessionActionResourcePhaseRunning {
		if err := r.applyReloadSubject(ctx, session.GetName(), snapshot, now); err != nil {
			status, result := scheduleRetryStatus(resource, now, err.Error(), platformv1alpha1.AgentSessionActionResourceFailureClassPermanent, r.retryPolicy)
			return status, result, nil
		}
		return runningStatus(resource, now, "", pendingReloadMessage(snapshot), resource.Status.AttemptCount, resource.Status.CandidateIndex), ctrl.Result{Requeue: true}, nil
	}
	return runningStatus(resource, now, "", pendingReloadMessage(snapshot), resource.Status.AttemptCount, resource.Status.CandidateIndex), ctrl.Result{RequeueAfter: runningRequeueAfter}, nil
}

func reloadSubjectSuperseded(session *platformv1alpha1.AgentSessionResource, snapshot *agentsessionactionv1.AgentSessionReloadSubjectSnapshot) bool {
	if session == nil || session.Spec.Session == nil || snapshot == nil {
		return true
	}
	currentSnapshotID := strings.TrimSpace(session.Spec.Session.GetResourceConfig().GetSnapshotId())
	if currentSnapshotID != strings.TrimSpace(snapshot.GetSnapshotId()) {
		return true
	}
	current := agentresourceconfig.Snapshot(session.Spec.Session.GetResourceConfig(), snapshot.GetSubject())
	return current == nil || strings.TrimSpace(current.SubjectRevision) != strings.TrimSpace(snapshot.GetSubjectRevision())
}

func reloadSubjectApplied(session *platformv1alpha1.AgentSessionResource, snapshot *agentsessionactionv1.AgentSessionReloadSubjectSnapshot) bool {
	if session == nil || session.Spec.Session == nil || snapshot == nil {
		return false
	}
	return agentresourceconfig.Matches(sessionResourceRevisions(session), snapshot)
}

func (r *Reconciler) applyReloadSubject(ctx context.Context, sessionID string, snapshot *agentsessionactionv1.AgentSessionReloadSubjectSnapshot, now time.Time) error {
	if r == nil || r.resources == nil {
		return validation("resource materializer is unavailable")
	}
	state, err := r.sessions.Get(ctx, strings.TrimSpace(sessionID))
	if err != nil {
		return err
	}
	if state.GetSpec() == nil || state.GetSpec().GetResourceConfig() == nil {
		return validationf("session %q resource config is missing", sessionID)
	}
	if err := r.resources.Ensure(ctx, sessionID, state.GetSpec().GetResourceConfig()); err != nil {
		return err
	}
	status := state.GetStatus()
	if status == nil {
		status = &agentsessionv1.AgentSessionStatus{SessionId: strings.TrimSpace(sessionID)}
	} else {
		status = proto.Clone(status).(*agentsessionv1.AgentSessionStatus)
	}
	realized := agentresourceconfig.Revisions{
		Rule:  strings.TrimSpace(status.GetRealizedRuleRevision()),
		Skill: strings.TrimSpace(status.GetRealizedSkillRevision()),
		MCP:   strings.TrimSpace(status.GetRealizedMcpRevision()),
	}
	agentresourceconfig.Apply(&realized, snapshot)
	status.RealizedRuleRevision = realized.Rule
	status.RealizedSkillRevision = realized.Skill
	status.RealizedMcpRevision = realized.MCP
	status.UpdatedAt = timestamppb.New(now.UTC())
	_, err = r.sessions.UpdateStatus(ctx, sessionID, status)
	return err
}

func sessionResourceRevisions(session *platformv1alpha1.AgentSessionResource) agentresourceconfig.Revisions {
	if session == nil {
		return agentresourceconfig.Revisions{}
	}
	return agentresourceconfig.Revisions{
		Rule:  strings.TrimSpace(session.Status.RealizedRuleRevision),
		Skill: strings.TrimSpace(session.Status.RealizedSkillRevision),
		MCP:   strings.TrimSpace(session.Status.RealizedMCPRevision),
	}
}

func pendingReloadMessage(snapshot *agentsessionactionv1.AgentSessionReloadSubjectSnapshot) string {
	return "AgentSession " + agentresourceconfig.SubjectSlug(snapshot.GetSubject()) + " reload is waiting for session observation."
}

func completedReloadMessage(snapshot *agentsessionactionv1.AgentSessionReloadSubjectSnapshot) string {
	return "AgentSession " + agentresourceconfig.SubjectSlug(snapshot.GetSubject()) + " reload completed."
}

func canceledReloadMessage(snapshot *agentsessionactionv1.AgentSessionReloadSubjectSnapshot) string {
	return "AgentSession " + agentresourceconfig.SubjectSlug(snapshot.GetSubject()) + " reload was superseded by a newer revision."
}
