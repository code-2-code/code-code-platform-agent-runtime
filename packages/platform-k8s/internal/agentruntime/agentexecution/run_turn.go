package agentexecution

import (
	"context"

	agentcorev1 "code-code.internal/go-contract/agent/core/v1"
	agentrunv1 "code-code.internal/go-contract/platform/agent_run/v1"
	platformv1alpha1 "code-code.internal/platform-k8s/api/v1alpha1"
	"google.golang.org/protobuf/proto"
)

// ResolveRunTurn freezes the ordered runtime candidates for one run_turn
// action while reusing the standard execution image resolution path.
func (r *Resolver) ResolveRunTurn(ctx context.Context, session *platformv1alpha1.AgentSessionResource, request *agentcorev1.RunRequest) (*Resolution, error) {
	if request == nil || request.GetInput() == nil {
		return nil, validation("run request is invalid")
	}
	resolution, err := r.Resolve(ctx, session)
	if err != nil {
		return nil, err
	}
	candidates, err := r.resolveRuntimeCandidates(ctx, session, request)
	if err != nil {
		return nil, err
	}
	resolution.AuthRequirement = cloneAuthRequirement(candidates[0].AuthRequirement)
	resolution.RuntimeCandidates = candidates
	return resolution, nil
}

func (r *Resolver) resolveRuntimeCandidates(ctx context.Context, session *platformv1alpha1.AgentSessionResource, request *agentcorev1.RunRequest) ([]*RuntimeCandidate, error) {
	if session == nil || session.Spec.Session == nil {
		return nil, validation("session is invalid")
	}
	primaryInstance, err := r.loadPrimaryProviderSurfaceBinding(ctx, session)
	if err != nil {
		return nil, err
	}
	primary, err := r.resolvePrimaryRuntimeCandidate(ctx, session, request, primaryInstance)
	if err != nil {
		return nil, err
	}
	candidates := []*RuntimeCandidate{primary}
	for _, item := range session.Spec.Session.GetRuntimeConfig().GetFallbacks() {
		candidate, err := r.resolveFallbackRuntimeCandidate(ctx, session, item)
		if err != nil {
			return nil, err
		}
		candidates = append(candidates, candidate)
	}
	return candidates, nil
}

func cloneAuthRequirement(input *agentrunv1.AgentRunAuthRequirement) *agentrunv1.AgentRunAuthRequirement {
	if input == nil {
		return nil
	}
	return proto.Clone(input).(*agentrunv1.AgentRunAuthRequirement)
}
