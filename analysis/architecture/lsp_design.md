# LSP Server Design

Status: design contract for adversarial review. Not yet wired.

## Principle

The language server is a **thin projection of solved judgments**, not a second analyzer.
Every LSP response — diagnostics, hover, code actions, semantic tokens — is a query over
the same solved state and JIR that the diagnostics renderer and the harness consume.
There is one checker (judgment_ir principle); the LSP is a transport and an
incremental-invalidation scheduler around it.

This is the SOTA lever: rust-analyzer and gopls re-implement large parts of their
compiler's analysis inside the server (their own type inference, name resolution, and
flow views) because the batch compiler was never designed to answer editor queries
incrementally. Here the *checker already produces judgments as data* (verdicts, evidence,
origins, verified repairs). The server does not re-derive; it schedules solves and reads
results.

## What SOTA means here (and what we skip)

**We match or exceed:**
- **Judgment-native diagnostics.** Diagnostics are checker verdicts, not a separately
  maintained lint pass. A diagnostic *is* a Refuted/conditional judgment with its origin
  trace as related-information — richer than a gopls type error, which carries no
  provenance chain.
- **Verified code actions.** Quick-fixes are *counterfactually verified* repairs (JIR
  `Repair`, task `13897ee5`): the server offers only edits the checker re-proved flip the
  verdict to Proven. rust-analyzer's assists are syntactic and unverified; ours carry a
  proof. No "apply and hope."
- **Pull-model diagnostics** (LSP 3.17 `textDocument/diagnostic` + workspace pull) so the
  client drives freshness and the server never pushes stale results across an edit.
- **Provenance hover.** Hover surfaces the solved type *and*, on a diagnosed subject, the
  origin trace ("nil born at L6, survives join L9").

**We deliberately skip:**
- **Whole-workspace live re-index on keystroke.** We invalidate by manifest digest, not
  by re-solving the world (see incrementality). No global symbol index maintained
  eagerly.
- **Macro/build-system integration, formatting, rename-across-crates.** Out of scope for
  v1; rename is a later item and rides subject refs, not a bespoke index.
- **Speculative type-inference-on-partial-parse.** On an unparseable buffer we serve the
  last good solve for unaffected bodies and a parse diagnostic for the broken region; we
  do not invent a recovery type system.

## Architecture

    client ── LSP ──▶ server ──▶ SolveCache ──▶ checker (admission solve)
                         │            │
                         │            └── digest-keyed unit results (judgments, JIR, readmodel)
                         └── projections: diagnostics | hover | code actions | semantic tokens

The server owns a `SolveCache` keyed by **unit digest**. A unit is a module plus the
manifest digests of everything it imports. A cache entry holds the unit's solved
readmodel + judgments + JIR. Projections are pure functions of a cache entry + a span.

## Incrementality: manifest-digest invalidation

The invalidation question is: *on an edit, which bodies must re-solve?*

