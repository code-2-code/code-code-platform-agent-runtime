package agentsessions

import (
	"strings"

	modelv1 "code-code.internal/go-contract/model/v1"
)

func normalizeFallbackModelRef(ref *modelv1.ModelRef) *modelv1.ModelRef {
	if ref == nil || strings.TrimSpace(ref.GetModelId()) == "" {
		return nil
	}
	return &modelv1.ModelRef{
		VendorId: strings.TrimSpace(ref.GetVendorId()),
		ModelId:  strings.TrimSpace(ref.GetModelId()),
	}
}
