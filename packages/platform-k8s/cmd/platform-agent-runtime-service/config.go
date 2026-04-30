package main

import (
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"code-code.internal/platform-k8s/internal/agentruntime/agentsessionactions"
)

func envOrDefault(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func firstEnv(keys ...string) string {
	for _, key := range keys {
		value := strings.TrimSpace(os.Getenv(key))
		if value != "" {
			return value
		}
	}
	return ""
}

func actionRetryPolicyFromEnv() (*agentsessionactions.RetryPolicy, error) {
	maxRetriesRaw := strings.TrimSpace(os.Getenv("PLATFORM_AGENT_RUNTIME_SERVICE_ACTION_RETRY_MAX_RETRIES"))
	baseBackoffRaw := strings.TrimSpace(os.Getenv("PLATFORM_AGENT_RUNTIME_SERVICE_ACTION_RETRY_BASE_BACKOFF"))
	maxBackoffRaw := strings.TrimSpace(os.Getenv("PLATFORM_AGENT_RUNTIME_SERVICE_ACTION_RETRY_MAX_BACKOFF"))
	if maxRetriesRaw == "" && baseBackoffRaw == "" && maxBackoffRaw == "" {
		return nil, nil
	}
	policy := agentsessionactions.RetryPolicy{}
	if maxRetriesRaw != "" {
		value, err := strconv.ParseInt(maxRetriesRaw, 10, 32)
		if err != nil {
			return nil, err
		}
		policy.MaxRetries = int32(value)
	}
	if baseBackoffRaw != "" {
		value, err := time.ParseDuration(baseBackoffRaw)
		if err != nil {
			return nil, err
		}
		policy.BaseBackoff = value
	}
	if maxBackoffRaw != "" {
		value, err := time.ParseDuration(maxBackoffRaw)
		if err != nil {
			return nil, err
		}
		policy.MaxBackoff = value
	}
	return &policy, nil
}

func must(err error) {
	if err != nil {
		slog.Error("fatal error", "error", err)
		os.Exit(1)
	}
}
