package agentsessionactions

import (
	"testing"

	agentsessionactionv1 "code-code.internal/go-contract/platform/agent_session_action/v1"
	platformv1alpha1 "code-code.internal/platform-k8s/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestQueueOwnerIDSkipsTerminalActionWithRunID(t *testing.T) {
	items := []platformv1alpha1.AgentSessionActionResource{
		testQueueActionResource("action-1", platformv1alpha1.AgentSessionActionResourcePhaseFailed, "run-1"),
		testQueueActionResource("action-2", platformv1alpha1.AgentSessionActionResourcePhasePending, ""),
	}

	if got, want := queueOwnerID(items), "action-2"; got != want {
		t.Fatalf("queue owner = %q, want %q", got, want)
	}
}

func testQueueActionResource(actionID string, phase platformv1alpha1.AgentSessionActionResourcePhase, runID string) platformv1alpha1.AgentSessionActionResource {
	return platformv1alpha1.AgentSessionActionResource{
		ObjectMeta: metav1.ObjectMeta{
			Name:              actionID,
			CreationTimestamp: metav1.Unix(1, 0),
		},
		Spec: platformv1alpha1.AgentSessionActionResourceSpec{
			Action: &agentsessionactionv1.AgentSessionActionSpec{
				ActionId:  actionID,
				SessionId: "session-1",
				Type:      agentsessionactionv1.AgentSessionActionType_AGENT_SESSION_ACTION_TYPE_RUN_TURN,
			},
		},
		Status: platformv1alpha1.AgentSessionActionResourceStatus{
			Phase: phase,
			RunID: runID,
		},
	}
}
