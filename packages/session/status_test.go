package session

import (
	"testing"

	agentsessionv1 "code-code.internal/go-contract/platform/agent_session/v1"
	conditionv1 "code-code.internal/go-contract/platform/condition/v1"
)

func TestNormalizeStatusRequiresMatchingSessionID(t *testing.T) {
	_, err := NormalizeStatus("session-1", &agentsessionv1.AgentSessionStatus{SessionId: "other"})
	if err == nil {
		t.Fatalf("NormalizeStatus() error = nil, want mismatch error")
	}
}

func TestResourceStatusRoundTripIncludesConditions(t *testing.T) {
	status, err := NormalizeStatus("session-1", &agentsessionv1.AgentSessionStatus{
		Phase:              agentsessionv1.AgentSessionPhase_AGENT_SESSION_PHASE_READY,
		ObservedGeneration: 3,
		Conditions: []*conditionv1.Condition{
			{
				Type:               "ReadyForNextRun",
				Status:             conditionv1.ConditionStatus_CONDITION_STATUS_TRUE,
				Reason:             "Ready",
				Message:            "ready",
				ObservedGeneration: 3,
			},
		},
	})
	if err != nil {
		t.Fatalf("NormalizeStatus() error = %v", err)
	}
	resourceStatus := resourceStatusFromProto(status)
	state, err := stateFromAgentSessionResource(&agentSessionResource{
		Metadata: agentSessionMetadata{Name: "session-1"},
		Spec: agentSessionResourceSpec{Session: mustMarshalSpec(t, &agentsessionv1.AgentSessionSpec{
			SessionId: "session-1",
		})},
		Status: resourceStatus,
	}, 3)
	if err != nil {
		t.Fatalf("stateFromAgentSessionResource() error = %v", err)
	}
	if got := state.GetStatus().GetConditions(); len(got) != 1 || got[0].GetType() != "ReadyForNextRun" {
		t.Fatalf("conditions = %#v, want one ReadyForNextRun condition", got)
	}
	if got := state.GetStatus().GetPhase(); got != agentsessionv1.AgentSessionPhase_AGENT_SESSION_PHASE_READY {
		t.Fatalf("phase = %v, want READY", got)
	}
}

func mustMarshalSpec(t *testing.T, spec *agentsessionv1.AgentSessionSpec) []byte {
	t.Helper()
	payload, err := marshalSessionSpec(spec)
	if err != nil {
		t.Fatalf("marshalSessionSpec() error = %v", err)
	}
	return payload
}
