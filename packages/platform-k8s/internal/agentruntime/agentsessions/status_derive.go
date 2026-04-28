package agentsessions

import (
	"context"
	"strings"
	"time"

	platformv1alpha1 "code-code.internal/platform-k8s/api/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
)

func (r *Reconciler) deriveStatus(ctx context.Context, resource *platformv1alpha1.AgentSessionResource, now time.Time) *platformv1alpha1.AgentSessionResourceStatus {
	session := resource.Spec.Session
	if session == nil {
		return failedStatus(&resource.Status, resource.Generation, now, "InvalidSpec", "AgentSession spec.session is required.")
	}
	if strings.TrimSpace(session.GetSessionId()) == "" {
		return failedStatus(&resource.Status, resource.Generation, now, "InvalidSpec", "AgentSession sessionId is required.")
	}
	if strings.TrimSpace(session.GetProviderId()) == "" {
		return failedStatus(&resource.Status, resource.Generation, now, "InvalidSpec", "AgentSession providerId is required.")
	}
	if strings.TrimSpace(session.GetExecutionClass()) == "" {
		return failedStatus(&resource.Status, resource.Generation, now, "InvalidSpec", "AgentSession executionClass is required.")
	}

	evaluation := r.evaluateReadiness(ctx, resource)
	activeRunID, hasActiveRun := r.observeActiveRun(ctx, resource.Status.ActiveRunID)
	previousRuntimeGen, previousResourceGen, _ := previousConfigGenerations(&resource.Status)
	stateGeneration, observedHomeStateID := observeStateGeneration(&resource.Status, session, resource.Generation, evaluation.warmStateReady)
	status := &platformv1alpha1.AgentSessionResourceStatus{
		CommonStatusFields: platformv1alpha1.CommonStatusFields{
			ObservedGeneration: resource.Generation,
		},
		Phase:                   evaluation.phase,
		RuntimeConfigGeneration: observedConfigGeneration(previousRuntimeGen, evaluation.runtimeReady, resource.Generation),
		ResourceConfigGeneration: observedConfigGeneration(
			previousResourceGen,
			evaluation.resourceReady,
			resource.Generation,
		),
		RealizedRuleRevision:  strings.TrimSpace(resource.Status.RealizedRuleRevision),
		RealizedSkillRevision: strings.TrimSpace(resource.Status.RealizedSkillRevision),
		RealizedMCPRevision:   strings.TrimSpace(resource.Status.RealizedMCPRevision),
		ObservedHomeStateID:   observedHomeStateID,
		StateGeneration:       stateGeneration,
		Message:               evaluation.message,
		ActiveRunID:           activeRunID,
		UpdatedAt:             timePtr(now),
	}
	status.Conditions = evaluation.conditions(previousConditions(&resource.Status), resource.Generation, now, hasActiveRun)
	if hasActiveRun {
		status.Phase = platformv1alpha1.AgentSessionResourcePhaseRunning
		status.Message = "AgentSession has an active run."
	}
	return status
}

