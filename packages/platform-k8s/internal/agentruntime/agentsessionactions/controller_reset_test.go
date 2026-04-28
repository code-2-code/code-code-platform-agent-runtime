package agentsessionactions

import (
	"context"
	"testing"

	agentsessionactionv1 "code-code.internal/go-contract/platform/agent_session_action/v1"
	platformcontract "code-code.internal/platform-contract"
	platformv1alpha1 "code-code.internal/platform-k8s/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func TestReconcilerAppliesResetWarmStateAction(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	session := readySessionResource()
	action := pendingResetWarmStateAction("reset-1", "session-1", "home-1", "home-2")
	client := newClient(t, session, action)
	reconciler := newTestReconciler(t, client)

	if _, err := reconciler.Reconcile(ctx, requestFor("reset-1")); err != nil {
		t.Fatalf("first reconcile: %v", err)
	}

	updatedSession := getSessionResource(t, ctx, client, "session-1")
	if got, want := updatedSession.Spec.Session.GetHomeStateRef().GetHomeStateId(), "home-2"; got != want {
		t.Fatalf("home_state_id = %q, want %q", got, want)
	}
	updatedAction := getActionResource(t, ctx, client, "reset-1")
	if got, want := updatedAction.Status.Phase, platformv1alpha1.AgentSessionActionResourcePhaseRunning; got != want {
		t.Fatalf("phase = %q, want %q", got, want)
	}

	updatedSession.Status.ObservedHomeStateID = "home-2"
	updatedSession.Status.StateGeneration = 8
	updatedSession.Status.Conditions = replaceWarmStateCondition(updatedSession.Status.Conditions, true)
	if err := client.Status().Update(ctx, updatedSession); err != nil {
		t.Fatalf("update session status: %v", err)
	}

	if _, err := reconciler.Reconcile(ctx, requestFor("reset-1")); err != nil {
		t.Fatalf("second reconcile: %v", err)
	}
	updatedAction = getActionResource(t, ctx, client, "reset-1")
	if got, want := updatedAction.Status.Phase, platformv1alpha1.AgentSessionActionResourcePhaseSucceeded; got != want {
		t.Fatalf("phase = %q, want %q", got, want)
	}
}

func pendingResetWarmStateAction(actionID string, sessionID string, sourceHomeStateID string, targetHomeStateID string) *platformv1alpha1.AgentSessionActionResource {
	return &platformv1alpha1.AgentSessionActionResource{
		TypeMeta: metav1.TypeMeta{APIVersion: platformv1alpha1.GroupVersion.String(), Kind: platformv1alpha1.KindAgentSessionActionResource},
		ObjectMeta: metav1.ObjectMeta{
			Name:      actionID,
			Namespace: "code-code",
			Labels:    actionLabels(sessionID, agentsessionactionv1.AgentSessionActionType_AGENT_SESSION_ACTION_TYPE_RESET_WARM_STATE),
		},
		Spec: platformv1alpha1.AgentSessionActionResourceSpec{
			Action: &agentsessionactionv1.AgentSessionActionSpec{
				ActionId:  actionID,
				SessionId: sessionID,
				Type:      agentsessionactionv1.AgentSessionActionType_AGENT_SESSION_ACTION_TYPE_RESET_WARM_STATE,
				InputSnapshot: &agentsessionactionv1.AgentSessionActionInputSnapshot{
					Snapshot: &agentsessionactionv1.AgentSessionActionInputSnapshot_ResetWarmState{
						ResetWarmState: &agentsessionactionv1.AgentSessionResetWarmStateSnapshot{
							SessionGeneration: 7,
							SourceHomeStateId: sourceHomeStateID,
							TargetHomeStateId: targetHomeStateID,
						},
					},
				},
			},
		},
	}
}

func getSessionResource(t *testing.T, ctx context.Context, client ctrlclient.Client, name string) *platformv1alpha1.AgentSessionResource {
	t.Helper()
	resource := &platformv1alpha1.AgentSessionResource{}
	if err := client.Get(ctx, types.NamespacedName{Namespace: "code-code", Name: name}, resource); err != nil {
		t.Fatalf("Get(session) error = %v", err)
	}
	return resource
}

func replaceWarmStateCondition(conditions []metav1.Condition, ready bool) []metav1.Condition {
	out := append([]metav1.Condition(nil), conditions...)
	for i := range out {
		if out[i].Type != string(platformcontract.AgentSessionConditionTypeWarmStateReady) {
			continue
		}
		if ready {
			out[i].Status = metav1.ConditionTrue
			out[i].Reason = string(platformcontract.AgentSessionConditionReasonWarmStatePrepared)
			out[i].Message = "Warm state is ready."
		} else {
			out[i].Status = metav1.ConditionFalse
			out[i].Reason = string(platformcontract.AgentSessionConditionReasonWarmStateUnavailable)
			out[i].Message = "Warm state carrier is not ready."
		}
	}
	return out
}
