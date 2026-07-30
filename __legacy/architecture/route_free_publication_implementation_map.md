# Route-free publication prerequisites: implementation map

Status: blocked on the stabilized formal evaluator payload, 2026-07-18. This
document is the exact implementation/cut map requested by the route-runtime
retirement. No compatibility type or publication adapter was added.

## Verdict

The two publication authorities cannot be implemented soundly in the current
tree yet:

- `formalRelationRegionInventory` owns only the sealed cell universe,
  influence graph, roots/outcomes and `solve.WTOPlan`
  (`formal_relation_region_inventory.go:70-76`). It has no evaluated cell
  payload and therefore no stabilized semantic artifact to certify.
- The only evaluated publication input is still
  `transformer.StabilizedApplicationCoordinates`, consumed route by route in
  `program/relation_program_execution.go:191-380`.
- `stabilizedApplicationResultVersion`
  (`program/relation_program_execution.go:416-536`) hashes concrete route
  states and narrows each state/call outcome to a `uint64`. It is neither
  route-free nor collision-safe authority.
- `body.computeResultVersionLineageWithApplications`
  (`body/result_version.go:58-116`) already states that its full-width digest is
  incomplete because semantic payloads still enter through `uint64` hashes;
  it deliberately sets `complete := false` at line 86.

Minting `InputBoundaryClosed` from sealed syntax would prove only lexical term
closure, not closure of the stabilized fixed-point payload. Minting it from
the concrete route coordinates would preserve the implementation being
deleted. Either choice would be false authority. The correct producer belongs
in the single seal transaction immediately after formal WTO stabilization.

## The one value that must exist first

The formal evaluator should produce one private, immutable
`stabilizedFormalArtifact` per lexical body. This is not another IR: it is the
stabilized payload of the existing `relationCode` cells and existing registered
formal product.

It must own, without route or concrete caller identity:

1. the lexical `StableLexicalBodyID` and sealed `relationVar`;
2. the exact final payload of every body-owned formal outcome/root needed by
   publication, in sealed cell order;
3. the guarded observation/evidence payload and ordered effects already owned
   by `relationCode`'s `ValueTerm`, `PathTerm`, `Guard` and `EffectTerm` arenas;
4. the final registered residual factors, in registry order;
5. explicit external/import/native content identities; and
6. lexical callee references in sealed relation-variable order.

The object is minted only after the body's WTO component has completed ascent
and joint narrowing. It contains no `state.State`, `keyspace.Key`, invocation
route, call occurrence, scheduler edge, entry digest, or caller binding. A
failed/canceled solve publishes no partial object.

## Single seal transaction

Add one private transaction beside the future formal evaluator, not under
`body` or `program`:

```text
sealStabilizedFormalArtifact(ctx, program, evaluatedRegion, body)
    -> validate exact output/input dependency closure
    -> canonical encode the certified artifact
    -> construct InputBoundaryClosed owning that exact encoding
    -> construct RouteFreeArtifactFingerprint over the same encoding
    -> return LexicalPublication{artifact, closure, fingerprint}
```

There must be no separately callable `NewInputBoundaryClosed(bool)` or
fingerprint constructor from arbitrary bytes. The closure proof and
fingerprint are siblings minted from the same already-stabilized object in one
all-or-nothing transaction.

## `InputBoundaryClosed`

### Representation

Use an opaque typed proof whose fields are private to the formal publication
package. It owns:

- the exact lexical body ID;
- the formal schema identity/version;
- the complete, canonically ordered retained dependency inventory;
- the canonical artifact bytes (or an immutable owner of those same bytes);
- the full-width artifact digest used only as an index.

Validity is structural, not a boolean: owner and schema are nonzero, the
certified dependency inventory is canonical and nonempty where required, and
its artifact bytes/digest are the exact siblings returned by the seal
transaction. Copying a proof cannot detach it from the payload it certified.

### Exact closure walk

Do not add a second term visitor. Generalize the existing
`relationCodeTermRefs` census in
`transformer/relation_code_closure.go:622-689` so the formal seal uses the same
complete reference inventory already used by freeze-time closure. Its existing
`effect`/`effectTarget`/`pathStoreWrite` methods at lines 710-758 already cover
effect-embedded value/path terms. Add a read-only enumeration mode to this one
census and add registered residual-factor formal dependencies; do not restate
the value/path/effect switch elsewhere. The walker traverses the stabilized
reachable payload only, not all sealed syntax and never a body/CFG.

