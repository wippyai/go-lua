# Scalar Key Design

Status: measured prototype lane in progress.

## Scope

The current state engine has already moved the hottest value lanes away from
raw `path.PathKey` string map keys and onto `keyspace.Key`, a comparable
struct with solve-local segment/root intern IDs. This design is the next step:
intern the whole canonical key into a dense scalar ID and use that ID inside
hot state maps. The scalar ID is a runtime optimization only. Summaries,
manifests, fixtures, diagnostics, and any cached cross-body artifact continue to
store canonical paths or typed state keys.

## Key-Domain Inventory

Hot solve state keys fall into two categories.

### State Lanes Using `keyspace.Key`

- `analysis/engine/state/pathevidence`
  - `Lane.refinements map[keyspace.Key]product.Value`
  - `Lane.staticMembers map[keyspace.Key]product.Value`
  - `map[BranchProof]struct{}` where `BranchProof.Path` and `Other` are
    `keyspace.Key`
  - `map[PathPresenceImplication]struct{}` where trigger/target are
    `keyspace.Key`
  - Dominant operations: point reads/writes, batched edit clones, lattice
    equality/order/join/widen through `lift.MustMap` and `lift.MustSet`, alias
    expansion, deterministic snapshot ordering, and subtree/descendant
    invalidation.
  - Invalidation crux: `InvalidatePathKeySubtreePrefixes` and
    `InvalidatePathKeyDescendantPrefixes` convert prefixes to keys, scan every
    map entry, and call `ks.HasPrefix`/`ks.HasStrictPrefix`. Branch-proof and
    presence-implication sets are scanned similarly.

- Numeric bounds
  - `numBoundLane` for `numFloors` and `numCeils` stores
    `lift.MustMapLane[keyspace.Key,int64]`.
  - Dominant operations: `ReadNumFloor/Ceil`, `WriteNumFloor/Ceil`,
    `ClearNumFloor/Ceil`, and must-map equality/order/join/widen. There is no
    subtree scan on this lane today; roots writes clear exact target keys.
  - This is the prototype lane because it is self-contained and already covered
    by state benchmarks.

- Length floors
  - `lenFloorLane` stores `lift.MustMapLane[keyspace.Key,lenbound.Floor]`.
  - Dominant operations mirror numeric bounds, but invalidation scans all floor
    keys and calls prefix predicates for subtree and descendant mutations.

- Dynamic-index facts
  - `dynamicindex.Key{Table keyspace.Key, Site dynamicindex.Site}` is the map
    key for the dynamic-index fact lane.
  - Dominant operations: point lookup/write by composite key, map-domain
    equality/order/join/widen, and subtree invalidation by scanning facts and
    prefix-testing the table key.

- Key memberships and user lattices
  - `KeyMembership.Container` is a `keyspace.Key`; several membership paths are
    still typed `pathaddr.StateKey` and are parsed back through `KeySpace` during
    invalidation.
  - `userLatticeKey` embeds a `keyspace.Key`.
  - Dominant operations: lookup/write, finite-set or finite-map lattice
    operations, snapshot/rekey, and subtree scans for memberships.

- Heap/static-member state
  - `heapidentity.TableObject.StaticMembers` and related boundary/materialize
    paths use `map[keyspace.Key]product.Value`.
  - Dominant operations: read/materialize static members, merge/join object
    state, snapshot/rekey, and deterministic export ordering.

### Boundary and Summary Path Keys

Interprocedural artifacts still use stable path representations:

- summary normal-return lanes normalize on `path.Path.Key()` strings, for
  example `numFloorFactKey path.PathKey`;
- manifests and signatures encode `path.Path`, stable state keys, or canonical
  strings;
- diagnostics, fixture oracles, and export surfaces format paths for stable
  output.

These are intentionally out of scope for scalar IDs. They cross solve/body
boundaries and must remain deterministic, canonical, and serializable.

## Operation Profile

The repeated hot operations are:

- lookup: path/key reads in `ReadPathKey`, `ReadNumFloor`, source-value reads,
  dynamic-index reads, and user-lattice reads;
- insert/update/delete: transfer facts write into persistent maps, often cloning
  a finite map first;
- equality/order: product-lattice joins use lane equality and `LessOrEq`
  heavily before deciding whether to return an input state;
- join/widen: must-map joins intersect key sets; map lanes join pointwise over
  key union;
- iterate/snapshot: summaries, diagnostics, and fixture output sort by formatted
  path for deterministic output;
- subtree invalidation: mutation invalidation scans key maps and key-bearing
  sets using prefix predicates.

The subtree invalidation requirement is the hard part. String prefixes used to
make this cheap to express but expensive to execute. `keyspace.Key` removed raw
string parsing from most scans, but still requires per-entry structural prefix
checks. Scalar IDs only pay off if the interner also owns parent/child edges so
subtree membership can be resolved as an ID walk.

## Design

Add a whole-key scalar interner to `keyspace.KeySpace`.

```
type ID uint32

type keyEntry struct {
    key      keyspace.Key
    parent   ID
    children []ID
}

type KeySpace struct {
    ...
    keyByValue map[keyspace.Key]ID
    keyEntries []keyEntry // index 0 is invalid
}
```

`KeySpace.InternID(keyspace.Key) (ID, bool)` interns a valid key and returns a
dense non-zero ID. `KeySpace.Key(ID) (keyspace.Key, bool)` recovers the key for
formatting, snapshots, rekeying, and boundary projection. Interning also interns
the structural parent, if one exists, and appends the new ID to the parent's
children list.

Parent identity is defined over the existing structural key:

- a root key has parent 0;
- a member key's parent is the same root with the last segment removed;
- root flavor, symbol/version, stable/root spelling, and `Canon` are preserved;
- field/string-index equivalence remains a `KeySpace` responsibility, not an ID
equality shortcut.

The core fast paths then become:

- equality: `id == id`;
- map keys: `map[keyspace.ID]V` or `[]V`/bitsets where a dense lane can justify
  sparse-set bookkeeping;
- deterministic iteration: collect IDs, sort with `ks.Less(ks.Key(id), ...)` or
  by formatted path. Sorting by raw ID is allowed only for internal debug output
  where insertion order is the desired order; user-visible and serialized output
  must preserve canonical path order;
- subtree invalidation: expand `Descendants(prefixID)` by walking child lists,
  then delete by ID set. For exact path-evidence semantics, callers still layer
  alias expansion on top of the structural subtree seed set.

The child-list design changes invalidation from:

```
for candidate := range lane {
    if ks.HasPrefix(candidate, prefix) { delete(candidate) }
}
```

to:

```
kill := ks.SubtreeIDs(prefixID)
for id := range kill { delete(lane, id) }
```

This is only asymptotically better when the lane's map is large and the mutated
subtree is smaller than the whole lane. It is still a win for exact equality and
hashing even when invalidation must scan composite lanes that have not migrated.

## Lifetime and Boundaries

Scalar IDs are solve-local. They are no more stable than the existing
`SegmentsID` and root intern IDs inside `keyspace.Key`; two keyspaces may assign
different IDs to the same path.

Rules:

- never serialize scalar IDs;
- never put scalar IDs into summaries, manifests, diagnostics, fixture oracles,
  exported signatures, or persistent caches;
- rekey every scalar lane at solve/body boundaries using canonical path
  formatting and the destination `KeySpace`;
- memoization keys must not include scalar IDs unless the memo is explicitly
  scoped to a single `KeySpace` instance;
- cached summaries and manifests store paths, not IDs, and instantiate IDs only
  when applied to a caller solve.

For body-local fixpoint state, ID stability only needs to hold within one
`KeySpace` instance. Adding new keys during solving must never renumber existing
IDs. Dense IDs are append-only.

## Memory Cost

