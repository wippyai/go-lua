# Evidence-with-Origins Design

Status: design contract for adversarial review. Not yet wired.

## Problem

Today a judgment carries flat evidence: an ordered list of `Evidence{kind, trust,
span, message}` entries (`kind` = abstract fact | user assertion, `trust` = proven |
claimed). Evidence explains *why a verdict holds* but not *where the offending value
came from*. A Refuted `type.nil.unsafe_use` can say "x may be nil" without saying nil
was born in the else-less `if` at L6 and survived the join at L9 before reaching the
use at L10.

Witness traces (task `13897ee5`), send-safety seal points (task `ad6c114f`), and
verified repair candidates all need the causal chain, not just the terminal fact. This
document specifies **evidence with origins**: an origin graph the solver already
implies, surfaced as first-class provenance so a trace renders as a born → survives →
use narrative and repairs can target the birth site.

## Principle

The origin graph is a *projection of solved dataflow*, never a re-derivation. A fact
cell at a program point already knows its predecessor cells (the lattice transfer that
produced it). Origins record the load-bearing subset of those edges keyed to the value
that carries the disproof, so rendering is a walk, not an analysis.

## Origin node structure

An origin node is an immutable, interned record:

    OriginNode {
        ID        OriginID      // dense u32, arena-interned per module solve
        Site      Span          // file/line/column of the producing program point
        Kind      OriginKind    // Birth | Assign | Join | Narrow | Widen | Call | Return | Seal
        Subject   SubjectRef    // the value/place this node is about (canonical read-model ref)
        FactDelta FactRef       // the axis value asserted here (e.g. nilable=true, IntRange=[1,+inf))
        Preds     []OriginID    // immediate provenance edges (empty for Birth)
        Cause     CauseRef      // optional: syntactic reason token (e.g. "else arm has no assignment")
    }

Key points:

- **Interned + dense.** `OriginID` is a per-solve u32 index into a flat slab. Nodes are
  value types; edges are `OriginID` (not pointers) so the graph adds zero GC pointers to
  long-lived state (memory rule: scalar handles into central tables). A judgment
  references an origin chain by a single `OriginID` (the terminal use node); the walk
  materializes on demand.
- **Subject-keyed, not point-keyed.** Two distinct values live at the same span (e.g. a
  reused local) get distinct subjects, so traces never cross values.
- **Kind is the render verb.** `Birth` → "born", `Join` → "survives the join", `Narrow`
  → "narrowed to", `Seal` → "sealed at". The renderer is a `Kind`→template table.
- **FactDelta ties the node to an axis.** A trace is per-axis: the nil trace walks only
  nilability-carrying predecessors; the IntRange trace walks range-carrying ones. This
  keeps chains short and on-topic (see JOIN semantics).

## JOIN semantics — the central decision

At a control-flow join, an axis value merges predecessors via the lattice `Join`. The
question is which origin predecessors to keep on the merged cell.

**Option A — union.** Keep every incoming origin edge. Complete provenance; unbounded
fan-in. A value merged in a loop accumulates one predecessor per back-edge iteration →
O(iterations) edges, quadratic traces, unbounded storage. Rejected.

**Option B — bounded-set (recommended).** Keep at most *k* representative predecessors
per merged cell, chosen by a deterministic dominance/salience rule: prefer the
predecessor whose `FactDelta` is *responsible* for the merged value (the join arm that
contributed the disproving lattice element — e.g. the nil-introducing arm), then the
nearest dominating birth, then drop the rest. `k` small (2–3). Bounded storage, bounded
trace length, and the kept edges are exactly the ones a witness narrative needs
("survives *this* join because *this* arm introduced nil"). Ties broken by lowest
`OriginID` for determinism.

**Option C — drop.** Keep only the terminal fact; recompute a chain lazily on demand
from re-run dataflow when a judgment refutes. Zero steady-state storage; but the lazy
recompute is a second analysis (violates the projection principle) and races the
incremental invalidation model. Rejected as the primary path; acceptable as a
degradation fallback under memory pressure (see storage strategy).

### Soundness argument for bounded-set

The verdict is computed from the merged lattice value, which is independent of the
origin set — origins are metadata, so dropping predecessors **cannot change any
verdict**. Soundness reduces to *trace faithfulness*: every node on a rendered chain is
a real program point whose `FactDelta` really contributed to the merged value along a
real path. Bounded-set preserves this because it only ever *removes* edges; it never
fabricates one. The selection rule guarantees the *responsible* predecessor is retained,
so the chain always reaches a birth that actually justifies the disproof. The cost of
bounding is *completeness of provenance* (some contributing paths omitted), not
*correctness*. We accept incompleteness: a witness trace is an existence proof of one
disproving path, not an enumeration of all of them.

## Widening interaction

Widening jumps a cell to a coarser element (e.g. `IntRange` → `[1,+inf)`). The widened
cell gets a `Widen` origin node whose predecessor is the pre-widening cell and whose
`Cause` records the widening site. This matters for two consumers:

- **Traces** render "widened to [1,+inf) at the loop head" so an imprecision-driven
  disproof is legible as *widening-induced*, not a real bound.
