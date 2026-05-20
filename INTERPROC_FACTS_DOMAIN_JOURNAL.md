# Interproc Facts And Checker Domain Design Journal

## 2026-05-19 Design Consolidation Checkpoint

This document records the design model before the next implementation pass. It is
not an implementation plan for incremental bridges. The intended correction is a
flash migration: design the final shape, migrate directly to it, delete the old
helper clusters, and do not leave compatibility wrappers or fallback layers in
the production checker.

## 2026-05-19 Implementation Checkpoint

First flash-migration slice landed for parameter evidence ownership.

Changed production shape:

- `api.FunctionFact` now owns canonical parameter evidence in `Params`.
- `api.Facts.ParamHints` was removed.
- `api.SnapshotStore.GetParamHintsSnapshot` was removed.
- same-iteration merge and interproc widening now combine parameter evidence
  through `FunctionFacts`.
- post-flow call observation publication now emits `FunctionFacts` deltas with
  `Params` instead of writing a side channel.
- return inference seeds local function parameter evidence from canonical
  `FunctionFacts`.
- Salsa snapshot facts now track one canonical fact product for parameters,
  returns, narrow returns, and function type projection.

This is intentionally not a bridge. No production code reads a legacy
`ParamHints` fact channel and no compatibility writer reconstructs it from
`FunctionFacts`.

Second cleanup slice in the same migration:

- the local inference package was renamed from `infer/paramhints` to
  `infer/paramevidence`, then the domain was moved to
  `domain/paramevidence` when its lattice laws were consolidated;
- `LocalFuncInfo.ParamHints` became `LocalFuncInfo.ParameterEvidence`;
- phase input `ParamHintSignatures` became `ParameterEvidenceSignatures`;
- local call-graph propagation now exposes `PropagateParameterEvidence`;
- helper files and regression fixtures were renamed to parameter-evidence
  terminology;
- production checker code no longer contains `ParamHint` or `paramhints`
  identifiers.
- parameter-use projection now treats builtin `type(param)` checks and
  `param = param or {}` self-default assignments as shape-neutral guard/default
  operations instead of whole-parameter escapes. Those operations must not turn
  a call-site record observation into a closed public contract.

Verification notes:

- `go test ./...` passes.
- `git diff --check` passes.
- `../scripts/verify-suite.sh` passes go-lua checker tests and builds the Wippy
  binary, then exits non-zero in external lint targets while building Wippy
  against `github.com/wippyai/go-lua v1.5.16`.
- A temp local-replace replay under `/tmp/wippy-golua-local-replace` builds
  Wippy against this checkout without editing external code. It reduced the
  projection-related false positives, but the full external sweep is still not
  clean: tests/app 2 errors/4 warnings, session 20, actor/test 3, agent/src 12,
  docker-demo 72, llm/src 10, llm/test 9, migration 1, views 1.

Remaining cleanup after this parameter-evidence slice:

- return/narrow/type projections still need the same treatment: read-only views
  over the canonical function summary product, not separate authorities.
- Remaining local-replace external diagnostics must be classified in the next
  engine slice. Some are soundness-preserving real-code issues (`any` flowing
  into concrete contracts); some still expose missing checker power, especially
  public functions that validate invalid input with `type(...)` guards and
  should infer a wider accepted input domain without weakening the guarded body.

## 2026-05-19 Domain Rectification Checkpoint

The next flash-migration slice moved parameter evidence out of inference/return
orchestration and into a domain owner:

- `compiler/check/infer/paramevidence` was moved to
  `compiler/check/domain/paramevidence`;
- shared value-shape predicates that were duplicated during the first move were
  factored into `compiler/check/domain/value`;
- parameter-evidence vector/map normalization, join, widening, table-top
  absorption, nilability splitting, soft/concrete selection, and truthy-key
  refinement now live under domain packages;
- `returns` no longer owns parameter evidence merge helpers. Function-fact
  parameter slots delegate to `paramevidence.JoinVectors`,
  `paramevidence.FilterEmptyVector`, and `paramevidence.RefinesFunctionParam`;
- return-summary and parameter-evidence code both call `domain/value` for
  optional elision, truthy refinements, soft/concrete preference, recursive
  structural scanning, and record-extension checks;
- parameter-evidence law tests moved with the domain, so the tests describe the
  owner instead of the old return package.

This is not a compatibility bridge. The old package path and old
`WidenParameterEvidence` API were deleted. Call sites moved directly to the
domain package.

Verification for this slice so far:

- `go test ./compiler/check/domain/value` passes.
- `go test ./compiler/check/domain/paramevidence` passes.
- `go test ./compiler/check/returns` passes.
- `go test ./compiler/check/...` passes.
- `go test ./...` passes.
- `git diff --check` passes.
- Standard `../scripts/verify-suite.sh` passes the go-lua checker tests and
  Wippy binary build, then exits non-zero on external lint targets while the
  Wippy checkout is still using its pinned go-lua module: session 8 errors,
  agent/src 8 errors, docker-demo 21 errors and 2 warnings.
- Local-replace replay with
  `WIPPY_DIR=/tmp/wippy-golua-local-replace GOFLAGS=-buildvcs=false` also
  passes the go-lua checker tests and Wippy binary build, then exits non-zero
  on known external diagnostics: tests/app 2 errors/4 warnings, session 20,
  actor/test 3, agent/src 11, docker-demo 72, llm/src 9, llm/test 9,
  migration 1, views 1.

Design result:

- orchestration still decides when evidence is collected from calls, body use,
  post-flow observations, or signatures;
- the parameter-evidence domain now decides how evidence combines;
- the value domain owns shared structural predicates instead of duplicating them
  under returns and parameter evidence;
- helper names that encode parameter-specific lattice laws are no longer local
  return-package predicates.

## 2026-05-19 Value Shape Domain Checkpoint

The follow-up rectification removed another cluster of domain laws from
`returns` and moved it into `compiler/check/domain/value`.

Moved value-shape laws:

- soft container refinement;
- stale falsy map-key refinement;
- nested nil-only regression detection;
- recursive structural-growth detection;
- structural-shape unwrapping and shallow shape equality;
- union member extraction after structural unwrapping.

`returns` now keeps return-vector orchestration, but it asks `domain/value` for
value-shape facts. This preserves the current behavior while making the mental
model cleaner:

```text
returns          = return-vector policy and function-summary alignment
domain/value     = reusable structural value relations
domain/paramevidence = parameter evidence lattice and parameter-slot refinement
```

This is a direct ownership move, not a bridge. The old local helpers were
deleted from `returns`.

Verification for this slice so far:

- `go test ./compiler/check/domain/value ./compiler/check/returns` passes.
- `go test ./compiler/check/...` passes.
- `go test ./...` passes.
- `git diff --check` passes.
- Standard `../scripts/verify-suite.sh` passes the go-lua checker tests and
  Wippy binary build, then exits non-zero on the existing external lint targets:
  session 8 errors, agent/src 8 errors, docker-demo 21 errors and 2 warnings.

## 2026-05-19 Return Summary Domain Checkpoint

The next rectification slice moved return-vector policy and function-signature
return alignment out of `compiler/check/returns` and into
`compiler/check/domain/returnsummary`.

Moved domain laws:

- return-vector equality and nil-slot canonicalization;
- return-vector normalization with soft-union pruning;
- directional refinement, optional elision, record extension, and nil-slot fill;
- concrete-over-soft summary preference and stale falsy map-key refinement;
- nested nil-only regression protection;
- recursive structural-growth stopping for table builders;
- nested `never` artifact repair;
- higher-order monotone summary merge for function-returning-function and
  self-recursive method shapes;
- summary-to-function-return alignment and conservative unknown return
  attachment for otherwise returnless callable values.

Production callers now import `domain/returnsummary` directly. The old
`returns.ReturnTypes*`, `returns.MergeReturnSummary`,
`returns.NormalizeReturnVector*`, `returns.AlignFunctionTypeWithSummary`,
`returns.WithSummaryOrUnknown`, `canonicalReturnVector`, and
`normalizeAndPruneReturnVector` names were deleted instead of wrapped.

Current package ownership:

```text
domain/value         = reusable structural value relations
domain/paramevidence = parameter evidence lattice, equality, and parameter-slot refinement
domain/returnsummary = return-vector lattice and function-return alignment
returns              = function-fact product orchestration and interproc widening
```

This keeps one clear abstract-interpreter data flow:

1. flow and return inference produce candidate return evidence;
2. `domain/returnsummary` decides how return vectors normalize, compare, merge,
   and align to callable types;
3. `returns` only decides when function-fact products are joined or widened;
4. Salsa snapshots continue to observe the canonical fact product rather than a
   compatibility mirror.

This is a flash migration, not a bridge. Production code no longer calls the old
return-summary helpers through `returns`.

Verification for this slice so far:

- `go test ./compiler/check/domain/returnsummary ./compiler/check/returns`
  passes.
- `go test ./compiler/check/...` passes.
- `go test ./...` passes.
- `git diff --check` passes.
- `go test ./compiler/check -run '^$' -bench BenchmarkCheck_LargeFunction
  -benchmem -count=3` reports about 1.15 ms/op, 882 KB/op, and 9390
  allocs/op on this machine.
- Standard `../scripts/verify-suite.sh` passes go-lua checker tests and builds
  the Wippy binary, then exits non-zero on the known external pinned lint
  targets: session 8 errors, agent/src 9 errors, docker-demo 21 errors and
  2 warnings.

## 2026-05-19 Function Fact Domain Checkpoint

The next rectification slice moved the per-function fact laws out of
`compiler/check/returns` and into `compiler/check/domain/functionfact`.

Moved domain laws:

- canonicalization and emptiness for one `api.FunctionFact`;
- same-iteration join for one function fact;
- merge policy for function-type fact projections;
- compatible function-variant collapse inside unions while preserving residual
  non-function union members;
- same-shape function merging across params, variadic params, returns, effects,
  error-return specs, and refinements;
- parameter-slot fact merge policy that delegates to `domain/paramevidence`;
- return-slot fact merge policy that delegates to `domain/returnsummary`.

Production callers now import `domain/functionfact` directly for individual
function facts. The old `returns.JoinFunctionFact`,
`returns.MergeFunctionFactType`, `returns.NormalizeFunctionFact`, and
`returns.NormalizeFunctionFacts` names were deleted instead of wrapped.

Current package ownership:

```text
domain/value         = reusable structural value relations
domain/paramevidence = parameter evidence lattice, equality, and parameter-slot refinement
domain/returnsummary = return-vector lattice and function-return alignment
domain/functionfact  = one-function fact normalization, join, and type projection
returns              = function-fact maps, captured effects, local SCC orchestration, and interproc widening
```

The resulting data flow is now narrower:

1. inference and post-flow code produce one-function deltas through
   `functionfact.Join`;
2. `returns.JoinFacts` and `returns.WidenFacts` decide how those deltas combine
   across symbol maps and fixpoint iterations;
3. convergence-specific widening remains in `returns` because it depends on the
   whole interprocedural product and iteration boundary;
4. no production code calls legacy per-function fact helpers through `returns`.

Verification for this slice so far:

- `go test ./compiler/check/domain/functionfact ./compiler/check/returns
  ./compiler/check/...` passes.
- `go test ./...` passes.
- `git diff --check` passes.
- `go test ./compiler/check -run '^$' -bench BenchmarkCheck_LargeFunction
  -benchmem -count=3` reports about 1.15-1.17 ms/op, 882 KB/op, and 9390
  allocs/op on this machine.
- Standard `../scripts/verify-suite.sh` passes go-lua checker tests and builds
  the Wippy binary, then exits non-zero on the known external pinned lint
  targets: session 8 errors, agent/src 10 errors, docker-demo 21 errors and
  2 warnings.

## 2026-05-19 Fact Product Domain Checkpoint

The next rectification slice moved the whole interprocedural fact product out of
`compiler/check/returns` and into `compiler/check/domain/factproduct`.

Moved product laws:

- `api.Facts` equality;
- same-iteration product join;
- recursive-boundary product widening;
- function-fact map canonicalization and deterministic symbol enumeration;
- literal signature join/widen;
- captured type join/widen;
- captured field assignment join/widen;
- captured container mutation join/widen;
- constructor-field join/widen;
- deterministic captured field/container equality and merge helpers.

Production callers now import `domain/factproduct` directly. The old
`returns.WidenFacts`, `returns.JoinFacts`, `returns.FactsEqual`,
`returns.ConstructorFieldsEqual`, `returns.WidenLiteralSigs`,
`returns.JoinLiteralSigs`, captured-fact join/widen/equality helpers, and
captured merge helpers were deleted from `returns` instead of wrapped.

Test ownership was rectified at the same time:

- return-vector and return-summary law tests moved to `domain/returnsummary`;
- one-function fact join/type-merge tests moved to `domain/functionfact`;
- whole-product tests moved to `domain/factproduct`;
- `returns` keeps only local return orchestration tests.

Current package ownership:

```text
domain/value         = reusable structural value relations
domain/paramevidence = parameter evidence lattice, equality, and parameter-slot refinement
domain/returnsummary = return-vector lattice and function-return alignment
domain/functionfact  = one-function fact normalization, join, and type projection
domain/factproduct   = whole api.Facts product join, widening, equality, and map domains
returns              = local return SCC orchestration, call graph, overlays, signature seeding
store                = snapshot/Salsa wiring and fixpoint publication
```

This separates the abstract interpreter more cleanly:

1. local inference produces function and mutation deltas;
2. `domain/functionfact` and the other slot domains define one-slot meaning;
3. `domain/factproduct` defines how the whole interprocedural product combines;
4. the store decides when to apply join or widening and when to publish Salsa
   snapshot inputs;
5. `returns` no longer owns cross-graph product laws.

Verification for this slice so far:

- `go test ./compiler/check/domain/factproduct ./compiler/check/store
  ./compiler/check/returns ./compiler/check/...` passes.
- `go test ./...` passes.
- `git diff --check` passes.
- `go test ./compiler/check -run '^$' -bench BenchmarkCheck_LargeFunction
  -benchmem -count=3` reports 1.17-1.19 ms/op, 881 KB/op, and 9390 allocs/op
  on this machine.
- Standard `../scripts/verify-suite.sh` passes go-lua checker tests and builds
  the Wippy binary, then exits non-zero on the known external pinned lint
  targets: session 8 errors, agent/src 10 errors, docker-demo 21 errors and
  2 warnings.

## 2026-05-19 Convergence Law Ownership Checkpoint

The next rectification slice removed convergence and structural value laws from
`domain/factproduct`. The fact-product domain now composes slot domains instead
of carrying private copies of their logic.

Moved laws:

- higher-order recursive-growth detection moved to `domain/value`;
- convergence widening for one `typ.Type` moved to `domain/value`;
- unsafe precision-drop detection moved to `domain/value`;
- return-vector convergence widening moved to `domain/returnsummary`;
- one-function fact convergence widening moved to `domain/functionfact`;
- same-signature return-slot merging for function literals moved to
  `domain/functionfact`;
- related tests moved to the packages that own the laws.

The old local helper names are gone from production code:

```text
mergeFunctionReturnsIfSameShape
widenFunctionFactTypeForConvergence
widenReturnSummaryForConvergence
maybeWidenTypeForConvergence
widenValueTypeForConvergence
typeUnsafePrecisionDrop
returnsummary.HasHigherOrderGrowthRisk
```

Current convergence flow:

1. `domain/value` defines structural type relations and finite-height
   convergence approximations.
2. `domain/returnsummary` widens return vectors using the value domain.
3. `domain/functionfact` widens one `api.FunctionFact` using parameter evidence,
   return summaries, and value relations.
4. `domain/factproduct` widens maps and fact slots only by delegating to those
   owners.

Verification for this slice so far:

- `go test ./...` passes.
- `git diff --check` passes.
- `go test ./compiler/check -run '^$' -bench BenchmarkCheck_LargeFunction
  -benchmem -count=3` reports 1.14-1.16 ms/op, 881 KB/op, and 9390 allocs/op
  on this machine.
- Standard `../scripts/verify-suite.sh` passes go-lua checker tests and builds
  the Wippy binary, then exits non-zero on the known external pinned lint
  targets: session 8 errors, agent/src 10 errors, docker-demo 21 errors and
  2 warnings. One first run printed agent/src 12 errors; direct replay of that
  target and a full rerun both returned 10.

## 2026-05-19 False-Positive Replay And Domain Refinement Checkpoint

The next pass classified remaining local-replace lint failures and fixed the
ones that were checker false positives without weakening `any` soundness.

Direct engine fixes:

- the call pipeline now re-synthesizes every expected-sensitive argument form
  that can change meaning under a concrete callee parameter expectation:
  function literals, table literals, identifiers, attribute reads, explicit
  casts, logical operators, call expressions, and non-nil assertions;
- intersection callees now publish contextual expected-argument vectors during
  phase one, using the same merge law as union callees while still requiring
  `FinishCall` to validate every intersection member;
- positive field-literal narrowing is now a domain meet for top/open table
  shapes instead of only a union filter. A guard such as `part.type == "image"`
  materializes the proven field on `any`, table top, maps, and open records with
  row-tail evidence. Existing closed broad fields keep the previous "may match"
  policy, so `field: string` does not collapse to a literal singleton merely
  because one branch compared it.

The key false-positive class was:

```lua
for _, part in ipairs(content) do
    if part.type == "text" and part.text and part.text ~= "" then
        table.insert(content_blocks, { text = part.text })
    elseif part.type == "image" then
        convert_image_to_converse(part)
    end
end
```

When `content` came from `any`, the negative side of the text branch could
create an open `{text: ""}` shape. The later `part.type == "image"` check kept
that open shape because the open tail could contain `type`, but it failed to
record the hard proof that this branch's `type` field is present and equal to
`"image"`. The result was a false error when passing `part` to a helper that
requires a `type: string` field.

Correct abstract interpretation:

```text
Observation: part.type == "image"
Location:    Location(part).field("type")
Evidence:    hard runtime proof, field-literal equality
Domain:      value/shape meet
State:       open row-tail shape plus explicit type = "image"
Query:       helper parameter assignability sees required type field
```

Wrong interpretation:

```text
open row-tail may contain type -> keep the old shape unchanged
```

That wrong interpretation lost proof. It was not a reason to let `any` flow
into concrete contracts generally.

Regression coverage added:

- imported optional response-body fallback into an imported string call;
- explicit cast of an imported unknown field into an imported method call;
- intersection callee expected-argument publication;
- logical/cast/call/non-nil expected-sensitive argument re-synthesis;
- discriminated array elements from typed and untyped sources;
- open-record field-literal meet commutativity and union refinement laws.

Local-replace Wippy replay after this fix:

- `wippy.llm.bedrock:mapper` line 240 is clean; the reproduced checker false
  positive is gone.
- `wippy.llm.bedrock:mapper` still reports line 503 (`parse_text_tool_call(text,
  tool_names)` with `text` from `text_blocks`). This is not fixed in go-lua
  because `text_blocks` is populated from `block.text` on an untyped external
  payload. `if block.text then` proves truthiness, not stringness. Treating that
  as string would be an `any`-to-concrete unsoundness unless the engine grows an
  explicit successful-operator refinement model for `..` and string methods.
- session dependency diagnostics such as `expected string, got string?` remain
  tied to pinned/locked external source shapes without a local fallback or cast.
- larger local-replace sweeps still contain true strictness diagnostics where
  `any`, `unknown`, optional values, or intentionally invalid test inputs flow
  into concrete contracts. Those must not be hidden by changing go-lua
  assignability.

Verification for this pass:

- `go test ./types/constraint ./types/flow ./types/narrow` passes.
- `go test ./compiler/check/synth/phase/extract ./compiler/check/synth/ops
  ./compiler/check/tests/regression` passes.
- `go test ./compiler/check/...` passes.
- `go test ./...` passes.
- `git diff --check` passes.
- `go test ./compiler/check -run '^$' -bench BenchmarkCheck_LargeFunction
  -benchmem -count=3` reports about 1.13-1.15 ms/op, 881 KB/op, and 9390
  allocs/op on this machine.
- `../scripts/verify-suite.sh` passes the go-lua checker tests and Wippy binary
  build, then exits non-zero on the external pinned lint targets:
  session 8 errors, agent/src 8 errors, docker-demo 21 errors and 2 warnings.
  The rest of the verify-suite lint targets report zero diagnostics.

## 2026-05-19 Typed Write Boundary Reactualization

The current validation pass found a real soundness gap while classifying
external diagnostics:

```lua
local CONFIG = { chars_per_token = 4 }

local function configure(new_config)
    for key, value in pairs(new_config) do
        if CONFIG[key] ~= nil then
            CONFIG[key] = value
        end
    end
end

configure({ chars_per_token = "bad" })
return 10 * CONFIG.chars_per_token
```

This is not a false positive. The write `CONFIG[key] = value` is a typed write
boundary. The guard proves that the key names an existing slot; it does not
prove that the incoming value is compatible with that slot. Accepting the write
and only complaining later at arithmetic is weaker and can miss the real source
of unsoundness.

Correct abstract interpretation:

```text
Observation: CONFIG[key] = value
Location:    dynamic index location under CONFIG with key evidence
Evidence:    assignment value evidence plus key-domain evidence
Domain:      memory/write projection asks the value domain for the target slot
State:       write accepted only if value <= writable slot type
Query:       later reads can trust unchanged numeric slots
Diagnostic:  failed write compatibility, not arithmetic fallout
```

Wrong interpretation:

```text
dynamic key exists -> mutate CONFIG with value and hope later reads catch it
```

The final ownership must not be a local assignment-hook helper. A checker hook
may format the diagnostic, but the semantic operation is a pure domain/query
law:

```text
WriteProjection(containerType, keyType) -> writable value type
```

For the current codebase shape, the flash migration target is:

- `types/query/core` owns pure read/write projection over structural types;
- assignment checking asks that query for computed-index write targets;
- ordinary field enrichment remains a memory/evidence operation, not a subtype
  check against the current read projection;
- numeric literal fields widen to their primitive numeric type for writes so
  mutable defaults are not frozen as singleton slots;
- `any` and `unknown` remain unresolved, not concrete proof.

Implementation ownership was corrected accordingly:

- `types/query/core.IndexWrite` is the pure write-side projection.
- exact finite key domains use a write-side meet: a value must satisfy every
  slot the key may denote.
- broad dynamic keys only produce a projection when all possible slots have one
  uniform writable type. Heterogeneous dynamic keys require memory/key-value
  relation evidence; a type-only projection must not invent that relation.
- mixed direct-field plus row-tail writes are projected only when the direct
  slot and row-tail slot agree. Otherwise the write belongs to the memory
  relation domain, not to a single structural slot.
- mutable structural slots may widen singleton defaults for write projection,
  but closed finite literal domains remain closed. A field declared as
  `"queued" | "started"` must not become `string` just because the destination
  is mutable.
- the assignment hook now only asks the query for a target slot and checks the
  source against it. It does not own the structural write law.

This keeps the rule aligned with the abstract machine:

```text
Transfer observes a write.
Type query projects the writable slot type.
Assignability checks value evidence against that slot.
Diagnostics project failure evidence.
```

Regression requirements for this class:

- negative: untyped URL/resource/config data flowing into concrete contracts is
  rejected;
- positive: explicit `type(...) == "string"` guards feed string contracts;
- positive: typed config updates preserve numeric reads;
- negative: bad config updates fail at the write boundary;
- law: write projection does not overwrite unrelated named fields for dynamic
  keys.

This also reclassifies the clean external replay:

- `tests/app` overlay URL errors are source/contract issues because `args.url`
  is untyped and may be truthy non-string.
- `views` resource ID error is source/contract unless the source validates each
  `entry.data.resources` element as string.
- `llm` provider model and Bedrock text-block errors are source/contract unless
  the source or manifest proves stringness.
- `llm.util:compress` config mutation is a checker-design stressor: reads must
  stay precise for valid typed config updates, and invalid dynamic writes must
  be rejected at the write boundary.

## 2026-05-19 Partial-Interpreter Architecture Diagnosis

The recurring bug shape is not "one more missed helper". It is a split semantic
authority problem. The checker still has several partial interpreters:

- synthesis decides some expression meaning and some contextual typing;
- flow transfer decides some path and mutation meaning;
- narrowing queries decide some refined read meaning;
- assignment/call/return hooks decide some compatibility and diagnostic meaning;
- interprocedural inference decides some function-summary meaning;
- query/subtype/value packages decide some structural laws.

This split is why a local fix can look correct and still expose another nearby
failure. The same evidence can pass through different semantic owners depending
on whether it appears as a table literal, call argument, returned closure,
dynamic write, field read, or interprocedural fact. That is the architectural
issue two steps back.

The current event-bus projector failure is in that class:

```lua
function Builder:build(): protocol.Projector
    return function(state: protocol.BusState, event: protocol.Event, at)
        state.projections[event.id] = { updated_at = at }
    end
end
```

The return annotation provides an expected function type
`(BusState, Event, time.Time) -> ()`. That expected type must become contextual
parameter evidence for the returned closure before the closure body is checked.
If `at` remains `unknown` inside the closure, the abstract interpreter lost
evidence at a phase boundary. The typed write check is then correctly strict:
`unknown` cannot be assigned to `time.Time?`. The bug is not the write
projection; the bug is missing canonical expected-function transfer into the
nested function's environment.

Correct abstract interpretation:

```text
Observation: return function(...) ... end
Context:     enclosing function has expected return slot protocol.Projector
Evidence:    returned expression is checked against that expected slot
Transfer:    expected function params seed the closure parameter locations
State:       closure body sees at: time.Time
Query:       dynamic write projection accepts updated_at: time.Time?
```

Wrong interpretation:

```text
return expression type is checked after closure body synthesis
closure parameter at defaults to unknown
write boundary rejects table field using that unknown
```

The final architecture must make expected type context part of the same
abstract-machine input as flow and facts. It cannot be a hook-local retry. The
flash-migration rule for this class is:

- expected types are contextual evidence at expression boundaries;
- function-literal contextual evidence owns parameter seeding for the nested
  function body;
- table-literal contextual evidence owns field/value synthesis;
- call-argument contextual evidence owns argument synthesis;
- assignment/write contextual evidence owns source synthesis;
- diagnostics are emitted after the canonical transfer/query has answered.

No production code should grow another "if returned closure, then synth again"
bridge. The implementation must route the expected function type into the
existing function-literal type construction and nested-function analysis path so
all returned callbacks, assigned callbacks, table-held callbacks, and call
arguments use the same rule.

## 2026-05-19 Remaining Architecture Tasks After Split-Authority Diagnosis

The work is not complete until these items are true at the engine level:

- `subtype` is a pure structural relation. It must not be the owner of
  provenance-sensitive rules such as "this mutable record can widen because it
  is fresh".
- write-slot projection is a pure destination query. It may widen singleton
  defaults for mutable local ergonomics, but it must preserve closed finite
  literal domains such as `"queued" | "started"`; those domains are semantic
  contracts, not incidental singleton defaults.
- assignability is the single owner for checking a value against an expected
  destination under a mode: call argument, return slot, local declaration,
  structured write, and contextual literal checking.
- freshness and escape state are represented by the abstract interpreter or by a
  conservative provenance query over the solved graph. Syntax alone is not
  proof; a direct table literal and an unescaped local whose current value is
  that literal should be accepted for the same reason.
- mutable record singleton-to-union widening is allowed only at a proven fresh
  contextual boundary. A narrower alias must not be allowed to observe later
  writes through a wider mutable slot.
- callback contextual typing must seed nested function parameter locations
  before body/write diagnostics are produced.
- convergence must remain domain-owned: no iteration caps, no equality-time
  repairs, no producer-specific fallback channels.
- Salsa should cache immutable graph summaries, function results, type queries,
  and eventually provenance/use summaries. It must not hide incomplete semantic
  inputs.

Immediate implementation checklist:

- keep the `ApplyParamList` nil-annotation-slot fix, because it is the canonical
  parameter-list law for contextual function parameters;
- add the missing fresh-literal escape rule at the assignability/provenance
  boundary, not by weakening global mutable record subtyping;
- add a positive regression where a returned callback builds a local projection
  literal and writes that local into a typed map;
- add a negative regression where a narrower mutable alias is written through a
  wider destination and then observed through the narrow alias;
- do not enable full static field-path write diagnostics until class/metatable
  self-reference assignments (`T.__index = T`) have a canonical self-type model;
- replay the real event-bus fixture and local external lint cases before
  claiming there are no false positives;
- update this journal with the final owner names and verification output.

## Goal

The checker should read as one abstract interpreter over a product domain.

The current implementation is already powerful:

- it tracks flow-sensitive path facts,
- it narrows through guards and assertions,
- it propagates table and container mutations,
- it infers local and interprocedural function facts,
- it correlates value/error return slots,
- it handles soft annotation evidence,
- it uses Salsa-style query inputs for function-result invalidation.

The design problem is that these capabilities are encoded by many local helper
clusters. That makes the system hard to reason about even when the behavior is
mostly correct. Helpers such as `typeRefinesTableKeyByTruthiness` are not just
helpers; they are domain laws living in the wrong place.

The target is a smaller, clearer checker where each law has exactly one owner.

## Non-Negotiable Constraints

- No production transition layer.
- No legacy mirror fact channels.
- No raising iteration caps to hide non-convergence.
- No external application-code edits as part of go-lua design correction.
- No weakening soundness by making `any` assignable to concrete contracts.
- No helper-specific exceptions for external lint targets.
- No pools as the first answer to performance; use structural ownership and
  caching first.
- Every final abstraction must have law tests and paired positive/negative
  behavioral tests.

## Current Mental Model

The checker is a multi-phase abstract interpreter:

1. Scope and CFG construction establish symbols, lexical parents, control-flow
   points, and function graph identity.
2. Declared-phase synthesis extracts initial types, table literal shapes,
   function literal signatures, and call/effect evidence.
3. `flowbuild` lowers AST and synthesis facts into flow inputs:
   declarations, assignments, table/index mutations, call effects, branch
   predicates, return constraints, numeric constraints, aliases, and termination
   facts.
4. `types/flow` solves a forward dataflow problem over canonical SSA path keys.
   The persistent solved state is currently split across value maps, conditions,
   numeric states, alias maps, field overlays, and local caches.
5. Narrowing queries are demand-side interpretation: read solved facts at a
   point, apply propagated constraints, and answer refined path/type questions.
6. Return inference and local function SCC solving use the flow result plus
   interprocedural snapshots to infer return vectors, parameter evidence, function
   facts, captured fields, and captured container mutations.
7. The interprocedural store combines same-iteration deltas with a precise join
   and combines recursive fixpoint boundaries with widening.
8. Salsa-style snapshot inputs connect function-result queries to exact
   interproc facts, refinements, and constructor-field snapshots.

This is the right high-level shape. The weakness is that the product domain is
not first-class enough in code.

## Clean Abstract Interpreter Target

The final checker should be explainable as:

```text
AbstractInterpreter = CFG + AbstractState + Transfer + Join + Widen + Query
```

Where:

- `CFG` owns control-flow order and dominance.
- `AbstractState` owns the full product of memory, value, numeric, relation,
  effect, and termination facts.
- `Transfer` is the only way statements and expressions change state.
- `Join` is the only way same-phase branch/predecessor evidence combines.
- `Widen` is the only way recursive or interprocedural cycles are forced to
  converge.
- `Query` reads solved state without inventing another analysis path.

This is the mental model the code should expose. If a rule cannot be explained
as one of these operations, it is either orchestration or a design smell.

The current checker has the right ingredients but not the right ownership. It
has preflow inference, flow solving, narrowing queries, return SCC inference,
overlay refresh, mutation replay, and interproc widening. Those should become
clients of the same abstract-state and domain APIs. They should not remain
separate places where local helpers decide what refinement means.

## State-Of-The-Art Bar

The target is not just cleaner Go packages. The target is a modern static
analysis engine with explicit theory:

- monotone abstract domains with named `Normalize`, `Leq`, `Join`, `Meet`, and
  `Widen` operations;
- transfer functions over a product state instead of helper-specific rewrites;
- a first-class memory model for paths, fields, indexes, aliases, mutations,
  row tails, and dominance;
- relational facts for tuple slots and path correlations instead of hardcoded
  error-return branches;
- principled distinction between `unknown`, `any`, `nil`, absent fields, soft
  evidence, hard evidence, table top, and open row tails;
- explicit widening at recursive boundaries and optional narrowing only after a
  post-fixpoint is reached;
- deterministic canonicalization and equality, never equality-time repair;
- cache keys derived from immutable inputs and domain snapshots, not incidental
  phase call order;
- paired positive/negative law tests so the implementation cannot get faster by
  becoming less sound.

Anything less will keep producing local helper patches. The migration should
make the checker look like the theory it is implementing.

## Core Moral Model

The checker should be taught and reasoned about with one sentence:

```text
Evidence is produced by transfer, combined by domains, stabilized by widening,
and observed by queries.
```

That sentence is the guardrail.

- Extraction does not decide lattice policy. It only converts source syntax into
  typed evidence and transfer instructions.
- Transfer does not decide cross-iteration convergence. It only updates the
  current abstract state.
- Domains do not inspect AST. They only combine abstract values and facts.
- Widening does not recover precision. It only guarantees convergence.
- Queries do not produce new facts. They only read the solved state and apply
  already-recorded constraints.
- Interprocedural producers do not mutate old state. They emit deltas.

If a function violates one of these rules, it is a design smell even if the
behavioral test passes.

## One-Page Doctrine

The final checker should fit in this operational doctrine.

```text
1. Source syntax is lowered once into graph-indexed transfer IR.
2. Transfer IR is interpreted over one product AbstractState.
3. AbstractState owns every persistent intraprocedural fact.
4. Domain objects own every combine/refine/widen law.
5. Queries are read-only views over solved AbstractState.
6. Function inference publishes immutable InterprocDelta values.
7. FactsDomain is the only interprocedural merge/widen authority.
8. Salsa tracks immutable inputs and query dependencies.
```

Everything else is implementation detail.

The doctrine gives a direct review test:

- If code lowers syntax, it belongs in graph/IR/extract.
- If code changes state, it is transfer.
- If code combines facts, it is a domain operation.
- If code forces convergence, it is widening.
- If code answers a question, it is a query.
- If code crosses function/module boundaries, it emits or consumes a delta.
- If code caches, it must name immutable inputs and invalidation.

No rule should need to be implemented twice under different helper names.

## Abstract Machine Specification

The final checker should be specified as a small abstract machine. This gives the
code a single target shape and gives reviews a way to reject scattered helper
logic.

```text
Machine =
  Inputs
  + Program
  + State
  + Domains
  + Worklist
  + QueryView
  + Publisher
```

### Inputs

Inputs are immutable during one function analysis query:

- graph identity,
- parent scope identity,
- manifest/module environment,
- declared type environment,
- canonical interproc snapshot,
- constructor snapshot,
- effect/refinement snapshot,
- graph summaries,
- pure type-query engine.

Inputs are the only values allowed to affect the answer besides the transfer
program. If an answer depends on something not listed here, the dependency model
is incomplete.

### Program

The program is normalized checker IR:

- no source AST policy decisions,
- no hidden synthesis callbacks,
- no direct store mutation,
- no cache-dependent control flow.

Each instruction has one meaning as a transfer over `AbstractState`.

### State

The state is the product:

```text
State =
  Memory
  x Values
  x Shapes
  x NumericFacts
  x Relations
  x Effects
  x Termination
  x DiagnosticsEvidence
```

`DiagnosticsEvidence` is not user diagnostics. It is proof metadata such as
"this constraint failed here" or "this widening lost precision here". User
diagnostics are emitted after solving by querying this evidence. This keeps
diagnostic formatting out of domain semantics.