Every retained dependency must classify as exactly one of:

- `formal.Input`, owned by this body's sealed input schema;
- `formal.Output`, owned by this body and declared by the output schema;
- a body-owned `formal.Middle` root which is existentially bound by exactly
  one declaration in this same artifact's sealed middle-register schema and
  whose defining/reaching formal-region equations are included in the
  artifact closure;
- a canonical concrete constant or concrete identity;
- an explicit external/import/native content identity; or
- an allocation template paired with its exact body-owned formal allocation
  authority.

Reject the complete publication transaction on any:

- free `formal.Middle`: a foreign, undeclared, multiply declared, or retained
  middle root whose defining/reaching equations are absent from the same
  artifact closure;
- foreign-body formal root after final lexical composition;
- undeclared input/output ordinal;
- unresolved environment/local/cell/frame-result term;
- caller `keyspace.Key`, concrete entry `state.State`, invocation/route ID or
  call occurrence;
- allocation template without its formal owner/ordinal authority;
- registered residual factor that cannot enumerate its retained formal
  dependencies through the closed registered dependency contract.

The existing neutral root identity is sufficient:
`formal.Root` owns full body ID, `uint64` ordinal and IN/MID/OUT vocabulary
(`domain/formal/root.go:8-29`). `SlotSpace` validates ordinals against the exact
sealed `Shape` (`transformer/formal_slot_space.go:70-119`). No additional root
vocabulary is needed.

### Fail-closed consumer

`InputBoundaryClosed` therefore means **no free Middle**, not “the artifact
contains no Middle.” Internal MID coordinates are ordinary existential
variables of the canonical relation and remain in its canonical bytes. A
caller-visible output may refer to one only through the same sealed artifact's
complete binder/equation closure; no unresolved MID is exported as an input
obligation or caller binding.

The post-cut publication API must accept a single typed `LexicalPublication`,
not independent optional proof/fingerprint fields. `ExecutionFactory` may
project a `body.Result` only after this value validates against its prepared
body identity. Summary/manifest/observation builders consume the same value.

## Route-free full-width artifact fingerprint

### Authority and equality

Use `analysis/internal/canonical.Writer` with a new pinned domain and schema
version, for example `go-lua/formal-relation-artifact`, version 1. Retain the
finished canonical bytes and `sha256.Sum256(bytes)`. The digest is an index;
collision-safe equality compares canonical bytes (after equal digest/length as
cheap filters).

Do not reuse or widen `state.SemanticFingerprint`, `Result.resultVersion`, or
`BodyInputDigest`. Those are different contracts and currently include
concrete solve inputs or narrowed semantic hashes.

### Canonical stream

Encode, with explicit record kinds and collection counts:

1. artifact schema/version and registered axis/schema identity;
2. full lexical body ID;
3. canonical sealed formal root schema;
4. stabilized guarded outcome/root payloads in sealed formal cell order;
5. registered residual factors in registry order using their canonical codecs;
6. ordered effects and normalized observation/evidence sidecar;
7. sorted external/import/native content identities; and
8. lexical callee dependencies in sealed `relationVar` order.

No dense term ID is semantic by itself. Arena terms are encoded structurally
with memoized back-references assigned by deterministic first visit from the
ordered stabilized roots. No map iteration order, interner insertion order,
pointer, route or scheduler identity enters the stream.

### Recursive SCC normalization

Never recursively hash a callee artifact through a call stack. Compute
identities over the sealed lexical call-SCC DAG:

1. encode each member's local stabilized payload without callee digests;
2. sort SCC members by full lexical body ID;
3. encode intra-SCC call edges as canonical member ordinals;
4. encode outgoing edges as the already-sealed full canonical identities of
   target SCC members, in target `relationVar` order;
5. seal the SCC record bottom-up in condensation order; and
6. seal each body artifact as its local record plus the SCC record and its
   canonical member ordinal.

This gives one identity per lexical body while making self/mutual recursion
finite and independent of caller count or traversal order.

## Exact source cut

