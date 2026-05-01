# code-code-platform

Aggregate platform control-plane and runtime Go services for Code Code.

This repository owns:

- `packages/platform-k8s`: aggregate platform service implementations, controllers, workers, runtime orchestration, provider/auth/egress/model/profile services.
- `packages/platform-contract`: platform-owned Go domain contracts.
- `packages/agent-runtime-contract`: agent runtime Go contracts.
- `packages/session`: shared session and transcript domain logic used by platform-facing Go services.

Focused service splits now exist:

- `code-code-platform-catalog`: model, support, and CLI runtime catalog services.
- `code-code-platform-auth-network`: auth, OAuth, egress, and network policy services.
- `code-code-platform-profile`: profile configuration service.
- `code-code-platform-notifications`: notification dispatcher and callback services.

This split preserves source history from the original monorepo. Contract
dependency migration is the next step: platform code should consume
`code-code-contracts` through versioned dependencies instead of local generated
contract paths.

Useful checks:

```bash
cd packages/platform-k8s && go test ./...
cd packages/platform-contract && go test ./...
cd packages/agent-runtime-contract && go test ./...
cd packages/session && go test ./...
```
