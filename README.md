# code-code-platform-agent-runtime

Agent session, AgentRun, timeline, and Temporal-backed runtime service for
Code Code.

This repository owns:

- `packages/platform-k8s/internal/agentruntime`: AgentSession,
  AgentSessionAction, AgentRun, timeline, runtime materialization, and runtime
  workflow behavior.
- `packages/platform-k8s/cmd/platform-agent-runtime-service`: agent runtime
  service and worker entrypoint.
- `packages/session`: session persistence and transcript domain logic.
- `packages/platform-contract`: platform-owned Go domain contract helpers used
  by session and runtime state projection.
- `code-code-contracts`: generated shared contracts as a Git submodule.

Useful checks:

```bash
cd packages/platform-contract && go test ./...
cd packages/session && go test ./...
cd packages/platform-k8s && go test ./internal/agentruntime/... ./cmd/platform-agent-runtime-service
```