- **Narrowing recovery** (task `7c17de64`): the bounded decreasing pass re-descends the
  widened cell. Each narrowing step appends a `Narrow` node (pred = the widened node),
  so a recovered bound carries its own provenance ("widened to [1,+inf), narrowed back to
  [1,10]"). A judgment that would refute on the widened value but *proves* on the
  narrowed value renders no trace; the `Narrow` chain exists only to explain recovered
  precision when asked (hover, JIR export).

Widening never merges origins (single predecessor), so it needs no bounding.

## Storage cost strategy

- **Per-solve arena.** Origin slabs live in the module solve arena, freed wholesale at
  solve completion. Long-lived artifacts (manifests, cross-module summaries) store *no*
  origin graphs — only the terminal `FactRef`s. Cross-module traces stitch at render
  time from the boundary summary's recorded seam nodes (see migration).
- **Lazy materialization.** Only judgments that *fire* (Refuted / conditional / Unknown
  with a diagnostic) retain their terminal `OriginID`. Proven judgments discard origins
  unless export/hover requests them, in which case they are recomputed from the still-live
  arena during the same solve.
- **Bounded fan-in (k)** caps per-node cost; **subject-keying** caps node count to
  distinct (place × axis-transition) pairs, which is linear in program size, not in path
  count.
- **Degradation.** Under the fixture memory guard or admission-time budget, fan-in `k`
  can drop to 1 (birth + terminal only) or to Option C (recompute-on-refute). This is a
  precision-of-*explanation* knob with zero soundness or verdict impact.

## Migration path from the current evidence axis

1. **Additive origin slab.** Add the `OriginNode` slab + `OriginID` to solved state
   behind an accessor. No producer changes; slab stays empty until populated.
2. **Populate at transfer.** The lattice transfer/join/widen/narrow steps that already
   compute merged cells emit origin nodes for the axes that carry disproofs first
   (nilability, IntRange, escape/placement, freshness). One axis at a time; each is a
   contained change to that axis's transfer.
3. **Evidence adapter.** Today's `Evidence{kind, trust, span, message}` becomes a
   *view* over an origin chain: `kind`/`trust` derive from the terminal node's provenance
   (a `Birth` from a user annotation → `user assertion`/`claimed`; a solver-derived
   `Join`/`Narrow` → `abstract fact`/`proven`), `span` = node `Site`, `message` =
   `Kind`+`FactDelta` template. Existing flat-evidence fixtures keep passing because the
   adapter yields the same terminal entries; origin-ordered fixtures (the `origins/` pack)
   additionally assert the intermediate `Join` hops via `render_ordered_contains`.
4. **Boundary seams.** Cross-module summaries record a single `Seam` origin per exported
   place (birth-or-return site + axis), so a consumer-side trace can render "value from
   `lib.wrap` return (lib.lua:L)" without shipping the library's full origin arena.
5. **Retire ad-hoc evidence builders.** Once producers read origins, the hand-built
   evidence lists in judgment producers collapse into the adapter (kills a slice of the
   readmodel residue flagged in journal #1417).

## How witness traces render

A witness trace for a fired judgment is: walk `Preds` from the terminal `OriginID`
depth-first along the judgment's axis, ordered by `Site` (origin → use), bounded by `k`
per join. Each node emits one line via the `Kind` template:

    error[type.nil.unsafe_use]: x may be nil at this call
      --> main.lua:10:12
    because:
      1. proven:   x born nil            (main.lua:6:11)   [Birth]
      2. proven:   survives if-join      (main.lua:9)       [Join, cause: else arm has no assignment]
      3. proven:   reaches use           (main.lua:10:12)   [Use]
      claimed: x declared string?        (main.lua:6:14)    [Birth, user assertion]
    help: assign x on every branch, or guard with `if x ~= nil then`

`render_ordered_contains` in the `origins/` fixtures pins this ordering. Conditional
verdicts (send-safety `Proven-if-sealed-at-P`) render the same walk with a `Seal` node
marking P and a "would be proven if sealed at P (main.lua:13)" tail.

## Open questions for adversarial review

1. **k and the salience rule.** Is dominance-then-birth the right predecessor-selection
   order, or should the *most recently narrowed* predecessor win for numeric axes? Does
   any real judgment need k>3? Adversary: construct a join where the responsible arm is
   not dominance-nearest and the trace misleads.
2. **Per-axis vs unified chains.** Per-axis traces are short but a single diagnostic
   spanning two axes (nil *and* a bad range) renders two chains. Merge them, or keep
   separate and let the renderer interleave by `Site`?
3. **Loop-carried births.** A value born inside a loop body and used after: does the
   Birth node point at the *first* iteration's birth, the widened representative, or the
   syntactic literal? Which reads correctly to a code-gen repair consumer?
4. **Seam granularity.** One `Seam` per exported place may be too coarse for
   field-granular placement (a returned record where only one sub-field escapes). Do
   seams need field paths, and does that reintroduce unbounded storage across modules?
5. **Cause tokens vs re-deriving syntax.** `Cause` (e.g. "else arm has no assignment")
   is a syntactic reason. Storing it risks an AST-past-the-fact-boundary violation
   (judgment_ir principle). Should `Cause` be an enum of structural reasons lowered at
   fact time rather than free text?
6. **Trust of a `Narrow` origin.** A narrowed bound is `proven` by the solver — but its
   *recovery* depended on the narrowing pass terminating at the true bound. Is there a
   case where a narrowed value should render as lower trust than a directly-proven one?
