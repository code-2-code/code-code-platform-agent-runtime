package session

import (
	"encoding/json"
	"testing"
)

func TestAgentSessionResourcePreservesMetadataFinalizers(t *testing.T) {
	payload := []byte(`{
		"apiVersion":"platform.code-code.internal/v1alpha1",
		"kind":"AgentSessionResource",
		"metadata":{
			"name":"session-1",
			"namespace":"code-code",
			"finalizers":["agentsession.code-code.internal/runtime-cleanup"],
			"generation":7,
			"resourceVersion":"7"
		},
		"spec":{"session":{"sessionId":"session-1"}}
	}`)
	resource := &agentSessionResource{}
	if err := json.Unmarshal(payload, resource); err != nil {
		t.Fatalf("unmarshal resource: %v", err)
	}
	encoded, err := json.Marshal(resource)
	if err != nil {
		t.Fatalf("marshal resource: %v", err)
	}
	roundTrip := &agentSessionResource{}
	if err := json.Unmarshal(encoded, roundTrip); err != nil {
		t.Fatalf("unmarshal round trip: %v", err)
	}
	if got, want := len(roundTrip.Metadata.Finalizers), 1; got != want {
		t.Fatalf("finalizers = %d, want %d", got, want)
	}
	if got, want := roundTrip.Metadata.Finalizers[0], "agentsession.code-code.internal/runtime-cleanup"; got != want {
		t.Fatalf("finalizer = %q, want %q", got, want)
	}
}
