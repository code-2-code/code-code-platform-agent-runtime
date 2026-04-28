package agentruns

import (
	agentcorev1 "code-code.internal/go-contract/agent/core/v1"
	agentrunv1 "code-code.internal/go-contract/platform/agent_run/v1"
	agentsessionactionv1 "code-code.internal/go-contract/platform/agent_session_action/v1"
	"google.golang.org/protobuf/proto"
)

type CreateRequest struct {
	RunID    string
	Snapshot *agentsessionactionv1.AgentSessionRunTurnSnapshot
}

func buildRunSpec(sessionID string, request *CreateRequest, runID string) (*agentrunv1.AgentRunSpec, error) {
	if request == nil || request.Snapshot == nil {
		return nil, validation("run snapshot is nil")
	}
	runRequest, err := cloneRunRequest(request.Snapshot.GetRunRequest(), runID)
	if err != nil {
		return nil, err
	}
	return &agentrunv1.AgentRunSpec{
		RunId:                    runID,
		SessionId:                sessionID,
		SessionGeneration:        request.Snapshot.GetSessionGeneration(),
		RuntimeConfigGeneration:  request.Snapshot.GetRuntimeConfigGeneration(),
		ResourceConfigGeneration: request.Snapshot.GetResourceConfigGeneration(),
		StateGeneration:          request.Snapshot.GetStateGeneration(),
		Request:                  runRequest,
		ProviderId:               request.Snapshot.GetProviderId(),
		ExecutionClass:           request.Snapshot.GetExecutionClass(),
		ContainerImage:           request.Snapshot.GetContainerImage(),
		CpuRequest:               request.Snapshot.GetCpuRequest(),
		MemoryRequest:            request.Snapshot.GetMemoryRequest(),
		AuthRequirement:          cloneAuthRequirement(request.Snapshot.GetAuthRequirement()),
		RuntimeEnvironment:       cloneRuntimeEnvironment(request.Snapshot.GetRuntimeEnvironment()),
		WorkspaceId:              request.Snapshot.GetWorkspaceId(),
		HomeStateId:              request.Snapshot.GetHomeStateId(),
		PrepareJobs:              clonePrepareJobs(request.Snapshot.GetPrepareJobs()),
	}, nil
}

func cloneRunRequest(request *agentcorev1.RunRequest, runID string) (*agentcorev1.RunRequest, error) {
	if request == nil {
		return nil, validation("run request is nil")
	}
	runRequest := proto.Clone(request).(*agentcorev1.RunRequest)
	if runRequest.GetInput() == nil {
		return nil, validation("run request input is nil")
	}
	runRequest.RunId = runID
	return runRequest, nil
}

func cloneAuthRequirement(input *agentrunv1.AgentRunAuthRequirement) *agentrunv1.AgentRunAuthRequirement {
	if input == nil {
		return nil
	}
	return proto.Clone(input).(*agentrunv1.AgentRunAuthRequirement)
}

func cloneRuntimeEnvironment(environment *agentcorev1.RuntimeEnvironment) *agentcorev1.RuntimeEnvironment {
	if environment == nil {
		return nil
	}
	return proto.Clone(environment).(*agentcorev1.RuntimeEnvironment)
}

func clonePrepareJobs(jobs []*agentrunv1.AgentRunPrepareJob) []*agentrunv1.AgentRunPrepareJob {
	if len(jobs) == 0 {
		return nil
	}
	out := make([]*agentrunv1.AgentRunPrepareJob, 0, len(jobs))
	for _, job := range jobs {
		if job != nil {
			out = append(out, proto.Clone(job).(*agentrunv1.AgentRunPrepareJob))
		}
	}
	return out
}
