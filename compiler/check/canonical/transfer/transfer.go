// Package transfer is the per-node transfer of the intraprocedural solver: the
// NodeTransfer the equation graph injects.
//
// It implements equation.NodeTransfer by interpreting one CFG node's syntactic
// evidence against the incoming flow.PointState and returning the post-node
// state. It carries no fixed-point driver and no private mutable store:
// the equation.Builder owns the worklist, the predecessor join, and the
// widening; this transfer is a pure function of (point, incoming, contracts).
//
// The per-node semantics are expressed directly over PointState domains:
//
//   - value: the Env maps a variable's symbol key to its product.AbstractValue.
//     A local declaration or assignment writes the source expression's value;
//     reads project the stored value. Joins and widens are the domain's
//     (product.Domain via the Env MapLattice), so a loop that accumulates a
//     growing type converges by the value-domain ACC widening at the loop-header
//     feedback-vertex set.
//   - numeric: a counter incremented by a constant in a loop body raises its
//     relational bound; the numeric domain's widen at the loop header cuts the
//     unbounded ascent to Top.
//   - condition: a branch narrows the path condition on its outgoing edges; the
//     transfer folds the branch's truthy/falsy/nil test into Cond.
//
// A body use that pins a parameter's type — a read, a field access, or a call
// argument — emits the observed requirement into that parameter's contract via
// the demand sink, which the builder routes backward to entry.
//
// SCOPE: this transfer covers the node kinds the end-to-end gate exercises —
// local declaration and assignment (literal, identifier, arithmetic,
// table-literal, and call sources), identifier reads, branch-condition
// narrowing, parameter-use demand, and the numeric counter increment. Node
// kinds NOT yet handled are recorded in DeferredNodeKinds for a follow-up;
// reaching one is a no-op carry-forward (sound: it loses precision, never
// soundness), never a half-implemented effect.
package transfer

