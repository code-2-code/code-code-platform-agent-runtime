# Agent Rules

- This repository owns platform runtime and control-plane implementation.
- Do not edit protobuf source or generated contract bindings here.
- If a public contract must change, make that change in `code-code-contracts` first, then update this repository to the released contract version.
- Keep state ownership inside the platform domain. Do not move UI or deployment-only behavior into platform packages.
- Keep changes narrow to one platform service/domain at a time.
