# LSP Core Design: Snapshot Algebra and Location Identity

Status: converged foundation for the checker embedding surface. This document
supersedes the source-identity, resolution, result-version, debug-correlation,
and incrementality premises in [lsp_design.md](lsp_design.md). It incorporates
the accepted RETHINK corrections from the LSP critique.

## The invariant

The engine receives immutable, materialized inputs and performs **zero
filesystem, registry, network, watcher, or overlay I/O while solving**. A
source location is meaningful only for the exact bytes that produced it. Human
labels and protocol URIs are projections over those identities, never cache
keys or semantic identity.

This keeps an editor overlay, a historical registry revision, and a deployed
artifact from being silently conflated merely because they refer to the same
logical code entry.

## Snapshot algebra

`DocumentID` is a stable, comparable logical-document identity:

```text
DocumentID = { Scheme: file | registry | mem, OpaqueKey }
```

Its equality is exactly `(Scheme, OpaqueKey)`. It deliberately excludes every
revision, digest, LSP document version, registry-view version, resolver view,
and solve sequence. Registry keys include an explicit logical source-slot or
language discriminator, not merely an entry ID.

The remaining identities are distinct values:

| Value | Meaning | Must not be used as |
| --- | --- | --- |
| `SourceSnapshot` | `{Document, ProviderRevision, ContentDigest, Content}` for one frozen byte sequence. The engine recomputes/verifies the digest. | A stable document identity or UI label. |
| `SourceLocation` | `{DocumentID, ContentDigest, ByteSpan}` plus optional line/column convenience fields. Byte offsets are canonical. | A location in a newer buffer or another artifact. |
| overlay/client version | A host-local edit-order fence. | Content identity or a registry revision. |
| `ResolutionSnapshot` | `{WorkspaceViewID, ViewDigest, UnitPlan[]}` for roots, profile, overrides, registry snapshot, aliases, and import policy. | A single document revision. |
| `UnitPlan` | Stable unit ID, module path, entry/source document IDs, and explicit imports `{Alias, TargetUnit, ManifestDigest}`. | An instruction to discover imports during solve. |
| `SolveSeq` | Session-local completed-publication ordinal. | A body digest, deployment identity, or runtime key. |
| `BodyInputDigest` | Identity of all inputs consumed by one body solve. | A session publication sequence. |

The current pinned `embedding` package owns these DTOs. Its schema version is
independent of JIR because a host can materialize units and consume source/query
identity without serializing the judgment IR. Additive capability evolution is
negotiated; removals or semantic reinterpretations require an embedding-schema
bump and a cross-repository compatibility update.

### Locations and display

Every engine result that points to source carries a document-aware,
digest-bound location internally. A location from digest A is not silently
applied to digest B; a host remaps it explicitly or marks the semantic result
stale. Line/column and LSP UTF-8/UTF-16 positions are derived from the exact
snapshot, not stored as cross-version identity.

Display is a separate `DocumentLabeler` projection. For the reference `file`
scheme, the label is precisely the supplied path, preserving the existing
fixture render byte-for-byte. Registry labels, URI encoding, case/symlink
normalization, and retaining a client's exact URI belong to Wippy. A URI has no
revision in it; Wippy owns a versioned, bijective registry URI codec.

## Materialize before solve

The resolution boundary is intentionally outside the checker:

```text
Wippy registry snapshot + client overlays
  -> resolver freezes roots, aliases, imports, manifests, and workspace view
  -> ResolutionSnapshot + exact SourceSnapshots
  -> materialized UnitInput
  -> go-lua parse/bind/check/immutable CompletedResult
```

Wippy obtains a single pinned registry snapshot, layers per-client overlays,
resolves its actual registry dependency graph, and freezes all referenced
source bytes before submitting a unit. An overlay is not silently written to
the registry. Save uses a conditional registry transaction against its base
registry revision.

Source providers answer only “give bytes for this exact `DocumentID` and
constraint.” They do not resolve imports. The resolver owns aliases, module
edges, manifests, roots, override precedence, and mixed registry/file policy.
Initially Wippy emits one independently checked unit per registry source entry
and passes dependency manifests explicitly: the current batch checker parses
only the entry source of a multi-source input.

## Repository split

go-lua is the engine and conservative embedding layer. It owns stable identity
DTOs, materialized service inputs, immutable result tags, source/query facts,
debug maps, and tiny reference adapters. It may provide file and memory source
adapters, a diagnostics-and-hover stdio example, and JSONL, but those adapters
do not define product behavior.

Wippy owns the product: registry source extraction/subscriptions, canonical
URI/display codecs, the authoritative registry/manifest resolver, tenant and
multi-root workspace views, overlay storage, tolerant current syntax, UTF-aware
position mapping, scheduling, hosted HTTP/WebSocket LSP, protocol capabilities,
code-action/rename UX, persistent workspace index, save/CAS policy, and DAP.
Wippy can change ranking, labels, panels, transport, and workflow without a
go-lua release; a new semantic fact or proof rule properly needs one.

The first semantic query floor is explicit rather than presumed: engine output
must grow parse/body maps, definitions and all occurrences, position-to-subject
and type facts, document symbols, call relations, judgments/evidence, semantic
facts, and structured counterfactually verified repair candidates. Current
judgment anchors are not stable binder IDs and cannot alone make rename or
runtime joins sound.

## Debug correlation vocabulary

Debug correlation names an admitted artifact and an execution, not just a
source anchor. The five-part vocabulary is:

```text
StaticArtifactID = unit digest + body digest + profile + engine build + debug-map digest
SourceSite       = DocumentID + content digest + subject/source anchor
ExecutionSite    = DebugPointID + phase (before/after/call/return/suspend/...)
Deployment       = admitted-artifact deployment/generation identity
RuntimeInstance  = actor/PID + activation/frame + optional resource/object handle
```

go-lua emits artifact-to-source maps and stable `DebugPointID`s during lowering.
Raw CFG points remain optional metadata scoped to that exact artifact; they are
not cross-digest identities. Wippy owns deployment, actor/frame/resource
identity, authorization, live fact kinds, subscriptions, and DAP presentation.
The UI must show an artifact/editor mismatch rather than joining live data to
an unrelated overlay by anchor alone.

## Honest incrementality

The landed service is cancellable, rejects obsolete-input publication, and
publishes immutable whole-unit results. It does **not** yet parse only dirty
bodies, maintain a program input graph, reuse body summaries, perform
transitive invalidation, or enforce server single-flight. A body digest is a
useful reusable-work identity, not evidence that reuse happens today.

The delivery sequence is therefore:

1. Start with measured whole-unit solves: Wippy debounces, cancels obsolete
   versions, enforces latest-only/single-flight per unit, and publishes only a
   result matching its still-current source and resolution snapshots.
2. Serve current tolerant-syntax parse diagnostics plus explicitly stale,
   last-complete semantic results during broken edits. Never invent a second
   semantic type system.
3. Measure parse, queue, solve, publication, stale-hit, cancellation, query,
   and index-update latency by unit size. Do not claim sub-second large-module
   feedback before the measurements support it.
4. Only then add stable body IDs, engine-consistent recoverable parse/body maps,
   a `ProgramInputGraph`, conservative dirty closure, and actual body/summary
   reuse. Manifest digest changes remain the module-boundary invalidation rule.

Pull diagnostics remain nonblocking cache reads. They include the exact source
and resolution snapshot/result tag that produced them; during a newer pending
solve Wippy returns the last completed tag and schedules the latest work.
