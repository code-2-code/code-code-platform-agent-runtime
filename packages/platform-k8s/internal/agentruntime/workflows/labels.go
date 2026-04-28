package workflows

import (
	"strings"
	"unicode"
)

const (
	LabelWorkflowKind = "platform.code-code.internal/workflow-kind"
	LabelOwnerKind    = "platform.code-code.internal/owner-kind"
	LabelOwnerID      = "platform.code-code.internal/owner-id"
	LabelTrigger      = "platform.code-code.internal/trigger"
	LabelTraceID      = "platform.code-code.internal/trace-id"
)

// WorkflowLabels creates the common platform workflow labels.
func WorkflowLabels(workflowKind, ownerKind, ownerID, trigger, traceID string) map[string]string {
	labels := map[string]string{}
	putLabel(labels, LabelWorkflowKind, workflowKind)
	putLabel(labels, LabelOwnerKind, ownerKind)
	putLabel(labels, LabelOwnerID, ownerID)
	putLabel(labels, LabelTrigger, trigger)
	putLabel(labels, LabelTraceID, traceID)
	return labels
}

// MergeLabels merges labels with overlay taking precedence.
func MergeLabels(base map[string]string, overlay map[string]string) map[string]string {
	out := map[string]string{}
	for key, value := range base {
		if strings.TrimSpace(key) != "" {
			out[key] = value
		}
	}
	for key, value := range overlay {
		if strings.TrimSpace(key) != "" {
			out[key] = value
		}
	}
	return out
}

// DNSLabelPart returns a DNS-label-safe fragment.
func DNSLabelPart(value, fallback string) string {
	out := dnsLabelPart(value)
	if out == "" && strings.TrimSpace(fallback) != "" {
		return dnsLabelPart(fallback)
	}
	return out
}

func dnsLabelPart(value string) string {
	var builder strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(value)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			builder.WriteRune(r)
		case r == '-' || r == '_' || unicode.IsSpace(r):
			builder.WriteByte('-')
		}
	}
	return strings.Trim(builder.String(), "-")
}

func putLabel(labels map[string]string, key, value string) {
	value = DNSLabelPart(value, "")
	if value == "" {
		return
	}
	if len(value) > 63 {
		value = strings.Trim(value[:63], "-")
	}
	labels[key] = value
}
