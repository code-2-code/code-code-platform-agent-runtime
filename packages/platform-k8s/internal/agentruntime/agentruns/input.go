package agentruns

import (
	"strings"

	corev1 "code-code.internal/go-contract/agent/core/v1"
	"google.golang.org/protobuf/types/known/structpb"
)

func requestPrompt(request *corev1.RunRequest) string {
	if request == nil || request.GetInput() == nil {
		return ""
	}
	return strings.TrimSpace(request.GetInput().GetText())
}

func requestModel(request *corev1.RunRequest) string {
	if request == nil {
		return ""
	}
	if request.GetInput() != nil {
		if model := structString(request.GetInput().GetParameters(), "model"); model != "" {
			return model
		}
	}
	return strings.TrimSpace(request.GetResolvedProviderModel().GetProviderModelId())
}

func structString(value *structpb.Struct, field string) string {
	if value == nil {
		return ""
	}
	fields := value.GetFields()
	if len(fields) == 0 {
		return ""
	}
	item, ok := fields[field]
	if !ok || item == nil {
		return ""
	}
	return strings.TrimSpace(item.GetStringValue())
}
