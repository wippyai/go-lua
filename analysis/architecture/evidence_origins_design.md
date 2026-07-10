# Evidence-with-Origins Design

Status: v2 design contract after adversarial review. Not yet wired.

## Problem

Today a judgment carries flat evidence: an ordered list of `Evidence{kind, trust,
span, message}` entries (`kind` = abstract fact | user assertion, `trust` = proven |
claimed). Evidence explains *why a verdict holds* but not *where the offending value
came from*. A Refuted `type.nil.unsafe_use` can say "x may be nil" without saying nil
was born in the else-less `if` at L6, survived the join at L9, crossed a call boundary
as a returned field, and reached the use at L10.

Witness traces (task `13897ee5`), send-safety seal points (task `ad6c114f`), and
verified repair candidates all need the causal chain, not just the terminal fact. This
document specifies **evidence with origins**: a provenance lane carried beside solved
facts so a trace renders as a born -> survives -> returned -> use narrative and repairs
can target the responsible birth site.

## Principle

Origins are a *projection of solved dataflow*, but they are not seam-only metadata and
they cannot be recovered after the fact from today's flat evidence. The current solved
state and summary records do not retain enough predecessor structure. Therefore v2 makes
provenance an explicit, bounded lane in the same transfer, join, widening, and summary
substitution operations that carry the facts themselves.

The invariant is:

    value/path/fact substitution across a call boundary must substitute its origin slot
    at the same time, with the same symbolic path mapping.

If a callee summary says `return[0].field.nilable = true`, it also carries a symbolic
origin slot for `return[0].field:nilability`. At application in the caller, the
callboundary substitutes both the returned path and that origin slot into caller-frame
subjects. This is the central soundness requirement for returned field/path facts.
Without it, a caller can refute on a returned path but cannot explain where the returned
value came from.

## Origin node structure

An origin node is an immutable, interned record:

    OriginNode {
        ID        OriginID      // dense u32, arena-interned per module solve
        Site      Span          // file/line/column of the producing program point
        Kind      OriginKind    // Birth | Assign | Join | Widen | Narrow | Call | Return | Seal | Use
        Subject   SubjectRef    // value/place/path this node is about
        Axis      AxisRef       // nilability, IntRange, placement, freshness, ...
        FactDelta FactRef       // the axis value asserted here, e.g. nilable=true
        Preds     []OriginID    // immediate provenance edges, bounded by axis policy
        Cause     CauseRef      // structural enum + params, never free text
    }

    CauseRef {
        Code   CauseCode        // MissingElseAssign | LoopHeadWiden | SummaryReturn |
                                // CallBoundarySubst | UserClaim | GuardNarrow | ...
        Params map[string]Atom  // branch id, path, callee key, summary digest, limit, ...
    }

Key points:

- **Interned + dense.** `OriginID` is a per-solve u32 index into a flat slab. Nodes are
  value types; edges are `OriginID` (not pointers) so the graph adds zero GC pointers to
  long-lived state. A judgment references an origin chain by a terminal `OriginID` per
  axis; the walk materializes on demand.
- **Subject-keyed, not point-keyed.** Two distinct values live at the same span (e.g. a
  reused local) get distinct subjects, so traces never cross values. Subject identity is
  the JIR stable identity layer, not raw `cfg.Point`.
- **Kind is the render verb.** `Birth` -> "born", `Join` -> "survives the join",
  `Return` -> "returned from", `Widen` -> "widened at loop head", `Narrow` -> "narrowed
  to", `Seal` -> "sealed at". The renderer is a `Kind` + `CauseCode` template table.
- **FactDelta ties the node to one axis.** Chains are per-axis. A nilability trace walks
  only nilability predecessors; an IntRange trace walks only range predecessors. A
  multi-axis diagnostic stores multiple terminal origins, and the renderer may interleave
  the separate chains by span.
- **Cause is structural.** No node stores prose like "else arm has no assignment".
  Producers store `CauseCode=MissingElseAssign` plus branch/path parameters; renderers
  own the human wording.

## JOIN semantics - the central decision

At a control-flow join, an axis value merges predecessors via the lattice `Join`. The
origin lane joins at the same program point and axis.

**Option A - union.** Keep every incoming origin edge. Complete provenance; unbounded
fan-in. A value merged in a loop accumulates one predecessor per back-edge iteration ->
O(iterations) edges, quadratic traces, unbounded storage. Rejected.

**Option B - bounded-set (required).** Keep at most **k=2** representative predecessors
per merged cell, chosen by an axis-specific salience rule. Salience is based on lattice
responsibility for that axis, not generic dominance-first ordering:

