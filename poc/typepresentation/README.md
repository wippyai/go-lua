# Semantic function type / presentation split POC

The production defect is a representation collision: `typ.Function` parameter
labels are ignored by equality/hash, but retained in `String()` and manifests.
The product interner therefore keeps the label from whichever concurrent unit
publishes an equal type first.

This POC makes the split explicit at immutable construction:

- semantic params carry type, optionality, and a receiver-convention bit;
- source labels live in an immutable side object used only by diagnostics,
  annotation display, and manifest encoding;
- recursive/generic child types are shared directly, so construction does not
  walk a large type graph and type-witness lookup stays O(1);
- semantic equality/hash is label-independent and receiver-sensitive.

Production migration seams are `typ.FunctionBuilder.Build`,
`typ.RebuildFunction`, `typ.CloneFunction`, structured manifest decode, Lua
annotation/function-type construction, and transform/substitution rebuilds.
Presentation consumers are `analysis/type/format`, diagnostics annotation and
assignment display, manifest encoding, contract/readmodel labels, and inferred
export construction. The current `Param.Name == "self"` checks must migrate to
an explicit receiver bit before names can leave the semantic node.

## PR2b record/union/recursive experiment

`semantic_graph.go` compares two bounded representations over record fields,
unions, nested functions, and paired recursive placeholders:

- eager `TypePair`: presentation and semantic graphs are constructed bottom-up;
- root-lazy `LazySemanticGraph`: the existing presentation graph is untouched,
  and one atomic root publishes a memoized semantic projection on first use.

For a 256-field manifest-shaped record (two labelled function alternatives plus
nil per field), eager pairing costs about 510us / 712KB / 12,562 allocations.
Presentation-only construction costs 297us / 362KB / 7,176 allocations. Lazy
semantic materialization adds 166us / 211KB / 3,608 allocations once, making
presentation + semantic availability about 465us / 573KB / 10,784 allocations.
Steady semantic selection is 2.38ns and allocation-free (eager selection is
effectively a field load). The root objects are 24 bytes lazy and 32 bytes eager.

Recommendation: use a root-lazy paired graph owned by the analysis database or
manifest/type cache, and prewarm it before values enter typewitness. This keeps
typewitness selection O(1), avoids an atomic pointer on every dormant composite,
preserves presentation roots, and uses one memo table to close recursive pairs.
Extending `semanticizeGraph` to remaining composites is mechanical. Do not put a
lazy pointer on every `typ.Record`/`typ.Union`: it retains dormant per-node bloat
without improving the one-time whole-graph projection cost.
