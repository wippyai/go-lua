# W41 semantic9 ledger

## Implemented fact

`table.insert(array, tableValue)` now carries the table value's already
published heap identity to the exact appended member. A subsequent exact
indexed read can therefore follow the existing member-identity chain through
nested fields. No identity is created when the inserted value has none.

This removes the false `first.parent.id is nil` assignment diagnostic from
`semantic/recursive-tree-parent-child`; the parent value is the exact table
that was inserted into `root.children`.

## Oracle delta

The base failure set has 129 fixtures and the final set has 128. There are no
added failures. The sole removed failure is:

```text
semantic/recursive-tree-parent-child
```

No `__legacy` source or immutable fixture data was modified.
