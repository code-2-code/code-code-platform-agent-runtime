package agentsessionactions

import "time"

const (
	automaticRetryBaseBackoff = 2 * time.Second
	automaticRetryMaxBackoff  = 1 * time.Minute
	automaticRetryLimit       = int32(5)
)

// RetryPolicy controls automatic retry scheduling for one action reconcile loop.
// MaxRetries=0 disables automatic retry.
type RetryPolicy struct {
	MaxRetries  int32
	BaseBackoff time.Duration
	MaxBackoff  time.Duration
}

func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{
		MaxRetries:  automaticRetryLimit,
		BaseBackoff: automaticRetryBaseBackoff,
		MaxBackoff:  automaticRetryMaxBackoff,
	}
}

func normalizeRetryPolicy(policy RetryPolicy) RetryPolicy {
	defaults := DefaultRetryPolicy()
	if policy.MaxRetries < 0 {
		policy.MaxRetries = 0
	}
	if policy.BaseBackoff <= 0 {
		policy.BaseBackoff = defaults.BaseBackoff
	}
	if policy.MaxBackoff <= 0 {
		policy.MaxBackoff = defaults.MaxBackoff
	}
	if policy.MaxBackoff < policy.BaseBackoff {
		policy.MaxBackoff = policy.BaseBackoff
	}
	return policy
}
