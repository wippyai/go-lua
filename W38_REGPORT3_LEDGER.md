# W38 regport3 ledger

## Ported fixture

| Fixture | Existing publication consumed | Projection correction |
| --- | --- | --- |
| `regression/field-defined-wrapper-return-local-alias-reassigned` | `assignment-member-surface` for the closed reassigned `M.run` callable | The published diagnostic preserves `M.run` as the proven source and places the `f` annotation assertion at its claim target span. |

The correction does not infer a callable value and does not inspect fixture
source during projection. It is reached only when the existing closed
member-surface publication is present. Without that fact, the normal
assignment projection remains unchanged and fail-closed.

## Regression coverage

`TestCheckPublishesReassignedMemberCallableAssignmentEvidence` checks the
production publication boundary: closed callable evidence, distinct claimed
target evidence, labels, and remediation text.

## Scope audit

- Hand-reconciled from the dirty `w37-regression5` source lane against
  `17ffd42f7`.
- No `__legacy` source changed.
- No `testdata/fixtures` file changed.
- No fixture expectations or tests were weakened, skipped, or replaced.