### Domains

Domains define the algebra:

```text
Normalize, Leq, Join, Meet, Refine, Widen, Equal
```

Every operation must be local to its owned component or explicitly part of a
product operation. For example, relation transfer can ask value and memory
domains to interpret a path predicate, but it cannot create a private value
merge law.

### Worklist

The worklist owns traversal, not meaning.

Allowed:

- schedule CFG points,
- schedule SCC members,
- detect local stabilization,
- invoke loop/SCC widening at declared boundaries.

Forbidden:

- prefer one fact over another,
- normalize facts,
- publish interproc state,
- recover precision after widening.

If the worklist needs semantic information to decide convergence, that
information must be exposed through `Leq` or `Equal` on the relevant domain.

### QueryView

The query view is a read-only projection over solved state.

It answers:

- type at location/point,
- relation at location/point,
- effect summary at call/function boundary,
- return tuple summary,
- parameter obligation summary,
- diagnostic projection.

It must not write facts, widen, repair state, or backfill caches that later act
as analysis state.

### Publisher

The publisher converts solved state into immutable deltas:

```text
State -> FunctionResult -> InterprocDelta
```

The publisher does not merge with previous results. It does not reconstruct
legacy channels. It emits the final product-domain representation expected by
`FactsDomain`.

### Machine Transition Rules

The core machine transitions are:

```text
step(instruction, state) = Transfer.Apply(instruction, state, domains)
join(predStates)         = AbstractState.Join(predStates, domains)
widen(prev, next)        = AbstractState.Widen(prev, next, domains)
query(state, question)   = QueryView.Answer(state, question)
publish(state)           = InterprocDelta
```

Every specialized feature should reduce to these transitions:

- branch narrowing is transfer plus join;
- field writes are memory transfer;
- table mutators are effect transfer plus memory transfer;
- assertions are effect transfer plus relation/value refinement;
- error-return behavior is relation transfer over tuple slots;
- callback behavior is higher-order effect transfer;
- local function inference is an SCC over function-state summaries;
- interproc inference is a fixpoint over `InterprocDelta` values.

If a feature cannot be expressed this way, either the machine is missing a
domain or the feature is implemented at the wrong layer.

### Machine Laws

The implementation should preserve these laws:

- Transfer is monotone with respect to domain `Leq`.
- Join is least-upper-bound or a documented approximation.
- Meet/refine never invents evidence without provenance.
- Widen is only applied at explicit recursive boundaries.
- Normalize is idempotent and is not hidden in equality.
- Query is pure over solved state.
- Publication is deterministic.
- Cache hits do not change semantics.
- Diagnostics are projections of evidence, not sources of evidence.

These laws should become test names. A regression that violates one of them is a
design regression, not a local bug.

## Ownership Ledger

Every semantic object should have one home. This table is the fastest review
tool for the future flash migration.

| Object | Born In | Canonical State | Transformed By | Queried By | Published As | Cache Boundary |
|---|---|---|---|---|---|---|
| symbol identity | graph build | graph bundle | never semantically transformed | location resolver | graph key/symbol key | graph input |
| parent scope | scope build | immutable scope state | never semantically transformed | analysis key lookup | parent hash | `FuncKey`/`GraphKey` |
| field/index path | IR/path lowering | `Location` / `MemoryState` | memory transfer | query view | captured path/mutation delta | location interning |
| local value fact | transfer | `AbstractState.Values` | value domain | type-at query | return/param/capture delta when exported | per-function state |
| table shape fact | literal/assignment transfer | value + memory domains | value/memory domains | field/index query | function/captured/container delta | type query + local state |
| branch truthiness | condition transfer | relation/value constraints | relation/value domains | query view | relation summary if it crosses boundary | per-function state |
| nil/absent evidence | assignment/field transfer | memory + value domains | memory/value domains | field query | return/param/capture delta | per-function state |
| parameter observation | call transfer | parameter evidence domain | parameter domain | function summary query | function fact delta | interproc facts input |
| body obligation | body transfer | parameter evidence domain | parameter domain | function summary query | function fact delta | graph summary + state |
| return tuple | return transfer | return summary domain | return domain | return query | function fact delta | interproc facts input |
| tuple/path relation | predicate/effect/return transfer | relation domain | relation domain | relation query | relation summary delta | local state / interproc facts |
| table mutation | assignment/effect transfer | memory domain | memory domain | iteration/field query | captured container delta | local state / interproc facts |
| call effect | effect resolution | effect domain | effect domain | transfer/query view | refinement/effect delta | effect snapshot input |
| termination fact | transfer/effect transfer | termination domain | termination domain | reachability query | function effect delta | per-function state |
| diagnostic evidence | failed constraint transfer/query | diagnostics evidence state | diagnostic projection only | diagnostics pass | no semantic delta | result only |
| constructor field | constructor transfer/publication | constructor field domain | memory/value domains | constructor query | constructor snapshot | constructor input |
| external dynamic value | manifest/effect transfer | value evidence with provenance | value/domain checks | assignability query | only if exported with provenance | manifest/type input |

Design rule:

```text
If a row needs two canonical states, the model is split incorrectly.
If a row has no cache boundary, the implementation will invent one locally.
If a row has two publishers, legacy mirror channels are coming back.
```

## Dataflow Moral Rules

The checker should be easy to explain because the direction of information never
reverses.

### Syntax To Evidence

Syntax can create observations. It cannot create authority by itself.

Examples:

- a table literal observes fields;
- a call observes arguments;
- a guard observes a branch condition;
- a return observes tuple slots.

These observations become evidence only through transfer and domain
qualification.

### Evidence To Fact

Evidence becomes a fact when the owning domain accepts it into state.

Examples:

- a field observation becomes a memory fact at a canonical location;
- a truthy guard becomes a relation/value constraint;
- a body use becomes a parameter obligation;
- a call argument becomes a parameter observation.

No producer decides global precedence. The evidence order belongs to the domain.

### Fact To Answer

Answers are read-only projections.

Examples:

- "what is the type here?",
- "does this path exclude nil?",
- "what does this function return?",
- "does this call terminate?",
- "which diagnostic should be emitted?".

An answer cannot become a fact unless a later transfer explicitly observes it
and routes it through the owning domain. This prevents query-time analysis.

### Fact To Delta

Only solved facts that cross a function or module boundary become deltas.

Examples:

- local temporary narrowing does not publish;
- body obligation publishes as parameter evidence;
- return tuple publishes as return summary and relation summary;
- captured mutation publishes as memory/effect summary;
- external contract application does not rewrite the contract.

The publisher emits a delta; `FactsDomain` combines it.

### Delta To Snapshot

Snapshots are cache inputs, not semantic repair points.

Examples:

- changed canonical facts update snapshot inputs;
- unchanged canonical facts do not invalidate queries;
- empty canonical facts clear stale inputs;
- compatibility projections are not written.

This keeps incremental revalidation honest: Salsa tracks dependencies, domains
track meaning.

## Boundary Invariants

Every boundary in the dataflow should have a small invariant that can be tested
or reviewed directly.

### Graph Boundary

Invariant:

```text
Graph identity changes only when syntax/binding identity changes.
```

This boundary may cache syntax summaries. It may not depend on interproc facts,
solved flow state, or expected call types.

### IR Boundary

Invariant:

```text
Checker IR contains operations, not answers.
```

The IR may say "apply this call effect" or "assign this value to this
location". It may not pre-decide the result type of an operation whose answer
depends on flow/interproc state.

### Transfer Boundary

Invariant:

```text
Transfer is the only state-writing semantics inside a function.
```

All writes to memory, value, relation, effect, and termination state must be
visible as transfer operations. A helper that writes state outside transfer is a
hidden interpreter.

### Join Boundary

Invariant:

```text
Branch merge uses domain Join and nothing else.
```

A branch-specific merge helper is allowed only if it is the domain's exported
join/meet/refine operation. If it knows about AST shape, it is in the wrong
layer.

### Widen Boundary

Invariant:

```text
Widen happens only at named recursive boundaries.
```

Loop widening, local function SCC widening, and interproc widening may have
different schedules, but they must call the same domain-level widening laws for
the same fact family.

### Query Boundary

Invariant:

```text
Query answers cannot become stored evidence.
```

Query caches are permitted only for answers. They must not publish facts or
change future convergence.

### Publication Boundary

Invariant:

```text
Publication emits immutable deltas and never merges them.
```

The same solved state must always produce the same delta. If publication reads
previous facts to decide how to shape the delta, it is doing merge work in the
wrong layer.

### Snapshot Boundary

Invariant:

```text
Snapshot updates are semantic no-ops except for dependency invalidation.
```

Setting a snapshot input can make queries rerun. It cannot normalize, widen,
infer, or delete evidence except by reflecting the already-canonical facts.

### Diagnostic Boundary

Invariant:

```text
Diagnostics observe proof failure; they do not define type behavior.
```

A diagnostic pass may ask why a check failed. It may not make the check pass or
fail by changing evidence.

## Evidence Authority Model

The checker should be precise because it carries proof, not because it guesses.
Authority is therefore part of evidence. It is not a global total order; it is a
domain-specific partial order over a specific question.

Canonical evidence shape:

```text
Evidence =
  Location
  + Value/Predicate/Effect
  + Provenance
  + Authority
  + Scope
  + Phase
  + SourceSpan
```

`SourceSpan` may be absent for synthetic or imported evidence, but provenance
must not be absent.

### Authority Classes

The final design should name these authority classes explicitly.

| Authority | Meaning | Can Prove Concrete Contract? | Can Be Weakened By Join? | Can Publish? |
|---|---|---|---|---|
| explicit contract | user/API annotation or manifest contract | yes | only through declared variance/summary abstraction | yes |
| hard runtime proof | guard, assertion, dominance-proven assignment | yes | yes at control-flow join | if it crosses boundary |
| relation proof | fact derived from tuple/path relation | yes for related locations | yes when relation path is lost | if relation crosses boundary |
| effect proof | applied call/effect summary | yes if effect declares it | yes at join/widen | yes as effect/summary |
| body obligation | function body requires a shape | yes for parameter contract inference | yes at recursive widen | yes |
| call observation | caller passed a shape | no by itself | yes | yes as weak evidence |
| contextual literal evidence | expected type applied at literal boundary | yes for that literal | yes | yes if literal escapes |
| soft annotation | low-authority annotation hint | no without compatible proof | yes | only as soft evidence |
| unresolved observation | `unknown` | no | yes but not erased silently | yes as unknown |
| dynamic top | `any` | no without explicit cast/contract | yes as dynamic top | yes as any |

This table prevents the common mistake of treating all useful evidence as the
same. A call observation is useful for inference, but it is not proof that the
callee accepts that shape. An explicit `any` is useful information, but it is
not proof of a concrete field.

### Conflict Resolution

Conflicts should be resolved by the owning domain, not by producer preference.

| Conflict | Owner | Correct Resolution |
|---|---|---|
| hard proof vs soft annotation | evidence/value domain | hard proof wins for the proven path |
| explicit `any` vs expected concrete param | assignability/value domain | reject unless cast/contract proves concrete |
| unknown return vs concrete return | return domain | preserve unresolved behavior unless domain law proves refinement |
| call observation vs body obligation | parameter domain | body obligation is stronger contract evidence |
| parent table shape vs child-path write | memory domain | child-path fact wins for that path |
| closed missing field vs open row tail | value/memory domain | closed absence and open unknown tail stay distinct |
| relation proof vs unrelated assignment | relation/memory domain | relation survives only if location identity is preserved |
| widening precision loss vs later query | owning domain | query observes widened state; no post-widen repair |

Conflict policy must be testable as a domain law. If the test has to construct a
whole checker to decide the conflict, the domain boundary is still too implicit.

### Proof-Carrying Facts

Every persistent fact should be explainable as:

```text
fact = domain.accept(observation, provenance, authority, location)
```

Queries should be able to answer both:

- the abstract answer, such as "this value is string";
- the proof route, such as "truthy guard on this location removed nil".

The proof route does not need to be exposed in normal diagnostics, but it must
exist in the design. Without it, the checker cannot distinguish real precision
from accidental broadening.

### Precision And Soundness Contract

Precision can increase only by proof.

Allowed precision gains:

- guard removes nil/false from the exact guarded location;
- assertion effect narrows the declared target relation;
- body obligation records a parameter shape the body actually reads;
- table literal contextual typing applies at the literal boundary;
- relation summary narrows linked tuple slots after a predicate.

Forbidden precision gains:

- callee expected type rewrites caller evidence;
- repeated callers vote a parameter into a concrete contract;
- `any` becomes a concrete record because a later field is used;
- closed missing field becomes open unknown tail to avoid an error;
- cached answer is reused after an untracked dependency changed.

Precision can decrease only at named abstraction boundaries:

- branch join,
- loop widening,
- local function SCC widening,
- interproc widening,
- published summary abstraction.

Precision must not decrease at:

- equality,
- snapshot update,
- diagnostics,
- compatibility projection,
- query cache lookup.

This is the soundness/performance contract. Faster analysis is valid only if it
computes the same evidence or a documented domain approximation at a named
boundary.

### Absence Of Evidence

Absence is not a proof.

Rules:

- no field evidence does not mean field is nil;
- no relation evidence does not mean slots are independent if a relation was
  dropped by a bug;
- no return evidence does not mean zero returns unless arity is known;
- no effect evidence does not mean pure call unless the effect row is closed;
- no param evidence does not mean `any`; it means unresolved until declared or
  inferred evidence exists.

This is where many false positives and false negatives start. The final domains
should model absence explicitly instead of using nil maps as semantic answers.

## Dataflow Proof Traces

Every important inference should have a trace format. This is not a logging
requirement for the first implementation. It is the mental model for proving the
checker did the right thing.

Trace skeleton:

```text
Observation
  -> Location
  -> Evidence
  -> Domain acceptance
  -> State fact
  -> Join/Widen if any
  -> Query answer
  -> Publication if any
```

### Guarded Field Trace

```text
Observation: if options.model then
Location:    Location(options).field("model")
Evidence:    truthy predicate, hard runtime proof
Domain:      RelationDomain + ValueDomain
State:       path excludes nil/false on true branch
Query:       provider.open argument reads non-nil field type
Publish:     none unless the relation escapes through a summary
```

Wrong trace:

```text
provider.open expects string -> options.model becomes string
```

The wrong trace reverses dataflow.

### Error Return Trace

```text
Observation: local value, err = f()
Location:    return tuple slots assigned to local locations
Evidence:    f publishes tuple relation
Domain:      RelationDomain accepts slot correlation
State:       err nil branch relates value slot to success case
Query:       value.field sees success-side value evidence
Publish:     wrapper republishes tuple relation only if slot identity is preserved
```

Wrong trace:

```text
function has two returns -> assume value/error convention
```

The wrong trace invents relation evidence from arity.

### Dynamic Payload Trace

```text
Observation: payload = json.decode(raw)
Location:    payload
Evidence:    imported dynamic value
Domain:      ValueDomain records any/unknown with provenance
State:       payload.name remains dynamic/unresolved
Query:       needs_string(payload.name) requires proof
Publish:     dynamic evidence only if exported
```

Wrong trace:

```text
needs_string expects string -> payload.name becomes string
```

The wrong trace treats expected type as evidence.

### Captured Mutation Trace

```text
Observation: nested function inserts into state.items
Location:    canonical location for state.items
Evidence:    mutation effect with captured provenance
Domain:      EffectDomain applies MemoryDomain mutation
State:       array element fact at state.items
Query:       ipairs reads element fact if dominance/escape permits it
Publish:     captured container mutation delta if it crosses function boundary
```

Wrong trace:

```text
captured mutation replay builds a new parent table shape
```

The wrong trace loses operator kind and child-path authority.

### Trace Review Rule

For any new inference, a reviewer should be able to ask:

- What was observed?
- What is the canonical location?
- What authority does the evidence have?
- Which domain accepted it?
- Where can it lose precision?
- Which query read it?
- Does it publish, and if so as which delta?
- Which cache boundary owns reuse?

If the answer starts with "this helper checks whether...", the design likely
needs another domain operation instead of another helper.

## Semantic Atoms

The final design should use a small shared vocabulary. These words should have
one meaning everywhere in the checker.

### Value

A `Value` is an abstract runtime Lua value.

It can be concrete, literal, structural, function-like, `nil`, `unknown`, or
`any`. It is not a source annotation and not a location. A value domain may say
how values combine; it may not decide where a value came from.

### Location

A `Location` is an abstract program place where evidence can attach.

Examples:

- symbol at SSA version,
- field path,
- index path,
- tuple slot,
- receiver slot,
- captured variable,
- return slot,
- graph/function identity.

Locations are canonical before transfer. AST paths and SSA paths cannot both be
authoritative.

## Location And Memory Calculus

The final checker needs one answer to the question:

```text
Are these two pieces of evidence about the same runtime place?
```

If that answer is local to each helper, precision will stay fragile. Guarded
fields, captured mutations, alias replay, tuple relations, and table-key
refinements all depend on the same location calculus.

### Location Shape

A location should be a canonical structured value, not a string path and not an
AST node.

```text
Location =
  Root
  + Version
  + PathSegments
  + ScopeIdentity
  + ProvenanceClass
```

Roots:

- local symbol root,
- parameter root,
- receiver `self` root,
- upvalue/captured root,
- return tuple root,
- temporary tuple result root,
- module export root,
- constructor instance root,
- external/imported value root.

Segments:

- named field,
- literal index,
- dynamic index with key evidence,
- array element,
- map value,
- tuple slot,
- metatable/member access when modeled,
- synthetic effect target.

`Version` belongs to the root or to a versioned location identity. It should not
be smuggled into a string suffix. `ScopeIdentity` is required for parent-scoped
facts so two equal-looking symbols in different parent scopes do not collide.

### Canonicalization Laws

Location canonicalization should obey these laws:

- resolving the same symbol/path at the same CFG point returns the same
  canonical location;
- resolving different lexical symbols never collides, even when names match;
- aliases are explicit equivalence/forwarding facts, not path rewrites;
- field and index segments are interned/normalized before storage;
- dynamic index evidence is preserved and not collapsed to `string` unless a
  proof refines it;
- tuple slots remain tuple slots until assignment or forwarding gives them a
  concrete destination;
- captured locations retain lexical owner identity;
- module/export locations retain module identity;
- open row-tail access and closed missing-field access produce different
  locations/evidence.

These laws should be tested without a whole checker. A location unit test should
be able to prove whether two references alias, differ, or are unknown.

### Memory State Shape

Memory state should be the product of several maps with one owner:

```text
MemoryState =
  ValueAt(Location)
  + PresenceAt(Location)
  + Children(Location)
  + AliasFacts
  + MutationLog
  + DominanceFacts
  + EscapeFacts
```

`ValueAt` says what value evidence is known at a location.
`PresenceAt` distinguishes present, absent, nil value, unknown presence, and
open row-tail unknown.
`Children` records known child facts without forcing a parent table rewrite.
`AliasFacts` records location identity relations and their dominance.
`MutationLog` records effectful writes with operator kind.
`DominanceFacts` tells whether a write/guard reaches a query point.
`EscapeFacts` tells whether a local fact can publish across a boundary.

None of these should be represented by "map missing means nil". Absence of a map
entry means no stored fact for that component.

### Read Law

A memory read answers by ordered evidence, not by helper preference.

Read order for a path should be:

1. exact dominated location fact;
2. exact relation-refined fact for the same location;
3. exact child-path mutation fact;
4. alias-forwarded fact whose alias is valid at the query point;
5. declared/constructed parent shape projected through the path;
6. open row-tail evidence;
7. unresolved evidence.

Forbidden read behavior:

- expected callee type becomes read evidence;
- parent table shape overwrites explicit child mutation;
- closed missing field becomes open row-tail unknown;
- dynamic index write broadens every named field without proof;
- stale query cache answers for a different location version.

This read law is where many current helper clusters should collapse.

### Write And Mutation Law

A write is not just "join this type into a table".

Write shape:

```text
Write =
  Target Location
  + OperatorKind
  + ValueEvidence
  + Dominance
  + Provenance
```

Operator kinds:

- assignment,
- field write,
- nil overwrite,
- deletion/absence write if Lua semantics or API effect establishes deletion,
- dynamic index write,
- array element insert,
- map value update,
- container send/receive,
- captured mutation replay.

The operator kind is semantic. `table.insert(x, v)`, `x[k] = v`, and
`x.field = v` may all affect a table, but they do not have the same path law.
Captured replay must preserve the original operator kind.

### Alias And Dominance Law

Alias facts are valid only over a control-flow region.

Rules:

- alias created by assignment is valid until reassignment or invalidating
  mutation;
- field alias preserves the exact field path it came from;
- dynamic index alias preserves key evidence;
- branch-local alias facts do not leak unless dominance proves they reach the
  query point;
- loop-carried aliases widen at the loop boundary;
- captured aliases include lexical owner and escape information.

Relation facts must reference canonical locations, not syntactic expressions.
If assignment preserves location identity, relations can transfer. If it copies
only a value and loses tuple/path identity, relation facts must not silently
survive.

### Tuple Slot Law

Tuple slots are locations, not just positions in a slice.

Rules:

- return arity is part of tuple identity;
- nil padding is explicit;
- wrapper forwarding preserves tuple-slot relation only when forwarding is
  identity-preserving;
- assignment from tuple slot to local location records a relation edge from slot
  to local;
- swapped or reordered returns update relation mapping explicitly;
- vararg expansion has its own location/evidence policy and cannot be treated
  as fixed tuple identity without proof.

This prevents the `(value, err)` convention from becoming an arity heuristic.

### Presence Law

Presence is separate from value type.

States:

- present with value evidence,
- present with nil value,
- absent from closed structure,
- optional in declared structure,
- unknown via open row tail,
- unknown via dynamic table top.

Important distinctions:

- `field = nil` is not automatically the same as absent unless the domain rule
  for that context says so;
- optional declared field is not the same as proven absence;
- open record tail gives unknown evidence, not nil evidence;
- map value may be nil even when key presence is unknown;
- table top preserves that a value is table-like without proving named fields.

Presence should be tested as its own domain law. It is too important to hide in
record subtyping or field lookup helpers.

### Publication Law

Only memory facts that escape the local function become interproc deltas.

Publishable memory evidence:

- captured variable type,
- captured field assignment,
- captured container mutation,
- constructor field,
- return value/tuple slot,
- parameter obligation/effect,
- module export field.

Non-publishable memory evidence:

- branch-local narrowing,
- local alias that does not escape,
- temporary tuple slot after assignment unless relation summary requires it,
- diagnostic-only failure evidence,
- query cache answer.

Publication should project from memory state. It should not reconstruct memory
facts by rescanning AST or replaying helper-specific summaries.

### Performance Consequences

The location calculus is also a performance boundary.

Expected wins:

- interned locations make map keys cheap and stable;
- path parsing disappears from hot query paths;
- child-path facts avoid rebuilding whole parent tables;
- alias and dominance checks become graph-indexed facts;
- relation queries compare location IDs instead of syntactic paths;
- captured mutation replay reuses the same mutation operator.

Rejected performance shapes:

- stringifying paths to compare them in hot loops;
- reparsing path suffixes during every narrowed query;
- rebuilding parent records for each child write;
- using object pools before ownership of locations and memory facts is proven;
- caching read answers without a solved-state/location-version key.

### Location Law Tests

The flash migration should add focused tests for:

- same expression at same point resolves to same location;
- same name in different scopes resolves to different locations;
- alias validity ends at reassignment;
- branch-local alias does not leak;
- dynamic index write does not overwrite unrelated named field;
- child field write outranks parent shape at that child;
- closed missing field differs from open row-tail field;
- tuple relation survives identity forwarding;
- tuple relation dies on reorder unless remapped;
- captured mutation preserves operator kind and target location;
- nil value and absence remain distinguishable.

These tests are foundational. If they pass, many higher-level inference tests
become much simpler because they no longer need to encode location policy.

### Evidence

`Evidence` is a value plus provenance and authority.

Examples:

- explicit annotation,
- hard runtime proof,
- body obligation,
- call observation,
- soft annotation,
- unresolved observation,
- imported dynamic value.

Evidence is not automatically truth. Domains decide how evidence combines.

### Fact

A `Fact` is evidence that has been accepted into a domain state.

Facts are persistent inside `AbstractState` or inside an immutable
`InterprocDelta`. Raw observations are not facts until transfer/domain logic
accepts them.

### Constraint

A `Constraint` restricts possible facts along a control-flow path.

Examples:

- truthy/falsy,
- type test,
- nil/non-nil,
- has-field,
- numeric bound,
- relation branch.

Constraints do not mutate storage by themselves. Transfer applies them to
`AbstractState`; queries read the result.

### Relation

A `Relation` connects multiple locations.

Examples:

- return slot 1 being nil implies return slot 0 is non-nil,
- assertion on one symbol narrows a sibling path,
- method receiver relation to `self`,
- callback argument relation to caller state.

Relations are not encoded as special value types. They are first-class domain
facts.

### Effect

An `Effect` describes what execution of a call or instruction can do.

Examples:

- mutate memory,
- terminate,
- refine an argument,
- produce a tuple relation,
- call a callback,
- collect keys.

Effects are applied by transfer. They do not rewrite types directly.

## Relation And Effect Calculus

Relations and effects are the bridge between local flow precision and
interprocedural power. They must be first-class domain facts, not names of known
functions.

Core rule:

```text
Relations describe conditional truth between locations.
Effects describe state transitions caused by execution.
```

An assertion, predicate, table mutator, callback, error-return convention, and
terminating function all fit this rule.

### Relation Shape

A relation should be represented as a structured fact:

```text
Relation =
  RelationID
  + Participants
  + Arms
  + Directionality
  + Validity
  + Provenance
```

Participants are canonical locations:

- tuple slots,
- locals,
- fields,
- indexes,
- receiver/self,
- callback arguments,
- captured paths.

Arms describe conditional cases:

- success/failure branch,
- true/false predicate branch,
- nil/non-nil branch,
- type-test branch,
- discriminant branch,
- custom effect branch.

Directionality matters. Some relations are bidirectional; many are not. For
example, `err == nil` may imply success-side value evidence, but using a value
does not necessarily prove `err == nil` unless the relation declares that
reverse implication.

Validity records when the relation is safe to apply:

- CFG region,
- dominance/post-dominance requirement,
- location identity requirement,
- alias validity,
- tuple-slot identity,
- function summary boundary,
- effect precondition.

### Relation Operations

The relation domain should own these operations:

```text
Attach(relation, state)
Assume(location predicate, state)
Remap(relation, location mapping)
Project(location, state)
Join(a, b)
Widen(prev, next)
Publish(relation, boundary)
```

`Attach` stores a relation after validating participants.
`Assume` applies a branch predicate and derives consequences.
`Remap` preserves a relation through assignment, wrapper forwarding, or tuple
reordering only when identity mapping is explicit.
`Project` answers what a relation proves about a queried location.
`Join` keeps only facts valid on all incoming paths or marks path-conditional
arms explicitly.
`Widen` bounds recursive relation growth.
`Publish` emits only relations that remain meaningful across the boundary.

Forbidden relation operations:

- infer relation from return arity alone;
- preserve relation after assignment without location mapping;
- treat a predicate function name as proof outside effect transfer;
- erase relation provenance during join;
- encode relation as a special `typ.Type`.

### Tuple Relation Law

The `(value, err)` convention is one tuple relation instance:

```text
SuccessArm: err is nil     -> value is success value
FailureArm: err is non-nil -> value is nil/unknown failure value
```

It is not:

- any two-return function,
- any call followed by `test.is_nil`,
- a special return-summary vector,
- a call-checking hack.

Custom error records, boolean-success APIs, result objects, and status-code
APIs should be expressible by defining different relation arms over locations.

### Predicate Relation Law

Predicate/assertion functions apply relations through effects.

Examples:

- `is_nil(x)` proves nil/non-nil branches for `x`;
- `is_string(x)` proves string/non-string branches for `x`;
- `assert_type(x, "string")` refines `x` or terminates;
- `has_field(x, "name")` proves presence for `x.name`;
- custom manifest predicate proves declared relation arms.

The function name is only a lookup key for an effect summary. The effect summary
is the semantic object.

Wrong shape:

```text
if call name == "test.is_nil" then patch value type
```

Correct shape:

```text
call -> effect summary -> relation transfer -> query
```

### Effect Shape

An effect summary should be a structured transition:

```text
Effect =
  EffectID
  + Preconditions
  + MemoryEffects
  + RelationEffects
  + ValueEffects
  + TerminationEffect
  + CallbackEffects
  + PublicationPolicy
  + Provenance
```

Preconditions decide when the effect is valid.
Memory effects mutate locations through `MemoryDomain`.
Relation effects attach or assume relations through `RelationDomain`.
Value effects refine or produce value evidence through `ValueDomain`.
Termination effects update reachability through `TerminationDomain`.
Callback effects describe higher-order execution.
Publication policy decides whether the summary can cross a function/module
boundary.

### Effect Application Law

Applying an effect is transfer:

```text
Call instruction
  -> resolve callee/effect summary
  -> instantiate summary with actual argument/receiver/return locations
  -> validate preconditions
  -> apply memory effects
  -> apply relation effects
  -> apply value effects
  -> apply termination effects
  -> schedule callback effects if invoked
```

Every sub-step calls the owning domain. The effect domain coordinates; it does
not own memory, value, relation, or termination laws.

### Callback Effect Law

Callbacks are effectful calls whose callee is a parameter or field.

Rules:

- callback invocation has its own call site and locations;
- callback argument evidence flows as call observations;
- callback return/effect evidence flows back only through declared callback
  summary;
- captured caller memory can be mutated only through explicit captured location
  effects;
- unknown callback effects are not pure unless the effect row is closed.

This prevents higher-order code from becoming a blind spot or a source of
unsound broadening.

### Termination Law

Termination is an effect, not a diagnostic side channel.

Examples:

- `error()` terminates the current path;
- assertion failure terminates one branch;
- `return` terminates the current function path;
- infinite loop may terminate analysis reachability differently from runtime
  non-return depending on proof.

Reachability must update before value queries observe post-call state. Otherwise
the checker can report false positives from impossible paths or accept values
from dead branches.

### Open And Closed Effect Rows

Effects need the same open/closed discipline as structural types.

Closed effect row:

```text
This call has exactly these modeled effects.
```

Open effect row:

```text
This call has at least these effects; unknown effects may remain.
```

Rules:

- no effect summary does not mean pure call;
- closed pure summary can prove no mutation/termination/refinement;
- open summary cannot prove absence of unknown mutation;
- unknown external call must not refine values without a declared effect;
- manifest effects are typed inputs, not hardcoded behavior.

### Relation/Effect Join And Widen

Join:

- keeps relations/effects valid on all joined paths;
- preserves path-conditional arms when the domain represents them explicitly;
- drops or weakens facts whose participant locations are no longer identical;
- never converts absence of relation into proof of independence.

Widen:

- bounds recursive relation chains;
- bounds callback/effect expansion;
- bounds recursive captured mutation growth;
- preserves sound top/unknown effects when precision is lost.

Precision loss here must be visible as domain widening, not hidden in query or
publication.

### Publication Law

Publishable relations/effects:

- function return tuple relation,
- predicate/assertion function relation summary,
- captured memory mutation effect,
- callback invocation effect,
- termination/non-returning effect,
- external manifest effect,
- constructor/receiver mutation effect.

Non-publishable relations/effects:

- branch-local guard that does not escape;
- local assertion proof after the checked value dies;
- relation over temporary tuple slots unless remapped to exported locations;
- query-only refinement;
- diagnostic-only proof.

Publication should remap local locations to boundary locations. If a relation or
effect cannot be remapped, it does not publish.

### Performance Consequences

The relation/effect calculus should improve performance by making reuse
structural.

Expected wins:

- relation queries index by participant location;
- effect summaries are cached by callee identity and manifest/source version;
- effect instantiation is local and cheap because locations are canonical;
- callback expansion is bounded by summary widening;
- wrapper forwarding remaps relation IDs instead of resynthesizing return
  behavior;
- predicate handling uses one transfer path.

Rejected shapes:

- scanning all relations for every type query;
- recomputing effect summaries inside every call check;
- using string function names in hot semantic paths;
- replaying captured mutations by rebuilding table types;
- preserving all recursive callback effects without widening;
- clearing false positives by treating unknown effects as pure.

### Relation And Effect Law Tests

The flash migration should add focused tests for:

- tuple relation attaches only from declared summary, not arity;
- tuple relation survives identity wrapper forwarding;
- tuple relation remaps through swapped returns only with explicit mapping;
- predicate effect narrows only declared participants;
- assertion termination removes impossible paths before value query;
- unknown external call does not refine argument;
- closed pure effect proves no mutation;
- open effect row does not prove no mutation;
- callback call observation reaches callback parameter evidence;
- callback unknown effects do not mutate closed state without declaration;
- captured mutation effect preserves operator kind and target location;
- relation join does not invent independence;
- recursive relation/effect widening converges without erasing all useful proof.

## Function Boundary Summary Calculus

A function boundary is where local abstract state becomes reusable evidence for
callers. This boundary must have one product-domain object. It should not be
spread across parameter evidence, return summaries, narrow summaries, function
types, captured fields, captured containers, literal signatures, and effect
maps as independent authorities.

Core rule:

```text
FunctionSummary = abstraction(QueryView(SolvedState), BoundaryMap)
```

The summary is not a second analysis. It is a deterministic abstraction of the
solved state through the function boundary.

### Boundary Map

The boundary map explains how local locations become external locations.

```text
BoundaryMap =
  Parameters
  + Receiver
  + Returns
  + Captures
  + Exports
  + Constructors
  + CallbackSlots
```

Examples:

- parameter location maps to parameter slot;
- receiver `self` maps to receiver slot;
- local return tuple slots map to return slots;
- captured upvalue paths map to captured locations;
- module fields map to export locations;
- constructor writes map to constructor instance fields;
- callback parameters map to callback function slots.

Any summary fact that cannot be expressed through the boundary map is not
publishable. It remains local evidence.

### Summary Product

The canonical function summary should be a product:

```text
FunctionSummary =
  SignatureSurface
  x ParameterEvidence
  x ReturnTupleSummary
  x RelationSummary
  x EffectSummary
  x CaptureSummary
  x ConstructorSummary
  x ExportSummary
```

`SignatureSurface` is the user-facing callable type projection. It is derived
from the product. It is not the stored authority.

`ParameterEvidence` records annotations, body obligations, call observations,
soft evidence, contextual literal evidence, and recursive widening state.

`ReturnTupleSummary` records explicit arity, nil padding, unknown slots, any
slots, multivalue expansion policy, and per-slot provenance.

`RelationSummary` records tuple/path relations that survive the boundary map.

`EffectSummary` records memory, relation, value, termination, and callback
effects that callers must apply through transfer.

`CaptureSummary` records captured value/path/mutation evidence that escaped the
function body.

`ConstructorSummary` records constructor field facts only when construction
semantics prove them.

`ExportSummary` records module-visible fields and functions.

### Parameter Summary Law

Parameters have several evidence sources, but one domain.

Evidence sources:

- explicit parameter annotation,
- manifest/API contract,
- body obligation,
- call observation,
- function literal expected type,
- soft annotation,
- recursive SCC seed,
- interproc snapshot.

Merge policy:

- explicit contracts define the checked surface;
- body obligations can infer required structure;
- call observations are weak evidence and cannot create a hard contract alone;
- soft evidence refines only when compatible proof exists;
- recursive evidence widens only at SCC/interproc boundaries;
- optionality and nilability are separate axes;
- `any` remains dynamic top unless explicit cast/contract changes the question;
- absence of parameter evidence is unresolved, not `any`.

Wrong shape:

