# LSP Server Design

> **Superseded foundation.** The source-identity, resolver, result-tag, debug
> correlation, and incrementality assumptions in this historical server
> proposal are superseded by [LSP Core Design](lsp_core_design.md). In
> particular, `DocumentVersion`/`ResultVersion` are not durable identities,
> body-level reuse is not yet implemented, and hosts must materialize source
> and resolution snapshots before invoking the checker. This document remains
> useful only for the later server-side scheduling and projection discussion,
> subject to that foundation.

Status: v2 design contract after adversarial review. Not yet wired.

## Principle

The language server is a **thin projection of solved judgments**, not a second analyzer.
Every LSP response - diagnostics, hover, code actions, semantic tokens - is a query over
the same solved state and JIR that the diagnostics renderer and the harness consume.
There is one checker (judgment_ir principle); the LSP is a transport and a versioned
incremental-invalidation scheduler around it.

This is the SOTA lever: rust-analyzer and gopls re-implement large parts of their
compiler's analysis inside the server (their own type inference, name resolution, and
flow views) because the batch compiler was never designed to answer editor queries
incrementally. Here the *checker already produces judgments as data* (verdicts, evidence,
origins, verified repairs). The server does not re-derive; it schedules solves and reads
tagged results.

## What SOTA means here (and what we skip)

**We match or exceed:**
- **Judgment-native diagnostics.** Diagnostics are checker verdicts, not a separately
  maintained lint pass. A diagnostic *is* a Refuted/conditional judgment with its origin
  trace as related-information - richer than a gopls type error, which carries no
  provenance chain.
- **Verified code actions.** Quick-fixes are *counterfactually verified* repairs (JIR
  `Repair`, task `13897ee5`): the server offers only edits the checker re-proved flip the
  verdict to Proven. rust-analyzer's assists are syntactic and unverified; ours carry a
  proof. No "apply and hope."
- **Pull-model diagnostics** (LSP 3.17 `textDocument/diagnostic` + workspace pull) where
  the client observes the server's completed-result cache. Pull never blocks on an
  in-flight solve and never returns untagged stale diagnostics.
- **Provenance hover.** Hover surfaces the solved type *and*, on a diagnosed subject, the
  origin trace ("nil born at L6, survives join L9").

**We deliberately skip:**
- **Whole-workspace live re-index on keystroke.** Keystrokes use per-body input digests
  and an intra-unit dependency graph. Manifest digests remain save/admission granularity
  for module-boundary propagation.
- **Macro/build-system integration, formatting, rename-across-crates.** Out of scope for
  v1; rename is a later item and rides subject refs, not a bespoke index.
- **Partial checker results.** The service does not publish a partial solve or
  partial-result flag. On an unparseable buffer it may serve the last completed
  result for its own document version as stale, alongside parser feedback; it
  never infers from a malformed AST.

## Architecture

    client -- LSP --> server --> SolveScheduler --> checker
                         |              |
                         |              +-- BodyInputGraph (versioned intra-unit deps)
                         |              `-- SolveCache (tagged completed results)
                         `-- projections: diagnostics | hover | code actions | semantic tokens

The server owns a `SolveCache` keyed by unit digest and a `BodyInputGraph` keyed by
document version. A completed cache entry holds the unit's solved readmodel + judgments +
JIR plus:

    SolveResultTag {
        ResultVersion    u64       // monotonically increasing per server
        DocumentVersion  int       // LSP text document version solved
        UnitDigest       Digest
        BodyDigests      map[BodyID]Digest
        Profile          CompileProfile
    }

All projections return data tagged with `ResultVersion` and `DocumentVersion`. Untagged
diagnostics are invalid by design.

## Incrementality: body-input and manifest invalidation

The invalidation question is: *on an edit, which bodies must re-solve now, and which
consumers must re-solve after save/admission?* v2 splits those concerns.

### Keystroke: per-body input digests

