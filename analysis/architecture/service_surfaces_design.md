# Checker Service Surfaces Design

Status: lane 31 design contract. No adapters are wired yet.

## Grounding

The checker already has the semantic pieces needed for a shared service, but
they are assembled in test and diagnostics entry points instead of one
production session.

- `analysis/check/checktest` composes parse, manifests, `program.RunChunk`,
  diagnostics, export manifests, and placement plans for fixtures. Its package
  comment explicitly says it is a harness helper, not a semantic owner.
- `analysis/check/body` owns reusable per-body preparation (`Static`) and
  per-solve results (`Result`). `body.Result.ResultVersion()` is a stable
  content digest of body inputs: WIR, symbol types, globals, manifest sources,
  state lanes, entry state, initial states, and consumed summary input digests.
- `analysis/check/fixpoint/program` owns the batch fixed-point program solve.
  It builds function/context summary keys, runs `query.Run`, returns
  `program.Result`, exposes `Snapshot`, `RootResult`, `FunctionKey`,
  `TargetKey`, and `PathKey`, and materializes body results after convergence.
- `analysis/check/fixpoint/query` is a reusable pure fixed-point driver over
  `summary.SummaryKey -> summary.Summary`; `summary.Snapshot` is immutable and
  deterministic by exact key.
- `analysis/check/readmodel.Reader` is the public, syntax-free obligation query
  surface. `analysis/check/internal/readmodel.Reader` adapts `body.Result` into
  that interface. It is currently producer-shaped: many `ForEach...` methods,
  not a general point/path SDK.
- `analysis/check/obligation/pass` produces raw `judgment.Judgment` values from
  `readmodel.Reader`. It normalizes stable `SubjectAnchor`s and stamps
  `ResultVersion`.
- `analysis/check/diagnostics` still primarily exposes rendered
  `diagnostic.Diagnostic` values. Many families now flow through judgments, but
  callers usually get diagnostics after raw judgments are rendered and dropped.
- `analysis/check/judgment` owns stable semantic codes, code descriptors
  (`CodeSpec`), verdicts, subject anchors, structured evidence causes, policy,
  and schema tests.
- `analysis/diagnostic` owns human diagnostic rendering options and
  `diffreport`, whose JSONL comparison already matches by `SubjectAnchor` when
  present.
- `analysis/check/placementplan` projects solved body/program results into a
  compact `Plan` DTO; adapters should expose it, not recompute placement.
- `analysis/module/manifest` owns deterministic manifest encode/decode, while
  `analysis/check/exportmanifest` derives a manifest from a solved
  `program.Result`.
- `scripts/wippy_lint_harness.sh` shells out to Wippy `lint --json`, extracts
  summary/code/family/diagnostic JSONL rows, and optionally calls
  `scripts/wippy_diag_delta.sh`, which delegates to `analysis/diagnostic/diffreport`.

The named runtime admission profile `CompileProfileTyped` is not defined in
this repository. The checker service should therefore take profile identity and
profile-affecting options as inputs from the embedding Wippy runtime rather
than import runtime admission code.

## Missing Core

What does not exist yet is the service layer above these pieces:

1. A long-lived workspace session that owns source snapshots, parsed modules,
   module manifests, dependency edges, solve scheduling state, and completed
   result retention.
2. A production module/workspace loader. Today the fixture harness orders Lua
   files, checks dependency modules with `CheckFileAndExport`, and hands their
   manifests to the entry by name.
3. A versioned result store. The system can compute body `ResultVersion`
   digests, but it does not retain completed unit results as queryable
   `{tag, program.Result, judgments, diagnostics, manifest, placement, summaries}`.
4. An SDK query API over the solved graph. The public read model is enough for
   obligation producers, not for agents asking point/path/callgraph/evidence
   questions.
5. A concurrency contract. `typevalue.Cache`, body query caches, materialized
   body results, and summary snapshots need an explicit owner before a server
   exposes concurrent queries and solves.

## Principle

There is one checker engine.

