package agentruns

import (
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

type RunActionInput struct {
	Action string
	Body   []byte
}

func AgentRunWorkflow(ctx workflow.Context, input TemporalWorkflowInput) (runErr error) {
	activityCtx := workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
		StartToCloseTimeout: time.Hour,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval: time.Second,
			MaximumInterval: time.Minute,
			MaximumAttempts: 3,
		},
	})
	defer func() {
		cleanupCtx, _ := workflow.NewDisconnectedContext(activityCtx)
		cleanupErr := workflow.ExecuteActivity(cleanupCtx, triggerRunActionActivity, RunActionInput{Action: cleanupRunAction, Body: input.CleanupBody}).Get(cleanupCtx, nil)
		if runErr == nil {
			runErr = cleanupErr
		}
	}()
	for _, body := range input.PrepareBodies {
		runErr = workflow.ExecuteActivity(activityCtx, triggerRunActionActivity, RunActionInput{Action: prepareJobAction, Body: body}).Get(activityCtx, nil)
		if runErr != nil {
			break
		}
	}
	if runErr == nil {
		runErr = workflow.ExecuteActivity(activityCtx, executeRunJobActivity, input).Get(activityCtx, nil)
	}
	return runErr
}
