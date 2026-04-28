package agentrunauth

import (
	"strings"

	managementv1 "code-code.internal/go-contract/platform/management/v1"
)

// Projection describes the runtime auth metadata used by L7 auth processing.
type Projection struct {
	MaterializationKey             string
	RuntimeURL                     string
	TargetHosts                    []string
	TargetPathPrefixes             []string
	RequestHeaderNames             []string
	HeaderValuePrefix              string
	RequestHeaderReplacementRules  []*managementv1.AgentRunRuntimeHeaderReplacementRule
	ResponseHeaderReplacementRules []*managementv1.AgentRunRuntimeHeaderReplacementRule
	EgressPolicyID                 string
	AuthPolicyID                   string
	ObservabilityProfileIDs        []string
	ProviderID                     string
	VendorID                       string
	ProviderSurfaceBindingID       string
	CLIID                          string
}

// ApplyToRuntimeMetadata copies projection details into the management-facing
// runtime metadata view.
func ApplyToRuntimeMetadata(metadata *managementv1.AgentRunRuntimeMetadata, projection Projection) {
	if metadata == nil {
		return
	}
	metadata.TargetHosts = trimNonEmpty(projection.TargetHosts)
	metadata.TargetPathPrefixes = trimNonEmpty(projection.TargetPathPrefixes)
	metadata.RequestHeaderNames = trimNonEmpty(projection.RequestHeaderNames)
	metadata.RequestHeaderReplacementRules = cloneHeaderReplacementRules(projection.RequestHeaderReplacementRules)
	metadata.ResponseHeaderReplacementRules = cloneHeaderReplacementRules(projection.ResponseHeaderReplacementRules)
	metadata.HeaderValuePrefix = strings.TrimSpace(projection.HeaderValuePrefix)
	metadata.EgressPolicyId = strings.TrimSpace(projection.EgressPolicyID)
	metadata.AuthPolicyId = strings.TrimSpace(projection.AuthPolicyID)
	metadata.ObservabilityProfileIds = trimNonEmpty(projection.ObservabilityProfileIDs)
	if metadata.CliId == "" {
		metadata.CliId = strings.TrimSpace(projection.CLIID)
	}
	if metadata.ProviderId == "" {
		metadata.ProviderId = strings.TrimSpace(projection.ProviderID)
	}
	if metadata.AuthMaterializationKey == "" {
		metadata.AuthMaterializationKey = strings.TrimSpace(projection.MaterializationKey)
	}
	if metadata.RuntimeUrl == "" {
		metadata.RuntimeUrl = strings.TrimSpace(projection.RuntimeURL)
	}
}

func trimNonEmpty(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func cloneHeaderReplacementRules(rules []*managementv1.AgentRunRuntimeHeaderReplacementRule) []*managementv1.AgentRunRuntimeHeaderReplacementRule {
	out := make([]*managementv1.AgentRunRuntimeHeaderReplacementRule, 0, len(rules))
	for _, rule := range rules {
		if rule == nil {
			continue
		}
		out = append(out, &managementv1.AgentRunRuntimeHeaderReplacementRule{
			Mode:              strings.TrimSpace(rule.GetMode()),
			HeaderName:        strings.TrimSpace(rule.GetHeaderName()),
			MaterialKey:       strings.TrimSpace(rule.GetMaterialKey()),
			HeaderValuePrefix: strings.TrimSpace(rule.GetHeaderValuePrefix()),
			Template:          strings.TrimSpace(rule.GetTemplate()),
		})
	}
	return out
}
