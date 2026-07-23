# JIR - Judgment IR JSON Export Design

Status: v2 design contract after adversarial review. Not yet wired.

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
judgments. JIR is a **projection of solved state**, never a parallel truth - it is
generated from the same judgment records the diagnostics renderer consumes.

## Document shape

One JIR document per checked unit (module or admission unit):

    {
      "jir_version": "1.1",
      "unit": {
        "module": "webhooks/repo",
        "digest": "sha256:...",           // deterministic unit digest
        "entry": "main.lua",
        "profile": "CompileProfileTyped"
      },
      "capabilities": {
        "codes": "judgment-codes@1",
        "origins": ["birth","join","widen","narrow","call","return","seal"],
        "spans": ["line_col","byte_offset_len"]
      },
      "sources": [ Source ],
      "bodies":  [ Body ],
      "subjects": [ Subject ],
      "judgments": [ Judgment ],
      "origins":  [ OriginNode ],     // interned, referenced by id
      "repairs":  [ Repair ]          // verified repair candidates, optional
    }

    Source { "file": "main.lua", "digest": "sha256:...", "lines": 42,
             "bytes": 1024 }
    Body   { "id": "b7", "file": "main.lua", "digest": "sha256:...",
             "span": Span, "cfg_point_base": 120 }

`unit.digest` is the determinism/admission key. `sources[].digest` and `bodies[].digest`
are churn keys for LSP and longitudinal consumers that want to distinguish a local body
edit from a whole-unit identity change.

### Subject

    Subject {
      "id": "s12",                    // stable within a unit digest
      "identity": {
        "scope_id": "scope:main.lua:fn@binder42",
        "symbol_id": "sym:acc@binder57",      // absent for anonymous temporaries
        "anchor": { "file": "main.lua", "offset": 188, "len": 3,
                    "role": "binding" | "use" | "synthetic" },
        "ref": "local:acc" | "field:Cfg.hook" | "param:list#1" | "return:list#0"
      },
      "span": Span,
      "type": "string?" | "...",      // read-model type string, canonical form
      "placement": "ActorLocal" | ... // when the placement family solved this subject
    }

The identity layer is independent of `cfg.Point`. `scope_id` and `symbol_id` are backed
by binder IDs from name resolution; `anchor` is the byte-stable source anchor used when a
symbol id is unavailable or a consumer needs edit-to-edit matching. `Subject.id` remains
the compact per-document handle, but longitudinal consumers should key on
`identity.scope_id + identity.symbol_id + identity.anchor + judgment.code`, not on raw
`id`.

Per-attempt training is unaffected by identity churn because each reward vector is read
inside one JIR document. Longitudinal reward analysis, LSP churn reduction, and edit-to-
edit dashboards require the identity fields above.

### Judgment

    Judgment {
      "code": "type.nil.unsafe_use" | "send.isolation" | "placement.class" | ...,
      "code_version": "judgment-codes@1",
      "family": "type" | "send" | "placement" | "advice",
      "subject": "s12",
      "span": Span,                   // primary location (the use / boundary / alloc site)
      "verdict": "proven" | "refuted" | "unknown"
               | { "conditional": "sealed@P" | "claim" | "guard" | "variant",
                   "at": Span },      // Proven-if-{...}
      "severity": "error" | "warning" | "hint" | "none",
      "evidence": [ Evidence ],       // terminal evidence entries (kind/trust/span/message)
      "trace": ["o44","o31","o12"],   // origin ids, use -> birth order (optional)
      "message": "x may be nil at this call"
    }

    Span {
      "file": "main.lua",
      "line": 10, "col": 12, "end_line": 10, "end_col": 20,
      "offset": 188, "len": 8
    }
    Evidence { "kind": "abstract fact" | "user assertion", "trust": "proven" | "claimed",
               "span": Span, "message": "..." }
    OriginNode { "id": "o12", "kind": "birth"|"join"|"narrow"|"widen"|"call"|"return"|"seal",
                 "subject": "s12", "site": Span, "fact": "nilable=true" | "range=[1,10]" | ...,
                 "preds": ["o09"],
                 "cause": { "code": "MissingElseAssign", "params": { "branch": "else" } } }

