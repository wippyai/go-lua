# W33 realworld6 ledger

## Tail census

The realworld oracle was run directly at base `c1deb1f3e`. It found 47
fixtures: 25 pass and 22 fail. The 22 failing names are exactly the entries
already marked **Blocked / deferred** in `W31_REALWORLD5_LEDGER.md`:

- `advanced-type-system-stress`
- `agent-workflow-engine`, `agent-workflow-engine-soundness`
- `cqrs-order-runtime`, `cqrs-order-runtime-soundness`
- `event-bus-saga-runtime`, `event-bus-saga-runtime-soundness`
- `iterator-pipeline`
- `middleware-session-router`, `middleware-session-router-soundness`
- `notification-delivery-runtime`, `notification-delivery-runtime-soundness`
- `plugin-runtime-pipeline`, `plugin-runtime-pipeline-soundness`
- `plugin-supervisor-runtime`, `plugin-supervisor-runtime-soundness`
- `recursive-alias-array-index`
- `tenant-policy-runtime`, `tenant-policy-runtime-soundness`
- `trait-registry`
- `transactional-saga-orchestrator`, `transactional-saga-orchestrator-soundness`

No fixture, test, or engine fact path was changed. The remaining 25 realworld
fixtures, including the previously repaired `discriminated-tool-dispatch`,
pass their checked-in expectations. Therefore this lane deliberately makes no
new claim publication: each possible failing target is excluded by the
instruction to skip marked-blocked work.

## Disposition

This is an analysis-only, neutral lane. `testdata/fixtures` and `__legacy`
remain untouched. A later implementation lane needs a newly authorized proof
mechanism or an explicit decision to revisit one of the deferred seams before
the realworld pass count can rise.
