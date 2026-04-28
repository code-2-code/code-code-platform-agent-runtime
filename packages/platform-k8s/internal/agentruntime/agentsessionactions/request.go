package agentsessionactions

import (
	"strings"

	corev1 "code-code.internal/go-contract/agent/core/v1"
)

type CreateRequest struct {
	ActionID   string
	TurnID     string
	RunRequest *corev1.RunRequest
}

type RetryRequest struct {
	TurnID string
}

func normalizeTurnID(value string) string {
	return strings.TrimSpace(value)
}
