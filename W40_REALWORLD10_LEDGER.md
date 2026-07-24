# W40 realworld10 ledger

## Published fix

`make()[1]` is an immediate indexed temporary. When its existing type witness
is optional and the non-nil projection has the requested method, calling that
method without a nil guard now publishes the ordinary direct-call failure. A
named receiver is intentionally excluded: it may have a dominating truthiness
guard whose refinement is not represented by its value alone.

The generic import regression test also records the compatible-later-argument
case: a reducer callback can establish `A = number` while a literal initial
value supplies the narrower `integer` inhabitant.

## Per-fixture tail census

| Fixture(s) | Disposition |
| --- | --- |
| `recursive-alias-array-index` | Fixed: optional indexed method receiver now reports the checked-in unguarded call failure. |
| `advanced-type-system-stress` | Deferred: cross-module optional-member and union diagnostics. |
| `agent-workflow-engine`, `cqrs-order-runtime`, `event-bus-saga-runtime`, `middleware-session-router`, `plugin-runtime-pipeline`, `plugin-supervisor-runtime`, `transactional-saga-orchestrator` | Deferred: cyclic runtime evaluation reaches the conservative boundary. |
| `agent-workflow-engine-soundness`, `cqrs-order-runtime-soundness`, `event-bus-saga-runtime-soundness`, `middleware-session-router-soundness`, `notification-delivery-runtime-soundness`, `plugin-runtime-pipeline-soundness`, `plugin-supervisor-runtime-soundness`, `tenant-policy-runtime-soundness`, `transactional-saga-orchestrator-soundness` | Deferred: multi-module optional/call/index rejection publications are absent or incomplete. |
| `iterator-pipeline` | Deferred: project export-relation transport still collapses generic intermediate results, while the direct imported-type path is covered separately. |
| `notification-delivery-runtime`, `tenant-policy-runtime` | Deferred: multi-step runtime summaries and union-array claim proofs. |

No `testdata/fixtures` or `__legacy` file was changed.
