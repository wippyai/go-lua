# Stage 2 semantic operation-plan slice

This isolated POC compiles the existing syntax-free `factflow.PathAssignment`
DTO into an immutable typed operation plan. Production behavior is unchanged.

Path assignment was selected as a representative cross-lane seam, not as a
measured hotspot. It exercises value/path mutation, subtree and heap
invalidation, alias/equality closure, dynamic/key/length cleanup, user-lattice
propagation, point visibility, keyspace ownership, and root rebasing. Existing
measurements indicate this individual family is too small to be a standalone
performance lever; its purpose is to validate the architecture.

## Plan contract

`CompilePathAssignments` copies structural `path.Path` and `ValueSource`
identities from `factflow.FactsInput`. It never retains resolver-versioned path
strings or keyspace intern IDs. Each operation declares:

- seven conservative lane dependencies: Values, PathEvidence,
  HeapTableIdentity, DynamicIndex, KeyMemberships, LenFloors, UserLattices;
- lexical-path, resolver-keyspace, and heap-identity ownership;
- source-root and target-root rebasing;
- same-point `PathStaticMemberWrite` and `CovariantExposure` companions.

Every other same-point node operation fails compilation. The concrete plan
reconstructs a PathAssignment-only immutable `Facts` snapshot plus its supported
companions and source-path metadata, so unrelated caller facts cannot leak into
the delegated operation.

## Concrete differential result

The concrete interpreter delegates to the existing exported
`factapply.NewFactsNodeTransfer`; it does not duplicate the unexported
`applyPathAssignment` semantics. Tests compare the ordinary transfer and plan
delegate across:

- all 17 single-lane State domains;
- 512 deterministic randomized target subtrees, siblings, aliases/equality
  closure shapes, and same-point lazy source reads;
- missing visibility and missing-source no-ops;
- immutable input-map mutation;
- companion metadata and unsupported-companion fallback.

This proves the plan plumbing is behavior-neutral for the isolated operation.
It does not yet prove that operation dispatch can replace production
`factapply`, because both paths deliberately share the concrete oracle.

## Symbolic result and executable slice

The symbolic registry is derived from `state.DefaultLaneCatalog`. Every current
lane has an explicit role: seven term-producing adapters and ten explicitly
unaffected adapters. Missing/orphan/duplicate roles fail closed; adding a State
lane therefore requires one role adapter and cannot silently disappear.

The seven supported adapters build one guarded, correlated effect row in exact
production order:

1. invalidate root origins;
2. invalidate heap descendants/subtree;
3. invalidate path/dynamic/key/length facts;
4. write the assigned value;
5. relate source and target paths;
6. propagate user-lattice facts.

Structural root substitution is cheap and preserves the source-value guard.
Same-point static-member/covariant companions atomically fall back because
their symbolic handlers are not implemented.

`SymbolicTransformer.Execute` is now an independent concrete interpreter for a
strict slice. It uses exported State semantics for subtree invalidation,
equivalent-alias writes, field/index canonicalization, equality proof creation,
typestate canonicalization, and user-lattice propagation. A deterministic
randomized differential compares it to `NewFactsNodeTransfer` in every one of
the 17 single-lane domains.

The slice is intentionally narrow. Root-origin invalidation, heap member
invalidation, and source static-member copying are implemented by unexported
production helpers. Rather than imitate those helpers, the POC rejects any
State with finite/top root values, finite/top heap objects, or source/target
static-member evidence. Rejection returns the input State unchanged. Companion
operations also still reject atomically. This proves that an operation plan can
execute exactly through existing public State boundaries; it does **not** yet
prove full PathAssignment parity or Summary construction.

The next semantic gate is to factor the three rejected operations into shared
engine primitives, then rerun the same differential over populated origins,
heap identities, and static descendants. Production must remain on the current
applicator until that gate is green.

## Cost

AMD Ryzen 9 7950X3D, Go 1.23.3, three benchmark repetitions:

| Operation | Time | Allocations |
| --- | ---: | ---: |
| Plan build | 0.85–0.89 us | 1,964 B / 16 |
| Existing concrete oracle | 6.30–6.38 us | 2,576 B / 24 |
| Concrete plan delegate | 6.42–6.44 us | 2,576 B / 24 |
| Symbolic term lift | 2.16–2.21 us | 7,384 B / 32 |
| Structural term substitution | 1.32–1.33 us | 2,296 B / 30 |

Concrete dispatch adds roughly 1–2% and no allocations. The optimistic term
substitution cost is about 21% of one current concrete application, inside the
48% overhead allowance from the combined context/phase cost model. However,
30 allocations per instantiation are unacceptable, and the terms are not yet
executable. The timing says the representation budget is plausible; it does not
clear the semantic or allocation gate.

Run:

```sh
go test -race ./poc/semanticplan
go test -run '^$' -bench BenchmarkPathAssignment -benchmem ./poc/semanticplan
```
