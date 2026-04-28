package agentresourceconfig

import (
	"context"
	"fmt"
	"strings"

	capv1 "code-code.internal/go-contract/agent/cap/v1"
	"code-code.internal/platform-k8s/internal/platform/resourceops"
	"google.golang.org/protobuf/encoding/protojson"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	materializationDataKey       = "agent_resources.json"
	materializationSnapshotKey   = "agentsession.code-code.internal/resource-snapshot-id"
	materializationRuleKey       = "agentsession.code-code.internal/rule-revision"
	materializationSkillKey      = "agentsession.code-code.internal/skill-revision"
	materializationMCPKey        = "agentsession.code-code.internal/mcp-revision"
	materializationSessionIDKey  = "agentsession.code-code.internal/session-id"
	materializationManagedByKey  = "app.kubernetes.io/managed-by"
	materializationManagedByName = "platform-agent-runtime-service"
)

type Materializer struct {
	client    ctrlclient.Client
	namespace string
}

func NewMaterializer(client ctrlclient.Client, namespace string) (*Materializer, error) {
	if client == nil {
		return nil, fmt.Errorf("platformk8s/agentresourceconfig: materializer client is nil")
	}
	namespace = strings.TrimSpace(namespace)
	if namespace == "" {
		return nil, fmt.Errorf("platformk8s/agentresourceconfig: materializer namespace is empty")
	}
	return &Materializer{client: client, namespace: namespace}, nil
}

func (m *Materializer) Ensure(ctx context.Context, sessionID string, config *capv1.AgentResources) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return fmt.Errorf("platformk8s/agentresourceconfig: session_id is empty")
	}
	if config == nil {
		return fmt.Errorf("platformk8s/agentresourceconfig: resource config is nil")
	}
	payload, err := protojson.MarshalOptions{UseProtoNames: true}.Marshal(config)
	if err != nil {
		return fmt.Errorf("platformk8s/agentresourceconfig: marshal resource config: %w", err)
	}
	revisions := DesiredRevisions(config)
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      MaterializationName(sessionID),
			Namespace: m.namespace,
			Labels: map[string]string{
				materializationSessionIDKey: sessionID,
				materializationManagedByKey: materializationManagedByName,
			},
			Annotations: map[string]string{
				materializationSnapshotKey: strings.TrimSpace(config.GetSnapshotId()),
				materializationRuleKey:     strings.TrimSpace(revisions.Rule),
				materializationSkillKey:    strings.TrimSpace(revisions.Skill),
				materializationMCPKey:      strings.TrimSpace(revisions.MCP),
			},
		},
		Data: map[string]string{
			materializationDataKey: string(payload),
		},
	}
	return resourceops.UpsertResource(ctx, m.client, configMap, m.namespace, configMap.Name)
}

func (m *Materializer) IsCurrent(ctx context.Context, sessionID string, config *capv1.AgentResources) (bool, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" || config == nil {
		return false, nil
	}
	if strings.TrimSpace(config.GetSnapshotId()) == "" && Ready(config, Revisions{}) {
		return true, nil
	}
	current := &corev1.ConfigMap{}
	if err := m.client.Get(ctx, types.NamespacedName{Namespace: m.namespace, Name: MaterializationName(sessionID)}, current); err != nil {
		return false, ctrlclient.IgnoreNotFound(err)
	}
	revisions := DesiredRevisions(config)
	if strings.TrimSpace(current.Annotations[materializationSnapshotKey]) != strings.TrimSpace(config.GetSnapshotId()) {
		return false, nil
	}
	if strings.TrimSpace(current.Annotations[materializationRuleKey]) != strings.TrimSpace(revisions.Rule) {
		return false, nil
	}
	if strings.TrimSpace(current.Annotations[materializationSkillKey]) != strings.TrimSpace(revisions.Skill) {
		return false, nil
	}
	if strings.TrimSpace(current.Annotations[materializationMCPKey]) != strings.TrimSpace(revisions.MCP) {
		return false, nil
	}
	if strings.TrimSpace(current.Data[materializationDataKey]) == "" {
		return false, nil
	}
	return true, nil
}

func MaterializationName(sessionID string) string {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return ""
	}
	return sessionID + "-resources"
}

func MaterializationSessionID(object ctrlclient.Object) string {
	if object == nil {
		return ""
	}
	return strings.TrimSpace(object.GetLabels()[materializationSessionIDKey])
}
