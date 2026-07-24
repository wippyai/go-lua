# W43 narrowing congruence ledger

## Published fact

A true branch may project a non-nil member across a connected set of exact
path-equality facts only when both member paths have the same existing
published type. The projection is a guarded correlation value, not a heap
write.

Each projection records the current epoch of the source member, target
member, and every enclosing path segment. Reads accept it only while every
recorded epoch is still current. A reassignment therefore revokes only the
correlation cone that named that path; it cannot invalidate independent branch
or member facts.

Malformed cones, absent type publications, unequal member types, dynamic
aliases, and paths outside a complete static suffix fail closed.

## Scope

- No `testdata/fixtures` file changed.
- No `__legacy` source changed or supplied implementation authority.
- No fixture-specific names or test-only controls were introduced.

## Verification

- `go build ./...`
- `go vet ./...`
- all non-engine package tests
- `go test ./analysis/check/engine -run '^TestStage1Red' -count=1`
- `go test ./analysis/check/fixpoint/front/fronttest -count=1`
- targeted `narrowing/congruence-access` oracle
- full oracle, with an isolated-base exact set comparison: `552/673`, zero
  regressions, one rise

`TestCheckDoesNotPublishUncheckedCastTypeWitness` fails at both base and final
with the same assertion; it is unrelated to this change and is not counted as
an oracle regression.
