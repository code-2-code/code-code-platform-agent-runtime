package platform

import "testing"

func TestValidateAgentRunConditionAcceptsKnownCondition(t *testing.T) {
	condition := &AgentRunCondition{
		Type:               string(AgentRunConditionTypeAccepted),
		Status:             ConditionStatusTrue,
		Reason:             string(AgentRunConditionReasonAccepted),
		ObservedGeneration: 1,
	}

	if err := ValidateAgentRunCondition(condition); err != nil {
		t.Fatalf("ValidateAgentRunCondition() error = %v", err)
	}
}

func TestValidateAgentRunConditionRejectsUnknownType(t *testing.T) {
	condition := &AgentRunCondition{
		Type:   "Ready",
		Status: ConditionStatusTrue,
		Reason: string(AgentRunConditionReasonAccepted),
	}

	if err := ValidateAgentRunCondition(condition); err == nil {
		t.Fatal("ValidateAgentRunCondition() expected error, got nil")
	}
}

func TestValidateAgentRunConditionRejectsNonCamelCaseReason(t *testing.T) {
	condition := &AgentRunCondition{
		Type:   string(AgentRunConditionTypeAccepted),
		Status: ConditionStatusTrue,
		Reason: "invalid_reason",
	}

	if err := ValidateAgentRunCondition(condition); err == nil {
		t.Fatal("ValidateAgentRunCondition() expected error, got nil")
	}
}

func TestValidateAgentRunConditionRejectsReasonMismatch(t *testing.T) {
	condition := &AgentRunCondition{
		Type:   string(AgentRunConditionTypeAccepted),
		Status: ConditionStatusTrue,
		Reason: string(AgentRunConditionReasonRunSucceeded),
	}

	if err := ValidateAgentRunCondition(condition); err == nil {
		t.Fatal("ValidateAgentRunCondition() expected error, got nil")
	}
}

func TestValidateAgentRunStatusRejectsDuplicateConditionType(t *testing.T) {
	status := &AgentRunStatus{
		RunId:              "run-1",
		Phase:              AgentRunPhasePending,
		ObservedGeneration: 1,
		Conditions: []*AgentRunCondition{
			{
				Type:               string(AgentRunConditionTypeAccepted),
				Status:             ConditionStatusTrue,
				Reason:             string(AgentRunConditionReasonAccepted),
				ObservedGeneration: 1,
			},
			{
				Type:               string(AgentRunConditionTypeAccepted),
				Status:             ConditionStatusTrue,
				Reason:             string(AgentRunConditionReasonAccepted),
				ObservedGeneration: 1,
			},
		},
	}

	if err := ValidateAgentRunStatus(status); err == nil {
		t.Fatal("ValidateAgentRunStatus() expected error, got nil")
	}
}

func TestValidateAgentRunStatusRejectsConditionObservedGenerationAheadOfStatus(t *testing.T) {
	status := &AgentRunStatus{
		RunId:              "run-1",
		Phase:              AgentRunPhaseRunning,
		ObservedGeneration: 1,
		Conditions: []*AgentRunCondition{
			{
				Type:               string(AgentRunConditionTypeWorkloadReady),
				Status:             ConditionStatusTrue,
				Reason:             string(AgentRunConditionReasonRunStarted),
				ObservedGeneration: 2,
			},
		},
	}

	if err := ValidateAgentRunStatus(status); err == nil {
		t.Fatal("ValidateAgentRunStatus() expected error, got nil")
	}
}