The core owns loading, solving, retaining versioned results, producing raw
judgments, rendering diagnostics, exporting manifests, and answering solved
graph queries. Terminal, HTTP/LSP, in-runtime, and agent SDK forms are thin
adapters over that core.

Law: adapters contain ZERO checker logic. If an adapter needs a checker
decision, digest rule, filtering rule, query, render input, or result-retention
shape that another adapter could also need, it moves into core first.

## Core Shape

The core is a `WorkspaceSession` plus a query API.

```go
type WorkspaceSession interface {
    UpsertUnit(ctx context.Context, unit UnitInput) (UnitState, error)
    RemoveUnit(ctx context.Context, id UnitID) error
    EnsureSolved(ctx context.Context, req SolveRequest) (ResultTag, error)
    LastComplete(ctx context.Context, req ResultRequest) (CompletedResult, bool)
    Query(ctx context.Context, req QueryRequest) (QueryResult, error)
}
```

The exact Go package can change, but the ownership should not:

- `UnitInput` contains module path, entry file, source files, external manifests,
  stdlib flag, globals, state lanes, diagnostic/judgment policy, and profile id.
- `SolveRequest` selects a unit, profile, trigger (`keystroke`, `save`,
  `admission`, `batch`), and freshness requirements.
- `ResultTag` is the public version key:

```go
type ResultTag struct {
    UnitID          UnitID
    SolveSeq        uint64 // session-local monotonic completion id
    UnitDigest      Digest
    ManifestDigest  Digest
    SourceDigests   map[string]Digest
    BodyVersions    map[BodyID]uint64 // body.Result.ResultVersion()
    Profile         string
    DocumentVersion int64 // LSP only; zero outside editor sessions
}
```

This resolves the LSP design question: keep `body.ResultVersion()` as the stable
body input digest, and add `SolveSeq` when a transport needs a monotonic result
id.

`CompletedResult` is immutable after publication:

```go
type CompletedResult struct {
    Tag          ResultTag
    Program      program.Result
    Bodies       []BodyResultRef
    Summaries    summary.Snapshot
    Judgments    []judgment.Judgment
    Diagnostics  []diagnostic.Diagnostic
    Manifest     *manifest.Manifest
    Placement    placementplan.Plan
}
```

Core publication steps:

1. Parse and bind sources for the unit.
2. Prepare/check via `program.RunChunk` or future incremental program runner.
3. Export manifest with `exportmanifest.FromProgramResult`.
4. Produce raw judgments through `pass.New(...).Run(pass.Context{Reader:
   internalreadmodel.NewWithParents(...)})`.
5. Render diagnostics from judgments with the judgment policy.
6. Build placement with `placementplan.FromProgramResult`.
7. Publish one immutable `CompletedResult` under `ResultTag`.

## Incrementality

Digest-driven incrementality is core-owned, not adapter-owned.

- Body-level invalidation uses `body.ResultVersion()` and the body-input digest
  work described in `analysis/architecture/lsp_design.md`.
- Unit/save/admission invalidation uses deterministic manifest encode bytes plus
  source/profile digests.
- A body result can be reused only if its body input digest, dependency summary
  digests, manifest inputs, state lanes, and profile-affecting options match.
- A unit result can be reused only if its `UnitDigest`, `ManifestDigest`, and
  profile match the requested solve.
- Adapters can request freshness, but they cannot compute dirty closures,
  debounce solves, or decide staleness themselves.

The existing LSP review constraints remain binding:

- Pull diagnostics return versioned last-complete results.
- Pull requests do not block on checker completion.
- The server owns debounce, cancellation, and single-flight solves.
- Untagged diagnostics are invalid.

## Surface Forms

### 1. Terminal

Transport: a CLI that reads file paths, workspace specs, or source on stdin and
writes JSONL by default. Human rendering is an optional projection, not the
canonical output.

Core queries used:

- `EnsureSolved`
- `ListJudgments`
- `ListDiagnostics`
- `EvidenceTrace`
- `ExportManifest`
- `PlacementPlan`
- `DiffDiagnostics` through `diagnostic/diffreport`

