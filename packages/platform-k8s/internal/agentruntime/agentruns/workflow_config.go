package agentruns

const (
	agentRunWorkflowKind = "agent-run-execution"
	prepareJobAction     = "prepare-agent-run-job"
	cleanupRunAction     = "cleanup-agent-run"
)

type agentRunWorkflowConfig struct {
	ExecutionNamespace    string
	TriggerHTTPBaseURL    string
	CLIOutputSidecarImage string
}
