package agentruns

import (
	"context"
	"fmt"
	"strings"

	platformcontract "code-code.internal/platform-contract"
	platformv1alpha1 "code-code.internal/platform-k8s/api/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type fakeActiveRunSlots struct {
	client    ctrlclient.Client
	namespace string
	claims    []string
	releases  []string
}

func newFakeActiveRunSlots(client ctrlclient.Client) *fakeActiveRunSlots {
	return &fakeActiveRunSlots{client: client, namespace: "code-code"}
}

func (s *fakeActiveRunSlots) Claim(ctx context.Context, sessionID string, runID string) (*platformv1alpha1.AgentSessionResource, error) {
	session := &platformv1alpha1.AgentSessionResource{}
	if err := s.client.Get(ctx, types.NamespacedName{Namespace: s.namespace, Name: strings.TrimSpace(sessionID)}, session); err != nil {
		return nil, err
	}
	activeRunID := strings.TrimSpace(session.Status.ActiveRunID)
	if activeRunID != "" && activeRunID != strings.TrimSpace(runID) {
		return nil, fmt.Errorf("session already has an active run")
	}
	if activeRunID == "" && !fakeSessionDispatchReady(session) {
		return nil, fmt.Errorf("session is not ready to dispatch")
	}
	session.Status.ActiveRunID = strings.TrimSpace(runID)
	session.Status.Phase = platformv1alpha1.AgentSessionResourcePhaseRunning
	if err := s.client.Status().Update(ctx, session); err != nil {
		return nil, err
	}
	s.claims = append(s.claims, strings.TrimSpace(runID))
	return session.DeepCopy(), nil
}

func (s *fakeActiveRunSlots) Release(ctx context.Context, sessionID string, runID string) (bool, error) {
	session := &platformv1alpha1.AgentSessionResource{}
	if err := s.client.Get(ctx, types.NamespacedName{Namespace: s.namespace, Name: strings.TrimSpace(sessionID)}, session); err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	if strings.TrimSpace(session.Status.ActiveRunID) != strings.TrimSpace(runID) {
		return false, nil
	}
	session.Status.ActiveRunID = ""
	session.Status.Phase = platformv1alpha1.AgentSessionResourcePhasePending
	if err := s.client.Status().Update(ctx, session); err != nil {
		return false, err
	}
	s.releases = append(s.releases, strings.TrimSpace(runID))
	return true, nil
}

func fakeSessionDispatchReady(session *platformv1alpha1.AgentSessionResource) bool {
	if session == nil {
		return false
	}
	return fakeConditionTrue(session.Status.Conditions, string(platformcontract.AgentSessionConditionTypeWorkspaceReady)) &&
		fakeConditionTrue(session.Status.Conditions, string(platformcontract.AgentSessionConditionTypeWarmStateReady))
}

func fakeConditionTrue(conditions []metav1.Condition, conditionType string) bool {
	for _, condition := range conditions {
		if condition.Type == conditionType && string(condition.Status) == "True" {
			return true
		}
	}
	return false
}
