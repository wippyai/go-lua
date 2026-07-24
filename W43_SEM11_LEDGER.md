# W43 semantic continuation ledger

Base: `ad47d30bf` (`551/673`). Fixture data and `__legacy` were not modified.

## Published fact

An exact dynamic write now mirrors its replacement through the deepest
already-published member-identity chain. This covers an aliased nested table
member such as `slots[key].value`: the literal key is resolved, every identity
hop must have been previously published, and only the remaining exact member
is updated. Missing identities, non-literal keys, and unresolved values retain
the existing fail-closed behavior.

## Oracle delta

| Run | Passing fixtures | Failing fixtures |
| --- | ---: | ---: |
| Base | 551/673 | 122 |
| Final | 553/673 | 120 |

Removed failures:

- `semantic/dynamic-key-variant-write-invalidates-alias`
- `semantic/dynamic-key-variant-write-invalidates-guard`

Exact set comparison: `0` added failures and `2` removed failures.

## Verification

- `go build ./...`
- `go vet ./...`
- all non-engine packages: `go test $(go list ./... | rg -v '/analysis/check/engine$') -count=1`
- engine publication/recursive/value comparison suite
- `go test ./analysis/check/engine -run '^TestStage1Red' -count=1`
- `go test ./analysis/check/fixpoint/front/fronttest -count=1`
- focused alias/dynamic-write engine regressions
- full fixture oracle: `553/673`

`TestCheckDoesNotPublishUncheckedCastTypeWitness` remains failing in an
unmodified clean clone at `ad47d30bf` with the same `claim "string" is not
proven` result; it is outside this change and was not weakened or changed.
