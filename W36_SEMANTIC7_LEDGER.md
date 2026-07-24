# W36 semantic7 ledger

## Implemented facts

- An assignment from a published recursive optional type to a target that
  excludes nil now reports the decisive nilability refutation. It does not use
  the recursive present-member shape, whose callable parameter spellings can
  differ without being the reason the assignment is invalid.
- Exact invoked child entries now publish their already-computed assignment
  diagnostics, alongside the existing direct-call and concat contracts. Body
  nesting is not a reason to hide a diagnostic after the child entry completed.

## Oracle delta

The base failure set contains `153` fixture names and the final set contains
`152`. `comm -13 base.failures final.failures` is empty. The sole removed
failure is:

```text
semantic/recursive-method-table-chain
```

No `__legacy` source or immutable fixture data was modified.
