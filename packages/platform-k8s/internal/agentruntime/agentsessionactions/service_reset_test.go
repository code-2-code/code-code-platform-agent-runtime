package agentsessionactions

import (
	"context"
	"testing"
)

func TestResetWarmStateCreatesResetAction(t *testing.T) {
	t.Parallel()

	service := newTestService(t, readySessionResource())
	state, err := service.ResetWarmState(context.Background(), "session-1", &ResetWarmStateRequest{ActionID: "reset-1"})
	if err != nil {
		t.Fatalf("ResetWarmState() error = %v", err)
	}
	if got, want := state.GetSpec().GetType().String(), "AGENT_SESSION_ACTION_TYPE_RESET_WARM_STATE"; got != want {
		t.Fatalf("action type = %q, want %q", got, want)
	}
	snapshot := state.GetSpec().GetInputSnapshot().GetResetWarmState()
	if snapshot == nil {
		t.Fatal("reset_warm_state = nil, want frozen snapshot")
	}
	if got, want := snapshot.GetSourceHomeStateId(), "home-1"; got != want {
		t.Fatalf("source_home_state_id = %q, want %q", got, want)
	}
	if got := snapshot.GetTargetHomeStateId(); got == "" || got == snapshot.GetSourceHomeStateId() {
		t.Fatalf("target_home_state_id = %q, want non-empty new id", got)
	}
}

func TestResetWarmStateRejectsDuplicateNonterminalReset(t *testing.T) {
	t.Parallel()

	service := newTestService(t,
		readySessionResource(),
		pendingResetWarmStateAction("reset-1", "session-1", "home-1", "home-2"),
	)
	if _, err := service.ResetWarmState(context.Background(), "session-1", &ResetWarmStateRequest{ActionID: "reset-2"}); err == nil {
		t.Fatal("ResetWarmState() error = nil, want duplicate reset rejection")
	}
}
