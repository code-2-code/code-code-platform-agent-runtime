package agentsessions

import (
	"strings"
	"time"

	capv1 "code-code.internal/go-contract/agent/cap/v1"
	agentsessionv1 "code-code.internal/go-contract/platform/agent_session/v1"
	platformcontract "code-code.internal/platform-contract"
	platformv1alpha1 "code-code.internal/platform-k8s/api/v1alpha1"
	"code-code.internal/platform-k8s/internal/agentruntime/agentresourceconfig"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type readinessEvaluation struct {
	workspaceReady   bool
	workspaceMessage string
	warmStateReady   bool
	warmStateMessage string
	runtimeReady     bool
	resourceReady    bool
	resourceMessage  string
	phase            platformv1alpha1.AgentSessionResourcePhase
	message          string
}

func evaluateReadiness(resource *platformv1alpha1.AgentSessionResource) readinessEvaluation {
	session := (*agentsessionv1.AgentSessionSpec)(nil)
	if resource != nil {
		session = resource.Spec.Session
	}
	return summarizeReadiness(readinessEvaluation{
		workspaceReady:   strings.TrimSpace(session.GetWorkspaceRef().GetWorkspaceId()) != "",
		workspaceMessage: "Workspace reference is missing.",
		warmStateReady:   strings.TrimSpace(session.GetHomeStateRef().GetHomeStateId()) != "",
		warmStateMessage: "Warm state reference is missing.",
		runtimeReady:     runtimeConfigReady(session.GetRuntimeConfig()),
		resourceReady:    resourceConfigReady(session.GetResourceConfig(), realizedResourceConfigRevisions(resource)),
		resourceMessage:  resourceConfigReadinessMessage(session.GetResourceConfig(), realizedResourceConfigRevisions(resource)),
	}, "")
}

func summarizeReadiness(evaluation readinessEvaluation, runtimeMessage string) readinessEvaluation {
	if !evaluation.workspaceReady {
		evaluation.phase = platformv1alpha1.AgentSessionResourcePhasePending
		evaluation.message = strings.TrimSpace(evaluation.workspaceMessage)
		if evaluation.message == "" {
			evaluation.message = "AgentSession workspace is not ready."
		}
		return evaluation
	}
	if !evaluation.warmStateReady {
		evaluation.phase = platformv1alpha1.AgentSessionResourcePhasePending
		evaluation.message = strings.TrimSpace(evaluation.warmStateMessage)
		if evaluation.message == "" {
			evaluation.message = "AgentSession warm state is not ready."
		}
		return evaluation
	}
	if !evaluation.runtimeReady {
		evaluation.phase = platformv1alpha1.AgentSessionResourcePhasePending
		if strings.TrimSpace(runtimeMessage) == "" {
			runtimeMessage = "AgentSession runtime config is not ready."
		}
		evaluation.message = runtimeMessage
		return evaluation
	}
	if !evaluation.resourceReady {
		evaluation.phase = platformv1alpha1.AgentSessionResourcePhasePending
		if strings.TrimSpace(evaluation.resourceMessage) == "" {
			evaluation.resourceMessage = "AgentSession resource config is not ready."
		}
		evaluation.message = evaluation.resourceMessage
		return evaluation
	}
	evaluation.phase = platformv1alpha1.AgentSessionResourcePhaseReady
	evaluation.message = "AgentSession is ready for the next run."
	return evaluation
}

func runtimeConfigReady(config *agentsessionv1.AgentSessionRuntimeConfig) bool {
	if config == nil {
		return false
	}
	return strings.TrimSpace(config.GetProviderRuntimeRef().GetSurfaceId()) != ""
}

func resourceConfigReady(config *capv1.AgentResources, realized agentresourceconfig.Revisions) bool {
	return config != nil && agentresourceconfig.Ready(config, realized)
}

func (e readinessEvaluation) conditions(previous []metav1.Condition, generation int64, now time.Time, activeRun bool) []metav1.Condition {
	readyForNextRun := e.workspaceReady && e.warmStateReady && e.runtimeReady && e.resourceReady && !activeRun
	readyForNextRunReason := platformcontract.AgentSessionConditionReasonSessionNotReady
	readyForNextRunMessage := e.message
	if activeRun {
		readyForNextRunReason = platformcontract.AgentSessionConditionReasonActiveRunInProgress
		readyForNextRunMessage = "AgentSession has an active run."
	}
	conditions := knownSessionConditions(previous)
	setCondition(
		&conditions,
		platformcontract.AgentSessionConditionTypeWorkspaceReady,
		e.workspaceReady,
		platformcontract.AgentSessionConditionReasonWorkspacePrepared,
		platformcontract.AgentSessionConditionReasonWorkspaceUnavailable,
		"Workspace is ready.",
		e.workspaceMessage,
		generation,
		now,
	)
	setCondition(
		&conditions,
		platformcontract.AgentSessionConditionTypeWarmStateReady,
		e.warmStateReady,
		platformcontract.AgentSessionConditionReasonWarmStatePrepared,
		platformcontract.AgentSessionConditionReasonWarmStateUnavailable,
		"Warm state is ready.",
		e.warmStateMessage,
		generation,
		now,
	)
	setCondition(
		&conditions,
		platformcontract.AgentSessionConditionTypeRuntimeConfigReady,
		e.runtimeReady,
		platformcontract.AgentSessionConditionReasonRuntimeConfigPrepared,
		platformcontract.AgentSessionConditionReasonRuntimeConfigInvalid,
		"Runtime config is ready.",
		"Runtime config requires providerRuntimeRef.surfaceId.",
		generation,
		now,
	)
	setCondition(
		&conditions,
		platformcontract.AgentSessionConditionTypeResourceConfigReady,
		e.resourceReady,
		platformcontract.AgentSessionConditionReasonResourceConfigPrepared,
		platformcontract.AgentSessionConditionReasonResourceConfigInvalid,
		"Resource config is ready.",
		e.resourceMessage,
		generation,
		now,
	)
	setCondition(
		&conditions,
		platformcontract.AgentSessionConditionTypeReadyForNextRun,
		readyForNextRun,
		platformcontract.AgentSessionConditionReasonReady,
		readyForNextRunReason,
		"AgentSession is ready for the next run.",
		readyForNextRunMessage,
		generation,
		now,
	)
	return conditions
}

func setCondition(conditions *[]metav1.Condition, conditionType platformcontract.AgentSessionConditionType, ready bool, readyReason platformcontract.AgentSessionConditionReason, notReadyReason platformcontract.AgentSessionConditionReason, readyMessage string, notReadyMessage string, generation int64, now time.Time) {
	if ready {
		setStatusCondition(conditions, conditionType, metav1.ConditionTrue, string(readyReason), readyMessage, generation, now)
		return
	}
	setStatusCondition(conditions, conditionType, metav1.ConditionFalse, string(notReadyReason), notReadyMessage, generation, now)
}

func setStatusCondition(conditions *[]metav1.Condition, conditionType platformcontract.AgentSessionConditionType, status metav1.ConditionStatus, reason string, message string, generation int64, now time.Time) {
	meta.SetStatusCondition(conditions, metav1.Condition{
		Type:               string(conditionType),
		Status:             status,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: generation,
		LastTransitionTime: metav1.NewTime(now),
	})
}

func knownSessionConditions(conditions []metav1.Condition) []metav1.Condition {
	out := make([]metav1.Condition, 0, len(conditions))
	for _, condition := range conditions {
		if platformcontract.IsKnownAgentSessionConditionType(condition.Type) {
			out = append(out, condition)
		}
	}
	return out
}
