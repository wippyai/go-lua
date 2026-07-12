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
