package sessionapi

import (
	"context"
	"testing"

	resultv1 "code-code.internal/go-contract/agent/result/v1"
	agentrunv1 "code-code.internal/go-contract/platform/agent_run/v1"
	"code-code.internal/platform-k8s/internal/platform/runevents"
)

func TestRecordTerminalResultPublishesToAgentRunService(t *testing.T) {
	t.Parallel()

	runs := &fakeAgentRunService{}
	server := &SessionServer{agentRuns: runs}

	err := server.recordTerminalResult(context.Background(), runevents.TerminalResult{
		SessionID: "session-1",
		RunID:     "run-1",
		Result:    &resultv1.RunResult{Status: resultv1.RunStatus_RUN_STATUS_COMPLETED},
	})
	if err != nil {
		t.Fatalf("recordTerminalResult() error = %v", err)
	}
	if got, want := runs.publishedRunID, "run-1"; got != want {
		t.Fatalf("published run_id = %q, want %q", got, want)
	}
	if runs.publishedResult.GetStatus() != resultv1.RunStatus_RUN_STATUS_COMPLETED {
		t.Fatalf("published status = %v, want completed", runs.publishedResult.GetStatus())
	}
}

func TestRecordTerminalResultIgnoresInvalidEvent(t *testing.T) {
	t.Parallel()

	runs := &fakeAgentRunService{}
	server := &SessionServer{agentRuns: runs}

	if err := server.recordTerminalResult(context.Background(), runevents.TerminalResult{}); err != nil {
		t.Fatalf("recordTerminalResult() error = %v", err)
	}
	if runs.publishedRunID != "" {
		t.Fatalf("published run_id = %q, want empty", runs.publishedRunID)
	}
}

type fakeAgentRunService struct {
	publishedRunID  string
	publishedResult *resultv1.RunResult
}

func (f *fakeAgentRunService) Get(context.Context, string) (*agentrunv1.AgentRunState, error) {
	return nil, nil
}

func (f *fakeAgentRunService) PublishTerminalResult(_ context.Context, runID string, result *resultv1.RunResult) error {
	f.publishedRunID = runID
	f.publishedResult = result
	return nil
}