- nilability: predecessor(s) that introduce or preserve `nilable=true`;
- range: predecessor(s) that set the bound responsible for the refutation or widening;
- placement/escape: predecessor(s) that introduce the escaping/shared placement class;
- freshness/seal: predecessor(s) that lose freshness or cross the relevant seal edge.

Ties are broken by deterministic subject/path order and then lowest `OriginID`. An axis
may request a third candidate only with a proof that two independent responsible
contributors are both required for a faithful explanation; v2's default and budget is
k=2.

**Option C - drop.** Keep only the terminal fact; recompute a chain lazily on demand
from re-run dataflow when a judgment refutes. Zero steady-state storage; but the lazy
recompute is a second analysis and cannot reconstruct interprocedural origin
substitution from summaries that did not carry it. Rejected as the primary path;
acceptable only as a degradation fallback that explicitly marks explanation precision as
degraded.

### Soundness argument for bounded-set

The verdict is computed from the merged lattice value, which is independent of the
origin set. Dropping predecessors cannot change any verdict. Soundness reduces to
*trace faithfulness*: every rendered node is a real program point or summary boundary
whose `FactDelta` contributed to the merged value along a real path. Bounded-set
preserves this because it only removes edges; it never fabricates one. The
axis-specific salience rule keeps the responsible contributor for the disproving axis,
so the chain reaches a birth or summary seam that justifies the disproof. The cost of
bounding is *completeness of provenance*, not correctness.

## Widening interaction

Widening is part of the provenance lane, not a renderer annotation. A loop-carried value
has:

1. a **syntactic `Birth`** at the literal/allocation/assignment site inside the loop;
2. an explicit **loop-head `Widen`** node at the widening point, with
   `CauseCode=LoopHeadWiden` and parameters for loop header, axis, pre/post facts;
3. optional **`Narrow`** nodes from the decreasing pass, with
   `CauseCode=LoopHeadNarrow`.

There is no "first iteration" origin. The birth is the syntactic birth, and the loop
head explains the abstraction step that made the value stable across iterations. This
keeps repair consumers pointed at code they can edit while keeping imprecision visible.

Narrowing remains `proven` when the narrowing pass is sound. Trust does not drop because
the precision was recovered by narrowing; instead the node records precision provenance
(`Widen` -> `Narrow`) so hovers/JIR can explain why the value was temporarily coarser.

## Summary and callboundary carriage

Interprocedural provenance is a first-class part of summaries. A summary lane is:

    SummaryLane {
        Path       SymbolicPath        // formal, captured upvalue, return, exported field
        Axis       AxisRef
        Value      product.Value
        Boundary   BoundaryFactRef
        Origins    BoundedOriginSet    // symbolic origin ids for this path/axis
    }

`Origins` is not a full callee arena dump. It is the bounded, exported provenance needed
to explain exported facts. Summary origin records use symbolic subjects (`formal[0]`,
`return[0].field`, `upvalue[x]`) and structural causes. At call application, the
callboundary substitutes:

- formal paths to actual argument paths;
- return/exported paths to caller destination paths;
- symbolic origin subjects to caller-frame subjects;
- callee summary seams to caller `Call`/`Return` origin nodes.

Summary widening joins `Origins` with the same k=2, axis-specific salience used inside a
body. This matters for recursive calls and mutually recursive summaries: the summary
value and the summary origin slot must converge together. A summary digest includes the
origin-lane schema version and exported origin slots that can affect rendered
provenance; it does not include non-exported callee-internal arena nodes.

Seam granularity is field/path-level and bounded by exported summary facts. A returned
record with only `return[0].cfg.token` escaping gets an origin slot for that path/axis,
not one monolithic return seam and not every internal field.

## Storage cost strategy

- **Per-solve arena.** Intra-body origin slabs live in the module solve arena and are
  freed wholesale at solve completion. Cached solved entries retain terminal origin ids
  only for requested/fired judgment axes.
- **Sparse summary slots.** Cross-module artifacts carry only exported
  `(symbolic path, axis) -> bounded origin set` slots. This is linear in exported
  summary facts, not callee CFG size or path count.
- **Dense handles.** Runtime origin edges are u32 handles into slabs. Summary origin ids
  are compact ordinals in the summary blob and instantiate into caller arena nodes only
  when the summary fact is applied.
- **k=2 fan-in.** Each join/widened summary lane stores at most two predecessor handles
  per axis. Subject/path/axis interning prevents duplicate nodes for the same transition.
