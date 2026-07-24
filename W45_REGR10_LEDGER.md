# W45 regression-10 ledger

Base: `4e1cf1e74e21b2cec65ac37e91859739a0e235a2` (`560/673`). No file below
`testdata/fixtures` or `__legacy` was changed.

## Resolved fixture

| Fixture | Closed published fact | Result |
| --- | --- | --- |
| `regression/error-return-second-slot-contract` | The exact local callable's sealed optional result, transported only through its paired call-result and local-write epochs to the static `cfg.host` read. | resolved |

`FieldAtPath` structurally selects a member through an optional receiver. The
engine now restores nilability only when the same path carries the exact local
call-result marker published by the existing call-results owner. The marker is
then propagated solely by the normal local-write boundary. Imported summaries,
annotations, and unrelated optional fields do not enter this path. Existing
indexed-read nilability remains unchanged.

## Oracle accounting

- Base failure set: 113 fixtures (`560/673`).
- Final failure set: 112 fixtures (`561/673`).
- Added failures: none.
- Removed failure: `regression/error-return-second-slot-contract`.
- `comm -13 base.failures final.failures` is empty.

See `W45_REGR10_SCORECARD.md` for gate results.
