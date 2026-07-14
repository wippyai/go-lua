# Portable Recursive Type Identity Plan

Status: reviewed implementation plan. This document defines the recursive-type
identity authority required by canonical analysis artifacts. It does not claim
that the current `typ.Recursive` representation or manifest wire already
implements that authority.

## 1. Problem statement

`typ.Recursive` currently receives a process-global numeric `ID` when it is
allocated. That number is useful as an in-memory cycle token, but it is not
source identity and must not enter a durable artifact key or canonical value.
Its value depends on unrelated allocation order, process history and concurrent
work.

The typewitness axis has a stricter equality contract than `typ.TypeEquals`:

```text
same reachable recursive-identity set
AND
structurally equal type graph
```

This distinction is intentional. Two independently allocated anonymous
mu-types can unfold to the same graph without being interchangeable witnesses.
Replacing the identity set with a structural digest would therefore weaken the
current lattice equality and could collapse proofs that the domain keeps
separate.

At the same time, a recursive declaration loaded twice from the same logical
source or module must not receive different durable identities merely because
it was decoded or resolved twice. Exact disk caching needs a portable
declaration authority, not a serialized process counter.

The required solution is a scoped dual identity:

- authoritative declarations carry a full-width stable identity;
- every recursive node also has a node-local identity for graph construction;
- anonymous or synthesized recursion has only local identity;
- disk encoding fails closed when exact equality depends on local identity.

This changes recursive identity authority, not recursive type semantics. The
17-axis product, typewitness order, `typ.TypeEquals`, aliases, generic
instantiation, subtype rules and diagnostics retain their existing meanings.

## 2. Current origin map

There are only a few production constructors, but they represent different
ownership classes and must not be conflated.

### 2.1 Source annotations and aliases

`analysis/lua/typeresolve/resolver.go` creates recursive placeholders in
`activeAliasRef` and `activeInterfaceRef`. The resolver is currently lazy: it
creates a placeholder only when resolution encounters a backedge, and keys its
construction state by run-local `bind.TypeDeclID`.

This handles direct self-recursion, but it does not provide portable identity.
It also makes a mutually recursive alias component dependent on query order.
For example, resolving `A` first in `A -> B -> A` can create only an `A`
placeholder, while resolving `B` first creates the corresponding `B`-rooted
representation. That is a construction defect, not a property of recursive
types.

A noncyclic alias to a recursive declaration must remain transparent and
inherit the target's recursive authority. It must not receive a second family
identity merely because the alias has a name.

### 2.2 Interfaces

Recursive interfaces use the same lazy placeholder mechanism. A nonrecursive
interface remains an ordinary `typ.Interface`; an interface receives a
`typ.Recursive` binder only when it belongs to a cyclic type-declaration
component.

### 2.3 Generics and instantiations

Recursive generic declarations use the forward body on `typ.Generic`, not a
`typ.Recursive` binder. `typ.Instantiate` reuses the generic plus its type
arguments and creates no recursive identity of its own.

A generic body or argument can still reach a nongeneric recursive alias. Those
reachable `typ.Recursive` identities must be preserved by instantiation. This
plan does not make generics nominal: their documented identity remains name,
parameters and body structure.

### 2.4 Imported and standard-library types

`analysis/module/manifest/type_codec.go` creates recursive placeholders while
decoding a `recursive` wire node. The wire `Binder` is local to one encoded type
tree and exists only to express backedges.

Each top-level `decodeType` currently owns a new binder registry. Export types,
named types, global types, function signatures and types embedded in effects
are decoded through separate roots. Consequently, a recursive family shared by
two manifest sections cannot be reconstructed as one identity family from the
current wire alone. Standard-library recursive types loaded from manifests have
the same limitation.

### 2.5 Synthetic recursion

`analysis/type/transform/rewrite_nodes.go:rewriteRecursive` creates a new
placeholder when a recursive body actually changes. An unchanged rewrite
returns the original node. Production has no other constructor of new
recursive graphs outside the manifest decoder and lexical resolver; direct
`NewRecursive` calls are currently confined to tests and POC code.

