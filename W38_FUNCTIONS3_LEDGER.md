# W38 functions3 ledger

## Fixed facts

- A child-body colon call now recovers an exact formal receiver type from the
  existing WIR boundary publication. A declared `string` receiver therefore
  resolves through the standard-library contract rather than an opaque global
  named after the parameter.
- An uncalled, straight-line child may consume that published method contract
  only for a formal receiver and only to publish a missing-member or declared
  return-contract diagnostic. Branching bodies remain demand-driven.
- A method is reported missing only when the receiver's published declared
  surface is closed and neither the canonical member graph nor the standard
  library publishes that capability.

## Oracle scorecard

| Run | Passing fixtures | Failures |
| --- | ---: | ---: |
| Base `17ffd42f7` | 526/673 | 147 |
| Final | 528/673 | 145 |

Removed failures:

- `functions/method-on-union-fails`
- `functions/string-method-resolution`

`comm -13 base.failures final.failures` produced no entries; the regression
set difference is zero. The final functions category is `71 pass / 0 fail`.

## Verification

- `go build ./...`
- `go vet ./...`
- All non-engine package tests: `go test $(go list ./... | rg -v '^github\\.com/wippyai/go-lua/analysis/check/engine$') -count=1`
- Engine suites: `go test ./analysis/check/engine -run '^(TestCheck|TestPublished|TestRecursive|TestValueAgainst)' -count=1`
- Stage 1: `go test ./analysis/check/engine -run '^TestStage1Red' -count=1`
- Front corpus: `go test ./analysis/check/fixpoint/front/fronttest -count=1`
- Full oracle: `go test ./analysis/check/engine -run '^TestFullOracle$' -count=1 -v`

No fixture data or `__legacy` files were modified.