Each body gets a `BodyInputDigest`:

    BodyInputDigest = hash(
        canonical source bytes for the body,
        referenced binder set,
        imported/exported dependency summary digests used by the body,
        local dependency summary digests for bodies it calls/reads,
        profile-independent checker options that affect surfaced families
    )

The **referenced binder set** is the resolved set of locals/upvalues/params/imported
symbols the body reads or writes, using binder IDs rather than textual names. This catches
edits like changing a local annotation, capture, import binding, or sibling function that
changes a body's inputs even when the enclosing file still parses.

The server maintains an intra-unit dependency graph:

    BinderID --> BodyID readers/writers
    BodyID   --> BodyID dependents through local summary use
    Import   --> BodyID users of imported summary digest

On `didChange`, the server reparses the edited file, recomputes body source spans and
referenced binder sets, then recomputes body input digests. Any body whose digest changes
is dirty; dirty bodies invalidate dependent bodies through the graph until a fixed point.
Only that dirty closure is scheduled for the next incremental solve. If parsing fails,
the graph from the last parseable document version remains active outside the broken
region, and the broken region gets parse diagnostics.

### Save/admission: manifest digests

The canonical manifest digest remains the module-boundary admission key:

1. On save or admission, re-derive the unit's exported manifest.
2. If the exported manifest digest is unchanged, no importing unit is invalidated.
3. If it changed, invalidate every unit whose import set includes it and propagate by
   digest closure.

Manifest digests are correct for save/admission propagation because they describe module
interfaces and exported summaries. They are too coarse for keystroke latency and are not
used to decide body-level scheduling inside an open document.

## What runs on keystroke vs save vs admission

| Trigger | Work | Latency target |
|---|---|---|
| **keystroke** (`didChange`) | update parse snapshot; recompute edited file body digests and dependency edges; cancel obsolete solve for older document version; schedule dirty body closure | interactive (< frame for scheduling) |
| **debounce / idle** (server-owned, ~150ms) | single-flight incremental solve for latest document version; publish only when a result completes; refresh diagnostics/tokens/hovers from tagged cache entry | sub-second |
| **pull diagnostics mid-solve** | return last completed diagnostics tagged with their `resultVersion` and `documentVersion`; ensure a solve is scheduled for the requested version | non-blocking |
| **save** (`didSave`) | re-derive exported manifest; if digest changed, invalidate + re-solve consumer closure | seconds acceptable |
| **admission** (the real gate) | full unit solve under the admission profile; authoritative verdict and export | batch |

The scheduler is server-owned: it debounces, cancels obsolete work by document version,
and enforces single-flight solves per unit. Multiple pulls while a solve is running all
observe the same last completed result and may attach to the pending result token; they
do not start parallel solves.

## Pull diagnostics contract

Pull diagnostics are versioned cache reads:

- If the latest requested document version has a completed solve, return those
  diagnostics with `resultId = ResultVersion` and `documentVersion = requested`.
- If a newer solve is pending, return the last completed diagnostics for that document
  with their original `resultId` and `documentVersion`, and schedule/cancel work so the
  latest version is being solved.
- If no solve has ever completed for the document, return parse diagnostics if available
  and an empty checker-diagnostic set tagged as `unsolved` for that document version.
- Never block a pull request on checker completion.
- Never serve diagnostics without the solved document version that produced them.

This makes staleness explicit. A client may display older diagnostics, but it can also
tell that `documentVersion < current buffer version` and render them as pending/refreshing
according to client policy.

## Projections

- **Diagnostics (pull).** Filter the tagged cache entry's judgments to
  `severity != none`, map each to an LSP `Diagnostic` (span, code, message), and attach
  the origin trace as `relatedInformation` (one entry per trace node, birth -> use).
  Conditional verdicts render as `Hint` severity with the seal/guard site as related
  info.
