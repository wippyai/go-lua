# transitive-libs

Pending fixture pack for **placement-polymorphic library summaries**: one library
module is analyzed once, producing a *relational* escape/placement summary over its
boundary placeholders (`$0`/`$1`). Each consumer module admits that summary by
substituting its own call-site escape facts, and gets a **per-consumer placement
conclusion** from the single shared analysis. Same library bytecode, N plans.

These fixtures target machinery that does **not** exist yet — manifests today carry
only coarse boolean `ParamEscapes`, not relational escape/placement lanes. Every
fixture is therefore pending/skipped so the suite stays green:

- `check.skip` — `"pending: manifest relational escape/placement lanes + placement-polymorphic summaries (tasks 3eb96899, 150f00f9) not yet landed"`
- `run.skip`   — `"checker-only placement fixture"`

The `placement { ... }` block in each manifest is an **inert oracle** documenting the
aggregate expectation once the lanes land. Per-site / per-consumer conclusions live in
the tables below. Removing the two skips un-pends the pack when the machinery arrives.

## Placement classes and the aggregate bucket mapping

Placement classes, lifetime-ordered:

```
Scalar < FrameLocal < ActorLocal < Shared < PinnedExternal
```

Three-outcome admission model:

- **proven-local** (FrameLocal / ActorLocal) — arena-allocated, never crosses a boundary.
- **proven-shared-or-sealed** (Shared) — escapes; sealed before share.
- **deferred+promote** — input-dependent escape; the boundary inserts a runtime clone
  because no static choice dominates.

Manifest `placement{}` thresholds only expose aggregate buckets, so classes map onto
buckets as follows (this is the mapping used in both manifests):

| Placement class      | Aggregate bucket        |
| -------------------- | ----------------------- |
| Scalar, FrameLocal   | `stack`                 |
| ActorLocal           | `owned_heap`            |
| Shared               | `shared_heap`           |
| deferred+promote     | `unknown` / `no_fact`   |

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
| `mixed_path`       | `if cond then process.send(..., box) else read box.body` | **deferred+promote** | `unknown` / `no_fact` |

`mixed_path` sends the box only on an input-dependent condition (`cond`). No single
static placement dominates: choosing Shared unconditionally would over-promote the
`else` branch that stays local; choosing local would be unsound for the `then` branch
that escapes. The information-theoretically optimal decision is therefore **deferred**:
emit a runtime clone at the send boundary and keep the allocation local otherwise. In
the aggregate this site is counted as `unknown`/`no_fact` (runtime-decided), so the
manifest sets `require_complete: false` and allows `max_unknown: 1` / `max_no_fact: 1`,
while still asserting `min_stack: 1` for the proven-local `local_path` box.

## Pending on tasks 3eb96899 + 150f00f9

- **3eb96899** — manifest relational escape/placement lanes.
- **150f00f9** — placement-polymorphic summaries.

Both fixtures stay skipped until these land. When they do, removing the `check.skip`
and `run.skip` entries from each manifest un-pends the pack, and the `placement{}`
oracles become live assertions.