A changed rewrite is synthesized meaning. Unless the transform supplies an
explicit, versioned derivation authority, the result must remain local-only and
must not become disk portable by hashing its structure.

## 3. Scoped dual identity model

### 3.1 Identity components

Each `typ.Recursive` owns an immutable identity record conceptually equivalent
to:

```go
type recursiveIdentity struct {
	local  *localToken
	stable recursiveidentity.Stable // zero when no durable authority exists
}
```

`localToken` is unique to the constructed node or construction scope. It is
never serialized and never contributes to a durable key. The stable identity
is a domain-separated full-width value derived by the declaration owner.

Recursive nodes are not globally interned by stable identity. Two independent
loads may build two nodes with the same stable identity. Their bodies must
still be compared, and construction state must remain isolated.

### 3.2 Runtime equality

The typewitness relation remains:

```text
RecursiveIdentitySet(a) == RecursiveIdentitySet(b)
AND
typ.TypeEquals(a, b)
```

An identity-set member is the stable identity when present and the local token
otherwise. Therefore:

- separately rebuilt copies of the same authoritative declaration can agree;
- distinct declarations with identical names and bodies remain distinct;
- anonymous same-shape recursive nodes remain distinct;
- the same stable declaration with a changed body remains unequal because
  `typ.TypeEquals` still runs.

`typ.TypeEquals` must not use stable identity as an equality shortcut.
`typ.IsRecursiveRef` must remain local-node reference equality; a separately
loaded declaration with the same stable authority is not a self-edge in the
current graph.

### 3.3 Canonical disk encoding

The exact canonical projection of a concrete typewitness is:

```text
typewitness state tag
framed typ.EncodeCanonical structural bytes
sorted count of reachable stable recursive identities
each full-width stable recursive identity
```

This is deliberately two-part. `typ.EncodeCanonical` follows structural
`TypeEquals` semantics and must continue to ignore allocation identity. The
second part preserves the stricter typewitness identity set.

If any reachable recursive node has no stable identity, canonical typewitness
encoding returns a typed nonportable error. Callers must omit or reject the
containing durable artifact. They must not erase the witness, replace it with
Top, substitute structural identity, or silently skip the axis.

The runtime hash may use stable identities as a prefilter. Local-only identity
may deliberately share a structural hash bucket; unequal values are allowed to
collide, while equality still performs the exact identity-set comparison.

### 3.4 No process-global semantic ID

The public numeric `Recursive.ID` and its global counter cease to be semantic
authority. A scoped local ordinal may be used as nonportable construction
scratch where deterministic in-memory ordering requires it, but it is owned by
the resolver, decoder or transform session and never emitted.

Anonymous recursive values have no source-stable ordering by definition. Any
boundary requiring cross-process deterministic ordering must reject them unless
an authoritative owner supplies a stable derivation.

## 4. Source declaration construction

### 4.1 Stable declaration anchors

`bind.Result` must expose a declaration anchor independent of value symbols:

```go
type TypeDeclAnchor struct {
	Owner   TypeBodyOwner
	Ordinal uint64
	Kind    TypeDeclKind
}
```

The owner identifies the lexical body that declares the type. The ordinal is
the canonical declaration order within that owner, not an allocation counter.
Shadowed same-name declarations therefore remain distinct. Exact source loaded
twice under the same logical unit produces the same anchor.

The stable recursive identity is domain separated, for example:

```text
H("wippy.recursive.lexical.v1",
  StableLexicalBodyID(owner),
  declaration ordinal,
  declaration kind)
```

The source revision does not belong in nominal declaration identity. Source and
dependency revisions are already fenced by semantic artifact inputs, and body
structural equality prevents a changed declaration from comparing equal to an
old one. The logical unit namespace does belong in the lexical body ID: two
copies under distinct logical ownership are distinct declarations even if
their bytes happen to match.

### 4.2 Binder-owned dependency SCCs

The binder already knows the resolved target of every lexical type reference.
It must build the type-declaration dependency graph after binding and compute a
deterministic Tarjan condensation. An SCC is recursive when it has more than
one member or contains a self-edge.