func (r *Reconciler) evaluateReadiness(ctx context.Context, resource *platformv1alpha1.AgentSessionResource) readinessEvaluation {
	evaluation := evaluateReadiness(resource)
	if r == nil || r.carriers == nil {
		return summarizeReadiness(readinessEvaluation{
			workspaceReady:   false,
			workspaceMessage: "AgentSession carrier manager is unavailable.",
			warmStateReady:   false,
			warmStateMessage: "AgentSession carrier manager is unavailable.",
			runtimeReady:     evaluation.runtimeReady,
			resourceReady:    evaluation.resourceReady,
			resourceMessage:  evaluation.resourceMessage,
		}, "")
	}
	observedCarriers, err := r.carriers.Observe(ctx, resource)
	if err != nil {
		return summarizeReadiness(readinessEvaluation{
			workspaceReady:   false,
			workspaceMessage: "AgentSession workspace carrier observation failed: " + strings.TrimSpace(err.Error()),
			warmStateReady:   false,
			warmStateMessage: "AgentSession warm state carrier observation failed: " + strings.TrimSpace(err.Error()),
			runtimeReady:     evaluation.runtimeReady,
			resourceReady:    evaluation.resourceReady,
			resourceMessage:  evaluation.resourceMessage,
		}, "")
	}
	evaluation.workspaceReady = observedCarriers.workspaceReady
	evaluation.workspaceMessage = observedCarriers.workspaceMessage
	evaluation.warmStateReady = observedCarriers.warmStateReady
	evaluation.warmStateMessage = observedCarriers.warmStateMessage
	if r != nil && r.actions != nil {
		resetPending, err := r.actions.HasNonterminalResetWarmState(ctx, resource.Spec.Session.GetSessionId())
		if err != nil {
			return summarizeReadiness(readinessEvaluation{
				workspaceReady:   evaluation.workspaceReady,
				workspaceMessage: evaluation.workspaceMessage,
				warmStateReady:   false,
				warmStateMessage: "AgentSession warm state reset observation failed: " + strings.TrimSpace(err.Error()),
				runtimeReady:     evaluation.runtimeReady,
				resourceReady:    evaluation.resourceReady,
				resourceMessage:  evaluation.resourceMessage,
			}, "")
		}
		if resetPending {
			evaluation.warmStateReady = false
			evaluation.warmStateMessage = "AgentSession warm state reset is in progress."
			return summarizeReadiness(evaluation, "")
		}
	}
	if !evaluation.runtimeReady {
		return summarizeReadiness(evaluation, "")
	}
	if r == nil || r.resolver == nil {
		return summarizeReadiness(readinessEvaluation{
			workspaceReady: evaluation.workspaceReady,
			warmStateReady: evaluation.warmStateReady,
			runtimeReady:   false,
			resourceReady:  evaluation.resourceReady,
		}, "AgentSession runtime resolver is unavailable.")
	}
	if _, err := r.resolver.Resolve(ctx, resource); err != nil {
		return summarizeReadiness(readinessEvaluation{
			workspaceReady: evaluation.workspaceReady,
			warmStateReady: evaluation.warmStateReady,
			runtimeReady:   false,
			resourceReady:  evaluation.resourceReady,
		}, "AgentSession runtime config is not ready: "+strings.TrimSpace(err.Error()))
	}
	if r.resources != nil {
		current, err := r.resources.IsCurrent(ctx, resource.Spec.Session.GetSessionId(), resource.Spec.Session.GetResourceConfig())
		if err != nil {
			return summarizeReadiness(readinessEvaluation{
				workspaceReady:  evaluation.workspaceReady,
				warmStateReady:  evaluation.warmStateReady,
				runtimeReady:    evaluation.runtimeReady,
				resourceReady:   false,
				resourceMessage: "AgentSession resource materialization is unavailable: " + strings.TrimSpace(err.Error()),
			}, "")
		}
		if !current {
			evaluation.resourceReady = false
			evaluation.resourceMessage = "Resource config materialization is not current."
			return summarizeReadiness(evaluation, "")
		}
	}
	return summarizeReadiness(evaluation, "")
}

func (r *Reconciler) observeActiveRun(ctx context.Context, activeRunID string) (string, bool) {
	activeRunID = strings.TrimSpace(activeRunID)
	if activeRunID == "" || r == nil {
		return "", false
	}
	resource := &platformv1alpha1.AgentRunResource{}
	if err := r.client.Get(ctx, types.NamespacedName{Namespace: r.namespace, Name: activeRunID}, resource); err != nil {
		if apierrors.IsNotFound(err) {
			return "", false
		}
		return activeRunID, true
	}
	if isTerminalRunPhase(resource.Status.Phase) {
		return "", false
	}
	return activeRunID, true
}

func isTerminalRunPhase(phase platformv1alpha1.AgentRunResourcePhase) bool {
	switch phase {
	case platformv1alpha1.AgentRunResourcePhaseSucceeded,
		platformv1alpha1.AgentRunResourcePhaseFailed,
		platformv1alpha1.AgentRunResourcePhaseCanceled:
		return true
	default:
		return false
	}
}
