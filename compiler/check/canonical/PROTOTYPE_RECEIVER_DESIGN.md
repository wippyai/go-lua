# Prototype Receiver Design

Status: implemented for the current canonical migration slice.

Implementation result:

- `PrototypeSelf` is a first-class `PointState` and `Summary` product axis.
- Static receiver/prototype topology lives in immutable canonical facts.
- `setmetatable(instance, mt)` and `self.field = value` update the product axis
  in transfer.
- Method entry `self` uses deterministic `EntryValues`, not
  `SetInferredParams`.
- Value receivers are no longer marked declared/annotated from `moduleCaptures`;
  only named receiver types are source-declared.
- Deleted: `captureRefinePasses`, `seedMethodSelfFromCaptures`,
  `enrichPrototypeReceivers`, `joinSelfFieldWrites`, and the metatable/index
  driver scan helpers.
- Gates after deletion: `go test -count=1 ./compiler/check/canonical/...` and
  `go test -count=1 -v -run 'TestCanonicalCurated(Oracle|Gate)' .` both pass;
  curated oracle score is 422/422 with deadlock fixtures 2/2.

This component owns Lua split-pattern object semantics:

```lua
local methods = {}
local mt = { __index = methods }

function node.new(args)
    local instance = { node_id = args.node_id }
    return setmetatable(instance, mt)
end

function methods:route()
    return self.node_id
end
```

At runtime, `self` in `methods:route` is the instance table returned by
`setmetatable(instance, mt)`. The instance carries data fields; method lookup is
provided by `mt.__index = methods`. The abstract interpreter must model that
relation directly. It must not recover it by re-solving the module in the driver
after seeing a diagnostic precision failure.

## 1. Removed Deviation

The pre-migration precision came from a driver-owned second fixed point:

- `captureRefinePasses` is a hard pass cap, not a lattice/widening guarantee.
- `seedMethodSelfFromCaptures` mutates transfer inputs after a solve by calling
  `SetInferredParams` for slot 0.
- `enrichPrototypeReceivers` scans solved point states, reconstructs
  `setmetatable(instance, mt)` data, mutates the module-wide capture map for the
  prototype receiver, then asks the module to solve again.
- `joinSelfFieldWrites` scans solved method bodies for `self.field = value` and
  grafts those fields onto the prototype record outside transfer/summary.
- `receiverType` and the observation bridge read `moduleCaptures` as semantic
  state.

That shape was useful as an oracle, but it was not the canonical interpreter.
The driver was implementing an untyped product-state relation:

```text
prototype symbol -> instance self abstract value
```

The migration made that relation an explicit abstract component and updates it
through transfer.

## 2. Paper constraints

This follows the same lock as the super-design:

- Cousot-Cousot: abstract semantics is a monotone equation system over lattices;
  widening, not pass counting, cuts ascending chains.
- Kildall: per-point state is solved by the worklist over a product domain.
- Sharir-Pnueli: interprocedural precision lives in summary transformers, keyed
  calling contexts, or an equivalent supergraph, not in body patching after a
  local solve.
- Astree-style products: precision comes from explicit cooperating domains and
  reductions, not fixture-specific driver graph walks.
- Salsa: query keys and values are immutable and deterministic. No mutable
  closure installed after query construction may affect cached semantics.

Primary anchors:

- https://cs.nyu.edu/~pcousot/COUSOTpapers/POPL77.shtml
- https://cds.cern.ch/record/120118
- https://research.cs.wisc.edu/wpis/papers/popl95.pdf
- https://salsa-rs.github.io/salsa/reference/algorithm.html

## 3. Concrete semantics

For a metatable edge `mt.__index = proto`, and a construction
`setmetatable(instance, mt)`, every method body whose receiver symbol is `proto`
may be entered with runtime `self = instance`.

Therefore `self` is not the prototype table alone. It is the instance value with
the method surface reachable through the metatable `__index` relation.

The abstract self value for a prototype is the join of:

1. instance values returned or published by `setmetatable(instance, mt)` sites
   whose metatable statically indexes that prototype;
