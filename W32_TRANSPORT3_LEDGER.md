# W32 seam-1 transport-consumption ledger

## Fixed

| Fixture | First proof death | Repair |
|---|---|---|
| `flow/callback-result-preserves-type` | `call-results` held the sealed generic `apply` callable but received no call operands, so it could not instantiate `U` from the already-published callback result. | `apply` now publishes its exact argument references under its application coordinate; `call-results` consumes only those references. The generic result bridge admits a sealed canonical function only after complete structural argument inference, including contravariant callback parameters. |

## Blocked after the next eight-fixture trace

| Fixture | First proof death | Status |
|---|---|---|
| `flow/higher-order-function-types` | Generic `map` returns a child-built `{U}` whose container/optional-index witness is not published to the caller. | blocked: requires sealed generic container-result and index-presence transport. |
| `functions/return-call-wrong-arg` | The invalid `add("bad", 2)` call is inside `f`'s unadmitted child body. | blocked: requires child diagnostic publication for an invoked return body. |
| `regression/field-defined-wrapper-return` | `M.run` reaches its captured `M.dep.get` call without a cross-body current-member result witness. | blocked: requires captured heap-member result transport. |
| `regression/field-defined-wrapper-return-local-alias` | The static `M.run` member has no published callable surface at the alias assignment. | blocked: requires current member-callable projection across the table boundary. |
| `regression/field-defined-wrapper-return-local-alias-reassigned` | The reassigned `M.run` member is Top at the assignment consumer, so the proven `nil` callable surface is absent. | blocked: requires current member write-to-callable publication. |
| `regression/non-dominating-field-defined-wrapper-return` | The conditional `M.dep` write is not joined into the child call result as `nil | {answer: string}`. | blocked: requires branch-sensitive captured member-result join. |
| `regression/reassigned-field-call-assignment` | The direct reassigned member call has no wrapper-return projection at its result consumer. | blocked: requires the same current-member result transport as the wrapper family. |

## Controls

- `testdata/fixtures` and `__legacy` remain unchanged.
- The full-oracle set-diff is measured against the base 510/673 failure set.
