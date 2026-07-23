# V2 Analysis And Code Artifact Cache

Status: integration design. This document separates the currently reconstructed
lint, the current v2 compiler/runtime, and the intended shared artifact path. It
does not describe either existing implementation as already unified.

## Current systems

### Reconstructed lint

The reconstructed lint is a host adapter over canonical go-lua analysis. It
materializes resolved units, invokes the checker service, and consumes completed
diagnostics and manifests. It does not emit arena VM code and must not become a
second compiler frontend.

Canonical go-lua publishes an immutable `service.CompletedResult` containing
manifests, judgments, rendered diagnostics, summaries, placement plans, body
digests, debug maps, and static artifact identities. The service deliberately
calls `ReleaseTransientTree` before publication, so solved CFG state and the
mutable body result tree do not escape the solve.

### Current v2 compiler

The v2 runtime compiles through go-lua-arena. Its effective pipeline is:

```text
source
  -> parse and source rewrites
  -> arena checker and exported manifest
  -> codegen function tree
  -> LIR and optimization
  -> arena Proto
  -> v2 CodeGraph CompiledNode and Program
```

`runtime/lua/engine.CodeGraph` owns registry revisions, dependency invalidation,
program validation, and runtime artifact generations. Its recursive build cache
is limited to one build. Manager-held programs and the eval LRU are in-memory
runtime caches, not a durable compiler cache.

`CompiledNode.ArtifactDigest` is a runtime generation identity. It is not a
portable compilation key: it is revision-based and does not fully fence the
analyzer, compiler, optimizer, VM ABI, runtime-module ABI, transforms, or
admission policy.

### Future integration

Lint and compilation will share one canonical semantic solve. The solve will
publish a schema-pinned immutable analysis capsule with a compiler projection.
Lint will derive diagnostic projections from that capsule; arena lowering will
derive executable artifacts from the compiler projection. The v2 CodeGraph will
continue to own live runtime generations and references.

This is an integration at the analysis-to-lowering seam, not a replacement of
the abstract domain, solver, arena LIR/emitter, VM, or actor runtime.

## One CAS, several artifacts

Use one content-addressed store with typed immutable artifacts and explicit
dependency edges. Do not use one monolithic result blob or one key for every
consumer.

```text
SemanticInputKey
       |
       v
AnalysisCapsule --------------------+
       |                            |
       v                            v
LintProjection                 CompilerProjection
                                    |
                                    v
                               CodegenArtifact
                                    |
                                    v
                            AdmissionCertificate
                                    |
                                    v
                       Runtime Artifact Generation
```

The first five nodes may be stored in the CAS. A runtime artifact generation is
live node-local state that references CAS digests; it is not made safe to free
by CAS eviction.

Every artifact envelope must contain:

- artifact kind and schema version;
- canonical payload digest;
- ordered dependency artifact digests;
- producer build/schema tags needed by that artifact kind;
- deterministic encoding, with unknown required fields rejected.

## Keys and payloads

### Semantic input

`SemanticInputKey` covers every input that can change analysis meaning:

- stable unit and document identities plus exact source content digests;
- normalized resolution snapshot/view digest and unit plan;
- each import alias, resolved target unit, and canonical manifest digest;
- canonical runtime-module and standard-library manifests;
- global names and types, enabled state lanes, and semantic analysis profile;
- analyzer build tag and analysis/manifest schema versions.

It excludes display labels, document versions, diagnostic severity or
enablement, and judgment rendering policy. Those inputs do not change program
semantics and must not invalidate compiler output.

The current checker-service `UnitDigest` is close but includes diagnostic and
judgment policy. Introduce a policy-free semantic digest rather than using the
current digest unchanged as a durable shared key.

### Analysis capsule

The `AnalysisCapsule` is keyed by `SemanticInputKey` and contains only immutable,
schema-pinned projections:

- canonical exported manifest bytes and digest;
- normalized function summaries and their digests;
- effects, typestate, guards, obligations, and compiler-relevant proof facts;
- placement, ownership, escape, suspension, and lifetime facts;
- stable body identities, source anchors, debug maps, and debug-map digests;
- the immutable compiler projection described below.

It must not retain CFG worklists, transfer closures, mutable query caches,
interner ownership, or `body.Result` pointers.

### Lint projection

`LintProjectionKey` is:

```text
analysis capsule digest
+ JIR and diagnostic schema versions
+ judgment policy digest
+ diagnostic policy digest
+ display-label digest
```

Its payload is raw judgments, rendered diagnostics, and presentation evidence.
Changing severity, wording policy, or a displayed path reuses semantic analysis
while producing a new lint projection. Document version and solve sequence are
publication metadata, never durable semantic identity.

### Compiler projection

Canonical analysis must create an immutable `CompilerProjection` before
`ReleaseTransientTree`. This is the missing integration seam. It must be a
versioned DTO, not an exported mutable checker object.

At minimum it contains:

- canonical typed function/body structure needed by lowering;
- stable operation, value, allocation, call-site, and body IDs;
- resolved type/effect/typestate facts used by specialization;
- placement and ownership licenses joined to those stable IDs;
- import, runtime-module, host, type-value, and chunk binding facts;
- source/debug anchors sufficient to preserve diagnostics and operational maps;
- declared static module dependencies and remaining dynamic-require facts.

