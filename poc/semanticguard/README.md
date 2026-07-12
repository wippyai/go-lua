# Guarded call-boundary composition POC

This isolated Stage 2 proof captures a complete `BranchPathRelation` set as one
correlated row and instantiates that row at a caller boundary.

What is proven:

- all four current relation kinds retain their true/false-edge guards;
- `$N` parameter roots and `ret[N]` result roots are rebased, including suffixes;
- concrete lexical paths remain concrete;
- a missing binding rejects the entire selected row, never a partial prefix;
- inactive relations do not demand bindings;
- compiled and returned paths do not alias caller-owned mutable slices;
- 5,000 deterministic randomized cases agree with the public production
  `BranchPathRelation` and `callboundary.PathBindings` behavior.

What is deliberately **not** proven:

`factapply.applyBranchPathRelation` is point-local and private because exact
execution requires visibility versions, keyspace identity, type caches, channel
selection and several State lanes. This POC therefore reports `Executable() ==
false`; its output must not be published as analyzer State. The next production
gate is a shared operation interpreter that can execute both the concrete and
symbolic terms against those dependencies and differentially match the current
applicator. Until then this operation remains on the contextual fallback.

