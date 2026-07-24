# W31 SEAM-2 partitions ledger

## Fixed

None. No implementation change survived verification.

## Blocked

- `narrowing/congruence-access`: no child-body partition is evaluated for the
  returned parameterized closures, so neither equality nor member-refinement
  publications reach the three annotated reads.
- `narrowing/partitioning/dependent-reassigned` and
  `discriminant-reassigned`: their guarded call operations are in the same
  unevaluated returned child body. The pre-call `x = nil` write is present in
  the artifact, but has no published consumer partition.
- `narrowing/partitioning/channel-identity-result-reassigned-no-stale-fact`:
  the select child is evaluated, and stale select correlation is revoked, but
  the post-reassignment member read resolves through the sealed literal heap
  fact as definite `nil` instead of the required declared `string | nil`
  member summary. A trial declaration-summary publication did not reach this
  consumer's partition and was reverted.

## Guardrails

- No files under `testdata/fixtures` changed.
- No files under `__legacy` changed or supplied implementation authority.
- The requested `673` total, a count rise over `500`, and a zero-regression
  set-diff have not been achieved; this lane must not be merged as a fix.
