# W44 placement ledger

Fixture: `placement/transitive-cross-module-send-escape`.

## Diagnosis

Temporary test-only projection dump (removed before this patch) reported four
published allocations from `alloc` and `pass`, all `lua.table`/`lua.closure`
owned heaps. `main` published only call arguments, including the payload
`path/sym4`; it published neither a placement allocation nor a placement
binding for that returned payload. Consequently `process.send` had no exact
allocation identity to seal and mark shared.

The missing witness was a fresh returned-allocation graph at the imported
call result, not a coroutine suspension or a send-contract witness.

## Fix

An exporter now carries a finite table-return relation through a strict local
table-return or a one-step imported forwarding wrapper. The current exported
callable surface and the imported relation must both already be published.
At the exact consumer call result, the engine instantiates only that finite
relation as `manifest.allocation` placement facts. Ordinary assignment emits
the binding, and the existing `process.send` rule is the sole source of the
shared/sealed event. No source-name lookup, inferred type shape, or opaque
call can manufacture the witness.

The fixture now has two shared `manifest.allocation` sites with depth two.
