# JIR — Judgment IR JSON Export Design

Status: design contract for adversarial review. Not yet wired.

## Problem

The solved checker produces judgments (verdicts + evidence + origins over subjects and
spans). Two external consumers need a stable serialized form:

1. **GSPO/DPO harness reward.** A training loop needs a *dense, per-location* signal:
   every subject the checker reasoned about, its verdict, and a machine rationale, so a
   generated program yields a reward vector (and verified repairs yield chosen/rejected
   pairs). Density and stability matter more than human legibility.
2. **LSP.** An editor needs diagnostics, hovers, code actions, and semantic tokens
   derived from the same judgments, addressed by span, delivered incrementally.

Both must read *one* export. This document specifies JIR: the stable JSON projection of
judgments. JIR is a **projection of solved state**, never a parallel truth — it is
generated from the same judgment records the diagnostics renderer consumes.

## Document shape

One JIR document per checked unit (module or admission unit):

    {
      "jir_version": "1.0",
      "unit": { "module": "webhooks/repo", "digest": "sha256:...", "entry": "main.lua" },
      "sources": [ { "file": "main.lua", "digest": "sha256:...", "lines": 42 } ],
      "subjects": [ Subject ],
      "judgments": [ Judgment ],
      "origins":  [ OriginNode ],     // interned, referenced by id
      "repairs":  [ Repair ]          // verified repair candidates, optional
    }

### Subject

    Subject {
      "id": "s12",                    // stable within a unit given a fixed digest
      "ref": "local:acc" | "field:Cfg.hook" | "param:list#1" | "return:list#0",
      "span": Span,
      "type": "string?" | "…",        // read-model type string, canonical form
      "placement": "ActorLocal" | …   // when the placement family solved this subject
    }

### Judgment

    Judgment {
      "code": "type.nil.unsafe_use" | "send.isolation" | "placement.class" | …,
      "family": "type" | "send" | "placement" | "advice",
      "subject": "s12",
      "span": Span,                   // primary location (the use / boundary / alloc site)
      "verdict": "proven" | "refuted" | "unknown"
               | { "conditional": "sealed@P" | "claim" | "guard" | "variant",
                   "at": Span },      // Proven-if-{…}
      "severity": "error" | "warning" | "hint" | "none",
      "evidence": [ Evidence ],       // terminal evidence entries (kind/trust/span/message)
      "trace": ["o44","o31","o12"],   // origin ids, use → birth order (optional)
      "message": "x may be nil at this call"
    }

    Span     { "file": "main.lua", "line": 10, "col": 12, "end_line": 10, "end_col": 20 }
    Evidence { "kind": "abstract fact" | "user assertion", "trust": "proven" | "claimed",
               "span": Span, "message": "…" }
    OriginNode { "id": "o12", "kind": "birth"|"join"|"narrow"|"widen"|"call"|"return"|"seal",
                 "subject": "s12", "site": Span, "fact": "nilable=true" | "range=[1,10]" | …,
                 "preds": ["o09"], "cause": "else arm has no assignment" }

### Repair (verified candidates, task 13897ee5)

    Repair {
      "judgment": <index>,            // the judgment it flips
      "kind": "guard" | "claim" | "seal" | "variant" | "construct",
      "edit": { "span": Span, "replacement": "if x ~= nil then …" },
      "verified_verdict": "proven",   // re-checked against solved state; only Proven attaches
      "rationale": "guarding x severs the nil-birth path at main.lua:6"
    }

Repairs are the DPO gold: `verified_verdict == proven` is the *chosen* branch; the
original refuted judgment is the *rejected* branch; `rationale` is the machine
preference reason.

## Reward density for the harness

The harness reward reads `judgments[]` as a per-location vector: each `(subject, code)`
is a location; `verdict` maps to a scalar (proven=+1, refuted=−1, unknown=0, conditional
= partial with the repair delta). Density requirements:

