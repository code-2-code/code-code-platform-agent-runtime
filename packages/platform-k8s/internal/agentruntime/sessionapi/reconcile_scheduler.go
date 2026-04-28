package sessionapi

import (
	"context"
	"encoding/json"
	"time"

	ctrl "sigs.k8s.io/controller-runtime"
)

const (
	defaultRequeueDelay = time.Second
)

// ReconcileScheduleRequest describes one delayed runtime reconcile trigger.
type ReconcileScheduleRequest struct {
	Action    string
	Body      []byte
	Delay     time.Duration
	OwnerKind string
	OwnerID   string
}

// ReconcileScheduler persists delayed runtime reconcile triggers outside the service process.
type ReconcileScheduler interface {
	ScheduleReconcile(context.Context, ReconcileScheduleRequest) error
}

func scheduleRequest(action string, body any, delay time.Duration, ownerKind string, ownerID string) (ReconcileScheduleRequest, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return ReconcileScheduleRequest{}, err
	}
	return ReconcileScheduleRequest{
		Action:    action,
		Body:      payload,
		Delay:     delay,
		OwnerKind: ownerKind,
		OwnerID:   ownerID,
	}, nil
}

func requeueDelay(result ctrl.Result) (time.Duration, bool) {
	if result.RequeueAfter > 0 {
		return result.RequeueAfter, true
	}
	if result.Requeue {
		return defaultRequeueDelay, true
	}
	return 0, false
}