`offset` and `len` are byte offsets into the exact source bytes identified by
`sources[].digest`. They are required for tokenizer alignment in training. Line/column
fields remain for diagnostics and editors; byte spans are the canonical machine span.

### Repair (verified candidates, task 13897ee5)

    Repair {
      "judgment": <index>,            // the judgment it flips
      "kind": "guard" | "claim" | "seal" | "variant" | "construct",
      "edit": { "span": Span, "replacement": "if x ~= nil then ..." },
      "verified_verdict": "proven",   // re-checked against solved state; only Proven attaches
      "rationale": "guarding x severs the nil-birth path at main.lua:6"
    }

Repairs are the DPO gold: `verified_verdict == proven` is the *chosen* branch; the
original refuted judgment is the *rejected* branch; `rationale` is the machine
preference reason.

## Reward density for the harness

The harness reward reads `judgments[]` as a per-location vector: each `(subject
identity, code)` is a location; `verdict` maps to a scalar according to the active code
registry and reward policy. Density requirements:

- **Every reasoned subject appears**, including proven ones - the harness needs the
  positives, not only failures. Export is not filtered by severity (unlike the LSP view,
  which filters to `severity != none`).
- **Origins present** so the reward can localize *why* (the birth byte span), enabling
  credit assignment to the offending token/line, not just the file.
- **Repairs present** so each refutation ships its verified counterfactual - a chosen
  edit with a proof - which is exactly a DPO pair with a machine rationale.
- **Conditional verdicts stay raw.** JIR exports the conditional and repair delta. The
  harness decides whether a conditional maps to a fractional scalar; JIR does not bake in
  reward semantics.

New judgment codes do not automatically enter old dense reward vectors. If a JIR
contains a code introduced after the consumer's reward version, that code is ignored by
that reward version unless the registry explicitly aliases it to a known scalar slot.

## Versioning and stability guarantees

- **`jir_version` = MAJOR.MINOR.** MINOR is additive-only (new optional fields, new
  origin `kind`s, new code registry references). MAJOR bumps on any removal or semantic
  change to an existing field.
- **Code registry is explicit.** Judgment codes are defined by a named registry version,
  e.g. `judgment-codes@1`. Each registry entry has:

        CodeRegistryEntry {
            code: "type.nil.unsafe_use",
            schema_version: 1,
            family: "type",
            aliases: [],
            reward_policy: {
                reward_version: "reward@1",
                scalar_slot: "type.nil.unsafe_use",
                proven: +1, refuted: -1, unknown: 0,
                conditional: "raw_plus_repair_delta"
            }
        }

  A new code added in `judgment-codes@2` must state whether it is included in
  `reward@2`, aliases an older slot, or is **ignored by reward vN** for every older
  reward policy. Unknown code insertion must never silently change dense reward
  dimensionality.
- **Consumer capability negotiation.** Consumers declare supported `jir_version`,
  `code_version`, origin kinds, span forms, and optional sections (`repairs`,
  `origins=all`). Producers either emit a compatible projection or fail closed with a
  capability error. LSP can request only diagnostic codes/origin kinds it knows how to
  display; harness runs pin a reward version.
- **Subject id stability.** `Subject.id` is stable across re-exports *of the same unit
  digest*. It is derived from `identity` plus a deterministic ordinal, not allocation
  order. Cross-edit stability is provided by `scope_id`, `symbol_id`, and `anchor`;
  `Subject.id` is not the longitudinal key.
- **Digest granularity.** `unit.digest` gates deterministic equality of the full export.
  `sources[].digest` and `bodies[].digest` are churn keys used by LSP and longitudinal
  consumers to reuse identities and diagnostics across local edits.