2. fields written by methods on that runtime self, as ordinary transfer effects;
3. the prototype method surface reachable through metatable lookup.

This value is a normal `product.AbstractValue`; metatable-aware field lookup
already exists in the value/query layer. The missing piece is ownership and
fixed-point placement of the prototype-to-self relation.

## 4. Data classification

### Immutable facts

These are finite, syntax/name-resolution facts and belong in
`compiler/check/canonical/facts`:

- `MetatableIndex(mtSym, protoSym)`: from `{ __index = methods }` and
  `Class.__index = Class`.
- `MethodReceiver(ref, protoSym, selfSlot)`: a function literal is a method or
  field definition on `protoSym`; its implicit `self` slot is slot 0.
- `SetMetatableSite(ref, point, instanceExpr, mtSym)`: a site whose second
  argument is a statically resolved metatable symbol.

Facts store stable identities only: `ref.FuncRef`, `cfg.SymbolID`, `cfg.Point`,
slot indexes. They do not store `typ.Type` produced by solved point state.

### Product and summary state

These depend on solved abstract state and must not be facts:

- the concrete abstract value of `instance`;
- field values copied from `args` into `instance`;
- fields assigned later via `self.field = value`;
- caller-context-sensitive precision that flows through captured cells or
  parameters.

These belong in the product fixed point and are carried across functions by
summary.

## 5. Abstract carrier

Add a finite map lattice as a product-state component:

```text
PrototypeSelf = map[cfg.SymbolID]product.AbstractValue
```

Key: method prototype symbol.

Value: the joined runtime `self` value for methods installed on that prototype.

Order, join, widen: pointwise over `product.Domain`, with absent key denoting
bottom. Canonical storage must be sorted or converted to a deterministic key for
query values. A later cleanup may use a typed map carrier instead of raw Go map;
the semantic key is `cfg.SymbolID`, never source string.

Add it to `flow.PointState` and `summary.Summary`:

```text
PointState = Env x Cond x Num x Rel x ReturnRel x Cells x CellEffects x PrototypeSelf
Summary = Returns x Params x Relations x CellEffects x CaptureExports x PrototypeSelf
```

The existing componentwise `PointStateDomain` and `SummaryDomain` are the right
patterns.

## 6. Transfer

Source semantics enter the relation in transfer, not in the driver and not in a
summary projection scan.

For a `setmetatable(instance, mt)` call whose immutable facts resolve
`MetatableIndex[mt] = protoSym`:

```text
instance = eval instanceExpr in current PointState
receiver = instance with metatable/prototype lookup preserved
PointState.PrototypeSelf[protoSym] join= receiver
```

For a method body fact `(ref, protoSym, selfSlot)`, a field assignment:

```text
self.field = value
```

updates the slot's value in `PointState.Env` or a typed successor to Env, then
joins the updated receiver value into `PointState.PrototypeSelf[protoSym]`.

This is the canonical replacement for `joinSelfFieldWrites`: the write is handled
where assignment semantics already live. No driver scan is allowed to add fields
after solving.

## 7. Summary projection

Summary projection carries the already-solved relation across function
boundaries:

```text
Summary.PrototypeSelf = join of solved PointState.PrototypeSelf at exits
                        and any caller-visible receiver effects
```

Projection does not rediscover `setmetatable` sites or self writes. It only
projects product state.

## 8. Entry seed

Method `self` is an entry input, not a post-solve transfer mutation and not a
parameter contract.

Extend the intraprocedural builder input with product-domain entry values:

```text
EntryValues(ref) = deterministic slot -> product.AbstractValue relation
```

For a method fact `(ref, protoSym, selfSlot)`:

```text
EntryValues(ref)[selfSlot] =
    call-site receiver value, when solving a receiver-keyed method call
    OR join over receiver dependency summaries: Summary.PrototypeSelf[protoSym]
```

The builder writes these values into the entry point's `PointState` before
running the node transfer at the entry. The transfer's ordinary entry seeding
then applies declared parameters, call-site inferred parameters, and co-solved
contracts. Those stronger sources overwrite the fallback value instead of being
joined with it.

