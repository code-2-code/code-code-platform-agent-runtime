package sessionapi

import (
	"encoding/json"
	"testing"
	"time"

	ctrl "sigs.k8s.io/controller-runtime"
)

func TestRequeueDelay(t *testing.T) {
	t.Parallel()

	if delay, ok := requeueDelay(ctrl.Result{}); ok || delay != 0 {
		t.Fatalf("empty result delay = %s, %v; want no requeue", delay, ok)
	}
	if delay, ok := requeueDelay(ctrl.Result{Requeue: true}); !ok || delay != defaultRequeueDelay {
		t.Fatalf("requeue delay = %s, %v; want %s", delay, ok, defaultRequeueDelay)
	}
	if delay, ok := requeueDelay(ctrl.Result{RequeueAfter: 3 * time.Second}); !ok || delay != 3*time.Second {
		t.Fatalf("requeue_after delay = %s, %v; want 3s", delay, ok)
	}
}

func TestScheduleRequestMarshalsBody(t *testing.T) {
	t.Parallel()

	request, err := scheduleRequest("reconcile-session", map[string]string{"sessionId": "session-1"}, 1500*time.Millisecond, "session", "session-1")
	if err != nil {
		t.Fatalf("scheduleRequest() error = %v", err)
	}
	var body map[string]string
	if err := json.Unmarshal(request.Body, &body); err != nil {
		t.Fatalf("body is not json: %v", err)
	}
	if got, want := body["sessionId"], "session-1"; got != want {
		t.Fatalf("session_id = %q, want %q", got, want)
	}
	if request.Delay != 1500*time.Millisecond {
		t.Fatalf("delay = %s, want 1.5s", request.Delay)
	}
}
