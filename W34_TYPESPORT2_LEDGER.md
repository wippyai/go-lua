# W34 typesport2 ledger

## Reconciliation

`patch_w33_types4.patch` was based on the earlier engine blob
`2e08eabeb`, so a direct apply failed against this lane's `1a189f5bf` engine
shape. The port retains the newer root/child diagnostic publication behavior
and applies the patch's guarded structural-assignment projection on top of it.

## Implemented facts

- A sealed table assignment to a record now identifies the first already
  refuted record member for its existing assignment diagnostic.
- The diagnostic's source anchor comes only from the exact WIR table-entry
  value span. If that source metadata, a closed shape, a matching claim, or a
  refuting member is absent, the ordinary claim span and message remain.
- Recursive record declarations are unwrapped only for the existing finite
  shape relation; unproved and open shapes remain unprojected.
- Explicit `any` boundaries retain their established authoritative diagnostic
  path and cannot be replaced with an initializer-member diagnostic.
- The regression test covers a recursive `TreeNode` record whose `label`
  member is a proven `number`/`string` mismatch, including its field span and
  two-piece evidence chain.

## Oracle delta

The base failure set contains 157 fixtures and the port set contains 156.
`comm -13 base.failures port.failures` is empty: there are zero newly failing
fixtures. `comm -23 base.failures port.failures` contains exactly:

```text
types/recursive-mismatch-rejected
```

This is the one-pass rise from 516/673 to 517/673. The full oracle's nonzero
exit is expected because the remaining fixtures still fail their checked-in
expectations; it is not masked or skipped.
