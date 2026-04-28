package agentsessionactions

import (
	"context"
	"strings"

	agentcorev1 "code-code.internal/go-contract/agent/core/v1"
	"code-code.internal/go-contract/domainerror"
	agentrunv1 "code-code.internal/go-contract/platform/agent_run/v1"
	agentsessionactionv1 "code-code.internal/go-contract/platform/agent_session_action/v1"
	providerv1 "code-code.internal/go-contract/provider/v1"
	platformv1alpha1 "code-code.internal/platform-k8s/api/v1alpha1"
	"code-code.internal/platform-k8s/internal/agentruntime/agentexecution"
	"google.golang.org/protobuf/proto"
)

func buildRunTurnSnapshot(
	ctx context.Context,
	resolver *agentexecution.Resolver,
	session *platformv1alpha1.AgentSessionResource,
	request *CreateRequest,
) (*agentsessionactionv1.AgentSessionRunTurnSnapshot, error) {
	runRequest, err := cloneRunRequest(request)
	if err != nil {
		return nil, err
	}
	if session == nil || session.Spec.Session == nil {
		return nil, validation("session is invalid")
	}
	if session.Generation == 0 {
		return nil, validation("session generation is not observed yet")
	}
	if session.Status.RuntimeConfigGeneration != session.Generation || session.Status.ResourceConfigGeneration != session.Generation || session.Status.StateGeneration == 0 {
		return nil, validationf("session %q generations are not ready", session.Spec.Session.GetSessionId())
	}
	if resolver == nil {
		return nil, validation("execution resolver is unavailable")
	}
	resolution, err := resolver.ResolveRunTurn(ctx, session, runRequest)
	if err != nil {
		return nil, err
	}
	if len(resolution.RuntimeCandidates) == 0 || resolution.RuntimeCandidates[0] == nil || resolution.RuntimeCandidates[0].ResolvedProviderModel == nil {
		return nil, validation("resolved runtime candidates are empty")
	}
	prepareJobs, err := prepareJobsForSession(session.Spec.Session, resolution.AuthRequirement)
	if err != nil {
		return nil, err
	}
	runRequest.ResolvedProviderModel = cloneResolvedProviderModel(resolution.RuntimeCandidates[0].ResolvedProviderModel)
	return &agentsessionactionv1.AgentSessionRunTurnSnapshot{
		RunRequest:               runRequest,
		SessionGeneration:        session.Generation,
		RuntimeConfigGeneration:  session.Status.RuntimeConfigGeneration,
		ResourceConfigGeneration: session.Status.ResourceConfigGeneration,
		StateGeneration:          session.Status.StateGeneration,
		ProviderId:               session.Spec.Session.GetProviderId(),
		ExecutionClass:           session.Spec.Session.GetExecutionClass(),
		ContainerImage:           resolution.ContainerImage,
		CpuRequest:               resolution.CPURequest,
		MemoryRequest:            resolution.MemoryRequest,
		AuthRequirement:          cloneAuthRequirement(resolution.AuthRequirement),
		RuntimeCandidates:        cloneRuntimeCandidates(resolution.RuntimeCandidates),
		RuntimeEnvironment:       defaultRuntimeEnvironment(),
		WorkspaceId:              strings.TrimSpace(session.Spec.Session.GetWorkspaceRef().GetWorkspaceId()),
		HomeStateId:              strings.TrimSpace(session.Spec.Session.GetHomeStateRef().GetHomeStateId()),
		PrepareJobs:              prepareJobs,
	}, nil
}

func defaultRuntimeEnvironment() *agentcorev1.RuntimeEnvironment {
	return &agentcorev1.RuntimeEnvironment{
		WorkspaceDir: "/workspace",
		DataDir:      "/home/agent",
	}
}

func cloneRunRequest(request *CreateRequest) (*agentcorev1.RunRequest, error) {
	if request == nil || request.RunRequest == nil {
		return nil, domainerror.NewValidation("platformk8s/agentsessionactions: run request is nil")
	}
	runRequest := proto.Clone(request.RunRequest).(*agentcorev1.RunRequest)
	if runRequest.GetInput() == nil {
		return nil, domainerror.NewValidation("platformk8s/agentsessionactions: run request input is nil")
	}
	return runRequest, nil
}

func cloneAuthRequirement(input *agentrunv1.AgentRunAuthRequirement) *agentrunv1.AgentRunAuthRequirement {
	if input == nil {
		return nil
	}
	return proto.Clone(input).(*agentrunv1.AgentRunAuthRequirement)
}

func cloneResolvedProviderModel(model *providerv1.ResolvedProviderModel) *providerv1.ResolvedProviderModel {
	if model == nil {
		return nil
	}
	return proto.Clone(model).(*providerv1.ResolvedProviderModel)
}

func cloneRuntimeCandidates(candidates []*agentexecution.RuntimeCandidate) []*agentsessionactionv1.AgentSessionRuntimeCandidate {
	if len(candidates) == 0 {
		return nil
	}
	out := make([]*agentsessionactionv1.AgentSessionRuntimeCandidate, 0, len(candidates))
	for _, item := range candidates {
		if item == nil || item.ResolvedProviderModel == nil {
			continue
		}
		out = append(out, &agentsessionactionv1.AgentSessionRuntimeCandidate{
			ResolvedProviderModel: cloneResolvedProviderModel(item.ResolvedProviderModel),
			AuthRequirement:       cloneAuthRequirement(item.AuthRequirement),
		})
	}
	return out
}
