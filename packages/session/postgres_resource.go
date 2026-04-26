package session

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"

	agentsessionv1 "code-code.internal/go-contract/platform/agent_session/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	agentSessionAPIVersion = "platform.code-code.internal/v1alpha1"
	agentSessionKind       = "AgentSessionResource"
)

type agentSessionResource struct {
	APIVersion string                     `json:"apiVersion,omitempty"`
	Kind       string                     `json:"kind,omitempty"`
	Metadata   agentSessionMetadata       `json:"metadata,omitempty"`
	Spec       agentSessionResourceSpec   `json:"spec,omitempty"`
	Status     agentSessionResourceStatus `json:"status,omitempty"`
}

type agentSessionMetadata struct {
	Name              string   `json:"name,omitempty"`
	Namespace         string   `json:"namespace,omitempty"`
	Finalizers        []string `json:"finalizers,omitempty"`
	Generation        int64    `json:"generation,omitempty"`
	ResourceVersion   string   `json:"resourceVersion,omitempty"`
	CreationTimestamp string   `json:"creationTimestamp,omitempty"`
}

type agentSessionResourceSpec struct {
	Session json.RawMessage `json:"session,omitempty"`
}

type agentSessionResourceStatus struct {
	ObservedGeneration       int64                   `json:"observedGeneration,omitempty"`
	Conditions               []agentSessionCondition `json:"conditions,omitempty"`
	Phase                    string                  `json:"phase,omitempty"`
	RuntimeConfigGeneration  int64                   `json:"runtimeConfigGeneration,omitempty"`
	ResourceConfigGeneration int64                   `json:"resourceConfigGeneration,omitempty"`
	RealizedRuleRevision     string                  `json:"realizedRuleRevision,omitempty"`
	RealizedSkillRevision    string                  `json:"realizedSkillRevision,omitempty"`
	RealizedMCPRevision      string                  `json:"realizedMcpRevision,omitempty"`
	ObservedHomeStateID      string                  `json:"observedHomeStateId,omitempty"`
	StateGeneration          int64                   `json:"stateGeneration,omitempty"`
	Message                  string                  `json:"message,omitempty"`
	ActiveRunID              string                  `json:"activeRunId,omitempty"`
	UpdatedAt                string                  `json:"updatedAt,omitempty"`
}

func newAgentSessionResource(namespace string, session *agentsessionv1.AgentSessionSpec, generation int64) (*agentSessionResource, error) {
	payload, err := marshalSessionSpec(session)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	return &agentSessionResource{
		APIVersion: agentSessionAPIVersion,
		Kind:       agentSessionKind,
		Metadata: agentSessionMetadata{
			Name:              session.GetSessionId(),
			Namespace:         namespace,
			Generation:        generation,
			ResourceVersion:   strconv.FormatInt(generation, 10),
			CreationTimestamp: now,
		},
		Spec: agentSessionResourceSpec{Session: payload},
	}, nil
}

func stateFromAgentSessionResource(resource *agentSessionResource, generation int64) (*agentsessionv1.AgentSessionState, error) {
	spec, err := specFromAgentSessionResource(resource)
	if err != nil {
		return nil, err
	}
	if spec.GetSessionId() == "" {
		spec.SessionId = resource.Metadata.Name
	}
	status := &agentsessionv1.AgentSessionStatus{
		SessionId:                spec.GetSessionId(),
		Phase:                    sessionPhase(resource.Status.Phase),
		ObservedGeneration:       resource.Status.ObservedGeneration,
		RuntimeConfigGeneration:  resource.Status.RuntimeConfigGeneration,
		ResourceConfigGeneration: resource.Status.ResourceConfigGeneration,
		StateGeneration:          resource.Status.StateGeneration,
		Message:                  resource.Status.Message,
		Conditions:               conditionsFromResource(resource.Status.Conditions),
		RealizedRuleRevision:     resource.Status.RealizedRuleRevision,
		RealizedSkillRevision:    resource.Status.RealizedSkillRevision,
		RealizedMcpRevision:      resource.Status.RealizedMCPRevision,
		ObservedHomeStateId:      resource.Status.ObservedHomeStateID,
	}
	if activeRunID := strings.TrimSpace(resource.Status.ActiveRunID); activeRunID != "" {
		status.ActiveRun = &agentsessionv1.AgentSessionActiveRunRef{RunId: activeRunID}
	}
	if updatedAt := strings.TrimSpace(resource.Status.UpdatedAt); updatedAt != "" {
		if parsed, err := time.Parse(time.RFC3339Nano, updatedAt); err == nil {
			status.UpdatedAt = timestamppb.New(parsed)
		}
	}
	return &agentsessionv1.AgentSessionState{
		Generation: generation,
		Spec:       spec,
		Status:     status,
	}, nil
}

func specFromAgentSessionResource(resource *agentSessionResource) (*agentsessionv1.AgentSessionSpec, error) {
	if resource == nil || len(resource.Spec.Session) == 0 {
		return nil, status.Error(codes.InvalidArgument, "session resource is invalid")
	}
	spec := &agentsessionv1.AgentSessionSpec{}
	if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(resource.Spec.Session, spec); err != nil {
		return nil, err
	}
	return spec, nil
}

func marshalSessionSpec(session *agentsessionv1.AgentSessionSpec) (json.RawMessage, error) {
	payload, err := protojson.MarshalOptions{UseProtoNames: false}.Marshal(session)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(payload), nil
}

func sessionPhase(phase string) agentsessionv1.AgentSessionPhase {
	switch strings.TrimSpace(phase) {
	case "PENDING":
		return agentsessionv1.AgentSessionPhase_AGENT_SESSION_PHASE_PENDING
	case "READY":
		return agentsessionv1.AgentSessionPhase_AGENT_SESSION_PHASE_READY
	case "RUNNING":
		return agentsessionv1.AgentSessionPhase_AGENT_SESSION_PHASE_RUNNING
	case "FAILED":
		return agentsessionv1.AgentSessionPhase_AGENT_SESSION_PHASE_FAILED
	default:
		return agentsessionv1.AgentSessionPhase_AGENT_SESSION_PHASE_UNSPECIFIED
	}
}

func resourcePhase(phase agentsessionv1.AgentSessionPhase) string {
	switch phase {
	case agentsessionv1.AgentSessionPhase_AGENT_SESSION_PHASE_PENDING:
		return "PENDING"
	case agentsessionv1.AgentSessionPhase_AGENT_SESSION_PHASE_READY:
		return "READY"
	case agentsessionv1.AgentSessionPhase_AGENT_SESSION_PHASE_RUNNING:
		return "RUNNING"
	case agentsessionv1.AgentSessionPhase_AGENT_SESSION_PHASE_FAILED:
		return "FAILED"
	default:
		return ""
	}
}
