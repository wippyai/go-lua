# Symbolic call composition POC

This package is isolated from the production checker. It tests whether the
existing product lattice and WTO solver can support solve-once/apply-many call
composition without changing the engine's 17 state lanes or actor semantics.

The first layer composes unconditional parameter/constant/join expressions.
The guarded layer adds correlated multi-return rows: a guard applies to the
whole return tuple, so `(ok, number)` and `(error, string)` never become four
independent combinations. Unknown guards retain rows; contradictory guards
drop impossible rows; missing Lua return slots become absent/nil rather than
lattice Bottom.

Requirements are a separate contravariant lane. Equal `(guard,param)` keys
combine with `product.Meet`; Top canonicalizes to no requirement; Bottom and
requirement-budget overflow force contextual fallback. Return-row widening may
weaken guards or collapse many rows to one unguarded slotwise join. It never
silently weakens a requirement.

Recursive transformer equations use `analysis/engine/solve`'s WTO directly.
The POC does not implement another SCC engine.

The boundary layer adds namespace-distinct parameter, lexical-capture, and
vararg roots. A closure environment is bound explicitly when the transformer
is instantiated. Vararg rows carry an exact or ranged pack length alongside
positional expressions, which preserves the distinction between an absent
position and an explicitly supplied nil. Entry requirements remain a separate
contravariant lane and are checked after binding. Canonical row/expression
ordering is structural; a product-hash collision fails closed rather than
silently deciding order by allocation identity.

The boundary transformer itself is value-only: heap mutation, allocation,
typestate, channels, escape, placement, and actor state require an explicit
effect capability rather than being smuggled through a value expression.

The effects layer then tests the next boundary without enabling it in the
checker. Module globals have structural `(module,name)` roots and are supplied
explicitly at instantiation; mutable ambient globals are never read implicitly.
Fresh allocation sites are rebased by call-instance identity, so two callers
cannot accidentally share an abstract object. Writes to a fresh, non-escaped
site are strong; writes to captures, module roots, or pre-existing heap cells
weak-join. Escaping heap, cross-actor heap, mailboxes, and actor state fall back
as one transformer. Return values, returned references, allocations, and writes
remain correlated in the same effect row.

Composition capabilities are not hardcoded as a switch over today's 17 State
lanes. A registry is derived from `state.DefaultLanes`, and each lane adapter
implements summarize, substitute, and effect-join. The values adapter delegates
the complete registered `product.Value`, so product-axis additions retain the
product domain's modularity. Missing or newly registered State lanes default to
contextual fallback; adding support is one adapter, not a call-engine rewrite.

The syntax census is therefore an optimistic ceiling, not a count of functions
that could be switched to composition today. The POC implements the values
lane and the new root/allocation/heap boundary components; the other 16 default
State lanes still intentionally report contextual. Production eligibility must
intersect syntactic capability with this per-lane registry before any default
can change.

Run:

```sh
go test ./poc/symboliccall
go test -bench=. -benchmem ./poc/symboliccall
```

Passing POC laws are necessary but not sufficient for production integration.
The behavior-neutral program census decides which capability has enough real
body-solve cost behind it to justify the next implementation slice.
