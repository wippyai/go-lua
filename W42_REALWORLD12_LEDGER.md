# W42 realworld12 ledger

## Published fix

Generic module calls now infer arguments from only the newest exact
publication owned by the argument term. An instantiated import result carries
its canonical `summary-type` through the next call edge; a checked annotation
can carry its matching `type` publication. Both require the matching latest
value operation, so a stale annotation, an entry declaration, unresolved type
parameter, `any`/`unknown` boundary, or recursive graph fails closed.

This keeps the `User` array type argument and the `string` then `number`
intermediate summaries of `realworld/iterator-pipeline` through
`filter -> map -> map -> reduce` to the final `number` consumer.

## Oracle delta

| Run | Passing fixtures | Failures |
| --- | ---: | ---: |
| Base `341a04e49` | 549/673 | 124 |
| Final | 550/673 | 123 |

Removed failure:

- `realworld/iterator-pipeline`

Exact full-oracle failure-set comparison: zero added failures and one removed
failure. No file under `testdata/fixtures` and no `__legacy` source changed.

## Verification

- `go build ./...`
- `go vet ./...`
- all non-engine package tests
- `go test ./analysis/check/engine -run '^TestStage1Red$' -count=1`
- `go test ./analysis/check/fixpoint/front/fronttest -count=1`
- targeted generic-summary regression tests
- full oracle: `550/673`