import (
	"math"
	"sort"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/callsite"
	"github.com/wippyai/go-lua/compiler/check/canonical/input"
	canonicalplace "github.com/wippyai/go-lua/compiler/check/canonical/place"
	"github.com/wippyai/go-lua/compiler/check/domain/callobligation"
	abstractcond "github.com/wippyai/go-lua/compiler/check/domain/cond"
	"github.com/wippyai/go-lua/compiler/check/domain/fieldkey"
	"github.com/wippyai/go-lua/compiler/check/domain/guard"
	"github.com/wippyai/go-lua/compiler/check/domain/iteration"
	"github.com/wippyai/go-lua/compiler/check/domain/literal"
	"github.com/wippyai/go-lua/compiler/check/domain/metatable"
	"github.com/wippyai/go-lua/compiler/check/domain/paramevidence"
	domainpath "github.com/wippyai/go-lua/compiler/check/domain/path"
	"github.com/wippyai/go-lua/compiler/check/synth/ops"
	"github.com/wippyai/go-lua/compiler/pathseg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/flow/pathkey"
	"github.com/wippyai/go-lua/types/flow/propagate"
	"github.com/wippyai/go-lua/types/kind"
	querycore "github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// DeferredNodeKinds names the CFG node kinds and source forms this transfer does
// not yet interpret. Reaching one carries the incoming state forward unchanged
// (sound: precision loss, not unsoundness). Listed for the follow-up that
// extends current transfer coverage.
//
// Implemented (no longer deferred): field/index writes (t.f = v, t[k] = v) and the
// read-back (product.WithField/WriteIndex/FieldOf/IndexOf over the Env);
// container-targeted function definitions (function M.f); table-constructor field
// typing; call-return typing through the bridged signatures (predeclared globals,
// recursive/forward function references); path-sensitive condition narrowing
// (x ~= nil, type(x) == k, x.kind == "tag") per branch edge via NarrowEdge
// (narrow.go).
var DeferredNodeKinds = []string{
	"container/map writes not represented by table.insert or spec-level ContainerElementUnion (table.remove)",
	"generic-for iteration variable element typing",
	"method-call receiver narrowing (unannotated self) and OnReturn argument refinement",
	"logical-op (and/or) value flow and field-default (x.f or d) patterns",
}

// OperatorResolver resolves operator result types from operand types. It mirrors
// flow.OperatorResolver: the transfer consumes it for arithmetic/relational
// sources when present, and falls back to a structural default otherwise. It is
// optional so a caller without a synthesis resolver can still run.
type OperatorResolver interface {
	BinaryOp(left typ.Type, op string, right typ.Type) typ.Type
	UnaryOp(op string, operand typ.Type) typ.Type
}

// FuncTyper resolves a function literal to its declared signature (parameter and
// return annotations resolved against the module base scope). The transfer
// consumes it to type a function-valued source — a table-literal field that holds
// a function, so a field read of it resolves to a callable rather than the empty
// record. It is optional: a nil typer leaves a function-valued source untyped (the
// sound carry-forward), the prior behavior.
type FuncTyper interface {
	FuncType(fn *ast.FunctionExpr) *typ.Function
	// MethodFuncType resolves a method definition's signature with the implicit
	// leading `self` parameter typed as the receiver's class. info carries the
	// receiver (the class table T in `function T:m()`); the returned signature is
	// the callable the class field T.m holds, so an instance assignment whose field
	// the method backs (`on = T.on`) sees `fun(self: T, ...)` rather than the
	// self-stripped source signature.
	MethodFuncType(info *cfg.FuncDefInfo) *typ.Function
}

type functionRefProvider interface {
	FuncRef(fn *ast.FunctionExpr) (flow.FunctionRef, bool)
	MethodFuncRef(info *cfg.FuncDefInfo) (flow.FunctionRef, bool)
}

type closureCaptureProvider interface {
	CapturedSymbols(ref flow.FunctionRef) []cfg.SymbolID
}

type closureReferenceProjectionProvider interface {
	ReferenceProjection(ref flow.FunctionRef) flow.ReferencePathProjection
}

// CallTyper is the transfer's seam for call-shaped recognizers that are not the
// selected product-call outcome: iteration effects and callable-type casts. The
// driver implements it by resolving the callee/receiver type (from module
// signatures, predeclared globals, captured values, or the live Env via the
// supplied resolver). Selected call-outcome facts are exposed through
// ProductCallProvider so values, effects, obligations, and control travel in one
// carrier.
//
// exprType resolves an expression's value type against the live point Env (the
// transfer's evalExpr): the driver reads it for the callee, the receiver, and any
// callee/receiver field path whose value the transfer tracks. It returns the
// value-domain unknown for an expression the transfer does not determine, so the
// driver falls back to its module-wide signatures/globals.
type CallTyper interface {
	// IterVars types a generic-for loop's iteration variables from the loop's
	// iterator expression (`for i, v in ipairs(arr)`): it resolves the iterator
	// function's iteration effect (indexed/keyed) and the iterated container's
	// element/key/value types, returning one type per loop variable. count is the
	// loop-variable count. exprType resolves an argument/source expression against the
	// live Env. It returns false when the iterator is not a recognized iteration form,
	// so the transfer leaves the loop variables untyped (the sound carry-forward).
	IterVars(iter *ast.FuncCallExpr, count int, exprType func(ast.Expr) typ.Type) ([]typ.Type, bool)
	// KeyedIterSource reports whether iter is a keyed (pairs-style) iteration and,
	// if so, returns the iterated source-argument expression. It resolves the
	// iterator function's declared iteration effect (the same contract-spec / builtin
	// recognition IterVars uses), so a key drawn from a keyed iteration's first loop
	// variable is provably a key of that source container. A non-keyed iterator
	// (ipairs-style indexed, or an unrecognized form) yields false.
	KeyedIterSource(iter *ast.FuncCallExpr) (ast.Expr, bool)
	// IndexedIterSource reports the live source array path for an indexed
	// (ipairs-style) iteration. Transfer uses it to read point-sensitive
	// key-array provenance from PointState.KeyPresence; the proof itself is not
	// recomputed here.
	IndexedIterSource(iter *ast.FuncCallExpr) (constraint.Path, bool)
	// KeysCollectorContainer reports whether call's return slot retIndex is the
	// keys array returned by a structurally recognized keys-collector, and if so
	// returns the runtime container whose keys it holds. Transfer seeds this as live
	// product-state provenance on the assigned array; indexed iteration consumes
	// only the live fact after intervening writes have had a chance to kill it.
	KeysCollectorContainer(call *cfg.CallInfo, retIndex int) (constraint.Path, bool)
	// TypeCastTarget reports whether call is a type-cast/assertion call `T(arg)` (a
	// type name used as a callable constructor, recognized by the same CallableType
	// effect the call-return typing uses), and if so returns the asserted type T. A
	// failed cast raises, so on the post-call path the argument provably IS T; the
	// transfer narrows the argument value to T. A call that is not a type cast yields
	// false.
	TypeCastTarget(call *ast.FuncCallExpr, exprType func(ast.Expr) typ.Type) (typ.Type, bool)
}

type IterVarProjector interface {
	IterVarProjection(iter *ast.FuncCallExpr, count int, exprType func(ast.Expr) typ.Type) (iteration.VarProjection, bool)
}

// CallEffects groups product call-outcome effects that mutate caller-visible
// state after a call returns. Transfer consumes this as one carrier so call-side
// effects are projected once and then applied by axis-specific reducers.
type CallEffects struct {
	CellEffects     flow.CaptureEffects
	ReceiverEffects flow.ReceiverEffects
	BoundaryFacts   flow.BoundaryFacts
	ElementUnions   []effect.ContainerElementUnion
}

func EmptyCallEffects() CallEffects {
	return CallEffects{
		CellEffects:     flow.CaptureEffectsDomain.Bottom(),
		ReceiverEffects: flow.ReceiverEffectsDomain.Bottom(),
		BoundaryFacts:   flow.BoundaryFactsDomain.Top(),
	}
}

// ProductCallResult is the product-carrier result of evaluating one concrete
// call site. Values, callable identities, return relations, and caller-visible
// effects, pre-call obligations, and control facts travel together so transfer
// does not rebuild selected call outcomes through parallel provider routes.
type ProductCallResult struct {
	ReturnValues    []product.AbstractValue
	HasReturnValues bool
	ReturnRefs      flow.ReturnRefs
	ReturnRelations flow.ReturnRelations
	Effects         CallEffects
	ArgDemands      []callobligation.Obligation
	NeverReturns    bool
	ParamNarrows    []ParamNarrow
}

func EmptyProductCallResult() ProductCallResult {
	return ProductCallResult{
		ReturnRelations: flow.ReturnRelationsDomain.Top(),
		Effects:         EmptyCallEffects(),
	}
}

type ProductCallProvider interface {
	ProductCallFromValues(call *ast.FuncCallExpr, ctx ProductCallContext) ProductCallResult
}

type functionValueProvider interface {
	FunctionValue(query flow.CallableSignatureQuery) (typ.Type, bool)
}

// Config is immutable construction-time configuration for a Transfer. These are
// analysis inputs/resolvers, not mutable post-build state.
type Config struct {
	// Ops resolves arithmetic/relational operator result types. Nil falls back to
	// the structural default (arithmetic on numbers stays numeric).
	Ops OperatorResolver
	// FuncTyper resolves a function literal's declared signature, so a
	// function-valued table-literal field types as a callable. Nil leaves such a
	// field untyped (the sound carry-forward).
	FuncTyper FuncTyper
	// CallTyper exposes call-derived facts. When it also implements
	// ProductCallProvider, expression calls receive product results; otherwise a
	// call result remains untyped (the sound carry-forward).
	CallTyper CallTyper
	// TypeChecks are type-check value-narrowing binds precomputed from graph
	// assignments such as `local val, err = T:is(x)`.
	TypeChecks []guard.TypeCheckBind
	// SelfType, when non-nil, is the receiver class of a method body's implicit
	// `self` (function T:m()): it seeds self's entry value.
	SelfType typ.Type
	// CastType resolves the annotated type of an `expr :: T` cast against the
	// function's base scope.
	CastType func(expr ast.TypeExpr) typ.Type
	// TypeNameValue resolves an identifier that names a type used as a value to the
	// reified Meta of that type.
	TypeNameValue func(name string) typ.Type
	// MethodReceivers are immutable receiver/prototype topology facts for this
	// function. They seed runtime self through the PrototypeSelf product axis.
	MethodReceivers []metatable.MethodReceiver
	// SetMetatableSites are this function's static setmetatable construction sites.
	SetMetatableSites []metatable.SetMetatableSite
	// MetatableIndexes are module-wide static metatable -> prototype facts.
	MetatableIndexes []metatable.Index
	// PrototypeMethods are module-wide static prototype method identities. Transfer
	// publishes them into FunctionRefs when an instance is linked to a prototype.
	PrototypeMethods []metatable.PrototypeMethod
	// PredicateFacts are module-local type-predicate function facts.
	PredicateFacts []guard.PredicateFunction
	// PredicateGuards are this function's assigned predicate-result facts.
	PredicateGuards []guard.PredicateResult
}

// Transfer is the canonical per-node transfer. It carries no fixed-point state:
// every Transfer call is a pure function of the incoming state and the node's
// evidence. Transfer-owned reductors may memoize immutable CFG/expression
// reductions, but not abstract values from solver iterations.
type Transfer struct {
	in input.Inputs
	// ops resolves arithmetic/relational operator result types. Nil falls back
	// to the structural default (arithmetic on numbers stays numeric).
	ops OperatorResolver
	// funcTyper resolves a function literal's declared signature, so a
	// function-valued table-literal field types as a callable. Nil leaves such a
	// field untyped (the sound carry-forward).
	funcTyper FuncTyper
	// callTyper exposes call-derived facts. Product results are consumed only when
	// this value also implements ProductCallProvider.
	callTyper CallTyper
	// paramBySym maps a parameter's symbol ID to its parameter index, so a body
	// use of a parameter routes demand to the right contract cell.
	paramBySym map[cfg.SymbolID]int
	// typeCheckByErr maps a type-check assignment's error-result symbol to the
	// value narrowing the guard proves on the err == nil edge. NarrowEdge reads it
	// so a branch testing the error symbol narrows the checked value to the checked
	// type.
	typeCheckByErr map[cfg.SymbolID]guard.TypeCheckBind
	// predicateByFunc maps a local type-predicate function's symbol to the parameter
	// and kind it tests. NarrowEdge reads it so a branch `if P(arg)` narrows arg to
	// the predicate's kind on the true edge.
	predicateByFunc map[cfg.SymbolID]guard.PredicateFunction
	// predicateByCondSym maps an assigned predicate result `local ok = P(arg)` keyed
	// by the ok symbol to the argument narrowing it proves. NarrowEdge reads it so a
	// branch `if ok` narrows the argument on the true edge, the assigned counterpart
	// of the direct-call form.
	predicateByCondSym map[cfg.SymbolID]guard.PredicateResult
	// unannotatedParam marks a parameter symbol with no declared type. A read of one
	// the body does not pin resolves to gradual `any` (a Lua parameter without an
	// annotation is dynamic, usable in every operation), the same default the
	// observation surface applies. The transfer reads it when typing an operator over
	// such a parameter (`prefix or "["`) so the result is the gradual join rather than
	// undetermined.
	unannotatedParam map[cfg.SymbolID]bool
	// declaredTypes maps an annotated symbol (a parameter or a `local r: A|B = ...`
	// declaration) to its declared type. Discriminant / typeof / equality narrowing
	// reads it to narrow over the DECLARED type rather than the precise constructor
	// value the Env seeds: a `local r: A|B = {tag="a", ...}` seeds the singleton
	// `{tag:"a",...}`, so excluding `r.tag=="a"` on the false edge would collapse it to
	// Never, but the false edge must keep the declared variant B. Narrowing the union
	// recovers the consistent variant(s) per edge; the merge-LUB rebuilds A|B.
	declaredTypes map[cfg.SymbolID]typ.Type
	// declaredParamBySlot maps a parameter SLOT index to its resolved declared type.
	// The canonical ParamSymbols layout includes an implicit method receiver `self` at
	// slot 0, whereas the source-indexed DeclaredParamTypes keys by the position in
	// fn.ParList; for a `function T:m(x: A)` the two differ by one. This map re-keys the
	// declared types into slot order through the graph's ParamSlots so seedEntry pins
	// each parameter slot to its own annotation rather than the previous slot's.
	declaredParamBySlot map[int]typ.Type
	// symbolStorage maps lexical symbols to the PointState storage axis they use
	// (Env, owner cell, or captured cell). It is the transfer-owned compiler policy
	// above the flow package's primitive PointFacts/PointWriter mechanics.
	symbolStorage symbolStoragePolicy
	// castType resolves the annotated type of an `expr :: T` cast against the
	// function's base scope. A `::` cast asserts the operand has type T, so the
	// expression's value is T regardless of the operand's inferred type (the same
	// unsafe-cast semantics the synth flow applies via ResolveType). Without it a
	// cast resolves to unknown and a `pairs(m :: {[string]: string})` loop, or a
	// `(x :: SomeType)` operand, collapses to unknown. Nil leaves a cast resolved
	// only by its inner expression (the sound carry-forward).
	castType func(expr ast.TypeExpr) typ.Type
	// typeNameValue resolves an identifier that names a TYPE used as a value
	// (`M.AppError = AppError`, where AppError is a `type` not a local) to the
	// reified Meta of that type — the type value carrying the built-in `:is` guard
	// method. The driver binds it to the base scope's MetaForName, which yields the
	// Meta only when the name is a defined type with NO shadowing value binding, so a
	// real value variable is never mistaken for a type value. Nil leaves a bare
	// identifier resolved only by its Env value (the sound carry-forward).
	typeNameValue func(name string) typ.Type
	// prototypeReceiverSym is the prototype symbol this method body's `self`
	// belongs to. setMetatableProtoByPoint records construction sites in this
	// function that publish runtime self values for a prototype.
	prototypeReceiverSym     cfg.SymbolID
	prototypeSelfSlot        int
	prototypeSelfSymbol      cfg.SymbolID
	prototypeMethods         []metatable.PrototypeMethod
	metatablePrototypeBySym  map[cfg.SymbolID]cfg.SymbolID
	setMetatableProtoByPoint map[cfg.Point]cfg.SymbolID
	conditionProjector       *propagate.ConditionProjector
	branchConditions         branchConditionReductor
	// loopAppendLengthsByPoint indexes passive loop-summary facts by the point
	// where the fact first holds. Transfer applies them before interpreting the
	// point's own node so ordinary writes can kill stale proofs.
	loopAppendLengthsByPoint map[cfg.Point][]input.LoopAppendLengthFact
}

// New builds the transfer for the given canonical inputs. in.Scope.DeclaredTypes
// maps annotated symbols (parameters, globals, or annotated locals) to their
// declared type, so edge narrowing refines the declared union rather than the
// precise constructor value the Env seeds; absence leaves narrowing on the Env
// value.
func New(in input.Inputs, config Config) *Transfer {
	t := &Transfer{
		in:            in,
		ops:           config.Ops,
		funcTyper:     config.FuncTyper,
		callTyper:     config.CallTyper,
		paramBySym:    make(map[cfg.SymbolID]int),
		declaredTypes: in.Scope.DeclaredTypes,
		castType:      config.CastType,
		typeNameValue: config.TypeNameValue,
	}
	t.branchConditions = branchConditionReductor{
		transfer:  t,
		pathCache: make(map[abstractcond.PathCacheKey]constraint.Path),
	}
	t.declaredParamBySlot = declaredParamBySlot(in)
	// A method body's implicit `self` occupies slot 0 with no source annotation, so the
	// slot map has no entry for it; seed it from the resolved receiver class so the
	// entry state pins self to its record.
	if config.SelfType != nil && len(in.Scope.ParamSymbols) > 0 {
		if t.declaredParamBySlot == nil {
			t.declaredParamBySlot = make(map[int]typ.Type, 1)
		}
		t.declaredParamBySlot[0] = config.SelfType
	}
	for i, sym := range in.Scope.ParamSymbols {
		if sym == 0 {
			continue
		}
		t.paramBySym[sym] = i
		if _, declared := t.declaredParamBySlot[i]; !declared {
			if t.unannotatedParam == nil {
				t.unannotatedParam = make(map[cfg.SymbolID]bool)
			}
			t.unannotatedParam[sym] = true
		}
	}
	t.symbolStorage = newSymbolStoragePolicy(in.Graph, t.paramBySym, in.Scope.CellSymbols)
	for _, b := range config.TypeChecks {
		if b.ErrSym == 0 || b.Type == nil || len(b.NarrowSyms) == 0 {
			continue
		}
		if t.typeCheckByErr == nil {
			t.typeCheckByErr = make(map[cfg.SymbolID]guard.TypeCheckBind, len(config.TypeChecks))
		}
		t.typeCheckByErr[b.ErrSym] = b
	}
	t.installPrototypeReceiverFacts(config.MethodReceivers, config.SetMetatableSites, config.MetatableIndexes, config.PrototypeMethods)
	t.installPredicateGuards(config.PredicateFacts, config.PredicateGuards)
	t.loopAppendLengthsByPoint = indexLoopAppendLengthFacts(in.LoopAppendLengths)
	if in.ConditionDemand != nil {
		t.conditionProjector = propagate.NewConditionProjector(&propagate.Inputs{
			Graph:  in.Graph,
			Demand: in.ConditionDemand,
		})
	}
	return t
}

// ConditionProjector supplies the canonical equation builder with the shared
// condition relevance abstraction for this function. It is derived solely from
// CFG reads/defs, so it is immutable transfer input rather than mutable solver
// state.
func (t *Transfer) ConditionProjector() *propagate.ConditionProjector {
	if t == nil {
		return nil
	}
	return t.conditionProjector
}

// installPrototypeReceiverFacts installs immutable receiver/prototype topology
// facts for this function. The facts are pre-solve identities; transfer uses
// them to update the PrototypeSelf product axis when source semantics run.
func (t *Transfer) installPrototypeReceiverFacts(receivers []metatable.MethodReceiver, sites []metatable.SetMetatableSite, metas []metatable.Index, methods []metatable.PrototypeMethod) {
	t.prototypeReceiverSym = 0
	t.prototypeSelfSlot = 0
	t.prototypeSelfSymbol = 0
	t.prototypeMethods = append([]metatable.PrototypeMethod(nil), methods...)
	t.metatablePrototypeBySym = nil
	for _, m := range receivers {
		if m.PrototypeSym == 0 || m.SelfSlot < 0 || m.SelfSlot >= len(t.in.Scope.ParamSymbols) {
			continue
		}
		sym := t.in.Scope.ParamSymbols[m.SelfSlot]
		if sym == 0 {
			continue
		}
		t.prototypeReceiverSym = m.PrototypeSym
		t.prototypeSelfSlot = m.SelfSlot
		t.prototypeSelfSymbol = sym
		break
	}
	if len(metas) > 0 {
		t.metatablePrototypeBySym = make(map[cfg.SymbolID]cfg.SymbolID, len(metas))
		for _, m := range metas {
			if m.MetatableSym != 0 && m.PrototypeSym != 0 {
				t.metatablePrototypeBySym[m.MetatableSym] = m.PrototypeSym
			}
		}
	}
	if len(sites) == 0 {
		t.setMetatableProtoByPoint = nil
		return
	}
	t.setMetatableProtoByPoint = make(map[cfg.Point]cfg.SymbolID, len(sites))
	for _, s := range sites {
		if s.Point != 0 && s.PrototypeSym != 0 {
			t.setMetatableProtoByPoint[s.Point] = s.PrototypeSym
		}
	}
}

// EvalExprValue types expr against the converged point state out, returning the
// AbstractValue the per-node transfer would produce for it and whether it resolves.
// It is the precise expression typing the intra-function transfer uses (nested table
// literals, calls, field reads), exposed for post-convergence readers. The
// parameter-demand callback is a no-op: a post-convergence read records no new
// contract.
func (t *Transfer) EvalExprValue(out *flow.PointState, expr ast.Expr) (product.AbstractValue, bool) {
	if out == nil || expr == nil {
		return product.AbstractValue{}, false
	}
	return t.evalExpr(out, expr, func(int, paramevidence.ParamContract) {})
}

// ProductCallContext returns the product-domain call context for a converged
// point state. It uses the same expression evaluator as the node transfer, but
// records no new parameter-demand edges because post-convergence readers must not
// mutate the solved equation.
func (t *Transfer) ProductCallContext(out *flow.PointState, call *ast.FuncCallExpr) ProductCallContext {
	return t.productCallContext(out, call, func(int, paramevidence.ParamContract) {})
}

// installPredicateGuards installs the local type-predicate facts (function symbol
// to tested parameter/kind) and the assigned predicate guards (ok symbol to the
// argument narrowing it proves). NarrowEdge reads them so a branch `if P(arg)` or
// `if ok` (with `local ok = P(arg)`) narrows the predicate argument to its tested
// kind on the true edge.
func (t *Transfer) installPredicateGuards(byFunc []guard.PredicateFunction, byCondSym []guard.PredicateResult) {
	t.predicateByFunc = nil
	for _, entry := range byFunc {
		if entry.FuncSym == 0 || entry.Kind == "" {
			continue
		}
		if t.predicateByFunc == nil {
			t.predicateByFunc = make(map[cfg.SymbolID]guard.PredicateFunction, len(byFunc))
		}
		t.predicateByFunc[entry.FuncSym] = entry
	}
	t.predicateByCondSym = nil
	for _, entry := range byCondSym {
		if entry.CondSym == 0 || entry.Kind == "" || entry.NarrowSym == 0 {
			continue
		}
		if t.predicateByCondSym == nil {
			t.predicateByCondSym = make(map[cfg.SymbolID]guard.PredicateResult, len(byCondSym))
		}
		t.predicateByCondSym[entry.CondSym] = entry
	}
}

// declaredParamBySlot re-keys the source-indexed DeclaredParamTypes into parameter
// SLOT order via the graph's ParamSlots, so an implicit method receiver `self` at
// slot 0 (SourceIndex -1, no annotation) does not shift every subsequent parameter's
// declared type by one. A slot whose SourceIndex addresses a resolved declared type
// carries that type; the implicit self slot carries none (its type is seeded
// separately as the receiver class). Without a graph (the standalone path) the source
// and slot indices coincide, so the original map passes through unchanged.
func declaredParamBySlot(in input.Inputs) map[int]typ.Type {
	src := in.Scope.DeclaredParamTypes
	if in.Graph == nil {
		return src
	}
	slots := in.Graph.ParamSlotsReadOnly()
	if len(slots) == 0 {
		return src
	}
	out := make(map[int]typ.Type, len(src))
	for i, slot := range slots {
		srcIdx, ok := slot.SourceParamIndex()
		if !ok {
			continue
		}
		if t, ok := src[srcIdx]; ok && t != nil {
			out[i] = t
		}
	}
	return out
}

// Transfer implements equation.NodeTransfer. It computes the state holding after
// node p from the joined predecessor state, folding in the assumed entry
// contracts at the entry point and emitting parameter demand for body uses.
func (t *Transfer) Transfer(
	g *cfg.Graph,
	p cfg.Point,
	incoming flow.PointState,
	entryContracts paramevidence.Contracts,
	demand func(param int, c paramevidence.ParamContract),
) flow.PointState {
	// Kildall transfer functions must be strict: f(⊥)=⊥. The entry point is the
	// sole exception because its incoming CFG join is initially Bottom and the
	// entry transfer establishes reachability plus parameter seeds. Every other
	// point reached at the actual lattice Bottom is unreachable; interpreting its
	// syntax would fabricate value facts and parameter demands from dead code.
	if g != nil && p != g.Entry() && flow.PointStateDomain.Equal(incoming, flow.PointStateDomain.Bottom()) {
		return flow.PointStateDomain.Bottom()
	}
	out := flow.ClonePointState(incoming)

	// At the entry point the assumed contracts ARE the parameter values a caller
	// supplies: seed each parameter's Env slot from its declared type joined with
	// the demanded contract, so body reads see the entry value.
	//
	// The entry also establishes reachability for the relational numeric
	// component. The solver seeds every cell at the lattice Bottom, whose numeric
	// state is the UNSAT element (the unreachable point); the entry has no
	// predecessors, so its incoming numeric is that unsat Bottom. The entry is
	// reachable with no numeric constraints, so it lifts the component to the
	// satisfiable empty state — the initial reachable numeric environment that
	// the forward join propagates and the per-node numeric transfer refines.
	if p == g.Entry() {
		flow.LiftEntryReachability(&out)
		t.seedEntry(&out, entryContracts)
	}
	t.applyLoopAppendLengthFacts(&out, t.loopAppendLengthsByPoint[p])

	switch info := g.Info(p).(type) {
	case *cfg.AssignInfo:
		t.applyAssign(&out, p, info, demand)
	case *cfg.BranchInfo:
		t.applyBranch(&out, info, demand)
	case *cfg.ReturnInfo:
		t.applyReturn(&out, p, info, demand)
	case *cfg.CallInfo:
		t.applySetMetatablePrototypeSelf(&out, p, info.Call, demand)
		if dead := t.applyCallArgs(&out, p, info, demand); dead {
			// An assert whose condition cannot hold proves the continuation
			// unreachable; the post-state is the lattice Bottom (UNSAT), so the
			// successors' in-states drop out as unreachable, exactly as error()
			// terminates the live flow.
			return flow.PointStateDomain.Bottom()
		}
	case *cfg.FuncDefInfo:
		t.applyFuncDef(&out, info)
	default:
		// Deferred node kinds carry the state forward unchanged.
	}

	return out
}

// seedEntry writes each parameter's composed entry value through EntrySeedEffect:
// declared annotation, exact caller entry value, and body-demand contract all
// meet at the same product-state location.
func (t *Transfer) seedEntry(out *flow.PointState, contracts paramevidence.Contracts) {
	declaredParamBySlot := t.closedDeclaredParamBySlot(out)
	for i, sym := range t.in.Scope.ParamSymbols {
		if sym == 0 {
			continue
		}
		var effect EntrySeedEffect
		effect.Symbol = sym
		if declared, ok := declaredParamBySlot[i]; ok && declared != nil {
			effect.Declared = declared
		} else if existing, ok := t.symbolValue(out, sym); ok && !existing.IsZero() {
			// Entry values supplied by the equation graph are already product-domain
			// facts. Keep them in the same Env/Cells location and let contracts join
			// below, instead of reading a transfer-local side channel.
			effect.Entry = existing
		}
		if effect.Declared != nil {
			if existing, ok := t.symbolValue(out, sym); ok && !existing.IsZero() {
				effect.Entry = existing
			}
		}
		if c, ok := contracts[i]; ok {
			effect.Contract = c
		}
		t.applyEntrySeedEffect(out, effect)
	}
}

// SeedEntryValues writes product-domain entry values for parameters into the
// same storage the ordinary transfer uses. The equation graph calls this before
// seedEntry; seedEntry then composes them with declared annotations and inferred
// body contracts through EntrySeedEffect.
func (t *Transfer) SeedEntryValues(out *flow.PointState, values map[int]product.AbstractValue) {
	if out == nil || len(values) == 0 {
		return
	}
	slots := make([]int, 0, len(values))
	for slot := range values {
		slots = append(slots, slot)
	}
	sort.Ints(slots)
	for _, slot := range slots {
		if slot < 0 || slot >= len(t.in.Scope.ParamSymbols) {
			continue
		}
		sym := t.in.Scope.ParamSymbols[slot]
		av := values[slot]
		if sym == 0 || av.IsZero() {
			continue
		}
		t.setSymbolValue(out, sym, av, false)
	}
}

// SeedEntryFacts writes parameter-relative path facts into the same PointState
// fact lanes the ordinary transfer uses. It is called by the equation graph at
// the entry point before local node semantics run, so subsequent writes and
// branch flow own fact lifetime through the usual reducers.
func (t *Transfer) SeedEntryFacts(out *flow.PointState, facts flow.BoundaryFacts) {
	if t == nil || out == nil || !facts.HasProof() {
		return
	}
	for _, fact := range facts.IndexWrites() {
		table, ok := t.rebaseEntryBoundaryPath(fact.Table)
		if !ok {
			continue
		}
		key, ok := t.rebaseEntryBoundaryPath(fact.Key)
		if !ok {
			continue
		}
		flow.ApplyMapWritePathProof(out, flow.MapWritePathProof{
			TablePath:              table,
			KeyPath:                key,
			KeyValue:               product.FromType(typ.Unknown),
			Value:                  fact.Value,
			AllowOpaqueKeyReadback: true,
		})
	}
	for _, fact := range facts.KeyPresence() {
		table, ok := t.rebaseEntryBoundaryPath(fact.Table)
		if !ok {
			continue
		}
		key, ok := t.rebaseEntryBoundaryPath(fact.Key)
		if !ok {
			continue
		}
		t.applyKeyProvenancePathProof(out, flow.KeyProvenancePathProof{
			Kind:      flow.KeyProvenanceDynamicIndexWrite,
			TablePath: table,
			KeyPath:   key,
		})
	}
	for _, fact := range facts.KeyArrays() {
		array, ok := t.rebaseEntryBoundaryPath(fact.Array)
		if !ok {
			continue
		}
		table, ok := t.rebaseEntryBoundaryPath(fact.Table)
		if !ok {
			continue
		}
		t.applyKeyProvenancePathProof(out, flow.KeyProvenancePathProof{
			Kind:      flow.KeyProvenanceKeyArrayAssignment,
			ArrayPath: array,
			TablePath: table,
		})
	}
	for _, fact := range facts.KeyArrayValues() {
		array, ok := t.rebaseEntryBoundaryPath(fact.Array)
		if !ok {
			continue
		}
		table, ok := t.rebaseEntryBoundaryPath(fact.Table)
		if !ok {
			continue
		}
		flow.ApplyKeyArrayValuePathProof(out, flow.KeyArrayValuePathProof{
			ArrayPath: array,
			TablePath: table,
			Value:     fact.Value,
		})
	}
	for _, fact := range facts.AppendKeys() {
		array, ok := t.rebaseEntryBoundaryPath(fact.Array)
		if !ok {
			continue
		}
		key, ok := t.rebaseEntryBoundaryPath(fact.Key)
		if !ok {
			continue
		}
		flow.ApplyAppendKeyPathProof(out, flow.AppendKeyPathProof{
			ArrayPath: array,
			KeyPath:   key,
		})
	}
	for _, fact := range facts.AppendElementFieldOrigins() {
		array, ok := t.rebaseEntryBoundaryPath(fact.Array)
		if !ok {
			continue
		}
		source, ok := t.rebaseEntryBoundaryPath(fact.Source)
		if !ok {
			continue
		}
		flow.ApplyAppendElementFieldOriginPathProof(out, flow.AppendElementFieldOriginPathProof{
			ArrayPath:   array,
			Field:       fact.Field,
			SourcePath:  source,
			SourceField: fact.SourceField,
		})
	}
	var ops []flow.NumericOp
	for _, fact := range facts.LengthLowerBounds() {
		target, ok := t.rebaseEntryBoundaryPath(fact.Target)
		if !ok || target.Symbol == 0 {
			continue
		}
		if op, ok := flow.NumericLenGeConstPathOp(target, fact.Lower); ok {
			ops = append(ops, op)
		}
	}
	if len(ops) > 0 {
		flow.ApplyNumericEffect(out, flow.NumericEffect{Ops: ops})
	}
}

func (t *Transfer) rebaseEntryBoundaryPath(path flow.BoundaryPath) (constraint.Path, bool) {
	if t == nil || path.Kind != flow.BoundaryPathParam || path.Index < 0 || path.Index >= len(t.in.Scope.ParamSymbols) {
		return constraint.Path{}, false
	}
	sym := t.in.Scope.ParamSymbols[path.Index]
	if sym == 0 {
		return constraint.Path{}, false
	}
	return constraint.Path{
		Symbol:   sym,
		Segments: append([]constraint.Segment(nil), path.Segments...),
	}, true
}

// SeedEntrySymbolValues writes immutable entry values keyed directly by graph
// symbol into the same product-state store ordinary transfer reads. The equation
// graph calls this only at the entry point; local node semantics and widening
// then own the fact like any other Env/Cells value.
func (t *Transfer) SeedEntrySymbolValues(out *flow.PointState, values map[cfg.SymbolID]product.AbstractValue) {
	if out == nil || len(values) == 0 {
		return
	}
	syms := make([]cfg.SymbolID, 0, len(values))
	for sym := range values {
		syms = append(syms, sym)
	}
	sort.Slice(syms, func(i, j int) bool { return syms[i] < syms[j] })
	for _, sym := range syms {
		av := values[sym]
		if sym == 0 || av.IsZero() {
			continue
		}
		t.setSymbolValue(out, sym, av, false)
	}
}

// applyAssign writes each target's source value into Env. It reads the source
// expression's value from the incoming Env (for identifiers), the literal domain
// (for literals/table constructors), or the operator resolver (for arithmetic).
// Identifier sources that are parameters emit demand: the assignment observes
// the parameter as the assigned value's type.
func (t *Transfer) applyAssign(
	out *flow.PointState,
	p cfg.Point,
	info *cfg.AssignInfo,
	demand func(int, paramevidence.ParamContract),
) {
	if info.NumericFor != nil {
		t.applyNumericFor(out, info)
		return
	}
	if len(info.IterExprs) > 0 {
		t.applyGenericFor(out, info, demand)
		return
	}
	// A type-cast call source (`local v = T(arg)`) asserts its argument IS T on the
	// continuation, exactly as a statement cast does. Narrow the argument before the
	// targets are bound, so a later read of the argument sees the asserted type. The
	// cast's RESULT type (T) is bound to the target through the ordinary call typing.
	for _, src := range info.Sources {
		if srcCall, ok := src.(*ast.FuncCallExpr); ok {
			t.applyTypeCastNarrow(out, p, srcCall, demand)
		}
	}
	callPostconditions := t.buildAssignCallPostconditions(out, info, demand)
	// A call source feeding more targets than sources expands to a multi-return
	// tuple: bind each target to the matching return slot (target i -> return i),
	// the Lua multi-assignment semantics (`local a, b = f()`). Resolved once here so
	// the per-target loop below reads the pre-typed return vector rather than
	// re-typing the call for every fed target.
	callReturns := t.callExpansionReturns(out, info, demand)
	info.EachTargetSource(func(i int, target cfg.AssignTarget, src ast.Expr) {
		if target.Kind == cfg.TargetField || target.Kind == cfg.TargetIndex {
			t.applyContainerWriteForAssign(out, target, src, info, i, demand)
			return
		}
		if target.Kind != cfg.TargetIdent || target.Symbol == 0 {
			return
		}
		keyArrayTable, _ := t.keyArrayTableForAssignment(info, i, target)
		references := t.referenceWritesForAssignedPlace(out, Place{Root: target.Symbol}, info, i, src, demand)
		applyAssignmentProvenance := func(value product.AbstractValue) {
			if provenance, ok := t.assignmentProvenanceEffectWithSourceSymbol(target, src, assignSourceSymbol(info, i), value); ok {
				t.applyAssignmentProvenanceEffect(out, provenance)
			}
		}
		val, ok := t.targetValue(out, p, info, i, src, callReturns, demand)
		constructorCardinalityLower := int64(0)
		if tbl, isTable := src.(*ast.TableExpr); isTable {
			constructorCardinalityLower = t.tableLiteralCardinalityLowerBound(out, tbl)
		}
		if !ok {
			if src != nil && t.pendingUnannotatedParamSource(out, src) {
				t.applySymbolWriteEffect(out, target, product.AbstractValue{}, false, false, src, keyArrayTable, references)
				applyAssignmentProvenance(product.AbstractValue{})
				return
			}
			if t.symbolStorage.isCellBacked(target.Symbol) && src != nil {
				t.applySymbolWriteEffect(out, target, product.Domain.Top(), false, false, nil, keyArrayTable, references)
				applyAssignmentProvenance(product.Domain.Top())
				return
			}
			if src == nil {
				// A target with no aligned source and no expanding call return: a
				// parameter declaration node, or a multi-value tail slot whose producer
				// is deferred. The slot's value is established elsewhere (entry seeding
				// for a parameter), so leave it untouched rather than clobber it.
				return
			}
			// A local declared `any` carries strict dynamic top regardless of the
			// initializer: the annotation is an opt-in to the strict dynamic contract,
			// so the slot stays `any` (a placeholder) even when the source is
			// unresolved. A field read off it is then a read off a placeholder, which
			// the gradual-admission gate does not silently admit.
			if t.declaredGradualTop(target.Symbol) {
				anyValue := product.FromType(typ.Any)
				t.applySymbolWriteEffect(out, target, anyValue, false, false, nil, keyArrayTable, references)
				applyAssignmentProvenance(anyValue)
				return
			}
			// A declared keyed/indexed local (`local m: {[string]: string} = f()`) is
			// statically that container regardless of an unresolved initializer: the
			// annotation is the slot's authority (resolve.go's hierarchy: declared
			// overrides structural inference). Seed it so a later read/iteration of the
			// slot sees the declared element/key/value rather than collapsing to unknown.
			if dc, has := t.declaredContainerType(target.Symbol); has {
				declaredValue := product.FromType(dc)
				t.applySymbolWriteEffect(out, target, declaredValue, false, false, nil, keyArrayTable, references)
				applyAssignmentProvenance(declaredValue)
				return
			}
			// Unknown source: clear any stale narrowing so the slot is the value
			// domain's Top (the most general value), never a stale precise type.
			t.applySymbolWriteEffect(out, target, product.AbstractValue{}, true, false, nil, keyArrayTable, references)
			applyAssignmentProvenance(product.AbstractValue{})
			return
		}
		// A local declared `any` keeps strict dynamic top rather than the initializer's
		// precise shape: `local raw: any = {id = "x"}` is the dynamic `any`, not the
		// record literal `{id: "x"}`. The annotation is the slot authority (the same
		// declared-over-inferred hierarchy declaredContainerType applies), so a later
		// field read `raw.id` is a read off a placeholder the gradual-admission gate
		// must flag against a concrete target, not the literal's precise field type.
		if t.declaredGradualTop(target.Symbol) {
			val = product.FromType(typ.Any)
		} else if dc, has := t.declaredContainerType(target.Symbol); has && declaredContainerOverridesKnownValue(dc) {
			// A declared keyed/indexed local's slot carries its declared container type
			// rather than the initializer's narrower value: `local m: {[string]: string}
			// = {}` is a string-keyed map (so `pairs(m)` types its key/value), not the empty
			// closed record the `{}` constructor yields. The annotation is the authority for
			// a mutable container slot when it carries real key/element information. Broad
			// soft containers such as `any[]` are contracts over future writes, not hard
			// element facts: preserving the empty initializer lets later mutator effects
			// refine the element from observed inserts instead of joining forever with any.
			val = product.FromType(dc)
		} else if dt, has := t.declaredObjectInitializerType(info, i, target.Symbol); has {
			// A local object/class annotation is the slot contract for constructor
			// self values. Keep the declared object identity after the initializer has
			// been checked so `local self: Store = {...}` returns Store rather than a
			// union of Store and the transient literal shape. Reassignments and
			// declared unions still flow from the current value so narrowing retains
			// discriminant precision.
			val = product.FromType(dt)
		}
		if withOrigin, ok := t.attachVariantOriginToAssignedValue(p, target, val); ok {
			val = withOrigin
		}
		// Assignment transfer is a strong update to the target location. Loop and
		// branch joins belong to the equation/fixpoint layer; weak-updating here
		// pollutes ordinary branch precision (for example an `any` local overwritten
		// by a string inside the true edge stays `any` forever).
		t.applySymbolWriteEffect(out, target, val, false, false, src, keyArrayTable, references)
		applyAssignmentProvenance(val)
		if provenance, ok := t.arrayElementKeyProvenanceEffect(target, src, val); ok {
			t.applyArrayElementKeyProvenanceEffect(out, provenance)
		}
		if src != nil {
			t.applyNumeric(out, target.Symbol, src)
			t.seedArrayLiteralLength(out, target.Symbol, src, constructorCardinalityLower)
		}
	})
	t.applyAssignCallPostconditions(out, callPostconditions)
}

// declaredGradualTop reports whether sym is declared with the strict dynamic top `any`,
// the opt-in dynamic contract a local `local raw: any = ...` carries. Such a slot
// stays `any` regardless of its initializer: the declared annotation is the slot
// authority (the same declared-over-inferred hierarchy declaredContainerType
// follows), so a record-literal initializer never refines the slot to a precise
// shape that would let a later field read off it pass a concrete-type assignment.
// Only `any` qualifies; `unknown` (the opaque top) is not erased here — it carries
// its own strict contract the value domain already enforces.
func (t *Transfer) declaredGradualTop(sym cfg.SymbolID) bool {
	declared, ok := t.declaredTypes[sym]
	if !ok || declared == nil {
		return false
	}
	return typ.IsAny(declared)
}

// declaredContainerType returns the declared type of sym when that annotation is a
// keyed/indexed container — a Map (`{[K]: V}`) or an Array (`{T}`). A local declared
// as such a container is statically that container: the slot's element/key/value
// relation is the annotation, not the structure of whatever initializer the
// constructor produced (an empty `{}` is a closed record that loses the map/array
// shape, and a partial literal is a subtype that under-counts the declared keys). The
// resolution hierarchy (declared annotation over structural inference) makes the
// annotation the authority, so the slot carries it; a non-container annotation (a
// union, a record, a scalar) returns false so the ordinary value-domain flow runs and
// per-edge discriminant narrowing over the declared union is undisturbed.
func (t *Transfer) declaredContainerType(sym cfg.SymbolID) (typ.Type, bool) {
	declared, ok := t.declaredTypes[sym]
	if !ok || declared == nil || typ.IsAbsentOrUnknown(declared) {
		return nil, false
	}
	// A nilable container annotation (`{T}?`) stays under per-edge nil narrowing: a
	// guard refines the optional to its non-nil container, so re-seeding the declared
	// optional here would clobber that narrowing and re-introduce nil (a later
	// index/method read then reports a spurious optional-access error). Only a
	// non-optional container is the unconditional slot authority.
	if _, optional := typ.SplitNilableFieldType(declared); optional {
		return nil, false
	}
	switch unwrap.Underlying(declared).(type) {
	case *typ.Map, *typ.ReadonlyMap, *typ.Array:
		return declared, true
	}
	return nil, false
}

func declaredContainerOverridesKnownValue(declared typ.Type) bool {
	return declared != nil && !typ.IsSoft(declared, typ.SoftPlaceholderPolicy)
}

func (t *Transfer) declaredObjectInitializerType(info *cfg.AssignInfo, targetIndex int, sym cfg.SymbolID) (typ.Type, bool) {
	if info == nil || !info.IsLocal || info.TypeAnnotationAt(targetIndex) == nil || sym == 0 {
		return nil, false
	}
	declared, ok := t.declaredTypes[sym]
	if !ok || declared == nil || typ.IsAbsentOrUnknown(declared) || typ.IsAny(declared) {
		return nil, false
	}
	if _, optional := typ.SplitNilableFieldType(declared); optional {
		return nil, false
	}
	switch unwrap.Underlying(declared).(type) {
	case *typ.Record, *typ.Recursive, *typ.Interface:
		return declared, true
	default:
		return nil, false
	}
}

// callExpansionReturns types the return vector of an assignment's single source
// call that expands across multiple targets (`local a, b = f()`), so each target
// binds to the matching return slot. It returns nil when the assignment is not a
// multi-target call expansion (the ordinary per-source typing then applies). The
// aligned single-target call (`local x = f()`) is typed through the per-target
// source path, not here.
func (t *Transfer) callExpansionReturns(
	out *flow.PointState,
	info *cfg.AssignInfo,
	demand func(int, paramevidence.ParamContract),
) []product.AbstractValue {
	call, first := info.ExpandingSourceCall()
	if call == nil || call.Call == nil || first != len(info.Sources)-1 {
		return nil
	}
	returns, ok := t.evalCall(out, call.Call, demand)
	if !ok {
		return nil
	}
	return returns
}

func (t *Transfer) attachVariantOriginToAssignedValue(p cfg.Point, target cfg.AssignTarget, val product.AbstractValue) (product.AbstractValue, bool) {
	if val.IsZero() || len(t.in.VariantFieldOrigins) == 0 {
		return product.AbstractValue{}, false
	}
	targetPath, ok := t.staticPathOfAssignTarget(target)
	if !ok || targetPath.IsEmpty() {
		return product.AbstractValue{}, false
	}
	targetPath = domainpath.WithVersion(targetPath, t.in.Graph, p)
	var family uint64
	var cases []int
	for _, origin := range t.in.VariantFieldOrigins {
		if origin.OriginFamily == 0 || !origin.Target.Equal(targetPath) {
			continue
		}
		if family == 0 {
			family = origin.OriginFamily
		}
		if family != origin.OriginFamily {
			return product.AbstractValue{}, false
		}
		cases = append(cases, origin.CaseIndex)
	}
	if family == 0 || len(cases) == 0 {
		return product.AbstractValue{}, false
	}
	return product.WithVariantOrigin(val, family, cases), true
}

func (t *Transfer) callReturnRelations(
	out *flow.PointState,
	call *ast.FuncCallExpr,
	demand func(int, paramevidence.ParamContract),
) flow.ReturnRelations {
	if call == nil {
		return flow.ReturnRelationsDomain.Top()
	}
	return t.productCallResult(call, t.productCallContext(out, call, demand)).ReturnRelations
}

// targetValue resolves the value bound to target index i. A target aligned with a
// source expression is the source's value. A trailing target fed by an expanding
// call (`local a, b = f()`, where b has no aligned source) is the call's return at
// the matching slot.
func (t *Transfer) targetValue(
	out *flow.PointState,
	p cfg.Point,
	info *cfg.AssignInfo,
	i int,
	src ast.Expr,
	callReturns []product.AbstractValue,
	demand func(int, paramevidence.ParamContract),
) (product.AbstractValue, bool) {
	// A target fed by the expanding call (at or past the call's source index) reads
	// the pre-typed return vector: slot i maps to return (i - first), binding target
	// i to return i per the Lua multi-assignment rule. Targets before the expanding
	// call keep their own aligned source.
	if callReturns != nil {
		if _, first := info.ExpandingSourceCall(); i >= first {
			idx := i - first
			if idx < 0 {
				return product.AbstractValue{}, false
			}
			// A target past the expanding call's return arity binds to nil: Lua fills a
			// multi-assignment slot with no corresponding return value with nil (`local
			// k, v = f()` where f returns one value makes v nil). The call resolved (the
			// vector is present), so the absent slot is provably nil, not unknown.
			if idx >= len(callReturns) {
				return product.FromType(typ.Nil), true
			}
			av := callReturns[idx]
			if av.IsZero() {
				// A slot the pipeline could not type (within arity) is unknown, not
				// provably nil; leave it to drop to the value-domain Top.
				return product.AbstractValue{}, false
			}
			return av, true
		}
	}
	if src != nil {
		return t.evalExprAt(out, p, src, demand)
	}
	return product.AbstractValue{}, false
}

func assignSourceSymbol(info *cfg.AssignInfo, i int) cfg.SymbolID {
	if info == nil || i < 0 || i >= len(info.SourceSymbols) {
		return 0
	}
	return info.SourceSymbols[i]
}

// pendingUnannotatedParamSource reports whether src failed to resolve because it
// is rooted in an unannotated parameter whose entry value is still the product
// Bottom. During the interprocedural summary fixed point, caller-projected
// EntryValues arrive monotonically after the first bottom iteration; treating
// that temporary absence as the value-domain Top would permanently pollute
// captured-cell effects and prevent the later precise entry value from flowing.
//
// True dynamic values are admitted at explicit boundaries (declared `any`,
// unknown call results, external seeds). A local unannotated parameter with no
// entry value yet is not such a boundary; it is pending data in the same product
// fixed point.
func (t *Transfer) pendingUnannotatedParamSource(out *flow.PointState, src ast.Expr) bool {
	return t.exprUsesPendingUnannotatedParam(out, src)
}

func (t *Transfer) sourceRootSymbol(src ast.Expr) cfg.SymbolID {
	switch e := src.(type) {
	case *ast.IdentExpr:
		return t.symbolOf(e)
	case *ast.AttrGetExpr:
		return t.sourceRootSymbol(e.Object)
	case *ast.FuncCallExpr:
		if e.Method != "" {
			return t.sourceRootSymbol(e.Receiver)
		}
		return t.sourceRootSymbol(e.Func)
	default:
		return 0
	}
}

func (t *Transfer) exprUsesPendingUnannotatedParam(out *flow.PointState, expr ast.Expr) bool {
	switch e := expr.(type) {
	case nil:
		return false
	case *ast.IdentExpr:
		sym := t.symbolOf(e)
		if sym == 0 || !t.unannotatedParam[sym] {
			return false
		}
		_, ok := t.symbolValue(out, sym)
		return !ok
	case *ast.AttrGetExpr:
		return t.exprUsesPendingUnannotatedParam(out, e.Object) || t.exprUsesPendingUnannotatedParam(out, e.Key)
	case *ast.FuncCallExpr:
		if t.exprUsesPendingUnannotatedParam(out, e.Func) || t.exprUsesPendingUnannotatedParam(out, e.Receiver) {
			return true
		}
		for _, arg := range e.Args {
			if t.exprUsesPendingUnannotatedParam(out, arg) {
				return true
			}
		}
		return false
	case *ast.CastExpr:
		return t.exprUsesPendingUnannotatedParam(out, e.Expr)
	case *ast.ArithmeticOpExpr:
		return t.exprUsesPendingUnannotatedParam(out, e.Lhs) || t.exprUsesPendingUnannotatedParam(out, e.Rhs)
	case *ast.StringConcatOpExpr:
		return t.exprUsesPendingUnannotatedParam(out, e.Lhs) || t.exprUsesPendingUnannotatedParam(out, e.Rhs)
	case *ast.LogicalOpExpr:
		return t.exprUsesPendingUnannotatedParam(out, e.Lhs) || t.exprUsesPendingUnannotatedParam(out, e.Rhs)
	case *ast.RelationalOpExpr:
		return t.exprUsesPendingUnannotatedParam(out, e.Lhs) || t.exprUsesPendingUnannotatedParam(out, e.Rhs)
	case *ast.UnaryLenOpExpr:
		return t.exprUsesPendingUnannotatedParam(out, e.Expr)
	case *ast.UnaryMinusOpExpr:
		return t.exprUsesPendingUnannotatedParam(out, e.Expr)
	case *ast.UnaryNotOpExpr:
		return t.exprUsesPendingUnannotatedParam(out, e.Expr)
	default:
		return false
	}
}

// applyFuncDef types a function-definition assignment. A root definition
// (`function f()`) writes the function value and identity into the binding symbol;
// a container definition (`function M.add()`, `function M:add()`) writes the
// function value and identity into the receiver field. A definition whose function
// type the typer cannot resolve leaves the state untouched (sound carry-forward).
func (t *Transfer) applyFuncDef(out *flow.PointState, info *cfg.FuncDefInfo) {
	if out == nil || info == nil || info.TargetPath.Symbol == 0 {
		return
	}
	place, ok := canonicalplace.FromStaticPath(info.TargetPath)
	if !ok || place.Root == 0 {
		return
	}
	t.applyWriteEffect(out, WriteEffect{
		Place:        place,
		References:   sourceReferenceWrite(),
		RecordStatic: true,
	})
	if t.funcTyper == nil || info.FuncExpr == nil {
		return
	}
	base := info.TargetPath.Symbol
	segs := info.TargetPath.Segments
	var fn *typ.Function
	if info.IsMethod {
		// A method definition (function T:m()) stores the callable with the implicit
		// leading `self` typed as the receiver class T, so an instance field backed by
		// the method (`on = T.on`) sees `fun(self: T, ...)`.
		fn = t.funcTyper.MethodFuncType(info)
	} else {
		fn = t.funcTyper.FuncType(info.FuncExpr)
	}
	if fn == nil {
		return
	}
	if len(segs) == 0 {
		_, had := t.symbolValue(out, base)
		t.applyWriteEffect(out, WriteEffect{
			Place:         place,
			Value:         product.FromType(fn),
			JoinExisting:  had,
			Source:        info.FuncExpr,
			References:    t.referenceWriteForFuncDef(info),
			KillRelations: true,
			RecordStatic:  true,
		})
		return
	}
	for _, seg := range segs {
		if key, ok := value.MemberFromSegment(seg); !ok || key.Kind() == value.MemberKindIntIndex {
			// An integer-indexed function target is not a record field write.
			return
		}
	}
	t.applyWriteEffect(out, WriteEffect{
		Place:        place,
		Value:        product.FromType(fn),
		Source:       info.FuncExpr,
		References:   t.referenceWriteForFuncDef(info),
		RecordStatic: true,
	})
}

func (t *Transfer) referenceWriteForFuncDef(info *cfg.FuncDefInfo) referenceWrite {
	if provider, ok := t.funcTyper.(functionRefProvider); ok {
		if ref, ok := provider.MethodFuncRef(info); ok {
			return sourceReferenceWrite().WithFunctionTree(flow.FunctionRefTree{
				Root:    flow.FunctionRefSetOf(ref),
				HasRoot: true,
			})
		}
		if ref, ok := provider.FuncRef(info.FuncExpr); ok {
			return sourceReferenceWrite().WithFunctionTree(flow.FunctionRefTree{
				Root:    flow.FunctionRefSetOf(ref),
				HasRoot: true,
			})
		}
	}
	return sourceReferenceWrite()
}

// applyContainerWrite applies a field or index write (t.f = v, t.a.b = v, t[k] = v)
// to the base container's value in Env. It reads the assigned source value and the
// base container, applies the value-domain field overlay (product.WithField, the
// AbstractValue-native record field-write that extends the record with a fresh key
// the way Lua field assignment admits one) or the indexed write
// (product.WriteIndex, which widens the container to admit the key/value and
// convergence-merges), and writes the updated container back to the base symbol's
// Env slot. A write whose base or source does not resolve is left alone: the slot
// keeps its prior over-approximation rather than dropping to Top, which is sound
// (the field is at worst less precise).
func (t *Transfer) applyContainerWrite(
	out *flow.PointState,
	target cfg.AssignTarget,
	src ast.Expr,
	demand func(int, paramevidence.ParamContract),
) {
	t.applyContainerWriteWithRefs(out, target, src, demand, sourceReferenceWrite())
}

func (t *Transfer) applyContainerWriteForAssign(
	out *flow.PointState,
	target cfg.AssignTarget,
	src ast.Expr,
	info *cfg.AssignInfo,
	targetIndex int,
	demand func(int, paramevidence.ParamContract),
) {
	t.applyContainerWriteWithRefResolver(out, target, src, demand, func(place Place) referenceWrite {
		return t.referenceWritesForAssignedPlace(out, place, info, targetIndex, src, demand)
	})
}

func (t *Transfer) applyContainerWriteWithRefs(
	out *flow.PointState,
	target cfg.AssignTarget,
	src ast.Expr,
	demand func(int, paramevidence.ParamContract),
	references referenceWrite,
) {
	t.applyContainerWriteWithRefResolver(out, target, src, demand, func(Place) referenceWrite {
		return references
	})
}

func (t *Transfer) applyContainerWriteWithRefResolver(
	out *flow.PointState,
	target cfg.AssignTarget,
	src ast.Expr,
	demand func(int, paramevidence.ParamContract),
	refWrites func(Place) referenceWrite,
) {
	var base product.AbstractValue
	if target.BaseSymbol != 0 {
		var had bool
		base, had = t.symbolValue(out, target.BaseSymbol)
		if !had || base.IsZero() {
			// The base container has no value the transfer tracks: an imported module, a
			// captured variable, or a parameter whose type lives in the observation
			// surface, not the Env. The WriteEffect below may still invalidate side
			// products at the lowered Place, but assignPlaceValue will not fabricate a
			// closed root value for the untracked container.
			base = product.AbstractValue{}
		}
	}
	place, ok := t.placeOfAssignTarget(out, target, base, demand)
	if !ok || place.Root == 0 || len(place.Steps) == 0 {
		symbolicValue := t.symbolicDynamicIndexWriteValue(out, target, src, demand)
		// A write through an unresolved dynamic key may still have a tracked static
		// container prefix (`items.byName[k] = v`). Emit an invalidation-only effect
		// for that footprint; do not pretend we know the exact key or root value.
		if prefix, prefixOK := t.invalidationPlaceOfAssignTarget(target); prefixOK && prefix.Root != 0 {
			references := sourceReferenceWrite()
			if refWrites != nil {
				references = refWrites(prefix)
			}
			t.applyWriteEffect(out, WriteEffect{
				Place:        prefix,
				Source:       src,
				RecordProto:  true,
				References:   references,
				RecordStatic: true,
			})
		}
		t.applySymbolicDynamicIndexWriteProof(out, target, src, symbolicValue)
		// A write through a non-identifier base (e.g. f().x = v) has no root slot
		// to update; its container value lives nowhere this transfer tracks.
		return
	}
	references := sourceReferenceWrite()
	if refWrites != nil {
		references = refWrites(place)
	}
	val := product.AbstractValue{}
	if src != nil {
		expected := t.expectedContainerWriteValueType(place)
		if resolved, ok := t.evalExprWithExpected(out, src, expected, demand); ok && !resolved.IsZero() {
			val = resolved
		} else if !t.pendingUnannotatedParamSource(out, src) {
			val = product.Domain.Top()
		}
	}

	mode := DynamicWriteForeign
	if target.Kind == cfg.TargetIndex && target.BaseSymbol == place.Root && len(place.Steps) == 1 && t.writeIsSelfDerived(out, target, src) {
		mode = DynamicWriteSelfDerived
	}
	t.applyWriteEffect(out, WriteEffect{
		Place:        place,
		Value:        val,
		Source:       src,
		IndexTarget:  target,
		DynamicMode:  mode,
		LengthTarget: target,
		RecordProto:  true,
		References:   references,
		RecordStatic: true,
	})
	if provenance, ok := t.assignmentProvenanceEffect(target, src, val); ok {
		t.applyAssignmentProvenanceEffect(out, provenance)
	}
}

func (t *Transfer) symbolicDynamicIndexWriteValue(
	out *flow.PointState,
	target cfg.AssignTarget,
	src ast.Expr,
	demand func(int, paramevidence.ParamContract),
) product.AbstractValue {
	if target.Kind != cfg.TargetIndex || src == nil {
		return product.AbstractValue{}
	}
	value, ok := t.evalExpr(out, src, demand)
	if !ok || value.IsZero() {
		return product.AbstractValue{}
	}
	return value
}

func (t *Transfer) expectedContainerWriteValueType(place Place) typ.Type {
	targetPath, ok := place.FinalDynamicIndexTargetPath()
	if !ok || targetPath.IsEmpty() || len(place.Steps) == 0 {
		return nil
	}
	container, ok := t.declaredTypeForExactStaticPath(targetPath)
	if !ok || container == nil || typ.IsAbsentOrUnknown(container) || typ.IsRefinableAnnotation(container) {
		return nil
	}
	step := place.Steps[len(place.Steps)-1]
	if step.Kind != PlaceStepDynamicIndex || step.Key.IsZero() {
		return nil
	}
	expected := value.SealedIndexedWriteObligation(container, product.ProjectValueOrUnknown(step.Key))
	if expected == nil || typ.IsAbsentOrUnknown(expected) || typ.IsAny(expected) || typ.IsNever(expected) {
		return nil
	}
	return expected
}

func (t *Transfer) installStaticMemberWriteFactForPlace(out *flow.PointState, place Place, val product.AbstractValue) {
	if out == nil || val.IsZero() || !val.DefinitelyPresent() {
		return
	}
	path, ok := place.StaticPath()
	if !ok || path.Symbol == 0 || len(path.Segments) == 0 {
		return
	}
	t.installStaticMemberWriteFact(out, path.Symbol, path.Segments, val)
}

func (t *Transfer) invalidateIterationTargetWrites(out *flow.PointState, targets []cfg.AssignTarget) {
	for _, target := range targets {
		t.applyIterationTargetWrite(out, target, product.AbstractValue{}, false)
	}
}

func (t *Transfer) applyIterationTargetWrite(
	out *flow.PointState,
	target cfg.AssignTarget,
	val product.AbstractValue,
	joinExisting bool,
) bool {
	return t.applySymbolWriteEffect(
		out,
		target,
		val,
		false,
		joinExisting,
		nil,
		constraint.Path{},
		sourceReferenceWrite(),
	)
}

func (t *Transfer) installStaticMemberWriteFact(out *flow.PointState, sym cfg.SymbolID, segs []constraint.Segment, val product.AbstractValue) {
	if out == nil || sym == 0 || len(segs) == 0 || val.IsZero() || !val.DefinitelyPresent() {
		return
	}
	flow.SetStaticMemberSymbolPath(out, sym, segs, val)
}

func (t *Transfer) referenceWritesForAssignedPlace(
	out *flow.PointState,
	place Place,
	info *cfg.AssignInfo,
	targetIndex int,
	src ast.Expr,
	demand func(int, paramevidence.ParamContract),
) referenceWrite {
	if references, ok := t.callReturnReferenceWritesForAssignedPlace(out, place, info, targetIndex, src, demand); ok {
		return references
	}
	return sourceReferenceWrite()
}

func (t *Transfer) callReturnReferenceWritesForAssignedPlace(
	out *flow.PointState,
	place Place,
	info *cfg.AssignInfo,
	targetIndex int,
	src ast.Expr,
	demand func(int, paramevidence.ParamContract),
) (referenceWrite, bool) {
	if info != nil {
		if call, retIndex := info.CallForTarget(targetIndex); call != nil && call.Call != nil {
			return t.callReturnReferenceWritesForPlace(out, place, call.Call, retIndex, demand)
		}
	}
	if call, ok := src.(*ast.FuncCallExpr); ok {
		return t.callReturnReferenceWritesForPlace(out, place, call, 0, demand)
	}
	return referenceWrite{}, false
}

func (t *Transfer) callReturnReferenceWritesForPlace(
	out *flow.PointState,
	place Place,
	call *ast.FuncCallExpr,
	retIndex int,
	demand func(int, paramevidence.ParamContract),
) (referenceWrite, bool) {
	if out == nil || call == nil || retIndex < 0 {
		return referenceWrite{}, false
	}
	if path, ok := place.StaticPath(); !ok || path.IsEmpty() {
		return referenceWrite{}, false
	}
	returns := t.productCallResult(call, t.productCallContext(out, call, demand)).ReturnRefs
	writes := sourceReferenceWrite()
	got := false
	if tree, ok := returns.FunctionRefTree(retIndex); ok {
		writes = writes.WithFunctionTree(tree)
		got = true
	}
	if tree, ok := returns.ClosureRefTree(retIndex); ok {
		writes = writes.WithClosureTree(tree)
		got = true
	}
	return writes, got
}

func (t *Transfer) recordFunctionRefAt(out *flow.PointState, path constraint.Path, src ast.Expr) {
	if out == nil {
		return
	}
	if srcPath, ok := t.staticPathOfExpr(src); ok {
		flow.AssignFunctionRefSubtreePath(out, srcPath, path)
		return
	}
	tree := flow.FunctionRefTree{
		Entries: t.nestedFunctionRefSetsOfExpr(out, src),
	}
	set, ok := t.functionRefSetOfExpr(out, src)
	if ok {
		tree.Root = set
		tree.HasRoot = true
	}
	flow.ReplaceFunctionRefTreePath(out, path, tree)
}

func (t *Transfer) recordClosureRefAt(out *flow.PointState, path constraint.Path, src ast.Expr) {
	if out == nil {
		return
	}
	if srcPath, ok := t.staticPathOfExpr(src); ok {
		flow.AssignClosureRefSubtreePath(out, srcPath, path)
		return
	}
	tree := flow.ClosureRefTree{
		Entries: t.nestedClosureRefSetsOfExpr(out, src),
	}
	set, ok := t.closureRefSetOfExpr(out, src)
	if ok {
		tree.Root = set
		tree.HasRoot = true
	}
	flow.ReplaceClosureRefTreePath(out, path, tree)
}

func (t *Transfer) closureRefSetOfExpr(out *flow.PointState, expr ast.Expr) (flow.ClosureRefSet, bool) {
	if expr == nil || out == nil {
		return flow.ClosureRefSet{}, false
	}
	if fn, ok := expr.(*ast.FunctionExpr); ok && fn != nil {
		provider, ok := t.funcTyper.(functionRefProvider)
		if !ok || provider == nil {
			return flow.ClosureRefSet{}, false
		}
		ref, ok := provider.FuncRef(fn)
		if !ok {
			return flow.ClosureRefSet{}, false
		}
		captured := t.closureCapturedSymbols(ref)
		entryRefs := flow.ProjectFunctionRefsBySymbols(out.FunctionRefs, captured)
		entryClosures := flow.ProjectClosureRefsBySymbols(out.ClosureRefs, captured)
		entryCells := t.closureCaptureCells(out, captured)
		if projection, ok := t.closureReferenceProjection(ref); ok {
			entryRefs = flow.ProjectFunctionRefsByReferencePaths(out.FunctionRefs, projection)
			entryClosures = flow.ProjectClosureRefsByReferencePaths(out.ClosureRefs, projection)
			entryCells = entryCells.ProjectPaths(projection)
		}
		return flow.ClosureRefSetOf(flow.ClosureRefOf(
			ref,
			entryCells,
			entryRefs,
			entryClosures,
		)), true
	}
	path, ok := t.staticPathOfExpr(expr)
	if !ok {
		return flow.ClosureRefSet{}, false
	}
	return flow.ClosureRefAtPath(out.ClosureRefs, path)
}

func (t *Transfer) closureCapturedSymbols(ref flow.FunctionRef) []cfg.SymbolID {
	provider, ok := t.funcTyper.(closureCaptureProvider)
	if !ok || provider == nil {
		return nil
	}
	return provider.CapturedSymbols(ref)
}

func (t *Transfer) closureReferenceProjection(ref flow.FunctionRef) (flow.ReferencePathProjection, bool) {
	provider, ok := t.funcTyper.(closureReferenceProjectionProvider)
	if !ok || provider == nil {
		return flow.ReferencePathProjection{}, false
	}
	return provider.ReferenceProjection(ref), true
}

func (t *Transfer) closureCaptureCells(out *flow.PointState, captured []cfg.SymbolID) flow.CaptureCells {
	if out == nil || len(captured) == 0 {
		return flow.CaptureCellsDomain.Bottom()
	}
	cells := out.Cells.Project(captured)
	for _, sym := range captured {
		if sym == 0 {
			continue
		}
		base, has := t.symbolValue(out, sym)
		if existing, ok := cells.Value(sym); ok && !valueIsBottom(existing) {
			base = existing
			has = true
		}
		if refined, ok := t.conditionRefinedCaptureValue(out, sym, base, has); ok && !valueIsBottom(refined) {
			cells = cells.With(sym, refined)
			continue
		}
		if has && !valueIsBottom(base) {
			cells = cells.With(sym, base)
		}
	}
	return cells
}

func (t *Transfer) nestedClosureRefSetsOfExpr(out *flow.PointState, expr ast.Expr) []flow.ClosureRefTreeEntry {
	table, ok := expr.(*ast.TableExpr)
	if !ok || table == nil {
		return nil
	}
	var entries []flow.ClosureRefTreeEntry
	t.collectTableClosureRefs(out, table, nil, &entries)
	return entries
}

func (t *Transfer) collectTableClosureRefs(out *flow.PointState, table *ast.TableExpr, prefix []constraint.Segment, entries *[]flow.ClosureRefTreeEntry) {
	if table == nil {
		return
	}
	for _, field := range table.Fields {
		if field == nil || field.Key == nil || field.Value == nil {
			continue
		}
		seg, ok := pathseg.StaticTableFieldSegment(field)
		if !ok {
			continue
		}
		segments := append(append([]constraint.Segment(nil), prefix...), seg)
		switch v := field.Value.(type) {
		case *ast.TableExpr:
			t.collectTableClosureRefs(out, v, segments, entries)
		default:
			if set, ok := t.closureRefSetOfExpr(out, field.Value); ok {
				*entries = append(*entries, flow.ClosureRefTreeEntry{
					Segments: segments,
					Set:      set,
				})
			}
		}
	}
}

func (t *Transfer) functionRefSetOfExpr(out *flow.PointState, expr ast.Expr) (flow.FunctionRefSet, bool) {
	if expr == nil {
		return flow.FunctionRefSet{}, false
	}
	if fn, ok := expr.(*ast.FunctionExpr); ok && fn != nil {
		if provider, ok := t.funcTyper.(functionRefProvider); ok {
			if ref, ok := provider.FuncRef(fn); ok {
				return flow.FunctionRefSetOf(ref), true
			}
		}
		return flow.FunctionRefSet{}, false
	}
	path, ok := t.staticPathOfExpr(expr)
	if !ok {
		return flow.FunctionRefSet{}, false
	}
	return flow.FunctionRefAtPath(out.FunctionRefs, path)
}

func (t *Transfer) nestedFunctionRefSetsOfExpr(out *flow.PointState, expr ast.Expr) []flow.FunctionRefTreeEntry {
	table, ok := expr.(*ast.TableExpr)
	if !ok || table == nil {
		return nil
	}
	var entries []flow.FunctionRefTreeEntry
	t.collectTableFunctionRefs(out, table, nil, &entries)
	return entries
}

func (t *Transfer) collectTableFunctionRefs(out *flow.PointState, table *ast.TableExpr, prefix []constraint.Segment, entries *[]flow.FunctionRefTreeEntry) {
	if table == nil {
		return
	}
	for _, field := range table.Fields {
		if field == nil || field.Key == nil || field.Value == nil {
			continue
		}
		seg, ok := pathseg.StaticTableFieldSegment(field)
		if !ok {
			continue
		}
		segments := append(append([]constraint.Segment(nil), prefix...), seg)
		switch v := field.Value.(type) {
		case *ast.TableExpr:
			t.collectTableFunctionRefs(out, v, segments, entries)
		default:
			if set, ok := t.functionRefSetOfExpr(out, field.Value); ok {
				*entries = append(*entries, flow.FunctionRefTreeEntry{
					Segments: segments,
					Set:      set,
				})
			}
		}
	}
}

func (t *Transfer) staticMemberExprPathAt(out *flow.PointState, p cfg.Point, expr ast.Expr) (constraint.Path, bool) {
	if out == nil {
		return t.staticPathOfExpr(expr)
	}
	place, ok := t.placeOfExprAt(out, p, expr, nil)
	if !ok {
		return t.staticPathOfExpr(expr)
	}
	return place.StaticPath()
}

func fieldSegments(names []string) []constraint.Segment {
	if len(names) == 0 {
		return nil
	}
	segs := make([]constraint.Segment, 0, len(names))
	for _, name := range names {
		if name == "" {
			continue
		}
		segs = append(segs, constraint.Segment{Kind: constraint.SegmentField, Name: name})
	}
	return segs
}

func (t *Transfer) recordPrototypeSelfWrite(
	out *flow.PointState,
	sym cfg.SymbolID,
	updated product.AbstractValue,
	publishParamEffect bool,
	mutations ...flow.ReceiverMutation,
) bool {
	if out == nil || sym == 0 || updated.IsZero() {
		return false
	}
	changed := false
	if t.prototypeReceiverSym != 0 && t.prototypeSelfSymbol != 0 && sym == t.prototypeSelfSymbol &&
		flow.RecordPrototypeSelf(out, t.prototypeReceiverSym, updated) {
		changed = true
	}
	if publishParamEffect {
		if slot, ok := t.paramBySym[sym]; ok {
			if flow.RecordReceiverWrite(out, slot, updated, mutations...) {
				changed = true
			}
		} else if t.prototypeSelfSymbol != 0 && sym == t.prototypeSelfSymbol && t.prototypeSelfSlot >= 0 &&
			flow.RecordReceiverWrite(out, t.prototypeSelfSlot, updated, mutations...) {
			changed = true
		}
	}
	return changed
}

func (t *Transfer) applySetMetatableInstanceBinding(out *flow.PointState, src ast.Expr, sym cfg.SymbolID) bool {
	if out == nil || sym == 0 {
		return false
	}
	proto, ok := t.setMetatablePrototypeFromSource(src)
	if !ok || proto == 0 {
		return false
	}
	changed := flow.BindPrototypeInstance(out, sym, proto)
	if t.publishPrototypeMethodRefs(out, proto, constraint.NewPath(sym, "")) {
		changed = true
	}
	return changed
}

func (t *Transfer) publishPrototypeMethodRefs(out *flow.PointState, proto cfg.SymbolID, base constraint.Path) bool {
	if out == nil || proto == 0 || base.IsEmpty() || len(t.prototypeMethods) == 0 {
		return false
	}
	tree, ok := t.prototypeMethodRefTree(proto)
	if !ok {
		return false
	}
	return flow.JoinFunctionRefTreePath(out, base, tree)
}

func (t *Transfer) prototypeMethodRefTree(proto cfg.SymbolID) (flow.FunctionRefTree, bool) {
	if proto == 0 || len(t.prototypeMethods) == 0 {
		return flow.FunctionRefTree{}, false
	}
	bySegment := make(map[constraint.Segment]flow.FunctionRefSet)
	for _, method := range t.prototypeMethods {
		if method.PrototypeSym != proto || method.FuncRef == (flow.FunctionRef{}) || method.Field == (constraint.Segment{}) {
			continue
		}
		set := flow.FunctionRefSetOf(method.FuncRef)
		if existing, ok := bySegment[method.Field]; ok {
			set = flow.FunctionRefSetDomain.Join(existing, set)
		}
		bySegment[method.Field] = set
	}
	if len(bySegment) == 0 {
		return flow.FunctionRefTree{}, false
	}
	entries := make([]flow.FunctionRefTreeEntry, 0, len(bySegment))
	for segment, set := range bySegment {
		entries = append(entries, flow.FunctionRefTreeEntry{
			Segments: []constraint.Segment{segment},
			Set:      set,
		})
	}
	return flow.FunctionRefTree{Entries: entries}, true
}

func (t *Transfer) setMetatablePrototypeFromSource(src ast.Expr) (cfg.SymbolID, bool) {
	call, ok := src.(*ast.FuncCallExpr)
	if !ok || call == nil || t.in.Graph == nil || !metatable.IsSetMetatableCall(call, t.in.Graph.Bindings()) {
		return 0, false
	}
	return t.setMetatablePrototype(0, call)
}

func staticMemberKey(attr *ast.AttrGetExpr) (value.MemberKey, bool) {
	seg, ok := pathseg.StaticAttrSegment(attr)
	if !ok {
		return value.MemberKey{}, false
	}
	return value.MemberFromSegment(seg)
}

func staticMemberKeyWithConst(attr *ast.AttrGetExpr, constResolver func(string) *flow.ConstValue) (value.MemberKey, bool) {
	if constResolver == nil {
		return staticMemberKey(attr)
	}
	seg, ok := domainpath.StaticAttrSegmentWithConst(attr, constResolver)
	if !ok {
		return value.MemberKey{}, false
	}
	return value.MemberFromSegment(seg)
}

func staticAttrFieldName(attr *ast.AttrGetExpr) (string, bool) {
	seg, ok := pathseg.StaticAttrSegment(attr)
	if !ok || seg.Kind != constraint.SegmentField || seg.Name == "" {
		return "", false
	}
	return seg.Name, true
}

func staticIndexMemberKey(key ast.Expr) (value.MemberKey, bool) {
	seg, ok := staticIndexSegment(key)
	if !ok {
		return value.MemberKey{}, false
	}
	return value.MemberFromSegment(seg)
}

func staticIndexSegment(key ast.Expr) (constraint.Segment, bool) {
	switch k := key.(type) {
	case *ast.StringExpr:
		return constraint.Segment{Kind: constraint.SegmentIndexString, Name: k.Value}, true
	case *ast.NumberExpr:
		idx, ok := pathkey.ParseIntLiteral(k.Value)
		if !ok {
			return constraint.Segment{}, false
		}
		return constraint.Segment{Kind: constraint.SegmentIndexInt, Index: idx}, true
	default:
		return constraint.Segment{}, false
	}
}

func staticIndexSegmentFromValue(av product.AbstractValue) (constraint.Segment, bool) {
	t := av.ProjectValue()
	return staticIndexSegmentFromType(t)
}

func staticIndexSegmentFromType(t typ.Type) (constraint.Segment, bool) {
	switch v := unwrap.Alias(t).(type) {
	case *typ.Annotated:
		return staticIndexSegmentFromType(v.Inner)
	case *typ.Literal:
		switch v.Base {
		case kind.String:
			if s, ok := v.Value.(string); ok {
				return constraint.Segment{Kind: constraint.SegmentIndexString, Name: s}, true
			}
		case kind.Integer:
			if i, ok := v.Value.(int64); ok {
				return constraint.Segment{Kind: constraint.SegmentIndexInt, Index: int(i)}, true
			}
		}
	}
	return constraint.Segment{}, false
}

// applyNumericFor types the control variable of a numeric for-loop
// (for i = init, limit[, step]). The loop body executes with the variable
// ranging over the integer interval the control expressions describe, so the
// variable's value is integer. The relational numeric component seeds the
// variable's loop RANGE [init, limit] (not its init value): the body executes only
// for an in-range index, so a body read `arr[i]` reads in range exactly when the
// range lies within the container's length. Pinning the variable to its init would
// model only the first iteration and over-narrow every subsequent body read.
func (t *Transfer) applyNumericFor(out *flow.PointState, info *cfg.AssignInfo) {
	target, ok := info.FirstTarget()
	if !ok || target.Kind != cfg.TargetIdent || target.Symbol == 0 {
		return
	}
	t.applyIterationTargetWrite(out, target, product.FromType(typ.Integer), false)
	if out.Num != nil && info.NumericFor != nil {
		t.seedNumericForBounds(out, target.Symbol, info.NumericFor)
	}
}

// applyGenericFor types the iteration variables of a generic for-loop
// (for i, v in ipairs(arr)) from the loop's iterator. It resolves the iterator
// function's iteration effect and the iterated container's element/key/value types
// through the CallTyper's IterVars seam, then writes each loop variable's element
// type into its Env slot as a strong assignment update. The loop-header
// join/widening bounds the
// chain; the local target transfer must not join with its previous value or stale
// optional/foreign arms can survive a fresh iterator yield. Every loop target
// first emits an invalidation-only WriteEffect, so even an unrecognized iterator
// clears stale target-derived side facts while preserving the prior value-slot
// carry-forward behavior.
func (t *Transfer) applyGenericFor(
	out *flow.PointState,
	info *cfg.AssignInfo,
	demand func(int, paramevidence.ParamContract),
) {
	for _, iter := range info.IterExprs {
		t.demandConditionReads(out, iter, demand)
	}
	iterCall, ok := info.IterExprs[0].(*ast.FuncCallExpr)
	if !ok {
		iterCall = nil
	}
	t.applyGenericForBinding(out, info, iterCall, demand)
}

// applyGenericForBinding applies the semantic rebinding performed by one
// successful generic-for iterator step. The CFG contains a target assignment
// before the loop branch, but the loop backedge returns to the branch, not that
// assignment point. Canonical flow therefore reuses this operation both at the
// assignment node and on the branch->body edge so each iteration re-establishes
// the current key/value variables and their KeyPresence provenance inside the
// single Kildall equation graph.
func (t *Transfer) applyGenericForBinding(
	out *flow.PointState,
	info *cfg.AssignInfo,
	iterCall *ast.FuncCallExpr,
	demand func(int, paramevidence.ParamContract),
) {
	if out == nil || info == nil {
		return
	}
	t.invalidateIterationTargetWrites(out, info.Targets)
	if iterCall == nil || t.callTyper == nil {
		return
	}
	exprType := func(e ast.Expr) typ.Type {
		return t.resolveExprType(out, e, demand)
	}
	proj, ok := t.iterVarProjection(iterCall, len(info.Targets), exprType)
	if ok && proj.Empty {
		t.assignGenericForEmpty(out, info.Targets)
		return
	}
	// KeyPresence facts only describe values yielded by a live iteration. A
	// recognized-empty source returns above and deliberately seeds no key facts; an
	// unrecognized iterator may still run, so we preserve the prior conservative
	// provenance seeding when the iterator source itself is recognized.
	if ok && len(proj.Types) > 0 {
		for i := range info.Targets {
			target := info.Targets[i]
			if target.Kind != cfg.TargetIdent || target.Symbol == 0 || i >= len(proj.Types) {
				continue
			}
			vt := proj.Types[i]
			if vt == nil || typ.IsAbsentOrUnknown(vt) {
				continue
			}
			val := product.FromType(vt)
			t.applyIterationTargetWrite(out, target, val, false)
		}
	}
	t.seedKeyedIterKeyOf(out, info, iterCall)
	t.seedIndexedIterKeyOf(out, info, iterCall)
	t.seedIteratorValueOrigins(out, info, iterCall)
}

func (t *Transfer) seedIteratorValueOrigins(out *flow.PointState, info *cfg.AssignInfo, iterCall *ast.FuncCallExpr) {
	if t.callTyper == nil || out == nil || info == nil || iterCall == nil {
		return
	}
	if source, ok := t.callTyper.IndexedIterSource(iterCall); ok && !source.IsEmpty() {
		t.seedIteratorValueOriginsFromPath(out, info, source, flow.ValueOriginIndexedIterator)
		return
	}
	if sourceExpr, ok := t.callTyper.KeyedIterSource(iterCall); ok {
		source, ok := t.containerExprPath(sourceExpr)
		if ok && !source.IsEmpty() {
			t.seedIteratorValueOriginsFromPath(out, info, source, flow.ValueOriginKeyedIterator)
		}
	}
}

func (t *Transfer) seedIteratorValueOriginsFromPath(out *flow.PointState, info *cfg.AssignInfo, source constraint.Path, kind flow.ValueOriginKind) {
	for i, target := range info.Targets {
		if target.Kind != cfg.TargetIdent || target.Symbol == 0 {
			continue
		}
		flow.ApplyValueOriginPathProof(out, flow.ValueOriginPathProof{
			ValuePath:  constraint.NewPath(target.Symbol, target.Name),
			SourcePath: source,
			Kind:       kind,
			VarIndex:   i,
		})
	}
}

func (t *Transfer) iterVarProjection(
	iterCall *ast.FuncCallExpr,
	count int,
	exprType func(ast.Expr) typ.Type,
) (iteration.VarProjection, bool) {
	if projector, ok := t.callTyper.(IterVarProjector); ok {
		return projector.IterVarProjection(iterCall, count, exprType)
	}
	varTypes, ok := t.callTyper.IterVars(iterCall, count, exprType)
	if !ok {
		return iteration.VarProjection{}, false
	}
	return iteration.VarProjection{Types: varTypes}, true
}

func (t *Transfer) assignGenericForEmpty(out *flow.PointState, targets []cfg.AssignTarget) {
	for _, target := range targets {
		if target.Kind != cfg.TargetIdent || target.Symbol == 0 {
			continue
		}
		t.applyIterationTargetWrite(out, target, product.FromType(typ.Nil), false)
	}
}

// seedKeyedIterKeyOf records the key-presence fact a keyed (pairs-style)
// iteration establishes: the first loop variable `k` of
// `for k in pairs(container)` is provably a key of `container`, so
// `container[k]` inside the loop body returns a present value. KeyPresence is the
// canonical product-state axis for that runtime provenance; Cond is not used as
// compatibility storage for this fact.
func (t *Transfer) seedKeyedIterKeyOf(out *flow.PointState, info *cfg.AssignInfo, iterCall *ast.FuncCallExpr) {
	if len(info.Targets) == 0 || t.callTyper == nil {
		return
	}
	source, ok := t.callTyper.KeyedIterSource(iterCall)
	if !ok {
		return
	}
	keyTarget := info.Targets[0]
	if keyTarget.Kind != cfg.TargetIdent || keyTarget.Symbol == 0 {
		return
	}
	tablePath, ok := t.containerExprPath(source)
	if !ok {
		return
	}
	keyPath := constraint.NewPath(keyTarget.Symbol, keyTarget.Name)
	effect := flow.KeyProvenancePathProof{
		Kind:      flow.KeyProvenanceKeyedIteration,
		TablePath: tablePath,
		KeyPath:   keyPath,
	}
	if len(info.Targets) > 1 {
		valueTarget := info.Targets[1]
		if valueTarget.Kind == cfg.TargetIdent && valueTarget.Symbol != 0 {
			effect.ValuePath = constraint.NewPath(valueTarget.Symbol, valueTarget.Name)
		}
	}
	t.applyKeyProvenancePathProof(out, effect)
}

// seedKeyArrayForAssignment records the live provenance established by
// `array = keys(container)`: the assigned array's current elements are keys of
// container. The fact lives in PointState.KeyPresence so subsequent writes to the
// array or table can kill it before indexed iteration consumes it.
func (t *Transfer) seedKeyArrayForAssignment(out *flow.PointState, info *cfg.AssignInfo, targetIndex int, target cfg.AssignTarget) {
	if out == nil {
		return
	}
	tablePath, ok := t.keyArrayTableForAssignment(info, targetIndex, target)
	if !ok {
		return
	}
	arrayPath := constraint.NewPath(target.Symbol, target.Name)
	t.applyKeyProvenancePathProof(out, flow.KeyProvenancePathProof{
		Kind:      flow.KeyProvenanceKeyArrayAssignment,
		ArrayPath: arrayPath,
		TablePath: tablePath,
	})
}

func (t *Transfer) keyArrayTableForAssignment(info *cfg.AssignInfo, targetIndex int, target cfg.AssignTarget) (constraint.Path, bool) {
	if info == nil || t.callTyper == nil || target.Kind != cfg.TargetIdent || target.Symbol == 0 {
		return constraint.Path{}, false
	}
	callInfo, retIndex := info.CallForTarget(targetIndex)
	if callInfo == nil || callInfo.Call == nil {
		return constraint.Path{}, false
	}
	tablePath, ok := t.callTyper.KeysCollectorContainer(callInfo, retIndex)
	if !ok || tablePath.IsEmpty() {
		return constraint.Path{}, false
	}
	return tablePath, true
}

// seedIndexedIterKeyOf records the interprocedural key-presence fact an indexed
// (ipairs-style) iteration establishes when the iterated array provably holds
// keys of a container: the value variable `v` of `for _, v in ipairs(names)` is a
// key of that container when live product state still proves `names` is a
// keys-collector result over it (`local names = sorted_keys(c)`, with no
// intervening write to names or c). KeyPresence is the authoritative product
// state.
//
// The value variable is the second loop target (the `v` of `for _, v`), or the
// only target when the loop binds one variable.
func (t *Transfer) seedIndexedIterKeyOf(out *flow.PointState, info *cfg.AssignInfo, iterCall *ast.FuncCallExpr) {
	if t.callTyper == nil || out == nil {
		return
	}
	valueIdx := 1
	if len(info.Targets) == 1 {
		valueIdx = 0
	}
	if valueIdx >= len(info.Targets) {
		return
	}
	valueTarget := info.Targets[valueIdx]
	if valueTarget.Kind != cfg.TargetIdent || valueTarget.Symbol == 0 {
		return
	}
	source, ok := t.callTyper.IndexedIterSource(iterCall)
	if !ok || source.IsEmpty() {
		return
	}
	t.applyKeyProvenancePathProof(out, flow.KeyProvenancePathProof{
		Kind:      flow.KeyProvenanceIndexedKeyArrayIteration,
		ArrayPath: source,
		KeyPath:   constraint.NewPath(valueTarget.Symbol, valueTarget.Name),
	})
}

// evalExpr computes the value of a source expression against the current Env.
// Returns false when the expression's value is not determined here (deferred
// forms), so the caller can drop the target to Top rather than invent a type.
func (t *Transfer) evalExpr(
	out *flow.PointState,
	expr ast.Expr,
	demand func(int, paramevidence.ParamContract),
) (product.AbstractValue, bool) {
	switch e := expr.(type) {
	case *ast.IdentExpr:
		return t.evalIdent(out, e, demand)
	case *ast.NilExpr:
		return product.FromType(typ.Nil), true
	case *ast.StringExpr, *ast.NumberExpr, *ast.TrueExpr, *ast.FalseExpr:
		if lit, ok := literal.FromExpr(expr); ok {
			return product.FromType(lit), true
		}
		return product.AbstractValue{}, false
	case *ast.TableExpr:
		return t.evalTable(out, e, demand)
	case *ast.FunctionExpr:
		if t.funcTyper != nil {
			if fn := t.funcTyper.FuncType(e); fn != nil {
				return product.FromType(fn), true
			}
		}
		return product.AbstractValue{}, false
	case *ast.ArithmeticOpExpr:
		return t.evalBinary(out, e.Operator, e.Lhs, e.Rhs, demand)
	case *ast.StringConcatOpExpr:
		return t.evalConcat(out, e.Lhs, e.Rhs, demand)
	case *ast.LogicalOpExpr:
		return t.evalLogical(out, e, demand)
	case *ast.RelationalOpExpr:
		return t.evalRelational(out, e, demand)
	case *ast.UnaryNotOpExpr:
		// `not x` reads x and yields a boolean.
		t.demandConditionReads(out, e.Expr, demand)
		return product.FromType(typ.Boolean), true
	case *ast.Comma3Expr:
		return t.evalVararg()
	case *ast.UnaryMinusOpExpr:
		return t.evalUnary(out, "-", e.Expr, demand)
	case *ast.UnaryLenOpExpr:
		// `#x` yields an integer only when the operand's abstract value proves it
		// supports Lua length. Gradual `any`/unknown atoms are representation, not
		// evidence; routing through the shared unary operator query keeps the
		// transfer layer aligned with the diagnostic proof boundary.
		return t.evalUnary(out, "#", e.Expr, demand)
	case *ast.CastExpr:
		// `expr :: T` asserts the operand has type T; its value is T. The operand is
		// still read (parameter demand). When the cast type does not resolve, fall
		// back to the operand's own value (the sound carry-forward).
		t.demandConditionReads(out, e.Expr, demand)
		if t.castType != nil && e.Type != nil {
			if ct := t.castType(e.Type); ct != nil && !typ.IsAbsentOrUnknown(ct) {
				return product.FromType(ct), true
			}
		}
		return t.evalExpr(out, e.Expr, demand)
	case *ast.AttrGetExpr:
		return t.evalAttrGet(out, e, demand)
	case *ast.FuncCallExpr:
		// A call (or method call) in a single-value context yields its first return
		// value. The full return vector is produced by evalCall; the multi-target
		// binding in applyAssign reads slot i directly.
		returns, ok := t.evalCall(out, e, demand)
		if !ok || len(returns) == 0 || returns[0].IsZero() {
			return product.AbstractValue{}, false
		}
		return returns[0], true
	default:
		return product.AbstractValue{}, false
	}
}

func (t *Transfer) evalExprWithExpected(
	out *flow.PointState,
	expr ast.Expr,
	expected typ.Type,
	demand func(int, paramevidence.ParamContract),
) (product.AbstractValue, bool) {
	if table, ok := expr.(*ast.TableExpr); ok && table != nil && tableLiteralContextExpected(expected) {
		return t.evalTableWithExpected(out, table, expected, demand)
	}
	return t.evalExpr(out, expr, demand)
}

func (t *Transfer) evalExprAt(
	out *flow.PointState,
	p cfg.Point,
	expr ast.Expr,
	demand func(int, paramevidence.ParamContract),
) (product.AbstractValue, bool) {
	switch e := expr.(type) {
	case *ast.AttrGetExpr:
		return t.evalAttrGetAt(out, p, e, demand)
	case *ast.CastExpr:
		t.demandConditionReads(out, e.Expr, demand)
		if t.castType != nil && e.Type != nil {
			if ct := t.castType(e.Type); ct != nil && !typ.IsAbsentOrUnknown(ct) {
				return product.FromType(ct), true
			}
		}
		return t.evalExprAt(out, p, e.Expr, demand)
	default:
		return t.evalExpr(out, expr, demand)
	}
}

// evalCall types a call or method-call expression's Lua return vector through the
// product call seam. It resolves argument product values from the current Env and
// emits parameter demand for arguments that are parameter reads. The driver may
// still use type-only signatures internally, but transfer no longer exposes a
// second type-only return route beside the product carrier.
//
// The receiver of a method call and any field-path callee are resolved through the
// live point state, so a method on a tracked local resolves its receiver value.
// Returns false when no CallTyper is wired or the callee does not resolve, so the
// caller drops the slot to the value-domain Top (sound: precision loss).
func (t *Transfer) evalCall(
	out *flow.PointState,
	call *ast.FuncCallExpr,
	demand func(int, paramevidence.ParamContract),
) ([]product.AbstractValue, bool) {
	if call == nil {
		return nil, false
	}
	if set, ok := t.evalSetMetatableCall(out, call, demand); ok {
		return []product.AbstractValue{set}, true
	}
	if created, ok := t.evalTableCreateCall(out, call); ok {
		return []product.AbstractValue{created}, true
	}
	if t.callTyper == nil {
		return nil, false
	}
	// Emit parameter demand for argument and receiver reads, and resolve each
	// argument's product value for the call pipeline's generic inference and arity.
	ctx := t.productCallContext(out, call, demand)
	result := t.productCallResult(call, ctx)
	t.applyCallArgDemands(out, call, result.ArgDemands, demand)
	t.applyCallResultEffects(out, call, ctx, result.Effects, demand)
	if result.HasReturnValues && len(result.ReturnValues) > 0 {
		out2 := make([]product.AbstractValue, len(result.ReturnValues))
		copy(out2, result.ReturnValues)
		return out2, true
	}
	if ctx.PendingInput {
		return nil, false
	}
	return nil, false
}

// IntrinsicCallReturnValues projects transfer-owned intrinsic call semantics for
// diagnostic observation. These calls depend on source syntax or product
// allocation state rather than a reusable function summary, so the observation
// layer should consume the same transfer reducer instead of falling back to a
// weaker type-only stdlib signature.
func (t *Transfer) IntrinsicCallReturnValues(
	out *flow.PointState,
	call *ast.FuncCallExpr,
	demand func(int, paramevidence.ParamContract),
) ([]product.AbstractValue, bool) {
	if call == nil {
		return nil, false
	}
	if created, ok := t.evalTableCreateCall(out, call); ok {
		return []product.AbstractValue{created}, true
	}
	return nil, false
}

func (t *Transfer) evalSetMetatableCall(
	out *flow.PointState,
	call *ast.FuncCallExpr,
	demand func(int, paramevidence.ParamContract),
) (product.AbstractValue, bool) {
	if out == nil || call == nil || len(call.Args) < 2 {
		return product.AbstractValue{}, false
	}
	if t.in.Graph == nil || !metatable.IsSetMetatableCall(call, t.in.Graph.Bindings()) {
		return product.AbstractValue{}, false
	}
	instance, ok := t.evalExpr(out, call.Args[0], demand)
	if !ok || instance.IsZero() {
		return product.AbstractValue{}, false
	}
	var meta product.AbstractValue
	var metaOK bool
	if proto, ok := t.setMetatablePrototype(0, call); ok && proto != 0 {
		if protoMeta, ok := t.prototypeMetatableValue(out, proto); ok && !protoMeta.IsZero() {
			meta = protoMeta
			metaOK = true
		}
	}
	if !metaOK {
		meta, metaOK = t.evalExpr(out, call.Args[1], demand)
	}
	if !metaOK || meta.IsZero() {
		return product.AbstractValue{}, false
	}
	return product.WithMetatable(instance, meta)
}

func (t *Transfer) runtimeArgumentValues(
	out *flow.PointState,
	call *ast.FuncCallExpr,
	argValues []product.AbstractValue,
	demand func(int, paramevidence.ParamContract),
) []product.AbstractValue {
	if call == nil {
		return nil
	}
	n := len(call.Args)
	if call.Method != "" {
		n++
	}
	if n == 0 {
		return nil
	}
	outValues := make([]product.AbstractValue, n)
	offset := 0
	if call.Method != "" {
		t.demandConditionReads(out, call.Receiver, demand)
		if av, ok := t.resolveExprValue(out, call.Receiver, demand); ok && !av.IsZero() {
			outValues[0] = av
		}
		offset = 1
	}
	for i := range call.Args {
		if i < len(argValues) && !argValues[i].IsZero() {
			outValues[i+offset] = argValues[i]
		}
	}
	return outValues
}

func (t *Transfer) callArgumentValues(
	out *flow.PointState,
	call *ast.FuncCallExpr,
	demand func(int, paramevidence.ParamContract),
) []product.AbstractValue {
	if call == nil {
		return nil
	}
	t.demandConditionReads(out, call.Receiver, demand)
	outValues := make([]product.AbstractValue, len(call.Args))
	for i, arg := range call.Args {
		t.demandConditionReads(out, arg, demand)
		if av, ok := t.resolveExprValue(out, arg, demand); ok && !av.IsZero() {
			outValues[i] = av
		}
	}
	return outValues
}

func (t *Transfer) applyCallArgDemands(
	out *flow.PointState,
	call *ast.FuncCallExpr,
	expected []callobligation.Obligation,
	demand func(int, paramevidence.ParamContract),
) {
	if out == nil || call == nil || demand == nil || len(expected) == 0 {
		return
	}
	for i, arg := range call.Args {
		if i >= len(expected) {
			break
		}
		if !expected[i].Informative() {
			continue
		}
		if contract, ok := paramevidence.ObligationContract(expected[i]); ok {
			t.demandExprContractCtx(out, arg, contract, demand)
			continue
		}
		t.demandExprCtx(out, arg, expected[i].Type, demand)
	}
}

func (t *Transfer) exprValueResolver(
	out *flow.PointState,
	demand func(int, paramevidence.ParamContract),
) func(ast.Expr) (product.AbstractValue, bool) {
	return func(e ast.Expr) (product.AbstractValue, bool) {
		return t.resolveExprValue(out, e, demand)
	}
}

func (t *Transfer) boundaryFactsAppendPlans(
	out *flow.PointState,
	call *ast.FuncCallExpr,
	facts flow.BoundaryFacts,
) (flow.BoundaryFacts, []boundaryAppendKeyPlan) {
	if !facts.HasProof() {
		return facts, nil
	}
	return facts, t.boundaryAppendKeyPlans(out, call, facts, nil)
}

func (t *Transfer) applyBoundaryFacts(
	out *flow.PointState,
	call *ast.FuncCallExpr,
	facts flow.BoundaryFacts,
	returns map[int]constraint.Path,
) bool {
	plans := t.boundaryAppendKeyPlans(out, call, facts, returns)
	return t.applyBoundaryFactsWithAppendPlans(out, call, facts, returns, plans)
}

type boundaryAppendKeyPlan struct {
	array               constraint.Path
	key                 constraint.Path
	table               constraint.Path
	hasTable            bool
	writtenTables       []constraint.Path
	freshEmpty          bool
	preserveHistoryBase bool
}

func (t *Transfer) boundaryAppendKeyPlans(
	out *flow.PointState,
	call *ast.FuncCallExpr,
	facts flow.BoundaryFacts,
	returns map[int]constraint.Path,
) []boundaryAppendKeyPlan {
	if out == nil || call == nil {
		return nil
	}
	appendKeys := facts.AppendKeys()
	if len(appendKeys) == 0 {
		return nil
	}
	var plans []boundaryAppendKeyPlan
	for _, fact := range appendKeys {
		array, ok := t.rebaseBoundaryPath(call, returns, fact.Array)
		if !ok || array.IsEmpty() {
			continue
		}
		key, ok := t.rebaseBoundaryPath(call, returns, fact.Key)
		if !ok || key.IsEmpty() {
			continue
		}
		factsView := flow.PointFactsOf(*out)
		plan := boundaryAppendKeyPlan{
			array:               array,
			key:                 key,
			freshEmpty:          factsView.HasEmptyKeyArray(array) || flow.AppendHistoryBaseWithoutEventsPath(*out, array) || t.arrayPathCurrentlyEmpty(out, array),
			preserveHistoryBase: factsView.HasAppendHistoryBase(array),
		}
		if fact.HasTable {
			table, ok := t.rebaseBoundaryPath(call, returns, fact.Table)
			if !ok || table.IsEmpty() {
				continue
			}
			plan.table = table
			plan.hasTable = true
		}
		plan.writtenTables = t.boundaryIndexWriteTablesForAppendedKey(call, returns, facts.IndexWrites(), key, plan.table, plan.hasTable)
		plans = append(plans, plan)
	}
	return plans
}

func (t *Transfer) boundaryIndexWriteTablesForAppendedKey(
	call *ast.FuncCallExpr,
	returns map[int]constraint.Path,
	indexWrites []flow.BoundaryIndexWriteFact,
	key constraint.Path,
	explicitTable constraint.Path,
	hasExplicitTable bool,
) []constraint.Path {
	if call == nil || key.IsEmpty() || len(indexWrites) == 0 {
		return nil
	}
	var tables []constraint.Path
	for _, fact := range indexWrites {
		writeKey, ok := t.rebaseBoundaryPath(call, returns, fact.Key)
		if !ok || !writeKey.Equal(key) {
			continue
		}
		table, ok := t.rebaseBoundaryPath(call, returns, fact.Table)
		if !ok || table.IsEmpty() {
			continue
		}
		if hasExplicitTable && !table.Equal(explicitTable) {
			continue
		}
		tables = append(tables, table)
	}
	return tables
}

func (t *Transfer) applyBoundaryFactsWithAppendPlans(
	out *flow.PointState,
	call *ast.FuncCallExpr,
	facts flow.BoundaryFacts,
	returns map[int]constraint.Path,
	appendPlans []boundaryAppendKeyPlan,
) bool {
	if out == nil || call == nil {
		return false
	}
	changed := false
	for _, fact := range facts.IndexWrites() {
		table, ok := t.rebaseBoundaryPath(call, returns, fact.Table)
		if !ok {
			continue
		}
		key, ok := t.rebaseBoundaryPath(call, returns, fact.Key)
		if !ok {
			continue
		}
		changed = flow.ApplyMapWritePathProof(out, flow.MapWritePathProof{
			TablePath:              table,
			KeyPath:                key,
			KeyValue:               product.FromType(typ.Unknown),
			Value:                  fact.Value,
			AllowOpaqueKeyReadback: true,
		}) || changed
	}
	for _, fact := range facts.KeyPresence() {
		table, ok := t.rebaseBoundaryPath(call, returns, fact.Table)
		if !ok {
			continue
		}
		key, ok := t.rebaseBoundaryPath(call, returns, fact.Key)
		if !ok {
			continue
		}
		changed = t.applyKeyProvenancePathProof(out, flow.KeyProvenancePathProof{
			Kind:      flow.KeyProvenanceDynamicIndexWrite,
			TablePath: table,
			KeyPath:   key,
		}) || changed
	}
	for _, fact := range facts.KeyArrays() {
		array, ok := t.rebaseBoundaryPath(call, returns, fact.Array)
		if !ok {
			continue
		}
		table, ok := t.rebaseBoundaryPath(call, returns, fact.Table)
		if !ok {
			continue
		}
		changed = t.applyKeyProvenancePathProof(out, flow.KeyProvenancePathProof{
			Kind:      flow.KeyProvenanceKeyArrayAssignment,
			ArrayPath: array,
			TablePath: table,
		}) || changed
	}
	for _, fact := range facts.KeyArrayValues() {
		array, ok := t.rebaseBoundaryPath(call, returns, fact.Array)
		if !ok {
			continue
		}
		table, ok := t.rebaseBoundaryPath(call, returns, fact.Table)
		if !ok {
			continue
		}
		changed = flow.ApplyKeyArrayValuePathProof(out, flow.KeyArrayValuePathProof{
			ArrayPath: array,
			TablePath: table,
			Value:     fact.Value,
		}) || changed
	}
	changed = t.applyBoundaryAppendKeyPlans(out, appendPlans) || changed
	for _, fact := range facts.AppendElementFieldOrigins() {
		array, ok := t.rebaseBoundaryPath(call, returns, fact.Array)
		if !ok {
			continue
		}
		source, ok := t.rebaseBoundaryPath(call, returns, fact.Source)
		if !ok {
			continue
		}
		changed = flow.ApplyAppendElementFieldOriginPathProof(out, flow.AppendElementFieldOriginPathProof{
			ArrayPath:   array,
			Field:       fact.Field,
			SourcePath:  source,
			SourceField: fact.SourceField,
		}) || changed
	}
	var ops []flow.NumericOp
	for _, fact := range facts.LengthLowerBounds() {
		target, ok := t.rebaseBoundaryPath(call, returns, fact.Target)
		if !ok {
			continue
		}
		if op, ok := flow.NumericLenGeConstPathOp(target, fact.Lower); ok {
			ops = append(ops, op)
		}
	}
	if len(ops) > 0 {
		changed = flow.ApplyNumericEffect(out, flow.NumericEffect{Ops: ops}) || changed
	}
	return changed
}

func (t *Transfer) applyBoundaryAppendKeyPlans(out *flow.PointState, plans []boundaryAppendKeyPlan) bool {
	if out == nil || len(plans) == 0 {
		return false
	}
	changed := false
	for _, plan := range plans {
		changed = flow.ApplyAppendKeyReplayPathProof(out, flow.AppendKeyReplayPathProof{
			ArrayPath:           plan.array,
			KeyPath:             plan.key,
			ExplicitTablePath:   plan.table,
			HasExplicitTable:    plan.hasTable,
			WrittenTablePaths:   plan.writtenTables,
			FreshEmpty:          plan.freshEmpty,
			PreserveHistoryBase: plan.preserveHistoryBase,
		}) || changed
	}
	return changed
}

func (t *Transfer) rebaseBoundaryPath(
	call *ast.FuncCallExpr,
	returns map[int]constraint.Path,
	path flow.BoundaryPath,
) (constraint.Path, bool) {
	var out constraint.Path
	switch path.Kind {
	case flow.BoundaryPathParam:
		arg := callsite.RuntimeArgExprAt(call, path.Index)
		if arg == nil {
			return constraint.Path{}, false
		}
		argPath, ok := t.staticPathOfExpr(arg)
		if !ok || argPath.IsEmpty() {
			return constraint.Path{}, false
		}
		out = argPath
	case flow.BoundaryPathReturn:
		if returns == nil {
			return constraint.Path{}, false
		}
		retPath, ok := returns[path.Index]
		if !ok || retPath.IsEmpty() {
			return constraint.Path{}, false
		}
		out = retPath
	default:
		return constraint.Path{}, false
	}
	for _, seg := range path.Segments {
		out = out.Append(seg)
	}
	return out, true
}

func (t *Transfer) callClosurePath(call *ast.FuncCallExpr) (constraint.Path, bool) {
	if call == nil {
		return constraint.Path{}, false
	}
	if call.Method != "" {
		path, ok := t.containerExprPath(call.Receiver)
		if !ok || path.IsEmpty() {
			return constraint.Path{}, false
		}
		path.Segments = append(append([]constraint.Segment(nil), path.Segments...), constraint.Segment{
			Kind: constraint.SegmentField,
			Name: call.Method,
		})
		return path, true
	}
	return t.containerExprPath(call.Func)
}

func (t *Transfer) applySetMetatablePrototypeSelf(
	out *flow.PointState,
	p cfg.Point,
	call *ast.FuncCallExpr,
	demand func(int, paramevidence.ParamContract),
) {
	if out == nil || call == nil || len(call.Args) < 2 ||
		(len(t.setMetatableProtoByPoint) == 0 && len(t.metatablePrototypeBySym) == 0) {
		return
	}
	proto, ok := t.setMetatablePrototype(p, call)
	if !ok || proto == 0 {
		return
	}
	instance, ok := t.evalExpr(out, call.Args[0], demand)
	if !ok || instance.IsZero() {
		return
	}
	meta, ok := t.prototypeMetatableValue(out, proto)
	if !ok || meta.IsZero() {
		meta, ok = t.evalExpr(out, call.Args[1], demand)
	}
	if ok && !meta.IsZero() {
		if withMeta, ok := product.WithMetatable(instance, meta); ok && !withMeta.IsZero() {
			instance = withMeta
		}
	}
	flow.RecordPrototypeSelf(out, proto, instance)
}

func (t *Transfer) setMetatablePrototype(p cfg.Point, call *ast.FuncCallExpr) (cfg.SymbolID, bool) {
	if proto, ok := t.setMetatableProtoByPoint[p]; ok && proto != 0 {
		return proto, true
	}
	if proto := t.inlineSetMetatablePrototype(call); proto != 0 {
		return proto, true
	}
	if call == nil || len(call.Args) < 2 || len(t.metatablePrototypeBySym) == 0 {
		return 0, false
	}
	mt := t.symbolOfIdent(call.Args[1])
	if mt == 0 {
		return 0, false
	}
	proto, ok := t.metatablePrototypeBySym[mt]
	return proto, ok && proto != 0
}

func (t *Transfer) inlineSetMetatablePrototype(call *ast.FuncCallExpr) cfg.SymbolID {
	if call == nil || len(call.Args) < 2 || t.in.Graph == nil || t.in.Graph.Bindings() == nil {
		return 0
	}
	tbl, ok := call.Args[1].(*ast.TableExpr)
	if !ok {
		return 0
	}
	for _, field := range tbl.Fields {
		if field == nil || field.Key == nil {
			continue
		}
		name, ok := fieldkey.StringKeyFromTableField(field)
		if !ok || name != "__index" {
			continue
		}
		return t.symbolOfIdent(field.Value)
	}
	return 0
}

func (t *Transfer) prototypeMetatableValue(out *flow.PointState, proto cfg.SymbolID) (product.AbstractValue, bool) {
	base := typ.NewRecord().Build()
	protoType, hasMethodSurface := t.prototypeSurfaceType(out, proto, base)
	if !hasMethodSurface {
		return product.AbstractValue{}, false
	}
	meta := typ.NewRecord().Field("__index", protoType).Build()
	return product.FromType(meta), true
}

func (t *Transfer) prototypeSurfaceType(out *flow.PointState, proto cfg.SymbolID, base typ.Type) (typ.Type, bool) {
	surface := base
	hasMethodSurface := false
	for _, method := range t.prototypeMethods {
		if method.PrototypeSym != proto || method.FuncRef == (flow.FunctionRef{}) || method.Field == (constraint.Segment{}) {
			continue
		}
		fnType, ok := t.callableSignature(out, flow.CallableSignatureQuery{Ref: method.FuncRef})
		if !ok || typ.IsAbsentOrUnknown(fnType) {
			continue
		}
		switch method.Field.Kind {
		case constraint.SegmentField:
			surface = typ.ExtendRecordWithField(surface, method.Field.Name, fnType)
			hasMethodSurface = true
		case constraint.SegmentIndexString:
			surface = typ.ExtendRecordWithField(surface, method.Field.Name, fnType)
			hasMethodSurface = true
		}
	}
	if surface == nil {
		return typ.Unknown, hasMethodSurface
	}
	return surface, hasMethodSurface
}

func (t *Transfer) symbolOfIdent(expr ast.Expr) cfg.SymbolID {
	ident, ok := expr.(*ast.IdentExpr)
	if !ok {
		return 0
	}
	return t.symbolOf(ident)
}

// evalTable types a table-constructor value. It builds a record whose
// string/identifier-keyed fields carry the value the transfer resolves for each
// field source (a literal, a nested table, an identifier, or a function literal),
// the AbstractValue-native counterpart of the observation table typing. A field
// whose source does not resolve is omitted (the record stays open, so a later read
// of it is unknown, not a false absence). An array part or a dynamic key is not
// added as a named field; the resulting record over-approximates such a literal as
// the typed-field record, which a subsequent read narrows soundly.
func (t *Transfer) evalTable(
	out *flow.PointState,
	e *ast.TableExpr,
	demand func(int, paramevidence.ParamContract),
) (product.AbstractValue, bool) {
	// A vararg-spread literal (`{...}`) is an array of the function's vararg element
	// (`{...}` over `function f(...: number)` is number[]), the array shape the
	// generic-for iteration reads. This is the array-like literal built from a
	// vararg field; an ordinary positional literal (`{1, 2, 3}`)
	// keeps the prior record over-approximation, so a tuple-arity index read is
	// unaffected.
	if elem := t.varargSpreadElement(e); elem != nil {
		return product.FromType(typ.NewArray(elem)), true
	}
	// A pure positional literal (`{1, 2, 3}`) is a fixed-arity tuple: its runtime
	// length is its element count, so a literal/in-range index read resolves to the
	// matching element rather than the optional array element. This matches the
	// observation surface's positional-literal typing (a record over-approximation
	// would lose the static arity the in-bounds proof relies on).
	if tup, ok := t.positionalTupleLiteral(out, e, demand); ok {
		return tup, true
	}
	if len(e.Fields) == 0 {
		return product.FromType(typ.NewFreshEmptyRecord()), true
	}
	av := product.FromType(typ.NewRecord().Build())
	for _, field := range e.Fields {
		if field == nil || field.Key == nil {
			continue
		}
		key, ok := fieldkey.FromTableField(field)
		if !ok {
			continue
		}
		// A statically-named field IS present in the literal: the constructor writes
		// it. When its value type does not resolve (an unannotated parameter source, a
		// call whose return is unknown), the field is still present with a gradual
		// `any` value — recording it keeps a later read of the field from a false
		// absence, while a value that resolves carries its precise type.
		ft := typ.Any
		if fv, ok := t.evalExpr(out, field.Value, demand); ok && !fv.IsZero() {
			if pt := fv.ProjectValue(); pt != nil && !typ.IsUnknown(pt) {
				ft = pt
			}
		}
		member, ok := value.MemberFromSegment(key)
		if !ok {
			continue
		}
		av = product.WithMember(av, member, product.FromType(ft))
	}
	return av, true
}

func (t *Transfer) evalTableWithExpected(
	out *flow.PointState,
	e *ast.TableExpr,
	expected typ.Type,
	demand func(int, paramevidence.ParamContract),
) (product.AbstractValue, bool) {
	expected = discriminatedTableExpected(e, expected)
	if !tableLiteralContextExpected(expected) {
		return t.evalTable(out, e, demand)
	}
	entries, elems, complete := t.evalTableEntriesWithExpected(out, e, expected, demand)
	if !complete {
		return t.evalTable(out, e, demand)
	}
	result := ops.CheckTableEntries(entries, elems, expected)
	if len(result.Errors) != 0 {
		return t.evalTable(out, e, demand)
	}
	if result.Type != nil && !typ.IsAbsentOrUnknown(result.Type) && !typ.IsAny(result.Type) {
		return product.FromType(result.Type), true
	}
	return product.FromType(expected), true
}

func (t *Transfer) evalTableEntriesWithExpected(
	out *flow.PointState,
	e *ast.TableExpr,
	expected typ.Type,
	demand func(int, paramevidence.ParamContract),
) ([]ops.EntryDef, []typ.Type, bool) {
	if e == nil {
		return nil, nil, false
	}
	entries := make([]ops.EntryDef, 0, len(e.Fields))
	var elems []typ.Type
	complete := true
	for _, field := range e.Fields {
		if field == nil {
			continue
		}
		if field.Key == nil {
			elemExpected := ops.ExpectedTableElementType(expected, len(elems))
			elemType, ok := t.evalTableEntryValueType(out, field.Value, elemExpected, demand)
			if !ok {
				complete = false
			}
			elems = append(elems, elemType)
			continue
		}
		key, ok := fieldkey.FromTableField(field)
		if !ok {
			complete = false
			continue
		}
		fieldExpected := ops.ExpectedTableEntryType(expected, key)
		fieldType, ok := t.evalTableEntryValueType(out, field.Value, fieldExpected, demand)
		if !ok {
			complete = false
		}
		entries = append(entries, ops.EntryDef{Key: key, Type: fieldType})
	}
	return entries, elems, complete
}

func (t *Transfer) evalTableEntryValueType(
	out *flow.PointState,
	expr ast.Expr,
	expected typ.Type,
	demand func(int, paramevidence.ParamContract),
) (typ.Type, bool) {
	if table, ok := expr.(*ast.TableExpr); ok && table != nil && tableLiteralContextExpected(expected) {
		if av, ok := t.evalTableWithExpected(out, table, expected, demand); ok && !av.IsZero() {
			pt := product.ProjectValueOrUnknown(av)
			return pt, tableEntryValueIsProof(pt)
		}
		return typ.Unknown, false
	}
	av, ok := t.evalExpr(out, expr, demand)
	if !ok || av.IsZero() {
		return typ.Unknown, false
	}
	pt := product.ProjectValueOrUnknown(av)
	return pt, tableEntryValueIsProof(pt)
}

func tableEntryValueIsProof(t typ.Type) bool {
	return t != nil && !typ.IsAbsentOrUnknown(t) && !typ.IsAny(t)
}

func discriminatedTableExpected(e *ast.TableExpr, expected typ.Type) typ.Type {
	if e == nil || expected == nil {
		return expected
	}
	if match := querycore.TryDiscriminatedUnionMember(e, expected); match != nil && match.Member != nil {
		return match.Member
	}
	return expected
}

func tableLiteralContextExpected(expected typ.Type) bool {
	if expected == nil || typ.IsAbsentOrUnknown(expected) || typ.IsAny(expected) {
		return false
	}
	expected = typ.UnwrapAnnotated(expected)
	switch v := expected.(type) {
	case *typ.Alias:
		return tableLiteralContextExpected(v.Target)
	case *typ.Optional:
		return tableLiteralContextExpected(v.Inner)
	case *typ.Instantiated:
		if resolved, err := querycore.ResolveInstantiated(v); err == nil {
			return tableLiteralContextExpected(resolved)
		}
		return false
	case *typ.Recursive:
		if v.Body == nil || v.Body == v {
			return false
		}
		return tableLiteralContextExpected(v.Body)
	case *typ.Union:
		for _, member := range v.Members {
			if !tableLiteralContextExpected(member) {
				return false
			}
		}
		return len(v.Members) > 0
	case *typ.Intersection:
		for _, member := range v.Members {
			if tableLiteralContextExpected(member) {
				return true
			}
		}
		return false
	case *typ.Array, *typ.Map, *typ.ReadonlyMap, *typ.Record, *typ.Tuple:
		return true
	default:
		return false
	}
}

// varargSpreadElement reports the element type of a pure vararg-spread literal
// (`{...}`): the function's declared vararg element. It applies only when the literal's
// sole field is the vararg expression with no key, the array-construction form a
// generic-for iterates. A literal with any other field, or with no resolved vararg
// element, yields nil so evalTable keeps the record over-approximation.
func (t *Transfer) varargSpreadElement(e *ast.TableExpr) typ.Type {
	if len(e.Fields) != 1 {
		return nil
	}
	field := e.Fields[0]
	if field == nil || field.Key != nil {
		return nil
	}
	if _, isVararg := field.Value.(*ast.Comma3Expr); !isVararg {
		return nil
	}
	return t.varargElem()
}

// evalAttrGet computes the runtime value of a field or index read base.key
// against the current Env. It reads the base container's value, then applies the
// product-domain runtime member/index read: strict structural lookup when the
// slot is present, nil when a table-like shape proves the slot is absent, and
// unresolved only for non-table/deferred reads.
func (t *Transfer) evalAttrGet(
	out *flow.PointState,
	e *ast.AttrGetExpr,
	demand func(int, paramevidence.ParamContract),
) (product.AbstractValue, bool) {
	if out != nil {
		if path, hasPath := t.staticPathOfExpr(e); hasPath {
			if fact, ok := flow.PointFactsOf(*out).StaticMemberValue(path); ok {
				return t.callablePathRead(out, path, fact)
			}
		}
	}
	base, ok := t.evalExpr(out, e.Object, demand)
	if !ok || base.IsZero() {
		return product.AbstractValue{}, false
	}
	if member, isStatic := staticMemberKey(e); isStatic {
		fv, ok := product.RuntimeMemberOf(base, member)
		if !ok || fv.IsZero() {
			if path, hasPath := t.staticPathOfExpr(e); hasPath {
				if cv, ok := t.callablePathReadIfCallable(out, path, fv); ok {
					return cv, true
				}
			}
			return product.AbstractValue{}, false
		}
		if path, hasPath := t.staticPathOfExpr(e); hasPath {
			if cv, ok := t.callablePathReadIfCallable(out, path, fv); ok {
				return cv, true
			}
		}
		return t.refineIndexRead(out, e, base, fv), true
	}
	key, ok := t.evalExpr(out, e.Key, demand)
	if !ok || key.IsZero() {
		return product.AbstractValue{}, false
	}
	ev, ok := product.RuntimeIndexOf(base, key)
	if !ok || ev.IsZero() {
		if admitted, admittedOK := t.refineByIndexWriteAdmission(out, e); admittedOK {
			return admitted, true
		}
		return product.AbstractValue{}, false
	}
	return t.refineIndexRead(out, e, base, ev), true
}

func (t *Transfer) evalAttrGetAt(
	out *flow.PointState,
	p cfg.Point,
	e *ast.AttrGetExpr,
	demand func(int, paramevidence.ParamContract),
) (product.AbstractValue, bool) {
	readPath := constraint.Path{}
	if out != nil {
		if path, hasPath := t.staticMemberExprPathAt(out, p, e); hasPath {
			readPath = path
			if fact, ok := flow.PointFactsOf(*out).StaticMemberValue(path); ok {
				return t.callablePathRead(out, path, fact)
			}
		}
	}
	base, ok := t.evalExprAt(out, p, e.Object, demand)
	if !ok || base.IsZero() {
		return product.AbstractValue{}, false
	}
	if member, isStatic := staticMemberKeyWithConst(e, t.constResolverAt(p)); isStatic {
		fv, ok := product.RuntimeMemberOf(base, member)
		if !ok || fv.IsZero() {
			if !readPath.IsEmpty() {
				if cv, ok := t.callablePathReadIfCallable(out, readPath, fv); ok {
					return cv, true
				}
			}
			return product.AbstractValue{}, false
		}
		if !readPath.IsEmpty() {
			if cv, ok := t.callablePathReadIfCallable(out, readPath, fv); ok {
				return cv, true
			}
		}
		return t.refineIndexRead(out, e, base, fv), true
	}
	key, ok := t.evalExprAt(out, p, e.Key, demand)
	if !ok || key.IsZero() {
		return product.AbstractValue{}, false
	}
	ev, ok := product.RuntimeIndexOf(base, key)
	if !ok || ev.IsZero() {
		if admitted, admittedOK := t.refineByIndexWriteAdmission(out, e); admittedOK {
			return admitted, true
		}
		return product.AbstractValue{}, false
	}
	return t.refineIndexRead(out, e, base, ev), true
}

// evalIdent reads an identifier's value from Env and emits parameter demand when
// the identifier is a parameter (a body read pins the parameter's value).
func (t *Transfer) evalIdent(
	out *flow.PointState,
	e *ast.IdentExpr,
	demand func(int, paramevidence.ParamContract),
) (product.AbstractValue, bool) {
	sym := t.symbolOf(e)
	if sym == 0 {
		if meta, ok := t.typeValueOf(e); ok {
			return meta, true
		}
		return product.AbstractValue{}, false
	}
	av, ok := t.symbolValue(out, sym)
	path := constraint.NewPath(sym, "")
	if !ok || av.IsZero() {
		if cv, ok := t.callablePathValue(out, path); ok {
			return cv, true
		}
		// A symbol with no flow value may name a `type` used as a value (the `type X`
		// binding carries a symbol but no runtime value); resolve it to that type's Meta.
		if meta, ok := t.typeValueOf(e); ok {
			return meta, true
		}
		return product.AbstractValue{}, false
	}
	if pt := av.ProjectValue(); pt != nil && pt.Kind() == kind.Function {
		if cv, ok := t.callablePathRead(out, path, av); ok {
			return cv, true
		}
	}
	return av, true
}

func (t *Transfer) callableSignatureResolver(out *flow.PointState) flow.CallableSignatureResolver {
	return func(query flow.CallableSignatureQuery) (typ.Type, bool) {
		return t.callableSignature(out, query)
	}
}

func (t *Transfer) callableSignature(out *flow.PointState, query flow.CallableSignatureQuery) (typ.Type, bool) {
	if out == nil || t.callTyper == nil {
		return nil, false
	}
	provider, ok := t.callTyper.(functionValueProvider)
	if !ok {
		return nil, false
	}
	if query.Ref == (flow.FunctionRef{}) && query.Path.IsEmpty() {
		return nil, false
	}
	query.State = flow.PointState{
		Cells:        out.Cells,
		FunctionRefs: out.FunctionRefs,
		ClosureRefs:  out.ClosureRefs,
	}
	ft, ok := provider.FunctionValue(query)
	if !ok || typ.IsAbsentOrUnknown(ft) {
		return nil, false
	}
	return ft, true
}

func (t *Transfer) callablePathValue(out *flow.PointState, path constraint.Path) (product.AbstractValue, bool) {
	if out == nil || path.IsEmpty() {
		return product.AbstractValue{}, false
	}
	return flow.PointFactsOf(*out).CallablePathValue(path, t.callableSignatureResolver(out))
}

func (t *Transfer) callablePathRead(out *flow.PointState, path constraint.Path, read product.AbstractValue) (product.AbstractValue, bool) {
	if out == nil || path.IsEmpty() {
		if read.IsZero() {
			return product.AbstractValue{}, false
		}
		return read, true
	}
	return flow.PointFactsOf(*out).CallablePathRead(path, read, t.callableSignatureResolver(out))
}

func (t *Transfer) callablePathReadIfCallable(out *flow.PointState, path constraint.Path, read product.AbstractValue) (product.AbstractValue, bool) {
	if out == nil || path.IsEmpty() {
		return product.AbstractValue{}, false
	}
	facts := flow.PointFactsOf(*out)
	if _, ok := facts.CallablePathType(path, t.callableSignatureResolver(out)); !ok {
		return product.AbstractValue{}, false
	}
	return facts.CallablePathRead(path, read, t.callableSignatureResolver(out))
}

func (t *Transfer) symbolValue(out *flow.PointState, sym cfg.SymbolID) (product.AbstractValue, bool) {
	return t.symbolStorage.read(out, sym)
}

// valueBySlot is the transfer-owned logical-slot read boundary. Symbol slots
// route through symbolStoragePolicy so Env-backed locals cannot be shadowed by a
// same-ID capture-cell entry; non-symbol key slots remain ordinary Env facts.
func (t *Transfer) valueBySlot(out flow.PointState, slot flow.ValueSlot) (product.AbstractValue, bool) {
	if sym, ok := slot.Symbol(); ok {
		return t.symbolValue(&out, sym)
	}
	if key, ok := slot.Key(); ok {
		av, ok := flow.PointFactsOf(out).ValueKeyValue(key)
		if !ok || valueIsBottom(av) {
			return product.AbstractValue{}, false
		}
		return av, true
	}
	return product.AbstractValue{}, false
}

func (t *Transfer) setSymbolValue(out *flow.PointState, sym cfg.SymbolID, val product.AbstractValue, joinExisting bool) {
	t.writeSymbolValue(out, sym, val, joinExisting, false)
}

func (t *Transfer) writeSymbolValue(out *flow.PointState, sym cfg.SymbolID, val product.AbstractValue, joinExisting bool, emitEffect bool) {
	t.symbolStorage.write(out, sym, val, joinExisting, emitEffect)
}

func (t *Transfer) setValueBySlot(out *flow.PointState, slot flow.ValueSlot, val product.AbstractValue, joinExisting bool) {
	if sym, ok := slot.Symbol(); ok {
		t.setSymbolValue(out, sym, val, joinExisting)
		return
	}
	if key, ok := slot.Key(); ok {
		flow.NewPointWriter(out).WriteValueKey(key, val, joinExisting)
	}
}

// typeValueOf resolves an identifier naming a `type` used as a value to that type's
// reified Meta (the type value carrying the built-in `:is` guard), via the driver's
// MetaForName-backed resolver. MetaForName yields nil for a name that is not a defined
// type or has a shadowing value binding, so a genuine value variable or undefined
// identifier never resolves here.
func (t *Transfer) typeValueOf(e *ast.IdentExpr) (product.AbstractValue, bool) {
	if t.typeNameValue == nil || e == nil {
		return product.AbstractValue{}, false
	}
	meta := t.typeNameValue(e.Value)
	if meta == nil || typ.IsAbsentOrUnknown(meta) {
		return product.AbstractValue{}, false
	}
	return product.FromType(meta), true
}

// isCapturedFreeVar reports whether sym is a free variable this body reads from an
// enclosing scope rather than a value the body's own flow determines. The shared
// binding table gives a captured variable the same id its defining scope assigns,
// but this graph's scope tracker classifies it: a symbol DECLARED as a local in
// this body (SymbolLocal) carries its value through the transfer (its absent Env
// value is undetermined flow, recovered by the assignment that defines it), so it
// is NOT a capture. A parameter is seeded at entry, also not a capture. Every other
// classification — an upvalue, or a captured outer local the tracker resolves to
// the enclosing scope (recorded here as a non-local) — is a free variable whose
// type is the module-wide value its defining scope holds.
func valueIsBottom(v product.AbstractValue) bool {
	return v.IsZero() || product.Domain.Equal(v, product.Domain.Bottom())
}

// demandParamCtx records that a parameter used in a typing context (where its
// type is pinned by the operation, e.g. an arithmetic operand pinned to number)
// must supply that context type. It is the contextual half of parameter demand:
// it fires even when the parameter currently has no value, because the use site
// itself constrains the parameter regardless of its inferred value.
func (t *Transfer) demandParamCtx(expr ast.Expr, ctx typ.Type, demand func(int, paramevidence.ParamContract)) {
	if demand == nil || ctx == nil {
		return
	}
	ident, ok := expr.(*ast.IdentExpr)
	if !ok {
		return
	}
	sym := t.symbolOf(ident)
	if sym == 0 {
		return
	}
	if idx, isParam := t.paramBySym[sym]; isParam {
		demand(idx, paramevidence.DemandFromType(ctx))
	}
}

func stringableContextType() typ.Type {
	return typ.NewUnion(typ.String, typ.Number)
}

func lengthContextType() typ.Type {
	return typ.NewUnion(
		typ.String,
		typ.NewArray(typ.Any),
		typ.NewMap(typ.Any, typ.Any),
		typ.NewReadonlyMap(typ.Any, typ.Any),
		typ.NewRecord().SetOpen(true).Build(),
	)
}

func orderableContextType() typ.Type {
	return typ.NewUnion(typ.String, typ.Number)
}

func orderedComparisonContexts(left, right typ.Type) (typ.Type, typ.Type) {
	if family := concreteOrderFamily(right); family != nil {
		return family, family
	}
	if family := concreteOrderFamily(left); family != nil {
		return family, family
	}
	orderable := orderableContextType()
	return orderable, orderable
}

func (t *Transfer) demandOrderedComparisonCtx(
	out *flow.PointState,
	lhs, rhs ast.Expr,
	demand func(int, paramevidence.ParamContract),
) {
	if family := concreteOrderFamily(t.operandType(out, rhs, demand)); family != nil {
		t.demandExprCtx(out, lhs, family, demand)
		t.demandExprCtx(out, rhs, family, demand)
		return
	}
	if family := concreteOrderFamily(t.operandType(out, lhs, demand)); family != nil {
		t.demandExprCtx(out, lhs, family, demand)
		t.demandExprCtx(out, rhs, family, demand)
		return
	}
	t.demandExprCapabilityCtx(out, lhs, paramevidence.CapabilityOrderable, demand)
	t.demandExprCapabilityCtx(out, rhs, paramevidence.CapabilityOrderable, demand)
}

func concreteOrderFamily(t typ.Type) typ.Type {
	if t == nil {
		return nil
	}
	t = typ.UnwrapAnnotated(t)
	if lit, ok := t.(*typ.Literal); ok {
		switch lit.Base {
		case kind.Integer, kind.Number:
			return typ.Number
		case kind.String:
			return typ.String
		}
		return nil
	}
	switch t.Kind() {
	case kind.Integer, kind.Number:
		return typ.Number
	case kind.String:
		return typ.String
	default:
		return nil
	}
}

// evalBinary resolves an arithmetic operator over its operand values. With an
// operator resolver it uses the resolved result; otherwise it uses the shared
// pure query-core operator reducer. The final numeric fallback is only for
// cases the shared reducer cannot classify.
func (t *Transfer) evalBinary(
	out *flow.PointState,
	op string,
	lhs, rhs ast.Expr,
	demand func(int, paramevidence.ParamContract),
) (product.AbstractValue, bool) {
	// An arithmetic operand is pinned to number; a parameter operand demands it.
	t.demandParamCtx(lhs, typ.Number, demand)
	t.demandParamCtx(rhs, typ.Number, demand)
	l, lok := t.evalExpr(out, lhs, demand)
	r, rok := t.evalExpr(out, rhs, demand)
	if t.ops != nil && lok && rok {
		res := t.ops.BinaryOp(l.ProjectValue(), op, r.ProjectValue())
		if res != nil && !typ.IsAbsentOrUnknown(res) {
			return product.FromType(res), true
		}
	}
	if lok && rok {
		res := querycore.BinaryOp(l.ProjectValue(), op, r.ProjectValue())
		if res != nil && !typ.IsAbsentOrUnknown(res) {
			return product.FromType(res), true
		}
	}
	if isNumeric(l, lok) && isNumeric(r, rok) {
		return product.FromType(typ.Number), true
	}
	return product.AbstractValue{}, false
}

// evalConcat materializes the value of `lhs .. rhs` only when the shared operator
// resolver proves both operands participate in Lua concatenation. Dynamic
// placeholder atoms (`any`, `unknown`) intentionally project to unknown in the
// query layer, so they do not become string evidence here; diagnostics and typed
// sinks then force a cast, assertion, or narrowing proof at the boundary.
func (t *Transfer) evalConcat(
	out *flow.PointState,
	lhs, rhs ast.Expr,
	demand func(int, paramevidence.ParamContract),
) (product.AbstractValue, bool) {
	t.demandExprCapabilityCtx(out, lhs, paramevidence.CapabilityStringable, demand)
	t.demandExprCapabilityCtx(out, rhs, paramevidence.CapabilityStringable, demand)
	t.demandConditionReads(out, lhs, demand)
	t.demandConditionReads(out, rhs, demand)
	l, lok := t.evalExpr(out, lhs, demand)
	r, rok := t.evalExpr(out, rhs, demand)
	if t.ops == nil || !lok || !rok {
		return product.AbstractValue{}, false
	}
	res := t.ops.BinaryOp(l.ProjectValue(), "..", r.ProjectValue())
	if res == nil || typ.IsAbsentOrUnknown(res) {
		return product.AbstractValue{}, false
	}
	return product.FromType(res), true
}

// evalUnary resolves a unary operator over its operand value.
func (t *Transfer) evalUnary(
	out *flow.PointState,
	op string,
	operand ast.Expr,
	demand func(int, paramevidence.ParamContract),
) (product.AbstractValue, bool) {
	if op == "-" {
		t.demandParamCtx(operand, typ.Number, demand)
	} else if op == "#" {
		t.demandExprCapabilityCtx(out, operand, paramevidence.CapabilityLength, demand)
	}
	v, ok := t.evalExpr(out, operand, demand)
	if t.ops != nil && ok {
		res := t.ops.UnaryOp(op, v.ProjectValue())
		if res != nil && !typ.IsAbsentOrUnknown(res) {
			return product.FromType(res), true
		}
	}
	if ok {
		res := querycore.UnaryOp(op, v.ProjectValue())
		if res != nil && !typ.IsAbsentOrUnknown(res) {
			return product.FromType(res), true
		}
	}
	if op == "-" && isNumeric(v, ok) {
		return product.FromType(typ.Number), true
	}
	return product.AbstractValue{}, false
}

// evalLogical types a short-circuit logical expression (`a and b`, `a or b`)
// through the shared logical-op semantics (ops.LogicalAndTyped /
// ops.LogicalOrTyped). The result is the join of the surviving operand values:
// for `or` the truthy part of the left joined with the right (so `prefix or "["`
// yields the left's truthy type or the default), for `and` the falsy part of the
// left joined with the right. Operand types resolve through operandType, which
// falls back to gradual `any` for an unannotated parameter so a default pattern
// over an undeclared parameter still types rather than dropping to unknown.
func (t *Transfer) evalLogical(
	out *flow.PointState,
	e *ast.LogicalOpExpr,
	demand func(int, paramevidence.ParamContract),
) (product.AbstractValue, bool) {
	left := t.operandType(out, e.Lhs, demand)
	right := t.operandType(out, e.Rhs, demand)
	var res typ.Type
	switch e.Operator {
	case "and":
		res = ops.LogicalAndTyped(left, right)
	case "or":
		res = ops.LogicalOrTyped(left, right)
	default:
		return product.AbstractValue{}, false
	}
	if res == nil || typ.IsUnknown(res) {
		return product.AbstractValue{}, false
	}
	return product.FromType(res), true
}

// evalRelational types a comparison expression. Equality is total in Lua and can
// always be materialized as boolean. Ordered comparisons are partial operations:
// they yield boolean only when the shared operator query proves the operands are
// comparable, so `any < 1` does not become boolean evidence until a cast,
// assertion, or narrowing proves the ordered family.
func (t *Transfer) evalRelational(
	out *flow.PointState,
	e *ast.RelationalOpExpr,
	demand func(int, paramevidence.ParamContract),
) (product.AbstractValue, bool) {
	t.demandConditionReads(out, e.Lhs, demand)
	t.demandConditionReads(out, e.Rhs, demand)
	switch e.Operator {
	case "==", "~=":
		return product.FromType(typ.Boolean), true
	case "<", "<=", ">", ">=":
		t.demandOrderedComparisonCtx(out, e.Lhs, e.Rhs, demand)
	default:
		return product.AbstractValue{}, false
	}
	l, lok := t.evalExpr(out, e.Lhs, demand)
	r, rok := t.evalExpr(out, e.Rhs, demand)
	if t.ops == nil || !lok || !rok {
		return product.AbstractValue{}, false
	}
	res := t.ops.BinaryOp(l.ProjectValue(), e.Operator, r.ProjectValue())
	if res == nil || typ.IsAbsentOrUnknown(res) {
		return product.AbstractValue{}, false
	}
	return product.FromType(typ.Boolean), true
}

// evalVararg types the vararg expression (`...`) as its element type: the function's
// declared vararg element resolved through the function signature, or gradual `any`
// when the vararg is undeclared. The function literal's signature is built by the
// funcTyper from the declared annotations, so `function f(...: number)` types `...`
// as number. In a single-value context the vararg is its element type; a spread of it
// (`{...}`, `f(...)`) is handled by the consumer (table/call typing).
func (t *Transfer) evalVararg() (product.AbstractValue, bool) {
	if elem := t.varargElem(); elem != nil {
		return product.FromType(elem), true
	}
	return product.AbstractValue{}, false
}

// varargElem resolves the function's declared vararg element type from its signature
// (the funcTyper-built typ.Function.Variadic). A function with no funcTyper, no vararg,
// or an unresolved element yields nil so the consumer keeps the sound carry-forward.
func (t *Transfer) varargElem() typ.Type {
	if t.funcTyper == nil || t.in.Graph == nil {
		return nil
	}
	fn := t.in.Graph.Func()
	if fn == nil || fn.ParList == nil || !fn.ParList.HasVargs {
		return nil
	}
	sig := t.funcTyper.FuncType(fn)
	if sig == nil || sig.Variadic == nil || typ.IsAbsentOrUnknown(sig.Variadic) {
		return nil
	}
	return sig.Variadic
}

// operandType resolves an expression's value type for a typ.Type operator
// semantics query (logical and/or). It is the value-domain projection of
// evalExpr, with one refinement: an unannotated parameter the body has
// not pinned resolves to gradual `any` rather than nil, so a default pattern over an
// undeclared parameter (`prefix or "["`) joins against a usable left operand instead
// of an undetermined one. A determined value projects normally; an undetermined,
// non-parameter expression yields nil (the operator's nil-left handling applies).
func (t *Transfer) operandType(
	out *flow.PointState,
	expr ast.Expr,
	demand func(int, paramevidence.ParamContract),
) typ.Type {
	if av, ok := t.evalExpr(out, expr, demand); ok && !av.IsZero() {
		if pt := av.ProjectValue(); pt != nil && !typ.IsUnknown(pt) {
			return pt
		}
	}
	// A read whose root is a genuinely-gradual source — an unannotated parameter, or
	// a field/index read off one — is gradual `any`, not nil/unknown. `args.url` (the
	// field of an untyped `args`) carries `any` exactly as a bare `args` read does, so
	// a default pattern over it (`args.url or d`) joins against a usable `any` left
	// operand rather than collapsing to the right default. Without this the operand
	// resolves to nil and the `or` drops the left, losing the gradual top that an
	// untyped source must flow to a typed sink for the consistency check to fire.
	if t.gradualAnySource(out, expr) {
		return typ.Any
	}
	return nil
}

// gradualAnySource reports whether expr is a read whose value is a genuine gradual
// `any` — a value from an UNTYPED Lua source that flows dynamically. A bare read of
// an unannotated parameter is the base case; a field or index read off such a source
// is itself gradual `any` (Lua admits any field of a dynamic value as dynamic), so
// `args.url` over an untyped `args` is `any`. A logical operand chain (`a and b`,
// `a or b`) is gradual when its surviving operand is — `(args and args.url)` is `any`
// because both arms are gradual. The judgment is rooted in the symbol's declared-ness,
// not a name match: an annotated parameter, an annotated local, or a resolved value is
// NOT a gradual source, so this never over-admits a typed read.
func (t *Transfer) gradualAnySource(out *flow.PointState, expr ast.Expr) bool {
	switch e := expr.(type) {
	case *ast.IdentExpr:
		sym := t.symbolOf(e)
		if sym == 0 {
			return false
		}
		if t.unannotatedParam[sym] {
			return true
		}
		// A symbol whose tracked product value carries gradual-top evidence is itself
		// a gradual source. A strict declared `any` projects to typ.Any too, but has no
		// such evidence and must not be admitted as gradual.
		if av, ok := t.symbolValue(out, sym); ok && !av.IsZero() {
			if av.IsGradualTop() {
				return true
			}
		}
		return false
	case *ast.AttrGetExpr:
		// A field/index read resolves to a determined value when the base is tracked;
		// only treat it as gradual when the base itself is a gradual source AND the read
		// did not resolve to a concrete value (handled by the caller: this path is taken
		// only after evalExpr failed to determine the read).
		return t.gradualAnySource(out, e.Object)
	case *ast.LogicalOpExpr:
		// `a and b` survives to b on the truthy path, `a or b` to a on the truthy path;
		// a logical whose relevant operand is gradual is gradual. Both `(args and
		// args.url)` arms are gradual, so the result is gradual `any`.
		return t.gradualAnySource(out, e.Lhs) || t.gradualAnySource(out, e.Rhs)
	default:
		return false
	}
}

// resolveExprType resolves an expression's value type against the live Env for the
// driver's callee/receiver/iterator-source resolution. It projects a determined
// value, and otherwise falls back to gradual `any` for a read of an unannotated
// parameter (the same gradual-source projection operandType applies): an
// unannotated parameter is a genuinely-gradual source, so a callee/iterator over it
// is gradual `any`, not strict unknown. It returns the value-domain unknown when no
// value resolves and the expression is not a gradual-parameter read, so the driver
// falls back to its module-wide signatures and globals.
func (t *Transfer) resolveExprType(
	out *flow.PointState,
	e ast.Expr,
	demand func(int, paramevidence.ParamContract),
) typ.Type {
	if e == nil {
		return typ.Unknown
	}
	return t.exprTypeResolver(t.exprValueResolver(out, demand))(e)
}

func (t *Transfer) resolveExprValue(
	out *flow.PointState,
	e ast.Expr,
	demand func(int, paramevidence.ParamContract),
) (product.AbstractValue, bool) {
	if e == nil {
		return product.AbstractValue{}, false
	}
	if av, ok := t.evalExpr(out, e, demand); ok && !av.IsZero() {
		return av, true
	}
	if ident, ok := e.(*ast.IdentExpr); ok {
		if sym := t.symbolOf(ident); sym != 0 && t.unannotatedParam[sym] {
			return product.GradualAny(), true
		}
	}
	return product.AbstractValue{}, false
}

// applyNumeric updates the relational numeric component for an assignment to the
// integer-valued local named by key.
//
//   - `key = <int literal>` seeds the slot's exact value.
//   - `key = key + c` (a constant self-increment, the loop-counter shape) sets
//     the slot to the prior value plus c, read from the incoming bound. Because
//     the loop-header join keeps the loop-entry arm, the merged upper bound
//     strictly ascends each iteration (the deadlock shape); the value-domain
//     numeric Widen at the loop-header feedback-vertex set then cuts the
//     unbounded ascent to Top, which is what makes the flow engine terminate
//     where a widen-free SCC solve would not.
//
// Other numeric assignments leave the component untouched (the slot's prior
// relation is dropped only when it is overwritten, which ApplyEqConst does).
func (t *Transfer) applyNumeric(out *flow.PointState, sym cfg.SymbolID, src ast.Expr) {
	if out == nil {
		return
	}
	pk, ok := flow.NumericVarKeyOfSymbol(sym)
	if !ok {
		return
	}
	if c, ok := t.constInt(src); ok {
		op, ok := flow.NumericVarEqConstSymbolOp(sym, c)
		if !ok {
			return
		}
		flow.ApplyNumericEffect(out, flow.NumericEffect{
			Ops:             []flow.NumericOp{op},
			RequireExisting: true,
		})
		return
	}
	arith, ok := src.(*ast.ArithmeticOpExpr)
	if !ok || arith.Operator != "+" {
		return
	}
	delta, self := t.constIncrement(arith, sym)
	if !self || delta == 0 {
		return
	}
	if out.Num == nil {
		return
	}
	_, prevUpper, bounded := out.Num.BoundsFor(pk)
	if !bounded {
		return
	}
	// An incoming upper bound at the MaxInt64 sentinel is unbounded-above (e.g.
	// after the loop-header widen cut the ascending counter to Top). Incrementing
	// an unbounded value stays unbounded; computing prevUpper+delta would overflow
	// int64 and wrap to a spurious finite bound. When the increment would overflow,
	// leave the slot unbounded rather than apply the wrapped value.
	if (delta > 0 && prevUpper > math.MaxInt64-delta) || (delta < 0 && prevUpper < math.MinInt64-delta) {
		return
	}
	op, ok := flow.NumericVarEqConstSymbolOp(sym, prevUpper+delta)
	if !ok {
		return
	}
	flow.ApplyNumericEffect(out, flow.NumericEffect{
		Ops:             []flow.NumericOp{op},
		RequireExisting: true,
	})
}

// constIncrement reports whether arith is `sym + const` or `const + sym` (the
// self-increment of the counter named by sym) and the constant delta.
func (t *Transfer) constIncrement(arith *ast.ArithmeticOpExpr, sym cfg.SymbolID) (int64, bool) {
	if c, ok := t.constInt(arith.Rhs); ok && t.isSymExpr(arith.Lhs, sym) {
		return c, true
	}
	if c, ok := t.constInt(arith.Lhs); ok && t.isSymExpr(arith.Rhs, sym) {
		return c, true
	}
	return 0, false
}

func (t *Transfer) constInt(expr ast.Expr) (int64, bool) {
	num, ok := expr.(*ast.NumberExpr)
	if !ok {
		return 0, false
	}
	if lit, ok := literal.FromExpr(num); ok && lit.Base == kind.Integer {
		if v, isInt := lit.Value.(int64); isInt {
			return v, true
		}
	}
	return 0, false
}

func (t *Transfer) isSymExpr(expr ast.Expr, sym cfg.SymbolID) bool {
	ident, ok := expr.(*ast.IdentExpr)
	if !ok {
		return false
	}
	return sym != 0 && sym == t.symbolOf(ident)
}

// applyBranch records the parameter demand a branch condition imposes. Guard
// facts themselves belong to the successor edge chosen by the branch, not to the
// branch node's shared out-state: before an edge is selected, neither the guard
// nor its negation holds unconditionally. NarrowEdge (narrow.go) installs the
// true-edge guard or false-edge complement before the predecessor join.
func (t *Transfer) applyBranch(
	out *flow.PointState,
	info *cfg.BranchInfo,
	demand func(int, paramevidence.ParamContract),
) {
	t.demandConditionReads(out, info.Condition, demand)
}

// applyReturn records demand for parameters read in return expressions and
// stores each returned expression's evaluated value under a return-slot Env key.
// The summary projection reads those slots so the function's return tuple carries
// the value of a non-identifier return (a literal, an arithmetic result, a call)
// rather than the value-domain Top.
func (t *Transfer) applyReturn(
	out *flow.PointState,
	p cfg.Point,
	info *cfg.ReturnInfo,
	demand func(int, paramevidence.ParamContract),
) {
	effect := ReturnEffect{Relations: flow.ReturnRelationsDomain.Top()}
	if info == nil {
		t.applyReturnEffect(out, effect)
		return
	}
	if len(info.Exprs) == 1 {
		if call := info.SourceCallAt(0); call != nil && call.Call != nil {
			effect.Relations = t.callReturnRelations(out, call.Call, demand)
			t.applySetMetatablePrototypeSelf(out, p, call.Call, demand)
			if returns, ok := t.evalCall(out, call.Call, demand); ok && len(returns) > 0 {
				for i, val := range returns {
					slot := ReturnSlotEffect{Index: i, Value: val}
					if i == 0 {
						slot.Source = call.Call
					}
					if tree, ok := t.returnSetMetatableFunctionRefTree(p, call.Call); ok {
						slot.FunctionRefTree = tree
						slot.HasFunctionRefTree = true
					}
					effect.Slots = append(effect.Slots, slot)
				}
				t.applyReturnEffect(out, effect)
				return
			}
		}
	}
	for i, expr := range info.Exprs {
		t.demandConditionReads(out, expr, demand)
		slot := ReturnSlotEffect{Index: i, Source: expr}
		if call := nestedCallExpr(expr); call != nil {
			t.applySetMetatablePrototypeSelf(out, p, call, demand)
			if tree, ok := t.returnSetMetatableFunctionRefTree(p, call); ok {
				slot.FunctionRefTree = tree
				slot.HasFunctionRefTree = true
			}
		}
		t.publishReturnedPrototypeSelf(out, expr)
		// A returned identifier already carries its value in the variable's Env
		// slot, which the projection reads directly; only stash non-identifier
		// return values, whose value lives nowhere else.
		if _, isIdent := expr.(*ast.IdentExpr); !isIdent {
			if val, ok := t.evalExpr(out, expr, demand); ok && !val.IsZero() {
				slot.Value = val
			}
		}
		effect.Slots = append(effect.Slots, slot)
	}
	t.applyReturnEffect(out, effect)
}

func (t *Transfer) returnSetMetatableFunctionRefTree(p cfg.Point, call *ast.FuncCallExpr) (flow.FunctionRefTree, bool) {
	if call == nil || t.in.Graph == nil || !metatable.IsSetMetatableCall(call, t.in.Graph.Bindings()) {
		return flow.FunctionRefTree{}, false
	}
	proto, ok := t.setMetatablePrototype(p, call)
	if !ok || proto == 0 {
		return flow.FunctionRefTree{}, false
	}
	return t.prototypeMethodRefTree(proto)
}

func (t *Transfer) publishReturnedPrototypeSelf(out *flow.PointState, expr ast.Expr) {
	if out == nil || expr == nil {
		return
	}
	sym := t.returnedPrototypeInstanceSymbol(expr)
	if sym == 0 {
		return
	}
	protos, ok := out.PrototypeInstances.Prototypes(sym)
	if !ok || len(protos) == 0 {
		return
	}
	current, ok := t.symbolValue(out, sym)
	if !ok || current.IsZero() {
		return
	}
	for _, proto := range protos {
		if proto == 0 {
			continue
		}
		flow.RecordPrototypeSelf(out, proto, current)
	}
}

func (t *Transfer) returnedPrototypeInstanceSymbol(expr ast.Expr) cfg.SymbolID {
	switch e := expr.(type) {
	case *ast.IdentExpr:
		return t.symbolOf(e)
	case *ast.CastExpr:
		return t.returnedPrototypeInstanceSymbol(e.Expr)
	default:
		return 0
	}
}

func nestedCallExpr(expr ast.Expr) *ast.FuncCallExpr {
	switch e := expr.(type) {
	case *ast.FuncCallExpr:
		return e
	case *ast.CastExpr:
		return nestedCallExpr(e.Expr)
	default:
		return nil
	}
}

func (t *Transfer) constResolverAt(p cfg.Point) func(string) *flow.ConstValue {
	if t == nil || t.in.Graph == nil || len(t.in.ConstValues) == 0 {
		return nil
	}
	return func(name string) *flow.ConstValue {
		sym, ok := t.in.Graph.SymbolAt(p, name)
		if !ok || sym == 0 {
			if bindings := t.in.Graph.Bindings(); bindings != nil {
				symbols := bindings.SymbolsByName(name)
				if len(symbols) == 1 {
					sym = symbols[0]
				}
			}
			if sym == 0 {
				return nil
			}
		}
		at := t.in.ConstValues[sym]
		if at == nil {
			return nil
		}
		val := at[p]
		if val == nil || val.Kind == flow.ConstUnknown {
			return nil
		}
		return val
	}
}

// applyCallArgs records demand for parameters passed as call arguments and read
// inside the callee position, and applies the continuation narrowing an assert
// imposes. It reports dead=true when an assert proves its continuation unreachable
// (assert of an always-false condition), so the caller terminates the flow.
func (t *Transfer) applyCallArgs(
	out *flow.PointState,
	p cfg.Point,
	info *cfg.CallInfo,
	demand func(int, paramevidence.ParamContract),
) (dead bool) {
	if info.Call == nil {
		return false
	}
	t.demandConditionReads(out, info.Call.Receiver, demand)
	for _, arg := range info.Call.Args {
		t.demandConditionReads(out, arg, demand)
	}
	t.applyTableInsert(out, info, demand)
	// A type-cast/assertion call `T(arg)` proves its argument IS T on the post-call
	// path (a failed cast raises). Narrow the argument value to T, so a later read of
	// it observes the asserted type — the same continuation refinement an assert imposes.
	t.applyTypeCastNarrow(out, p, info.Call, demand)
	// assert(cond, ...) narrows its asserted value in the continuation (the value
	// holds truthy past the call, or the call raised). It is recognized by the
	// CalleeName the CFG pre-extracted, the genuine Lua builtin, not a name heuristic.
	if info.CalleeName == "assert" {
		return t.applyAssertNarrow(out, info.Call)
	}
	// A statement call to a no-return function terminates the live flow: its
	// continuation is unreachable, exactly as the builtin error() prunes its
	// successor. The post-state is the lattice Bottom, so the branch arm holding
	// this call drops out of the post-guard merge.
	if t.callTyper != nil {
		ctx := t.productCallContext(out, info.Call, demand)
		result := t.productCallResult(info.Call, ctx)
		if result.NeverReturns {
			return true
		}
		t.applyCallArgDemands(out, info.Call, result.ArgDemands, demand)
		t.applyCallResultEffects(out, info.Call, ctx, result.Effects, demand)
		// A call to a function that narrows its parameters (a wrapper around assert /
		// `if x == nil then error()`) carries that narrowing to the matching argument
		// here, so `check(y); use(y)` sees `y` narrowed.
		if len(result.ParamNarrows) > 0 && t.ApplyParamNarrowsAtPoint(out, p, info.Call, result.ParamNarrows) {
			return true
		}
	}
	return false
}

// applyTypeCastNarrow narrows the argument of a type-cast/assertion call `T(arg)` to
// the asserted type T in out. A cast raises on a value that is not T, so on the
// continuation the argument provably IS T — narrowing it to T is the same sound
// continuation refinement an assert imposes, just to a concrete type. It applies to a
// bare-symbol or static field-path argument the transfer tracks; the argument's Env
// slot (or the field path within it) is rewritten to T. A non-cast call, or an
// argument the transfer does not track, leaves out unchanged.
func (t *Transfer) applyTypeCastNarrow(
	out *flow.PointState,
	p cfg.Point,
	call *ast.FuncCallExpr,
	demand func(int, paramevidence.ParamContract),
) {
	if t.callTyper == nil || call == nil || call.Method != "" || len(call.Args) != 1 {
		return
	}
	exprType := t.projectExprTypeResolver(out)
	target, ok := t.callTyper.TypeCastTarget(call, exprType)
	if !ok || target == nil || typ.IsAbsentOrUnknown(target) {
		return
	}
	sym, segs, ok := t.pathSymbolInStateAt(out, p, call.Args[0], demand)
	if !ok || sym == 0 {
		return
	}
	place, ok := staticPlace(sym, segs)
	if !ok {
		return
	}
	t.applyRefinementEffect(out, RefinementEffect{
		Place:     place,
		Kind:      RefinementTypeCast,
		Target:    target,
		PreferEnv: true,
	})
}

// demandConditionReads walks an expression for identifier reads, emitting
// parameter demand for each parameter read. Operator nodes also emit the same
// operand contracts evalExpr would emit in a value-producing context, so a use
// that appears only in a branch condition (for example `#xs == 0`) still feeds
// the entry contract cell. It does not mutate value state.
func (t *Transfer) demandConditionReads(
	out *flow.PointState,
	expr ast.Expr,
	demand func(int, paramevidence.ParamContract),
) {
	if expr == nil || demand == nil {
		return
	}
	switch e := expr.(type) {
	case *ast.IdentExpr:
		t.evalIdent(out, e, demand)
	case *ast.AttrGetExpr:
		t.demandConditionReads(out, e.Object, demand)
		t.demandConditionReads(out, e.Key, demand)
	case *ast.FuncCallExpr:
		t.demandConditionReads(out, e.Func, demand)
		t.demandConditionReads(out, e.Receiver, demand)
		for _, arg := range e.Args {
			t.demandConditionReads(out, arg, demand)
		}
	case *ast.ArithmeticOpExpr:
		t.demandExprCtx(out, e.Lhs, typ.Number, demand)
		t.demandExprCtx(out, e.Rhs, typ.Number, demand)
		t.demandConditionReads(out, e.Lhs, demand)
		t.demandConditionReads(out, e.Rhs, demand)
	case *ast.RelationalOpExpr:
		switch e.Operator {
		case "<", "<=", ">", ">=":
			t.demandOrderedComparisonCtx(out, e.Lhs, e.Rhs, demand)
		}
		t.demandConditionReads(out, e.Lhs, demand)
		t.demandConditionReads(out, e.Rhs, demand)
	case *ast.LogicalOpExpr:
		t.demandConditionReads(out, e.Lhs, demand)
		t.demandConditionReads(out, e.Rhs, demand)
	case *ast.StringConcatOpExpr:
		t.demandExprCapabilityCtx(out, e.Lhs, paramevidence.CapabilityStringable, demand)
		t.demandExprCapabilityCtx(out, e.Rhs, paramevidence.CapabilityStringable, demand)
		t.demandConditionReads(out, e.Lhs, demand)
		t.demandConditionReads(out, e.Rhs, demand)
	case *ast.UnaryMinusOpExpr:
		t.demandExprCtx(out, e.Expr, typ.Number, demand)
		t.demandConditionReads(out, e.Expr, demand)
	case *ast.UnaryNotOpExpr:
		t.demandConditionReads(out, e.Expr, demand)
	case *ast.UnaryLenOpExpr:
		t.demandExprCapabilityCtx(out, e.Expr, paramevidence.CapabilityLength, demand)
		t.demandConditionReads(out, e.Expr, demand)
	}
}

// symbolOf resolves an identifier to its symbol via the graph's binding table.
func (t *Transfer) symbolOf(e *ast.IdentExpr) cfg.SymbolID {
	if e == nil || t.in.Graph == nil {
		return 0
	}
	bindings := t.in.Graph.Bindings()
	if bindings == nil {
		return 0
	}
	sym, ok := bindings.SymbolOf(e)
	if !ok {
		return 0
	}
	return sym
}

// isNumeric reports whether a determined value projects to a numeric type.
func isNumeric(v product.AbstractValue, ok bool) bool {
	if !ok || v.IsZero() {
		return false
	}
	pt := v.ProjectValue()
	if pt == nil {
		return false
	}
	switch pt {
	case typ.Number, typ.Integer:
		return true
	}
	if lit, isLit := pt.(*typ.Literal); isLit {
		return lit.Base == kind.Integer || lit.Base == kind.Number
	}
	return false
}
