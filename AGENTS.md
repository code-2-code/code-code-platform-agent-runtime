# Agent Rules

- This repository owns AgentSession, AgentSessionAction, AgentRun, timeline,
  and Temporal-backed runtime orchestration behavior.
- Keep workflow code deterministic: no network, database, Kubernetes, random,
  wall-clock, goroutine, channel, or blocking APIs inside workflow functions.
- Put side effects in activities or service/domain code outside workflow
  functions.
- Do not edit protobuf source or generated contract bindings here.
- If a public contract must change, make that change in `code-code-contracts`
  first, then update this repository to the released contract version.
- Do not move provider, auth, egress, profile, catalog, notification, UI, or
  deployment behavior into this repository.
- Keep changes narrow to one runtime/session use case at a time.
