# Staged Table Shapes

## Goal

The current heap-identity shape proof is binary: a table is either a final
`StableShape` or generic. That is sound but loses the dominant v1 legacy shape:
a locally born table is allocated empty, filled by a sequence of field writes,
and then consumed while later code may still append fields. This document
defines the staged tiers between generic and final stable shape that let the
arena JIT license fixed offsets for proven fields without requiring the entire
table shape to be closed.

The tiers are ordered by what a consumer may assume:

1. **Generic**: no fixed-offset license.
2. **Prefix-stable**: a required prefix of fields is present with proven types.
3. **Stable with optional fields**: required fields plus optional typed fields.
4. **Stable-after-P**: the complete shape is final for reads dominated by point
   `P`.
5. **StableShape**: the complete shape is final at the boundary being queried.

The tiers are proof tiers, not runtime representations. A read may receive the
strongest proof available at that program point and must not reuse it at another
point unless the proof's point/dominance condition still holds.

## Prefix-Stable

### Domain

For each heap table identity, prefix-stable evidence is:

```text
PrefixStable(fields: finite map<static suffix, value/type fact>)
```

The `fields` map is the set of static fields proven present on every path to the
query point and never structurally rewritten since the proof started. It is
rootless, using the same suffix key space as heap-identity static members, so
dot fields and equivalent static string indexes canonicalize to the same offset
candidate.

Only one-segment static field/string/int suffixes are licensable as fixed root
offsets in phase 1. Nested static members may still be transported as ordinary
heap facts, but they are not prefix offset slots until the nested owner also has
a staged-shape proof.

### Transfer

Prefix-stable starts for locally born table identities and for boundary objects
whose producer exports a prefix snapshot. A static field write to a known alias
of the same identity has these effects:

- New static field addition: extend the prefix with that field.
- Same field, same proven type/value class: keep the field.
- Same field, incompatible type/value class: kill prefix-stable.
- Write of nil/deletion: kill prefix-stable.

The transfer is alias-sensitive through heap identity, not source-name-sensitive.
Writes through local aliases of the exact same identity are known writers and
are allowed when they are monotone additions. Writes through an unknown writer,
an escaped value with unknown mutation authority, or a call whose summary may
mutate the identity kill prefix-stable.

Dynamic-key writes kill prefix-stable even when the key value later narrows to a
literal. This avoids licensing an offset from a proof whose structural write was
not statically a fixed field at the mutation point.

### Join

Join is prefix intersection:

```text
join(PrefixStable(A), PrefixStable(B)) =
  PrefixStable({ k -> join(A[k], B[k]) | k in keys(A) intersect keys(B)
                 and joined fact is not bottom })
```

A field present on only one branch is not required and therefore leaves the
prefix. If a common field joins to a value/type that no longer proves a single
offset-compatible field type, that field is removed from the prefix. If the
intersection is empty, the tier may still exist as an empty proof internally,
but it licenses no offsets and should usually be reported as generic.

### Widen And Termination

Widen is the same as join for the prefix lane. This is finite because keys are
bounded by the finite set of syntactic static write sites and canonical suffixes
interned during the body/summary solve. Repeated loop iterations can only add
fields that already correspond to those finite sites, and the must-map/intersect
semantics removes non-invariant fields rather than growing without bound.

Conservative loop policy for phase 1: if a structural write to an identity is
reachable from itself through a cycle, prefix licenses are allowed only for
fields that remain present after widening. The loop-created field itself is not
licensed unless the ordinary fixpoint proves it on every path after the loop.

### Kill Rules

Prefix-stable is killed by:

- Static delete or nil write to any field of the identity.
- Static retype/rewrite of an existing prefix field to an incompatible type.
- Dynamic-key write to the identity.
- Structural descendant invalidation that may include the identity root.
- Escape to an unknown writer or a call boundary that may structurally mutate
  the identity without a summary proving only monotone additions.
- Join with a generic/unknown object for the same identity.

Non-structural value updates below a known member table affect that child
identity's proof, not the parent prefix, unless the parent member itself is
replaced.

## Stable With Optional Fields

This tier is not a new heap lattice in phase 1. It is the existing record
optionality machinery interpreted as a stable shape:

```text
StableOptional(required fields, optional fields with proven types)
```

Required fields license non-nil fixed offsets. Optional fields license the same
offset only as a nilable read: the generated read must preserve the existing
optional/nilable type behavior and must not assume presence. Conditional static
adds therefore produce a record shape with optional members when the type domain
already proves the optional field type.

The soundness rule is the same as record optionality elsewhere in the checker:
missing optional members are equivalent to nil for reads, and writes must still
satisfy the optional member's declared/proven type. If the existing type domain
cannot express the optional field precisely, the table remains prefix-stable or
generic rather than introducing a parallel optional lattice.

## Stable-After-P

### Domain

`StableAfter(P, shape)` is a point-qualified proof:

```text
No structural write to identity is reachable from P to any queried read,
escape point, or return boundary.
```

