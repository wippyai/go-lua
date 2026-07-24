# W31 realworld5 ledger

## Fixed

- `realworld/discriminated-tool-dispatch`
  - Proof previously died at `ipairs(batch_results)`: the resolved
    `ToolResult[]` publication reached the iterator call, but its generic-for
    result bindings were Top.
  - The call-result seam now reads the existing standard-library indexed
    iterator effect and publishes the element type only when the exact source
    is a closed typed array. The generic-for seam consumes that publication;
    it publishes the integer index and the sealed element witness.

## Blocked / deferred

These remain failing after the per-fixture census and were not changed in this
lane because they need distinct proof mechanisms rather than another iterator
publication:

- `advanced-type-system-stress` — optional-member and union diagnostics.
- `agent-workflow-engine`, `cqrs-order-runtime`, `event-bus-saga-runtime`,
  `middleware-session-router`, `plugin-runtime-pipeline`,
  `plugin-supervisor-runtime`, `transactional-saga-orchestrator` — cyclic
  evaluation reaches the conservative timeout boundary.
- `agent-workflow-engine-soundness`, `cqrs-order-runtime-soundness`,
  `event-bus-saga-runtime-soundness`, `middleware-session-router-soundness`,
  `notification-delivery-runtime-soundness`, `plugin-runtime-pipeline-soundness`,
  `plugin-supervisor-runtime-soundness`, `tenant-policy-runtime-soundness`,
  `transactional-saga-orchestrator-soundness` — optional/call/index rejection
  publication remains absent or incomplete.
- `iterator-pipeline`, `module-with-generics` — generic callable result
  instantiation, independent of indexed iterator element transport.
- `notification-delivery-runtime`, `tenant-policy-runtime` — multi-step
  runtime summary and union-array claim proofs.
- `recursive-alias-array-index`, `trait-registry` — optional indexed access
  diagnostic publication.