Each interned key adds:

- one `keyEntry` containing the old comparable `keyspace.Key`, parent ID, and a
  children slice header;
- one `map[keyspace.Key]ID` entry;
- one child ID in the parent slice for non-root keys.

This intentionally spends memory once per distinct key to remove repeated
per-lane key hashing and repeated prefix checks. Child lists are append-only and
freed with the solve keyspace. If a corpus shows many one-off keys and few
subtree invalidations, the design can store children lazily behind a build tag
or only for lanes that enable subtree deletion. The default should include child
lists because invalidation is the correctness-sensitive operation that otherwise
forces map scans.

## Determinism

Scalar ID allocation depends on discovery order, so raw ID order is not a stable
output contract. Any output-visible iteration must sort by canonical key order:

- `ks.Less(keyA, keyB)` for in-memory key ordering;
- `ks.Format(key)` when crossing to path strings.

State equality, lattice order, and joins are unaffected by iteration order
because they are map/set operations over comparable IDs.

## Migration Plan

1. Add `keyspace.ID` and whole-key interning with parent/children relations.
2. Convert one self-contained lane to scalar IDs behind the existing public
   `State` methods.
3. Verify byte-identical fixture/oracle output.
4. Measure state benchmarks and one stress fixture.
5. Continue only if the measured end-to-end fixture improvement is at least 5%
   or if a narrower benchmark shows a decisive win on a proven bottleneck that
   the fixture does not exercise.

Prototype target: `numBoundLane` for `numFloors` and `numCeils`.

Reasons:

- self-contained lane with no public API change;
- map key is exactly `keyspace.Key`, so scalar conversion is direct;
- existing state benchmarks exercise `WriteNumFloor` and domain join paths;
- output surfaces already snapshot through `KeySpace`, so byte identity is easy
  to preserve.

Limitation: this prototype does not prove the child-list invalidation algorithm,
because numeric bounds do not currently use subtree invalidation. It proves the
lookup/hash/map part of the representation and keeps the risk low before a
path-evidence or length-floor migration.

## Baseline Measurements

Command:

```
go test ./analysis/engine/state -run '^$' -bench 'BenchmarkState|BenchmarkInvalidateSubtreeStructuralKey' -benchmem -count=5
go test . -run '^$' -bench 'BenchmarkFixture' -benchmem -count=3
```

Initial baseline for the state package did not include the pathevidence subpackage
benchmark because it lives in `analysis/engine/state/pathevidence`; that benchmark
must be measured separately before a path-evidence migration.

Representative baseline medians:

| Benchmark | ns/op | B/op | allocs/op |
| --- | ---: | ---: | ---: |
| StateDomainJoinMostlyEqual | 1,947 | 0 | 0 |
| StateDomainJoinIdentical | 717.6 | 0 | 0 |
| StateSeedValues | 4,005 | 1,128 | 7 |
| StateValueEditMultipleWrites | 3,423 | 1,128 | 7 |
| StateSequentialMultipleWrites | 11,457 | 5,584 | 51 |
| Fixture semantic/nested-channel-select-union-stress | 1,361,370,676 | 191,162,016 | 1,471,990 |
| Fixture realworld/advanced-type-system-stress | 278,126,914 | 71,170,968 | 660,698 |
| Fixture realworld/plugin-runtime-pipeline-soundness | 83,516,052 | 21,387,221 | 180,311 |

The prototype commit will update this section with after numbers and a go/no-go
recommendation.

## Verdict (2026-07-08, measured)

No-go for full migration. A complete solve-local scalar-ID conversion of the numeric bound lane
(public APIs unchanged, byte-identical outputs) moved no state benchmark beyond noise and regressed
the advanced-type-system-stress fixture ~3.3% end-to-end. The string-key cost visible in profiles is
not reachable by lane-local rekeying. The only variant worth a future prototype is child-list subtree
walks replacing prefix scans in path-evidence/len-floor invalidation, measured the same way.
