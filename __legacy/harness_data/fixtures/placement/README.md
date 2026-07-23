# placement — kickside placement benchmark fixtures (pending)

Executable specifications for per-site allocation placement, distilled from the five
kickside benchmark functions plus four clean positives (journal task `150f00f9`,
events #1413/#1414). Each fixture is a minimal self-contained Lua module reproducing
one benchmark's placement-relevant shape.

## Status: PENDING

Every fixture sets `check.skip` and `run.skip` so the fixture sweep stays green.
The gap these encode is the checker precision the placement task still owes:
derivation sensitivity, field granularity, closure-capture lists, and the
input-dependent deferred+promote residual. When that machinery lands, delete the two
skip lines and tighten the manifest `placement` thresholds to the per-site table below.

## Placement class model

Lifetime-ordered lattice (journal #1414/#1415):

    Scalar < FrameLocal < ActorLocal < Shared < PinnedExternal

Three-outcome total model:
- **proven-local** — `Scalar` / `FrameLocal` / `ActorLocal`. Lives in the actor arena,
  freed wholesale; zero cross-actor risk. `FrameLocal` is a proof-only future tier
  (bump-pointer in the coroutine/thread frame, freed at frame completion); unproven
  frame-locals fall back to `ActorLocal` (no-maybe theorem preserved).
- **proven-shared-or-sealed** — `Shared`. A retained handle published zero-copy at the
  send boundary (Content model), or sealed-on-transfer.
- **deferred+promote** — no static class dominates; the runtime deep-clones at the
  escape boundary. Information-theoretically optimal for input-dependent escape;
  promotion counters drive profile-guided class flips (the ratchet).

Mapping to the harness `placement` buckets used in the manifests:
`Scalar`+`FrameLocal` -> `stack`; `ActorLocal` -> `owned_heap`; `Shared` -> `shared_heap`;
`deferred+promote` -> counted as `unknown` / `no_fact` (runtime-decided).

## Benchmark fixtures (expected per-site class)

### bridge-main-event-loop (bridge.lua:282-716 main)
| site | class | rationale |
|---|---|---|
| `processed` counter | Scalar | numeric accumulator |
| `key` (derived string) | FrameLocal | derived per iteration, dropped |
| `scratch` (Response literal) | FrameLocal | built, read locally, never escapes |
| `cache` (module map) | ActorLocal | closure-captured actor state |
| `msg` at `sink:send(msg)` | deferred+promote | input-dependent forward (`kind == "forward"`) |

### hub-inbound-parking (channel/hub.lua:44-226 main)
| site | class | rationale |
|---|---|---|
| `parked` map | ActorLocal | retained inbound registry |
| `order` index | ActorLocal | insertion order index |
| `m` at `parked[m.topic] = m` | Shared | inbound handle parked by shared-handle retain (zero-copy) |
| `m.topic` appended | FrameLocal | derived string into actor-local index |

### notify-cache-ttl-registry (notify_cache.lua:15-56)
| site | class | rationale |
|---|---|---|
| `registry` map | ActorLocal | module cache |
| `entry` (Entry literal) | ActorLocal | stored into the cache |
| `hit` (lookup result) | FrameLocal | borrowed handle, dies at return; entry stays actor-local |

### enrichment-debounce (enrichment_service.lua:240-346 main)
| site | class | rationale |
|---|---|---|
| `pending` map | ActorLocal | debounced events awaiting flush |
| `last` map | ActorLocal | last-seen timestamps |
| `prev` | FrameLocal | borrowed timestamp |
| `fresh` | Scalar | boolean derivation |
| `e` at `pending[e.key] = e` | Shared | event retained by shared-handle retain |

### webhooks-mlist-row-filter (webhooks/repo.lua:236-265 M.list)
| site | class | rationale |
|---|---|---|
| `out` (returned subset) | deferred+promote | membership is input-dependent (`only_active`); no oracle-free static choice dominates |
| `r` source rows | FrameLocal (borrow) | iterated, selectively aliased into `out` |

This is the canonical residual case: `deferred+promote` is optimal, and promotion
counters feed profile-guided flips.

## Clean positives (all proven-local; zero shared, zero deferred)

### list-inbox-clean (list_inbox.lua:103-117)
`n` Scalar; `rows`/`r` borrowed and dropped. Only a scalar leaves the frame. No promotion.

### enrichment-derive-clean (enrichment_service.lua:90-105)
`top` Scalar, `best` FrameLocal string; DB rows die local; a derived string escapes. Derive-then-drop.

### upload-materialized-row-clean (upload_read_model.lua materialized_row)
`kb` Scalar; `view` ActorLocal — a materialized read-model row returned *within* the actor
(no send), so proven actor-local, no promotion.

### connection-negotiator-clean (connection_negotiator.lua:64)
`proto` Scalar; option records borrowed. A single scalar string leaves the frame. No allocation escapes.

## Un-pending

For each fixture: remove `check.skip` and `run.skip`; then encode the table above as
tighter `placement` thresholds (`min_stack`/`min_owned_heap`/`min_shared_heap`,
`min_seal_before_share`, `min_*_kind`, and `max_unknown` — left non-zero only where a
`deferred+promote` residual is expected, i.e. bridge and webhooks).