1. **Edit lands in file F of unit U.** Recompute F's source digest → U's unit digest.
2. **Re-solve U** (only U's bodies). U's *exported manifest* is re-derived.
3. **Compare U's new manifest digest to the cached one:**
   - **Unchanged** (edit was body-internal, no signature/effect/placement-summary change):
     no consumer re-solves. This is the common case and the reason manifest digests, not
     file digests, gate propagation — an editor keystroke inside a function body almost
     never changes the exported summary.
   - **Changed:** invalidate every unit whose import set includes U; re-solve them
     (transitively, by digest closure). Per-consumer placement plans recompute at each
     consumer's admission from U's summary (journal #1412: library artifact untouched,
     consumer plans re-derived).
4. **Body-level granularity within U.** Within a re-solved unit, only bodies whose
   fact-inputs changed re-run; the solver already memoizes per-body on input facts. A
   one-line edit re-solves its enclosing body and any body dominated by a changed local
   summary, not the whole file.

This gives near-constant editor latency for the dominant case (body-internal edits) and
correct, bounded propagation for signature-affecting edits — the incremental contract the
manifest system was built for (ShapeID/digest pinning), reused rather than re-invented.

## What runs on keystroke vs save vs admission

| Trigger | Work | Latency target |
|---|---|---|
| **keystroke** (didChange) | reparse edited file; if parse ok, re-solve *edited body only*; publish parse diagnostics immediately, defer full diagnostics to the debounce | interactive (< frame) |
| **debounce / idle** (~150ms) | re-solve unit U; refresh U's diagnostics, semantic tokens, hovers from the new cache entry; recompute code actions lazily on request | sub-second |
| **save** (didSave) | re-derive U's exported manifest; if digest changed, invalidate + re-solve the consumer closure; refresh cross-file diagnostics | seconds acceptable |
| **admission** (the real gate) | full unit solve under the admission profile (`CompileProfileTyped`); this is the authoritative verdict the editor previews. The LSP solve is the *same* solve at a lower profile, so editor and admission never disagree | batch |

The invariant: the editor never shows a verdict admission would not. The LSP runs the
admission checker, just scheduled incrementally and projected per span.

## Projections

- **Diagnostics (pull).** Filter the cache entry's judgments to `severity != none`, map
  each to an LSP `Diagnostic` (span, code, message), and attach the origin trace as
  `relatedInformation` (one entry per trace node, birth → use). Conditional verdicts
  render as `Hint` severity with the seal/guard site as related info.
- **Hover.** `readmodel` query at the cursor span → the solved type string; on a subject
  that carries a judgment, append the verdict + origin trace. No inference on hover — a
  map lookup into solved state.
- **Code actions.** Read JIR `Repair[]` for judgments overlapping the requested range;
  offer each as a `CodeAction` with its `edit` and, in the title, the verified verdict
  ("Guard x (verified: proven)"). Only `verified_verdict == proven` repairs surface;
  never widening/as-any (task `13897ee5` menu: per-variant constructor / validated wrap /
  runtime-validated claim / guard / seal).
- **Semantic tokens.** Derived from solved state, not the lexer: a token's modifier set
  encodes solved facts — e.g. `readonly` for frozen/sealed places, a custom modifier for
  `deferred`/`shared` placement, `defaultLibrary` for manifest imports. This makes
  placement class and sealing *visible in the buffer*, which no mainstream server does
  because they lack a placement lattice to project.

## Memoization strategy

- **Cache key = unit digest** (source digests + imported manifest digests). Entry =
  {readmodel, judgments, JIR, exported-manifest-digest}. Pure function of the key ⇒ safe
  to cache indefinitely; evict by LRU under memory budget.
- **Body-level sub-memo** inside the checker (already present): per-body solve keyed on
  input facts; unchanged bodies reuse prior cells across a unit re-solve.
- **Projection memo** is unnecessary — projections are cheap span queries; recompute per
  request to avoid stale views. The expensive thing (the solve) is what we cache.
- **Origin materialization is lazy** (evidence-origins design): the cache stores terminal
  origin ids; hover/code-action requests walk the still-cached arena on demand, so the
  steady-state cache carries no per-proven-judgment trace cost.

## Open questions for adversarial review

1. **Body-only keystroke solve correctness.** Re-solving only the edited body assumes its
   input facts are unchanged by the in-progress edit. Is there an edit class (e.g.
   changing a local's annotation) that silently alters sibling bodies' inputs before
   save-time manifest comparison catches it? Do we need a body-input digest, not just a
   manifest digest, for intra-unit propagation?
2. **Pull vs push under rapid edits.** Pull-model diagnostics let the client coalesce,
   but a client that pulls aggressively mid-debounce may thrash the solver. Server-side
   debounce vs client-side coalescing — where does the authority sit?
3. **Semantic-token placement modifiers** are non-standard; do we ship custom modifiers
   (requires client capability negotiation) or fold placement into hover only for v1?
4. **Cross-file trace rendering.** A diagnostic whose origin crosses a module boundary
   needs the library's seam origin; the consumer cache entry has the seam but not the
   library's internal arena. Is the seam node sufficient for `relatedInformation`, or does
   hover-into-library require solving the library on demand?
5. **Admission/LSP profile skew.** The LSP solves at a lower profile for latency; we claim
   verdict identity with the admission profile. Is that guaranteed for *all* families, or
   only the ones whose verdicts are profile-independent (and which families are not)?
