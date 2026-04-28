package agentsessions

import (
	credentialv1 "code-code.internal/go-contract/credential/v1"
	agentrunv1 "code-code.internal/go-contract/platform/agent_run/v1"
	agentsessionv1 "code-code.internal/go-contract/platform/agent_session/v1"
	platformv1alpha1 "code-code.internal/platform-k8s/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func readinessDependencyObjects() []ctrlclient.Object {
	return []ctrlclient.Object{
		&platformv1alpha1.CredentialDefinitionResource{
			TypeMeta:   metav1.TypeMeta{APIVersion: platformv1alpha1.GroupVersion.String(), Kind: platformv1alpha1.KindCredentialDefinitionResource},
			ObjectMeta: metav1.ObjectMeta{Name: "credential-openai", Namespace: "code-code"},
			Spec: platformv1alpha1.CredentialDefinitionResourceSpec{
				Definition: &credentialv1.CredentialDefinition{
					CredentialId: "credential-openai",
					Kind:         credentialv1.CredentialKind_CREDENTIAL_KIND_API_KEY,
				},
			},
		},
	}
}

func activeRunResource(runID string) *platformv1alpha1.AgentRunResource {
	return &platformv1alpha1.AgentRunResource{
		TypeMeta:   metav1.TypeMeta{APIVersion: platformv1alpha1.GroupVersion.String(), Kind: platformv1alpha1.KindAgentRunResource},
		ObjectMeta: metav1.ObjectMeta{Name: runID, Namespace: "code-code"},
		Spec: platformv1alpha1.AgentRunResourceSpec{
			Run: &agentrunv1.AgentRunSpec{
				RunId:     runID,
				SessionId: "agent-session-1",
			},
		},
		Status: platformv1alpha1.AgentRunResourceStatus{
			Phase: platformv1alpha1.AgentRunResourcePhaseRunning,
		},
	}
}

func boundCarrierPVCsForSession(session *agentsessionv1.AgentSessionSpec) []ctrlclient.Object {
	if session == nil {
		return nil
	}
	items := make([]ctrlclient.Object, 0, 2)
	if workspaceID := session.GetWorkspaceRef().GetWorkspaceId(); workspaceID != "" {
		items = append(items, &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      WorkspacePVCName(session.GetSessionId(), workspaceID),
				Namespace: "code-code-runs",
				Labels: map[string]string{
					carrierSessionIDLabel: session.GetSessionId(),
					carrierManagedByLabel: carrierManagedByValue,
				},
			},
			Status: corev1.PersistentVolumeClaimStatus{Phase: corev1.ClaimBound},
		})
	}
	if homeStateID := session.GetHomeStateRef().GetHomeStateId(); homeStateID != "" {
		items = append(items, &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      HomeStatePVCName(session.GetSessionId(), homeStateID),
				Namespace: "code-code-runs",
				Labels: map[string]string{
					carrierSessionIDLabel: session.GetSessionId(),
					carrierManagedByLabel: carrierManagedByValue,
				},
			},
			Status: corev1.PersistentVolumeClaimStatus{Phase: corev1.ClaimBound},
		})
	}
	return items
}
