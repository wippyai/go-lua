# W41 cyclic5 ledger

## Fixed fixture

`realworld/plugin-supervisor-runtime` now passes its checked-in clean-check
expectation. Its `requests` array contains an explicit `DispatchRequest ::
Request` cast. The cast's structural relation is already proven, but cyclic
evaluation previously retained only its scalar value; the enclosing array
claim therefore could not read a published type for that member.

The claim kernel now publishes a cast type witness only when the cast's own
structural relation is proven. The existing member-origin and aggregate
relation machinery consumes that publication. An unchecked assertion remains
non-authoritative and cannot discharge a later aggregate annotation.

## Guard

`TestCheckDoesNotPublishUncheckedCastTypeWitness` proves that `5 :: string`
does not make `{value}` satisfy `{string}`. This preserves the fail-closed
boundary for unchecked casts.

No `testdata/fixtures` or `__legacy` content was modified.
