package session

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"

	capv1 "code-code.internal/go-contract/agent/cap/v1"
	agentsessionv1 "code-code.internal/go-contract/platform/agent_session/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

func NormalizeSpec(sessionID string, spec *agentsessionv1.AgentSessionSpec) (*agentsessionv1.AgentSessionSpec, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, status.Error(codes.InvalidArgument, "session_id is required")
	}
	if spec == nil {
		return nil, status.Error(codes.InvalidArgument, "session is required")
	}
	next := CloneSpec(spec)
	if current := strings.TrimSpace(next.GetSessionId()); current != "" && current != sessionID {
		return nil, status.Error(codes.InvalidArgument, "session.session_id must match session_id")
	}
	next.SessionId = sessionID
	if next.WorkspaceRef == nil {
		next.WorkspaceRef = DefaultWorkspaceRef(sessionID)
	}
	if next.HomeStateRef == nil {
		next.HomeStateRef = DefaultHomeStateRef(sessionID)
	}
	return next, nil
}

func NewProfileSpec(sessionID string, profileID string) (*agentsessionv1.AgentSessionSpec, error) {
	profileID = strings.TrimSpace(profileID)
	if profileID == "" {
		return nil, status.Error(codes.InvalidArgument, "profileId is required")
	}
	return NormalizeSpec(sessionID, &agentsessionv1.AgentSessionSpec{ProfileId: profileID})
}

func CloneSpec(spec *agentsessionv1.AgentSessionSpec) *agentsessionv1.AgentSessionSpec {
	if spec == nil {
		return nil
	}
	return proto.Clone(spec).(*agentsessionv1.AgentSessionSpec)
}

func CloneRuntimeConfig(config *agentsessionv1.AgentSessionRuntimeConfig) *agentsessionv1.AgentSessionRuntimeConfig {
	if config == nil {
		return nil
	}
	return proto.Clone(config).(*agentsessionv1.AgentSessionRuntimeConfig)
}

func CloneWorkspaceRef(current *agentsessionv1.AgentSessionWorkspaceRef, fallback *agentsessionv1.AgentSessionWorkspaceRef) *agentsessionv1.AgentSessionWorkspaceRef {
	if current == nil {
		return fallback
	}
	return proto.Clone(current).(*agentsessionv1.AgentSessionWorkspaceRef)
}

func CloneHomeStateRef(current *agentsessionv1.AgentSessionHomeStateRef, fallback *agentsessionv1.AgentSessionHomeStateRef) *agentsessionv1.AgentSessionHomeStateRef {
	if current == nil {
		return fallback
	}
	return proto.Clone(current).(*agentsessionv1.AgentSessionHomeStateRef)
}

func ClonePrepareJobs(jobs []*agentsessionv1.AgentSessionPrepareJob) []*agentsessionv1.AgentSessionPrepareJob {
	if len(jobs) == 0 {
		return nil
	}
	out := make([]*agentsessionv1.AgentSessionPrepareJob, 0, len(jobs))
	for _, job := range jobs {
		if job != nil {
			out = append(out, proto.Clone(job).(*agentsessionv1.AgentSessionPrepareJob))
		}
	}
	return out
}

func DefaultWorkspaceRef(sessionID string) *agentsessionv1.AgentSessionWorkspaceRef {
	return &agentsessionv1.AgentSessionWorkspaceRef{WorkspaceId: strings.TrimSpace(sessionID) + "-workspace"}
}

func DefaultHomeStateRef(sessionID string) *agentsessionv1.AgentSessionHomeStateRef {
	return &agentsessionv1.AgentSessionHomeStateRef{HomeStateId: strings.TrimSpace(sessionID) + "-home"}
}

func CloneAndNormalizeResourceConfig(config *capv1.AgentResources) *capv1.AgentResources {
	if config == nil {
		return nil
	}
	normalized := proto.Clone(config).(*capv1.AgentResources)
	normalized.SnapshotId = resourceSnapshotID(normalized)
	return normalized
}

func resourceSnapshotID(config *capv1.AgentResources) string {
	if config == nil {
		return ""
	}
	normalized := proto.Clone(config).(*capv1.AgentResources)
	normalized.SnapshotId = ""
	if len(normalized.GetInstructions()) == 0 && len(normalized.GetToolBindings()) == 0 {
		return ""
	}
	payload, err := proto.MarshalOptions{Deterministic: true}.Marshal(normalized)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}
