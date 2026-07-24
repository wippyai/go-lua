# W44 regression-9 ledger

Base: `f9d613109` (`556/673`). No fixture or `__legacy` file was changed.

## Resolved fixture

| Fixture | Closed published fact | Result |
| --- | --- | --- |
| `regression/concat-operand-narrows-inferred-optional` | The registered `string.match` result's finite nilability, carried through its exact call-result and local-write epochs to the concat operand. | resolved |

The ordinary call-result value remains `Top`. Its optional provider fact is
available only to the same current concat operand, so it cannot become an
assignment, member-access, or general call-result proof. The guarded sibling
remains demand-driven and emits no warning.

## Oracle accounting

- Base failure set: 117 fixtures (`556/673`).
- Final failure set: 116 fixtures (`557/673`).
- Added failures: none.
- Removed failure: `regression/concat-operand-narrows-inferred-optional`.
- `comm -13 base.failures final.failures` is empty.
