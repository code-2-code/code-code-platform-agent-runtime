package sessionapi

import (
	"context"
	"testing"

	agentcorev1 "code-code.internal/go-contract/agent/core/v1"
	inputv1 "code-code.internal/go-contract/agent/input/v1"
	agentsessionv1 "code-code.internal/go-contract/platform/agent_session/v1"
	agentsessionactionv1 "code-code.internal/go-contract/platform/agent_session_action/v1"
	managementv1 "code-code.internal/go-contract/platform/management/v1"
	"code-code.internal/platform-k8s/internal/agentruntime/agentsessionactions"
)

func TestSessionServerCreateAgentSessionActionPassesInnerRequest(t *testing.T) {
	t.Parallel()

	service := &fakeAgentSessionActionService{
		createResult: &agentsessionactionv1.AgentSessionActionState{
			Spec: &agentsessionactionv1.AgentSessionActionSpec{ActionId: "action-1"},
		},
	}
	server := &SessionServer{agentSessionActions: service}
	runRequest := testRunRequest("run-1", "hello")

	response, err := server.CreateAgentSessionAction(context.Background(), &managementv1.CreateAgentSessionActionRequest{
		SessionId:  "session-1",
		ActionId:   "action-1",
		TurnId:     "turn-1",
		RunRequest: runRequest,
	})
	if err != nil {
		t.Fatalf("CreateAgentSessionAction() error = %v", err)
	}
	if got, want := response.GetAction().GetSpec().GetActionId(), "action-1"; got != want {
		t.Fatalf("action_id = %q, want %q", got, want)
	}
	if got, want := service.sessionID, "session-1"; got != want {
		t.Fatalf("session_id = %q, want %q", got, want)
	}
	if service.createRequest == nil || service.createRequest.RunRequest != runRequest {
		t.Fatalf("create request = %#v, want original run request", service.createRequest)
	}
}

func TestSessionServerGetAgentSessionPassesSessionID(t *testing.T) {
	t.Parallel()

	service := &fakeAgentSessionService{
		getResult: &agentsessionv1.AgentSessionState{
			Spec: &agentsessionv1.AgentSessionSpec{SessionId: "session-1"},
		},
	}
	server := &SessionServer{agentSessions: service}

	response, err := server.GetAgentSession(context.Background(), &managementv1.GetAgentSessionRequest{SessionId: "session-1"})
	if err != nil {
		t.Fatalf("GetAgentSession() error = %v", err)
	}
	if got, want := response.GetSession().GetSpec().GetSessionId(), "session-1"; got != want {
		t.Fatalf("session_id = %q, want %q", got, want)
	}
	if got, want := service.getID, "session-1"; got != want {
		t.Fatalf("get id = %q, want %q", got, want)
	}
}

func TestSessionServerResetAgentSessionWarmStatePassesInnerRequest(t *testing.T) {
	t.Parallel()

	service := &fakeAgentSessionActionService{
		createResult: &agentsessionactionv1.AgentSessionActionState{
			Spec: &agentsessionactionv1.AgentSessionActionSpec{ActionId: "reset-1"},
		},
	}
	server := &SessionServer{agentSessionActions: service}

	response, err := server.ResetAgentSessionWarmState(context.Background(), &managementv1.ResetAgentSessionWarmStateRequest{
		SessionId: "session-1",
		ActionId:  "reset-1",
	})
	if err != nil {
		t.Fatalf("ResetAgentSessionWarmState() error = %v", err)
	}
	if got, want := response.GetAction().GetSpec().GetActionId(), "reset-1"; got != want {
		t.Fatalf("action_id = %q, want %q", got, want)
	}
	if got, want := service.sessionID, "session-1"; got != want {
		t.Fatalf("session_id = %q, want %q", got, want)
	}
	if service.resetRequest == nil || service.resetRequest.ActionID != "reset-1" {
		t.Fatalf("reset request = %#v, want action_id reset-1", service.resetRequest)
	}
}

type fakeAgentSessionService struct {
	getID     string
	getResult *agentsessionv1.AgentSessionState
}

func (f *fakeAgentSessionService) Get(_ context.Context, sessionID string) (*agentsessionv1.AgentSessionState, error) {
	f.getID = sessionID
	return f.getResult, nil
}

func (f *fakeAgentSessionService) Create(context.Context, *agentsessionv1.AgentSessionSpec) (*agentsessionv1.AgentSessionState, error) {
	return nil, nil
}

func (f *fakeAgentSessionService) Update(context.Context, string, *agentsessionv1.AgentSessionSpec) (*agentsessionv1.AgentSessionState, error) {
	return nil, nil
}

type fakeAgentSessionActionService struct {
	sessionID     string
	createRequest *agentsessionactions.CreateRequest
	resetRequest  *agentsessionactions.ResetWarmStateRequest
	createResult  *agentsessionactionv1.AgentSessionActionState
}

func (f *fakeAgentSessionActionService) Get(context.Context, string) (*agentsessionactionv1.AgentSessionActionState, error) {
	return nil, nil
}

func (f *fakeAgentSessionActionService) Create(_ context.Context, sessionID string, request *agentsessionactions.CreateRequest) (*agentsessionactionv1.AgentSessionActionState, error) {
	f.sessionID = sessionID
	f.createRequest = request
	return f.createResult, nil
}

func (f *fakeAgentSessionActionService) ResetWarmState(_ context.Context, sessionID string, request *agentsessionactions.ResetWarmStateRequest) (*agentsessionactionv1.AgentSessionActionState, error) {
	f.sessionID = sessionID
	f.resetRequest = request
	return f.createResult, nil
}

func (f *fakeAgentSessionActionService) Stop(context.Context, string) (*agentsessionactionv1.AgentSessionActionState, error) {
	return nil, nil
}

func (f *fakeAgentSessionActionService) Retry(context.Context, string, *agentsessionactions.RetryRequest) (*agentsessionactionv1.AgentSessionActionState, error) {
	return nil, nil
}

func testRunRequest(runID string, text string) *agentcorev1.RunRequest {
	return &agentcorev1.RunRequest{
		RunId: runID,
		Input: &inputv1.RunInput{Text: text},
	}
}