The binder exposes the cyclic component and canonical member order to the
resolver. The resolver must not rediscover lexical dependencies structurally.
Generic parameters are scoped binders, not declaration graph members.

### 4.3 Predeclaration and completion

Before resolving any member body in a recursive SCC, the resolver predeclares
all members in anchor order:

- nongeneric aliases receive authoritative `typ.Recursive` placeholders;
- recursive interfaces receive authoritative `typ.Recursive` placeholders;
- generic aliases receive forward `typ.Generic` nodes under the existing
  generic identity contract.

It then resolves and seals every body against that complete environment. Each
placeholder is completed once. Failure discards the component rather than
publishing a partially completed graph.

This makes `A -> B -> A` independent of which `Decl` call arrives first and
gives each declared nongeneric recursive member its own stable family. An alias
outside the SCC that points to one of those members simply returns that member.

### 4.4 Resolver authority injection

Stable identity must be present at placeholder construction. It must not be
attached after a type has entered WIR, a hash cache or a product interner.

`typeresolve` therefore needs an options-based constructor or an identity
authority interface supplied by the prepared analysis unit. Production code
must reuse the prepared resolver rather than instantiate unconfigured fresh
resolvers in export or program projections.

A zero-authority standalone resolver remains valid for transient analysis, but
any recursive type it creates is local-only and prevents durable typewitness
encoding.

## 5. Manifest family and session schema

### 5.1 Binder is not family identity

The existing `typeWire.Binder` remains an intra-tree backedge handle. It cannot
identify a declaration across top-level manifest roots because its numbering
restarts for each encoded type.

The manifest type schema gains a recursive family field. One manifest-wide
encoding session maps each authoritative recursive identity to a deterministic
family ordinal while traversing manifest roots in canonical order. Repeated
appearances of the same family in named types, signatures or effects carry the
same family value even when each appearance contains its own local binder.

The decoder owns the corresponding manifest-wide family registry. A decoded
node receives a stable identity derived from:

```text
H("wippy.recursive.manifest.v1",
  manifest path,
  manifest version/schema,
  family ordinal)
```

It need not globally intern the resulting `typ.Recursive` nodes. Stable family
equality plus structural equality is sufficient.

### 5.2 Encoding authority

The manifest encoder assigns a family only to a recursive node that already
has authoritative stable identity. An anonymous or transformed recursive node
causes manifest encoding to fail closed. A module boundary must not convert an
unowned synthetic graph into a nominal declaration merely because it appears
in an export.

Semantic signature-content encoding remains structural. That codec is defined
against `typ.TypeEquals`, not typewitness identity, and must not acquire the
stricter family set accidentally.

### 5.3 Legacy manifests

Legacy manifests contain binder-local recursion but no cross-root family
authority. They may continue to decode into local-only nodes for runtime
compatibility. They cannot produce an exact portable recursive typewitness and
must be excluded from such disk artifacts.

A schema bump and manifest rebuild is the only honest way to recover cross-root
identity. Deriving a family from name and structure would collapse distinct
same-name declarations and is forbidden.

## 6. Concrete file and API migration

The exact names may be refined during implementation, but ownership must remain
as follows.

### 6.1 Identity vocabulary

Add `analysis/type/recursiveidentity/identity.go` with:

- opaque `Stable [32]byte`;
- zero/validity and lexicographic comparison helpers;
- domain-separated lexical and manifest derivation;
- no dependency on `typ.Type` and no process-global registry.

### 6.2 Type core

Change `analysis/type/typ/recursive.go` to:

- make the semantic identity fields private;
- retain anonymous constructors for tests and explicitly transient values;
- add authoritative recursive and placeholder constructors;
- expose a read-only stable-identity query;
- remove the process-global numeric ID from equality and durable behavior.

Update:

- `analysis/type/typ/dedup.go` to collect exact stable-or-local identity sets
  and expose a stable-only collection for codecs;
- `analysis/type/typ/equals_recursive.go` to remove numeric-ID shortcuts;
- `analysis/type/typ/equals.go` and union dedup to use the exact new set;
- `analysis/type/typ/canonical_codec.go` to remove numeric-ID validation while
  retaining structural graph encoding;
