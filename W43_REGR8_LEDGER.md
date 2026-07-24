# W43 regression-8 ledger

Base: `ad47d30bf` (`551/673`). No file in `testdata/fixtures` changed, and
`__legacy` was not modified or used as implementation authority.

## Resolved fixture

| Fixture | Required published fact | Result |
| --- | --- | --- |
| `core/narrow-no-check-fails` | exact declared optional-formal witness at a direct, unguarded annotation | resolved |

An uncalled, capture-free child may now enter its existing
declaration-owned boundary path when an annotation consumes an exact,
non-`any` declared formal before control flow. The child receives the already
encoded formal type witness and publishes the ordinary assignment mismatch.
Guarded direct-formal assignments remain demand-driven because their
refinement is caller-owned; arbitrary locals, inferred values, branches, and
call results are not admitted by this path.

## Supporting fail-closed correction

An unchecked cast keeps its existing exact-result witness for direct consumers
such as indexing, but that witness no longer serves as member-origin authority
when constructing a later aggregate. This restores the existing
`TestCheckDoesNotPublishUncheckedCastTypeWitness` contract without regressing
`types/cast-then-index`.

## Oracle accounting

- Base failure set: 122 fixtures (`551/673`, `315/630` diagnostics hit).
- Final failure set: 121 fixtures (`552/673`, `317/630` diagnostics hit).
- Added failures: none.
- Removed failure: `core/narrow-no-check-fails`.
- `comm -13 base.failures final.failures` is empty.

See `W43_REGR8_SCORECARD.md` for the complete gate results.