- **Hover.** Query the solved readmodel for the latest completed result whose
  `DocumentVersion` is at or before the buffer version. The response includes the result
  tag; on a subject with a judgment, append the verdict + origin trace.
- **Code actions.** Read JIR `Repair[]` for judgments overlapping the requested range;
  offer each as a `CodeAction` with its `edit` and, in the title, the verified verdict
  ("Guard x (verified: proven)"). Only `verified_verdict == proven` repairs surface;
  never widening/as-any (task `13897ee5` menu: per-variant constructor / validated wrap /
  runtime-validated claim / guard / seal).
- **Semantic tokens.** Derived from solved state, not the lexer: a token's modifier set
  can encode solved facts such as sealed/frozen, placement class, or manifest import.
  Custom token modifiers ship only when the client advertises support for them during
  capability negotiation. Without that capability, placement/seal facts remain available
  through hover and diagnostics, not semantic tokens.

## Memoization strategy

- **Unit cache key = unit digest** for completed admission/save results. Entry =
  `{readmodel, judgments, JIR, exported-manifest-digest, result tag}`. Pure function of
  the key => safe to cache indefinitely; evict by LRU under memory budget.
- **Body sub-cache key = BodyInputDigest** for open-document incremental solves. A body
  result can be reused only when its body digest and all dependency summary digests match
  the requested document version's graph.
- **Invalidation graph cache** is versioned by document version. Graph nodes use binder
  IDs and body IDs, not `cfg.Point`, so edits that renumber CFG points do not create
  artificial churn.
- **Projection memo** is unnecessary. Projections are cheap span queries over tagged
  solved results; recompute per request to avoid stale views.
- **Origin materialization is lazy** (evidence-origins design): the cache stores terminal
  origin ids and summary origin slots. Hover/code-action requests walk the cached arena
  on demand, while proven-judgment traces remain elided unless requested.

## Profile rule

The editor must not claim verdict identity with admission unless it ran the same profile.
The LSP has two legal modes:

1. **Same-profile mode.** Run the admission profile for all surfaced families. This is
   authoritative but may be slower.
2. **Profile-independent mode.** Run a lower-latency profile but surface only judgment
   families proven profile-independent. Families affected by omitted analyses are hidden
   or marked unavailable until a same-profile solve completes.

Any diagnostic, hover fact, semantic token modifier, or code action that depends on a
profile-skewed family is suppressed rather than displayed as if it were admission
equivalent.

## Changelog v2 - review counters resolved

- **Manifest digest was wrong for keystroke scheduling.** Resolved by adding
  `BodyInputDigest` and a versioned intra-unit dependency graph.
- **Body-only correctness was underspecified.** Resolved by including body source bytes,
  referenced binder set, dependency summary digests, and relevant profile options in the
  digest.
- **Pull diagnostics mid-solve were ambiguous.** Resolved with last-completed tagged
  results, non-blocking pulls, and explicit scheduling/cancellation for the requested
  document version.
- **Debounce authority was unclear.** Resolved by making debounce, cancellation, and
  single-flight solve ownership server responsibilities.
- **Profile skew invalidated verdict identity.** Resolved with same-profile mode or
  profile-independent surfaced families only.
- **Semantic-token custom modifiers needed negotiation.** Resolved by gating custom
  modifiers behind client capability negotiation and keeping hover as fallback.

## Open questions for adversarial review

1. **Dependency graph precision.** Which binder and local-summary edges are required for
   the first useful implementation, and which can be conservatively approximated by
   invalidating more bodies?
2. **Stale-result body mapping.** Which previous `BodyID` anchors remain useful
   for display while an edited document has syntax errors around a body boundary,
   before the next completed solve is published?
3. **Result tag shape.** Should `ResultVersion` be a server-local ordinal, a digest of
   the solved projection, or both for better LSP/client cache behavior?
4. **Profile-independent family list.** The design requires an audited list before a
   lower-profile LSP can surface those families. Which families are safe in v1?
