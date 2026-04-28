package agentsessions

import (
	"strings"

	"code-code.internal/platform-k8s/internal/platform/resourcemeta"
)

func ensureSessionID(requestedID string, fallback string) (string, error) {
	return resourcemeta.EnsureResourceID(requestedID, fallback, "session")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