- **Every reasoned subject appears**, including proven ones — the harness needs the
  positives, not only failures. Export is not filtered by severity (unlike the LSP view,
  which filters to `severity != none`).
- **Origins present** so the reward can localize *why* (the birth span), enabling
  credit assignment to the offending line, not just the file.
- **Repairs present** so each refutation ships its verified counterfactual — a chosen
  edit with a proof — which is exactly a DPO pair with a machine rationale.

## Versioning and stability guarantees

- **`jir_version` = MAJOR.MINOR.** MINOR is additive-only (new optional fields, new
  `code`s, new origin `kind`s); a consumer pinned to `1.x` must ignore unknown fields
  and unknown enum members (treat unknown verdict/kind as `unknown`). MAJOR bumps on any
  removal or semantic change to an existing field.
- **Subject id stability.** `Subject.id` is stable across re-exports *of the same source
  digest*. It is derived from the canonical subject ref + a deterministic ordinal, not
  from allocation order, so an unrelated edit elsewhere in the unit does not renumber a
  subject. Cross-edit stability is best-effort (digest changes ⇒ ids may move); consumers
  that need cross-edit identity key on `ref`+`span`, not `id`.
- **Ordering.** `subjects`, `judgments`, `origins` are emitted in deterministic order
  (by span, then ref) so two runs on identical input are byte-identical — a hard test
  gate, mirroring the manifest wire-codec determinism oracle (journal #1376).
- **Code stability.** Judgment `code`s are the same identifiers the diagnostics renderer
  uses; they never silently change meaning. A renamed code is a MAJOR bump with an alias
  window.

## Size budget

- **Baseline:** one judgment ≈ 6 short fields + evidence (2–3 entries) ≈ 300–600 bytes
  JSON. A 500-line module reasoning about ~200 subjects ≈ 60–120 KB uncompressed,
  ~10–20 KB gzipped. Acceptable for admission-time export and LSP pull.
- **Origins are the swing factor.** Full origin arenas would multiply size. Budget rule:
  export origins **only for fired judgments** (refuted / conditional / diagnosed
  unknown) by default; proven-judgment traces are elided unless the consumer requests
  `?origins=all` (harness credit-assignment mode). Bounded fan-in `k` (evidence-origins
  design) keeps each trace ≤ ~2k nodes.
- **Sources carry digests, not text.** JIR references source files by digest+span; it
  never inlines source lines. The consumer already has the sources (editor buffer /
  harness sample).
- **Streaming tier.** For batch harness runs, a newline-delimited `judgments` stream
  (one JSON object per line, no enclosing array) allows constant-memory consumption;
  `unit`/`subjects`/`origins` ship as a header record. The LSP tier uses the framed
  document form.

## Non-goals

- JIR is not a serialization of the lattice/solved state — it is the *judgment*
  projection. The abstract-interpreter cells stay internal.
- JIR does not carry AST. Spans + digests are the only source linkage (judgment_ir
  no-AST-past-the-fact-boundary principle extends to export).
- No embedded rendering. Human strings (`message`, `rationale`) are present but the
  terminal ANSI/`render_*` formatting is the diagnostics renderer's job, not JIR's.

## Open questions for adversarial review

1. **Subject id derivation.** Canonical-ref + ordinal is stable within a digest but two
   distinct locals named `x` in sibling scopes need disambiguation — is scope-path in the
   `ref` enough, or does the harness need a scope id field?
2. **Conditional verdict scalar.** How should the reward map `conditional:sealed@P` — as
   a fractional positive (near-miss), and does the repair delta double-count?
3. **Cross-module units.** Is one JIR per module right, or should an admission unit emit
   a single JIR spanning the entry + its consumed manifests (with seam origins)? The LSP
   wants per-file; the harness may want per-program.
4. **Digest granularity for stability.** Per-file digest vs per-unit digest changes how
   often subject ids move under edits; which minimizes LSP churn without weakening the
   determinism gate?
