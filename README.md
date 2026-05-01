# code-code-platform-agent-runtime

Agent session, AgentRun, timeline, and Temporal-backed runtime service for
Code Code.

This repository owns:

- `packages/platform-k8s/internal/agentruntime`: AgentSession,
  AgentSessionAction, AgentRun, timeline, runtime materialization, and runtime
  workflow behavior.
- `packages/platform-k8s/cmd/platform-agent-runtime-service`: agent runtime
  service and worker entrypoint.
- `code-code-contracts`: generated shared contracts as a Git submodule.
- `code-code-platform-session`: session persistence and platform domain helper
  packages as a Git submodule.

Useful checks:

```bash
cd packages/platform-k8s && go test ./internal/agentruntime/... ./cmd/platform-agent-runtime-service
```
