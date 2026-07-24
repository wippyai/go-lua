# W33 blocked-fixture retriage ledger

## Method

Every `w24`–`w32` scratchpad ledger log was searched after stripping ANSI CSI
escapes.  Repeated transcript copies were collapsed by fixture name.  This
ledger records named fixture entries actually marked `blocked`, `deferred`, or
`root-caused-blocked`; broad historical failure censuses were not promoted into
blocked entries merely because they contained an old failure.

The current hard oracle was then rerun on the affected categories.  The base
was `c1deb1f3e` at `513/673`.

## Now passing: no code change needed

- `modules/host-global-qualified-type`
- `realworld/module-with-generics`
- `semantic/cast-any-remains-untrusted`
- `semantic/channel-selected-value-assignment`
- `semantic/concat-operand-diagnostics-evidence`

These prior blockers are already covered by landed host-publication,
recursive-generic, boundary, and diagnostic-composition work.

## Newly fixable and fixed

- `regression/field-defined-wrapper-return`
- `regression/constructor-return-variant-inference`

Both were held back by the same conservative admission condition: a closed
local child returning an inferred table was rejected before its actual
one-result return publication could reach the caller.  The engine now admits
that existing publication while retaining the exclusions for declared
multi-return and recursive return graphs.

## Still blocked (current root cause confirmed)

| Area | Fixtures | Current proof boundary |
| --- | --- | --- |
| flow/functions | `flow/higher-order-function-types`; `functions/return-call-wrong-arg` | Generic container/index witness and invoked-child diagnostic publication are still absent. |
| modules | `active-session-any-time-sub-soundness`; `arithmetic-param-rejects-cross-module-nonnumber`; `google-client-metadata-regression`; `imported-eq-typeof-table-len`; `imported-field-cast-expected-record`; `imported-helper-forwards-arg-to-typed-method`; `imported-not-nil-field-typeof-table-len`; `imported-record-return-literal`; `imported-stable-local-function-export`; `providers-open-retry-captured-options`; `providers-open-retry-captured-options-realtest` | No sealed provider normal-return, container/presence, receiver-result, or captured imported-module relation reaches the consumer. |
| narrowing | `congruence-access`; `partitioning/dependent-reassigned`; `partitioning/discriminant-reassigned`; `partitioning/channel-identity-result-reassigned-no-stale-fact` | Returned/selected child partitions lack the guarded consumer publication; the channel case retains a concrete stale nil instead of the declared nilable member summary. |
| realworld | `advanced-type-system-stress`; `agent-workflow-engine`; `cqrs-order-runtime`; `event-bus-saga-runtime`; `middleware-session-router`; `plugin-runtime-pipeline`; `plugin-supervisor-runtime`; `transactional-saga-orchestrator`; their listed `*-soundness` counterparts; `iterator-pipeline`; `notification-delivery-runtime`; `tenant-policy-runtime`; `recursive-alias-array-index`; `trait-registry` | The remaining paths stop at timeout, generic-result, optional/index, or multi-step runtime-summary boundaries. |
| regression | `field-defined-wrapper-return-local-alias`; `field-defined-wrapper-return-local-alias-reassigned`; `non-dominating-field-defined-wrapper-return`; `reassigned-field-call-assignment` | Alias/reassignment needs a current member-callable fact; the branch case needs a guarded captured member-result join. |
| semantic | `array-length-floor-index-read`; `channel-summary-witness-composition`; `nested-channel-select-union-stress`; `typed-channel-coroutine-boundaries`; `channel-send-escape`; `concat-nilability-provenance`; `static-bracket-member-read-diagnostic` | The surviving gaps are escape/write invalidation, incomplete select temporaries, shared channel placement, equal-span evidence ordering, or an uncalled child read anchor. |
| types | `cast-then-index`; `cast-type-is-direct`; `cast-type-is-falsy-fail`; `cast-type-is-not-fail`; `recursive-mismatch-rejected`; `string-stdlib-return-types` | Optional/index, predicate-branch, recursive-member evidence, and conservative multi-return result-slot facts remain unproved. |

No fixture data or `__legacy` source was modified.