Named-type receiver annotations still win. An explicit user annotation is a
declared input, so the program only returns an `EntryValues` fallback when the
slot is unannotated or absent from declared parameter facts.

## 9. Dependencies

The method summary has a semantic dependency on summaries that can publish
`PrototypeSelf` for its receiver prototype. This dependency is not necessarily a
call edge: constructors and methods may be siblings.

The program summary interface therefore needs one of these equivalent shapes:

```text
ReceiverDependencies(ref) []FuncRef
ReceiverSelf(ref, summaryOf) PrototypeSelf
```

or a program-level equation cell:

```text
PrototypeSelf(protoSym)
```

The first shape is the smaller migration. It records a db dependency from a
method summary to summaries whose product projection may publish its `protoSym`;
db cycles and `SummaryWiden` handle constructor/method recursion. The second
shape is cleaner long-term if more receiver-like relations appear.

Method-call summaries should also be receiver-keyed when the call site has a
known receiver value. That is the call-string/summary-transformer path for
ordinary `obj:m()` calls. The prototype dependency is the fallback needed for
module-level diagnostics and sibling constructor/method patterns where a method
body is solved outside a concrete call site.

Either shape must be deterministic:

- sort refs by `FuncRef`;
- sort prototype keys by `cfg.SymbolID`;
- do not depend on Go map iteration;
- do not key semantic state by source names or stringified types.

## 10. Value keys

`flow.PointState.Env` uses a typed value key. The deterministic encoding still
looks like `s<ID>` for symbols and `r<ID>` for return slots, but that encoding is
not the semantic API.

```go
type ValueKey string
```

The legacy flow solver's stable value store and point-mutable stores are keyed by
`constraint.PathKey`, not `map[string]product.AbstractValue`. Prototype receiver
work must not add new semantic string maps. New relations use typed ids
(`cfg.SymbolID`, `ref.FuncRef`, `cfg.Point`, slot indexes).

## 11. Migration Slice

Implemented order:

1. Added immutable fact collectors for metatable/prototype and method receiver
   sites.
2. Added `flow.PrototypeSelf` carrier and law tests.
3. Extended `flow.PointState`, `summary.Summary`, and both componentwise
   domains.
4. Updated transfer so `setmetatable(instance, mt)` and `self.field = value`
   update `PointState.PrototypeSelf`.
5. Projected solved `PointState.PrototypeSelf` into summaries.
6. Added builder entry values and seeded unannotated method `self` from summary
   receiver dependencies.
7. Deleted `seedMethodSelfFromCaptures`.
8. Deleted `enrichPrototypeReceivers`, `joinSelfFieldWrites`, and their helper
   scans after the scoped oracle stayed green.
9. Deleted the `captureRefinePasses` loop.
10. Current capture-cell cut has also removed the closure-method flow-back
    compatibility path from live Go code; remaining mentions are migration
    history and forbidden-shape documentation.

## 12. Acceptance gates

This component is closed:

- `MetatableIndex` and method receiver facts have deterministic unit tests;
- `PrototypeSelf` has lattice law tests;
- transfer tests prove `setmetatable(instance, mt)` exports instance fields to
  `PointState.PrototypeSelf`;
- transfer tests prove `self.field = value` updates both the self slot and the
  receiver relation for that method's prototype;
- summary projection tests prove `PrototypeSelf` is carried, not manufactured;
- method-entry tests prove slot 0 receives the summary self seed without
  `SetInferredParams`;
- the deadlock-dataflow-node precision case stays green when
  `seedMethodSelfFromCaptures`, `enrichPrototypeReceivers`, and
  `joinSelfFieldWrites` are disabled or removed;
- scoped canonical tests pass;
- curated oracle/gate stays green;
- no global fixture suite is invoked for this component.

Forbidden acceptance evidence:

- another bounded driver loop;
- another post-solve mutation of transfer inputs;
- another scan that patches `moduleCaptures`;
- `map[string]typ.Type` or source-name maps as internal semantic state;
- fixture-specific branches for `deadlock-dataflow-node`.
