package platform

import "testing"

func TestValidateAgentSessionConditionAcceptsKnownCondition(t *testing.T) {
	condition := &AgentSessionCondition{
		Type:               string(AgentSessionConditionTypeRuntimeConfigReady),
		Status:             AgentSessionConditionStatusTrue,
		Reason:             string(AgentSessionConditionReasonRuntimeConfigPrepared),
		ObservedGeneration: 1,
	}

	if err := ValidateAgentSessionCondition(condition); err != nil {
		t.Fatalf("ValidateAgentSessionCondition() error = %v", err)
	}
}

func TestValidateAgentSessionConditionAcceptsSessionNotReady(t *testing.T) {
	condition := &AgentSessionCondition{
		Type:               string(AgentSessionConditionTypeReadyForNextRun),
		Status:             AgentSessionConditionStatusFalse,
		Reason:             string(AgentSessionConditionReasonSessionNotReady),
		ObservedGeneration: 1,
	}

	if err := ValidateAgentSessionCondition(condition); err != nil {
		t.Fatalf("ValidateAgentSessionCondition() error = %v", err)
	}
}

func TestValidateAgentSessionConditionRejectsUnknownType(t *testing.T) {
	condition := &AgentSessionCondition{
		Type:   "Ready",
		Status: AgentSessionConditionStatusTrue,
		Reason: string(AgentSessionConditionReasonReady),
	}

	if err := ValidateAgentSessionCondition(condition); err == nil {
		t.Fatal("ValidateAgentSessionCondition() expected error, got nil")
	}
}

func TestValidateAgentSessionConditionRejectsReasonMismatch(t *testing.T) {
	condition := &AgentSessionCondition{
		Type:   string(AgentSessionConditionTypeWarmStateReady),
		Status: AgentSessionConditionStatusTrue,
		Reason: string(AgentSessionConditionReasonRuntimeConfigPrepared),
	}

	if err := ValidateAgentSessionCondition(condition); err == nil {
		t.Fatal("ValidateAgentSessionCondition() expected error, got nil")
	}
}

func TestValidateAgentSessionStatusRejectsDuplicateConditionType(t *testing.T) {
	status := &AgentSessionStatus{
		SessionId:                "session-1",
		Phase:                    AgentSessionPhaseReady,
		ObservedGeneration:       1,
		RuntimeConfigGeneration:  1,
		ResourceConfigGeneration: 1,
		StateGeneration:          1,
		Conditions: []*AgentSessionCondition{
			{
				Type:               string(AgentSessionConditionTypeWorkspaceReady),
				Status:             AgentSessionConditionStatusTrue,
				Reason:             string(AgentSessionConditionReasonWorkspacePrepared),
				ObservedGeneration: 1,
			},
			{
				Type:               string(AgentSessionConditionTypeWorkspaceReady),
				Status:             AgentSessionConditionStatusTrue,
				Reason:             string(AgentSessionConditionReasonWorkspacePrepared),
				ObservedGeneration: 1,
			},
		},
	}

	if err := ValidateAgentSessionStatus(status); err == nil {
		t.Fatal("ValidateAgentSessionStatus() expected error, got nil")
	}
}

func TestValidateAgentSessionStatusRejectsConditionObservedGenerationAheadOfStatus(t *testing.T) {
	status := &AgentSessionStatus{
		SessionId:                "session-1",
		Phase:                    AgentSessionPhaseRunning,
		ObservedGeneration:       1,
		RuntimeConfigGeneration:  1,
		ResourceConfigGeneration: 1,
		StateGeneration:          2,
		Conditions: []*AgentSessionCondition{
			{
				Type:               string(AgentSessionConditionTypeWarmStateReady),
				Status:             AgentSessionConditionStatusTrue,
				Reason:             string(AgentSessionConditionReasonWarmStatePrepared),
				ObservedGeneration: 2,
			},
		},
	}

	if err := ValidateAgentSessionStatus(status); err == nil {
		t.Fatal("ValidateAgentSessionStatus() expected error, got nil")
	}
}
