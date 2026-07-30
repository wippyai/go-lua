# Judgment IR Migration Design

Status: Phase 0 design contract.

## Problem

`analysis/check/diagnostics` has grown into a shadow checker. It re-derives
expression types, guard flow, call contracts, argument assignability, and
producer suppression after the abstract interpreter has already solved the
program. That creates drift: the engine and diagnostics answer the same
semantic question through different representations.

The migration goal is not to rename helpers. It is to make diagnostics a
renderer for one semantic authority.

## Principle

There is one checker.

The solved abstract interpreter plus `readmodel.Reader` owns semantic truth.
Diagnostics render structured judgments; they do not rediscover types,
contracts, reachability, or guard facts from raw AST.

## Load-Bearing Abstractions

These decisions are part of Phase 0 because they are cheap to enforce before
migration and expensive to retrofit after producers move.

### No AST Past the Fact Boundary

Judgment production and rendering consume solved state, lowered facts, canonical
contracts, and read-model values. They do not import `compiler/ast`, `lua/bind`,
or engine state internals.

If a syntactic property is semantically relevant, lowering must preserve it as a
fact or origin reference. A judgment producer must not recover it by walking raw
AST.

### Canonical Contract

Callable contracts must have one canonical representation before direct-call
migration proceeds. That representation owns:

- parameters
- returns
- varargs
- receiver binding
- generic parameters and instantiation evidence
- declared obligations and effects

The obligation pass consumes canonical contracts. It must not re-lower function
types or annotations.

### Stable SubjectRef

Every judgment has a stable subject identity. It is the key for:

- deduplication
- precedence
- shadow-diff matching
- future incremental caching

Subject identity is not a rendered string. It should be derived from the
function/body identity, CFG point, code family, and canonical path/expression or
argument key.

### Evidence Chain With Origins

Evidence is structured and origin-carrying from the first migrated producer.
Each evidence node records what kind of proof or missing proof it represents and
where that fact came from.

The evidence model must define branch-join behavior before producer migration:

- proofs that hold on all paths may stay as proven evidence
- one-sided proof or one-sided taint must remain visible as unknown or
  precision-boundary evidence when it affects an obligation
- renderers may choose wording, but may not invent missing semantic origins

This is the mechanism that lets `guard_env` disappear instead of being rebuilt.

### Central Policy Table

Judgments carry verdicts, not severity. One policy table maps:

```text
(code, verdict, strictness context) -> severity/enabled
```

This is the strict-any and warnings dial. Producers must not bake
"unknown is error here" decisions into semantic generation.

### Judgment Code Registry

Judgment codes are registered once. The registry owns:

- code
- subject kind
- required evidence shape
- renderer
- default policy row

This keeps baselines, suppressions, and editor integrations stable.

### One Value-To-Type Projection

Judgments carry solved value references. Renderers display values through the
canonical read-model projection only. No renderer or producer may implement its
own `product.Value -> typ.Type` projection.

### Fact-Lane Freeze During Phase 2

Until the descriptor registry lands, adding a persistent fact lane requires a
stub descriptor entry. This keeps Phase 3 from growing while producer migration
is deleting old semantic paths.

## Judgment Shape

Judgments are produced by a post-solve obligation pass. They are not transfer
facts and do not affect fixpoint convergence.

```go
type Judgment struct {
	Code     Code
	Point    cfg.Point
	Subject  SubjectRef
	Expected TypeRef
	Actual   ValueRef
	Verdict  Verdict
	Evidence Chain
	Spans    []SpanRef
}
```

The concrete Go shape can differ, but these invariants are fixed:

- No user-facing message strings in the engine-side judgment.
- No severity in the engine-side judgment.
- `Actual` is a product value or stable reference to one read through the
  canonical read model at the relevant boundary.
- `Expected` is a resolved type reference, not an AST type expression.
- Evidence carries structured reasons and source origins needed for rendering.
- Subject identity is stable enough for deduplication and precedence.

## Placement

Judgment production is post-solve:

1. Build the body and solve normally.
2. Run obligation checkers over the solved body through `readmodel.Reader`.
3. Emit judgments with evidence and spans.
4. Render, deduplicate, order, and apply policy outside the engine.

Transfer remains pure. The judgment pass may read solved facts, but must not
write state, enqueue contexts, or change summaries.

## Migration Rule

Every producer migration follows the same deletion loop:

1. Characterize the current producer output with the fixture baseline.
2. Lift only semantic obligations into the judgment pass.
3. Render the new judgments with existing user-facing wording where possible.
4. Shadow-run old producer and new renderer.
5. Classify every delta:
   - byte/record-identical: accepted automatically
   - engine precision win: accepted only with a fixture and journal note
   - engine bug: fix in engine/readmodel with a fixture
   - old diagnostic bug: accepted only with a journal note
6. Delete the old semantic producer code and its suppression edges.

Fallback inference chains must not be ported as-is. Each fallback is classified:

- already represented by solved state/readmodel: delete
- missing engine fact: add the canonical engine fact with a focused test
- rendering-only context: keep in renderer

## First Slice

Migrate direct-call argument diagnostics first.

This slice is the proof of the design. It must remove the need for most of:

- direct-call argument fallback typing
- direct-call-local contract re-lowering
- cross-producer `wouldReport` suppression around direct calls
- guard-env-only narrowing used to justify call arguments

If this slice cannot delete code, the judgment design is wrong.

Before this slice starts, the canonical contract and evidence-origin decisions
must be explicit enough that direct-call migration does not recreate
`lowerDirectFunctionContract`, `expression_type`, or `guard_env` under new names.

## Oracle

`FIXTURE_BASELINE_OUT=<path> go test -run TestWriteFixtureDiagnosticBaseline`
writes the fixture diagnostic baseline as JSONL. The migration oracle is record
stability over:

- suite status
- diagnostic code
- severity
- primary span
- message
- help
- evidence chain
- labels
- curated missing/unexpected verdicts

Rendered byte snapshots may be added for specific diagnostics, but normalized
records are the primary semantic drift oracle.

## Non-Goals

- Do not make judgment production a second type checker.
- Do not make diagnostics query raw AST to recover semantic facts.
- Do not add compensation paths at diagnostic sites.
- Do not change runtime semantics or abstract-state transfer during the
  migration unless a classified engine bug requires it.
- Do not start with the fact-kind registry. Producer drift is the immediate
  source of false positives and false negatives.