JSONL records should be stable and append-friendly:

```json
{"kind":"result","unit":"webhooks/repo","result_version":17,"unit_digest":"..."}
{"kind":"judgment","result_version":17,"code":"assignment.type","anchor":"module:...","verdict":"unknown","cause":["missing-proof"]}
{"kind":"diagnostic","result_version":17,"code":"type.assignment","severity":"error","file":"main.lua","span":{"start_line":10,"start_col":7}}
```

Incrementality: batch CLI runs can be one-shot, but the output format and cache
directory are keyed by core digests so a future daemon/watch mode can reuse the
same session store.

Out of scope for v1: editor scheduling, LSP pull state, direct source mutation,
and adapter-local diagnostic family grouping. Code/family summaries come from
core or `diffreport`.

### 2. HTTP / LSP

Transport: LSP over stdio and the same LSP methods over HTTP, plus small HTTP
query endpoints for non-LSP clients. Both speak to one `WorkspaceSession`.

Core queries used:

- `UpsertUnit` on document/workspace changes
- `EnsureSolved` with trigger and document version
- `LastComplete` for pull diagnostics and hover
- `ListDiagnostics` for LSP diagnostics
- `JudgmentsByRange`, `JudgmentsByAnchor`, `TypeAt`, `EvidenceTrace` for hover
- `RepairsForJudgment` later, when JIR repairs are implemented
- `SemanticFactsAt` later, for semantic tokens

Incrementality: exactly the `lsp_design.md` contract. The adapter owns protocol
translation and capability negotiation. Core owns dirty closure, single-flight
solve, result publication, and last-complete cache reads. Syntax-invalid open
documents may expose parser feedback and a stale completed result; they do not
create a partial checker result.

Out of scope for v1: macro/build-system integration, recovery type inference,
rename, formatting, and any LSP-only type inference.

### 3. In-Runtime

Transport: in-process Go API embedded by Wippy runtime admission/compile paths.
This is the generalized form of the current typed admission gate.

Core queries used:

- `EnsureSolved` under the admission profile
- `ListDiagnostics` or `ListJudgments` for the admission verdict
- `ExportManifest` for module-boundary metadata
- `PlacementPlan` for runtime allocation/admission decisions
- `SummarySnapshot` only for trusted runtime internals that need compiled graph
  facts

Incrementality: save/admission reuse is keyed by source digests, manifest
digests, and profile. Runtime admission should ask for authoritative same-
profile results, not lower-latency profile-independent projections.

Out of scope for v1: terminal JSONL formatting, LSP scheduling, HTTP server
state, and agent exploration queries unless the embedding runtime explicitly
uses the SDK.

### 4. Agent SDK ("Boundary Driver")

Transport: internal Go SDK first. A thin HTTP/RPC wrapper can follow only after
the Go query vocabulary stabilizes. Agents query compiled/solved graph data and
must not re-parse Lua source.

Core queries used: the full query API below.

Incrementality: every query includes a `ResultSelector`. The default selector is
"latest complete", but mutating workflows should pin `ResultTag` and reject
answers if a newer result superseded it.

Out of scope for v1: applying edits, speculative repair synthesis, creating new
checker facts, exposing raw mutable `state.State`, or promising cross-edit
identity beyond `SubjectAnchor`/JIR identity fields.

## Query Vocabulary

All requests include:

```go
type ResultSelector struct {
    UnitID        UnitID
    ResultVersion uint64 // SolveSeq; zero means latest complete
    Profile       string
}
```

All responses include:

```go
type QueryMeta struct {
    Tag   ResultTag
    Stale bool
}
```

### Result and Status

```go
type GetResultRequest struct {
    Selector        ResultSelector
    RequireComplete bool
}

type GetResultResponse struct {
    Meta   QueryMeta
    Status ResultStatus // unsolved, parsing, solving, complete, failed
}
```

Used by every adapter to discover the tag it is reading.

### List Judgments

