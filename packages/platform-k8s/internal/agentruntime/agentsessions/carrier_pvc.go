package agentsessions

import (
	"crypto/sha1"
	"fmt"
	"regexp"
	"strings"

	platformv1alpha1 "code-code.internal/platform-k8s/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	carrierSessionIDLabel = "agentsession.code-code.internal/session-id"
	carrierKindLabel      = "agentsession.code-code.internal/carrier-kind"
	carrierIDLabel        = "agentsession.code-code.internal/carrier-id"
	carrierManagedByLabel = "app.kubernetes.io/managed-by"
	carrierManagedByValue = "platform-agent-runtime-service"
	defaultCarrierStorage = "1Gi"
	maxCarrierNamePrefix  = 40
)

type carrierKind string

const (
	carrierKindWorkspace carrierKind = "workspace"
	carrierKindHomeState carrierKind = "home-state"
)

var carrierNamePattern = regexp.MustCompile(`[^a-z0-9-]+`)

func WorkspacePVCName(sessionID string, workspaceID string) string {
	return carrierPVCName(sessionID, carrierKindWorkspace, workspaceID)
}

func HomeStatePVCName(sessionID string, homeStateID string) string {
	return carrierPVCName(sessionID, carrierKindHomeState, homeStateID)
}

func carrierPVCName(sessionID string, kind carrierKind, carrierID string) string {
	sessionID = strings.TrimSpace(sessionID)
	carrierID = strings.TrimSpace(carrierID)
	if sessionID == "" || carrierID == "" {
		return ""
	}
	sum := sha1.Sum([]byte(sessionID + "/" + string(kind) + "/" + carrierID))
	return fmt.Sprintf("%s-%s-%x", carrierNamePrefix(sessionID), kind, sum[:5])
}

func carrierNamePrefix(sessionID string) string {
	normalized := carrierNamePattern.ReplaceAllString(strings.ToLower(strings.TrimSpace(sessionID)), "-")
	normalized = strings.Trim(normalized, "-")
	if normalized == "" {
		return "session"
	}
	if len(normalized) > maxCarrierNamePrefix {
		normalized = strings.Trim(normalized[:maxCarrierNamePrefix], "-")
	}
	if normalized == "" {
		return "session"
	}
	return normalized
}

func carrierPVCSessionID(object ctrlclient.Object) string {
	if object == nil {
		return ""
	}
	return strings.TrimSpace(object.GetLabels()[carrierSessionIDLabel])
}

func buildCarrierPVC(session *platformv1alpha1.AgentSessionResource, namespace string, kind carrierKind, carrierID string) *corev1.PersistentVolumeClaim {
	storage := resource.MustParse(defaultCarrierStorage)
	mode := corev1.PersistentVolumeFilesystem
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      carrierPVCName(session.GetName(), kind, carrierID),
			Namespace: namespace,
			Labels: map[string]string{
				carrierSessionIDLabel: strings.TrimSpace(session.GetName()),
				carrierKindLabel:      string(kind),
				carrierIDLabel:        strings.TrimSpace(carrierID),
				carrierManagedByLabel: carrierManagedByValue,
			},
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: storage,
				},
			},
			VolumeMode: &mode,
		},
	}
	if strings.TrimSpace(namespace) == strings.TrimSpace(session.GetNamespace()) {
		controller := true
		blockOwnerDeletion := true
		pvc.OwnerReferences = []metav1.OwnerReference{{
			APIVersion:         platformv1alpha1.GroupVersion.String(),
			Kind:               platformv1alpha1.KindAgentSessionResource,
			Name:               session.Name,
			UID:                session.UID,
			Controller:         &controller,
			BlockOwnerDeletion: &blockOwnerDeletion,
		}}
	}
	return pvc
}
