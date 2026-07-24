# W41 realworld11 ledger

## Published fix

`realworld/plugin-supervisor-runtime` builds a sealed `Request[]` from
declared request values, including one successful `:: Request` cast. The
engine now preserves the exact target of a structurally successful cast as an
existing publication. The sealed-container relation consumes that publication
only for the cast result that is recorded as a direct member origin.

Nested table members are descendants of an existing direct container entry,
not independent array or map entries, and are therefore excluded from the
homogeneous-entry origin walk. Invalid paths and any missing origin or type
witness still fail closed.

## Oracle delta

| Run | Passing fixtures | Failures |
| --- | ---: | ---: |
| Base `144dc6350` | 544/673 | 129 |
| Final | 545/673 | 128 |

Removed failure:

- `realworld/plugin-supervisor-runtime`

The full-oracle set difference contains zero regressions. No fixture data or
`__legacy` source was changed.

## Verification

- `go build ./...`
- `go vet ./...`
- All non-engine package tests
- Engine suites: `TestCheck|TestPublished|TestRecursive|TestValueAgainst`
- Stage 1: `TestStage1Red`
- Front corpus: `fronttest`
- Full oracle: `545/673`