```go
type ListJudgmentsRequest struct {
    Selector ResultSelector
    Codes    []judgment.Code
    Verdicts []judgment.Verdict
    Anchors  []judgment.SubjectAnchor
    Range    *SourceRange
}

type ListJudgmentsResponse struct {
    Meta      QueryMeta
    Judgments []judgment.Judgment
    CodeSpecs []judgment.CodeSpec
}
```

This is the canonical terminal JSONL and agent diagnostic input. Adapters must
not rebuild verdicts from diagnostics.

### Judgments by Anchor

```go
type JudgmentsByAnchorRequest struct {
    Selector  ResultSelector
    AnchorKey string // judgment.SubjectAnchor.StableKey()
    Codes     []judgment.Code
}

type JudgmentsByAnchorResponse struct {
    Meta      QueryMeta
    Anchor    judgment.SubjectAnchor
    Judgments []judgment.Judgment
}
```

Used for LSP hover, diffreport stability, and agent follow-up on a known
subject.

### Diagnostics

```go
type ListDiagnosticsRequest struct {
    Selector ResultSelector
    Policy   judgment.PolicyConfig
    Range    *SourceRange
}

type ListDiagnosticsResponse struct {
    Meta        QueryMeta
    Diagnostics []diagnostic.Diagnostic
}
```

Core owns policy application and judgment rendering. Terminal human output then
calls `diagnostic.Render` with `diagnostic.RenderOptions`; LSP maps to protocol
diagnostics.

### Type or Value at Subject

```go
type TypeAtRequest struct {
    Selector ResultSelector
    Anchor   *judgment.SubjectAnchor
    Point    cfg.Point
    Path     path.Path
}

type TypeAtResponse struct {
    Meta          QueryMeta
    ProjectedType typ.Type
    Value         product.Value // Go SDK only; wire forms use a DTO
    Proven        bool
    Evidence      judgment.EvidenceChain
}
```

First implementation can support anchor-backed subjects produced by judgments.
Point/path lookup needs a public readmodel addition before it is stable.

### Solved Facts at Point

```go
type FactsAtPointRequest struct {
    Selector ResultSelector
    Point    cfg.Point
    Paths    []path.Path
    Lanes    []state.LaneID
}

type FactsAtPointResponse struct {
    Meta  QueryMeta
    Facts []SolvedFact
}

type SolvedFact struct {
    Lane      state.LaneID
    Path      path.Path
    Type      typ.Type
    Placement *placementplan.Entry
    Evidence  judgment.EvidenceChain
}
```

This is a DTO projection, not raw `state.State`. It starts narrow and expands as
SDK users need facts.

### Evidence Chain and Witness Trace

```go
type EvidenceTraceRequest struct {
    Selector  ResultSelector
    Judgment  JudgmentID
    AnchorKey string
    Code      judgment.Code
    FullTrace bool
}

type EvidenceTraceResponse struct {
    Meta      QueryMeta
    Judgment  judgment.Judgment
    Evidence  judgment.EvidenceChain
    Trace     []OriginNode // future evidence-origins/JIR nodes
}
```

Today this returns flat `judgment.EvidenceChain`. When evidence origins land,
the same query returns bounded origin nodes.

### Placement Plan

```go
type PlacementPlanRequest struct {
    Selector ResultSelector
}

type PlacementPlanResponse struct {
    Meta QueryMeta
    Plan placementplan.Plan
}
```

Adapters expose the plan directly. They do not inspect heap/object state.

### Summaries

```go
type SummarySnapshotRequest struct {
    Selector ResultSelector
    Keys     []summary.SummaryKey
}

type SummarySnapshotResponse struct {
    Meta      QueryMeta
    Summaries []summary.EntrySummary
    Digests   map[summary.SummaryKey]summary.Digest
}
```

Raw `summary.Summary` is in-process SDK only. Wire adapters should expose
descriptor-specific DTOs or JIR, not the entire lattice payload.

### Callgraph and Summary Relations

