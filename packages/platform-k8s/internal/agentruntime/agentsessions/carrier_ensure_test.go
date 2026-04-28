package agentsessions

import (
	"context"
	"fmt"
	"testing"

	corev1 "k8s.io/api/core/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func TestCarrierManagerEnsureDoesNotUpdateExistingPVCs(t *testing.T) {
	ctx := context.Background()
	resource := newSessionResource("agent-session-1", 7, validSessionSpec())
	carriers := boundCarrierPVCsForSession(resource.Spec.Session)
	client := &rejectPVCUpdateClient{Client: newClientWithoutAutoCarriers(append([]ctrlclient.Object{resource}, carriers...)...)}
	manager, err := NewCarrierManager(client, "code-code", "code-code-runs")
	if err != nil {
		t.Fatalf("new carrier manager: %v", err)
	}

	if err := manager.Ensure(ctx, resource); err != nil {
		t.Fatalf("ensure carriers: %v", err)
	}
	if client.pvcUpdateCount != 0 {
		t.Fatalf("pvc updates = %d, want 0", client.pvcUpdateCount)
	}
}

type rejectPVCUpdateClient struct {
	ctrlclient.Client
	pvcUpdateCount int
}

func (c *rejectPVCUpdateClient) Update(ctx context.Context, obj ctrlclient.Object, opts ...ctrlclient.UpdateOption) error {
	if _, ok := obj.(*corev1.PersistentVolumeClaim); ok {
		c.pvcUpdateCount++
		return fmt.Errorf("unexpected pvc update")
	}
	return c.Client.Update(ctx, obj, opts...)
}
