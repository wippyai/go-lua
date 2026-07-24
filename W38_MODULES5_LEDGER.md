# W38 modules5 ledger

## Fixed

| Fixture | Existing publication consumed | Result |
| --- | --- | --- |
| `modules/imported-record-return-literal` | The imported record already carries the host-published `time.Time` interface on `opened_at`. Method-result projection now falls back to the canonical interface member resolver only after the existing record/union projection is unavailable. | `now:sub(opened_at):seconds()` retains the published `number` result witness; the false `lint.claim.unproven` is removed. |

## Guardrails

- The interface fallback runs only when the existing `variant.FieldAtPath`
  projection cannot resolve the member, preserving record and union behavior.
- No source spelling, annotation, or synthetic result is used as authority;
  the receiver must already have an encoded type witness and the interface
  method must already be present in that witness.
- Added an engine regression test for an imported interface receiver. It proves
  a declared method result is usable, while the full-oracle set-diff proves no
  fixture regression.
- `testdata/fixtures` was untouched. `__legacy` was not modified.
