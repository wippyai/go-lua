# W46 placement ledger

Base: `3c98d3c5e` (`563/673`). Fixture data and `__legacy` were not modified.

## Resolved fixture

`semantic/placement-cross-module-owned-store`

The exporter now carries a placement ownership relation only for a public,
one-statement wrapper around the already-published `ownership.store` contract.
The relation records its two exact formal positions. At an imported call site,
the existing external-call boundary consumes that relation only after matching
the imported provider, member path, arity, and current allocation facts.

The stored-value position receives the existing owned placement event. The
owner position receives the same closed retaining contract, which discharges
the opaque-call blocker without promoting the owner graph. Thus the stored
graph is owned while the unrelated owner-side scaffolding remains stack-local.
Aliases, methods, extra statements, non-formal arguments, and opaque imports
publish no ownership relation.

## Oracle accounting

- Base failure set: 110 fixtures (`563/673`).
- Final failure set: 109 fixtures (`564/673`).
- Added failures: none.
- Removed failure: `semantic/placement-cross-module-owned-store`.

The exact `comm -13 base.failures final.failures` result is empty.
