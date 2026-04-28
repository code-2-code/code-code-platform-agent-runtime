package sessionapi

import (
	"context"
	"fmt"
	"strings"

	managementv1 "code-code.internal/go-contract/platform/management/v1"
	"code-code.internal/platform-k8s/internal/agentruntime/agentsessionactions"
	"code-code.internal/platform-k8s/internal/platform/runevents"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *SessionServer) GetAgentSession(ctx context.Context, request *managementv1.GetAgentSessionRequest) (*managementv1.GetAgentSessionResponse, error) {
	session, err := s.agentSessions.Get(ctx, request.GetSessionId())
	if err != nil {
		return nil, grpcError(err)
	}
	return &managementv1.GetAgentSessionResponse{Session: session}, nil
}

func (s *SessionServer) CreateAgentSession(ctx context.Context, request *managementv1.CreateAgentSessionRequest) (*managementv1.CreateAgentSessionResponse, error) {
	return nil, status.Error(codes.Unimplemented, "session setup is owned by the session repository")
}

func (s *SessionServer) UpdateAgentSession(ctx context.Context, request *managementv1.UpdateAgentSessionRequest) (*managementv1.UpdateAgentSessionResponse, error) {
	return nil, status.Error(codes.Unimplemented, "session setup is owned by the session repository")
}

func (s *SessionServer) GetAgentSessionAction(ctx context.Context, request *managementv1.GetAgentSessionActionRequest) (*managementv1.GetAgentSessionActionResponse, error) {
	action, err := s.agentSessionActions.Get(ctx, request.GetActionId())
	if err != nil {
		return nil, grpcError(err)
	}
	return &managementv1.GetAgentSessionActionResponse{Action: action}, nil
}

func (s *SessionServer) CreateAgentSessionAction(ctx context.Context, request *managementv1.CreateAgentSessionActionRequest) (*managementv1.CreateAgentSessionActionResponse, error) {
	action, err := s.agentSessionActions.Create(ctx, request.GetSessionId(), &agentsessionactions.CreateRequest{
		ActionID:   request.GetActionId(),
		TurnID:     request.GetTurnId(),
		RunRequest: request.GetRunRequest(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	if err := s.recordUserTurnMessage(ctx, request.GetSessionId(), action, request.GetRunRequest()); err != nil {
		return nil, grpcError(err)
	}
	return &managementv1.CreateAgentSessionActionResponse{Action: action}, nil
}

func (s *SessionServer) ResetAgentSessionWarmState(ctx context.Context, request *managementv1.ResetAgentSessionWarmStateRequest) (*managementv1.ResetAgentSessionWarmStateResponse, error) {
	action, err := s.agentSessionActions.ResetWarmState(ctx, request.GetSessionId(), &agentsessionactions.ResetWarmStateRequest{
		ActionID: request.GetActionId(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	return &managementv1.ResetAgentSessionWarmStateResponse{Action: action}, nil
}

func (s *SessionServer) StopAgentSessionAction(ctx context.Context, request *managementv1.StopAgentSessionActionRequest) (*managementv1.StopAgentSessionActionResponse, error) {
	action, err := s.agentSessionActions.Stop(ctx, request.GetActionId())
	if err != nil {
		return nil, grpcError(err)
	}
	return &managementv1.StopAgentSessionActionResponse{Action: action}, nil
}

func (s *SessionServer) RetryAgentSessionAction(ctx context.Context, request *managementv1.RetryAgentSessionActionRequest) (*managementv1.RetryAgentSessionActionResponse, error) {
	action, err := s.agentSessionActions.Retry(ctx, request.GetSourceActionId(), &agentsessionactions.RetryRequest{
		TurnID: request.GetNewTurnId(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	return &managementv1.RetryAgentSessionActionResponse{Action: action}, nil
}

func (s *SessionServer) GetAgentRun(ctx context.Context, request *managementv1.GetAgentRunRequest) (*managementv1.GetAgentRunResponse, error) {
	run, err := s.agentRuns.Get(ctx, request.GetRunId())
	if err != nil {
		return nil, grpcError(err)
	}
	return &managementv1.GetAgentRunResponse{Run: run}, nil
}

func (s *SessionServer) StreamAgentRunOutput(request *managementv1.StreamAgentRunOutputRequest, stream managementv1.AgentSessionManagementService_StreamAgentRunOutputServer) error {
	if s == nil || s.runOutputs == nil {
		return grpcError(fmt.Errorf("platformk8s/sessionapi: run output stream unavailable"))
	}
	runID := strings.TrimSpace(request.GetRunId())
	run, err := s.agentRuns.Get(stream.Context(), runID)
	if err != nil {
		return grpcError(err)
	}
	return grpcError(s.runOutputs.Stream(stream.Context(), runevents.Request{
		SessionID:     strings.TrimSpace(run.GetSpec().GetSessionId()),
		RunID:         runID,
		AfterSequence: request.GetAfterSequence(),
	}, func(event runevents.StreamEvent) error {
		switch {
		case event.Delta != nil:
			return stream.Send(&managementv1.StreamAgentRunOutputResponse{
				Event: &managementv1.StreamAgentRunOutputResponse_Delta{Delta: event.Delta},
			})
		case event.Result != nil:
			return stream.Send(&managementv1.StreamAgentRunOutputResponse{
				Event: &managementv1.StreamAgentRunOutputResponse_Result{Result: event.Result},
			})
		default:
			return nil
		}
	}))
}
