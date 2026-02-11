# Canonical Paths

This document defines the canonical package path for each subsystem in `go-lua`.
When adding or moving logic, prefer these paths to avoid scattered ownership.

## Layer Boundary

1. `types/*`: language-agnostic type/effect/constraint/flow domains and algebra.
2. `compiler/check/*`: checker pipeline, extraction, inference, and orchestration.
3. `compiler/*` (outside `check`): AST/bind/CFG/parser/front-end primitives.

Rule: `types/*` must not depend on `compiler/check/*`.

## Canonical Ownership

1. Callsite symbol/callee/receiver/runtime-arg resolution:
   - `compiler/check/callsite`
2. CFG-to-constraints extraction pipeline:
   - `compiler/check/flowbuild`
3. Flow extraction symbol/path/type resolution helpers:
   - `compiler/check/flowbuild/resolve`
4. Type-annotation AST resolver:
   - `compiler/check/synth/phase/resolve`
5. Return extraction from CFG (return kinds/constraints):
   - `compiler/check/flowbuild/returns`
6. Interprocedural return graph/SCC/join/widen/signature logic:
   - `compiler/check/returns`
7. Post-flow interprocedural fact propagation:
   - `compiler/check/infer/interproc`
8. Nested function graph coordination/enrichment:
   - `compiler/check/nested`
9. Nested inference processor over solved facts:
   - `compiler/check/infer/nested`
10. Driver/fixpoint orchestration and diagnostics:
    - `compiler/check/pipeline`
11. Type constructors/joins/normalization:
    - `types/typ`, `types/typ/join`
12. Flow domain/join/propagation:
    - `types/flow`, `types/flow/join`, `types/flow/propagate`
13. Constraint AST/solver/theory:
    - `types/constraint`, `types/constraint/theory`
14. Numeric parsing helpers:
    - `types/numparse`

## Intentional Name Collisions (Different Responsibilities)

1. `resolve`:
   - `compiler/check/flowbuild/resolve` = runtime/flow extraction resolution
   - `compiler/check/synth/phase/resolve` = type annotation resolution
2. `returns`:
   - `compiler/check/flowbuild/returns` = return extraction from CFG
   - `compiler/check/returns` = interprocedural return inference engine
3. `nested`:
   - `compiler/check/nested` = nested graph discovery/enrichment
   - `compiler/check/infer/nested` = nested post-flow inference processor

## Current Tree Anomaly

1. `compiler/check/numparse` exists but is empty.
2. Canonical numeric parsing implementation lives in `types/numparse` and all imports already point there.

## Cleanup Policy

1. Do not add new parsing logic under `compiler/check/numparse`.
2. Remove `compiler/check/numparse` if no near-term plan requires the directory.
3. If adding new call/callee symbol rules, implement in `compiler/check/callsite` and consume from other packages.
4. If adding new type lattice operations, implement in `types/typ` or `types/flow`, not in checker packages.

