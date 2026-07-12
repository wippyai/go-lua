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
reconstructs a PathAssignment-only immutable `Facts` snapshot plus source-path
metadata, so unrelated caller facts cannot leak into the operation. Declared
companions force atomic contextual fallback until they have complete handlers.

## Concrete differential result

The concrete interpreter invokes `factapply.ApplyConcretePathAssignment`, the
same transactional kernel used by `NewFactsNodeTransfer`. Tests compare the
ordinary whole-point transfer and operation kernel across:

- all 17 single-lane State domains;
- 256 deterministic randomized whole States spanning the complete 17-lane
  catalog, including populated root values/origins, heap objects, and static
  descendants;
- 512 deterministic randomized target subtrees, siblings, aliases/equality
  closure shapes, and same-point lazy source reads;
- missing visibility and missing-source no-ops;
- immutable input-map mutation;
- companion metadata and unsupported-companion fallback.

This proves the plan plumbing is behavior-neutral for the isolated operation
without cloning production semantics. Both paths deliberately share the
concrete kernel; this is a factoring proof, not an independent semantics proof.

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

`SymbolicTransformer.Execute` now uses that same complete kernel. The previous
handwritten strict-slice interpreter and its restrictions are gone: populated
origins, heap identities, static descendants, aliases, and all lane effects are
covered by the whole-State differential. Companion operations still reject
atomically because executing only half a point would be unsound.

`BranchPathRelationOp` is the first executable semantic guard. It owns cloned
structural paths and invokes `ApplyConcreteBranchPathRelation` against the
evolving edge output. Differentials cover equality, inequality, type-match,
and type-unmatch, including a preceding edge update that must survive and be
visible to the guard. Production remains unchanged; summary construction and
symbolic-state execution are still later gates.

## Cost

AMD Ryzen 9 7950X3D, Go 1.23.3, three benchmark repetitions:

| Operation | Time | Allocations |
| --- | ---: | ---: |
| Plan build | 0.85–0.89 us | 1,964 B / 16 |
| Existing whole-point oracle | 6.51 us | 2,576 B / 24 |
| Concrete plan kernel | 6.16–6.19 us | 2,544 B / 23 |
| Symbolic operation kernel | 6.34–6.38 us | 2,544 B / 23 |
| Symbolic term lift | 2.18–2.20 us | 7,208 B / 31 |
| Structural term substitution | 1.32–1.33 us | 2,296 B / 30 |

The operation wrapper adds no allocation over the shared kernel and is about
2% slower than direct plan dispatch, while both avoid one allocation made by
the whole-point oracle. The optimistic term
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
