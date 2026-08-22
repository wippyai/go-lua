# transitive-libs

These checker fixtures exercise mounted, transitive Placement summaries. A library
module is compiled once; its owner-issued formal and return relations are instantiated
at each mounted call position, and downstream store or publication effects contribute
ordinary monotone displacement facts. The manifests are live analyzer contracts. Their
`run.skip` fields only disable runtime execution of checker-only programs.

## Placement classes and the aggregate bucket mapping

Placement classes, lifetime-ordered:

```
Scalar < FrameLocal < ActorLocal < Shared < PinnedExternal
```

Static Placement admission:

- **proven-local** (FrameLocal / ActorLocal) — arena-allocated, never crosses a boundary.
- **proven-shared-or-sealed** (Shared) — escapes; sealed before share.

An input-dependent send is not an unknown Placement policy. The send edge contributes
`SharedHeap`, which monotonically dominates the local alternative at that position.
Whether lowering can keep a local original and conditionally move, seal-share, or copy
at the boundary is a separate strategy decision that consumes Placement together with
uniqueness and last-use evidence.

Manifest `placement{}` thresholds only expose aggregate buckets, so classes map onto
buckets as follows (this is the mapping used in both manifests):

| Placement class      | Aggregate bucket        |
| -------------------- | ----------------------- |
| Scalar, FrameLocal   | `stack`                 |
| ActorLocal           | `owned_heap`            |
| Shared               | `shared_heap`           |
| unresolved open/opaque evidence | `unknown`    |

## shared-lib-divergent-consumers

`lib.lua` exports `M.wrap(payload)`, which allocates exactly one fresh table `box`
and returns it. That allocation site is caller-fate-agnostic: its placement class is a
*function of what the consumer does with the returned box*. The relational summary over
`box` is what lets the single `lib.lua` analysis serve all three conclusions below.

Three consumers, each `require("lib")`, call `lib.wrap(...)`, and diverge:

| Consumer          | What it does with `box`                         | Substituted conclusion for `lib.lua` `box` | Bucket        |
| ----------------- | ----------------------------------------------- | ------------------------------------------ | ------------- |
| `consumer_send`   | `process.send("worker","topic", box)`           | **Shared** — send is a boundary; box escapes and is sealed before share | `shared_heap` |
| `consumer_local`  | reads `box.body` locally, then drops the box    | **FrameLocal / ActorLocal** (proven-local) — never crosses a boundary   | `stack` (or `owned_heap`) |
| `consumer_store`  | `ownership.store(box, registry)`                | **ActorLocal** (retained) — owned into long-lived actor state, no share | `owned_heap`  |

`main.lua` (entry, last in `files`) requires all three consumers and invokes each once,
so all three admission contexts are exercised against the same library summary.

Aggregate thresholds in the manifest are conservative and consistent with the table:
at least one `shared_heap` (from `consumer_send`), at least one `owned_heap` (from
`consumer_store`, which is unambiguously retained ActorLocal), at least one
`seal_before_share` (the send path), and `require_complete` — every site resolves to a
placement fact (no deferred sites in this fixture).

## deep-chain-mixed

A 3-frame chain: `main.lua` -> `mid.make` -> `lib.wrap`. The library allocation `box`
is created in `lib.wrap`, threaded up through `mid.make`, and its fate is decided two
frames up in `main.lua`. Two call paths mix distinct outcomes:

| Path in `main.lua` | Behavior                                              | Conclusion        | Bucket              |
| ------------------ | ----------------------------------------------------- | ----------------- | ------------------- |
| `local_path`       | `mid.make(...)` then reads `box.body`, drops the box  | **proven-local** (FrameLocal) | `stack`             |
| `mixed_path`       | `if cond then process.send(..., box) else read box.body` | **Shared** at the joined publication requirement | `shared_heap` |

`mixed_path` sends the box only on an input-dependent condition (`cond`). The exact
static requirement is nevertheless available: the send alternative raises the box to
`SharedHeap`; the local alternative cannot lower that monotone result. The manifest
therefore requires complete Placement publication, at least one shared allocation, and
zero `Unknown` rows. The separate `local_path` assertion remains a check that a distinct
local use can still be represented without treating the send policy itself as unknown.
