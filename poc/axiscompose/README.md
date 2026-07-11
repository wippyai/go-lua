# Axis Composition POC

This directory is an isolated experiment. It is not imported by the production
state, solver, checker, summary, or call-boundary packages.

The experiment asks three narrow questions:

1. Can a sealed descriptor catalog add and remove product axes without editing
   the product driver?
2. Can each selected lane declare an exact boundary projection/instantiation
   capability while any used unsupported lane falls back to the complete
   contextual path without publishing a hybrid result?
3. Can solve-local per-lane content stamps safely avoid semantic `LessOrEq`
   scans when most persistent coordinates are unchanged?

## Model

`Catalog` registers typed `Spec[T]` values and erases them into one operation
table. `Catalog.Seal` produces an immutable `Schema` in catalog order. States
from different schemas cannot be compared; `Reconfigure` is the explicit
add/remove operation. Common axes preserve their values and stamps, added axes
start at lattice bottom, and removed axes disappear.

`Stamp` is a content-identity token allocated by a solve-local `Arena`. A
semantic no-op preserves its stamp. A changed write or newly computed join gets
a new stamp. A join equal to an operand carries that operand's stamp. Equal
stamps therefore imply semantic equality; unequal stamps make no semantic
claim. `ChangeMask(a,b)` is derived for that exact pair. There is deliberately
no standalone dirty bit.

Boundary composition is all-or-nothing. Projection records unsupported *used*
lanes in `Projection.Fallback`. Instantiation checks the mask before applying
anything, and a late capability failure discards partial work and invokes the
contextual callback exactly once. Unsupported lanes can be omitted only with a
sound caller-provided `Used` mask; `AllUsed` is the conservative default.

## Toy lanes

- `may.tags`: subset order, union join, exact boundary capability.
- `must.init`: reverse-inclusion order, intersection join, exact boundary
  capability.
- `may.alias`: a may lane with no boundary capability, used to prove fallback.

The must lane is intentional: a registry that assumes every fact family is a
may-union is not a viable product abstraction.

## Tests and benchmarks

The tests cover all eight selections of the three toy axes, lattice/product
laws, schema add/remove behavior, must polarity, exact boundary round trips,
unsupported and late-failure fallback, stamp-history independence, and 20,000
randomized differential comparisons against a full-scan baseline.

Focused benchmarks compare masked and full-scan `LessOrEq` with one changed
coordinate at 3, 17, 32, and 64 lanes, plus join and boundary paths:

```sh
go test ./poc/axiscompose
go test -bench=. -benchmem ./poc/axiscompose
```

No production recommendation follows merely from correctness or a synthetic
speedup. Promotion would require representative engine measurements showing at
least 85% fewer lane comparisons, no more than 5% regression when every lane
changes, no shared-worker contention, and no more than 5% RSS growth. The POC's
boxed state and inline slot slice are measurement scaffolding, not a proposed
production representation.

## Current synthetic result

On an AMD Ryzen 9 7950X3D with Go 1.23.3, a 500 ms benchmark run measured the
17-lane one-change comparison at 43.25 ns/op masked versus 77.39 ns/op for the
full scan (1.79x), both with zero allocations. The semantic lane scan count is
1 versus 17, a 94.1% reduction. At 64 lanes the measured speedup was 1.72x.

The result does **not** clear the proposed 3x synthetic speed threshold. It
also exposes representation costs: the 17-lane join measured 199.8 ns/op and
allocated a 416-byte slot slice, while exact boundary instantiation measured
65.35 ns/op with one 48-byte allocation. RSS and production all-lanes-change
cost have not been measured. These numbers justify keeping the POC available
for comparison; they do not justify adapting production State.
