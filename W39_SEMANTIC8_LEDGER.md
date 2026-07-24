# W39 semantic8 ledger

## Implemented facts

- A typed dotted function definition now publishes its resolved callable type
  through the normal static-member path-replacement contract.
- A later static write reports an assignment diagnostic only when its value is
  a concrete refutation of that published callable contract. Unknown and
  signature-less callable values remain unreported.
- The diagnostic uses the written value span and the member target span, and
  carries the published literal value plus the declared callable contract as
  its evidence chain.

## Oracle delta

The base failure set contains `143` fixture names and the final set contains
`142`. `comm -13 base.failures final.failures` is empty. The sole removed
failure is:

```text
semantic/reassigned-function-field-invalidates-callable-type
```

No `__legacy` source or immutable fixture data was modified.

## Verification

- `go build ./...`
- `go vet ./...`
- All non-engine package tests
- Engine semantic suite (`TestCheck|TestPublished|TestRecursive|TestValueAgainst`)
- Stage 1 (`TestStage1Red`)
- Front corpus (`fronttest`)
- Full oracle: `531/673` fixtures pass