```text
ParamHints merge differently from FunctionFacts.Params
```

Correct shape:

```text
ParameterEvidenceDomain.Join(existing, candidate)
```

### Return Summary Law

Returns are tuples with attached relations and effects.

Rules:

- arity is explicit;
- nil padding is explicit;
- zero returns differ from one nil return;
- unknown return evidence is not bottom;
- any return evidence remains dynamic top;
- recursive return growth widens at the return domain boundary;
- narrow/success returns are derived views over tuple relation state;
- wrapper forwarding preserves return relations only through explicit location
  remapping;
- vararg return expansion has a distinct summary policy.

Wrong shape:

```text
ReturnSummaries and NarrowReturns are stored as separate truths
```

Correct shape:

```text
ReturnTupleSummary + RelationSummary -> projected narrow/success view
```

### Function Type Projection Law

A function type is a projection, not an authority.

Projection:

```text
FunctionType =
  params(ParameterEvidence)
  + returns(ReturnTupleSummary)
  + effects(EffectSummary)
  + relation metadata if the surface type can carry it
```

Rules:

- projection is deterministic and cacheable;
- projection does not write facts;
- projection does not reconcile legacy channels;
- projection must be invalidated by changes to the canonical summary product;
- two projections of the same summary must be equal.

This removes the need for bridge shapes such as "function types from facts" as a
semantic layer. A projection function may exist as a read-only view, but it is
not a merge or fallback path.

### Capture Summary Law

Captures are memory/effect facts remapped through lexical ownership.

Publishable capture facts:

- captured variable value evidence;
- captured field write;
- captured nil overwrite/deletion when modeled;
- captured table/container mutation;
- captured relation over exported/captured locations;
- captured callback effect.

Rules:

- captured paths use canonical locations with lexical owner identity;
- mutation operator kind is preserved;
- dominance/escape controls whether the mutation publishes;
- parent-derived table shape cannot overwrite child captured mutation;
- captured facts are applied by transfer in the receiving context, not by
  rebuilding parent table types.

### Constructor And Export Summary Law

Constructor and export facts are boundary memory facts.

Rules:

- constructor fields are published only from construction evidence;
- module export fields are published only from export locations;
- local helper facts do not publish just because the name is visible;
- exported functions publish their function summary product;
- imports read snapshots and apply summaries through transfer/query, not through
  local special cases.

### Call Application Law

Calling a function applies its summary to actual locations.

```text
CallSite
  + FunctionSummary
  + ActualArgumentLocations
  + ReturnDestinationLocations
  -> Transfer over AbstractState
```

Application steps:

1. check actuals against projected parameter contracts;
2. record call observations as weak parameter evidence;
3. instantiate effect summary over actual locations;
4. instantiate relation summary over return and argument locations;
5. bind return tuple summary to destination locations;
6. update termination/reachability;
7. publish caller-side deltas only after the caller solves.

Forbidden:

- expected parameter type rewrites actual evidence;
- callee summary mutates interproc store during call checking;
- caller synthesizes a new callee summary from local expectations;
- return arity heuristic creates relation summary;
- call application bypasses transfer.

### Summary Join And Widen

Function summaries combine through their domains.

Join:

- combines independent observations within one iteration;
- preserves provenance and authority;
- keeps tuple arity explicit;
- joins relations/effects only when participant remapping is compatible;
- avoids rebuilding equivalent maps or slices on no-op joins.

Widen:

- applies at local function SCC and interproc boundaries;
- bounds recursive parameter, return, capture, relation, and effect growth;
- preserves sound unknown/any distinction;
- emits precision-loss evidence for diagnostics/profiling;
- never hides convergence by equality-time normalization.

Leq/Equal:

- compare canonical summary state only;
- do not rebuild projections;
- do not normalize as repair;
- are the basis for fixpoint convergence and snapshot invalidation.

### Summary Storage Law

The stored authority should be one canonical product.

Allowed stored authority:

```text
FunctionSummary product
```

Allowed derived views:

- callable `typ.Function` surface;
- display signature;
- backward-compatible API response if needed outside production semantics;
- narrow/success return projection;
- parameter hint projection for UI/debugging.

Forbidden stored authority:

- parameter evidence as separate merge truth;
- return summaries as separate merge truth;
- narrow returns as separate merge truth;
- function type cache as separate merge truth;
- captured mutation helper summaries with custom merge;
- legacy compatibility view written back into facts.

The final flash migration should delete duplicate stored channels in the same
change that introduces the canonical product.

### Performance Consequences

The boundary summary calculus should make interproc faster because summaries
become smaller and more stable.

Expected wins:

- one summary hash/equality path instead of multiple channel comparisons;
- no function-type projection during convergence unless a caller asks for it;
- no return narrow projection during convergence unless a query asks for it;
- no-op joins can reuse previous summary components;
- snapshot inputs update only changed canonical summaries;
- wrapper forwarding remaps summaries instead of resynthesizing them;
- parameter-use graph summaries feed parameter evidence without AST rescans.

Rejected shapes:

- rebuilding all derived views on every merge;
- writing projections back into canonical facts;
- comparing function summaries by formatting types;
- widening by dropping entire summary families;
- adding iteration caps instead of domain widening;
- clearing caches manually to repair stale summary dependencies.

### Function Boundary Law Tests

The flash migration should add focused tests for:

- function type projection is deterministic from the same summary;
- parameter body obligation outranks call observation;
- call observation alone does not prove concrete callee contract;
- explicit `any` parameter does not become concrete from calls;
- zero returns differ from one nil return;
- unknown return survives merge with concrete return when unresolved;
- narrow/success return is derived from relation summary;
- wrapper forwarding preserves relation through explicit remap;
- captured field write and captured container mutation use same memory law;
- constructor field publishes only from constructor evidence;
- export summary does not include non-escaping locals;
- no-op summary join preserves equality and avoids snapshot rewrite;
- recursive function summary widens and converges without erasing all relation
  proof.

### Delta

A `Delta` is a completed analysis contribution to another scope or iteration.

Examples:

- function fact delta,
- parameter evidence delta,
- captured mutation delta,
- constructor field delta,
- relation summary delta.

Deltas are immutable. The store never lets a producer mutate canonical state in
place.

### Snapshot

A `Snapshot` is the immutable state observed by a query.

Snapshots are cache inputs. If a snapshot changes, dependent queries must
revalidate through Salsa or an explicitly documented cache invalidation rule.

## Canonical Dataflow Contract

The final dataflow should have explicit boundary objects.

```text
Source
  -> GraphBundle
  -> CheckerIR
  -> TransferProgram
  -> AbstractState
  -> QueryView
  -> FunctionResult
  -> InterprocDelta
  -> FactsDomain
  -> SnapshotInputs
```

### GraphBundle

Owns:

- AST function body,
- CFG,
- symbol table,
- parent scope identity,
- dominance/post-dominance indexes,
- local function indexes,
- parameter-use summaries.

It is immutable after construction. Anything expensive and graph-derived should
be cached here or through a Salsa query keyed by graph identity.

### CheckerIR

Owns the normalized checker program:

- declarations,
- assignments,
- branch predicates,
- calls,
- returns,
- table constructors,
- field/index writes,
- mutation effects,
- termination effects.

It should be AST-free except for source spans and stable graph references. This
is where the checker stops being syntax-driven and becomes analysis-driven.

### TransferProgram

Owns executable transfer instructions over `AbstractState`.

Examples:

```text
Assign(Location, ValueExpr)
Assume(Condition)
Mutate(Mutation)
Call(CallSite)
Return(ReturnTuple)
Terminate(Reason)
```

Every statement-level fact should enter the solver through an instruction like
this. A table insert, captured mutation replay, field assignment, and dynamic
index write should not each invent their own path rules.

### AbstractState

Owns the whole product:

```text
AbstractState =
  MemoryState
  x ValueFacts
  x NumericFacts
  x ShapeFacts
  x RelationFacts
  x EffectFacts
  x TerminationFacts
```

This must be the persistent state of the intraprocedural solver. Query-time
`ProductDomain` construction should be replaced by reading this product, or by
creating a cheap view over it. The state product is the source of truth.

### QueryView

Owns read-only answers:

- type at point,
- narrowed path type,
- field/index presence,
- tuple relation at call site,
- constant/numeric facts,
- reachability.

It cannot write facts. It cannot perform fresh synthesis that changes the
answer independently from `AbstractState`.

### InterprocDelta

Owns facts emitted by a completed function analysis:

- function fact,
- parameter evidence,
- literal signatures,
- captured field mutations,
- captured container/table mutations,
- constructor fields,
- relation summaries.

The delta is immutable. The store combines it through `FactsDomain` only.

## Evidence Lifecycle

Every fact in the checker should have a visible lifecycle:

```text
Observed -> Located -> Qualified -> Transferred -> Joined -> Widened -> Queried -> Published
```

### Observed

Evidence starts from one of a small number of sources:

- source annotation,
- literal syntax,
- assignment,
- guard/predicate/assertion,
- call argument,
- call return,
- effect spec,
- table/container mutation,
- imported manifest,
- previous interproc snapshot.

Observation records provenance. It does not decide final authority.

### Located

Every observation must attach to a location:

- symbol,
- field path,
- index path,
- tuple slot,
- function graph,
- parent scope,
- call site,
- return site.

Location must be canonical before the evidence enters transfer. This prevents
one helper using AST paths while another uses SSA path keys.

### Qualified

The evidence is tagged with its authority:

```text
explicit annotation > hard proof > body obligation > call observation >
soft annotation > unresolved evidence
```

`any` is not "very strong evidence." It is dynamic top. `unknown` is not "safe
to ignore." It is unresolved evidence. These two facts must remain distinct in
every domain.

The authority order is partial, not a simple global priority. For example:

- explicit annotation dominates inferred shape for assignment checking;
- hard branch proof dominates soft annotation for narrowing;
- body obligation dominates call observation for parameter contracts;
- explicit `any` remains dynamic top and does not become concrete because a
  later call expects concrete;
- unresolved `unknown` can be refined by proof, but cannot be silently replaced
  by unrelated precision.

This should become an explicit `EvidenceOrder`, not a set of local `if`
statements.

### Transferred

Transfer applies evidence to the current `AbstractState`.

Examples:

- assignment writes memory/value facts,
- guard writes relation and shape facts,
- call writes return tuple and effect facts,
- table insert writes a mutation fact,
- error-return check reads a tuple relation and narrows linked slots.

Transfer does not call interproc merge functions. Transfer does not widen.

### Joined

Control-flow joins combine same-phase predecessor states through domain `Join`.
This is where branch evidence meets.

Branch joins must preserve runtime alternatives. For Lua, `x or y` and `x and y`
return actual operand values, so the value domain cannot prune a live branch just
because the other branch is more precise.

### Widened

Widening is allowed only at named recursive boundaries:

- loop fixpoint,
- local function SCC,
- interprocedural fixpoint,
- recursive type/shape growth boundary.

Widening must be visible in code. If a helper "prefers" one side to force
stability, it is a widening rule and belongs to the domain that owns that
cycle.

### Queried

Queries produce read-only views:

- type at point,
- narrowed path,
- field/index evidence,
- relation state,
- effect summary.

Queries cannot publish facts. If a query has to synthesize new evidence to
answer correctly, that evidence belongs in transfer or in a cached derived input
computed before solving.

### Published

Only completed function analysis publishes interproc deltas. Publication is a
data move:

```text
FunctionResult -> InterprocDelta -> FactsDomain.Join/Widen -> SnapshotInputs
```

Publication is not another inference pass.

## Required Domain API Shape

Every domain should expose the same conceptual operations even if Go uses
concrete types instead of generics everywhere.

```go
type Domain[T any] interface {
    Normalize(T) T
    Leq(a, b T) bool
    Join(a, b T) T
    Meet(a, b T) T
    Widen(prev, next T) T
}
```

Transfer is separate:

```go
type Transfer[I any, S any] interface {
    Apply(input I, state S) S
}
```

Query is separate:

```go
type Query[S any, Q any, A any] interface {
    Answer(state S, question Q) A
}
```

This separation is important:

- `Join` and `Widen` do not inspect AST.
- `Transfer` does not know interproc storage.
- `Query` does not mutate state.
- `Normalize` is explicit and not hidden in equality.

## Domain Invariant Ledger

Each domain needs invariants that can be tested independently from the full
checker. These are the invariants that should guide the flash migration.

### Value Domain Invariants

- `unknown` means unresolved evidence and must not be silently dropped at
  return, branch, table, or relation joins.
- `any` means dynamic top and must not satisfy concrete contracts without an
  explicit proof, guard, schema, or cast.
- `nil` is a Lua value; absent field is structural absence; optional field is a
  type-level allowance for absence/nil depending on context.
- soft evidence is lower authority than hard evidence, but `nil` alone does not
  erase a soft structured shape.
- open row-tail field access produces row-tail evidence; closed missing field
  does not.
- table top absorbs table-like precision only in domains where table-likeness is
  the intended upper bound, not as a general precision eraser.

### Memory Domain Invariants

- every fact has exactly one canonical location;
- child-path facts outrank parent-derived fallback evidence for the same path;
- alias replay preserves identity and dominance;
- mutation replay preserves operator kind;
- nil overwrite and field deletion are represented explicitly;
- branch-local mutation does not leak unless control-flow dominance proves it.

### Relation Domain Invariants

- tuple/path relations are first-class facts;
- relation facts survive assignment, wrapper forwarding, and module export only
  when slot/path identity is preserved;
- relation narrowing is bidirectional only when the relation declares it;
- a guard helper such as `is_nil` can apply a relation but cannot invent one.

### Effect Domain Invariants

- effects are summaries, not post-hoc type rewrites;
- effect application goes through transfer;
- captured effects preserve location, operator kind, and provenance;
- termination effects affect reachability before value queries;
- external contract effects are typed inputs, not hardcoded checker behavior.

### Parameter Evidence Invariants

- call observations are weaker than body obligations;
- body obligations are inferred only from actual body demand;
- source annotations remain authoritative;
- soft annotations can refine but not override hard proof;
- recursive parameter evidence widens at SCC/interproc boundaries only;
- function-fact params and parameter evidence use the same evidence order.

### Return Summary Invariants

- tuple arity is explicit;
- nil padding is explicit;
- unknown return evidence is not bottom;
- relation summaries travel with tuple summaries;
- recursive container growth has one widening policy;
- narrow summary is derived from solved flow facts, not a second stored truth.

### Interproc Facts Invariants

- producers emit immutable deltas;
- store merge uses `FactsDomain.Join`;
- fixpoint boundary uses `FactsDomain.Widen`;
- equality compares canonical state only;
- derived views are not write targets;
- snapshot inputs mirror canonical read state exactly.

## Domain Interaction Protocol

The product-domain design is only useful if packages interact through a small
set of verbs. These verbs are the mental model for the future implementation.

```text
Syntax/Graph -> Instruction -> Transfer -> Domain operation -> AbstractState
AbstractState -> Query -> Answer
FunctionResult -> InterprocDelta -> FactsDomain -> Snapshot
```

### Transfer

Transfer applies one semantic instruction to one abstract state.

Allowed:

- read the instruction payload,
- ask a domain for local semantic operations,
- produce a new abstract state.

Forbidden:

- scan unrelated AST,
- publish interproc facts,
- mutate Salsa inputs,
- call compatibility projections,
- repair old facts.

If a transfer needs a type meaning question, it asks `ValueDomain`. If it needs
a location question, it asks `MemoryDomain`. If it needs a correlation question,
it asks `RelationDomain`. It does not inline those laws.

### Domain Operation

A domain operation defines what a fact means and how it combines.

Allowed:

- normalize owned values,
- compare owned values,
- join owned values,
- meet or refine owned values,
- widen owned values,
- answer pure owned-domain predicates.

Forbidden:

- depend on source syntax,
- depend on checker phase order,
- allocate hidden facts in another domain,
- read mutable store state,
- perform query invalidation.

Domain operations must be deterministic and law-testable without constructing a
whole checker.

### Abstract State

`AbstractState` is the one mutable semantic product during analysis.

Allowed:

- hold domain components,
- combine components through their domains,
- expose read-only query views after solving.

Forbidden:

- keep shadow facts that duplicate domain-owned facts,
- hide a second mini solver,
- let equality normalize,
- let queries write analysis evidence.

### Query

Queries answer questions against solved state.

Allowed:

- read state,
- memoize performance-only answers keyed by immutable input,
- project final user-facing answers.

Forbidden:

- create new evidence,
- change convergence,
- backfill facts into the store,
- call `Join` or `Widen`.

If a query discovers that useful information is missing, the correct response is
to add a transfer/effect/domain fact that produces it before solving. The query
must not become a hidden analysis phase.

### Publication

Publication converts a solved function result into an immutable interproc delta.

Allowed:

- summarize returns,
- summarize parameter obligations,
- summarize captured effects,
- summarize relations,
- emit deltas.

Forbidden:

- merge deltas directly,
- reconcile legacy channels,
- mutate existing facts,
- apply caller-specific preferences.

The only writer of canonical interproc state is `FactsDomain`.

### Snapshot

Snapshotting wires canonical facts into Salsa inputs.

Allowed:

- copy canonical facts into inputs,
- invalidate dependent queries through Salsa dependency tracking.

Forbidden:

- normalize,
- widen,
- infer,
- drop fields for compatibility,
- reconstruct projections that were not canonical facts.

Snapshotting is cache plumbing. It is not part of type semantics.

## Layering And Import Rules

The final code should make illegal designs difficult to express. Package
dependencies should encode the semantic architecture.

### Domain Packages

Domain packages may import:

- low-level type structures,
- subtype/query primitives,
- small immutable domain-local helper packages.

Domain packages must not import:

- AST packages,
- flow builders,
- checker store,
- Salsa database handles,
- diagnostics emitters,
- compatibility view builders.

Reason: a domain is a pure algebra over facts. If it can see syntax or mutable
store state, local helper logic will grow back.

### Memory And Location Packages

Memory/location packages may import:

- symbol/location identity,
- type values needed to represent field and container facts,
- relation keys where tuple/path identity must be preserved.

They must not import:

- call checking,
- interproc store,
- return inference,
- diagnostics formatting.

Reason: every producer must use the same path identity rules. No producer should
construct its own equivalent of "field path", "tuple slot", or "receiver self".

### Transfer Packages

Transfer packages may import:

- normalized checker IR,
- abstract state,
- domain set,
- memory/location model.

They must not import:

- old fact bridges,
- compatibility projections,
- checker diagnostics as control flow,
- global interproc store mutation.

Reason: transfer is the executable abstract semantics for one instruction. It
can create deltas inside state, but publication happens later.

### Store And Pipeline Packages

Store/pipeline packages may import:

- domain interfaces,
- abstract interpreter engine,
- Salsa database handles,
- diagnostics/reporting.

They must not implement:

- truthiness laws,
- soft/hard evidence ordering,
- return tuple relation semantics,
- path dominance rules,
- recursive type widening.

Reason: orchestration controls when analysis runs. Domains control what analysis
means.

### Query Packages

Query packages may import:

- read-only solved state,
- Salsa query APIs,
- pure domain predicates used for answering.

They must not import:

- mutable transfer state,
- publication writers,
- domain normalization writers.

Reason: a query can be cached aggressively only when it is pure.

### Test Packages

Tests should mirror these boundaries:

- domain law tests construct only domain values,
- transfer tests build small IR fragments and inspect abstract state,
- solver tests check convergence and widening,
- replay tests validate production programs,
- negative tests prove that convenience broadening did not happen.

Tests that require a whole checker to prove a simple domain law are a signal
that the domain boundary is still too implicit.

## Dataflow Walkthroughs

### Guarded Field To Call Argument

Pattern:

```lua
if options.model then
    provider.open(options.model)
end
```

Correct dataflow:

1. `options.model` is observed as a field read.
2. The guard transfers a truthy relation for `Location(options, "model")`.
3. The call argument query reads that relation and answers `NonNil(modelType)`.
4. Parameter evidence records a call observation for the callee.
5. If the callee body requires `string`, body obligation and call observation
   combine in `ParameterEvidenceDomain`.

Wrong shape:

- special-case `options.model` in call checking,
- make all truthy fields strings,
- accept `any` as string.

### Table Insert To Later Iteration

Pattern:

```lua
table.insert(state.items, value)
for _, item in ipairs(state.items) do ... end
```

Correct dataflow:

1. `state.items` resolves to one memory location.
2. `table.insert` transfers a `MutationTableElement` to that location.
3. Memory join preserves the element fact at the exact child path.
4. `ipairs` queries the array element evidence from memory.

Wrong shape:

- replay captured table insert through generic container mutation,
- let parent table literal shape override explicit child-path evidence,
- infer element type from the loop variable without memory provenance.

### Error Return Correlation

Pattern:

```lua
local value, err = f()
test.is_nil(err)
value.field
```

Correct dataflow:

1. `f()` returns a tuple with a relation summary.
2. Assignment binds tuple slots to locations.
3. `test.is_nil(err)` transfers a relation constraint on the error slot.
4. Relation query narrows the linked value slot.
5. Field access reads the narrowed value slot.

Wrong shape:

- hardcode `test.is_nil` as a value-slot refinement,
- assume every two-return function is `(value, err)`,
- drop tuple relation when a wrapper forwards returns.

### Unknown External Payload

Pattern:

```lua
local payload = json.decode(raw)
needs_string(payload.name)
```

Correct dataflow:

1. `json.decode` returns dynamic/unresolved data.
2. `payload.name` is unresolved or `any` depending on API contract.
3. Passing it to `string` must fail unless a guard, schema, cast, or contract
   proves it.

Wrong shape:

- treat unknown external fields as strings because most callers expect strings,
- let table shape contextualization silently rewrite explicit `any`,
- clear global lint by broadening assignability.

## Inference Model

Inference is not a separate magical subsystem. It is the process of solving for
unknown slots in the product domain under the evidence produced by transfer.

The final model should distinguish these inference layers:

### Local Value Inference

Scope:

- local variables,
- field/index reads,
- table literals,
- expression results,
- branch-local values,
- loop-carried values.

Authority:

```text
AbstractState.ValueFacts + MemoryState + RelationFacts
```

Rules:

- local value inference reads declared types, transfer assignments, and
  constraints;
- it never writes interprocedural facts directly;
- it must preserve the distinction between `unknown` and `any`;
- table literal contextualization is a transfer/type-domain operation, not a
  one-off hook;
- logical `and`/`or` inference must preserve actual Lua branch values.

### Parameter Inference

Scope:

- call-site argument observations,
- body-derived obligations,
- source annotations,
- soft annotations,
- current function facts,
- function literal expectations.

Authority:

```text
ParameterEvidenceDomain
```

Rules:

- call-site observations are evidence, not contracts;
- body obligations are contracts only when the function body proves it requires
  that shape;
- explicit source annotations dominate inferred hints;
- soft annotations refine only when hard evidence proves the refinement;
- recursive parameter evidence must join/widen through the parameter domain.

There should be no separate ad hoc policy for "parameter evidence" versus "function
fact params". Both are parameter evidence with different provenance and merge
mode.

### Return Inference

Scope:

- return statements,
- tuple/multivalue expansion,
- nil padding,
- recursive return vectors,
- summary and narrow summary slots,
- wrapper forwarding.

Authority:

```text
ReturnSummaryDomain
RelationDomain
```

Rules:

- return arity is part of the tuple domain;
- `unknown` return evidence is unresolved runtime behavior, not bottom;
- recursive return vectors widen only at the SCC/fixpoint boundary;
- relation facts such as `(value, err)` attach to return tuples explicitly;
- wrapper forwarding propagates tuple and relation facts together.

### Function Type Inference

Scope:

- local function literals,
- method receiver `self`,
- higher-order callbacks,
- literal signatures,
- exported functions,
- imported module functions.

Authority:

```text
FunctionFactDomain
ParameterEvidenceDomain
ReturnSummaryDomain
RelationDomain
```

Rules:

- function type inference is a product of parameter evidence, return summary,
  and relation/effect summaries;
- a same-body function fact may seed analysis only through non-narrowing domain
  merge;
- higher-order signatures must use variance-aware merge rules;
- literal signatures are facts in the interproc product, not a second function
  authority.

### Effect Inference

Scope:

- built-in and manifest call effects,
- assertion/predicate refinements,
- table and container mutations,
- callback invocation effects,
- termination and non-returning calls,
- return tuple relation attachment,
- captured mutation summaries,
- external contract effects.

Authority:

```text
EffectDomain
MemoryDomain
RelationDomain
TerminationDomain
```

Rules:

- an effect is an abstract transfer summary, not a postflow patch;
- applying an effect must produce the same state change as inlining its
  corresponding transfer instructions would produce, up to the abstraction;
- effect summaries preserve target locations, tuple slots, operator kind,
  dominance, and provenance;
- effects that refine values must emit relation/value constraints through the
  owning domains;
- effects that mutate memory must emit memory mutations through the memory
  domain;
- effects that terminate execution must update reachability before any value
  query observes the post-call state;
- callback effects are higher-order summaries and must be applied at the call
  edge that invokes the callback, not at publication time;
- external effects are typed inputs to the domain, not hardcoded names in call
  checking.

Wrong effect inference shapes:

- "after this call, rewrite argument type" in call checking;
- "after this function, patch captured fields" in interproc merge;
- "if function name is `test.is_nil`, narrow slot" outside relation/effect
  transfer;
- "if table mutator is seen later, replay as generic container mutation";
- "if global harness fails, add a special accepted shape".

Correct effect inference shape:

```text
call instruction
  -> resolve effect summary
  -> EffectDomain.Apply(summary, state)
  -> MemoryDomain/RelationDomain/ValueDomain/TerminationDomain operations
  -> new AbstractState
```

Effect inference must be compositional. A user-defined wrapper around an effect
should publish the same kind of summary that the built-in effect uses, so callers
do not need wrapper-specific logic.

### Inference Soundness Boundary

The checker should infer every property that is proven by:

- source annotations,
- reachable transfer facts,
- memory/path identity,
- relation facts,
- effect summaries,
- interproc summaries,
- declared external contracts.

The checker must not infer a property from:

- the type expected by a later failing call,
- most callers preferring a shape,
- `any`,
- absent evidence,
- a compatibility projection,
- a cache hit whose input identity is incomplete.

This boundary is the core soundness rule:

```text
Expected type is a constraint to check against evidence.
It is not evidence unless a declared contract explicitly says so.
```

Contextual typing is still valid, but it must be represented as evidence with
provenance. For example, a table literal checked against an expected record can
receive contextual field types at the literal boundary. A dynamic payload flowing
through `any` cannot acquire those field types because a callee wanted them.

## Phase Responsibility Table

| Phase | May Create | May Combine | May Widen | May Query | Forbidden |
|---|---|---|---|---|---|
| Scope/CFG | graph identity, symbols | no type facts | no | no | type merge policy |
| Extract/IR | transfer instructions | no domain joins | no | declared-only queries | fixpoint repair |
| Flow solve | abstract state updates | domain joins at CFG joins | loop-local widening only if owned by flow domain | internal state reads | interproc fact writes |
| Narrow/query | read-only answers | no persistent joins | no | yes | producing facts |
| Return SCC | local return/param deltas | local domain joins | SCC widening through domain only | solved flow state | AST-specific merge laws |
| Interproc store | immutable deltas | `FactsDomain.Join` | `FactsDomain.Widen` | snapshot reads | producer-specific callbacks |
| Salsa | dependencies/cache | no semantic joins | no | query execution | hidden state mutation |

## Foundational Diagnosis

The checker has accumulated strong behavior before it acquired the right
vocabulary.

Current scattered concepts:

- value-type joins,
- return-slot joins,
- function-param fact joins,
- parameter-evidence joins,
- table-top absorption,
- soft-placeholder replacement,
- open-record row-tail merging,
- recursive structural-growth cutoffs,
- truthiness refinements,
- error-return tuple correlations,
- captured table/container mutation replay,
- path identity and alias identity,
- body-derived parameter contracts,
- call-site observations,
- signature projection to body use.

These concepts are real. The problem is not that they exist. The problem is that
they appear as local helpers in `returns`, `paramevidence`, `flow`, `synth`, and
`typ`, with overlapping responsibilities.

That creates the "guacamole" feeling: behavior is strong, but the mental model
is not visible at the package boundary.

## Canonical Product Domain

The final checker should have these explicit products.

### Value Domain

Owns pure type operations that are independent of checker phase:

- `NormalizeType`
- `JoinValue`
- `JoinReturnSlot`
- `Meet`
- `WidenShape`
- `Refines`
- `TruthinessRefinement`
- `Nilability`
- `SoftEvidence`
- open/closed record row-tail policy
- map/array/table-top classification

Candidate home:

```text
types/typ/domain
```

or, if it needs checker-only evidence policy:

```text
compiler/check/domain/value
```

Rule: domain-level predicates such as "candidate refines baseline by removing a
falsy table key" cannot live in `compiler/check/returns/join.go`.

### Memory And Path Domain

Owns the question "what program location does this fact describe?"

It must unify:

- `constraint.Path`
- CFG symbol/version identity
- SSA path keys
- field/index segments
- aliases
- dynamic index writes
- table mutator paths
- captured mutation paths
- field overlays

Candidate home:

```text
compiler/check/memory
```

The public model should be:

```go
type Location struct { ... }
type MemoryState struct { ... }
type Mutation struct { Kind, Target, Key, Value, Dominance }
```

Current scattered path helpers should collapse into this package. The solver
should not need to know whether a fact came from a table literal, field write,
alias replay, or captured mutation to apply the same path-law rules.

### Flow State Domain

Owns the persistent state of intraprocedural analysis.

The final `AbstractState` should be a product:

- memory facts,
- numeric facts,
- shape/presence facts,
- relation facts,
- termination facts,
- effect facts.

Candidate home:

```text
compiler/check/flowstate
```

or inside `types/flow` if it remains independent of checker-specific APIs.

Current weakness:

`types/flow.ProductDomain` is the closest modern abstraction, but it is mostly
used transiently during narrowing queries. The main solver still stores raw
maps and side caches. That split should disappear. Query-time narrowing should
read from the same abstract state product that transfer functions update.

### Relation Domain

Owns facts that connect multiple paths or tuple slots:

- error-return `(value, err)` correlation,
- sibling return-slot narrowing,
- predicate links,
- assertion links,
- type-test links,
- tuple-slot relation facts,
- custom error records.

Candidate home:

```text
compiler/check/domain/relation
```

This is where error-return convention should live. It should not be encoded as
scattered checks for exactly two return slots at call sites. The canonical
shape is a relation:

```go
type TupleRelation struct {
    Slots []SlotPredicate
}
```

The current `(value, err)` convention is then one predefined relation, not a
special checker behavior.

### Effect Domain

Owns facts about what a function or call can do:

- termination,
- error-return relation attachment,
- path refinements caused by assertions/predicates,
- table/container mutation effects,
- callback effects,
- key-collector effects,
- external contract effects.

Candidate home:

```text
compiler/check/domain/effect
```

Effect inference must be a normal abstract-interpretation output:

```text
CallSite + CalleeSummary + AbstractState -> EffectDelta
```

The effect delta is then applied by transfer or stored in function facts. It
must not be an after-the-fact patch that rewrites types without going through
the memory/relation/effect domains.

Effect summaries should be explicit:

```go
type EffectSummary struct {
    Mutations []memory.Mutation
    Relations []relation.TupleRelation
    Refinements []relation.PathRelation
    Terminates TerminationEffect
}
```

Current effects such as error-return correlation, captured container mutation,
and key-collector propagation become instances of this summary.

### Function Fact Domain

Owns all interprocedural facts about functions.

The stored authority remains:

```go
type FunctionFact struct {
    Summary []typ.Type
    Narrow  []typ.Type
    Type    typ.Type
}
```

But its operations should move out of `returns`:

```text
compiler/check/domain/functionfact
```

It owns:

- same-shape function fact merge,
- param-slot merge,
- return-vector merge delegation,
- effect/spec/refinement merge,
- function fact widening,
- function fact normalization,
- function fact equality.

The param-slot policy must be a named domain object, not scattered helpers:

```go
type ParamSlotDomain struct {
    Mode MergeMode // precise join or convergence widening
}
```

The previous `candidateRefinesFunctionParam`,
`typeRefinesTableKeyByTruthiness`, `preferConcreteOverSoftType`, and related
return-package functions are being collapsed into domain-owned operations.
Parameter-specific pieces now live in `domain/paramevidence`; the remaining
function-fact merge should move to `domain/functionfact`.

### Return Summary Domain

Owns return-vector shape and convergence:

- arity normalization,
- nil-slot handling,
- `unknown` as unresolved runtime behavior,
- stale nil-only regression prevention,
- recursive structural-growth cutoff,
- concrete-over-soft container refinement,
- return-slot row-tail merging,
- function-return widening.

Candidate home:

```text
compiler/check/domain/returnsummary
```

The existing `returns` package can either become this package or stop owning
non-return policy.

### Parameter Evidence Domain

Owns all evidence about parameters:

- call-site observations,
- body-derived contracts,
- signature facts,
- param-use projection,
- soft annotations,
- table-top absorption,
- nilability splitting,
- map/record joins,
- call graph propagation.

Candidate home:

```text
compiler/check/domain/paramevidence
```

Current state after the first domain slice:

- merge/canonicalization policy lives in `compiler/check/domain/paramevidence`;
- return SCC inference and interproc postflow still collect observations, but
  they call the domain to merge them;
- remaining work is to separate collection orchestration from the pure domain
  surface where it improves readability without adding a bridge.

Final rule:

Orchestration may stay in inference packages, but merge/canonicalization policy
belongs to the parameter evidence domain.

### Interproc Fact Domain

Owns the whole product:

```go
type FactsDomain struct {
    FunctionFacts FunctionFactDomain
    ParamEvidence ParamEvidenceDomain
    LiteralSigs   LiteralSignatureDomain
    Captures      CaptureDomain
    Constructors  ConstructorDomain
    Effects       EffectDomain
}
```

Candidate home:

```text
compiler/check/domain/interproc
```

It exposes only:

```go
Normalize(facts)
Leq(a, b)
Join(a, b)
Widen(prev, next)
Equal(a, b)
```

The store calls this domain. Producers emit deltas. Producers do not call local
helper joins directly.

## Helper Cluster Ownership

| Current Cluster | Current Location | Final Owner |
|---|---|---|
| `JoinFacts`, `WidenFacts`, fact equality | `compiler/check/returns` | `domain/interproc` |
| function fact type merge | `compiler/check/returns/join.go` | `domain/functionfact` |
| function param-slot refinement | `domain/paramevidence` plus `domain/value`, called by `returns/join.go`, `widen.go` | `domain/functionfact.ParamSlotDomain` delegating value refinements to `domain/paramevidence`/`domain/value` |
| return-vector merge/repair | `compiler/check/returns/join.go` | `domain/returnsummary` |
| table-top absorption | `domain/paramevidence` | `domain/paramevidence` plus value-domain classifier |
| soft vs concrete evidence | `typ/soft.go`, `domain/value`, return overlay | `domain/value` evidence policy |
| open-record row-tail merge | `types/typ/policy.go` | `domain/value` row-shape policy |
| path/query/alias identity | `constraint`, `flowbuild/path`, `flow/pathkey` | `memory` |
| table/container mutation replay | `nested`, `returns`, `flowbuild`, `flow` | `memory` mutation domain |
| error-return convention | `erreffect`, call/return inference | `domain/relation` |
| effect inference | `effects`, `erreffect`, `flowbuild`, `nested`, `returns` | `domain/effect` |
| body parameter contracts | `infer/return`, `flowbuild/assign` | `domain/paramevidence` |
| Salsa snapshot inputs | `store/snapshot_inputs.go` | keep in store, but document as cache boundary |