The ID mapping must survive source rewrites and lowering. Source spans alone are
not a valid join key after inserted bindings, specialization, or inlining.

After projection, canonical analysis may release the transient result tree as it
does today. Arena lowering consumes this DTO and never reaches back into a
checker session.

### Code generation

`CodegenKey` is:

```text
compiler projection digest
+ transformed-source/lowering-input digest
+ complete compile profile and code-affecting policy
+ runtime-module ABI digest
+ source-transform and call-specializer revisions
+ ordered binding vectors
+ optimizer plan and compiler/lower build tag
+ VM opcode/Proto ABI
+ target/backend and debug/operational-map schemas
```

The runtime-module ABI digest includes module names and numeric IDs, dependency
classes, canonical manifests, runtime-call IDs, effects, yield/boundary
certificates, and every declaration embedded into emitted code.

The payload contains the arena `Proto`, exported manifest digest, module
dependencies, binding vectors, operational/debug maps, call certificates, claim
tables, and dependency codegen digests. Loading validates the payload and every
declared ABI/schema before use.

### Admission

Security admission is not implied by a codegen hit.

`AdmissionKey` is:

```text
codegen artifact digest
+ complete admission policy digest
+ admission schema/build tag
```

The policy digest includes allowed and denied module classes and names, denied
runtime-call flags, workflow determinism, unsafe-builtin rules, hot-call policy,
capability restrictions, and any host-specific admission constraint.

Its payload is a validated admission certificate with the exact policy and ABI
digests it proves. A host may share bytecode across policies, but it must obtain
or recompute a certificate for the requested policy.

## Runtime generations are separate

The CAS owns cold immutable bytes. The actor runtime owns executable lifetime.

A v2 runtime artifact generation binds:

- registry `NodeID` and monotonic node generation;
- the exact dependency generation vector;
- codegen artifact and admission certificate digests;
- runtime load/shape-catalog revision and loaded handles;
- retained imported artifacts and frozen pure-library exports.

Existing `CodeGraph.ProgramValid`, supersession, actor/pool/eval retention, and
pure-library borrower/refcount rules remain authoritative. Updating source
publishes a new generation; existing actors keep the old retained generation.
Evicting a CAS entry or an eval lookup cannot free a live `Program`, loaded
`Proto`, shape handle, or frozen export.

Registry programs, eval programs, and future JIT artifacts share content
identity but retain different lookup, eviction, and lifetime policies.

## Migration stages

1. **Pin identities.** Version the canonical manifest, analysis capsule,
   compiler projection, arena lowering, optimizer, runtime-module ABI, VM Proto,
   debug map, and admission schemas. Add hit/miss and rejection accounting.
2. **Materialize canonical units.** Adapt a v2 `CodeGraph` snapshot into canonical
   checker `UnitInput`: registry document identity, exact source, resolved import
   plan, dependency manifest digests, runtime manifests, globals, and profile.
3. **Run in observation mode.** Keep the current v2 checker/codegen authoritative
   while running canonical analysis. Diff manifests, diagnostics, effects,
   placement, entry requirements, and admission decisions.
4. **Publish analysis capsules.** Add the immutable compiler projection before
   transient release. Make reconstructed lint consume the capsule and lint
   projection without changing v2 compilation.
5. **Adapt arena lowering.** Add one explicit lowering entry that consumes the
   compiler projection. Keep the current source/checker entry as a per-unit
   fallback. Do not let lowering import checker internals.
6. **Enable codegen CAS reads and writes.** Reconstruct `CompiledNode` with the
   current graph generation and runtime ownership on a hit. Always validate ABI,
   dependencies, payload, and admission. Resolve static stdlib and type-value
   bindings before lowering so cache identity does not depend on retry compiles.
7. **Move registry and eval incrementally.** Select canonical artifacts by
   profile or unit, retain current fallback, and preserve eval LRU/pins as a
   lookup/lifetime policy over shared CAS identity.
8. **Retire duplicate checking.** Remove the arena legacy checker from the
   authoritative path only after corpus parity and security gates pass. Retain
   arena LIR/emission and the v2 runtime generation model.

## Parity and security gates

No stage becomes authoritative without all applicable gates:

- canonical manifest bytes and imported type aliases round-trip without loss;
- diagnostics/judgments have reviewed diffs, with no unchecked or deadline-
  skipped unit accepted for compilation;
- function effects, yields, suspension, process identity, typestate, guards,
  obligations, and entry admission agree or fail closed;
- placement/ownership facts map exactly to emitted allocation and call-site IDs;
- Proto, operational maps, certificates, and runtime requirements are
  deterministic for the same keys;
- changed source, resolution, dependency manifest, transform, runtime-call ABI,
  optimizer, compiler, VM ABI, or policy produces a miss or validation failure;
- a cache hit never widens module, command, send, workflow, or host capability;
- corrupt, partial, unknown-schema, or dependency-mismatched artifacts are
  rejected and rebuilt;
- canceled or failed solves publish no capsule or executable artifact;
- hot reload preserves old actor behavior until the last generation reference
  is released;
- cold and warm end-to-end tests compare diagnostics, manifests, executable
  behavior, and artifact accounting across registry, eval, workflow, function,
  process, and pure-library paths.

The acceptance condition is one semantic solve serving lint and compilation,
without merging presentation policy, executable admission, or actor lifetime
into a single unsafe cache key.
