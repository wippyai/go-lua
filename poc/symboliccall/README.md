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

Heap-mutated captures, allocation identity, globals, typestate, channel,
escape, placement, actor state, and operational-effect composition stay
unsupported and therefore contextual. Supporting immutable lexical captures
does not imply that mutable closure cells are safe to summarize.

Run:

```sh
go test ./poc/symboliccall
go test -bench=. -benchmem ./poc/symboliccall
```

Passing POC laws are necessary but not sufficient for production integration.
The behavior-neutral program census decides which capability has enough real
body-solve cost behind it to justify the next implementation slice.