## Worked Consolidation Examples

### Table-Key Truthiness Refinement

Previous smell:

```go
candidateRefinesFunctionParam(candidate, baseline)
typeRefinesTableKeyByTruthiness(candidate, baseline)
recordRefinesTableKeyByTruthiness(candidate, baseline)
```

These helpers are trying to express one domain law:

```text
A table-like parameter fact may refine its key domain by removing falsy key
members only if the table value domain and structural frame are preserved.
```

Final home:

```text
domain/value.Refinement
domain/functionfact.ParamSlotDomain
domain/paramevidence
```

Final expression:

```go
refinement := value.Refinement{
    Kind: value.RefineTruthyKey,
    PreserveFrame: true,
    PreserveValue: true,
}
paramSlot.Join(existing, candidate, refinement)
```

The check is no longer a local function-param helper. It is a value-domain
refinement rule reused by parameter evidence, function facts, and return
summary map-key refinement.

### Soft Evidence Replacement

Current smell:

```go
preferConcreteOverSoftType(a, b)
typ.PruneSoftUnionMembers(t)
reconcileSoftAnnotatedInference(base, inferred)
```

These are fragments of one evidence-ordering law:

```text
hard concrete evidence dominates soft placeholder evidence, but nil alone does
not erase soft structured evidence.
```

Final home:

```text
domain/value.EvidenceOrder
```

Final expression:

```go
EvidenceOrder.Select(existing, candidate)
```

Every caller gets the same policy:

- soft annotation refinement,
- function parameter facts,
- parameter evidence,
- return-summary container refinement,
- flow assignment refinement.

### Open-Record Row Tail

Current smell:

Open-record behavior is split between record join, subtyping, table literal
contextualization, and external-regression fixes.

Canonical law:

```text
A missing field on an open record is row-tail evidence, not proof of nil.
A missing field on a closed record is absence.
```

Final home:

```text
domain/value.RowShape
```

Final API:

```go
RowShape.FieldEvidence(record, fieldName) FieldEvidence
```

The rest of the checker asks for field evidence. It does not rediscover whether
the record is open, closed, map-like, or table-top.

### Captured Table Mutation Replay

Current smell:

Captured table inserts, generic container mutations, parent replay, direct
flow mutators, and nested function calls have separate paths.

Canonical law:

```text
A mutation has one semantic operator and one memory location. Replay is valid
only when alias identity, dominance, and operator kind are preserved.
```

Final home:

```text
compiler/check/memory
```

Final expression:

```go
MemoryState.Apply(Mutation{
    Kind: MutationTableElement,
    Target: Location,
    Value: Type,
    Provenance: CapturedCall,
})
```

The same apply path handles direct `table.insert`, nested captured insert, and
exported callback replay.

### Error-Return Correlation

Current smell:

Several phases know about the `(value, err)` convention, arity checks, and
success/failure narrowing.

Canonical law:

```text
Error-return behavior is a tuple relation over return slots, not a special case
of a two-result function.
```

Final home:

```text
domain/relation
```

Final expression:

```go
RelationDomain.Attach(ReturnTupleRelation{
    Success: { ErrSlot: Nil, ValueSlot: NonNilOrUnknown },
    Failure: { ErrSlot: NonNil, ValueSlot: NilOrUnknown },
})
```

The canonical Lua `(value, err)` convention is one predefined relation. Future
relations do not require new helper clusters.

## Target Data Flow

The final flow should be:

```text
source
  -> CFG + symbol graph
  -> normalized checker IR
  -> abstract transfer over AbstractState
  -> queryable solved state
  -> function result
  -> interproc fact delta
  -> FactsDomain.Join or FactsDomain.Widen
  -> Salsa input update
  -> dependent function-result query revalidation
```

Every arrow has one owner.

No phase should secretly perform another local abstract interpretation unless
that interpretation is a named domain transfer over the same `AbstractState`.

Preflow, local SCC inference, and return overlay currently exist for good
reasons. The design target is not to delete their semantics. The design target
is to make them clients of the same domain objects instead of separate local
machines.

## Dataflow State Machine

The checker should have one visible state machine.

```text
Unbuilt
  -> GraphBuilt
  -> IRBuilt
  -> Solving
  -> Solved
  -> Inferred
  -> Published
  -> Snapshotted
```

### Unbuilt -> GraphBuilt

Input:

- source AST,
- parent scope,
- manifest environment.

Output:

- immutable graph bundle.

No type-domain merge is allowed here.

### GraphBuilt -> IRBuilt

Input:

- graph bundle,
- declared type environment,
- known effect specs.

Output:

- transfer program.

This stage may observe syntax and produce instructions. It may not decide
fixpoint policy.

### IRBuilt -> Solving

Input:

- transfer program,
- initial abstract state,
- domain set.

Output:

- evolving abstract state.

All state changes go through transfer and domain operations.

### Solving -> Solved

Input:

- worklist convergence,
- loop widening if needed.

Output:

- solved abstract state plus query view.

No interproc publication happens before this state.

### Solved -> Inferred

Input:

- query view,
- function body,
- relation/effect summaries.

Output:

- function result and interproc delta.

Inference reads solved state. It does not create another path-sensitive solver.

### Inferred -> Published

Input:

- immutable interproc delta.

Output:

- canonical fact product after join or widening.

Only `FactsDomain` may combine this data.

### Published -> Snapshotted

Input:

- canonical fact product.

Output:

- Salsa snapshot inputs and dependent query invalidation.

No semantic repair is allowed here. Snapshotting is cache wiring only.

## Nested Fixed-Point Model

The final checker has several fixed points, but they should all use the same
domain vocabulary. The existence of multiple schedules does not justify
multiple semantic models.

### Level 0: Pure Graph Summaries

Graph summaries are not fixpoints over types. They are immutable facts about
syntax and binding:

- parameter uses,
- return sites,
- local function edges,
- call sites,
- mutator sites,
- captured path mentions,
- normalized transfer instructions.

They can be cached by graph identity because they do not read interproc facts or
solved flow state.

### Level 1: Intraprocedural CFG Fixpoint

The local solver computes:

```text
CFG x TransferProgram x InitialState -> SolvedState
```

Convergence boundary:

- CFG joins use `AbstractState.Join`;
- loops use the relevant domain widen only when a loop-carried component grows
  past the domain's finite-height fragment;
- dead/unreachable paths update termination/reachability before value queries.

Forbidden:

- AST rescans during solve,
- producer-specific joins,
- query-time narrowing that writes state,
- loop-specific precision hacks outside domain widening.

### Level 2: Local Function SCC Fixpoint

Local functions inside a graph can be mutually recursive. The final model should
treat their summaries as another domain product:

```text
FunctionSummary =
  Parameters
  x Returns
  x Relations
  x Effects
  x Captures
```

Convergence boundary:

- recursive calls read the current SCC summary through the function fact domain;
- each function body emits a new summary delta;
- SCC join/widen uses the same parameter, return, relation, effect, capture,
  and memory domains used elsewhere;
- when the SCC stabilizes, the solved summaries become ordinary evidence for
  the enclosing function analysis.

This replaces "return overlay", "preflow synthesis", and "local function
snapshot repair" as separate semantic concepts. Those may remain as scheduling
or performance techniques, but not as separate laws.

### Level 3: Interprocedural Fixpoint

The outer fixpoint computes canonical facts across function/module boundaries:

```text
InterprocPrev + all FunctionResult deltas -> InterprocNext
InterprocPrev' = FactsDomain.Widen(InterprocPrev, InterprocNext)
```

Convergence boundary:

- producers emit immutable deltas;
- `FactsDomain` is the only merge/widen authority;
- no producer reads its own just-emitted delta except through the declared
  current-iteration overlay contract;
- equality checks canonical state only;
- snapshot inputs are updated only after the canonical product changes.

Iteration caps are diagnostics, not semantics. If convergence requires raising a
cap for normal programs, the relevant `Widen` is missing or too precise.

### Level 4: Incremental Revalidation

Salsa does not define type semantics. It revalidates query results after inputs
change.

Required dependency shape:

```text
FuncResultQ
  reads GraphSummaryQ
  reads Manifest/Input queries
  reads SnapshotInputs
  reads TypeQuery caches
  computes local fixed points
  publishes deltas
```

When a snapshot input is unchanged, dependent results should revalidate without
re-solving. When a graph summary is unchanged, function queries should not rescan
the AST to rediscover it. When a type-query cache hits, it should only avoid
structural recomputation; it must not mask missing checker dependencies.

### Fixed-Point Proof Obligations

Each level needs a proof surface:

- finite input identity,
- monotone transfer or documented approximation,
- explicit join/widen boundary,
- stable equality without repair,
- deterministic publication,
- cache invalidation by immutable dependency.

Performance and soundness meet at these obligations. A cache that is missing a
dependency is unsound. A widen that erases too much precision causes false
positives. A join that keeps rebuilding equivalent maps causes unnecessary
invalidations.

## Salsa And Cache Model

Current good shape:

- function-result keys are stable graph/parent identities,
- interproc snapshots are `db.Input`s,
- updating facts bumps dependent queries through the database,
- core type queries are Salsa-style pure queries.

Current weak shape:

- the checker still has several non-Salsa local caches with implicit lifetimes,
- flow solution caches are manually invalidated,
- some expensive shape scans are repeated because domain operations are not
  centralized,
- param-use projection can rescan AST bodies instead of reading a graph-indexed
  use summary.

Canonical Salsa wiring:

```text
db.Input[ManifestKey]        -> module/type environment queries
db.Input[GraphKey]           -> graph-derived summaries
db.Input[InterprocGraphKey]  -> function-result queries
db.Input[SymbolKey]          -> constructor/refinement/effect summaries

FuncResultQuery(GraphID, ParentHash)
  reads graph bundle
  reads interproc snapshot inputs
  builds transfer program
  solves abstract state
  publishes immutable result
```

The query key is stable identity. The dependency edges come from the exact
inputs read during analysis. There should be no artificial revision number in
the function key and no manual cache clearing for correctness.

Final cache contracts:

1. Source inputs are `db.Input`s:
   - manifests,
   - parent scope,
   - CFG identity,
   - interproc facts,
   - constructor fields,
   - function refinements.
2. Pure expensive computations are `db.Query`s:
   - core type lookup/index/method/operator queries,
   - function result,
   - parameter-use summary by graph/function,
   - shape classification for large recursive types if profiling confirms it.
3. Intraprocedural flow state remains per-function and ephemeral unless it is
   keyed by the exact immutable input bundle. Do not put hot per-edge transfer
   into Salsa if dependency recording costs more than recomputation.
4. Domain operations must be pure and deterministic so they can be memoized
   safely when profiling justifies it.
5. Cache lifetime must be explicit in package docs. No cache should depend on
   call order for correctness.

Performance target:

- fewer repeated shape scans,
- fewer temporary maps in hot merges,
- copy-on-write vectors and maps,
- immutable fact snapshots,
- stable interning/hash-consing where already available,
- no object pools until ownership is proved and structural wins are exhausted.

### Cache Placement Decision Model

Use Salsa when:

- the computation is pure,
- the inputs are immutable identities,
- dependency tracking can precisely invalidate dependent queries,
- the result is reused across functions, modules, or fixpoint iterations,
- recomputation is more expensive than dependency tracking.

Use a per-function local cache when:

- the computation is hot inside one solve,
- the cache key is a small local identity,
- the result is invalid after the current function solve,
- Salsa dependency tracking would be more expensive than recomputation.

Use no cache when:

- the operation is a cheap domain primitive,
- the input is already interned,
- the allocation is caused by poor ownership rather than repeated work,
- correctness would require observing mutable phase order.

Do not use a pool until:

- the allocation site remains hot after domain consolidation,
- ownership of each pooled object is single-phase and obvious,
- tests prove no retained result can observe a reused object,
- profiling shows the pool wins after synchronization and clearing costs.

The main expected Salsa gains are:

- graph-indexed parameter-use summaries instead of AST rescans,
- function-result queries keyed by graph and parent scope,
- pure type/operator queries,
- shape classification for large recursive types if profiling proves reuse,
- canonical interproc snapshots as inputs instead of manually invalidated maps.

The main non-Salsa gains are:

- domain operations that avoid rebuilding maps for no-op joins,
- path/location interning,
- copy-on-write fact vectors,
- removing compatibility projections from hot publication paths,
- making equality structural instead of repair-driven.

### Concrete Salsa Wiring Plan

The final design should classify every current cache and summary producer before
implementation. The goal is not "put everything in Salsa". The goal is exact
incremental boundaries and no hidden semantic cache.

| Current Component | Final Role | Cache Kind | Owner |
|---|---|---|---|
| `api.FuncKey{GraphID, ParentHash}` | function analysis identity | Salsa query key | pipeline/analysis engine |
| `FuncResultQ` | analyze one function under one parent scope | Salsa query | analysis engine |
| `snapshotInputs.facts` | canonical interproc fact snapshot | Salsa input | store/facts domain boundary |
| `snapshotInputs.refinements` | function refinement/effect snapshot | Salsa input | effect/refinement boundary |
| `snapshotInputs.constructorFields` | constructor field snapshot | Salsa input | memory/constructor boundary |
| `types/query/core.Engine` | pure type operations | query engine cache | type-query layer |
| `types/flow.ProductDomain` | branch-local narrowing algebra | ephemeral domain state | abstract state / flow domain |
| `paramevidence.collectParamUses` | body-demand summary | graph-derived Salsa query | graph summary layer |
| `ProjectHintsToParamUse` | parameter evidence projection | domain operation over cached body summary | parameter domain |
| `PreCache` / `NarrowCache` | repeated expression synthesis inside one solve | per-function local cache | transfer/query phase |
| `FunctionTypeCache` | local function specialization during one solve | per-function local cache unless key is immutable | function analysis |
| `StableFunctionSnapshot` | read canonical function fact snapshot | Salsa query/input read, not ad hoc map | function fact domain |
| flow solution narrow caches | repeated solved-state query | solved-state local cache | query view |
| path suffix/root caches | identity interning | local/global intern cache if immutable | memory/location layer |

This table is a migration contract. If a component does not appear here or in a
successor table before coding, adding a cache for it should be rejected.

### Query Dependency Contract

`FuncResultQ` must read all semantic dependencies through tracked inputs or
tracked pure queries.

Required reads:

- graph bundle by `GraphID`,
- parent scope by `ParentHash`,
- canonical interproc facts by `GraphKey`,
- function refinements/effects by owning symbol key,
- constructor fields by owning symbol key,
- manifest/module environment through manifest inputs,
- graph-derived body summaries through graph summary queries,
- pure type operations through the type-query layer.

Forbidden reads:

- mutable `InterprocPrev` maps without snapshot input tracking,
- current-iteration `InterprocNext` except through the canonical overlay input
  contract,
- ad hoc stable snapshot maps inside synthesis,
- source AST rescans for reusable graph summaries,
- global variables whose mutation does not bump a tracked input.

When a function reads a fact for a graph or symbol, the query database must know
that dependency. When the fact does not change semantically, the input should not
be rewritten. This gives both correctness and performance: no stale result, no
unnecessary invalidation.

### Snapshot Update Protocol

The store should be the only bridge from fixpoint state to Salsa inputs.

```text
producer emits InterprocDelta
  -> FactsDomain.Join/Widen into InterprocNext
  -> iteration boundary computes canonical InterprocPrev
  -> compare canonical old/new with structural equality
  -> set only changed snapshot inputs
  -> Salsa revalidates dependent FuncResultQ entries
```

Required properties:

- `setFacts` receives canonical facts only;
- equality is structural and does not normalize;
- empty facts are represented explicitly enough to clear stale inputs;
- per-symbol inputs are used only for facts whose key is truly symbol-local;
- parent-scoped facts use `GraphKey` or `SymbolKey`, not raw `SymbolID`;
- current-iteration overlay is either part of the canonical input contract or is
  not visible to `FuncResultQ`.

This avoids manual cache clearing as a correctness mechanism. Clearing may still
exist as a memory-pressure tool, but a correct result must not depend on it.

### Graph Summary Queries

Several expensive operations are currently repeated because syntax-derived
summaries are computed by the consumer. These should become graph summary
queries.

Recommended summaries:

- parameter-use summary by `GraphID` and function symbol,
- return-site summary by `GraphID`,
- local function/call graph summary by `GraphID`,
- table mutator call summary by `GraphID`,
- key-collector summary by `GraphID`,
- captured variable/path summary by `GraphID`,
- normalized transfer program by `GraphID` plus declared environment identity.

These queries read immutable graph/source data and produce immutable summaries.
They do not read interproc facts and they do not infer types. The analysis query
then combines those summaries with parent scope and interproc snapshots.

### Hot Local Cache Contract

Some caches should remain local because they are only useful during one solve.

Local cache keys must include:

- phase (`declared`, `preflow`, `narrow`, or final query),
- expression identity or normalized instruction identity,
- CFG point,
- parent scope identity when the answer can depend on scope,
- solved-state token when the answer depends on flow facts.

Local caches must not:

- survive across `FuncResultQ` computations unless the key is fully immutable,
- contain mutable domain state,
- publish facts,
- suppress dependency tracking by reading snapshots behind Salsa's back.

This keeps hot expression synthesis fast without making it a second semantic
store.

### Type Query Layer Contract

The core type query engine is already the right kind of abstraction for
field/index/operator/subtype queries: pure inputs, stable type identities, and
memoized expensive structural work.

Final rules:

- checker domains may call pure type queries;
- type queries must not read checker store state;
- type query caches are performance-only;
- type query answers must be invalidated or keyed by all external type-provider
  inputs they depend on;
- domain law tests should not depend on query cache hit order.

This means Salsa does not replace `types/query/core`. Salsa coordinates checker
analysis dependencies. The type query engine owns repeated structural type
operations.

### Performance Proof Requirements

A performance correction is accepted only with a before/after profile or
benchmark that names the reduced work.

Required measurements for the flash migration:

- large-function checker benchmark,
- representative interproc convergence fixture,
- production replay wall time,
- allocation profile for hot joins and expression synthesis,
- cache hit/miss counters for `FuncResultQ` and graph summary queries,
- number of snapshot inputs rewritten per fixpoint iteration.

Expected improvements:

- fewer `collectParamUses` rescans,
- fewer repeated local function snapshot syntheses,
- fewer map allocations in no-op fact joins,
- fewer invalidated function queries after no-op fact updates,
- fewer expression synthesis calls during narrow/final query phases.

Regression rule:

```text
If a performance win comes from accepting less precise facts, it is invalid.
If a precision win causes repeated semantic recomputation, the cache boundary is
wrong and must be fixed before the flash migration lands.
```

## Weak Points To Fix In The Design

### 1. Domain Laws Are Not Named

The checker has laws such as:

- hard evidence dominates soft evidence,
- `unknown` in return summaries is unresolved runtime behavior,
- open record absent field means row-tail, not nil,
- nil field can satisfy optional absence in record subtyping,
- table-top can absorb precise table evidence in parameter evidence,
- truthy refinement can remove falsy key alternatives.

Today many of these appear as function names buried in unrelated packages. They
must become named laws of specific domains.

### 2. Too Many Local Abstract Interpreters

`flowbuild`, `types/flow`, return SCC inference, preflow synthesis, return
overlay refresh, condition extraction, and interproc widening each perform part
of the abstract interpretation.

The final design should have one abstract state model and several orchestration
phases. The orchestration may be complex; the lattice rules cannot be local.

### 3. Memory Is Not First-Class Enough

Field writes, table inserts, dynamic indexes, aliases, captured mutations, and
path queries all affect the same memory model. They are currently split across
multiple packages.

This causes bugs where:

- parent-derived structure outranks explicit child-path facts,
- captured table inserts replay through the wrong mutator kind,
- alias identity and dominance are checked locally,
- nil overwrite and optional absence need separate fixes.

The final memory domain must own these rules.

### 4. Parameter Evidence Has Multiple Authorities

Parameter evidence currently comes from:

- call sites,
- body contracts,
- function facts,
- literal signatures,
- soft source annotations,
- param-use projection.

The final design needs one `ParameterEvidence` lattice with evidence provenance
and merge mode. The implementation should not need separate helpers for
"parameter evidence" and "function param facts" that rediscover the same truthiness,
softness, and table-key laws.

### 5. Relation Facts Are Under-Modeled

The system supports powerful correlations, especially error-return behavior, but
the relation model is still too tied to known patterns.

The final design should model tuple/path relations directly. `(value, err)` is
then one relation instance. This keeps the system extensible without hardcoded
branch helpers or return-slot checks.

### 6. Effect Inference Is Too Distributed

Effects are currently inferred and replayed from several places:

- call specs,
- error-return inference,
- captured field/container mutation collection,
- nested mutator replay,
- key collector detection,
- predicate/assertion refinements.

Those are all effect facts. They need one summary model and one application path
through transfer. Otherwise each new effect creates its own mini analysis and
its own invalidation/caching risks.

### 7. Tests Are Too Positive-Heavy

Many external-lint regressions are "this must type-check" tests. Those are
useful, but insufficient. They can pass through accidental broadening.

Every major law needs:

- a positive test proving wanted inference,
- a negative test proving sound rejection,
- a domain law test proving normalize/join/widen idempotence and monotonicity.

## Anti-Pattern Catalog

These shapes should be rejected during the flash migration.

### Local Domain Predicate In An Orchestration Package

Example smell:

```go
func typeRefinesTableKeyByTruthiness(...)
```

If the helper defines what refinement means, it belongs to a domain package.
Orchestration packages can ask a domain whether a refinement is valid; they
cannot define the refinement locally.

### Equality-Time Repair

If equality normalizes, rebuilds, or reconciles facts to make two states look
equal, convergence bugs become invisible.

Correct shape:

```text
write boundary -> Normalize
merge boundary -> Join/Widen
equality -> structural comparison of canonical state
```

### Query-Time Fact Production

If a query discovers a fact that later code relies on as if it were stored
analysis state, the system has a hidden analysis path.

Correct shape:

```text
query can memoize an answer, but cannot publish evidence
```

### Producer-Specific Merge

If one producer has its own merge rules for a fact family, the product domain is
not canonical.

Correct shape:

```text
producer emits delta
store calls FactsDomain.Join or FactsDomain.Widen
```

### Compatibility View As Authority

A projection may exist for display or API response, but not as stored authority.
If production code writes through a view, it recreates the legacy mirror problem.

### Soundness Shortcut

Any change whose main effect is "fewer external diagnostics because `any` now
passes" is rejected unless a domain proof explains why that `any` was not truly
dynamic.

### Cache Without Input Contract

Every cache must state:

- exact key,
- immutable inputs,
- invalidation mechanism,
- whether it is semantic or performance-only.

If the cache depends on phase call order, it is not SOTA.

## Failure Taxonomy

Future regressions should be classified by failed domain responsibility, not by
the helper function that happened to produce the symptom.

| Symptom | Likely Owner | First Question |
|---|---|---|
| guarded field still nilable at call site | `RelationDomain` or `MemoryDomain` | Did the guard create a path relation for the same location queried by the call? |
| error-return refinement does not affect value slot | `RelationDomain` | Was the tuple relation preserved through return assignment and wrapper forwarding? |
| external dynamic value passes concrete parameter | `ValueDomain` or `ParameterEvidenceDomain` | Did `any` get treated as proof instead of dynamic top? |
| unknown disappears from return summary | `ReturnSummaryDomain` | Did join/widen erase unresolved evidence? |
| nil field write behaves like absent field | `MemoryDomain` | Was nil overwrite represented as a value fact instead of structural deletion? |
| closed missing field behaves like open row-tail | `ValueDomain` or `MemoryDomain` | Was openness carried on the record/map component being queried? |
| table insert lost before iteration | `MemoryDomain` and `EffectDomain` | Was mutation replay attached to the canonical child location and operator kind? |
| recursive type keeps growing | owning domain `Widen` | Is growth bounded at the correct SCC/fixpoint boundary? |
| result changes after no semantic input changed | Salsa/cache layer | Is a cache keyed by mutable state or phase order? |
| result does not change after facts changed | Salsa/cache layer | Did the query read the canonical snapshot input that changed? |
| lint clears by accepting too much | `ValueDomain` or assignability boundary | Which negative test proves the new acceptance is sound? |
| repeated performance hot spot after caching | domain/query boundary | Is the computation duplicated because the owner is unclear? |

Classification rule:

```text
If a symptom requires reading three unrelated helpers to understand why it
happened, the domain model is still wrong.
```

The fix should move the law to the owner, delete the scattered helpers, and add
domain law tests plus one production-shaped replay test.

## Traceability Matrix

Every high-value behavior should be traceable from syntax to proof.

| Behavior | Producer | Canonical Fact | Consumer | Proof |
|---|---|---|---|---|
| truthy field guard | condition transfer | path truthiness relation | call/type query | relation law + guarded-call fixture |
| `test.is_nil(err)` success branch | predicate effect transfer | tuple-slot relation constraint | value-slot query | relation law + error-return fixture |
| body demands parameter field | transfer over field read/use | parameter obligation | interproc fact join | parameter evidence law + SCC fixture |
| call observes argument type | call transfer | call observation | parameter evidence join | authority-order law + negative any fixture |
| table insert mutates array element | effect transfer | container element mutation | iteration query | memory law + dominance fixture |
| nil overwrite | assignment transfer | explicit nil value or deletion effect | field query | nil/absent law + record fixture |
| wrapper forwards returns | return transfer | tuple relation preservation | caller assignment | relation preservation law + wrapper fixture |
| imported dynamic payload | external contract transfer | `any` or `unknown` with provenance | assignability check | value law + negative concrete-param fixture |
| recursive local function | SCC solver | widened param/return evidence | function result query | widen law + convergence fixture |
| module export | publication | immutable interproc delta | dependent Salsa query | snapshot dependency test |

This matrix is not a test list by itself. It is the audit trail showing that a
behavior has one producer, one canonical representation, one consumer path, and
one proof family.

## Design Review Decision Tree

Every future rule should be classified before code is written.

### Is It About What A Type Means?

Examples:

- `unknown` vs `any`,
- open row tail,
- nilability,
- truthiness,
- soft evidence,
- table top.

Owner:

```text
ValueDomain
```

Reject if implemented in return inference, call checking, or postflow writer.

### Is It About Where A Fact Lives?

Examples:

- field path,
- dynamic index,
- alias target,
- tuple slot,
- captured mutation target,
- receiver `self`.

Owner:

```text
MemoryDomain / Location model
```

Reject if every producer computes its own path identity.

### Is It About How Facts Combine?

Examples:

- branch join,
- parameter evidence merge,
- return vector merge,
- function fact merge,
- recursive shape cutoff.

Owner:

```text
The domain that owns that fact family
```

Reject if implemented as a producer-specific helper.

### Is It About When Analysis Converges?

Examples:

- loop widening,
- local function SCC widening,
- interproc widening,
- recursive type growth.

Owner:

```text
Widen operation of the relevant domain
```

Reject if hidden inside equality, query, or local preference helpers.

### Is It About What A Call Does?

Examples:

- mutates a table,
- narrows an argument,
- returns `(value, err)`,
- terminates,
- invokes a callback,
- collects keys.

Owner:

```text
EffectDomain + RelationDomain + MemoryDomain transfer
```

Reject if modeled as a one-off postprocessing pass.

### Is It About Reusing Work?

Examples:

- graph summaries,
- parameter-use summaries,
- function result,
- type operator query,
- shape classification.

Owner:

```text
Salsa query or explicit local cache with named inputs
```

Reject if invalidation depends on call order or hidden mutable state.

## Edge-Case Matrix

The migration must consider edge cases beyond the failures already seen. The
design is not complete until each row below has an owner domain and tests.

| Area | Edge Cases To Model |
|---|---|
| `unknown` | branch join with concrete, return merge with concrete, exported summary, table field, array element, call argument, relation slot |
| `any` | explicit cast to any, imported dynamic data, any flowing to concrete param, any in record field, any as table key/value, any through relation facts |
| `nil` | nil as Lua value, nil as field deletion, nil satisfying optional absence, nil array slot, nil map value, nil return slot |
| absent field | closed record absence, open row-tail unknown, map-tail optional value, table-top field access, absence after mutation |
| soft evidence | soft table top, soft array element, soft map value, nil plus soft shape, hard evidence replacing soft evidence, soft evidence across imports |
| table top | `table`, `{...}`, `{[any]: any}`, arrays, maps, closed records, open records, unions with precise tables |
| row shape | open vs closed, readonly fields, optional fields, metatables, map component overlap, discriminant tags |
| truthiness | false/nil removal, literal false keys, `and`/`or` branch values, truthy field guards, truthy dynamic indexes |
| mutation | field write, nil overwrite, dynamic index write, table insert, container send, captured mutation, exported callback mutation |
| aliasing | local alias, field alias, imported alias, method receiver alias, self alias, cyclic alias, alias after reassignment |
| dominance | dominating writes, branch-local writes, loop-carried writes, post-dominated assertions, early returns, dead paths |
| functions | optional function values, union of function signatures, method `self`, varargs, higher-order callbacks, recursive locals |
| returns | zero returns, one return, two returns, more than two returns, tuple expansion, nil padding, recursive containers |
| relations | `(value, err)`, custom error record, multiple independent relations, swapped slots, relation through wrapper, relation through any |
| effects | termination, assertion refinements, callback effects, captured mutation effects, key collection, external contract effects |
| interproc | parent scope change, module boundary, literal signatures, captured fields, constructor fields, sibling overlay, stale snapshots |
| caching | stale query after fact change, query reuse after no-op fact change, cache key missing parent scope, cache key missing graph identity |
| performance | recursive structural scan, repeated AST projection, repeated map allocation, query dependency overhead, equality-time canonicalization |

Adversarial cases must include both:

- precision cases where the checker should infer the strongest provable type;
- soundness cases where similar-looking code must still fail.

Examples:

- guarded `options.model` should infer `string`; `provider_info as any` should
  not become `string` without proof;
- `response.body or ""` should be `string`; `response.body` alone remains
  `string?`;
- open row-tail field access is `unknown`; closed missing field is absent/nil
  evidence depending on context;
- table insert before an `ipairs` loop should feed element type; branch-local
  insert must not leak if the loop is not dominated by that branch;
- `test.is_nil(err)` may refine a related value slot only if a relation fact
  proves the tuple contract.

The suite should be generated around these matrices, not around the names of
the old helper functions.

## Flash Migration Shape

The implementation should be prepared privately but merged as a direct final
shape. The production branch should not pass through partial API compatibility.

Flash migration means:

1. Introduce final domain packages.
2. Move domain laws into those packages.
3. Replace all call sites in one migration.
4. Delete old helper clusters in the same migration.
5. Delete obsolete tests that asserted old helper behavior.
6. Add law-oriented tests for the new domain boundaries.
7. Run the global replay and classify remaining diagnostics.

No step should leave:

- old helper path plus new helper path,
- adapter projections like "legacy view from canonical facts",
- duplicate merge functions for the same semantic slot,
- fallback normalization in equality,
- broad `any` acceptance to clear lints.

## Flash Cutover Gate

The flash migration should be reviewed as one semantic cutover, not as a chain
of transitional accommodations. The cutover is ready only when the following
artifacts can be listed before coding starts.

### Deletion Map

For each old helper cluster:

- current file/package,
- semantic law it currently approximates,
- final domain owner,
- final API call site,
- tests that replace helper-specific tests,
- commit in which the helper disappears.

If a helper cannot be mapped to a domain owner, the design is incomplete. If it
maps to more than one owner, the fact representation is probably mixed and must
be split before implementation.

### Replacement Map

For each production call site:

- current call,
- final call,
- expected semantic output,
- changed cache dependency if any,
- changed diagnostic behavior if any.

The migration should not introduce "temporary" calls that are expected to be
removed later. A call site either moves to the final API or stays unchanged until
the cutover is ready.

### Proof Map

For each domain law:

- unit law test,
- one positive checker fixture,
- one negative checker fixture when soundness could be weakened,
- one replay/global-harness case if the law came from real code.

No proof should depend only on external lint going quiet. The suite must show
both the precision gain and the rejection boundary.

### Performance Map

For each expensive operation touched:

- current benchmark/profile location,
- final owner,
- expected cache key or no-cache reason,
- allocation behavior,
- invalidation story.

Performance work should favor fewer repeated analyses and fewer duplicated data
structures before object pools. Pools are allowed only after ownership is clear
and tests prove no fact lifetime can leak across checks.

### Cutover Rejection Rules

Reject the migration if it contains:

- compatibility authority,
- fallback repair,
- two writers for one fact,
- query-time publication,
- equality-time normalization,
- broad assignability introduced only to clear production code,
- new cache without an immutable input contract,
- new helper whose name describes a case instead of a domain law.

## Proposed Final Package Map

```text
compiler/check/domain/interproc
compiler/check/domain/functionfact
compiler/check/domain/returnsummary
compiler/check/domain/paramevidence
compiler/check/domain/relation
compiler/check/memory
compiler/check/flowstate
```

Existing packages remain as orchestration:

```text
compiler/check/abstract/transfer
compiler/check/synth
compiler/check/infer/return
compiler/check/infer/interproc
compiler/check/store
compiler/check/pipeline
```

Low-level pure type mechanics remain under:

```text
types/typ
types/subtype
types/query/core
types/db
```

The key rule:

Orchestration packages may decide when a fact is produced. Domain packages
decide what that fact means and how it combines.

## Minimum Final-Shape API Sketch

This is not a transitional API. It is the smallest final surface that should
exist after the flash migration.

```go
// compiler/check/analysis
type Engine struct {
    Graphs GraphProvider
    Domains Domains
    Queries Queries
}

func (e *Engine) AnalyzeFunction(input FunctionInput) FunctionResult
```

```go
// compiler/check/flowstate
type AbstractState struct {
    Memory MemoryState
    Values ValueFacts
    Numeric NumericFacts
    Shape ShapeFacts
    Relations RelationFacts
    Effects EffectFacts
    Termination TerminationFacts
}

func (s AbstractState) Join(other AbstractState, d Domains) AbstractState
func (s AbstractState) Widen(next AbstractState, d Domains) AbstractState
```

```go
// compiler/check/transfer
type Instruction interface {
    Apply(state flowstate.AbstractState, d Domains) flowstate.AbstractState
}
```

```go
// compiler/check/domain
type Domains struct {
    Value ValueDomain
    Memory MemoryDomain
    Relation RelationDomain
    Effect EffectDomain
    Parameter ParameterEvidenceDomain
    Return ReturnSummaryDomain
    Function FunctionFactDomain
    Interproc InterprocFactsDomain
}
```

```go
// compiler/check/domain/interproc
type InterprocFactsDomain interface {
    Normalize(api.Facts) api.Facts
    Leq(a, b api.Facts) bool
    Join(a, b api.Facts) api.Facts
    Widen(prev, next api.Facts) api.Facts
    Equal(a, b api.Facts) bool
}
```

```go
// compiler/check/query
type View interface {
    TypeAt(point cfg.Point, loc memory.Location) typ.Type
    RelationAt(point cfg.Point, rel relation.Query) relation.Answer
    EffectAt(point cfg.Point, call CallSite) effect.Summary
}
```

The important part is not exact names. The important part is that:

- state is one product;
- transfer mutates only that product;
- domains own all combination;
- query is read-only;
- interproc publication is delta-based;
- no package owns a shadow merge policy.

## Verification Model For The Future Migration

Required proof after the flash migration:

```text
go test ./...
git diff --check
../scripts/verify-suite.sh
```

Required domain law tests:

