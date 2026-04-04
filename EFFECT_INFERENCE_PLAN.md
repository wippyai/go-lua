# Effect Inference & Enforcement Plan

## Goal

Infer ownership effects (Borrow/Store/Send/Freeze) for user-defined functions and enforce them at call sites. This is the foundation for arena-based allocation and actor-safe value transfer.

## Current State

- Effect vocabulary: COMPLETE (Borrow, Store, Send, Freeze, BorrowAll, PassThrough, FlowInto)
- Builtin annotations: COMPLETE (rawset→Store, table.insert→Store, print→BorrowAll, etc.)
- Propagation engine: COMPLETE (fixpoint loop unions callee effects into caller)
- Serialization: COMPLETE (all labels round-trip through module manifests)
- Inference for user code: MISSING
- Enforcement: MISSING
- Diagnostics: MISSING

## Escape Sites (Where inference must happen)

### Table store: `t[k] = v` / `t.field = v`
- **File**: `compiler/check/flowbuild/assign/emit.go`
- **Location**: `TargetField` at line 515, `TargetIndex` at line 562
- **Effect**: If `v` is a parameter → `Store{Param: paramIdx(v), Into: paramIdx(t)}`
- **Note**: Only when target base symbol is a parameter or upvalue

### Upvalue capture
- **File**: `compiler/check/infer/captured/captured.go`, `FromParentFacts` line 11
- **Binding**: `compiler/bind/table.go`, `CapturedSymbols` line 428
- **Effect**: If captured symbol is a parameter AND the closure escapes → `Store{Param: paramIdx}`
- **Note**: Connect to `CaptureInfo.Escapes` (types/typ/capture.go line 28)

### Return escape
- **File**: `compiler/cfg/graph.go`, `EachReturn` line 647
- **Data**: `ReturnInfo.Symbols` in `compiler/cfg/types.go` line 349
- **Effect**: If returned symbol is a parameter → parameter escapes via return. Model as PassThrough (already exists) or Store{Into: -1}

### Yield escape  
- **File**: `compiler/stdlib/coroutine.go` line 20
- **Fix**: Annotate `coroutine.yield` with `Send{FromParam: 0}`
- **Note**: No new label needed, Send covers this

### Global store
- **File**: `compiler/check/flowbuild/assign/emit.go`, `TargetIdent` at line 280
- **Condition**: `bindings.Kind(sym) == cfg.SymbolGlobal`
- **Effect**: If RHS is a parameter → `Store{Param: paramIdx, Into: -1}`

## No New Labels Needed

| Pattern | Label | Rationale |
|---------|-------|-----------|
| Table store | Store{Param, Into} | Value stored into structure |
| Closure capture + escape | Store{Param, Into: -1} | Capture is opaque store |
| Return | PassThrough / FlowInto | Already exist |
| Yield | Send{FromParam} | Cross-boundary transfer |
| Global store | Store{Param, Into: -1} | Long-lived destination |
| Freeze | Freeze{Param} | Already exists |

## Implementation

### Step 1: Annotate stdlib gaps
- File: `compiler/stdlib/coroutine.go`
- Add Send to yield, BorrowAll to pure coroutine functions
- Review channel/process stubs for missing Send annotations

### Step 2: Create `InferOwnershipEffects`
- New file: `compiler/check/effects/ownership.go`
- Signature: `func InferOwnershipEffects(graph *cfg.Graph, bindings *bind.Table, params []ParamInfo) effect.Row`
- Walks: EachAssign (field/index targets), EachReturn, captured symbols
- Produces: Store/Borrow labels relative to function parameters
- For parameters not observed to escape → Borrow{Param: i}
- For parameters that escape → Store{Param: i, Into: ...}

### Step 3: Integrate into effects.Propagate
- File: `compiler/check/effects/propagate.go` line 20
- After computing fnEffect.Row, call InferOwnershipEffects and union result
- Ownership labels enter the fixpoint alongside Throw/IO/Diverge

### Step 4: Translate callee ownership at call sites
- The critical piece: callee Store labels reference callee params, not caller params
- New function: `TranslateCalleeOwnership(calleeRow, callInfo, callerParams) effect.Row`
- For each callee Store{Param: i}, find which caller expression maps to callee param i
- If that expression is a caller param → Store{Param: callerParamIdx} on caller
- If expression is not a param (literal, local) → no ownership effect on caller
- Integrate into the EachCallSite callback in Propagate

### Step 5: Enforcement hooks
- File: `compiler/check/hooks/call_check.go`, `checkSingleCall` line 64
- Check: callee has Store + arg is frozen → diagnostic
- Check: callee has Send + arg is mutable → diagnostic or auto-freeze
- Check: callee has Freeze → mark arg as frozen in subsequent flow

### Step 6: Testing
- Unit tests: `compiler/check/effects/ownership_test.go`
- Fixtures: `testdata/fixtures/ownership/{table-store,closure-capture,return-escape,global-store,frozen-write}/`
- Cross-module: multi-file fixtures verifying exported function effects
- Effect assertions: extend fixture harness with `-- expect-effect: store(param[0])` or check via `sess.Store.InterprocPrev.Refinements`

## Performance

- InferOwnershipEffects runs once per function per fixpoint iteration (cheap)
- Body scan is O(assignments + returns + closures) — already iterated
- Effect rows use deduplication — adding same label twice is no-op
- Union is O(n*m) where n,m are label counts — typically 1-5 per function
- No new flow domain needed — ownership is function-level summary, not per-point lattice

## CaptureInfo relationship

- `CaptureInfo.Escapes` (types/typ/capture.go) → runtime codegen concern (heap vs stack upvalue)
- Effect `Store` label → type-level ownership concern (lifetime extends beyond call)
- Complementary, not redundant
- When inferring ownership: consult CapturedSymbols to identify captures, use CaptureInfo.Escapes to determine if closure escapes

## Dependency Order

```
Step 1 (stdlib annotations)     → independent
Step 2 (InferOwnershipEffects)  → needs graph API understanding  
Step 3 (integrate Propagate)    → depends on Step 2
Step 4 (callee translation)     → depends on Step 3, most complex
Step 5 (enforcement)            → depends on Steps 3-4
Step 6 (testing)                → parallel with each step
```

## Critical Files

| Purpose | File |
|---------|------|
| Inference integration | `compiler/check/effects/propagate.go` |
| Escape site detection | `compiler/check/flowbuild/assign/emit.go` |
| Enforcement | `compiler/check/hooks/call_check.go` |
| Label definitions | `types/effect/label.go` |
| Fixpoint loop | `compiler/check/pipeline/driver.go` |
| Capture binding | `compiler/bind/table.go` |
| Capture types | `compiler/check/infer/captured/captured.go` |
| Return info | `compiler/cfg/graph.go` (EachReturn) |
| Effect export | `compiler/check/effects/export.go` |
| Stdlib annotations | `compiler/stdlib/coroutine.go` |