- recursive debug formatting so it cannot be mistaken for canonical identity.

### 6.3 Binding and resolution

Update `analysis/lua/bind/types.go` and `result.go`, and add
`analysis/lua/bind/type_scc.go`, for declaration anchors, declaration
enumeration, bound dependency edges and deterministic SCCs.

Update `analysis/lua/typeresolve/resolver.go` and add
`recursive_components.go` for authority injection, component predeclaration,
transactional completion and query-order independence.

Update `analysis/check/body/run.go` to provide the unit identity before
recursive type construction. Audit every production `typeresolve.New` call in
`analysis/check/exportmanifest` and `analysis/check/fixpoint/program`; reuse the
prepared resolver or pass the same authority explicitly.

### 6.4 Manifest codec

Update:

- `analysis/module/manifest/type_wire.go` with the family field and schema;
- `type_codec.go` with manifest-wide encoder/decoder sessions and separate
  binder/family registries;
- `manifest.go` so all canonical roots share one identity session;
- structured type, effect and operational-effect codec helpers so nested type
  roots receive their canonical manifest path/session.

Legacy decoding remains explicit and marks recursive families nonportable.

### 6.5 Typewitness canonical codec

Add `analysis/domain/value/axis/typewitness/canonical.go` using the shared
canonical writer. Encode state, structural type bytes and the complete sorted
stable identity set. Return the repository's typed nonportable result when the
stable set is unavailable.

Update `typewitness.go` to intern or bucket exact stable-or-local signatures
without relying on numeric global IDs. Any retained interner must have explicit
release/ownership or a measured bounded policy; this migration must not create
a new immortal process-global cache.

## 7. Correctness and regression gates

### 7.1 Type-core gates

- independently built authoritative nodes with the same stable ID and body
  have equal identity sets and canonical bytes;
- the same stable ID with a different body is unequal;
- different stable IDs with the same name and body are unequal;
- anonymous same-shape nodes remain unequal under recursive-identity equality;
- mixed stable/local graphs and graphs with more than eight recursive members
  take the exact slow path without changing results;
- anonymous canonical encoding fails closed;
- canonical bytes contain no address, local ordinal, numeric recursive ID,
  cached hash, revision or `String` output.

### 7.2 Source resolver gates

- parse the same source twice under the same namespace, resolve declarations in
  opposite order, and obtain equal corresponding recursive witnesses;
- the same source under different logical namespaces remains distinct;
- shadowed same-name declarations and structurally identical separate
  declarations remain distinct;
- two- and three-member alias/interface SCCs are query-order independent;
- direct self-recursion, alias-to-recursive, nonrecursive aliases and recursive
  interfaces retain their current type behavior;
- generic self-recursion stays structural, a generic containing a recursive
  alias preserves that alias authority, and instantiation creates no family;
- failed component resolution publishes no partial placeholder.

### 7.3 Manifest gates

- decoding the same manifest twice produces matching recursive family sets;
- one family repeated across `Types`, a function signature and an effect keeps
  one identity;
- two same-shape families in one manifest remain distinct;
- binder ordinals reused in separate roots do not collide;
- encode/decode/encode is deterministic;
- legacy recursive manifests remain usable transiently but are rejected by the
  portable typewitness codec;
- malformed, duplicate or out-of-scope family/binder references fail closed.

### 7.4 Transform and system gates

- an unchanged recursive rewrite returns the authoritative original;
- a changed rewrite is local-only and cannot enter a disk artifact;
- race tests cover concurrent independent resolve/decode and canonical reads;
- canonical literal goldens pin every tag, frame and schema version;
- full fixtures, lattice laws, oracle, soundness, manifest round trips and
  architecture gates remain green;
- cold/warm cross-process artifact tests prove that a warm hit preserves exact
  diagnostics and typewitness values.

## 8. Performance and modularity invariants

This identity layer must not become another solver or a global memoization
system.

- Stable identity derivation is O(1) per declaration or manifest family.
- Type SCC discovery is one binder-owned O(V+E) pass, not repeated by each
  resolver query.