```go
type CallRelationsRequest struct {
    Selector ResultSelector
    Caller   *summary.SummaryKey
    Range    *SourceRange
}

type CallRelationsResponse struct {
    Meta      QueryMeta
    Relations []CallRelation
}

type CallRelation struct {
    Caller      summary.SummaryKey
    Callee      summary.SummaryKey
    Point       cfg.Point
    Kind        string // direct, path, context, unresolved, ambiguous
    Subject     judgment.SubjectAnchor
    SummaryUsed bool
}
```

The current program package has internal maps for function/target/path keys, but
not a public call relation DTO. This query is a first-class SDK requirement.

### Manifest and Module Boundary

```go
type ExportManifestRequest struct {
    Selector ResultSelector
}

type ExportManifestResponse struct {
    Meta   QueryMeta
    Path   string
    Digest Digest
    Data   []byte // manifest.Encode output
}
```

The manifest digest is the save/admission invalidation key for importers.

### JIR Export

```go
type ExportJIRRequest struct {
    Selector     ResultSelector
    Origins      OriginMode // none, fired, all
    IncludeRepair bool
}

type ExportJIRResponse struct {
    Meta QueryMeta
    Data []byte
}
```

This is intentionally a projection over `CompletedResult`, not a separate solve.

## Reality Check: Hardest Gaps

1. **Program solve is batch-oriented.**
   `program.RunChunk` solves the whole bound program, then materializes bodies.
   It does not accept a dirty body closure or reuse per-body completed results.
   First step: publish a `ProgramInputGraph` from parse/bind/prepare that maps
   body ids, summary keys, binder references, and consumed summary digests. The
   first implementation can conservatively invalidate the whole unit while still
   storing the graph shape.

2. **No workspace session or production module loader.**
   Fixture code orders modules and passes manifests manually. There is no core
   owner for module paths, source snapshots, dependency order, or external
   manifest loading.
   First step: add `analysis/check/service` with `UnitInput`,
   `WorkspaceSession`, and a reference batch implementation that lifts the
   `checktest` composition into production names without importing test-only
   packages.

3. **Completed results are not retained as query snapshots.**
   Diagnostics APIs tend to render and return diagnostics, losing raw judgments
   and the readers needed for SDK queries.
   First step: introduce `CompletedResult` publication in core and produce both
   raw judgments and rendered diagnostics from one pass before any adapter is
   written.

4. **Thread-safety is undefined.**
   `typevalue.Cache`, body query caches, and materialized body results were built
   for single-run ownership. HTTP/LSP and agent SDK queries require concurrent
   reads and solve cancellation.
   First step: make sessions single-writer and publish immutable completed
   snapshots. Audit mutable caches; clone, freeze, or guard each value that
   escapes to concurrent readers.

5. **The SDK asks broader questions than `readmodel.Reader` answers.**
   The current `Reader.ForEach...` APIs are narrow solved-occurrence iterators
   for diagnostic passes. Agents need point/path types, call relations, summary
   relations, and witness traces.
   First step: implement `ListJudgments`, `JudgmentsByAnchor`, `PlacementPlan`,
   and `SummarySnapshot` first, because existing data already supports them.
   Then add narrowly tested readmodel/query DTOs for `TypeAt`, `FactsAtPoint`,
   and `CallRelations`.

## First Code Slice

Do not build terminal, LSP, runtime, or SDK adapters first.

The first implementation slice should be a core-only package:

1. Define `UnitInput`, `ResultTag`, `CompletedResult`, and `WorkspaceSession`.
2. Implement a batch `WorkspaceSession` using parse, `program.RunChunk`,
   `exportmanifest.FromProgramResult`, `pass.New(...).Run`, diagnostics
   rendering, and `placementplan.FromProgramResult`.
3. Store completed immutable results keyed by `UnitID`, `UnitDigest`, profile,
   and `SolveSeq`.
4. Add tests proving one source produces a completed result with:
   raw judgments, rendered diagnostics, manifest bytes/digest, placement plan,
   summary snapshot, and stable body `ResultVersion`s.

Only after this exists should adapters be added. The CLI then streams core
records. LSP maps core records to protocol responses. Runtime admission calls
core. Agents use core queries.