- `Normalize(Normalize(x)) == Normalize(x)`
- `Join(a, b) == Join(b, a)` where the domain is intended commutative
- `Join(Join(a, b), c) == Join(a, Join(b, c))` where applicable
- `Widen(Widen(a, b), b) == Widen(a, b)`
- `a <= Join(a, b)`
- `a <= Widen(a, b)`
- derived function type equals canonical function fact projection
- no equality-time normalization bridge

Required behavior suites:

- soft vs hard evidence,
- any vs unknown,
- nil vs absent,
- open vs closed records,
- table top vs precise table shapes,
- captured table/container mutations,
- alias and dominance,
- error-return tuple relations,
- local SCC parameter evidence,
- interproc non-convergence fixtures,
- external replay reductions.

## Review Checklist Before Coding

Before implementing the flash migration, each proposed package should answer:

- What domain or boundary object does this package own?
- What are the only mutable states in this package?
- Which operation is transfer, join, meet, widen, normalize, query, or publish?
- Which laws are tested at the package boundary?
- Which edge-case matrix rows does it cover?
- Which caches does it introduce, and what exact immutable inputs key them?
- Which old helper clusters will be deleted when this lands?
- Which production call sites will move directly to the final API?
- What negative tests prevent broadening `any`, erasing `unknown`, or treating
  absence as nil in the wrong domain?

If any answer is "handled by a fallback during migration", the design is not
ready. The next implementation must be flash migration, not coexistence.

## Current Conclusion

The checker is not fundamentally the wrong idea. It is closer to a serious
abstract interpreter than it looks from isolated helper functions.

The foundational problem is organizational: the product domain exists in
behavior but not cleanly enough in code. The next design correction should not
add more local helpers. It should move the existing laws into explicit domain
objects, make memory/path identity first-class, and make Salsa/cache boundaries
documented and deliberate.

If this is done as a flash migration, the codebase should become smaller because
many helper clusters collapse into a few named domains. It should also become
easier to reason about because every merge/refinement/widening decision will
have one owner and one law-test suite.

## 2026-05-19 Engine Verification And Classification Checkpoint

This pass removed the remaining parameter-count heuristic in call diagnostics.
The old shape was not a domain law: graph-local calls were relaxed based on
source arity. The replacement is a semantic boundary:

- function facts remain the call contract authority;
- explicit `any` arguments are only ignored for graph-local parameter slots
  whose value is never observed by the function body;
- observed parameters still enforce their declared or inferred contract;
- the unobserved-parameter mask is computed once per function symbol during the
  call-check pass from binder symbol identity, so shadowing and captured uses are
  handled by symbols, not names.

Regression coverage added:

- an internal `any` passed to an unobserved local parameter does not create a
  false positive;
- the same `any` passed to an observed `string` parameter remains an error;
- imported/manifest call boundaries that require `string` still reject `any`;
- the external-lint reductions now also cover selected HTTP response body
  fallback, error-guarded imported page field casts, and captured state-field
  map iteration.

Verification from this checkpoint:

```text
go test ./... -count=1
go test ./compiler/check -run '^$' -bench BenchmarkCheck_LargeFunction -benchmem -count=3
../scripts/verify-suite.sh
```

Results:

- `go test ./... -count=1` passes.
- `BenchmarkCheck_LargeFunction` is about 1.83-1.86 ms/op, about 1.05 MB/op,
  and 10,695 allocs/op on this machine.
- `../scripts/verify-suite.sh` passes go-lua checker tests and builds Wippy, but
  the external lint section still exits non-zero because the script builds Wippy
  against `github.com/wippyai/go-lua v1.5.16`, not this checkout.
- A temporary Wippy binary built with a `/tmp` `go.mod` replacement pointed at
  this checkout was used for classification without editing external Wippy code.

Current external-lint classification:

- The official pinned verify output cannot prove current go-lua regressions.
  It reported `session` 8, `framework/src/agent/src` 13 during the script run
  and 8 on direct replay, and `docker-demo` 21 errors / 2 warnings.
- The local-replace replay is stricter than the pinned binary. It reports many
  explicit `any` to concrete-contract errors. Those are soundness-preserving
  external code or manifest contract issues unless reduced to a go-lua false
  positive.
- High-confidence engine candidates were reduced where possible. The current
  reduced go-lua fixtures for response-body fallback, page-field casts,
  captured state map iteration, length guards, setmetatable prototypes, query
  builder back-references, and imported assertions pass.
- Remaining unreduced candidates are mostly context-sensitive Wippy package
  interactions: imported module manifests that expose `unknown`/`any`, generated
  package cache shape, and real code paths that pass unchecked dynamic values
  into concrete APIs. They should not be fixed by weakening `any` or erasing
  `unknown`.

Design rule retained for the next pass:

- Do not add compatibility channels or fallback facts.
- Do not make `any` silently assignable to concrete types.
- If an external diagnostic is a false positive, first reduce it into a
  go-lua regression that fails for the same semantic reason, then fix the
  owning domain or transfer rule.
- If a diagnostic is true external code, keep it classified and do not edit
  external Wippy sources from this go-lua PR.

## 2026-05-19 External Replay Classification Follow-Up

The local-replace Wippy binary still reports diagnostics in external packages,
but the new reductions did not expose a go-lua engine regression. The important
distinction is that the checker is now refusing to erase dynamic source shapes
that are not proven by the Lua program.

New regression coverage added in `external_lint_regression_test.go`:

- optional numeric fields defaulted with `or` become non-nil before arithmetic;
- exported model-card numeric defaults remain non-nil at a consumer;
- imported modules stored in table fields preserve those numeric defaults;
- registry-derived numeric defaults still feed arithmetic after the consumer
  guards the optional return;
- guarded string field values inserted into an accumulator retain a string
  element type when iterated into a helper call;
- a `type(x) == "table"` guard on an untyped value keeps dynamic field fallback
  reads open.

Classification of the remaining replay clusters:

- `llm.lua` provider contract calls are true code issues under the current
  soundness rule: `provider_info = model_card.providers[1] as any` explicitly
  discards the proof that `provider_model` is `string`, then passes
  `provider_info.provider_model` to contracts requiring `model: string`.
- Artifact/message metadata field errors are true code issues unless external
  code adds a table guard or guaranteed decode. The repositories decode JSON
  into `meta`/`metadata` on success but leave the original string when decode
  fails, then downstream code accesses fields after only a truthiness guard.
- `json.decode(response.body or "")` and HTTP stream-read diagnostics are still
  unreduced package-boundary candidates. The go-lua reductions for optional
  response body fallback and guarded stream reads pass, so the observed replay
  failures are not the simple `or` transfer rule.
- Bedrock text-block parsing is not a reproduced accumulator regression. The
  guarded string accumulator reduction passes; the replay source receives
  response blocks from a dynamic API shape, so the value is `any` unless the
  external package or manifest proves the field type.
- Docker-demo fixture failures are mostly true fixture/source issues: examples
  include `state.iteration_count` being initialized only on the first-iteration
  branch before arithmetic, dynamic maps passed to stricter contracts, optional
  method receivers called without guards, and generated/vendor stubs whose
  contextual shapes do not declare fields they later read.

Current rule:

- Keep explicit `any` and `unknown` barriers sound.
- Do not suppress these diagnostics in go-lua without a failing go-lua
  reduction that proves the checker lost information it already had.
- External Wippy fixes, if desired, should be explicit guards, casts at real
  trust boundaries, or stronger manifests; they are outside this go-lua PR.

## 2026-05-19 Expression Call Evidence Closure

One real engine regression remained in the external `compress` replay: local
helper calls nested under returned table fields were not always represented as
call sites for parameter evidence. The old collector handled statement calls,
top-level assignment/return source calls, and nested calls inside call
arguments, but missed expression positions like:

- `return { field = helper(value) }`;
- `local t = { field = helper(value) }`;
- `if helper(value) then ... end`;
- numeric/generic loop header expressions;
- calls wrapped by casts or non-nil assertions.

That was a domain bug, not a reason to weaken arithmetic or `any`/`unknown`.
The correction keeps `FunctionFacts.Params` as the only parameter-evidence
authority and expands the collector to visit every call expression that occurs
inside assignment sources, return expressions, branch conditions, and loop
headers. The final implementation walks each owned expression tree once from
its CFG point, so the collector does not need a compatibility call-site channel
or per-point dedupe maps.

Regression coverage added:

- returned-table and assigned-table helper calls feed numeric parameter
  evidence;
- branch-condition and numeric-for-bound helper calls feed numeric parameter
  evidence;
- the original `compress`/test-DSL mutable resolver reduction no longer
  pollutes `tokens_to_chars`;
- guarded config update reductions verify unrelated numeric config fields stay
  non-optional when call evidence proves the updates are safe;
- existing compress/model-card reductions remain green.
- negative soundness reductions verify the checker does not accept untyped
  model-card fields as numbers, explicit `any` provider models as strings, or
  untyped response text as `string?` without a real guard, cast, or manifest.

Verification from this pass:

```text
go test ./compiler/check/infer/interproc ./compiler/check/tests/regression -count=1
go test ./... -count=1
git diff --check
go test ./compiler/check -run '^$' -bench BenchmarkCheck_LargeFunction -benchmem -count=5
```

Results:

- `go test ./... -count=1` passes.
- `git diff --check` passes.
- `BenchmarkCheck_LargeFunction` is about 1.97-2.15 ms/op, about 1.054 MB/op,
  and 10,699 allocs/op on this machine after the expression-call scan.
- A local-replace Wippy binary built from this checkout now reduces the full
  `framework/src/llm/src` replay to 9 errors: the known 6 `llm.lua` contract
  errors, 1 Bedrock dynamic text-block parser error, and 2 `compress.lua`
  arithmetic errors.

Updated classification:

- The previous nested-call evidence bug is fixed in go-lua.
- The remaining `wippy.llm.util:compress` errors are now classified as an
  external source/manifest proof gap. Replaying the real
  `wippy.llm.discovery:models` export locally shows `get_by_name` exports
  `max_tokens` and `output_tokens` as `unknown`, because registry `entry.data`
  is not typed as numeric. `compress.lua` then uses those fields in arithmetic
  after `or` defaults. go-lua must not invent numeric proof across that module
  boundary; external code should either type the registry/model-card manifest or
  coerce with `tonumber(...) or <default>` before arithmetic.
- The same soundness boundary covers the Bedrock and `llm.lua` diagnostics:
  `block.text` comes from untyped provider JSON, and `provider_info` is
  explicitly cast to `any` before being used to build contract args whose
  `model` field must be `string`.
- The remaining global replay diagnostics are still true dynamic-boundary or
  source-shape issues unless independently reduced to a failing go-lua engine
  test. Current local-replace counts: `framework/src/llm/src` 9 errors,
  `framework/src/agent/src` 11 errors, `session` 38 errors, and `docker-demo`
  60 errors.
- Standard `../scripts/verify-suite.sh` still exits non-zero because external
  lint targets fail under the Wippy repo's pinned `github.com/wippyai/go-lua
  v1.5.16` build, but the go-lua checker tests and Wippy binary build pass.
  The external counts from that official path are currently `session` 8 errors,
  `framework/src/agent/src` 8 errors, and `docker-demo` 21 errors plus
  2 warnings.

## 2026-05-19 Remaining External Error Classification Pass

The remaining official lint failures were replayed with the exact failing
targets and then reduced against the current go-lua checker. The purpose was to
separate real checker regressions from external source/manifest obligations.

Additional reductions added in this pass:

- stdlib `json.decode(response.body or "")` accepts a `string?` body fallback;
- a casted truthiness-guarded field feeds a method argument expecting `string`;
- a casted table-literal field satisfies an annotated record field;
- `#xs > 0` proves both `xs[1]` and `xs[#xs]` access in the reduced sequence
  cases;
- an error-return guard narrows the successful value before field access.

These reductions pass, so the remaining package-level errors are not the
generic transfer laws above. Current classification:

- `json.decode(response.body or "")` diagnostics in the LLM packages are still
  package-boundary issues. The local reductions for stdlib JSON, imported JSON,
  and selected HTTP methods pass; the full packages depend on external
  `http_client` response manifests and stream surfaces outside go-lua.
- `response.stream:read(4096)` is a native/manifest arity issue, not a checker
  flow regression.
- `wippy.views:renderer` casted field calls and
  `wippy.views.api:list_pages` casted table fields are covered by reductions.
  Remaining full-package errors depend on the external page-registry export
  shape and should be fixed with stronger manifests or source guards/casts in
  the views package, not by weakening go-lua.
- Metadata field errors on `meta`/`metadata` are real source-shape problems:
  empty strings are truthy in Lua, so a truthiness guard alone does not prove a
  decoded table.
- Dynamic payload and provider diagnostics (`any`/`unknown` passed to string,
  number, contract-argument, time, or typed-option APIs) remain true dynamic
  boundary errors unless the external package provides a manifest, schema
  decoder, guard, or cast.
- Docker/webscout timeout and header diagnostics are source/manifest issues:
  `options.timeout = options.timeout or 30` preserves an existing truthy string,
  so a sound checker cannot turn that into `number`.

Verification after adding these reductions:

```text
go test ./compiler/check/tests/regression -count=1
go test ./... -count=1
git diff --check
../scripts/verify-suite.sh
```

The go-lua tests and diff check pass. The official verify suite still exits
non-zero only on external lint targets: `session` 8 errors,
`framework/src/agent/src` 8 errors, and `docker-demo` 21 errors plus
2 warnings.

## 2026-05-19 Advanced Type-System Stress Regressions

Added a focused regression suite and a real-world fixture whose purpose is to
stress the current abstract-interpreter model without weakening soundness.

The Go regression suite covers:

- dynamic decode into a discriminated `Event` union after explicit `type(...)`
  guards, followed by variant-specific field access;
- `(value?, err?)` multi-return correlation through higher-order callbacks;
- fluent builder state preservation through explicit self-typed methods;
- manifest/module export of tagged results and callback parameter shapes;
- generic `Result<T>` combinators that preserve payload type parameters across
  `map`, `and_then`, nested callbacks, and discriminant narrowing;
- nested config builders with typed arrays and string maps;
- negative soundness cases where truthy string fallbacks must not become
  numbers, and a truthiness guard over `string | record` must not prove record
  field access because Lua strings are truthy.

Added fixture:

- `testdata/fixtures/realworld/advanced-type-system-stress`

The fixture runs the same laws through the repository fixture harness with
separate modules for event decoding, session creation, a metatable-style
request builder, and pipeline config. The entrypoint validates cross-module
manifest exports and includes inline `expect-error` checks for the two
soundness boundaries.

One attempted fixture assertion was intentionally tightened: assigning
`first.config.level` directly to `string` from a `{[string]: any}` config is not
sound. The fixture now proves the local value with `type(level) == "string"`
before claiming it. This is the right model boundary: the checker should infer
what is proven by control flow and manifests, not invent structure out of
dynamic `any`.

Verification:

```text
go test ./compiler/check/tests/regression -run 'TestAdvancedTypeSystem' -count=1 -v
go test . -run 'TestFixtures/realworld/advanced-type-system-stress/check' -count=1 -v
go test ./... -count=1
git diff --check
```

All checks pass.

## 2026-05-19 Exhaustiveness Warnings for Closed Matches and Channel Select

Added the checker warning the user asked for. The standard term is
**exhaustiveness checking**; the diagnostic is a warning for a
**non-exhaustive match**.

Correction made during review: the real `channel.select` exhaustiveness target
is `result.channel`, not `result.value.kind`. `result.value` is the selected
receive payload. A payload discriminator such as `result.value.kind` only makes
sense after a channel guard has already proven which channel produced the
payload. It is a separate nested discriminated-union match, not the select-arm
match itself.

Diagnostic boundary:

- The diagnostic is warning-only: `diag.ErrNonExhaustive` with
  `SeverityWarning`. It does not make type checking fail.
- Closed literal-tag proof lives in `types/narrow.ClosedDiscriminantDomain`.
- The checker hook recognizes match-like Lua `if/elseif` chains and delegates
  closed literal-tag domain proof to the narrowing domain.
- The hook also recognizes real `channel.select` result arms by indexing
  assignments of `channel.select { ch:case_receive(), ... }` and matching
  `result.channel == ch` branches against the selected channel paths.
- The warning is emitted only for match-like chains with at least two explicit
  arms and no final `else`. A single early-return guard stays silent because
  fallthrough may intentionally handle the remaining case.
- Open or dynamic cases stay silent: `any`, `unknown`, `nil`, optional
  discriminants, broad tags like `kind: string`, missing tags, non-record
  members, unextractable select channels, and select calls with default cases.

Correct `channel.select` warning sample:

```lua
local result = channel.select {
    events_ch:case_receive(),
    stop_ch:case_receive(),
    timeout_ch:case_receive(),
}

if result.channel == events_ch then
    return result.value.kind
elseif result.channel == stop_ch then
    return result.value.reason
end
```

Warning:

```text
non-exhaustive match on result.channel; missing case: timeout_ch
```

Correct complete select sample:

```lua
if result.channel == events_ch then
    return result.value.kind
elseif result.channel == stop_ch then
    return result.value.reason
elseif result.channel == timeout_ch then
    return tostring(result.value.sec)
end
```

No warning is emitted there because every selected channel arm is represented.

The nested payload-discriminant case is still supported separately:

```lua
if result.channel == events_ch then
    if result.value.kind == "message" then
        ...
    elseif result.value.kind == "tool" then
        ...
    end
end
```

That warning is about the closed `Event` payload union after the `events_ch`
guard, not about the `channel.select` arm set.

Added coverage:

- `types/narrow/discriminant_domain_test.go`
  - closed string tag domains,
  - closed numeric tag domains,
  - broad tag rejection,
  - optional tag rejection.
- `compiler/check/tests/regression/exhaustiveness_warning_test.go`
  - plain discriminated-union missing case,
  - real `channel.select` missing channel case,
  - real `channel.select` all-cases-handled no-warning case,
  - real `channel.select` single early-return guard no-warning case,
  - final `else` suppresses warning,
  - all literal variants handled suppresses warning,
  - open discriminant suppresses warning,
  - numeric discriminant missing case.
- `testdata/fixtures/narrowing/channel-select-case-exhaustiveness-warning`
  pins the real fixture harness line-level `expect-warning` for the selected
  channel case pattern.

Verification:

```text
go test ./types/narrow -run TestClosedDiscriminantDomain -count=1 -v
go test ./compiler/check/tests/regression -run TestExhaustivenessWarning -count=1 -v
go test ./compiler/check/hooks -count=1
go test . -run 'TestFixtures/narrowing/channel-select-case-exhaustiveness-warning/check' -count=1 -v
go test ./... -count=1
```

All checks pass.

## 2026-05-19 Adversarial Gradual-Typing Regressions

Added a dedicated gradual-typing regression suite and fixture. The goal is to
prove the checker is permissive where the program supplies evidence, while
remaining strict at dynamic boundaries where the evidence is incomplete.

Added Go tests:

- `TestGradualTyping_DecodesDynamicPayloadAfterStructuralProof`
- `TestGradualTyping_DispatchesGuardedUnionThroughTypedRegistry`
- `TestGradualTyping_GenericValidatedCollectionPreservesElementType`
- `TestGradualTyping_ExplicitBoundaryCastProvidesPreciseLocalType`
- `TestGradualTyping_RejectsUncheckedAnyRecordAssignment`
- `TestGradualTyping_RejectsTruthyGuardAsStructuralProof`
- `TestGradualTyping_RejectsPartiallyCheckedCollectionAsTypedArray`
- `TestGradualTyping_RejectsDynamicCallbackAtTypedFunctionBoundary`
- `TestGradualTyping_RejectsExtraFieldsAfterNarrowBoundaryCast`

The positive cases cover dynamic payload decoding, discriminated command
dispatch through typed registries, generic validation/traversal over `{any}`,
and explicit boundary casts that produce a precise local type. The negative
cases pin the soundness laws: `any` cannot be assigned to a precise record
without proof, truthiness is not structural evidence, checking one array element
does not prove the whole array, dynamic callbacks cannot satisfy typed callback
contracts, and a narrowed cast type does not leak extra dynamic fields.

Added fixture:

- `testdata/fixtures/regression/gradual-typing-adversarial`

The fixture exercises the same model through normal fixture checking and inline
`expect-error` comments. One fixture detail is intentional: generic `ok({})`
needs a typed empty-table cast (`{} :: {string}` or
`{} :: {[string]: string}`) so the empty table does not instantiate the
validation result as an unshaped table. This keeps inference strong without
guessing structure that is not present in the literal.

Verification:

```text
go test ./compiler/check/tests/regression -run 'TestGradualTyping' -count=1 -v
go test . -run 'TestFixtures/regression/gradual-typing-adversarial/check' -count=1 -v
go test ./... -count=1
git diff --check
```

All checks pass.

## 2026-05-19 Loop-Carried Gradual Refinement Regressions

Extended the adversarial gradual-typing coverage with loop-shaped programs
where precision is earned over several steps and then carried through typed
accumulators or loop state.

Added Go cases:

- `TestGradualTyping_LoopRefinesDynamicRecordsIntoTypedArray` validates a
  dynamic array by stages (`table` guard, field guards, nested tag-loop guard)
  before inserting precise `Item` records into a typed array.
- `TestGradualTyping_PairsLoopRefinesDynamicMapValuesInStages` validates
  dynamic map keys, dynamic record values, nested header maps, and then stores
  precise `Endpoint` records in a typed string-keyed map.
- `TestGradualTyping_WhileLoopCarriesOptionalRefinementThroughState` exercises
  loop-carried optional state: a discriminated event loop writes `state.name`
  and arithmetic state separately, then a post-loop nil guard proves the final
  name before string use.
- `TestGradualTyping_NestedLoopsRefineMatrixCellsBeforeAggregation` covers
  nested `ipairs` loops where row/column/value evidence builds precise cell
  records.
- `TestGradualTyping_RejectsExistentialLoopProofAsSpecificElementProof` pins a
  soundness boundary: seeing some string somewhere in a loop does not prove
  that `raw.items[1]` is a string.

The fixture `testdata/fixtures/regression/gradual-typing-adversarial` now
mirrors the staged `pairs` map refinement, nested matrix refinement, and
existential-loop negative case with inline `expect-error` coverage.

Verification:

```text
go test ./compiler/check/tests/regression -run 'TestGradualTyping' -count=1 -v
go test . -run 'TestFixtures/regression/gradual-typing-adversarial/check' -count=1 -v
go test ./... -count=1
git diff --check
```

All checks pass.

## 2026-05-19 Exhaustiveness Lint Wiring and Real-Code Probe

The exhaustiveness checker is intentionally a configurable warning class for
Wippy lint, not a globally forced diagnostic. The Wippy runtime type-checker
already had `TypeCheckRules.Exhaustive` in its cache fingerprint; that bit is
now the single authority for installing `hooks.WithExhaustiveness()`.

Design notes:

- go-lua owns the semantic pass and exposes it as `hooks.WithExhaustiveness()`;
- Wippy lint exposes the policy switch as `wippy lint --exhaustiveness`;
- typecheck cache fingerprints already include `Rules.Exhaustive`, so cached
  diagnostics cannot hide the opt-in warning state;
- default lint remains unchanged and does not emit exhaustiveness warnings unless
  the flag is requested.

Real-code proof:

1. temporarily injected a third unhandled `channel.select` case into
   `framework/src/llm/src/llm.lua`;
2. rebuilt the temporary Wippy binary against this checkout with the local
   go-lua replace;
3. ran `wippy lint --cache-reset --json --exhaustiveness` in
   `framework/src/llm/src`;
4. observed the expected warning:
   `E0014 warning: non-exhaustive match on result.channel; missing case: c`;
5. restored `llm.lua` byte-for-byte and reran the same lint command;
6. confirmed `warning_count: 0`.

Added Wippy-side coverage:

- `TestTypeChecker_ExhaustiveRuleOptIn`
- `TestTypeChecker_ExhaustiveRuleOffByDefault`
- `TestParseLintFlags_Exhaustiveness`

Verification:

```text
env GOFLAGS=-modfile=/tmp/wippy-local-replace.mod go test ./runtime/lua/code -run 'TestTypeChecker_ExhaustiveRule|TestChannelSelectNarrowing_ProcessEvent' -count=1 -v
env GOFLAGS=-modfile=/tmp/wippy-local-replace.mod go test ./cmd/wippy/cmd -run TestParseLintFlags_Exhaustiveness -count=1 -v
env GOFLAGS=-modfile=/tmp/wippy-local-replace.mod go build -o /tmp/wippy-local-replace-bin ./cmd/wippy
env GOFLAGS=-modfile=/tmp/wippy-local-replace.mod /tmp/wippy-local-replace-bin lint --cache-reset --json --exhaustiveness
```

The restored real-code lint run still reports the known nine LLM errors and no
warnings. Exhaustiveness did not backfire on real code after the temporary probe
was removed.

## 2026-05-19 Flash Convergence Rectification: No Caps

Removed the artificial convergence caps from the checker pipeline, return SCC
inference, assignment inference, query cycle handling, constraint solving,
flow solving, and numeric solving. Non-convergence is no longer handled by
"iterate N times then warn/fallback"; it must be handled by finite-height
abstract domains, idempotent transfer functions, and explicit widening.

Key design decisions:

- Interprocedural facts are a product-domain fixpoint. Captured container
  mutations now canonicalize same-path writes and join element/value types on
  the fact boundary instead of preserving duplicate mutation events.
- Return SCCs merge with `returnsummary.WidenForConvergence`, so recursive
  return summaries stabilize through domain widening instead of an unknown
  fallback.
- Assignment SCCs now test the actual SCC product state for stability after a
  sweep. A transient update inside the sweep is not a semantic change unless
  the final vector differs.
- `any` is treated as top in local inference joins and call-expectation merges.
  This prevents `T -> any -> T` oscillation while preserving soundness.
- Numeric flow has per-point widening memory: once moving numeric facts widen
  to Top, that point remains Top for the solve. This prevents `Top -> fact ->
  Top -> fact` reintroduction caused by representing Top as an absent state.

Important fixes found by real replays:

- `types/typ.TypeEquals` no longer rejects structurally equal DAG-shaped types
  just because one side shares a subnode and the other side duplicates it. The
  equality proof now relies on pair-based coinduction for compound cycles.
- Array/map mutator widening is idempotent. Re-inserting an already-known array
  element type returns the original abstract value instead of rebuilding an
  equal value and causing false "changed" reports.
- Body-local parameter evidence is treated as evidence, not as a final declared
  upper bound. A stronger whole-parameter call contract can dominate compatible
  body evidence, including record evidence compatible with a string-keyed map.

Regression coverage added:

- same-iteration captured container mutation dedupe;
- captured container mutation joins for loop/table-insert patterns;
- assignment `any` top behavior;
- assignment SCC product-stability regressions from guarded options;
- structural equality for shared DAG-shaped records;
- array/map mutator idempotence;
- numeric widening-to-Top memory;
- body evidence plus whole-parameter call expectation;
- adversarial gradual-typing and loop-carried refinement cases;
- exhaustiveness opt-in warnings.

Verification:

```text
go test ./... -count=1 -timeout 180s
go test ./compiler/check -run '^$' -bench BenchmarkCheck_LargeFunction -benchmem -count=3
env GOFLAGS=-modfile=/tmp/wippy-local-replace.mod go build -o /tmp/wippy-local-replace-verify ./cmd/wippy
timeout 60s /tmp/wippy-local-replace-verify lint --cache-reset --json --ns wippy.session.api
timeout 60s /tmp/wippy-local-replace-verify lint --cache-reset --json
```

The go-lua suite passes. The local-replace Wippy replays that previously hung
now terminate. `wippy.session.api` no longer times out; the full session target
also terminates.

Final benchmark sample:

```text
BenchmarkCheck_LargeFunction-32  382  3399067 ns/op  1084024 B/op  10938 allocs/op
BenchmarkCheck_LargeFunction-32  345  3236163 ns/op  1084162 B/op  10938 allocs/op
BenchmarkCheck_LargeFunction-32  385  3162319 ns/op  1084096 B/op  10938 allocs/op
```

Remaining verification boundary:

- The stock `../scripts/verify-suite.sh` still cannot build Wippy without a
  local replace because the Wippy checkout references the new
  `hooks.WithExhaustiveness()` while its normal module graph resolves an older
  published go-lua.
- Local-replace Wippy lint is not clean. The remaining diagnostics are finite
  and must be classified separately as source/manifest issues or precision gaps;
  this pass fixed the convergence class, not every external diagnostic.
- `tests/app` still reports an `E9999` internal-error diagnostic for
  `app.test.types:lib_inner_types`; that is an engine-facing item and should be
  investigated before claiming the global harness is clean.

## 2026-05-19 Live Task Ledger: Finish the Checker Rectification

This pass is not done until the remaining diagnostics are classified against the
current engine and every checker false positive has a regression test. The
previous "done" statement was premature: the no-cap convergence class was fixed,
but the global replay still exposed finite diagnostics that must be separated
into source/manifest issues versus engine precision bugs.

Immediate tasks:

- Re-run local-replace lint against clean replay targets where possible so dirty
  external worktree changes do not get classified as go-lua behavior.
- Classify every remaining diagnostic as one of:
  - source/manifest issue: program supplies insufficient proof, external code is
    genuinely relying on `any`, optional config fields, or untyped manifest data;
  - checker false positive: the program supplies proof and the abstract
    interpreter loses it;
  - engine internal error: the checker fails to produce a normal diagnostic.
- Reduce every checker false positive or internal error into a minimal go-lua
  regression test before changing implementation.
- Keep fixes at domain boundaries, not as per-case bridges:
  - subtype remains a pure semantic relation;
  - assignment/checking owns expected-type write validation;
  - `IndexWrite` remains the pure write-side projection query;
  - provenance/freshness proves local literal writes without weakening escaped
    or aliased values;
  - contextual callback typing seeds nested parameter facts before body
    diagnostics;
  - convergence is through finite domains, idempotent transfer, and widening,
    never iteration caps.
- Continue deferring broad static field-path write diagnostics until the
  class/metatable self-reference model has a canonical self-type story. Dynamic
  index writes are the current sound typed-write boundary.
- Verify with:
  - focused regression tests for each reduced issue;
  - `go test ./...`;
  - local-replace replay of the affected external targets;
  - `git diff --check`;
  - the stock verify-suite, with the existing pinned-module boundary documented
    rather than hidden.

Open concrete items from the latest checkpoint:

- Re-check `framework/src/llm/src` local-replace diagnostics:
  - `google/mapper.lua` dynamic recursive schema filtering;
  - `util/compress.lua` optional numeric config arithmetic;
  - `bedrock/mapper.lua` dynamic text-block parsing;
  - `llm.lua` provider contract calls where `model` is currently `any`.
- Re-check global local-replace counts for `tests/app`, `session`,
  `actor/test`, `agent/src`, `docker-demo`, `views`, `relay/test`, and
  `llm/test`, prioritizing internal errors and diagnostics that were previously
  zero.
- Resolve the `tests/app` `E9999` class before claiming the engine has no
  replay-facing crashes.
- Keep the implementation and journal aligned; no transitional bridge should
  survive unless it is the named final owner of that semantic responsibility.

## 2026-05-19 Open-Record Iterator Rectification

The clean LLM replay exposed one remaining checker false positive in
`google/mapper.lua`:

```lua
obj.multipleOf = nil
obj.additionalProperties = nil
for key, value in pairs(obj) do
    if type(value) == "table" then
        obj[key] = recursive_filter(value)
    end
end
```

The foundational bug was not Google-specific. After `type(obj) == "table"`,
the abstract table is open: known field writes may refine visible fields, but
they must not erase the unknown row tail. `KeyType` and `ValueType` for records
were ignoring `Record.Open`, so an open table with two visible nil fields was
decomposed for `pairs()` as if its only possible values were nil. That made the
`type(value) == "table"` branch look impossible and made the same-key write
target appear to accept only `nil`.

Final-domain correction:

- record key decomposition now returns `string` for open records;
- record value decomposition includes `unknown` for the open row tail;
- assignment checking has a small canonical iteration-provenance query for
  `for key, value in pairs(table)`: when a dynamic write uses the same key and
  the ordinary write projection has collapsed to a deleted/nil slot, the write
  may use the paired loop value type as the expected slot type;
- closed heterogeneous records and typed map elements remain protected by the
  ordinary write projection, so this does not become a broad "dynamic key
  accepts anything" escape hatch.

Regression coverage added:

- positive: recursive schema filtering can write `recursive_filter(value)` back
  to `obj[key]` under a `type(value) == "table"` guard;
- negative: a closed record cannot use a `pairs()` loop to rewrite a numeric
  field through a dynamic key with `tostring(value)`;
- negative: a typed map `{[string]: Item}` cannot write `{}` back to an
  `Item` slot under a broad table guard;
- query-level tests: open records decompose to `string` keys and include
  `unknown` in value iteration.

Clean replay result after rebuilding the local-replace Wippy binary:

- `framework/src/llm/src`: 9 errors, down from 10. The Google recursive schema
  filtering error is gone.
- `framework/src/llm/test`: same 9 errors, inherited from the source package.
- `framework/src/actor/test`: 0 errors.
- `tests/app`: 2 errors, both untyped overlay URL values flowing into
  `http.get(url: string, ...)`.
- `framework/src/views`: 2 errors:
  - `api/list_routes.lua` writes `page.id: any` into `{[string]: string}`;
  - `page_registry.lua` passes untyped `resource_id: any` to a helper requiring
    `string?`.

Classification of remaining clean replay diagnostics:

- LLM Bedrock text-block parsing: source/manifest proof gap. The provider
  response shape gives `block.text` as `any`; truthiness alone does not prove a
  string for `parse_text_tool_call(text: string?)`.
- LLM `compress.lua` arithmetic: source/API proof gap. Exported
  `configure(new_config)` accepts untyped external values, so numeric config
  fields cannot be treated as permanently numeric without an annotation or
  runtime guard.
- LLM provider contract calls: source/manifest proof gap. `provider_info` is
  cast to `any`, and `provider_info.provider_model` is not proven string before
  calling contracts requiring `model: string`.
- tests/app overlay URLs: source proof gap. `(args and args.url) or fallback`
  is `any | string`; a string guard is required before `http.get`.
- views route/resource diagnostics: source proof gaps. Dynamic registry data
  needs guards before flowing into string maps or string helper parameters.

Verification for this pass:

```text
go test ./compiler/check/tests/regression -run 'TestExternalLint_(PairsSchemaFilterWritesRecursiveValueBackToSameKey|RejectsPairsWriteThatChangesClosedFieldDomain|RejectsPairsWriteThatWeakensTypedMapElement|UntypedPageIDRequiresStringProofForAccessibleRoutes|GuardedPageIDFeedsAccessibleRoutes|DynamicResourceIDsRequireStringProof|GuardedResourceIDsFeedQualifier|DynamicResponseTextRequiresStringProof|AnyProviderModelRequiresStringProof|DynamicModelCardNumericFieldsRequireProof)' -count=1 -v
go test ./types/query/core -run 'Test(KeyType|ValueType|IndexWrite)' -count=1
go test ./compiler/check/domain/iteration ./compiler/check/hooks ./compiler/check/api -count=1
go test ./... -count=1 -timeout 300s
git diff --check
../scripts/verify-suite.sh
env GOFLAGS=-modfile=/tmp/wippy-local-replace.mod go build -o /tmp/wippy-local-replace-verify ./cmd/wippy
timeout 90s /tmp/wippy-local-replace-verify lint --cache-reset --json
```

All go-lua tests pass. Clean local-replace replays terminate and the confirmed
engine false positive is removed. The remaining replay diagnostics are strict
dynamic-boundary errors unless later external code supplies stronger manifests
or runtime guards.

The stock verify script still exits non-zero at the same external module
boundary:

