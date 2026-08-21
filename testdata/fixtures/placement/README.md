# placement — allocation-placement fixtures

Executable specifications for per-site allocation placement, distilled from
benchmark functions, transfer cases, recursive/library cases, and clean
positives. Each fixture is a minimal self-contained Lua module reproducing one
placement-relevant shape; the canonical semantics live in `domain/placement`.

## Status: CANONICAL STATIC PATH GREEN

The declarative Placement axis, Heap-root denominator, seed rule, query family,
domain-owned result codec, and schema-driven `manifest.allocation` projection
are live. The bounded `./domain/placement/...` gate passes. Every manifest is
judged through the canonical Placement result surface; legacy run metadata is
intentionally absent because the harness does not honor it.

The remaining intentional uncertainty is capability-driven: live actor/thread
and destination-context identity must come from an owner-authenticated runtime
publication. Until that producer is mounted, affected transfers classify as
`Unknown`; Placement does not fabricate a same-context or cross-context fact.
This is an explicit external input boundary, not an unfinished static
Placement consumer. The repository-wide acceptance runner is tracked
separately because its current memory profile is not this fixture gate.

Fixtures with a complete static producer path pin `require_complete: true` and
`max_unknown: 0`. `webhooks-mlist-row-filter` now does so as well: the returned
container has an exact OwnedHeap return demand, and borrowed input rows do not
mint Program allocation roots. Input-dependent member selection therefore does
not leave an unclassified allocation row in the public denominator.

## Placement class model

The canonical allocation-root lattice is:

    Bottom < Stack < OwnedHeap < SharedHeap < Unknown

`Bottom` is the absent/unreachable value; `Stack` is a frame-local allocation;
`OwnedHeap` is retained within the owning actor/runtime; `SharedHeap` crosses a
shared, actor, or thread boundary; and `Unknown` is conservative top for an
open, opaque, or otherwise unresolved path. `Interpreter` and `Register` are
JIT-only spellings and are not Placement result classes.

The result denominator is one exact Heap allocation root. Scalar values do not
mint roots, and fields are not roots: containment propagates an escape through
the referenced graph so a holder and its payload may receive different
classes. Alternate paths join monotonically at the same root.

The benchmark prose below retains the older descriptive labels (`Scalar`,
`FrameLocal`, `ActorLocal`, `Shared`, and `deferred+promote`) to explain the
fixture shape. Their manifest assertions use the canonical buckets: Scalar has
no allocation row, FrameLocal maps to `stack`, ActorLocal maps to `owned_heap`,
Shared maps to `shared_heap`, and an unresolved deferred case remains
`unknown` until its static escape evidence is complete.

## Benchmark fixtures (expected per-site class)

### bridge-main-event-loop (bridge.lua:282-716 main)
| site | class | rationale |
|---|---|---|
| `processed` counter | no Heap root | numeric accumulator |
| `key` (derived string) | Stack | derived per iteration, dropped |
| `scratch` (Response literal) | Stack | built, read locally, never escapes |
| `cache` (module map) | OwnedHeap | closure-captured actor state |
| `msg` at `sink:send(msg)` | SharedHeap demand | conditional forwarding (`kind == "forward"`) joins at its source root |

### hub-inbound-parking (channel/hub.lua:44-226 main)
| site | class | rationale |
|---|---|---|
| `parked` map | OwnedHeap | retained inbound registry |
| `order` index | OwnedHeap | insertion order index |
| `m` at `parked[m.topic] = m` | OwnedHeap demand | inbound handle is retained by the owned registry |
| `m.topic` appended | Stack | derived string into actor-local index |

### notify-cache-ttl-registry (notify_cache.lua:15-56)
| site | class | rationale |
|---|---|---|
| `registry` map | OwnedHeap | module cache |
| `entry` (Entry literal) | OwnedHeap | stored into the cache |
| `hit` (lookup result) | Stack borrow | borrowed handle, dies at return; entry stays owned |

### enrichment-debounce (enrichment_service.lua:240-346 main)
| site | class | rationale |
|---|---|---|
| `pending` map | OwnedHeap | debounced events awaiting flush |
| `last` map | OwnedHeap | last-seen timestamps |
| `prev` | Stack borrow | borrowed timestamp |
| `fresh` | no Heap root | boolean derivation |
| `e` at `pending[e.key] = e` | OwnedHeap demand | event retained by the owned registry |

### webhooks-mlist-row-filter (webhooks/repo.lua:236-265 M.list)
| site | class | rationale |
|---|---|---|
| `out` (returned subset) | OwnedHeap demand | the return boundary owns the new container; member routes are input-dependent |
| `r` source rows | Stack borrow | iterated, selectively aliased into `out` |

This remains a conditional containment case: the result container has a known
return demand, while each selected source route must be joined by the
containment consumer. The borrowed input rows do not mint Program allocation
roots, so the public denominator is complete with the returned container at
`OwnedHeap` and no `Unknown` allocation row.

## Clean positives (all proven-local; zero shared, zero deferred)

### list-inbox-clean (list_inbox.lua:103-117)
`n` has no Heap root; `rows`/`r` are borrowed and dropped. No allocation leaves the frame.

### enrichment-derive-clean (enrichment_service.lua:90-105)
`top` has no Heap root, `best` is a Stack value; DB rows die local; a derived string escapes. Derive-then-drop.

### upload-materialized-row-clean (upload_read_model.lua materialized_row)
`kb` has no Heap root; `view` is OwnedHeap — a materialized read-model row returned *within* the actor
(no send), so it has no shared demand.

### connection-negotiator-clean (connection_negotiator.lua:64)
`proto` has no Heap root; option records are borrowed. A single scalar string leaves the frame. No allocation escapes.

### sealpoint-frozen-send
`table.freeze` is shallow, so the fixture seals `packet.meta` before `packet`.
The contract requires both allocation roots to carry `DeepFrozen=Proven` at
the send point; freezing only the outer table is not sufficient.

## Enabling the fixtures

Keep the per-root `placement` thresholds as the fixture contract. Tighten
`min_stack`/`min_owned_heap`/`min_shared_heap`, depth and kind evidence, and
`max_unknown` only from exact canonical result rows. An input-dependent path
must be justified by its static displacement evidence; it is not a separate
runtime placement class.