- **Corpus-scale budget.** Admission and harness runs may solve thousands of modules.
  The budget target is O(exported facts * axes * k) persistent origin metadata per
  module and O(body facts * axes * k) transient metadata per active solve. Proven
  judgments do not export full traces by default; harness `origins=all` is an explicit
  high-detail mode.
- **Degradation.** Under memory pressure, per-axis fan-in can drop to 1 and proven trace
  retention can be disabled. The JIR/LSP result must mark explanation precision as
  degraded. Verdict computation is unaffected.

## Migration path from the current evidence axis

1. **Additive origin lane.** Add `OriginNode` slabs plus per-cell origin handles to the
   solved state behind accessors. The lane is empty until individual axes opt in.
2. **Populate at transfer.** The transfer/join/widen/narrow steps that already compute
   merged cells emit origin nodes for disproof-carrying axes first (nilability, IntRange,
   escape/placement, freshness). Each axis owns its salience function.
3. **Add summary origin slots.** Extend summary lanes with symbolic origin slots for
   exported path/axis facts. Summary join/widen and digesting include these slots.
4. **Substitute at callboundary.** Summary application substitutes paths and origins in
   one operation. Returned field/path facts compose caller-frame origins with callee
   summary origins through `Call`/`Return` nodes.
5. **Evidence adapter.** Today's `Evidence{kind, trust, span, message}` becomes a view
   over origin chains. `kind`/`trust` derive from node provenance; `span` = node `Site`;
   `message` = renderer template for `Kind` + `CauseCode` + `FactDelta`.
6. **Retire ad-hoc evidence builders.** Once producers read origins, hand-built evidence
   lists in judgment producers collapse into the adapter. The `origins/` fixture pack
   asserts intermediate join, widen/narrow, and callboundary hops via
   `render_ordered_contains`.

## How witness traces render

A witness trace for a fired judgment walks the terminal `OriginID` for the judgment's
axis, follows `Preds` in bounded salience order, and renders origin -> use. Multi-axis
diagnostics keep chains separate; the renderer may interleave lines by `Site` but never
merges the chains semantically.

    error[type.nil.unsafe_use]: x may be nil at this call
      --> main.lua:10:12
    because:
      1. proven:   x born nil                  (main.lua:6:11)   [Birth]
      2. proven:   survives if-join            (main.lua:9)       [Join, MissingElseAssign]
      3. proven:   returned as lib.wrap().out   (lib.lua:4:5)     [Return, SummaryReturn]
      4. proven:   reaches use                 (main.lua:10:12)  [Use]
      claimed: x declared string?              (main.lua:6:14)   [Birth, UserClaim]
    help: assign x on every branch, or guard with `if x ~= nil then`

Conditional verdicts (send-safety `Proven-if-sealed-at-P`) render the same walk with a
`Seal` node marking P and a "would be proven if sealed at P (main.lua:13)" tail.

## Changelog v2 - review counters resolved

- **Origins cannot be seam-only metadata.** Resolved by adding origin slots to summary
  lanes and requiring callboundary substitution of paths and origins together.
- **Current solved state lacks predecessor edges.** Resolved by making provenance an
  explicit lane in transfer, join, widen, narrow, and summary widening.
- **Returned field/path facts need composed origins.** Resolved by field/path-granular
  summary slots bounded by exported facts.
- **k and salience were underspecified.** Resolved with default k=2 and axis-specific
  lattice-responsibility salience.
- **Loop births were ambiguous.** Resolved with syntactic births plus explicit loop-head
  `Widen`/`Narrow` nodes; never "first iteration".
- **Cause text violated the no-AST/prose boundary.** Resolved with structural
  `CauseCode` plus parameters.
- **Per-axis chain behavior was unclear.** Resolved by storing per-axis terminal chains
  separately and allowing only renderer-level interleaving.
- **Corpus memory cost was understated.** Resolved with sparse exported origin slots,
  u32 handles, k=2 fan-in, and explicit degraded explanation mode.

## Open questions for adversarial review

1. **Summary digest sensitivity.** Which origin-slot changes must affect summary digests:
   all exported provenance changes, or only changes visible to JIR/LSP consumers that
   request origins?
2. **Axis salience audits.** Nilability, range, placement, and freshness need concrete
   salience tests. Are there axes where k=2 cannot produce a faithful single-diagnostic
   explanation?
3. **Origin schema versioning.** Should origin-lane schema versioning live inside summary
   digests, JIR code registry versions, or both?