```text
== go-lua checker tests ==
... pass ...

== build wippy binary ==
runtime/lua/code/typecheck.go:397:29: undefined: hooks.WithExhaustiveness
skip lint checks: failed to build /tmp/wippy-local
```

This is not a go-lua checker regression from this pass; the normal Wippy module
graph still resolves a published go-lua version older than the local
`hooks.WithExhaustiveness()` API.

## 2026-05-19 Flash Rectification Continuation: Evidence, Writes, and Replay

This continuation fixed the checker-suite regressions that appeared after the
first "hard annotation authority" pass. The earlier rule was too blunt: it
treated every source annotation as hard, which removed legitimate soft
structural evidence from function bodies. The final rule is now:

- hard top annotations such as `any` and `unknown` remain authoritative;
- hard concrete annotations remain authoritative;
- soft structural annotations keep their container shape while evidence refines
  the element/value domain;
- call evidence records explicit `nil` arguments, because nil is a real branch
  fact, not absence of evidence;
- public call contracts must not be specialized to tuple arity just because one
  call passed a literal table.

Concrete examples now covered by tests:

- `param: {any}` plus a literal tuple call refines to an array element domain,
  not a fixed tuple contract, so a later `{}` call still type-checks.
- `merge_context(nil, {current_item = item, item_index = index})` records the
  nil base argument, so the base-copy branch is unreachable for that observed
  local call and the resulting map keeps a string key domain.
- maps remain invariant for concrete value domains, but a map value may widen
  to expected `any`, matching the existing record-field widening rule.

Write-side and table-shape corrections in this continuation:

- `IndexDelete` is the canonical write-query for `t[k] = nil`. Map nil writes
  delete entries; required record fields still reject deletion.
- optional record fields accept optional source values in table literals,
  modeling Lua nil-as-absence for optional fields.
- soft parameter evidence is applied to function-body overlays but hard
  annotations remain annotated in flow.

Additional regression coverage added:

- dynamic runner IDs must be proven string before calling `short_name`;
- guarded runner IDs feed the string contract;
- string metadata cannot be indexed as a record without a table proof;
- metadata table guards allow structured field access;
- manifests without assertion summaries do not narrow nil-only locals;
- untyped/variadic command handlers cannot enter typed handler maps through a
  dynamic key;
- typed command handlers can enter the same registry.

Current clean local-replace replay matrix with
`/tmp/wippy-clean-head-local-replace`:

```text
/tmp/wippy-clean-head/tests/app                 2 errors
/tmp/session-clean-head                         45 errors
/tmp/framework-clean-head/src/test              0 errors
/tmp/framework-clean-head/src/actor/test        4 errors
/tmp/framework-clean-head/src/agent/src         14 errors
/tmp/framework-clean-head/src/bootloader/src    0 errors
/tmp/framework-clean-head/src/bootloader/test   0 errors
/tmp/framework-clean-head/src/llm/src           14 errors
/tmp/framework-clean-head/src/llm/test          14 errors
/tmp/framework-clean-head/src/migration         0 errors
/tmp/framework-clean-head/src/relay/test        0 errors
/tmp/framework-clean-head/src/views             2 errors
```

Classification of remaining clean replay diagnostics:

- `tests/app` overlay URL errors are source proof gaps: URL values are `any`
  unless guarded before `http.get(url: string, ...)`.
- session `""` / `""?` metadata field errors are source/manifest proof gaps:
  string defaults are truthy strings, not decoded metadata records.
- session and actor `test.not_nil(...)` nil-index errors are locked
  manifest/source-boundary cases. Source-exported assertion modules with
  inferred summaries narrow correctly; a manifest without a summary does not
  narrow nil-only locals by design.
- session `message_repo` number error is a source proof gap: an untyped `limit`
  flows to a numeric contract.
- session `command_bus` handler error is a source proof gap: an untyped
  variadic handler is assigned into a typed handler map; the typed adapter form
  is covered and passes.
- session `control_handlers` errors are dynamic `any` op payloads flowing into
  typed handlers without proof.
- session `start_tokens_test` is an intentional invalid negative call.
- session `checkpoint` length-guarded query shape is already covered by
  real-shaped go-lua regressions; the clean replay failure depends on external
  package/manifest shape.
- LLM Bedrock text-block parsing is a source/manifest proof gap: `block.text`
  is `any` before the string contract.
- LLM `compress.lua` arithmetic is a source/API proof gap: exported
  `configure(new_config)` can mutate numeric config fields with untyped values.
- LLM provider contract calls are source/manifest proof gaps:
  `provider_info.provider_model` is `any` before contracts requiring `string`.
- LLM and actor provider/test nil-index diagnostics are the same locked
  assertion-summary boundary.
- views route/resource diagnostics are source proof gaps: dynamic registry IDs
  need string guards before flowing into string maps or string helper params.

Verification for this continuation:

```text
go test ./compiler/check/... -count=1
go test ./types/... -count=1
go test ./... -count=1 -timeout 300s
git diff --check
../scripts/verify-suite.sh
env GOFLAGS=-modfile=/tmp/wippy-local-replace.mod go build -buildvcs=false -o /tmp/wippy-clean-head-local-replace ./cmd/wippy
clean local-replace replay matrix above
go test ./compiler/check -run '^$' -bench BenchmarkCheck_LargeFunction -benchmem -count=3
```

The stock verify suite still passes go-lua checker tests and then stops at the
known external module boundary:

```text
runtime/lua/code/typecheck.go:397:29: undefined: hooks.WithExhaustiveness
skip lint checks: failed to build /tmp/wippy-local
```

Benchmark sample:

```text
BenchmarkCheck_LargeFunction-32  753  1883468 ns/op  988835 B/op  10030 allocs/op
BenchmarkCheck_LargeFunction-32  348  5596039 ns/op  989341 B/op  10030 allocs/op
BenchmarkCheck_LargeFunction-32  174  6461779 ns/op  989461 B/op  10031 allocs/op
```

The wall-time variance is high on this host, but allocations are materially
lower than the earlier recorded 1.44 MB / 20.5k allocs and the post-convergence
3.2-3.4 ms / 1.08 MB / 10.9k allocs samples. No iteration cap was reintroduced.

### 2026-05-19 field-path type guard parent materialization

The latest false-positive class was not an interproc fact problem. It was a
local abstract-domain shape problem: a proven fact about a field path was not
being reflected back into the parent container. The concrete failing shape was:

```lua
if type(op) == "table" and type(op.from_pid) == "string" then
    handle(op) -- handle expects {from_pid: string}
end
```

The checker knew `op.from_pid` was a string, but the parent value `op` still
looked like unstructured `any/table` at the function-call boundary. That made
the call look like an unproved `any` to record-contract flow.

The design correction is a canonical narrowing operator, not a checker-side
special case:

- `types/narrow.ByFieldTypeKey` owns positive runtime type guards on fields;
- `types/flow/domain.TypeDomain` uses it when applying path facts to flow state;
- `types/constraint.Solver` uses the same operator when applying constraints;
- `types/narrow.LiteralFromTypeKey` is the single literal-key decoder used by
  both field-literal and type-key paths.

Soundness rules:

- `type(t.k) == "string"` in a table-proven branch materializes/refines `t` to
  an open record containing `k: string`.
- `t.k == nil` materializes/refines `t` to an open record containing `k: nil`.
  This is the Lua absence proof required by optional record fields.
- Hash keys that resolve to literal singletons still route through
  `ByFieldLiteral`, so discriminated unions remain exact.
- Broad builtin type-key refinement must not be used as a literal-discriminant
  replacement. That avoids impossible intersections such as `true & false`.
- Closed records without the proven field remain unsatisfiable; no open-field
  escape is invented.

This is the intended abstract-interpreter mental model:

1. leaf-path facts are first-class facts;
2. when a leaf fact proves a field value, the parent shape must be updated in
   the same product domain;
3. contract checking reads the parent shape from that domain;
4. no legacy bridge, fallback channel, or post-hoc compatibility projection is
   involved.

Regression coverage added in this pass:

- `ByFieldTypeKey(any, "from_pid", string)` materializes
  `{from_pid: string, ...}`;
- nil field proofs materialize optional-field absence;
- open records refine missing fields;
- unions refine existing field domains;
- closed records with missing fields become `never`;
- flow-domain `HasType(parent.field)` and `IsNil(parent.field)` update
  `parent`;
- constraint-solver `HasType(parent.field)` and `IsNil(parent.field)` update
  `parent`;
- untyped limit/control/start-option cases still fail without proof;
- guarded limit/control/start-option cases pass with proof.

Verification after this correction:

```text
go test ./types/narrow ./types/flow/domain ./types/constraint \
  -run 'Test(ByFieldTypeKey|TypeDomain_Apply(IsNil|HasType)OnField|Solver_ApplyToSingle_(HasType|IsNil)OnField)' \
  -count=1 -v

go test ./compiler/check/tests/regression \
  -run 'TestExternalLint_(UntypedLimitRequiresNumberProof|GuardedLimitFeedsNumberContract|DynamicControlPayloadRequiresTypedProof|GuardedControlPayloadFeedsTypedHandler|StartOptionsRejectsPlainString|GuardedStartOptionsFeedsOptionalRecord)' \
  -count=1 -v

go test ./... -count=1 -timeout 300s
git diff --check
go test ./compiler/check -run '^$' -bench BenchmarkCheck_LargeFunction \
  -benchmem -count=3
```

All of the above passed.

Benchmark sample:

```text
BenchmarkCheck_LargeFunction-32  819  1436389 ns/op  988745 B/op  10034 allocs/op
BenchmarkCheck_LargeFunction-32  830  1480187 ns/op  988940 B/op  10034 allocs/op
BenchmarkCheck_LargeFunction-32  816  1495422 ns/op  988964 B/op  10034 allocs/op
```

The stock `../scripts/verify-suite.sh` result is not a clean verdict on this
checkout. It builds `/tmp/wippy-local` from `/home/wolfy-j/wippy/wippy`, whose
module graph is still pinned to `github.com/wippyai/go-lua v1.5.16`:

```text
dep github.com/wippyai/go-lua v1.5.16
```

That pinned-binary run passed go-lua checker tests and built the binary, then
exited non-zero on external lint targets:

```text
/home/wolfy-j/wippy/session                  errors=8  warnings=0
/home/wolfy-j/wippy/framework/src/agent/src  errors=8  warnings=0
/home/wolfy-j/wippy/docker-demo              errors=21 warnings=2
```

The current-PR local-replace binary was rebuilt with:

```text
dep github.com/wippyai/go-lua v1.5.16 => /home/wolfy-j/wippy/go-lua (devel)
```

Local-replace sampling still reports external diagnostics, but the remaining
classes are source or manifest proof gaps:

- untyped `any` values passed to `string`, `number`, `Time`, or record
  contracts;
- string defaults such as `""` indexed as metadata records;
- manifest-only assertion helpers without assertion-effect summaries;
- dynamic provider/model/config values passed to typed LLM contracts;
- exported config mutation that can invalidate numeric reads;
- intentional negative tests such as passing `"not a table"` to a record
  contract.

No current engine false-positive class remains from this pass. If a later
external diagnostic is reclassified as an engine issue, the rule is unchanged:
first reduce it into a go-lua regression, then fix the canonical domain or query
owner. Do not widen `any` into typed contracts, do not assume assertion effects
from names, and do not add bridge/fallback facts.

## 2026-05-19 Predicate Effect And Branch Product Checkpoint

The next external replay reclassified one docker/dataflow diagnostic as a real
engine issue, not source code:

```lua
local function validate_batch_size(size)
  return type(size) == "number" and size > 0 and size <= 1000
end

local batch_size = config.batch_size or DEFAULTS.BATCH_SIZE
if not validate_batch_size(batch_size) then
  return nil
end

for i = 1, #items, batch_size do
  ...
end
```

The old implementation had two foundational problems:

- direct predicate calls in conditions did not consume inferred function
  refinements, while variables assigned from predicate calls used a separate
  predicate-link path;
- the falsy side of a one-sided proof could be approximated by negating the
  truthy proof. For predicates such as `type(x) == "number" and x > 0`, that is
  unsound: a false result does not prove `x` is not a number.

The correction is the final abstract-interpreter shape for branch facts:

```text
branch(expr) -> { truthy: Condition, falsy: Condition }
```

The branch extractor now computes this product compositionally:

- `not e` swaps the truthy/falsy products;
- `a and b` uses short-circuit transfer:
  `truthy = a.truthy & b.truthy`,
  `falsy = a.falsy | (a.truthy & b.falsy)`;
- `a or b` uses:
  `truthy = a.truthy | (a.falsy & b.truthy)`,
  `falsy = a.falsy & b.falsy`;
- equality and inequality use their canonical positive operators on one side
  and the opposite relation on the other;
- ordered comparisons only prove numeric/string operand type on the truthy
  side. Their falsy side is intentionally not the negation of that type proof;
- direct predicate calls instantiate the same `FunctionRefinement` product used
  by assigned predicate results;
- assigned predicate variables apply only stored `OnTruthy` and `OnFalsy`
  channels. Missing `OnFalsy` means no argument narrowing, not
  `not OnTruthy`.

Return-expression effect inference was split into three channels:

```text
OnReturn = facts guaranteed after the callee returns normally
OnTrue   = facts guaranteed when the returned value is truthy
OnFalse  = facts guaranteed when the returned value is falsy
```

This matters because callable type casts and assertion-style helpers are normal
return effects, while predicate helper results are truthiness effects. A wrapper
around `Point(x)` must publish `OnReturn: x is Point`; a wrapper around
`type(x) == "number" and x > 0` must publish `OnTrue: x is number` without
inventing a useful `OnFalse`.

Regression coverage added in this pass:

- a local `validate_batch_size` predicate narrows an `any` batch size after
  early return and supports numeric `for` steps;
- direct predicate calls narrow their arguments on the truthy branch;
- assigned predicate results narrow their arguments on the truthy branch;
- direct and assigned one-sided predicates do not narrow the falsy branch;
- logical predicate paths narrow loop bounds through `and`;
- logical `else` paths do not over-narrow when only one disjunct proves the
  predicate;
- callable type-call wrappers still publish normal-return cast refinements;
- plain identifier returns still allow guard-established path conditions to
  publish assertion-style `OnReturn` refinements.

Verification after this correction:

```text
env GOCACHE=/tmp/go-build go test ./compiler/check/... -count=1
env GOCACHE=/tmp/go-build go test ./... -count=1 -timeout 300s
env GOCACHE=/tmp/go-build go test ./compiler/check -run '^$' \
  -bench BenchmarkCheck_LargeFunction -benchmem -count=3
```

All passed.

Benchmark sample:

```text
BenchmarkCheck_LargeFunction-32  811  1452509 ns/op  989363 B/op  10017 allocs/op
BenchmarkCheck_LargeFunction-32  831  1465673 ns/op  989516 B/op  10017 allocs/op
BenchmarkCheck_LargeFunction-32  817  1474103 ns/op  989520 B/op  10017 allocs/op
```

External local-replace replay after rebuilding
`/tmp/wippy-golua-predicate-current` against this checkout:

```text
/tmp/framework-clean-head/src/llm/src     14 errors
/tmp/framework-clean-head/src/views        2 errors
/tmp/wippy-clean-head/tests/app            2 errors
/tmp/session-clean-head                   45 errors
/tmp/framework-clean-head/src/actor/test   4 errors
/tmp/framework-clean-head/src/agent/src   14 errors
/tmp/framework-clean-head/src/test         0 errors
/tmp/framework-clean-head/src/bootloader/src 0 errors
/home/wolfy-j/wippy/docker-demo           66 errors
```

The docker/dataflow predicate false positive is fixed in the real replay:
`userspace.dataflow.node.parallel` dropped from 4 errors to 3, and the removed
diagnostic was the `batch_size` numeric-loop error. The remaining three
parallel diagnostics are map-key shape issues:

```text
argument 3: expected {[string]: any}?, got {[any]: any}?
```

Those remain classified as source/manifest proof gaps unless a smaller
go-lua-only reduction proves otherwise. The engine rule remains the same:
predicate/effect facts must flow through the canonical branch product and
function-refinement channels, not through call-site expectation backflow or
name-based special cases.

Official `../scripts/verify-suite.sh` result after this correction:

```text
go-lua checker tests: pass
wippy binary build: pass
wippy/tests/app: 0
session: 8 errors
framework/src/test: 0
framework/src/actor/test: 0
framework/src/agent/src: 11 errors
framework/src/bootloader: 0
docker-demo: 21 errors, 2 warnings
framework/src/llm/src: 0
framework/src/llm/test: 0
framework/src/migration: 0
framework/src/views: 0
framework/src/relay/test: 0
```

The script still exits non-zero because those external lint targets are part of
the Wippy checkout, not because go-lua tests or binary build failed. This is the
same pinned-suite caveat recorded above: use local-replace replay for this
checkout's checker behavior and official verify for the repository gate shape.

## 2026-05-19 Table Mutation And Static-Key Rectification

The next local-replace replay found two real engine false positives after the
predicate/effect checkpoint. Both were domain-boundary bugs, not reasons to add
new fact channels or compatibility bridges.

### Deletion Is Not Element Evidence

`userspace.docker.service:worker` failed after a table slot was initialized,
used, and later removed:

```lua
if not active[cid] then
  active[cid] = {}
  run_interactive(active, cid, c)
  active[cid] = nil
end
```

The old overlay merge treated `active[cid] = nil` as a value assignment and
merged `nil` into the map element domain. That polluted later writes and
produced an impossible element requirement. In Lua, `t[k] = nil` deletes the
slot; map reads are already optional because a key can be absent. Therefore the
write-side effect lattice is:

```text
index write with non-nil value -> element evidence
index write with nil-only value -> deletion effect, no element evidence
```

The canonical implementation now drops nil-only indexer mutations when building
return/overlay map evidence. This keeps deletion semantics local to the table
mutation domain instead of encoding absence as a stored value type.

Regression coverage:

- nil-only indexer writes do not create map evidence;
- mixed delete/write effects keep only the non-nil write value;
- guarded map-slot initialization still accepts valid element writes;
- invalid element writes to typed maps still fail;
- captured/async map parameter writes stay precise while later deletes do not
  poison the parameter evidence.

### Empty String Is A Static Key

`userspace.dataflow:client` failed on the root-output merge:

```lua
local outputs = {}
...
outputs[key] = content
...
outputs[""] = root_output
```

The extractor was using `keySeg.Name == ""` as the sentinel for "not a static
key". That is not a valid discriminator because `""` is a valid Lua table key.
The result was a bogus dynamic indexer assignment for `outputs[""]`, which
created `{[string]: never}` evidence and then rejected the real write as
`any -> never`.

The corrected model is explicit:

```text
static-key extraction returns (segment, ok)
ok=false means dynamic/unknown key
segment payload may be empty because [""] is a valid static key
```

The assignment extractor now tracks a separate `hasStaticKeySeg` boolean, and
the shared path segment helper accepts empty string indexes as
`SegmentIndexString{Name: ""}`. This keeps path identity canonical and prevents
empty string keys from falling into the dynamic map-widening path.

Regression coverage:

- `StaticAttrKeySegment("")` and `StaticTableFieldKeySegment("")` produce a
  static string-index segment;
- a dataflow-style output map can receive dynamic named outputs and then the
  root output at `[""]` without producing `never`;
- the same fixture keeps dynamic row content as `any`, proving the fix is not a
  narrow literal-only special case.

### Truthiness Must Remove All Nil Layers

One more precision repair was kept in the core narrowing library: `RemoveNil`
now recurses through optional wrappers. Constructors normally canonicalize nested
optionals, but imported/derived field shapes can still present equivalent
nil-capable layers. Truthy narrowing and `or` fallback must produce the non-nil
payload, not leave one optional layer behind.

Regression coverage uses deliberately non-canonical optional wrappers so the
test protects the narrowing algorithm rather than the constructor normalizer.

### Replay Classification After These Fixes

Targeted local-replace replay with `/tmp/wippy-golua-current-verify`:

```text
docker-demo userspace.docker.service: 0 errors, 0 warnings
docker-demo userspace.dataflow:        0 errors, 0 warnings
docker-demo full replay:              63 errors, 0 warnings
```

Official `../scripts/verify-suite.sh` after this correction:

```text
go-lua checker tests: pass
wippy binary build: pass
wippy/tests/app: 0
session: 8 errors
framework/src/test: 0
framework/src/actor/test: 0
framework/src/agent/src: 8 errors
framework/src/bootloader: 0
docker-demo: 21 errors, 2 warnings
framework/src/llm/src: 0
framework/src/llm/test: 0
framework/src/migration: 0
framework/src/views: 0
framework/src/relay/test: 0
```

The two fixed namespaces were engine false positives and are now clean.
Remaining sampled diagnostics remain source/proof-boundary issues, not current
engine regressions:

- `wippy.llm.util:compress` exposes an untyped public `compress.configure`
  method that can write arbitrary values into numeric `CONFIG` fields. The
  regression suite already has both the negative untyped-config case and the
  positive typed-config case.
- `wippy.llm.claude:client` in docker-demo calls `json.decode(response.body)`
  where the vendored response body is `string?`; unlike other copies, that
  source has no `or ""` fallback or cast at the decode site.
- the remaining full-replay classes are dynamic `any` values crossing typed
  contracts, stale/vendor source shapes, intentionally string metadata used as a
  record, or manifest/source proof gaps already represented by negative
  regressions.

The design rule remains unchanged: reduce every suspected false positive to a
go-lua test first, then fix the owning abstract domain. Do not add fallback fact
channels, bridge projections, or name-specific repairs.

## 2026-05-19 Replay Reclassification And Soundness Guardrails

After the table-mutation/static-key fixes, I replayed the current local
checker through `/tmp/wippy-golua-current-verify`. `go version -m` confirms the
binary is built with:

```text
github.com/wippyai/go-lua v1.5.16 => /home/wolfy-j/wippy/go-lua (devel)
```

The current go-lua suite passes:

```text
env GOCACHE=/tmp/go-build go test ./... -count=1 -timeout 300s
```

Official `../scripts/verify-suite.sh` after the final regression additions:

```text
go-lua checker tests: pass
wippy binary build: pass
wippy/tests/app: 0
session: 8 errors
framework/src/test: 0
framework/src/actor/test: 0
framework/src/agent/src: 11 errors
framework/src/bootloader: 0
docker-demo: 21 errors, 2 warnings
framework/src/llm/src: 0
framework/src/llm/test: 0
framework/src/migration: 0
framework/src/views: 0
framework/src/relay/test: 0
```

The script still exits non-zero because those external lint diagnostics are
part of the pinned Wippy verification target set. The go-lua checker suite and
binary build pass.

Focused local-replace external lint results:

```text
/home/wolfy-j/wippy/session                 37 errors, 0 warnings
/home/wolfy-j/wippy/framework/src/agent/src 11 errors, 0 warnings
/home/wolfy-j/wippy/docker-demo             63 errors, 0 warnings
```

These are not the fixed table-mutation/static-key false-positive classes. The
remaining sampled classes classify as source, manifest, or dynamic-boundary
issues unless a smaller go-lua-only reduction later proves otherwise.

Important source/version detail: the lint targets do not always use the live
framework checkout. The lock files point at vendored modules and packed `.wapp`
dependencies. For example, docker-demo uses vendored `wippy/llm 0.4.8`, whose
Claude client contains:

```lua
json.decode(response.body)
```

The live framework source has the safer fallback, but that is not the source
being linted by docker-demo. The checker must reject the vendored direct
nullable-body call, and must accept the current-source fallback:

```lua
json.decode(response.body or "")
```

Regression coverage now protects both sides:

- manifest-provided `http_client.Response.body: string?` plus
  `json.decode(response.body or "")` is accepted;
- direct `json.decode(response.body)` with `body: string?` is rejected;
- explicit method-call casts still satisfy parameter contracts;
- constant numeric table fields stay non-nil when no untyped mutator can poison
  them;
- untyped config mutation can invalidate numeric config fields and is rejected;
- truthy `"" | record` metadata does not become a guaranteed record, because
  empty strings are truthy in Lua;
- arbitrary-key dynamic maps are not accepted as string-key maps once numeric
  key evidence exists.

Representative classifications:

- `wippy.llm.util:compress` remains soundly rejected in vendored packages when
  public untyped `configure(new_config)` can write arbitrary values into
  numeric `CONFIG` fields. The typed-config positive case still passes.
- `wippy.llm.*:client` nullable-body diagnostics in older vendored packages
  are source/version issues when the call has no fallback.
- `wippy.views:renderer` in older vendored packages calls
  `tmpl:render(page.template_name, ...)` without the current-source cast; the
  explicit-cast reduction passes.
- `userspace.dataflow.node.parallel` context-map diagnostics come from dynamic
  `step.context` / arbitrary key copying into APIs requiring `{[string]: any}?`;
  the checker must not invent string keys.
- `session` artifact metadata diagnostics are source-boundary issues: decoded
  JSON metadata and SQL/string metadata need explicit shape proof. Lua truthiness
  does not turn an empty string into a metadata record.

The design invariant is unchanged and is now covered more directly: the
abstract interpreter may refine values only from semantic evidence in the
canonical domains. It must not use optimistic compatibility bridges, call-site
wishful typing, or old fallback fact projections to hide nullable values,
dynamic keys, or untyped mutation.

## 2026-05-20 Replay Boundary: No Heuristic Row-Shape Magic

I rechecked the current local-replace replay with
`/tmp/wippy-golua-fb0238a7`, which is built as:

```text
github.com/wippyai/go-lua v1.5.16 => /home/wolfy-j/wippy/go-lua (devel)
```

Current local-replace external lint remains:

```text
/home/wolfy-j/wippy/session                 37 errors, 0 warnings
/home/wolfy-j/wippy/framework/src/agent/src 11 errors, 0 warnings
/home/wolfy-j/wippy/docker-demo             63 errors, 0 warnings
```

The important convergence result still holds: the current local-replace
docker-demo replay has zero non-convergence warnings. The old
`inter-function fixpoint did not converge; unstable channels:
[InterprocFacts]` class was from the pinned verification binary, not this
checkout.

I reduced the session checkpoint diagnostic again because it is the easiest
place to accidentally justify a hack. The protected cases now include:

- imported fluent metatable query builders;
- repository fallback `contexts or {}` after an error-return pair;
- a separate repository module feeding a separate reader module;
- SQL-builder-shaped rows where `executor:query()` returns
  `{[string]: any}[]`;
- `table.sort` before reading `existing_summaries[1].text`;
- an untyped tool argument guarded only by `if not args.session_id then`.

All of those reduced go-lua tests pass. Therefore this is not evidence for a
new compatibility bridge, special-case length rule, or checkpoint-specific
repair.

The SOTA boundary is:

- The abstract interpreter may use length guards to prove indexed array
  presence.
- It may use error-return correlation and nil repair to remove impossible nil
  paths.
- It may propagate those facts through exported function summaries and fluent
  metatable receivers.
- It must not infer structured SQL row records from arbitrary SQL strings or
  dynamic database results unless the boundary provides a typed effect or typed
  manifest.

If we ever want SQL-builder row-shape inference, the final design is not an
ad-hoc checker heuristic over `sql.builder.select(...)`. It is a typed external
effect owned by the manifest/builtin boundary, for example:

```text
select("id", "text") -> SelectBuilder<Row{id: any, text: any}>
run_with(db)         -> QueryExecutor<Row>
query()              -> ({Row}, error?)
```

That effect would be a normal abstract-domain input, cached through the same
Salsa/query boundary as other builtin facts. It would not add another fact
channel and would not bypass canonical product-domain convergence.

Current classification rule for the replay:

- keep adding go-lua reductions for any diagnostic that looks like a precision
  regression;
- implement an engine fix only when the reduced case fails;
- otherwise record the external diagnostic as source, manifest, version, or
  dynamic-boundary proof debt;
- do not hide real dynamic-boundary errors with casts, fallbacks, compatibility
  projections, iteration caps, or source-specific helpers.

## 2026-05-20 Contextual Callback Rectification

The docker-demo replay exposed a real checker gap in:

```lua
str:gsub(pattern, function(c)
    fields[#fields + 1] = c
end)
```

The old stdlib type for `string.gsub` accepted `repl: any`. That was soundly
permissive for call acceptance but it erased the callback contract, so the
capture parameter was checked as `any` and assigning it into `{string}` became
a false positive. Lua's semantics give us a real contract here: replacement
callbacks receive the full match/captures as strings and may return
string/number/false/nil.

The first attempted shape was too broad: collecting contextual signatures for
all call arguments, including nested table callbacks, increased
`framework/src/agent/src` from 11 to 24 local-replace errors. That was a design
mistake, not an acceptable migration step. It globally stored too much
call-site context and over-constrained test harness callback tables.

Final shape:

- `string.gsub` now has a precise replacement union:
  string, string-key replacement table, or replacement callback.
- direct function-literal callback arguments are probed with a shallow
  signature only to discover the callee's expected callback contract;
- the actual callback body is then synthesized with that expected parameter
  type, while its body still contributes return types for generic inference;
- nested table callbacks are not globally context-typed from arbitrary call
  schemas;
- contextual callback signatures are stored only for direct callback literals
  whose callee provides a real callback function type.

This keeps the final model bidirectional and call-local instead of adding a
new fact channel. The compatibility view is not another fact source: the call
checker derives expected argument types from the canonical function type, and
the function-literal checker uses that expected type for the one literal at
that call site.

Regression protection now covers:

- `string.gsub` callback captures flowing into `{string}`;
- valid `gsub` replacement forms: string, table, callback returning
  string/number/false/nil;
- invalid callback replacement returns;
- generic result combinators where callback returns infer type parameters;
- the existing iterator/result fixtures that require callback return inference.

Local-replace replay with `/tmp/wippy-golua-narrow-callback`:

```text
/home/wolfy-j/wippy/session                 37 errors, 0 warnings
/home/wolfy-j/wippy/framework/src/agent/src 11 errors, 0 warnings
/home/wolfy-j/wippy/docker-demo             61 errors, 0 warnings
```

The two docker-demo errors removed from the previous 63-error baseline are the
`string.gsub` callback capture false positives in `agents_by_name.lua` and
`models_by_name.lua`. The 24-error spike in the agent replay is gone.

Performance note from `BenchmarkCheck_LargeFunction` after the callback fix:

```text
~2.0-3.35 ms/op, ~1.00 MB/op, 10223 allocs/op
```

Time is noisy on this machine, but allocations remain much lower than the
earlier ~20.5k alloc/op benchmark shape. The callback fix should not be
expanded into whole-program call-argument signature collection; that is both
slower and less precise.

## 2026-05-20 Callback Helper Consolidation And Degradation Audit

After the degradation report I reran both sides of the local-replace checkpoint
on the same external worktrees.

Saved checkpoint binary `/tmp/wippy-golua-fb0238a7`:

```text
/home/wolfy-j/wippy/session                 37 errors, 0 warnings
/home/wolfy-j/wippy/framework/src/agent/src 11 errors, 0 warnings
/home/wolfy-j/wippy/docker-demo             63 errors, 0 warnings
```

Current rebuilt binary `/tmp/wippy-golua-current-callback`:

```text
/home/wolfy-j/wippy/session                 37 errors, 0 warnings
/home/wolfy-j/wippy/framework/src/agent/src 11 errors, 0 warnings
/home/wolfy-j/wippy/docker-demo             61 errors, 0 warnings
```

So the active code is not the 11-to-24 agent regression. That spike belonged to
the rejected broad call-site-signature collection. The final patch removes that
shape and preserves only the direct callback contract needed by `string.gsub`.

I also collapsed the duplicated contextual-function helper into
`phase/core.ExpectedFunctionLiteralSignature`. That is now the single local
rule for turning an expected type into a function-literal signature, including
arity-compatible function members inside a union. The call synthesizer now
skips the contextual probe entirely when a call has no direct function literal
arguments, and it reuses already-synthesized non-callback argument types during
the probe path. That keeps the hot path simpler and avoids double synthesis of
ordinary arguments.

Current benchmark:

```text
BenchmarkCheck_LargeFunction-32  658  1764196 ns/op  1001938 B/op  10188 allocs/op
BenchmarkCheck_LargeFunction-32  694  1800709 ns/op  1002041 B/op  10188 allocs/op
BenchmarkCheck_LargeFunction-32  651  1716371 ns/op  1001966 B/op  10188 allocs/op
```

Official `../scripts/verify-suite.sh` still builds Wippy without the local
go-lua replacement. In that pinned-dependency mode, go-lua checker tests pass
and the Wippy binary builds, then external lint exits non-zero with:

```text
/home/wolfy-j/wippy/session                 8 errors, 0 warnings
/home/wolfy-j/wippy/framework/src/agent/src 7 errors, 0 warnings
/home/wolfy-j/wippy/docker-demo             21 errors, 2 warnings
```

Those pinned counts are useful for monitoring but are not the proof surface for
this go-lua patch. The proof surface for this patch is the local-replace replay
above plus the go-lua regression suite.

## 2026-05-20 Flash Migration: Abstract Transfer Evidence Boundary

Current migration state after the latest flash slice:

- `compiler/check/flowbuild` is gone. The final transfer owner is
  `compiler/check/abstract/transfer`.
- `compiler/check/domain/factproduct` is gone. The final interprocedural
  product owner is `compiler/check/domain/interproc`.
- `abstract.RunTransfer` returns one `TransferResult`: flow inputs plus
  transfer-owned evidence.
- `api.FuncResult` stores `Evidence api.FlowEvidence`, not a separate call side
  channel.
- `api.FuncAnalysisView` replaced the misleading `FuncResultSnapshot` name. It
  is a nested-processing view, not a fact snapshot authority.
- `FunctionFacts` remains the only authority for params, return summary, narrow
  return summary, function type, and refinement.
- `LiteralSigs`, captured types, captured field writes, captured container
  mutations, and constructor fields are product slots under `api.Facts`.
- Production publishers use `domain/interproc` delta constructors. Direct
  `api.Facts{...}` construction outside tests is now limited to product-domain
  internals and store cloning/empty values.

Evidence ownership after this slice:

- call discovery is centralized in `abstract/transfer`.
- `x.field or default` expression evidence is centralized in
  `abstract/transfer`.
- captured field writes and captured container mutations are discovered in
  `abstract/transfer`.
- postflow interproc publication no longer rescans nested bodies for captured
  writes; it reduces transfer evidence with narrowed expression types.
- local return SCC propagation no longer scans local-function call sites; it
  consumes `LocalFuncInfo.Evidence.Calls`.
- parent-call parameter evidence and nested mutation replay no longer scan
  parent call sites; they consume parent `FlowEvidence.Calls`.

Important invariant:

```text
CFG/AST event discovery belongs to abstract transfer.
Later phases may reduce transfer evidence with solved/narrowed types.
Later phases must not rediscover call/body evidence by walking the AST again.
```

This is the flash migration shape, not a compatibility bridge. The old
`FunctionTypesFromFacts`, return-summary mirrors, per-slice fact snapshot
getters, scratch literal signatures, and captured-write nested rescans are not
present in production checker code.

Remaining non-interproc graph walks are classified as follows:

- `abstract/transfer/*`: canonical event discovery and flow-input lowering.
- `hooks/*`: validation passes over already-built analysis results.
- `effects/propagate.go`: effect validation/propagation, not a fact authority.
- `synth/*`: expression synthesis helpers, not interproc fact publication.
- `infer/return` assignment and return walks: local return-vector construction
  and overlay mutation synthesis. These are still part of return inference, not
  a second interproc fact channel. If they start publishing cross-function facts
  directly, they must be moved behind transfer evidence first.