- Component placeholders are allocated once and completed once.
- The common typewitness case with zero or one recursive family remains inline
  and allocation-free after construction-time caching.
- Stable identity comparisons use full values for authority. Short hashes are
  prefilters only.
- Canonical encoding may allocate bounded scratch but retains no type graph or
  caller value after return.
- No global map interns recursive nodes. Any signature interner is run/session
  scoped, releasable and measured.
- The stable identity vocabulary is independent of axes. Adding or removing an
  axis does not alter recursive identity.
- The typewitness codec is the only layer that composes structural type bytes
  with recursive identity-set bytes. Other type consumers keep their declared
  structural or nominal contract.
- Manifest binder topology, declaration authority and artifact revision are
  three separate concepts and remain separately versioned.
- A missing authority reduces cache eligibility, never semantic precision.

## 9. Honest blockers and decisions

### 9.1 Standalone namespace timing

`body.prepare` currently derives its fallback standalone `UnitNamespace` after
WIR lowering, while WIR lowering already invokes type resolution. Stable
identity cannot be attached at construction in that ordering.

Disk-capable production solves must provide a namespace before resolution.
Until source/standalone namespace derivation moves earlier, a zero-namespace
standalone solve that constructs recursion is transient and nonportable.
Late mutation of recursive identity is forbidden.

### 9.2 Manifest schema migration

The current manifest wire has no family authority and cannot reconstruct exact
sharing across roots. Old recursive manifests need rebuild/schema upgrade or
cache exclusion. There is no sound inference that recovers missing identity
from names and structure.

### 9.3 Existing SCC representation changes

Predeclaring every recursive declaration corrects a query-order-dependent
representation. It can change hashes, formatting, and downstream diagnostics
that accidentally depended on which declaration was requested first. The
migration requires resolver differentials and the full oracle; byte identity
must not be asserted before measurement.

### 9.4 Synthetic cache coverage

Changed recursive rewrites intentionally become nonportable. If measurement
shows that authoritative artifacts routinely contain such results, add a
separate transform-domain identity API whose key includes the parent authority,
transform schema and canonical coordinate. Do not infer identity from the
rewritten body and do not add a generic escape hatch.

### 9.5 Public API compatibility

`Recursive.ID` is exported even though repository production code does not use
it outside the type implementation. Removing it is still a public Go API
change. Land a deprecation/compatibility decision explicitly; never retain it
as hidden durable authority merely to avoid the API decision.

## 10. Staged landing order

Each stage is independently reviewable and must keep the previous runtime path
green until its acceptance tests pass.

1. **Identity vocabulary and red tests.** Add the stable type, domain tags and
   adversarial equality/canonical tests. No producer uses it yet.
2. **Type-core dual identity.** Privatize semantic identity, migrate exact
   identity-set comparison, remove numeric-ID equality shortcuts, and preserve
   anonymous runtime behavior. Canonical disk encoding remains unavailable.
3. **Binder anchors and SCC oracle.** Add declaration ownership, dependency
   graph and deterministic SCC census without changing resolver construction.
4. **Resolver predeclaration.** Build source recursive SCCs transactionally with
   stable lexical authority. Remove lazy query-order construction after
   differential and fixture closure.
5. **Manifest family schema.** Add manifest-wide sessions and family wire,
   rebuild authoritative manifests, and quarantine legacy recursive values from
   portable artifacts.
6. **Typewitness canonical codec.** Encode the structural graph plus stable
   identity set, wire typed nonportability through product/artifact assembly,
   and prove cross-process exactness.
7. **Production caller cleanup.** Eliminate unconfigured fresh resolvers,
   require early namespaces on disk-capable paths, scope/release signature
   interners, and remove obsolete `Recursive.ID` compatibility code.
8. **Full artifact gate.** Run race, fixtures, laws, oracle, soundness,
   manifest, cold/warm Kickside and byte-determinism gates. Only then may
   recursive typewitnesses participate in durable analysis artifacts.

The terminal state has one recursive declaration authority, one binder-owned
SCC construction path, one manifest family schema and one exact typewitness
canonical composition. It has no raw process-global recursive identity and no
structural fallback that weakens the lattice.
