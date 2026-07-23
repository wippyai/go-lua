# Stage 6 fanout guide

Slice 1 (`equation/perf`) is the shared admission foundation. It adds
`CompiledArtifact`, `CompiledCyclicArtifact`, conservative operand kinds, and
the two VM-based differential gates. It does not change the production
execution path.

## Stable inputs

- `CompileArtifact` accepts one closed, acyclic `equation.Artifact`, freezes a
  deterministic topological operation order, and rejects cycles.
- `CompileCyclicArtifact` records compact operation descriptors beside the
  existing frozen WTO certificate. It must not derive a replacement schedule.
- `CompiledArtifact` stores one scalar byte arena plus index descriptors. An
  `EntryTerm` is an `OperandEntryProjection`; opaque closed terms are
  `OperandCanonicalConstant`. The other operand kinds are reserved until their
  source lowering proves them.
- `ReferenceArtifact` performs byte-for-byte canonical confirmation. It is a
  transitional test adapter, not a cache key or a production evaluator.
- `RunCompiledDifferential` compares the VM's complete output closure.
  `RunCompiledCyclicDifferential` additionally compares each widening-trace
  record.

## Parallel lanes

1. `equation/perf` may add generated operation-family stencils and fast
   acyclic execution using `CompiledArtifact`. Cyclic execution must consume
   the retained WTO hierarchy and preserve widening points exactly. Do not
   change `RunCompiled*Differential`; route each new executor through it.
2. `interproc/perf` owns worker-local `EvaluatorScratch`, projection encoding,
   and the fixed 64-byte `{ArtifactID, ProjectionID}` key. Every primary-key
   hit must confirm both canonical byte payloads before reuse. Scratch must be
   scalar-indexed, explicitly clear pointer-bearing temporary state, and meter
   overflow as a cold path.
3. `runtime-contract` owns an immutable `RuntimeProjection` codec. Facts need
   artifact/contract provenance and a `proven` or conservative `unknown`
   status. Consumers may optimize only `proven` facts; guards deopt.
4. `interproc/perf` cache work owns sharded scalar slots, cold publication and
   invalidation paths, and parity with `ProjectedTable` for collisions,
   invalidation, joins, and 1/10/100 fanout.
5. `perf/gates` owns warmed `0 B/op, 0 allocs/op` cached-hit checks, per-file
   2-second measurement, seq-2555 remeasurement against 106.5s, and actual
   4M-reference pointer/RSS/scan accounting.

## Integration order and non-negotiables

The evaluator, cache, and RuntimeProjection lanes can proceed independently.
Integrate only after each lane has retained its boundary contract; serialize
the compiled-call adapter and final full gate run. The reference VMs remain
the oracle until the 5531/5531 closure comparison and the cyclic trace
comparison both stay exact. Never treat a digest as equality, never infer an
operand/JIT fact from opaque bytes, and never hide an allocation or scratch
overflow on the hot path.

For every commit run `go build ./...`, the equation/interproc/program/
transformer/state/factapply tests, the scoped differential checktest sweep,
and checktest twice. Compare each census against `base309.txt`: zero added
failure names is required.