- `nested/constructor.go` and `nested/table.go`: constructor/self-shape pattern
  recognition. Constructor publication already goes through the module
  `ConstructorFields` product slot, but the pattern recognizer itself is still a
  specialized structural recognizer. This is the next design area to collapse if
  constructor/self inference expands.

Verification for this checkpoint:

```text
go test ./compiler/check/returns ./compiler/check/infer/return ./compiler/check/pipeline ./compiler/check/tests/flow ./compiler/check/tests/inference ./compiler/check -count=1

go test ./compiler/check/api ./compiler/check/store ./compiler/check/infer/interproc ./compiler/check/infer/nested ./compiler/check/infer/return ./compiler/check/pipeline ./compiler/check/domain/... ./compiler/check/abstract/... ./compiler/check/synth/phase/extract ./compiler/check/returns ./compiler/check/nested ./compiler/check ./compiler/check/tests/flow ./compiler/check/tests/errors ./compiler/check/tests/modules ./compiler/check/tests/inference -count=1
```

Both passed.

No external replay or global lint classification was performed in this
checkpoint because the active instruction was to finish the migration boundary
before returning to regression classification.

## 2026-05-20 Flash Migration: Transfer Event Trace Collapse

This slice completed the abstract-transfer boundary for the return/nested
checker path. The checker now has one canonical event source for the structural
events that downstream inference needs:

```text
CFG/AST -> abstract/transfer -> flow.Inputs + FlowEvidence
FlowEvidence + solved/narrowed types -> reducers/publishers
FunctionFacts/api.Facts -> only persisted interprocedural product
```

Directly migrated event ownership:

- `FlowEvidence` now carries assignment, return, call, field-default,
  function-definition, function-escape, captured-field, and captured-container
  events discovered by `abstract/transfer`.
- return inference no longer scans assignments to find local functions; it
  consumes `FlowEvidence.FunctionDefinitions`.
- return inference no longer scans returns to build return vectors; it consumes
  `FlowEvidence.Returns`.
- return overlay construction no longer scans assignments for local function
  values, local declaration seeds, local annotations, or captured parent
  annotations; it consumes local or parent `FlowEvidence.Assignments`.
- nested processing no longer calls `nested.GatherChildren` or
  `ResolveNestedFuncIdentity`; those helpers were deleted. Function identity is
  resolved once by transfer evidence.
- constructor/self pattern helpers no longer scan parent/nested graphs for
  assignments or returns; they consume `AssignmentEvidence` and `ReturnEvidence`.
- session graph hierarchy registration no longer has separate assignment,
  funcdef, and nested-function registration loops; it consumes
  `FunctionDefinitionEvidence`.
- module export no longer scans return nodes; it consumes result return
  evidence.

The invariant is now stronger:

```text
Production phases after transfer may inspect solved state, synthesize expression
types, and reduce transfer events. They must not rediscover transfer-owned
events by walking the CFG/AST in return/nested/interproc orchestration.
```

Proof scan for the migrated boundary:

```text
rg "EachAssign|EachCallSite|EachFuncDef|EachReturn|EachBranch|for _, stmt := range" \
  compiler/check/infer/return compiler/check/infer/nested compiler/check/returns \
  compiler/check/pipeline compiler/check/nested compiler/check/session.go \
  compiler/check/modules/export.go -g '!**/*_test.go' -n
```

The scan returns no production matches.

Legacy/fallback scan:

```text
rg "flowbuild|factproduct|FunctionTypesFromFacts|Get.*Snapshot|StoreLiteralSigs|ScratchLiteralSigs|legacy|bridge" \
  compiler/check -g '!**/*_test.go' -n
```

The scan returns no production matches.

Verification for this slice:

```text
go test ./compiler/check/abstract/transfer ./compiler/check/infer/return \
  ./compiler/check/infer/nested ./compiler/check/nested ./compiler/check/returns \
  ./compiler/check/pipeline ./compiler/check ./compiler/check/modules -count=1

git diff --check
```

Both passed.

Broader checker verification currently still fails in regression fixtures:

```text
go test ./compiler/check/... -count=1
```

Known failing fixtures at this checkpoint:

- `TestExternalLint_SessionReaderQueryBuilderRealShape`
- `TestExternalLint_CompressModelInfoNumericHelpersStayNonNil`
- `TestLinterFalsePositive_TestRunnerPattern`
- `TestLinterFalsePositive_TestRunnerExact`
- `TestLinterFalsePositive_GraphLocalUnusedParamAllowsInternalAny`

Those failures are not classified here because the active work was the flash
migration to the abstract-transfer event boundary, not the false-positive
repair pass. They remain the next correctness proof obligations after this
structural migration.

## 2026-05-20 Correction: Event Boundary Was Still Too Loose

The broad scan after the previous checkpoint found real remaining slop. The
earlier statement that the event boundary was clean was too strong.

Real unresolved migration seams:

- module alias extraction still called `modules.CollectAliases(graph)` from
  resolve, runner, driver, transfer declarations, and return inference;
- overlay mutation collection still scanned assignment nodes through
  `overlaymut`/`transfer/assign` wrappers;
- function-type synthesis still walked assignments, returns, and branches for
  local return inference and ordered-comparison hints;
- error-return inverse-pattern proof still walks return nodes directly;
- `synth/phase/extract` still has several local discovery scans for named
  functions and callback environments.

The corrected invariant is stricter:

```text
Graph/AST event discovery has one owner: abstract transfer.
Consumers may request transfer-owned graph evidence, then reduce that evidence
with the type state they own. Consumers must not run their own CFG event scans
for aliases, assignment mutations, returns, branches, calls, or function defs.
```

The immediate flash migration target is therefore not another compatibility
layer. It is one canonical graph evidence object:

```text
cfg.Graph -> transfer.ExtractGraphEvidence -> api.FlowEvidence
api.FlowEvidence + flow/product state -> phase reducers and publishers
```

Direct migration steps for this correction:

- `modules` becomes a reducer over `[]api.AssignmentEvidence`; it no longer
  scans a graph.
- overlay mutation collection becomes a reducer over
  `[]api.AssignmentEvidence`; it no longer scans a graph.
- `abstract.RunTransfer` seeds `FlowContext` with graph evidence before
  lowering so declaration extraction and post-transfer evidence use the same
  trace.
- runner/driver/resolve/return/synth callers use transfer graph evidence as
  their event source instead of reconstructing assignment scans locally.

This still leaves larger design work after the direct event collapse:

- synthesis should become a structural expression evaluator over a product
  query interface, rather than owning its own precedence rules;
- error-return inverse proof should consume return evidence;
- remaining named-function/callback discovery in `synth/phase/extract` should
  be converted to transfer evidence or a stable graph summary;
- store/query APIs still expose some slice-shaped projections of the canonical
  product and need a separate product-query cleanup.

## 2026-05-20 Correction: Canonical Trace Cutover

The corrective flash migration now has a concrete owner for event discovery:

```text
compiler/check/abstract/trace
```

`trace` is the only non-transfer package that walks CFG/AST structure to build
semantic evidence. `abstract/transfer` consumes that trace and may still walk
the CFG internally while lowering flow inputs. Other phases reduce trace
evidence; they do not rediscover the same events.

Moved to the canonical trace/reducer shape:

- module alias inference reduces `[]api.AssignmentEvidence`;
- overlay field/index mutation inference reduces `[]api.AssignmentEvidence`;
- table mutator overlay inference reduces `[]api.CallEvidence`;
- constructor/self field inference reduces `[]api.AssignmentEvidence`;
- literal function types/signatures consume trace assignments, function
  definitions, calls, and returns;
- synth local-function rebinding/captured-mutation checks consume trace
  assignments and function definitions;
- error-return inverse proof consumes `[]api.ReturnEvidence`;
- parameter-use projection now gets `[]api.ParameterUseEvidence` from
  `abstract/trace` instead of scanning bodies inside `domain/paramevidence`;
- iterator pair provenance consumes assignment evidence instead of scanning
  inside `domain/iteration`;
- phase/synth dependencies carry `api.FlowEvidence`, so temporary synthesizers
  reuse the same trace when they are analyzing the same graph.

Correct boundary after this slice:

```text
abstract/trace    = graph/body event discovery and semantic trace records
abstract/transfer = flow-input lowering plus solved-state-dependent evidence
domain/*          = lattice/reducer laws over already-lowered evidence
synth             = expression evaluator over product/query state and trace
hooks             = diagnostics over product state and trace
```

The broad production scan now classifies as:

- `abstract/trace/*`: canonical event discovery;
- `abstract/transfer/*`: canonical transfer lowering;
- `hooks/lspindex.go`: editor index, not type/effect fact authority;
- `hooks/control_check.go`: syntax/control validation;
- `hooks/exhaustiveness_check.go`: syntactic `if`/`elseif` shape walk for
  discriminated-union exhaustiveness after branch evidence indexing.

The scan no longer shows event rediscovery in `modules`, `overlaymut`,
`domain/paramevidence`, `domain/iteration`, `infer/*`, `returns`, `nested`,
`pipeline`, or `synth`.

Verification for this correction:

```text
go test ./compiler/check/... -run '^$'

go test ./compiler/check/abstract/trace ./compiler/check/abstract/transfer \
  ./compiler/check/domain/paramevidence ./compiler/check/domain/iteration \
  ./compiler/check/synth ./compiler/check/synth/phase/extract \
  ./compiler/check/hooks ./compiler/check/infer/return \
  ./compiler/check/pipeline -count=1
```

Both passed.

This is a design correction, not a file shuffle. The reason for the move is to
make semantic event discovery single-owner and cacheable. Later work can now
optimize around one trace instead of chasing assignments/calls/returns through
several helper clusters.

## 2026-05-20 Abstract Interpreter Evidence Closure

Follow-up flash migration tightened the boundary again. Several consumers still
looked harmless because they were diagnostics or synthesis helpers, but they
were still reconstructing semantic facts from CFG nodes. Those are now explicit
evidence lanes in the abstract interpreter trace:

- `api.IdentifierUseEvidence`: identifier reads are discovered once by
  `abstract/trace`; `hooks.CheckIdents` consumes that evidence instead of
  walking `graph.RPO()` and reopening node payloads.
- `api.FreshTableLiteralEvidence`: fresh table provenance for structured
  assignment checks is proven in `abstract/trace` at only the assignment sites
  that need it. `domain/provenance` now only matches source identifiers to this
  canonical proof; it no longer walks predecessors or reinterprets CFG events.
- `api.NormalExitEvidence`: termination reachability now checks return evidence
  plus the canonical normal-exit point. `effects.TerminatesFromReachability`
  no longer scans CFG nodes looking for returns.
- call-on-return extraction now receives the current `api.FlowEvidence`
  explicitly; condition extraction no longer builds an ad-hoc trace when it
  needs local predicate evidence.
- literal synthesis and keys/callback helpers no longer silently rebuild
  evidence when callers pass an empty trace. Production callers must pass the
  canonical trace; tests now build trace evidence explicitly.

This confirms the intended abstract-interpreter shape:

```text
CFG/AST -> abstract/trace.FlowEvidence
FlowEvidence + declared state -> abstract/transfer.Inputs
Inputs -> flow.Solution in DNF/path-sensitive form
FlowEvidence + flow/product state -> synth, effects, hooks, interproc facts
FunctionFacts/interproc product -> monotone SCC/fixpoint publication
```

DNF is already the checker's path-condition representation. It is not being
replaced here. The optional future SMT tier would sit after this architecture as
an obligation solver for formulas outside the current domain-specific DNF and
numeric/refinement reducers. The flash migration goal is to make that extension
possible without adding another scattered helper layer.

Current allowed direct CFG users:

- `abstract/trace`: canonical event/provenance discovery;
- `abstract/transfer`: graph topology, SSA versions, loop preheaders, edge
  conditions, and transfer lowering;
- `phase/scope` and `scope/typedefs`: lexical/type-scope construction before
  transfer facts exist;
- `phase/resolve`: initial symbol/type seeding for declared environments;
- `hooks/lspindex`: editor symbol/reference index, not checker fact authority.

Everything else should either consume `api.FlowEvidence`, consume solved flow
state, or move its missing fact into `abstract/trace` as a named evidence lane.

## 2026-05-20 Evidence Projection And Iterator Transfer Corrections

This checkpoint records the latest engine-level corrections after rechecking the
remaining focused false positives.

Canonical design clarifications:

- DNF is already the current path-condition language. It remains the core
  representation for path-sensitive flow and refinement constraints. A future
  SMT/refinement tier would be an optional solver behind this same evidence
  model, not a replacement for DNF and not a new scattered fact layer.
- Parameter evidence has two separate body-demand modes:
  - whole-parameter use preserves the full observed call-site shape;
  - direct field use completes demanded absent fields on that preserved shape.
  Whole forwarding therefore must not erase local field demands such as
  `options.stream`; it only prevents trimming unrelated observed fields.
- Generic-for transfer has two evidence sources:
  - the solved iterator source type;
  - the abstract interpreter's extracted assignment type for the loop target.
  If iterator derivation is top-like (`any`/`unknown`) and the interpreter has a
  concrete local refinement from sound evidence, the concrete interpreter
  evidence is authoritative. Concrete iterator derivation still remains
  authoritative when it proves a real element type.
- Explicit dynamic `any` is not silently specialized by a callee's expected
  argument type. Unknown or soft unresolved locals may be refined by use, but
  `any` remains a dynamic boundary unless a typed source or guard proves the
  concrete type.

Corrections made:

- `ProjectToParameterUse` now completes demanded fields even when the parameter
  is also forwarded as a whole value. This fixed the callback-local
  `client.request(..., {headers = {}})` regression where `http_options.stream`
  was falsely reported missing.
- Generic loop targets are now recorded as real value definitions for inference
  visibility. Loop variables are not treated as invisible just because their
  value comes from `IterExprs` instead of a normal RHS expression.
- Assignment extraction now lets the SCC-refined loop target type repair
  top-like iterator output before emitting flow inputs.
- Flow transfer now reconciles iterator-derived type with the extracted loop
  assignment type so `ipairs(any)` cannot erase a proven local call-expected
  refinement. The sorted-test-runner fixtures now model `io.args()` with a
  manifest returning `string[]`, which is the actual proof that `pattern` is
  `string`.
- The intentionally unsafe dynamic-resource fixture still requires a string
  proof for `resource_id`; this protects the soundness boundary that arbitrary
  `any` values from untyped input cannot be accepted as strings merely because a
  downstream function expects `string`.

Regression protection added or confirmed:

- `TestProjectToParameterUse_WholeForwardingCompletesDemandedFields` covers
  whole forwarding plus direct demanded absent field completion.
- `TestMergeIteratorAssignedType_PreservesPreciseExtractedAgainstDynamicDerived`
  covers iterator transfer reconciliation.
- Existing high-level regressions now pass:
  - `TestFalsePositive_CallbackLocalDelegatedErrorReturnNarrowsSibling`
  - `TestWippyRunner_SortedKeysWithFilterBranch`
  - `TestWippyRunner_NearLiteralTestRunnerFlow`

Verification for this correction:

```text
go test ./compiler/check/domain/paramevidence ./types/flow \
  ./compiler/check/tests/regression \
  -run 'TestProjectToParameterUse_WholeForwardingCompletesDemandedFields|TestMergeIteratorAssignedType_PreservesPreciseExtractedAgainstDynamicDerived|TestFalsePositive_CallbackLocalDelegatedErrorReturnNarrowsSibling|TestWippyRunner_SortedKeysWithFilterBranch|TestWippyRunner_NearLiteralTestRunnerFlow' \
  -count=1

go test ./compiler/check/tests/regression \
  -run 'TestFalsePositive_CallbackLocalDelegatedErrorReturnNarrowsSibling|TestWippyRunner_SortedKeysWithFilterBranch|TestWippyRunner_NearLiteralTestRunnerFlow' \
  -count=1

go test ./types/flow ./compiler/check/abstract/transfer/assign \
  ./compiler/check/domain/paramevidence -count=1
```

All commands passed.

## 2026-05-20 Correction: Field Probes Are Nil-Producing Queries

The `deadlock-compiler-lua` fixture exposed a remaining mismatch between the
abstract interpreter and Lua table semantics. A field read used only as a
truthiness/existence probe was still validated as a required value read:

```lua
if edge.target_node_id or edge.is_workflow_terminal then
```

When `edge` is an inferred closed record that lacks `is_workflow_terminal`,
the runtime result of `edge.is_workflow_terminal` is nil. That probe is safe;
what remains unsafe is using the absent field as a real value. The canonical
rule is now:

```text
value read              = declared field required on closed records
truthiness/nil probe    = missing table field may read as nil
primitive/non-table read = still an indexing error
```

The implementation keeps this distinction at the checker boundary instead of
weakening field lookup globally. `types/query/core.MissingFieldReadsNil` is the
single query for "does absent field access produce nil rather than an indexing
error"; union field lookup already needed the same fact to add nil for table
variants that do not carry a field.

Regression protection:

- `TestGuards_FieldTruthyNarrowsUnion/closed_record_missing_field_is_nil_in_truthiness_probe`
  covers `or` and `not` guards on absent table fields.
- `TestGuards_FieldTruthyNarrowsUnion/closed_record_missing_field_nil_comparison_is_existence_probe`
  covers `field == nil` existence probes.
- `TestStrictTypeChecks_FieldAndReturn/truthiness_probe_on_primitive_still_rejects_indexing`
  keeps primitive indexing errors intact.
- `TestStrictTypeChecks_FieldAndReturn/missing_closed_record_field_still_rejects_value_read`
  keeps strict value-read diagnostics intact.

Fixture classification:

- `regression/deadlock-compiler-lua/check` is now clean; the
  `is_workflow_terminal` diagnostic was a checker false positive.
- `regression/non-dominating-field-defined-wrapper-return/check` still reports
  the intended soundness error (`cannot assign unknown to string`). Its
  manifest was corrected from two expected diagnostics to one because the extra
  diagnostic was only duplicate/noisy reporting, not an independent proof
  obligation.

## 2026-05-20 Correction: Parameter Evidence Is Pre-State Only

The recursive schema regression exposed a foundational ownership leak in return
inference. `FunctionFact.Params` is the accepted input contract for a function,
but return inference was merging the post-body overlay back into parameter
evidence after applying body mutations. That let writes such as:

```lua
obj.multipleOf = nil
obj.additionalProperties = nil
```

become required fields on the public parameter type of `recursive_filter(obj)`.
The result was a false positive at the first valid call with a schema shape that
did not already contain fields created inside the callee body.

Correct invariant:

```text
FunctionFact.Params = accepted pre-state input evidence
body writes         = local abstract-interpreter state and return/output shape
FunctionFact.Type   = callable view over Params + return summaries/effects
```

Therefore:

- call-site observations and body-read/precondition evidence may refine
  `FunctionFact.Params`;
- field defaults such as `param.field or "x"` may add optional input evidence;
- helper-call expected arguments may add body-demand evidence when the parameter
  or a parameter field is read and passed onward;
- final overlay state after assignments must not be merged into
  `FunctionFact.Params`;
- body mutation state may still affect the return summary and local flow state.

The implementation removed the post-mutation overlay-to-parameter-evidence
merge from return inference. This is not a bridge or a special case: it restores
the abstract-interpreter product separation. The interpreter owns mutable value
state; the function-fact parameter slot owns the accepted pre-state contract.

Regression protection:

- `TestExternalLint_PairsSchemaFilterWritesRecursiveValueBackToSameKey`
  protects the positive recursive-schema case where body writes create fields
  before returning the object.
- `TestExternalLint_RejectsPairsWriteThatChangesClosedFieldDomain` protects the
  negative closed-record case so table write soundness is not weakened.
- `TestExternalLint_DynamicResourceIDsRequireStringProof` protects the dynamic
  `any` boundary: expected string parameters cannot silently specialize unknown
  external data.

Verification:

```text
go test ./compiler/check/tests/regression \
  -run 'TestExternalLint_PairsSchemaFilterWritesRecursiveValueBackToSameKey|TestExternalLint_RejectsPairsWriteThatChangesClosedFieldDomain|TestExternalLint_DynamicResourceIDsRequireStringProof|TestWippyRunner_SortedKeysWithFilterBranch|TestWippyRunner_NearLiteralTestRunnerFlow|TestFalsePositive_CallbackLocalDelegatedErrorReturnNarrowsSibling|TestExternalLint_BodyCallExpectationInfersWholeParameter|TestExternalLint_GuardedBodyUseDoesNotEraseOptionalParamBoundary' \
  -count=1

go test ./compiler/check/infer/return ./compiler/check/domain/paramevidence \
  ./compiler/check/abstract/transfer/assign ./types/flow -count=1

go test ./compiler/check/... -count=1
```

All commands passed.

## 2026-05-20 Correction: Table-Like Probes Include Arrays And Tuples

The local-replace framework replay exposed two `wippy.agent:context` diagnostics
on agent-style untyped probe code:

```lua
if type(tool_specs) == "string" or
   (type(tool_specs) == "table" and tool_specs.id) then
```

The previous correction made record/map/interface field probes nil-producing,
but left arrays and tuples out of the central query. That was inconsistent with
the existing type-kind model: `type(x) == "table"` maps to records, maps,
arrays, tuples, interfaces, and intersections. All of those are Lua table-like
values, so a missing named field used only as a truthiness/existence probe reads
nil rather than raising an indexing error.

Canonical rule:

```text
table-like value read of missing field   = strict diagnostic for value use
table-like field existence/truth probe   = nil-producing, no diagnostic
primitive/function/nil field probe       = still an indexing diagnostic
```

This is not a tuple-packaging workaround. It aligns
`types/query/core.MissingFieldReadsNil` with the same table-kind lattice used by
DNF/path-sensitive `type(x) == "table"` narrowing. Value reads remain strict, so
the checker still reports likely typos on precise table shapes.

Regression protection:

- `TestMissingFieldReadsNil_TableLikeContainers` covers record, map, array,
  tuple, interface, and primitive boundaries in the canonical query.
- `TestFieldProbeSemantics_UntypedTableGuardAllowsExistenceProbe` covers the
  agent-style untyped rewrap/probe pattern that previously produced
  `field 'id' does not exist on type ((any))`.
- Existing strict-field tests still protect primitive probes and direct missing
  field value reads.

Local-replace classification after the fix:

- `framework/src/agent/src` dropped from eight to six diagnostics; the two
  `context.lua` field-probe diagnostics were checker false positives and are
  fixed.
- Remaining inspected diagnostics are source/proof obligations, not abstract
  interpreter regressions:
  - untyped `any` passed to string APIs (`tool_id`, Bedrock parsed text,
    test network URLs/error fields, page resource ids);
  - optional HTTP bodies passed to `json.decode` without a fallback at that
    source version;
  - untyped mutable config writes that can invalidate numeric config fields;
  - manifest/source mismatches such as stream `read` arity and metadata modeled
    as string while code treats it as a table.

Verification for this correction:

```text
go test ./types/query/core \
  -run TestMissingFieldReadsNil_TableLikeContainers -count=1 -v

go test ./compiler/check/tests/regression \
  -run 'TestFieldProbeSemantics_UntypedTableGuardAllowsExistenceProbe|TestFieldProbeSemantics_InterfaceMissingFieldIsNilOnlyInProbe' \
  -count=1 -v

go test ./compiler/check/hooks ./compiler/check/tests/flow \
  ./compiler/check/tests/errors ./types/query/core -count=1
```

All commands passed.

## 2026-05-20 Flash Migration: Parameter Demand Is Flow Evidence

The next remaining non-canonical seam was parameter-use demand. Several
consumers were independently asking the old tracer to walk a function body and
rediscover which parameter surface was actually demanded. That made parameter
evidence a side channel instead of part of the abstract-interpreter product.

Canonical rule:

```text
CFG/AST event discovery = trace.GraphEvidence
consumer phases         = reducers over api.FlowEvidence
parameter demand        = FlowEvidence.ParameterUses
```

After this correction, `api.FlowEvidence` carries `ParameterUses` alongside
calls, returns, assignments, branches, field defaults, function definitions,
escapes, and captures. `trace.GraphEvidence` is the only production builder for
that lane. Scope construction, synthesized signature projection, return
inference, and call checking now consume the evidence lane directly.

The important design point is that this is not a fallback or compatibility
bridge. Production code does not recompute parameter use on demand. If a phase
needs to know whether call-site or fact evidence should narrow a whole
parameter, a field-only surface, or an unobserved parameter, it reduces the
already-built evidence product for that function.

Call checking also stopped rewalking nested callees. The pass now receives the
session result map and reads the callee result's `FlowEvidence.ParameterUses`.
That keeps nested local-call validation on the same product-domain data as the
rest of the checker instead of opening the callee AST again during diagnostics.

Proof invariant:

```text
rg "ParameterUses\\(" compiler/check -g '!**/*_test.go'
```

The only production matches are:

```text
compiler/check/abstract/trace/paramuse.go
compiler/check/abstract/trace/trace.go
```

Regression protection:

- `TestGraphEvidenceIncludesParameterUses` proves `GraphEvidence` carries
  whole-parameter and field-only demand in the canonical evidence product.
- Existing parameter-evidence projection tests still exercise the reducer logic
  directly, but production consumers no longer invoke that discovery path.

Verification:

```text
go test ./compiler/check/abstract/trace ./compiler/check/pipeline \
  ./compiler/check/phase ./compiler/check/hooks ./compiler/check/infer/return \
  ./compiler/check/tests/narrowing -count=1
```

The command passed.

## 2026-05-20 Flash Migration: Graph Provider Carries Evidence

After the value-domain consolidation, the remaining graph-evidence ownership
leak was in nested helper consumers. The synthesizer and keys-collector detector
could still rebuild `trace.GraphEvidence` when their graph provider did not
also provide canonical evidence. That was a fallback path in consumer code.

Canonical rule:

```text
api.GraphProvider = canonical CFG provider + canonical FlowEvidence provider
consumer helpers   = ask the provider or use the current function's evidence
trace.GraphEvidence = constructor used only by canonical materializers/tests
```

Implementation shape:

- `api.GraphProvider` now includes `EvidenceForGraph`.
- the old separate `GraphEvidenceProvider` capability name is gone; store
  readers declare the evidence method directly instead of embedding a second
  provider abstraction.
- `check.Session` already satisfied that shape through its store-backed
  evidence cache.
- Synth extraction no longer imports `abstract/trace` in production and returns
  no hidden rebuilt evidence when no provider is attached.
- Keys-collector classification no longer has a consumer-side graph-evidence
  rebuild fallback. Tests now provide an explicit graph/evidence provider when
  they exercise nested callee classification.

Proof invariant:

```text
rg "GraphEvidenceProvider" compiler/check -n

rg "trace\\.GraphEvidence" compiler/check -g '!**/*_test.go' -n
```

The first scan returns no matches. The second scan's remaining production
matches are canonical constructor/materializer sites:

```text
compiler/check/store/store.go
compiler/check/abstract/transfer/evidence.go
```

Verification:

```text
go test ./compiler/check/abstract/transfer/keyscoll \
  ./compiler/check/synth/phase/extract ./compiler/check/api \
  ./compiler/check/pipeline -count=1

go test ./compiler/check/... ./types/flow/... -count=1

go test ./... -count=1

git diff --check
```

All commands passed.

## 2026-05-20 Flash Migration: Value Domain Owns Structural Shape Laws

The next non-canonical helper cluster was in parameter evidence. Parameter
evidence was not only merging parameter vectors; it also carried local
implementations for:

- table-top collapse and table-top upper-bound selection;
- map/record structural reconstruction;
- soft structural annotation refinement from evidence;
- table-key truthiness refinement for parameter facts.

That was the wrong ownership boundary. Those rules are value-shape laws, not
parameter-vector laws. Keeping them in `paramevidence` created a second partial
shape lattice and made it too easy for later work to patch parameter facts
instead of improving the abstract value domain.

Canonical rule:

```text
domain/value           = structural type-shape laws
domain/paramevidence   = parameter evidence vector normalization and slot join
domain/functionfact    = function fact product law
```

Implementation shape:

- `domain/value/table.go` owns table-top collapse, table-top upper-bound
  selection, map/record reconstruction, and table-key truthiness refinement.
- `domain/value/annotation.go` owns structural annotation refinement from
  evidence. It accepts the caller's slot join function, so the value domain
  owns traversal/reconstruction while parameter evidence keeps its merge law.
- `domain/paramevidence` now calls these value-domain operations and no longer
  defines local table-top, map/record, annotation-shape, or table-key
  truthiness helpers.

This is not a bridge. The old helper bodies were deleted from parameter
evidence. There is still one parameter-specific public decision,
`RefinesFunctionParam`, but it is now a small composition of value-domain
relations:

```text
optional elision | truthy refinement | table-key truthiness refinement
```

Proof invariant:

```text
rg "refineAnnotationShape|arrayElementEvidence|mapEvidence|keyEvidenceCompatible|selectTableUpperBound|tableTopCoversEvidenceMember|collapseTableTopEvidence|joinMapRecordDirected|refinesTableKeyByTruthiness" \
  compiler/check/domain/paramevidence -n
```

The scan returns no matches.

Regression protection added in `domain/value`:

- table-top evidence absorbs precise table members;
- table-top upper bound absorbs a record-union observation;
- map/record shape reconstruction canonicalizes a pure map-component record
  back into a map;
- structural annotation refinement derives a map value from record evidence;
- table-key truthiness refinement covers maps, records with map components,
  nilable unions, and a negative value-change case.

Verification:

```text
go test ./compiler/check/domain/value ./compiler/check/domain/paramevidence \
  ./compiler/check/domain/functionfact ./compiler/check/domain/interproc -count=1

go test ./compiler/check/... ./types/flow/... -count=1

go test ./... -count=1

git diff --check
```

All commands passed.

## 2026-05-20 Flash Migration: Graph Evidence Is Module-Cached

After reducer-local evidence reconstruction was removed, the remaining
non-canonical shape was repeated graph-evidence construction by orchestration
and helper layers. The pipeline runner, driver, session hierarchy registration,
return inference, and nested helper consumers each knew how to call
`trace.GraphEvidence` directly.

Canonical rule:

```text
trace.GraphEvidence          = low-level constructor
store.EvidenceForGraph       = module-wide canonical evidence product/cache
pipeline/session/inference   = consumers of the evidence provider
```

The implementation added `api.GraphEvidenceProvider` and made
`store.SessionStore` the module-wide evidence cache. `Session` exposes the same
provider method because it already owns graph construction. Pipeline setup,
function analysis, session hierarchy registration, and return inference now ask
the provider instead of rebuilding graph evidence independently.

This is a structural simplification, not a compatibility bridge:

- graph evidence is computed at most once per registered graph ID in the module
  store;
- callers no longer choose bindings or re-run event discovery themselves;
- the graph provider can also provide evidence to nested analyses such as
  keys-collector detection and synthesis helper paths.

Remaining direct constructor calls are intentionally narrow:

- `store.SessionStore.EvidenceForGraph` is the canonical module cache fill;
- `transfer.MaterializeGraphEvidence` is the standalone transfer entry fill
  when a `FlowContext` is used without a store-backed provider;
- isolated utility/test paths may still construct evidence when no provider is
  available.

Regression protection:

- `TestSessionStore_EvidenceForGraph` proves the store materializes
  parameter-use evidence and returns the cached product on later reads.

Verification:

```text
go test ./compiler/check/store ./compiler/check/api ./compiler/check/pipeline \
  ./compiler/check/infer/return ./compiler/check/abstract/transfer/keyscoll \
  ./compiler/check/synth/phase/extract ./compiler/check -count=1
```

The command passed.

## 2026-05-20 Flash Migration: Keys-Collector Detection Owns Callee Body Classification

Assignment extraction still contained a duplicate keys-collector recovery path:
after calling the dedicated `keyscoll.BuildKeysCollectorDetector`, it performed
another module-binding function-literal lookup, built a nested CFG, rebuilt
graph evidence, and called `keyscoll.DetectKeysCollector` itself. That was a
local reimplementation of the detector's responsibility.

Canonical rule:

```text
assign.ExtractAssignments    = emits assignment products
keyscoll.BuildKeysCollectorDetector = resolves candidate callees and classifies keys collectors
```

The detector now resolves function literals through both graph-local bindings
and module bindings before classifying the callee body. Assignment extraction no
longer imports `keyscoll` for nested body classification and no longer builds
nested graph evidence for that case. It only consumes the detector callback and
the refinement product.

This removes one more reducer-local path that knew how to open a callee body.
The body classifier is centralized in `keyscoll`, where its cache and return
index logic already live.

Verification:

```text
go test ./compiler/check/abstract/transfer/assign \
  ./compiler/check/abstract/transfer/keyscoll \
  ./compiler/check/abstract/transfer/... -count=1
```

The command passed.

## 2026-05-20 Flash Migration: Evidence Materialization Is Single-Entry

The next evidence seam was not a type rule; it was ownership. Several transfer
reducers had their own local branch of the form "if this evidence slice is
empty, rebuild `trace.GraphEvidence` here". That made missing evidence look
valid and scattered the abstract-interpreter entry protocol across reducers.

Canonical rule:

```text
trace.GraphEvidence             = event discovery constructor
transfer.MaterializeGraphEvidence = transfer entry materializer
reducers                         = pure reducers over FlowContext.Evidence
api.FlowEvidence.IsZero          = only zero-product predicate
```

The implementation added `api.FlowEvidence.IsZero` and removed duplicate
`flowEvidenceEmpty` helpers. Transfer reducers for declarations, assignments,
table mutators, container mutators, and captured-container evidence no longer
rebuild graph evidence locally. `transfer.Run` materializes graph evidence once
before reducer execution. Standalone reducer tests now pass explicit
`trace.GraphEvidence` through `core.FlowContext`, matching the production
contract instead of relying on hidden reducer behavior.

The session hierarchy registration also stopped calling the partial
`trace.FunctionDefinitions` discovery function directly. It now obtains
function definitions from the canonical `GraphEvidence` product for that graph.

Proof invariant:

```text
rg "flowEvidenceEmpty|fc\\.Evidence = trace\\.GraphEvidence|trace\\.FunctionDefinitions|EnsureGraphEvidence" \
  compiler/check -g '!**/*_test.go'
```

The only remaining production graph-evidence assignment is the central transfer
materializer:

```text
compiler/check/abstract/transfer/evidence.go
```

Regression protection:

- `TestFlowEvidenceIsZero` covers the shared zero-product predicate, including
  parameter-demand evidence.
- Existing reducer tests now construct canonical evidence explicitly, proving
  reducers no longer depend on local rediscovery.

Verification:

```text
go test ./compiler/check/api ./compiler/check/abstract \
  ./compiler/check/abstract/transfer/... ./compiler/check/synth/phase/extract \
  ./compiler/check -count=1
```

The command passed.

## 2026-05-20 Flash Migration: Function Fact Projection Is Domain-Owned

The `compiler/check/abstract/facts` package was a misleading boundary. It did
not implement abstract transfer semantics; it projected stable function facts
from the interprocedural product stored in the checker store. Keeping that
projection under `abstract` made the mental model look like transfer owned a
second fact channel.

Canonical rule:

```text
domain/functionfact   = meaning, merge, widening, and store projection for one function fact
abstract/transfer     = graph-local abstract interpretation over explicit flow evidence
store                 = cached module products and graph evidence provider
```

The projection helpers now live beside the rest of the function-fact domain:

- `functionfact.ForSymbol` reads the canonical stable function fact for a
  symbol;
- `functionfact.TypeForSymbol` projects its callable type;
- `functionfact.RefinementsFromStore` exposes refinement facts as a store view.

This is not a bridge or compatibility layer. The old `abstract/facts` package
was deleted, and callers now name the domain that owns the data they consume.

Proof invariant:

