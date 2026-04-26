package platform

import "testing"

func TestAgentRunPhaseIsTerminal(t *testing.T) {
	tests := []struct {
		name  string
		phase AgentRunPhase
		want  bool
	}{
		{name: "pending", phase: AgentRunPhasePending, want: false},
		{name: "scheduled", phase: AgentRunPhaseScheduled, want: false},
		{name: "running", phase: AgentRunPhaseRunning, want: false},
		{name: "succeeded", phase: AgentRunPhaseSucceeded, want: true},
		{name: "failed", phase: AgentRunPhaseFailed, want: true},
		{name: "canceled", phase: AgentRunPhaseCanceled, want: true},
	}

	for _, tt := range tests {
		if got := IsTerminalAgentRunPhase(tt.phase); got != tt.want {
			t.Fatalf("%s: IsTerminalAgentRunPhase() = %v, want %v", tt.name, got, tt.want)
		}
	}
}