The shape is the heap/static-member shape at `P`. Fixed-offset licenses apply
only to reads dominated by `P` and only when no structural write to the identity
is reachable from `P` to the read.

### Choosing P

For phase 1, `P` is not a stored lattice point. It is derived at query time from
existing dominance/reachability:

- A read has a stable-after-P license when all prior structural writes that may
  target the identity dominate the read, and no later structural write that may
  target the identity is reachable from the read.
- Equivalently, the read itself is at or after the last reachable structural
  write for that identity.

This reuses the current `StableShape` point predicates and weakens only the
origin requirement: locally born stack tables may be final after their last
write even if they were build-as-you-go before that point.

### Loops

A write in a loop is treated as reachable after every point in the loop unless
the CFG proves the queried point is after the loop and no structural write is
reachable from that point. Phase 1 does not prove "last iteration" facts.
Therefore, `P` may be after a loop, but never inside the loop because of an
iteration-independent claim.

## Soundness

Prefix-stable licenses only fields in a must set. Join uses intersection, so a
field omitted on any incoming path cannot be licensed. Destructive writes,
dynamic writes, and unknown writers kill the tier, so a licensed field cannot be
silently removed, retyped, or shadowed by an unmodelled mutation. Static writes
through known aliases are safe because heap identity makes them the same object,
and the monotone-addition rule rejects conflicting writes to existing prefix
fields.

Stable optional fields are sound because reads remain nilable and reuse the
existing optional member semantics. They do not convert a maybe-present field
into a required field.

Stable-after-P is sound because the license is point-qualified: every structural
write that could establish the shape is before and dominates the read, and no
structural write that could invalidate the shape is reachable afterward before
the relevant use/escape. A license cannot be moved earlier than `P`.

## Transport

| Surface | StableShape | Prefix-stable | Stable with optional fields | Stable-after-P |
| --- | --- | --- | --- | --- |
| Intra-body reads | Full fixed-offset license when the closed shape proof holds at the read. | Fixed-offset license for fields in the prefix at the read. Later additions do not invalidate earlier prefix fields. | Required fields behave like stable fields; optional fields read as nilable. | License applies only to reads dominated by `P` and before any reachable structural write. |
| Summary return heap objects | Existing `stableShape` bit marks a returned object final when every return source has the proof. | Export the boundary snapshot as an additive prefix field set on the allocation object. The caller receives only that snapshot; callee-internal later additions are not assumed. | Export through the object/return type when it is representable as a record with optional members. | Do not export `P`; summarize only the snapshot that is true at the boundary. If final at every return, it may become `StableShape`; otherwise export prefix/optional facts. |
| Manifest operational effects | Existing `stableShape` JSON field. | Additive `prefixStable`/field list beside `stableShape`. Old manifests omit it and old consumers ignore it. | Existing type encoding carries optional record members. No new manifest lane unless phase 2 needs explicit optional offset metadata. | Not serialized as a point fact. Manifest boundaries receive only stable/prefix/optional snapshots. |
| Passed-in table arguments | Callee may consume caller prefix. If the summary proves only additions through the argument, caller keeps the intersected old prefix plus any summary-proven additions at the call boundary. Unknown mutation kills. | Same. | Optional members stay optional unless the callee proves required presence on all exits. | A call is a structural mutation point unless the summary proves it cannot mutate the identity. |
| Captures | Full graph survives for never-structurally-written captures as today. | Captured table keeps prefix only while the capture graph proves no unknown writer and only monotone additions through known aliases. Unknown closure writes kill. | Captured optional record members use the existing nilability export. | Capture creation can receive a stable-after-P proof only if `P` dominates the capture and no later structural write is reachable before capture use. |
| Placement | Stack/owned/shared placement does not change the field proof. Placement controls escape authority: stack-local known writes may extend; owned/shared/unknown placement requires a boundary or freeze proof to keep staged tiers. | Same. | Same. | Same. |

## Module Table Pattern Verdict

For a module that returns table `M` and then the module itself extends `M` after
the return statement, the consumer must not see those post-return additions as
required fields. A return boundary snapshot exports the prefix/stable shape true
before the return transfer. Code after `return M` is normally unreachable; if a
module system later models post-return self-mutation hooks, those writes are
outside the returned snapshot and must degrade the exported tier to the
intersection known at the actual boundary. Consumer reads are sound only for the
snapshot exported in the manifest/summary.

## Phase 1 Scope

Phase 1 implements:

- Prefix-stable heap-object lane and checker/readmodel/export surfaces.
- Query-time stable-after-P licensing using existing reachability/dominance.
- Optional-field tier only where it naturally falls out of existing record
  optional members.

Phase 2 can add:

- Explicit optional offset metadata for codegen if the type-only surface is not
  ergonomic enough.
- Callee summary deltas that distinguish "only monotone additions" from generic
  mutation of passed-in arguments.
- More precise loop last-write proofs.
- Nested prefix slot licensing for child table identities.