```text
rg "abstract/facts|abstractfacts|FunctionFactForSymbol|FunctionTypeForSymbol|RefinementsFromFunctionFacts|package facts" \
  compiler/check -n
```

The scan returns no matches.

Verification:

```text
go test ./compiler/check/domain/functionfact ./compiler/check/hooks \
  ./compiler/check/pipeline ./compiler/check/synth/phase/extract -count=1
```

The command passed.

## 2026-05-20 Flash Migration: Product Type Facts Are Domain-Owned

The second package-boundary inversion was `compiler/check/api` importing
`compiler/check/abstract/query`. The implementation in that package was not an
abstract-transfer reducer; it was the canonical `flow.TypeFacts` view over the
checker product state: declared types, canonical function facts, literal types,
annotation markers, and the optional solved flow state.

Canonical rule:

```text
api/env              = phase-typed environment contract and constructors
domain/typefacts     = product-state query implementing flow.TypeFacts
abstract/trace       = event discovery
abstract/transfer    = graph-local abstract transfer
```

The product query now lives in `compiler/check/domain/typefacts` as
`typefacts.New(typefacts.Config{...})`. The old `abstract/query` package was
deleted, so the API layer no longer depends on an abstract implementation
package.

This keeps the abstract-interpreter model direct:

- product domains own product queries;
- transfer owns event reduction;
- API environment construction no longer reaches into abstract packages.

Proof invariant:

```text
rg "compiler/check/abstract/(facts|query)|abstractfacts|FunctionFactForSymbol|FunctionTypeForSymbol|RefinementsFromFunctionFacts|package facts|package query|NewTypeFacts|TypeFactsConfig" \
  compiler/check -n
```

The scan returns no matches.

Verification:

```text
go test ./compiler/check/domain/typefacts ./compiler/check/api -count=1
```

The command passed.

## 2026-05-20 Flash Migration: Constraint Path Extraction Is Domain-Owned

`abstract/transfer/path` was another sideways dependency. It did not lower flow
inputs by itself; it converted AST expressions plus binding identity into
canonical `constraint.Path` values. That path identity is consumed by trace,
transfer reducers, hooks, return inference, and iteration helpers. Keeping it
under `abstract/transfer` made non-transfer packages import transfer internals
for a shared semantic operation.

Canonical rule:

```text
domain/path          = AST/binding -> constraint.Path identity
abstract/trace       = event discovery using domain paths where needed
abstract/transfer    = reducers that consume paths and produce flow inputs
hooks/infer          = validators/reducers that may also consume canonical paths
```

The package moved to `compiler/check/domain/path`. No behavior changed; all
callers now import the domain owner directly, and `abstract/transfer/path` was
deleted.

Proof invariant:

```text
rg "compiler/check/abstract/transfer/path" compiler/check -n
```

The scan returns no matches.

Verification:

```text
go test ./compiler/check/domain/path ./compiler/check/abstract/transfer/... \
  ./compiler/check/abstract/trace ./compiler/check/hooks \
  ./compiler/check/infer/return ./compiler/check/domain/iteration -count=1
```

The command passed.

## 2026-05-20 Flash Migration: Guard Extraction Is Domain-Owned

`abstract/transfer/guard` was shared by transfer reducers, field validation, and
expression synthesis. The package owns guard semantics: builtin `type(expr)`
probes, truthy path keys, and propagation of branch guard facts. That is a
domain law over AST/binding identity and branch evidence, not an implementation
detail of assignment transfer.

Canonical rule:

```text
domain/guard         = guard/probe extraction and guard-key semantics
abstract/transfer    = reducers that consume guard facts while building flow inputs
hooks/synth          = consumers of the same guard domain for validation and synthesis
```

The package moved to `compiler/check/domain/guard`, and
`abstract/transfer/guard` was deleted. This removes another sideways transfer
import without changing guard behavior.

Proof invariant:

```text
rg "compiler/check/abstract/transfer/guard" compiler/check -n
```

The scan returns no matches.

Verification:

```text
go test ./compiler/check/domain/guard ./compiler/check/abstract/transfer/assign \
  ./compiler/check/hooks ./compiler/check/synth/phase/extract -count=1
```

The command passed.

## 2026-05-20 Flash Migration: Symbol Resolution Is Domain-Owned

`abstract/transfer/resolve` had become a shared resolution utility package:
symbol display names, product-state type selection, input/global lookups,
context symbol resolvers, type-key lookup, and call-refinement lookup. Transfer
reducers need those operations, but they are not themselves transfer reducers.
Return inference, return callsite analysis, and pipeline captured-mutator replay
also consumed them directly.

Canonical rule:

```text
domain/resolve       = checker symbol/type/refinement resolution helpers
domain/path          = path identity construction
abstract/transfer    = reducers that call domain resolution while lowering evidence
infer/pipeline       = consumers of the same resolution domain
```

The package moved to `compiler/check/domain/resolve`, and
`abstract/transfer/resolve` was deleted. This removes another non-reducer
package from the transfer tree.

Proof invariant:

```text
rg "compiler/check/abstract/transfer/resolve" compiler/check -n
```

The scan returns no matches.

Verification:

```text
go test ./compiler/check/domain/resolve ./compiler/check/abstract/transfer/... \
  ./compiler/check/returns ./compiler/check/pipeline \
  ./compiler/check/infer/return -count=1
```

The command passed.

## 2026-05-20 Flash Migration: Overlay Mutation Collectors Have One Owner

`abstract/transfer/assign/collect.go` was a pure forwarding bridge to
`overlaymut.CollectFieldAssignments` and `overlaymut.CollectIndexerAssignments`.
That was exactly the kind of non-final shape the migration is removing: callers
could name assignment transfer while actually using overlay mutation collection.

Canonical rule:

```text
overlaymut           = collect and apply overlay field/indexer mutation evidence
abstract/transfer/assign = assignment transfer reducers, not overlay collector aliases
infer/nested/returns = consume overlaymut directly when they need overlay mutation facts
```

The forwarding file was deleted. Return inference, nested inference, and
constructor detection now call `overlaymut` directly. The former wrapper tests
moved to the owning package.

Proof invariant:

```text
rg "assign\\.Collect(Field|Indexer)Assignments" compiler/check -n
```

The scan returns no matches.

Verification:

```text
go test ./compiler/check/overlaymut ./compiler/check/abstract/transfer/assign \
  ./compiler/check/nested ./compiler/check/infer/nested \
  ./compiler/check/infer/return -count=1
```

The command passed.

## 2026-05-20 Flash Migration: Return Overlay Facade Removed

`compiler/check/returns/overlay.go` was another facade over `overlaymut`. It
re-exported field/indexer/direct mutation merge operations while adding no
return-specific semantics. That made return inference look like it had its own
overlay mutation domain.

Canonical rule:

```text
overlaymut           = overlay mutation collection and application
returns              = return graph/call/summary inference only
infer/return         = consumes returns for return SCCs and overlaymut for overlay mutation
```

The facade was deleted. Return inference and nested inference now call
`overlaymut` directly, and the former return-overlay tests moved to the
overlay mutation package.

Proof invariant:

```text
rg "returns\\.(MergeFieldAssignments|ApplyFieldMergeToOverlay|MergeFieldsIntoType|ApplyIndexerMergeToOverlay|JoinValueTypes|MergeMapComponentIntoType|ApplyDirectMutationsToOverlay)" \
  compiler/check -n
```

The scan returns no matches.

Verification:

```text
go test ./compiler/check/overlaymut ./compiler/check/returns \
  ./compiler/check/infer/nested ./compiler/check/infer/return -count=1
```

The command passed.

## 2026-05-20 Flash Migration: Keys-Collector Detection Is Domain-Owned

`abstract/transfer/keyscoll` detected the "collect keys from table parameter"
function pattern over graph evidence. Assignment transfer consumes that
detector, but the detector itself is a semantic domain over calls, assignments,
returns, function identity, and graph evidence. Phase-level signature projection
also needs it.

Canonical rule:

```text
domain/keyscoll      = keys-collector body/call classification
abstract/transfer    = assignment reducer consuming the detector result
phase                = signature projection consuming the same detector domain
```

The package moved to `compiler/check/domain/keyscoll`, and
`abstract/transfer/keyscoll` was deleted.

Proof invariant:

```text
rg "compiler/check/abstract/transfer/keyscoll" compiler/check -n
```

The scan returns no matches.

Verification:

```text
go test ./compiler/check/domain/keyscoll ./compiler/check/abstract/transfer \
  ./compiler/check/abstract/transfer/assign ./compiler/check/phase -count=1
```

The command passed.

## 2026-05-20 Flash Migration: Indexer Overlay Facts Are Overlay-Owned

The dynamic-index overlay shape `IndexerInfo` and the merge operation for
table-insert-derived indexer mutations still lived in transfer/mutator. That
forced `overlaymut` to import transfer just to name the data it owned.

Canonical rule:

```text
overlaymut           = overlay mutation data shapes and overlay merge/apply laws
abstract/transfer/mutator = call-pattern detection for table/container mutators
infer/synth          = combine transfer mutator observations with overlaymut facts
```

`overlaymut.IndexerInfo` and `overlaymut.MergeIndexerMutations` are now the
canonical APIs. Transfer mutator detection returns that overlay-owned shape;
the old `mutator.IndexerInfo` and `mutator.MergeIndexerMutations` names were
deleted.

Proof invariant:

```text
rg "mutator\\.IndexerInfo|mutator\\.MergeIndexerMutations" compiler/check -n
```

The scan returns no matches.

Verification:

```text
go test ./compiler/check/overlaymut ./compiler/check/abstract/transfer/mutator \
  ./compiler/check/infer/return ./compiler/check/synth/phase/extract -count=1
```

The command passed.

## 2026-05-20 Flash Migration: Call Effects Are Domain-Owned

`abstract/transfer/mutator` still mixed two responsibilities: resolving a
call's callee contract to discover table/container mutation effects, and
lowering those effects into flow input assignments. Return inference and
function synthesis also needed the same call-effect interpretation, which made
them reach sideways into transfer mutator code.

Canonical rule:

```text
domain/calleffect    = contract/effect interpretation at concrete call sites
overlaymut           = overlay mutation data and merge/apply laws
abstract/transfer/mutator = transfer reducers that emit flow mutator assignments
infer/synth          = consumers of domain call effects and overlaymut facts
```

The effect interpreters moved to `compiler/check/domain/calleffect`:

- `TableMutatorFromCall`
- `ContainerMutatorFromCall`
- `ContainerElementReturnFromCall`
- table-insert call-evidence reductions used by return/function overlays

The transfer mutator package now only consumes `domain/calleffect` while
emitting `flow.TableMutatorAssignment` and
`flow.ContainerMutatorAssignment`. The old call-effect functions and
table-insert overlay collectors were deleted from transfer/mutator.

Proof invariant:

```text
rg "mutator\\.(CollectTableInsert|TableMutatorFromCall|ContainerMutatorFromCall|ContainerElementReturnFromCall)" \
  compiler/check -n
```

The scan returns no matches. Production imports of
`compiler/check/abstract/transfer/mutator` are now limited to the transfer
orchestrator that runs mutator reducers.

Verification:

```text
go test ./compiler/check/domain/calleffect ./compiler/check/abstract/transfer/mutator \
  ./compiler/check/abstract/assign ./compiler/check/infer/return \
  ./compiler/check/synth/phase/extract -count=1
```

The focused command passed.

## 2026-05-20 Flash Migration: Assignment Interpreter Is Not A Transfer Subpackage

The assignment package had also become a shared abstract-interpreter component,
not just a private transfer reducer. It owns local assignment SCC inference,
RHS overlay synthesis, structured-write visibility, call-argument expectation
inference, and assignment-flow input emission. Return inference was previously
forced to construct a fake `transfer/core.FlowContext` only to call
`CollectInferredTypes`, which encoded the wrong mental model.

Canonical rule:

```text
abstract/assign      = assignment abstract interpreter and assignment reducers
abstract/transfer    = whole-flow transfer orchestration and reducer sequencing
infer/return         = calls abstract/assign local inference with explicit evidence/config
```

The package moved from `compiler/check/abstract/transfer/assign` to
`compiler/check/abstract/assign`. The old `CollectInferredTypes(*FlowContext,
...)` API was deleted. Shared local inference now enters through
`assign.InferLocalTypes(assign.LocalInferenceConfig{...})`, which names the
real inputs: graph, evidence, scopes, synthesis services, seed types,
annotations, optional flow inputs, and optional preflow branch solution.

This is a direct shape correction, not a wrapper: the old package path is gone,
and return inference no longer fabricates a transfer context.

Proof invariant:

```text
rg "compiler/check/abstract/transfer/assign|CollectInferredTypes" compiler/check -n
```

The scan returns no matches.

Verification:

```text
go test ./compiler/check/abstract/assign ./compiler/check/abstract/transfer \
  ./compiler/check/infer/return ./compiler/check/synth/phase/extract \
  ./compiler/check/domain/calleffect -count=1
```

The focused command passed.

Current note: the next entry supersedes this package-boundary statement by
removing `abstract/transfer` entirely. The surviving design is the root
`abstract` interpreter plus direct reducer packages.

## 2026-05-20 Flash Migration: `abstract/transfer` Package Removed

The previous slices exposed the remaining non-final shape: `abstract/assign`
was no longer a transfer subpackage, but it still depended on
`abstract/transfer/core`, `abstract/transfer/cond`,
`abstract/transfer/predicate`, and related reducer packages. That meant the
mental model was still split between "the abstract interpreter" and a legacy
"transfer" namespace.

Canonical rule:

```text
abstract             = top-level abstract interpreter entrypoint
abstract/core        = interpreter context, services, and derived resolvers
abstract/trace       = canonical graph event materialization
abstract/{assign,cond,constprop,decl,mutator,returns,...}
                     = interpreter reducers over one FlowEvidence stream
domain/*             = reusable semantic domains not tied to interpreter lowering
```

The old package `compiler/check/abstract/transfer` was removed. Its root
orchestrator moved into `compiler/check/abstract`:

- `abstract.Run(*core.FlowContext) abstract.Result` is the full interpreter
  entrypoint used by phase extraction.
- `abstract.BuildInputs(*core.FlowContext) *flow.Inputs` is the lower-level
  flow-input construction step.
- `abstract.ExtractEvidence` remains in the interpreter root and records the
  interpreter-owned event stream after input construction.

Reducer packages moved directly under `compiler/check/abstract`:

- `core`
- `cond`
- `constprop`
- `decl`
- `literal`
- `mutator`
- `numconst`
- `predicate`
- `returns`
- `sibling`
- `tblutil`

This is a flash migration, not a bridge. There is no `abstract/transfer`
package, no `RunTransfer`, and no `TransferResult`.
Residual code comments and import aliases were also normalized so production
checker code no longer talks about a transfer namespace or `fbcore`.

Proof invariant:

```text
rg "compiler/check/abstract/transfer|\\btransfer\\.|RunTransfer|TransferResult|package transfer" \
  compiler/check -n
rg "fbcore|abstract transfer|abstract-transfer|transfer-owned|transfer context|\\bTransfer\\b|\\btransfer\\b" \
  compiler/check/abstract compiler/check/api compiler/check/phase/flow.go -n
```

Both scans return no matches.

Verification:

```text
go test ./compiler/check/abstract/... ./compiler/check/phase \
  ./compiler/check/infer/return -count=1
go test ./compiler/check/... ./types/flow/... -count=1
go test ./... -count=1
git diff --check
go test ./compiler/check -run '^$' -bench BenchmarkCheck_LargeFunction -benchmem -count=3
```

The commands passed.

Benchmark sanity after the package removal stayed in the same allocation shape:
about 3.9-4.9 ms/op, 1.08 MB/op, and 10435-10436 allocs/op on this machine.

## 2026-05-20 Validation Boundary: Soundness vs. External Lint Counts

Current validation result:

- The go-lua tree is clean on `go test ./... -count=1`.
- The flash-migration residue scan is clean: no `abstract/transfer`,
  `RunTransfer`, `TransferResult`, `package transfer`, or `fbcore` production
  references remain under `compiler/check`.
- The official `../scripts/verify-suite.sh` is not a proof of this checkout's
  Wippy lint behavior because `/home/wolfy-j/wippy/wippy/go.mod` still pins
  `github.com/wippyai/go-lua v1.5.16`.
- A temporary local-replace Wippy binary built against this checkout still
  reports external diagnostics in several Wippy projects. Therefore the honest
  statement is not "all external lint errors are gone"; the correct next proof
  is classification: real unsound source/manifest use vs. checker false
  positive.

Canonical validation rule:

```text
Do not make go-lua accept unproven any/unknown/optional values just to reduce
external counts. A remaining diagnostic is only a checker regression when the
program has a local proof that the value satisfies the contract and the
abstract interpreter fails to use that proof.
```

Representative local-replace diagnostics classified as soundness-expected:

- `app.test.network:*`: `(args and args.url) or "..."`
  can be non-string when `args.url` is truthy non-string. This is covered by
  `TestExternalLint_UntypedOverlayURLRequiresStringProof`; the guarded variant
  is `TestExternalLint_GuardedOverlayURLFeedsStringContract`.
- `wippy.llm.util:compress`: untyped `configure(new_config)` can write
  non-numeric values into numeric `CONFIG` fields before arithmetic reads.
  This is covered by `TestExternalLint_UntypedConfigMutationCanInvalidateNumericReads`;
  the typed-update variant is
  `TestExternalLint_TypedConfigMutationPreservesNumericReads`.
- `wippy.session.api:get_artifact*`: `artifact.meta` can be non-table, so
  `if artifact.meta then artifact.meta.content_type end` is not enough proof.
  This is covered by `TestExternalLint_StringMetadataRequiresStructuredProof`;
  the table-guarded variant is
  `TestExternalLint_GuardedStructuredMetadataAllowsFieldAccess`.
- `wippy.session.process:command_bus`: a `fun(...any) -> any` handler is not a
  proof of the typed registry contract `(any, any) -> (any, string?)`. This is
  covered by `TestExternalLint_UntypedCommandHandlerCannotEnterTypedRegistry`;
  the typed-handler variant is
  `TestExternalLint_TypedCommandHandlerCanEnterTypedRegistry`.
- `wippy.session.process:control_handlers`: dynamic `any` control payloads do
  not satisfy typed handler records without field/type guards. This is covered
  by `TestExternalLint_DynamicControlPayloadRequiresTypedProof`; the guarded
  variant is `TestExternalLint_GuardedControlPayloadFeedsTypedHandler`.
- `wippy.session:start_tokens_test`: passing `"not a table"` to an optional
  record parameter is a real static contract violation even when the runtime
  test expects validation to reject it. This is covered by
  `TestExternalLint_StartOptionsRejectsPlainString`.

Engine capabilities that remain regression-protected and must stay hack-free:

- optional fallback to concrete strings:
  `TestExternalLint_OptionalStringFallbackIsConcreteString` and
  `TestExternalLint_ManifestHTTPBodyFallbackFeedsManifestJsonDecode`.
- imported assertion summaries and non-nil narrowing:
  `TestExternalLint_ImportedNotNilNarrowsNilInitializedCapturedLocal`,
  `TestExternalLint_ImportedNotNilMakesNilOnlyPathUnreachable`, and
  `TestExternalLint_ImportedNotNilNarrowsCapturedTableMethodWriteLocal`.
- local predicate/effect inference through control flow and loops:
  `TestExternalLint_LocalPredicateGuardNarrowsNumberAfterEarlyReturn`,
  `TestExternalLint_DirectPredicateTrueBranchNarrowsArgument`,
  `TestExternalLint_AssignedPredicateTrueBranchNarrowsArgument`, and
  `TestExternalLint_LogicalPredicateTruePathNarrowsThroughLoop`.
- expected function context for returned callbacks:
  `TestExternalLint_ReturnedCallbackUsesExpectedParameterTypesInBody`,
  `TestExternalLint_ReturnedMethodCallbackUsesExpectedParameterTypesInBody`,
  and `TestExternalLint_ReturnedCallbackContextFlowsThroughLocalProjectionWrite`.

Open proof obligation before claiming "no false positives":

```text
Run the local-replace Wippy lint harness, classify every remaining diagnostic,
and for any diagnostic whose source has a real proof, add a minimal go-lua
regression and fix the abstract interpreter. For diagnostics that are real
source/manifest issues, do not weaken the checker.
```

## 2026-05-20 Revalidation: Current Head Still Has Integration Diagnostics

Current head: `3cb32729`.

Commands rerun:

```text
rg "compiler/check/abstract/transfer|RunTransfer|TransferResult|package transfer|fbcore" compiler/check -n
go test ./compiler/check/tests/regression -run 'ExternalLint|Gradual|Advanced' -count=1
go test ./... -count=1
../scripts/verify-suite.sh
env WIPPY_DIR=/tmp/wippy-golua-validate \
    WIPPY_BIN=/tmp/wippy-local-replace-validate \
    GOFLAGS=-buildvcs=false \
    ../scripts/verify-suite.sh
```

Results:

- migration-residue scan: no matches.
- targeted regression suite: pass.
- full go-lua suite: pass.
- official verify suite: checker tests and Wippy binary build pass, then the
  script exits non-zero on pinned external lint targets:
  - `/home/wolfy-j/wippy/wippy/tests/app`: 0 errors
  - `/home/wolfy-j/wippy/session`: 8 errors
  - `/home/wolfy-j/wippy/framework/src/agent/src`: 11 errors
  - `/home/wolfy-j/wippy/docker-demo`: 21 errors, 2 warnings
  - all other listed targets: 0 errors
- local-replace verify suite against this checkout: checker tests and Wippy
  binary build pass, then external lint reports:
  - `/tmp/wippy-golua-validate/tests/app`: 4 errors
  - `/home/wolfy-j/wippy/session`: 33 errors
  - `/home/wolfy-j/wippy/framework/src/agent/src`: 6 errors
  - `/home/wolfy-j/wippy/docker-demo`: 68 errors
  - `/home/wolfy-j/wippy/framework/src/llm/src`: 3 errors
  - `/home/wolfy-j/wippy/framework/src/llm/test`: 3 errors
  - `/home/wolfy-j/wippy/framework/src/views`: 2 errors
  - all other listed targets: 0 errors

Conclusion:

```text
The flash migration is structurally clean and the go-lua regression suite
passes, but "all regressions are solved" is not proven by integration lint.
The remaining local-replace diagnostics must stay classified individually.
The checker must not add compatibility fallbacks or any-to-contract shortcuts
to hide diagnostics that are real soundness findings.
```

## 2026-05-20 Revalidation: Assertion Refinement Regression Fixed Without Contract Weakening

Current base before this patch: `cd46937b`.

Regression class:

```text
An unannotated assertion-style helper can accept a dynamic argument, prove a
more specific type on normal return, and then use that refined value internally.
That proven internal use is an effect/refinement, not a public parameter
precondition. Merging the body observation into `FunctionFact.Params` made the
helper look like `msg: string`, so callers passing `any` got false positives
even though the helper itself proved `msg:string` before string operations.
```

Canonical rule implemented:

```text
FunctionFact.Params stores caller obligations only.
If a dynamic call observation merges with a concrete body observation, and the
normal-return refinement proves that same concrete type for the parameter, keep
the parameter dynamic in the public function fact.
```

Call checking applies the same rule only when all of these are true:

- the actual argument is `any`;
- the source parameter is unannotated;
- the callee has a normal-return refinement proving the expected parameter type;
- the function AST is available from the canonical graph/evidence store.

This is not an `any` shortcut. Annotated parameters remain authoritative, and
unproven concrete demands remain preconditions.

Implementation shape:

- `compiler/check/domain/functionfact/refinement.go` owns the refinement-proof
  predicate and the join/widen parameter preservation rule.
- `compiler/check/domain/functionfact/fact.go` calls that rule from `Join` and
  `WidenForConvergence`.
- `compiler/check/hooks/call_check.go` uses the same proof predicate when
  checking function facts at a call site, so imported assertion summaries and
  local graph facts have the same semantics.

Regression coverage:

- `TestJoin_RefinementProvenParamDoesNotBecomePrecondition`
- `TestJoin_UnprovenDynamicParamUseRemainsPrecondition`
- `TestImportedTypeAssertionNarrowsArgumentInPlace`
- `TestImportedTypeAssertionNarrowsLocalBeforeStringUse`
- `TestAnnotatedImportedTypeAssertionKeepsPrecondition`

Validation commands run:

```text
go test ./compiler/check/domain/functionfact -count=1
go test ./compiler/check/tests/regression -run 'TestImportedTypeAssertion|TestAnnotatedImportedTypeAssertion|TestLinterFalsePositive_TestRunnerExact|TestLinterFalsePositive_GraphLocalUnusedParamAllowsInternalAny|TestLinterFalsePositive_GraphLocalObservedParamRejectsAny|TestSessionPlugin_UntypedSessionIDGuardStillRejectsStringAPI' -count=1
go test ./... -count=1
git diff --check
env GOFLAGS=-buildvcs=false go build -o /tmp/wippy-local-replace-validate ./cmd/wippy
/tmp/wippy-local-replace-validate lint --cache-reset --json
env WIPPY_DIR=/tmp/wippy-golua-validate WIPPY_BIN=/tmp/wippy-local-replace-validate GOFLAGS=-buildvcs=false ../scripts/verify-suite.sh
```

Results:

- focused function-fact tests: pass.
- focused regression tests: pass.
- full go-lua test suite: pass.
- diff whitespace check: pass.
- local-replace Wippy binary build: pass.
- local-replace `tests/app` replay: 2 errors, 0 warnings, 0 hints.
  The prior `denied_explicit` assertion false positives are gone.
- full local-replace verify suite: checker tests and binary build pass, then
  external lint exits non-zero with:
  - `/tmp/wippy-golua-validate/tests/app`: 2 errors
  - `/home/wolfy-j/wippy/session`: 33 errors
  - `/home/wolfy-j/wippy/framework/src/agent/src`: 6 errors
  - `/home/wolfy-j/wippy/docker-demo`: 66 errors
  - `/home/wolfy-j/wippy/framework/src/llm/src`: 3 errors
  - `/home/wolfy-j/wippy/framework/src/llm/test`: 3 errors
  - `/home/wolfy-j/wippy/framework/src/views`: 2 errors
  - all other listed targets: 0 errors

Current classification boundary:

```text
The fixed regression is the assertion-refined dynamic argument case. The two
remaining `tests/app` overlay diagnostics are soundness-expected: `(args and
args.url) or "..."` proves a string only when `args.url` is nil/false; a truthy
non-string dynamic value still reaches `http.get`.

The broader external local-replace counts remain integration diagnostics, not
proof that go-lua should weaken contracts. Each remaining item must be
classified as source/manifest proof gap vs. checker regression before changing
engine semantics.
```

## 2026-05-20 Validation: No Assertion-Refinement Regression Remains

Validated head: `ff27b721`.

Changed-code design scan:

```text
rg "hack|temporary|transitional|legacy|bridge|compat|fallback|TODO|FIXME|any shortcut|any-to" \
  compiler/check/domain/functionfact/refinement.go \
  compiler/check/domain/functionfact/fact.go \
  compiler/check/hooks/call_check.go \
  compiler/check/tests/regression/imported_type_assertion_refinement_test.go
```

Result: no matches. The refinement fix is not implemented as a fallback,
bridge, name special case, or blanket `any` relaxation.

Legacy fact-channel scan:

```text
rg "FunctionTypesFromFacts|ReturnSummaries|NarrowReturns|FuncTypes|NormalizeFacts|NormalizeFunctionFactChannels|legacy fact|compatibility view|compatibility bridge" \
  compiler/check types
```

Result: no production matches.

Regression proof rerun:

```text
go test ./compiler/check/domain/functionfact -count=1
go test ./compiler/check/tests/regression -run 'TestImportedTypeAssertion|TestAnnotatedImportedTypeAssertion|TestLinterFalsePositive_TestRunnerExact|TestLinterFalsePositive_GraphLocalUnusedParamAllowsInternalAny|TestLinterFalsePositive_GraphLocalObservedParamRejectsAny|TestSessionPlugin_UntypedSessionIDGuardStillRejectsStringAPI|ExternalLint|Gradual|Advanced' -count=1
go test ./... -count=1
git diff --check
```

Results:

- function-fact package: pass.
- targeted regression suite: pass.
- full go-lua suite: pass.
- diff check: pass.

Local-replace replay:

```text
env GOFLAGS=-buildvcs=false go build -o /tmp/wippy-local-replace-validate ./cmd/wippy
/tmp/wippy-local-replace-validate lint --cache-reset --json
env WIPPY_DIR=/tmp/wippy-golua-validate WIPPY_BIN=/tmp/wippy-local-replace-validate GOFLAGS=-buildvcs=false ../scripts/verify-suite.sh
```

Results:

- local-replace binary build: pass.
- `tests/app`: 2 errors, 0 warnings, 0 hints.
- full local-replace verify: checker tests and binary build pass, then external
  lint exits non-zero.

Current local-replace nonzero targets from detailed JSON:

- `/tmp/wippy-golua-validate/tests/app`: 2 errors.
- `/home/wolfy-j/wippy/session`: 33 errors.
- `/home/wolfy-j/wippy/framework/src/agent/src`: 6 errors.
- `/home/wolfy-j/wippy/docker-demo`: 68 errors in standalone JSON replay
  (`verify-suite.sh` printed 67 on the same validation pass).
- `/home/wolfy-j/wippy/framework/src/llm/src`: 3 errors.
- `/home/wolfy-j/wippy/framework/src/llm/test`: 3 errors.
- `/home/wolfy-j/wippy/framework/src/views`: 2 errors.

Assertion-regression check:

```text
jq '.diagnostics[] | select(.entry_id|test("denied_explicit|assert"))'
```

Result: no matching diagnostics in the replayed targets. The
`denied_explicit` false positives remain fixed.

Representative remaining diagnostics inspected:

- `app.test.network:overlay_*`: `(args and args.url) or default` does not prove
  `url:string` when `args.url` is truthy non-string.
- `wippy.llm.util:compress`: `compress.configure(new_config)` writes arbitrary
  values into numeric `CONFIG` fields, so arithmetic on those fields is not
  statically proven.
- `wippy.session.api:get_artifact*`: `artifact.meta` is truthy but not proven to
  be a table before field reads.
- `wippy.session.process:command_bus`: assigning `fun(...any) -> any` into a
  typed `(any, any) -> (any, string?)` handler table is a real contract gap.
- `wippy.views.api:list_pages`: `config_overrides` is cast to a string-keyed map
  without proving the incoming map key shape.
- `wippy.agent.compiler:compiler`: `string.gmatch(tool_id, ...)` receives an
  unproven dynamic `tool_id`.
- `wippy.llm.bedrock:mapper`: fallback text-tool parsing receives dynamic text
  without proving `string?`.

Conclusion:

```text
No go-lua regression is reproduced by the validation suite, and the
assertion-refined dynamic argument regression is absent from local-replace
diagnostics. The implementation is proof-gated by normal-return refinements and
source annotations, not by hacks or broad compatibility.

The remaining external diagnostics are still not a reason to weaken go-lua.
They are dynamic-boundary/source-proof gaps unless a reduced fixture shows that
the abstract interpreter missed an actual local proof.
```

## 2026-05-20 Flash Migration: Function-Fact Call Projection Owner

Problem:

```text
Call checking still owned part of the function-fact semantics. It selected
stable function facts, recovered source functions, projected refinement-proven
dynamic arguments, projected unobserved local parameters, and compared current
callee signatures against fact signatures. That kept the abstract-interpreter
meaning split between `hooks/call_check.go` and `domain/functionfact`.
```

Migration performed:

- Added `compiler/check/domain/functionfact/call_projection.go`.
- Moved stable function-fact call projection into `functionfact.ProjectCall`.
- Kept `hooks/call_check.go` as orchestration only: it now gathers local call
  evidence and asks the function-fact domain for the effective callee.
- Collapsed duplicate function-type rewriting into one private
  `rewriteFunctionParams` helper inside the function-fact domain.
- Removed hook-local helpers:
  - `functionFactCalleeType`
  - `callTypeWithRefinementProvenAnyArgs`
  - `callTypeWithUnobservedLocalAnyArgs`
  - `canonicalFactHasWiderParams`
  - hook-local unobserved-parameter mask handling

Invariant:

```text
The hook may decide when a call must be checked, but it must not encode what a
canonical function fact means. Function-fact projection is part of the
function-fact abstract domain.
```

Why this is a flash migration step, not a bridge:

- No compatibility channel was added.
- No external/source-specific case was added.
- No broad `any` relaxation was added.
- The old hook implementation was deleted, not wrapped.
- The only public surface is the canonical projection entry point:
  `functionfact.ProjectCall`.

Validation:

```text
go test ./compiler/check/domain/functionfact ./compiler/check/hooks -count=1
go test ./compiler/check/tests/regression -run 'TestImportedTypeAssertion|TestAnnotatedImportedTypeAssertion|TestLinterFalsePositive_TestRunnerExact|TestLinterFalsePositive_GraphLocalUnusedParamAllowsInternalAny|TestLinterFalsePositive_GraphLocalObservedParamRejectsAny|TestSessionPlugin_UntypedSessionIDGuardStillRejectsStringAPI|ExternalLint|Gradual|Advanced' -count=1
go test ./... -count=1
git diff --check
```

Results:

- function-fact and hooks package tests: pass.
- targeted regression suite: pass.
- full go-lua suite: pass.
- diff check: pass.

Structural result:

```text
`compiler/check/hooks/call_check.go` no longer contains function-fact
projection semantics. The function-fact domain owns both storage joins and
call-site projection from canonical facts, which is the correct abstract
interpreter boundary for this part of the migration.
```

## 2026-05-20 Flash Migration: Function-Fact Map Construction Owner

Problem:

```text
Return inference still rebuilt `api.FunctionFacts` from provisional return
vectors in local helpers. That meant the canonical map shape existed both in
the function-fact domain and in return-inference consumer code.
```

Migration performed:

- Added `compiler/check/domain/functionfact/map.go`.
- Added canonical constructors:
  - `functionfact.FromParts`
  - `functionfact.FromMaps`
  - `functionfact.FromSummaries`
  - `functionfact.FromSummariesExcept`
- Removed return-inference local builders:
  - `functionFactsFromReturnVectors`
  - `functionFactsExcludingCurrent`
- Replaced local assembly logic with `functionfact.FromMaps`.
- Added function-fact-domain tests for canonical map construction.

Invariant:

```text
Return inference may own provisional SCC return vectors, but it must not own
the shape or normalization policy of published `api.FunctionFacts`.
```

Why this is a flash migration step, not a bridge:

- No compatibility facts were introduced.
- No old builder was wrapped.
- Consumer-local map construction was deleted.
- The published shape now goes through the same `functionfact.Join` and
  `functionfact.Empty` policy used by the domain.

Validation:

```text
go test ./compiler/check/domain/functionfact ./compiler/check/infer/return -count=1
go test ./compiler/check/tests/regression -run 'TestImportedTypeAssertion|TestAnnotatedImportedTypeAssertion|TestLinterFalsePositive_TestRunnerExact|TestLinterFalsePositive_GraphLocalUnusedParamAllowsInternalAny|TestLinterFalsePositive_GraphLocalObservedParamRejectsAny|TestSessionPlugin_UntypedSessionIDGuardStillRejectsStringAPI|ExternalLint|Gradual|Advanced' -count=1
go test ./... -count=1
git diff --check
```

Results:

- function-fact and return-inference package tests: pass.
- targeted regression suite: pass.
- full go-lua suite: pass.
- diff check: pass.

Structural result:

```text
The function-fact domain now owns both per-symbol fact normalization and
canonical map construction. Return inference publishes facts through that
domain instead of reconstructing the product shape locally.
```
