package main

import (
	"testing"
	"time"
)

func TestActionRetryPolicyFromEnvReturnsNilWhenUnset(t *testing.T) {
	t.Setenv("PLATFORM_AGENT_RUNTIME_SERVICE_ACTION_RETRY_MAX_RETRIES", "")
	t.Setenv("PLATFORM_AGENT_RUNTIME_SERVICE_ACTION_RETRY_BASE_BACKOFF", "")
	t.Setenv("PLATFORM_AGENT_RUNTIME_SERVICE_ACTION_RETRY_MAX_BACKOFF", "")

	policy, err := actionRetryPolicyFromEnv()
	if err != nil {
		t.Fatalf("actionRetryPolicyFromEnv() error = %v", err)
	}
	if policy != nil {
		t.Fatalf("policy = %+v, want nil", policy)
	}
}

func TestActionRetryPolicyFromEnvParsesValues(t *testing.T) {
	t.Setenv("PLATFORM_AGENT_RUNTIME_SERVICE_ACTION_RETRY_MAX_RETRIES", "3")
	t.Setenv("PLATFORM_AGENT_RUNTIME_SERVICE_ACTION_RETRY_BASE_BACKOFF", "4s")
	t.Setenv("PLATFORM_AGENT_RUNTIME_SERVICE_ACTION_RETRY_MAX_BACKOFF", "30s")

	policy, err := actionRetryPolicyFromEnv()
	if err != nil {
		t.Fatalf("actionRetryPolicyFromEnv() error = %v", err)
	}
	if policy == nil {
		t.Fatal("policy = nil, want parsed policy")
	}
	if got, want := policy.MaxRetries, int32(3); got != want {
		t.Fatalf("max_retries = %d, want %d", got, want)
	}
	if got, want := policy.BaseBackoff, 4*time.Second; got != want {
		t.Fatalf("base_backoff = %s, want %s", got, want)
	}
	if got, want := policy.MaxBackoff, 30*time.Second; got != want {
		t.Fatalf("max_backoff = %s, want %s", got, want)
	}
}

func TestActionRetryPolicyFromEnvRejectsInvalidValue(t *testing.T) {
	t.Setenv("PLATFORM_AGENT_RUNTIME_SERVICE_ACTION_RETRY_MAX_RETRIES", "not-a-number")
	t.Setenv("PLATFORM_AGENT_RUNTIME_SERVICE_ACTION_RETRY_BASE_BACKOFF", "")
	t.Setenv("PLATFORM_AGENT_RUNTIME_SERVICE_ACTION_RETRY_MAX_BACKOFF", "")

	if _, err := actionRetryPolicyFromEnv(); err == nil {
		t.Fatal("actionRetryPolicyFromEnv() error = nil, want parse error")
	}
}