- **Ordering.** `subjects`, `judgments`, `origins`, and `repairs` are emitted in
  deterministic order (byte span, identity/ref, code, then ordinal) so two runs on
  identical input are byte-identical - a hard test gate, mirroring the manifest
  wire-codec determinism oracle (journal #1376).
- **`cfg.Point` ordinal determinism.** If a subject or judgment must include a CFG point
  for debugging, it is emitted only as `debug.cfg_point` and never participates in stable
  identity. Formal guarantee: for a fixed unit digest, parser, binder, CFG builder,
  profile, and checker version, CFG point ordinals are deterministic and may be used as a
  tie-breaker after stable identity fields; across different source digests they carry no
  identity promise.
- **Code stability.** Judgment `code`s never silently change meaning. A renamed code
  requires a registry alias window; a semantic change requires a new code or major schema
  version.

## Size budget

- **Baseline:** one judgment ~= 6 short fields + evidence (2-3 entries) ~= 300-600 bytes
  JSON. A 500-line module reasoning about ~200 subjects ~= 60-120 KB uncompressed,
  ~10-20 KB gzipped. Acceptable for admission-time export and LSP pull.
- **Identity fields add small constant cost.** `scope_id`, `symbol_id`, and byte anchors
  are repeated but gzip well. They are required for longitudinal correctness; compact
  `Subject.id` still serves all intra-document references.
- **Origins are the swing factor.** Full origin arenas would multiply size. Budget rule:
  export origins **only for fired judgments** (refuted / conditional / diagnosed
  unknown) by default; proven-judgment traces are elided unless the consumer requests
  `origins=all` (harness credit-assignment mode). Bounded fan-in `k` (evidence-origins
  design) keeps each trace bounded.
- **Sources carry digests, not text.** JIR references source files by digest+span; it
  never inlines source lines. The consumer already has the sources (editor buffer /
  harness sample).
- **Streaming tier.** For batch harness runs, a newline-delimited `judgments` stream
  (one JSON object per line, no enclosing array) allows constant-memory consumption;
  `unit`/`sources`/`bodies`/`subjects`/`origins` ship as header records. The LSP tier
  uses the framed document form.

## Non-goals

- JIR is not a serialization of the lattice/solved state - it is the *judgment*
  projection. The abstract-interpreter cells stay internal.
- JIR does not carry AST. Spans + digests + binder-backed identities are the only source
  linkage (judgment_ir no-AST-past-the-fact-boundary principle extends to export).
- No embedded rendering. Human strings (`message`, `rationale`) are present but the
  terminal ANSI/`render_*` formatting is the diagnostics renderer's job, not JIR's.

## Changelog v2 - review counters resolved

- **Subject identity was tied to `cfg.Point` churn.** Resolved with a stable identity
  layer: `scope_id`, `symbol_id`, and byte `anchor` backed by binder IDs.
- **Per-attempt vs longitudinal identity was conflated.** Resolved by documenting that
  per-attempt training can use document-local ids, while longitudinal consumers must use
  identity fields.
- **Unknown code insertion could corrupt reward vectors.** Resolved with an explicit
  code registry, schema versions, per-code reward policy, aliases, and "ignored by reward
  vN" semantics.
- **Consumers needed feature negotiation.** Resolved with producer/consumer capabilities
  for JIR version, code registry, origin kinds, spans, repairs, and origin detail.
- **Digest granularity was too coarse for churn.** Resolved with per-file and per-body
  digests in addition to the deterministic unit digest.
- **Tokenizer alignment needed byte spans.** Resolved by making `offset` + `len`
  canonical machine span fields alongside line/column.
- **`cfg.Point` determinism was informal.** Resolved with a formal fixed-input ordinal
  guarantee and exclusion from stable identity.

## Open questions for adversarial review

1. **Binder ID persistence.** Which binder-id scheme gives the best cross-edit stability
   without storing ASTs in JIR?
2. **Registry distribution.** Should the code registry be embedded in each export,
   referenced by digest, or shipped as a separate versioned artifact?
3. **Capability failure mode.** Should incompatible consumers receive a reduced JIR
   projection, a machine-readable error document, or both?
4. **Cross-module bundles.** Per-module JIR plus an admission bundle/index is preferred
   over one giant canonical document, but the exact bundle index shape remains open.