### Producer additions, in the same atomic evaluator cut

- `analysis/check/fixpoint/transformer/formal_relation_region_inventory.go`:
  retain the static inventory; the future evaluator adds the stabilized cell
  payload consumed by the seal transaction. Do not put publication logic in
  the inventory constructor at lines 84-144.
- `analysis/check/fixpoint/transformer/relation_code_closure.go:622-689`:
  make its reference census the sole complete formal-dependency enumeration,
  retaining its existing effect coverage and adding residual factors. Do not
  add a parallel publication walker.
- Add one private formal artifact canonical codec next to the formal evaluator.
  It consumes stabilized payloads only and returns the typed lexical
  publication bundle.
- `analysis/check/fixpoint/program/relation_program_execution.go`: after the
  sole formal solve, publish one lexical bundle per body. The driver must not
  construct either authority itself.

### Consumer migration

- Replace `body.ResultPublicationConfig.ApplicationDependencies`
  (`body/result_publication.go:55-61`) with one required typed lexical
  publication. `PublishResult` validates that its body owner matches the
  factory before projecting coordinates.
- Replace `computeResultVersionLineageWithApplications`
  (`body/result_version.go:58-116`) with lineage sourced from the route-free
  artifact identity. Keep the public legacy `uint64` result tag only as a
  derived presentation/index if downstream APIs still require it; it is not
  semantic authority.
- `body/call_argument_trust.go` consumes the certified formal input contract
  from the typed proof, without any result-route gate.
- `body/reportable_function_results.go`, `check/diagnostics/diagnostics.go`,
  `check/diagnostics/judgment_producers.go`, and
  `check/exportmanifest/function_signatures.go` currently infer observation,
  diagnostic, or manifest ownership directly from the single lexical
  publication's typed observation ownership. No context/definition enum
  remains.
- Summary, manifest and observation publication receive the same lexical
  bundle, not independently reconstructed digests.

### Delete once every caller consumes the bundle

- `program/relation_program_execution.go:312-333`: construction of
  `[]body.ApplicationDependency`.
- `program/relation_program_execution.go:383-414`:
  `relationRuntimeApplicationRoutes`.
- `program/relation_program_execution.go:416-536`:
  `stabilizedApplicationResultVersion` and its `encoding/binary`, `hash/fnv`
  imports.
- The per-route child-result field and accessors formerly in `body/api.go` and
  `body/result_accessors.go` are deleted; `ApplicationDependency` remains a
  separate lineage cut until its consumers migrate.
- `body/result_version.go:58-160`:
  `computeResultVersionLineageWithApplications`,
  `canonicalApplicationDependencies`, `sameApplicationDependencyEdge`, and
  `writeApplicationDependencies` after the sole caller migrates.
- Route-shaped result suppression/attachment in
  `program/relation_program_execution.go:538+`; replace it with one lexical
  result and body-owned observation ownership, not a synthetic context flag.

Whole-tree acceptance search after deletion:

```text
rg 'ApplicationDependency|stabilizedApplicationResultVersion|relationRuntimeApplicationRoutes' analysis
```

must return no production symbols or callers.

## Focused gates for the eventual implementation

1. Same stabilized lexical artifact reached from 1/10/100 callers has identical
   canonical bytes and fingerprint; only Apply binding work scales.
2. Two structurally unequal artifacts forced to share a digest/index remain
   unequal by canonical-byte comparison.
3. Map/interner/insertion permutations preserve canonical bytes.
4. A free/undeclared/foreign `MID`, foreign formal root, unresolved
   frame/cell/environment term, caller key, or unauthorized allocation makes
   the entire seal fail and publishes nothing; a same-artifact existential MID
   with its exact binder and equation closure is accepted.
5. Input/output roots at ordinals above `math.MaxUint32` remain exact.
6. Self and mutual recursion produce finite deterministic identities across
   independent solves and caller counts.
7. Cancellation during canonical encoding returns no bytes, proof,
   fingerprint, `body.Result`, summary or observation artifact.

Only after these focused gates and the atomic route-runtime deletion compile
should the full unskipped oracle run under `GOMEMLIMIT=3GiB`, followed by cold
Kickside. Testing a proof against the current concrete route artifact would
test the wrong engine and is explicitly excluded.
