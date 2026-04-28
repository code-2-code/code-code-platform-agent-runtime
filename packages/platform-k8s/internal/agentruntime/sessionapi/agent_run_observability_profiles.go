package sessionapi

import (
	"slices"
	"strings"

	observabilityv1 "code-code.internal/go-contract/observability/v1"
)

func observabilityProfileIDs(capability *observabilityv1.ObservabilityCapability) []string {
	if capability == nil {
		return nil
	}
	ids := make([]string, 0, len(capability.GetProfiles()))
	seen := map[string]struct{}{}
	for _, profile := range capability.GetProfiles() {
		id := strings.TrimSpace(profile.GetProfileId())
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids
}
