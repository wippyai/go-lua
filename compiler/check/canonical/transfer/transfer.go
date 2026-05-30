// Package transfer is the real per-node transfer of the canonical
// intraprocedural solver: the NodeTransfer the equation graph injects.
//
// It implements equation.NodeTransfer by interpreting one CFG node's syntactic
// evidence against the incoming flow.PointState and returning the post-node
// state. It carries no fixed-point driver and no Solution-style mutable store:
// the equation.Builder owns the worklist, the predecessor join, and the
// widening; this transfer is a pure function of (point, incoming, contracts).
//
// The per-node semantics are the sound value/condition/numeric effects of the
// legacy types/flow/transfer.go Solution methods, lifted off that driver and
// expressed directly over the canonical PointState domains:
//
//   - value: the Env maps a variable's symbol key to its product.AbstractValue.
//     A local declaration or assignment writes the source expression's value;
//     reads project the stored value. Joins and widens are the domain's
//     (product.Domain via the Env MapLattice), so a loop that accumulates a
//     growing type converges by the value-domain ACC widening at the loop-header
//     feedback-vertex set — the exact termination the legacy widen-free runSCC
//     lacks.
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
	"os"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/abstract/literal"
	"github.com/wippyai/go-lua/compiler/check/canonical/input"
	"github.com/wippyai/go-lua/compiler/check/domain/paramevidence"
	"github.com/wippyai/go-lua/compiler/check/synth/ops"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/flow/numeric"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// DeferredNodeKinds names the CFG node kinds and source forms this transfer does
// not yet interpret. Reaching one carries the incoming state forward unchanged
// (sound: precision loss, not unsoundness). Listed for the follow-up that
// extends coverage to the full legacy transfer.go surface.
//
// Implemented (no longer deferred): field/index writes (t.f = v, t[k] = v) and the
// read-back (product.WithField/WriteIndex/FieldOf/IndexOf over the Env);
// container-targeted function definitions (function M.f); table-constructor field
// typing; call-return typing through the bridged signatures (predeclared globals,
// recursive/forward function references); path-sensitive condition narrowing
// (x ~= nil, type(x) == k, x.kind == "tag") per branch edge via NarrowEdge
// (narrow.go).
var DeferredNodeKinds = []string{
	"container/map mutators other than table.insert (table.remove, channel.send)",
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

// CallTyper types a call or method-call expression's Lua return vector. It is the
// transfer's seam to the legacy call-typing machinery (ops.NewCallPipeline): the
// driver implements it by resolving the callee/receiver type (from the module's
// function signatures, predeclared globals, captured values, or the live Env via
// the supplied resolver) and running the call pipeline, which infers generic type
// arguments from the argument types, runs the cast/intercept chain, and produces
// the multi-return tuple. The transfer owns reading the argument and receiver
// values from the current Env; the CallTyper owns callee resolution and the
// pipeline. It is optional: a nil typer leaves a call result untyped (the sound
// carry-forward), so a slot assigned from a call drops to the value-domain Top.
//
// exprType resolves an expression's value type against the live point Env (the
// transfer's evalExpr): the driver reads it for the callee, the receiver, and any
// callee/receiver field path whose value the transfer tracks. It returns the
// value-domain unknown for an expression the transfer does not determine, so the
// driver falls back to its module-wide signatures/globals.
type CallTyper interface {
	CallReturns(call *ast.FuncCallExpr, argTypes []typ.Type, exprType func(ast.Expr) typ.Type) ([]typ.Type, bool)
	// IterVars types a generic-for loop's iteration variables from the loop's
	// iterator expression (`for i, v in ipairs(arr)`): it resolves the iterator
	// function's iteration effect (indexed/keyed) and the iterated container's
	// element/key/value types, returning one type per loop variable. count is the
	// loop-variable count. exprType resolves an argument/source expression against the
	// live Env. It returns false when the iterator is not a recognized iteration form,
	// so the transfer leaves the loop variables untyped (the sound carry-forward).
	IterVars(iter *ast.FuncCallExpr, count int, exprType func(ast.Expr) typ.Type) ([]typ.Type, bool)
	// ParamNarrows resolves the callee's parameter-narrowing effects: the
	// presence/truthy refinements the callee's body proves about its parameters on
	// every normal return (a wrapper around assert / `if x == nil then error()`).
	// The transfer applies them to the matching call arguments so `check(y)` narrows
	// `y`. A callee with no such effect, or one that does not resolve to a module
	// function, yields none.
	ParamNarrows(call *ast.FuncCallExpr) []ParamNarrow
	// IsNoReturn reports whether call's callee is a module function that never
	// returns normally (its body always raises — every exit path ends in error() or
	// a call to another no-return function). A statement call to such a function
	// terminates the live flow, exactly as the builtin error() does, so its
	// continuation is unreachable. A callee that does return, or one that does not
	// resolve to a module function, yields false.
	IsNoReturn(call *ast.FuncCallExpr) bool
}

// TypeCheckBind records the value-narrowing a `local val, err = T:is(x)` type-check
// assignment establishes: the symbols that the type guard proves carry the checked
// type T when the error result is nil (the `err == nil` / `val ~= nil` edge). It is
// the canonical counterpart of the legacy PredicateLink the assignment point-emitter
// builds for a Type:is(...) call: on the success edge the value symbols narrow to T,
// on the failure edge the error symbol is the diagnostic carrier.
//
// ErrSym is the assignment's error target (the second return of T:is). NarrowSyms
// are the symbols proved to be T on success (the checked argument and, when bound,
// the value target). Type is the resolved checked type T.
type TypeCheckBind struct {
	ErrSym     cfg.SymbolID
	NarrowSyms []cfg.SymbolID
	Type       typ.Type
}

// SiblingNilBind records the value/error correlation a multi-return assignment
// `local v, err = f()` establishes when the callee proves the Lua `(value, err)`
// inverse pattern (the body returns `(value, nil)` on success and `(nil, error)`
// on failure, or only `(value, nil)` with an optional value slot). On the edge a
// branch proves err nil, the correlated value symbols are non-nil, so they narrow
// by stripping nil from their flow value: a surviving `local v, err = f(); if err
// ~= nil then return end` then reads v as its non-optional type.
//
// ErrSym is the assignment's error target (the correlated error return). ValueSyms
// are the value targets the same call's inverse-correlated slot binds; on the
// err-nil edge each narrows by NarrowPresent over its current value, recovering the
// non-nil type without fabricating a fixed type the callee did not return.
type SiblingNilBind struct {
	ErrSym    cfg.SymbolID
	ValueSyms []cfg.SymbolID
}

// Transfer is the canonical per-node transfer. It is stateless across points:
// every Transfer call is a pure function of the incoming state and the node's
// evidence.
type Transfer struct {
	in input.Inputs
	// ops resolves arithmetic/relational operator result types. Nil falls back
	// to the structural default (arithmetic on numbers stays numeric).
	ops OperatorResolver
	// funcTyper resolves a function literal's declared signature, so a
	// function-valued table-literal field types as a callable. Nil leaves such a
	// field untyped (the sound carry-forward).
	funcTyper FuncTyper
	// callTyper types a call/method-call expression's return vector through the
	// legacy call pipeline. Nil leaves a call result untyped (the sound
	// carry-forward), so a slot assigned from a call drops to the value-domain Top.
	callTyper CallTyper
	// paramBySym maps a parameter's symbol ID to its parameter index, so a body
	// use of a parameter routes demand to the right contract cell.
	paramBySym map[cfg.SymbolID]int
	// typeCheckByErr maps a type-check assignment's error-result symbol to the
	// value narrowing the guard proves on the err == nil edge. NarrowEdge reads it
	// so a branch testing the error symbol narrows the checked value to the checked
	// type.
	typeCheckByErr map[cfg.SymbolID]TypeCheckBind
	// siblingNilByErr maps a multi-return assignment's error-result symbol to the
	// value symbols proven non-nil when that error is nil. NarrowEdge reads it so a
	// branch testing the error symbol strips nil from the correlated value symbols on
	// the err-nil edge.
	siblingNilByErr map[cfg.SymbolID]SiblingNilBind
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
	// inferredParamBySlot maps an UNANNOTATED parameter SLOT to the type inferred for
	// it from the module's call sites. seedEntry seeds an unannotated parameter from
	// it so the body sees the call-site type rather than the gradual any. A declared
	// parameter is unaffected (its annotation is authoritative).
	inferredParamBySlot map[int]typ.Type
	// captureType resolves the type of a free variable this body reads from an
	// enclosing scope (an upvalue / module-level capture) — a symbol with no Env
	// value here, that this function neither declares nor takes as a parameter. The
	// driver implements it by reading the capture's module-wide converged type
	// (the value its defining scope holds), so a body read of `renderer` (a local
	// of the enclosing builder captured into a returned closure) sees that type
	// rather than the value-domain unknown. Nil leaves a capture unresolved (the
	// sound carry-forward: a genuinely-unresolved capture stays unknown).
	captureType func(sym cfg.SymbolID) (typ.Type, bool)
}

// New builds the transfer for the given canonical inputs. ops, funcTyper, and
// callTyper may be nil. typeChecks are the type-check value-narrowing binds the
// caller precomputed from the graph's `local val, err = T:is(x)` assignments (nil
// for none). siblingNils are the (value, err) inverse-correlation binds the caller
// precomputed from the graph's `local v, err = f()` assignments whose callee proves
// the Lua error-return pattern (nil for none). declared maps an annotated symbol (parameter or annotated local) to
// its declared type, so edge narrowing refines the declared union rather than the
// precise constructor value the Env seeds; nil leaves narrowing on the Env value.
// selfType, when non-nil, is the receiver class of a method body's implicit `self`
// (function T:m()): it seeds the self parameter's entry value so self.field reads
// track the receiver record rather than collapsing to unknown.
func New(in input.Inputs, ops OperatorResolver, funcTyper FuncTyper, callTyper CallTyper, typeChecks []TypeCheckBind, siblingNils []SiblingNilBind, declared map[cfg.SymbolID]typ.Type, selfType typ.Type) *Transfer {
	t := &Transfer{in: in, ops: ops, funcTyper: funcTyper, callTyper: callTyper, paramBySym: make(map[cfg.SymbolID]int), declaredTypes: declared}
	t.declaredParamBySlot = declaredParamBySlot(in)
	// A method body's implicit `self` occupies slot 0 with no source annotation, so the
	// slot map has no entry for it; seed it from the resolved receiver class so the
	// entry state pins self to its record.
	if selfType != nil && len(in.Scope.ParamSymbols) > 0 {
		if t.declaredParamBySlot == nil {
			t.declaredParamBySlot = make(map[int]typ.Type, 1)
		}
		t.declaredParamBySlot[0] = selfType
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
	for _, b := range typeChecks {
		if b.ErrSym == 0 || b.Type == nil || len(b.NarrowSyms) == 0 {
			continue
		}
		if t.typeCheckByErr == nil {
			t.typeCheckByErr = make(map[cfg.SymbolID]TypeCheckBind, len(typeChecks))
		}
		t.typeCheckByErr[b.ErrSym] = b
	}
	for _, b := range siblingNils {
		if b.ErrSym == 0 || len(b.ValueSyms) == 0 {
			continue
		}
		if t.siblingNilByErr == nil {
			t.siblingNilByErr = make(map[cfg.SymbolID]SiblingNilBind, len(siblingNils))
		}
		t.siblingNilByErr[b.ErrSym] = b
	}
	return t
}

// SetInferredParams installs the call-site-inferred types of unannotated parameters
// after construction. Like SetSiblingNils, the inference needs the call graph
// resolved (a parameter's type comes from its callers' argument types), which is
// after the transfer is built, so the types are injected here.
func (t *Transfer) SetInferredParams(bySlot map[int]typ.Type) {
	if len(bySlot) == 0 {
		t.inferredParamBySlot = nil
		return
	}
	t.inferredParamBySlot = bySlot
}

// SetCaptureResolver installs the free-variable (upvalue / module-capture) type
// resolver. A nested function reads a captured variable under the same shared
// symbol id its enclosing scope assigns, but the captured value lives in no Env
// slot of this body; the resolver supplies the capture's module-wide converged
// type so the body's reads, the locals it feeds, and the records it builds carry
// that type rather than collapsing to unknown. Installed after the program is
// built (the capture type comes from the defining scope's solve, resolved through
// the same interprocedural query the call graph uses).
func (t *Transfer) SetCaptureResolver(resolve func(sym cfg.SymbolID) (typ.Type, bool)) {
	t.captureType = resolve
}

// SetSiblingNils installs the (value, err) inverse-correlation binds after
// construction. The driver computes them in a build phase that runs once the call
// graph's name resolution is complete (a sibling correlation needs the callee
// resolved to prove the error-return pattern), which is after the transfer is
// built, so they are injected here rather than passed to New.
func (t *Transfer) SetSiblingNils(siblingNils []SiblingNilBind) {
	t.siblingNilByErr = nil
	for _, b := range siblingNils {
		if b.ErrSym == 0 || len(b.ValueSyms) == 0 {
			continue
		}
		if t.siblingNilByErr == nil {
			t.siblingNilByErr = make(map[cfg.SymbolID]SiblingNilBind, len(siblingNils))
		}
		t.siblingNilByErr[b.ErrSym] = b
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

// ReturnSlotKey is the Env key under which applyReturn stashes the value of the
// i-th return expression at a return point. It is distinct from any symbol key
// (which is "s"+id), so it never collides with a variable's value. The summary
// projection reads it to recover the typed value of a non-identifier return.
func ReturnSlotKey(i int) string {
	return "r" + itoa(uint64(i))
}

// symKey is the Env key for a symbol: a stable per-function string identity. The
// CFG binds every variable occurrence to a SymbolID, so keying by symbol is
// sound and version-free within one function's point states (the per-point Env
// already carries the flow-sensitive value).
func symKey(sym cfg.SymbolID) string {
	return "s" + itoa(uint64(sym))
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
	out := clonePointState(incoming)

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
		if out.Num == nil || out.Num.IsUnsat() {
			out.Num = numeric.NewState()
		}
		t.seedEntry(&out, entryContracts)
	}

	switch info := g.Info(p).(type) {
	case *cfg.AssignInfo:
		t.applyAssign(&out, p, info, demand)
	case *cfg.BranchInfo:
		t.applyBranch(&out, info, demand)
	case *cfg.ReturnInfo:
		t.applyReturn(&out, info, demand)
	case *cfg.CallInfo:
		if dead := t.applyCallArgs(&out, info, demand); dead {
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

// seedEntry writes each parameter's entry value into Env: the declared type when
// annotated, refined by any demanded contract (the value a caller must supply).
func (t *Transfer) seedEntry(out *flow.PointState, contracts paramevidence.Contracts) {
	for i, sym := range t.in.Scope.ParamSymbols {
		if sym == 0 {
			continue
		}
		key := symKey(sym)
		var av product.AbstractValue
		if declared, ok := t.declaredParamBySlot[i]; ok && declared != nil {
			av = product.FromType(declared)
		} else if inferred, ok := t.inferredParamBySlot[i]; ok && inferred != nil {
			// An unannotated parameter takes its call-site-inferred type, so the body's
			// reads and narrowing of it have a concrete base rather than the gradual any.
			av = product.FromType(inferred)
		}
		if c, ok := contracts[i]; ok {
			if av.IsZero() {
				av = c
			} else {
				av = product.Domain.Join(av, c)
			}
		}
		if av.IsZero() {
			continue
		}
		out.Env[key] = av
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
	// A call source feeding more targets than sources expands to a multi-return
	// tuple: bind each target to the matching return slot (target i -> return i),
	// the Lua multi-assignment semantics (`local a, b = f()`). Resolved once here so
	// the per-target loop below reads the pre-typed return vector rather than
	// re-typing the call for every fed target.
	callReturns := t.callExpansionReturns(out, info, demand)
	info.EachTargetSource(func(i int, target cfg.AssignTarget, src ast.Expr) {
		if target.Kind == cfg.TargetField || target.Kind == cfg.TargetIndex {
			t.applyContainerWrite(out, target, src, demand)
			return
		}
		if target.Kind != cfg.TargetIdent || target.Symbol == 0 {
			return
		}
		val, ok := t.targetValue(out, info, i, src, callReturns, demand)
		if !ok {
			if src == nil {
				// A target with no aligned source and no expanding call return: a
				// parameter declaration node, or a multi-value tail slot whose producer
				// is deferred. The slot's value is established elsewhere (entry seeding
				// for a parameter), so leave it untouched rather than clobber it.
				return
			}
			// A declared keyed/indexed local (`local m: {[string]: string} = f()`) is
			// statically that container regardless of an unresolved initializer: the
			// annotation is the slot's authority (resolve.go's hierarchy: declared
			// overrides structural inference). Seed it so a later read/iteration of the
			// slot sees the declared element/key/value rather than collapsing to unknown.
			if dc, has := t.declaredContainerType(target.Symbol); has {
				out.Env[symKey(target.Symbol)] = product.FromType(dc)
				return
			}
			// Unknown source: clear any stale narrowing so the slot is the value
			// domain's Top (the most general value), never a stale precise type.
			delete(out.Env, symKey(target.Symbol))
			return
		}
		// A declared keyed/indexed local's slot carries its declared container type
		// rather than the initializer's narrower value: `local m: {[string]: string}
		// = {}` is a string-keyed map (so `pairs(m)` types its key/value), not the empty
		// closed record the `{}` constructor yields. The annotation is the authority for
		// a mutable container slot; narrowing operates over t.declaredTypes directly, so
		// the declared base here does not perturb per-edge discriminant refinement.
		if dc, has := t.declaredContainerType(target.Symbol); has {
			val = product.FromType(dc)
		}
		key := symKey(target.Symbol)
		if prev, had := out.Env[key]; had && !info.IsLocal {
			// A re-assignment (`x = ...`, not a `local` declaration) in a loop body
			// joins with the prior value: the loop-header widening then bounds the
			// accumulating chain (`x = x + 1`). A `local x = ...` REBINDS a fresh
			// variable each time it executes, so the loop-carried prior binding is
			// dead — overwriting it with this iteration's value is sound and avoids a
			// monotonic LUB that would re-admit a stale optionality the current
			// iteration's value (e.g. an in-bounds-refined arr[i]) no longer carries.
			out.Env[key] = product.Domain.Join(prev, val)
		} else {
			out.Env[key] = val
		}
		if src != nil {
			t.applyNumeric(out, key, src)
			t.seedArrayLiteralLength(out, key, src)
		}
	})
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
	case *typ.Map, *typ.Array:
		return declared, true
	}
	return nil, false
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

// targetValue resolves the value bound to target index i. A target aligned with a
// source expression is the source's value. A trailing target fed by an expanding
// call (`local a, b = f()`, where b has no aligned source) is the call's return at
// the matching slot.
func (t *Transfer) targetValue(
	out *flow.PointState,
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
		return t.evalExpr(out, src, demand)
	}
	return product.AbstractValue{}, false
}

// applyFuncDef types a container-targeted function definition (function M.add(),
// function M:add()) by writing the function's declared signature into the receiver
// container's field, so a later call M.add(...) resolves to the function rather
// than a missing field. A plain function definition (function f / local function f)
// binds a symbol directly and is typed by the bridge's function-signature map, so
// it is not handled here. A definition whose function type the typer cannot resolve
// leaves the container untouched (sound carry-forward).
func (t *Transfer) applyFuncDef(out *flow.PointState, info *cfg.FuncDefInfo) {
	if t.funcTyper == nil || info == nil || info.FuncExpr == nil {
		return
	}
	base := info.TargetPath.Symbol
	segs := info.TargetPath.Segments
	if base == 0 || len(segs) == 0 {
		return
	}
	path := make([]string, 0, len(segs))
	for _, seg := range segs {
		switch seg.Kind {
		case constraint.SegmentField, constraint.SegmentIndexString:
			if seg.Name == "" {
				return
			}
			path = append(path, seg.Name)
		default:
			// An integer-indexed function target is not a named field write.
			return
		}
	}
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
	baseKey := symKey(base)
	baseAV, had := out.Env[baseKey]
	if !had || baseAV.IsZero() {
		// The receiver container has no value the transfer tracks (an imported or
		// captured table whose type lives in the observation surface). Writing here
		// would clobber its real fields, so carry forward unchanged.
		return
	}
	out.Env[baseKey] = writeFieldPath(baseAV, path, product.FromType(fn))
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
	if target.BaseSymbol == 0 {
		// A write through a non-identifier base (e.g. f().x = v) has no symbol slot
		// to update; its container value lives nowhere this transfer tracks.
		return
	}
	if src == nil {
		return
	}
	val, ok := t.evalExpr(out, src, demand)
	if !ok || val.IsZero() {
		return
	}
	baseKey := symKey(target.BaseSymbol)
	base, had := out.Env[baseKey]
	if !had || base.IsZero() {
		// The base container has no value the transfer tracks: an imported module, a
		// captured variable, or a parameter whose type lives in the observation
		// surface, not the Env. Overwriting it here would fabricate a closed record
		// that drops the base's real fields and mask a genuine field. Carry forward
		// unchanged (sound: the observation surface keeps the base's actual type).
		return
	}

	switch target.Kind {
	case cfg.TargetField:
		if len(target.FieldPath) == 0 {
			return
		}
		out.Env[baseKey] = writeFieldPath(base, target.FieldPath, val)
	case cfg.TargetIndex:
		if name, isField := staticFieldName(target.Key); isField {
			out.Env[baseKey] = product.WithField(base, name, val)
			return
		}
		t.applyIndexWriteLength(out, target, baseKey)
		key, ok := t.evalExpr(out, target.Key, demand)
		if !ok || key.IsZero() {
			return
		}
		out.Env[baseKey] = product.WriteIndex(base, key, val)
	}
}

// writeFieldPath overlays val at the nested field path within base (the chain
// ["a","b"] for base.a.b = val), rebuilding each enclosing record so the innermost
// field write propagates outward. It reuses the value-domain product.FieldOf to
// read the intermediate container and product.WithField to write each level, so a
// missing intermediate field is created as a fresh record the way Lua admits a new
// nested key.
func writeFieldPath(base product.AbstractValue, path []string, val product.AbstractValue) product.AbstractValue {
	if len(path) == 0 {
		return base
	}
	if len(path) == 1 {
		return product.WithField(base, path[0], val)
	}
	child, ok := product.FieldOf(base, path[0])
	if !ok || child.IsZero() {
		child = product.FromType(typ.NewRecord().Build())
	}
	updated := writeFieldPath(child, path[1:], val)
	return product.WithField(base, path[0], updated)
}

// applyNumericFor types the control variable of a numeric for-loop
// (for i = init, limit[, step]). The loop body executes with the variable
// ranging over the integer interval the control expressions describe, so the
// variable's value is integer — the same type the legacy local-inference assigns
// the numeric-for induction variable. The relational numeric component seeds the
// variable at its integer init when that is a constant, so a body comparison sees
// a concrete numeric bound rather than the unbounded default.
func (t *Transfer) applyNumericFor(out *flow.PointState, info *cfg.AssignInfo) {
	target, ok := info.FirstTarget()
	if !ok || target.Kind != cfg.TargetIdent || target.Symbol == 0 {
		return
	}
	key := symKey(target.Symbol)
	out.Env[key] = product.FromType(typ.Integer)
	if out.Num != nil && info.NumericFor != nil {
		if c, ok := t.constInt(info.NumericFor.Init); ok {
			out.Num.ApplyEqConst(constraint.PathKey(key), c)
		}
		t.seedNumericForLength(out, target.Symbol, info.NumericFor)
	}
}

// applyGenericFor types the iteration variables of a generic for-loop
// (for i, v in ipairs(arr)) from the loop's iterator. It resolves the iterator
// function's iteration effect and the iterated container's element/key/value types
// through the CallTyper's IterVars seam (the same iteration-effect machinery the
// legacy synthesizer uses), then writes each loop variable's element type into its Env
// slot, joining with any prior value so the loop-header widening bounds the chain. An
// iterator the seam does not recognize leaves the variables untyped (sound carry-
// forward), the prior behavior.
func (t *Transfer) applyGenericFor(
	out *flow.PointState,
	info *cfg.AssignInfo,
	demand func(int, paramevidence.ParamContract),
) {
	for _, iter := range info.IterExprs {
		t.demandConditionReads(out, iter, demand)
	}
	iterCall, ok := info.IterExprs[0].(*ast.FuncCallExpr)
	if !ok || t.callTyper == nil {
		return
	}
	exprType := func(e ast.Expr) typ.Type {
		return t.resolveExprType(out, e, demand)
	}
	varTypes, ok := t.callTyper.IterVars(iterCall, len(info.Targets), exprType)
	if !ok || len(varTypes) == 0 {
		return
	}
	for i := range info.Targets {
		target := info.Targets[i]
		if target.Kind != cfg.TargetIdent || target.Symbol == 0 || i >= len(varTypes) {
			continue
		}
		vt := varTypes[i]
		if vt == nil || typ.IsAbsentOrUnknown(vt) {
			continue
		}
		key := symKey(target.Symbol)
		val := product.FromType(vt)
		if prev, had := out.Env[key]; had {
			out.Env[key] = product.Domain.Join(prev, val)
		} else {
			out.Env[key] = val
		}
	}
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
		// A concatenation reads both operands (parameter demand) and yields a string.
		t.demandConditionReads(out, e.Lhs, demand)
		t.demandConditionReads(out, e.Rhs, demand)
		return product.FromType(typ.String), true
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
		// #x is an integer; record the demand that x is read.
		t.evalExpr(out, e.Expr, demand)
		return product.FromType(typ.Integer), true
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

// evalCall types a call or method-call expression's Lua return vector through the
// CallTyper seam (the legacy ops.NewCallPipeline). It resolves the argument values
// from the current Env and emits parameter demand for arguments that are parameter
// reads, then routes the call through the CallTyper, which resolves the callee
// signature (module function, predeclared global, captured value, or live Env
// value), infers generic type arguments from the argument types, and produces the
// multi-return tuple. A slot per returned value carries its value-domain type. The
// receiver of a method call and any field-path callee are resolved through evalExpr
// against the live Env, so a method on a tracked local resolves its receiver type.
// Returns false when no CallTyper is wired or the callee does not resolve, so the
// caller drops the slot to the value-domain Top (sound: precision loss).
func (t *Transfer) evalCall(
	out *flow.PointState,
	call *ast.FuncCallExpr,
	demand func(int, paramevidence.ParamContract),
) ([]product.AbstractValue, bool) {
	if t.callTyper == nil || call == nil {
		return nil, false
	}
	// Emit parameter demand for argument and receiver reads, and resolve each
	// argument's value type for the call pipeline's generic inference and arity.
	t.demandConditionReads(out, call.Receiver, demand)
	argTypes := make([]typ.Type, len(call.Args))
	for i, arg := range call.Args {
		t.demandConditionReads(out, arg, demand)
		if av, ok := t.evalExpr(out, arg, demand); ok && !av.IsZero() {
			argTypes[i] = av.ProjectValue()
		} else {
			argTypes[i] = typ.Unknown
		}
	}
	// exprType resolves an expression against the live Env for the driver's callee/
	// receiver resolution (a function-valued local, a tracked receiver record). It
	// returns the value-domain unknown when the transfer does not track the value, so
	// the driver falls back to its module-wide signatures and globals; a read of an
	// unannotated parameter resolves to gradual `any` (a genuinely-gradual source).
	exprType := func(e ast.Expr) typ.Type {
		return t.resolveExprType(out, e, demand)
	}
	returns, ok := t.callTyper.CallReturns(call, argTypes, exprType)
	if !ok || len(returns) == 0 {
		return nil, false
	}
	out2 := make([]product.AbstractValue, len(returns))
	for i, rt := range returns {
		if rt == nil || typ.IsUnknown(rt) {
			out2[i] = product.AbstractValue{}
			continue
		}
		out2[i] = product.FromType(rt)
	}
	return out2, true
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
	// generic-for iteration reads. This is the array-like literal the legacy table
	// synthesis builds from a vararg field; an ordinary positional literal (`{1, 2, 3}`)
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
	builder := typ.NewRecord()
	for _, field := range e.Fields {
		if field == nil || field.Key == nil {
			continue
		}
		name, ok := staticFieldName(field.Key)
		if !ok {
			if id, isIdent := field.Key.(*ast.IdentExpr); isIdent {
				name = id.Value
			} else {
				continue
			}
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
		if zzEvalTableNoAny() && ft == typ.Any {
			continue
		}
		builder.Field(name, ft)
	}
	return product.FromType(builder.Build()), true
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

// evalAttrGet computes the value of a field or index read base.key against the
// current Env. It reads the base container's value, then applies the value-domain
// field read (product.FieldOf for a string-literal key) or index read
// (product.IndexOf for a dynamic key) — the AbstractValue-native forms of the
// legacy transfer's field/index query. A base or field that does not resolve
// reports false so the caller drops the slot to the value-domain Top.
func (t *Transfer) evalAttrGet(
	out *flow.PointState,
	e *ast.AttrGetExpr,
	demand func(int, paramevidence.ParamContract),
) (product.AbstractValue, bool) {
	base, ok := t.evalExpr(out, e.Object, demand)
	if !ok || base.IsZero() {
		return product.AbstractValue{}, false
	}
	if name, isField := staticFieldName(e.Key); isField {
		fv, ok := product.FieldOf(base, name)
		if !ok || fv.IsZero() {
			return product.AbstractValue{}, false
		}
		return fv, true
	}
	key, ok := t.evalExpr(out, e.Key, demand)
	if !ok || key.IsZero() {
		return product.AbstractValue{}, false
	}
	ev, ok := product.IndexOf(base, key)
	if !ok || ev.IsZero() {
		return product.AbstractValue{}, false
	}
	return t.refineIndexRead(out, e, base, ev), true
}

// staticFieldName reports whether key is a static field name (a dotted field
// access t.name lowers to a string-literal key) and that name. A non-literal key
// is a dynamic index, resolved through the index read instead.
func staticFieldName(key ast.Expr) (string, bool) {
	if s, ok := key.(*ast.StringExpr); ok {
		return s.Value, true
	}
	return "", false
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
		return product.AbstractValue{}, false
	}
	av, ok := out.Env[symKey(sym)]
	idx, isParam := t.paramBySym[sym]
	if isParam && demand != nil {
		// A parameter read demands that the parameter at least carries the value
		// observed for it here. With no narrower observation, demand the value's
		// own type; an unread-but-present slot demands nothing new.
		if ok && !av.IsZero() {
			demand(idx, av)
		}
	}
	if !ok || av.IsZero() {
		// A free variable captured from an enclosing scope has no Env value in this
		// body. It is neither a parameter nor a symbol this function declares, so its
		// type is the module-wide value its defining scope holds. A parameter or a
		// local declared here is NOT a capture (its absent Env value means undetermined,
		// the sound carry-forward), so the resolver is consulted only for a genuine
		// free variable.
		if !isParam && t.captureType != nil && t.isCapturedFreeVar(sym) {
			if ct, ok := t.captureType(sym); ok && ct != nil && !typ.IsAbsentOrUnknown(ct) {
				return product.FromType(ct), true
			}
		}
		return product.AbstractValue{}, false
	}
	return av, true
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
func (t *Transfer) isCapturedFreeVar(sym cfg.SymbolID) bool {
	if sym == 0 || t.in.Graph == nil {
		return false
	}
	if _, isParam := t.paramBySym[sym]; isParam {
		return false
	}
	if k, ok := t.in.Graph.SymbolKind(sym); ok && (k == cfg.SymbolLocal || k == cfg.SymbolParam) {
		return false
	}
	return true
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

// evalBinary resolves an arithmetic operator over its operand values. With an
// operator resolver it uses the resolved result; otherwise an arithmetic op over
// determined numeric operands stays numeric (the sound structural default).
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
	// Structural default: arithmetic on numbers is numeric.
	if isNumeric(l, lok) && isNumeric(r, rok) {
		return product.FromType(typ.Number), true
	}
	return product.AbstractValue{}, false
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
	}
	v, ok := t.evalExpr(out, operand, demand)
	if t.ops != nil && ok {
		res := t.ops.UnaryOp(op, v.ProjectValue())
		if res != nil && !typ.IsAbsentOrUnknown(res) {
			return product.FromType(res), true
		}
	}
	if isNumeric(v, ok) {
		return product.FromType(typ.Number), true
	}
	return product.AbstractValue{}, false
}

// evalLogical types a short-circuit logical expression (`a and b`, `a or b`)
// through the shared logical-op semantics (ops.LogicalAndTyped / ops.LogicalOrTyped),
// the same operator typing the legacy synthesizer uses. The result is the join of the
// surviving operand values: for `or` the truthy part of the left joined with the right
// (so `prefix or "["` yields the left's truthy type or the default), for `and` the
// falsy part of the left joined with the right. Operand types resolve through
// operandType, which falls back to gradual `any` for an unannotated parameter so a
// default pattern over an undeclared parameter still types rather than dropping to
// unknown.
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

// evalRelational types a comparison expression. A type-probe comparison
// (`type(x) == "string"`, the discriminant guard form) folds to the proven boolean
// outcome through the shared guard evaluation; any other comparison is a boolean. It
// reads both operands so a parameter compared here still emits demand.
func (t *Transfer) evalRelational(
	out *flow.PointState,
	e *ast.RelationalOpExpr,
	demand func(int, paramevidence.ParamContract),
) (product.AbstractValue, bool) {
	t.demandConditionReads(out, e.Lhs, demand)
	t.demandConditionReads(out, e.Rhs, demand)
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

// operandType resolves an expression's value type for an operator that reuses the
// legacy typ.Type operator semantics (logical and/or). It is the value-domain
// projection of evalExpr, with one refinement: an unannotated parameter the body has
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
	if ident, ok := expr.(*ast.IdentExpr); ok {
		if sym := t.symbolOf(ident); sym != 0 && t.unannotatedParam[sym] {
			return typ.Any
		}
	}
	return nil
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
	if av, ok := t.evalExpr(out, e, demand); ok && !av.IsZero() {
		if pt := av.ProjectValue(); pt != nil && !typ.IsUnknown(pt) {
			return pt
		}
	}
	if ident, ok := e.(*ast.IdentExpr); ok {
		if sym := t.symbolOf(ident); sym != 0 && t.unannotatedParam[sym] {
			return typ.Any
		}
	}
	return typ.Unknown
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
//     unbounded ascent to Top, which is what makes the canonical solver
//     terminate where the widen-free legacy runSCC does not.
//
// Other numeric assignments leave the component untouched (the slot's prior
// relation is dropped only when it is overwritten, which ApplyEqConst does).
func (t *Transfer) applyNumeric(out *flow.PointState, key string, src ast.Expr) {
	if out.Num == nil {
		return
	}
	pk := constraint.PathKey(key)
	if c, ok := t.constInt(src); ok {
		out.Num.ApplyEqConst(pk, c)
		return
	}
	arith, ok := src.(*ast.ArithmeticOpExpr)
	if !ok || arith.Operator != "+" {
		return
	}
	delta, self := t.constIncrement(arith, key)
	if !self || delta == 0 {
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
	out.Num.ApplyEqConst(pk, prevUpper+delta)
}

// constIncrement reports whether arith is `key + const` or `const + key` (the
// self-increment of the counter named by key) and the constant delta.
func (t *Transfer) constIncrement(arith *ast.ArithmeticOpExpr, key string) (int64, bool) {
	if c, ok := t.constInt(arith.Rhs); ok && t.isSymExpr(arith.Lhs, key) {
		return c, true
	}
	if c, ok := t.constInt(arith.Lhs); ok && t.isSymExpr(arith.Rhs, key) {
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

func (t *Transfer) isSymExpr(expr ast.Expr, key string) bool {
	ident, ok := expr.(*ast.IdentExpr)
	if !ok {
		return false
	}
	return symKey(t.symbolOf(ident)) == key
}

// applyBranch records the parameter demand a branch condition imposes and folds
// the branch's unconditional path test into this point's Cond (the condition that
// holds at the branch itself, before either edge is taken). The per-successor
// narrowing — the guard on the true edge and its negation on the false edge,
// applied to the Env values — is done by NarrowEdge (narrow.go) when a successor
// reads across the branch edge, since both successors share this point's out-state.
// A condition on a parameter still emits demand.
func (t *Transfer) applyBranch(
	out *flow.PointState,
	info *cfg.BranchInfo,
	demand func(int, paramevidence.ParamContract),
) {
	t.demandConditionReads(out, info.Condition, demand)
	if info.CondSymbol == 0 {
		return
	}
	path := constraint.NewPath(info.CondSymbol, info.CondVar)
	var c constraint.Condition
	switch info.CondCheck.Kind {
	case cfg.CheckTruthy:
		c = constraint.FromConstraints(constraint.Truthy{Path: path})
	case cfg.CheckFalsy:
		c = constraint.FromConstraints(constraint.Falsy{Path: path})
	case cfg.CheckNil:
		c = constraint.FromConstraints(constraint.IsNil{Path: path})
	case cfg.CheckNotNil:
		c = constraint.FromConstraints(constraint.NotNil{Path: path})
	default:
		return
	}
	out.Cond = constraint.Domain.Join(out.Cond, c)
}

// applyReturn records demand for parameters read in return expressions and
// stores each returned expression's evaluated value under a return-slot Env key.
// The summary projection reads those slots so the function's return tuple carries
// the value of a non-identifier return (a literal, an arithmetic result, a call)
// rather than the value-domain Top.
func (t *Transfer) applyReturn(
	out *flow.PointState,
	info *cfg.ReturnInfo,
	demand func(int, paramevidence.ParamContract),
) {
	for i, expr := range info.Exprs {
		t.demandConditionReads(out, expr, demand)
		// A returned identifier already carries its value in the variable's Env
		// slot, which the projection reads directly; only stash non-identifier
		// return values, whose value lives nowhere else.
		if _, isIdent := expr.(*ast.IdentExpr); isIdent {
			continue
		}
		if val, ok := t.evalExpr(out, expr, demand); ok && !val.IsZero() {
			out.Env[ReturnSlotKey(i)] = val
		}
	}
}

// applyCallArgs records demand for parameters passed as call arguments and read
// inside the callee position, and applies the continuation narrowing an assert
// imposes. It reports dead=true when an assert proves its continuation unreachable
// (assert of an always-false condition), so the caller terminates the flow.
func (t *Transfer) applyCallArgs(
	out *flow.PointState,
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
	if t.callTyper != nil && t.callTyper.IsNoReturn(info.Call) {
		zprobeNarrow("noReturn-call dead callee=%q", info.CalleeName)
		return true
	}
	// A call to a module function that narrows its parameters (a wrapper around
	// assert / `if x == nil then error()`) carries that narrowing to the matching
	// argument here, so `check(y); use(y)` sees `y` narrowed.
	if t.callTyper != nil {
		if effects := t.callTyper.ParamNarrows(info.Call); len(effects) > 0 {
			t.ApplyParamNarrows(out, info.Call, effects)
		}
	}
	return false
}

// demandConditionReads walks an expression for identifier reads, emitting
// parameter demand for each parameter read. It does not mutate value state.
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
		t.demandConditionReads(out, e.Lhs, demand)
		t.demandConditionReads(out, e.Rhs, demand)
	case *ast.RelationalOpExpr:
		t.demandConditionReads(out, e.Lhs, demand)
		t.demandConditionReads(out, e.Rhs, demand)
	case *ast.LogicalOpExpr:
		t.demandConditionReads(out, e.Lhs, demand)
		t.demandConditionReads(out, e.Rhs, demand)
	case *ast.StringConcatOpExpr:
		t.demandConditionReads(out, e.Lhs, demand)
		t.demandConditionReads(out, e.Rhs, demand)
	case *ast.UnaryMinusOpExpr:
		t.demandConditionReads(out, e.Expr, demand)
	case *ast.UnaryNotOpExpr:
		t.demandConditionReads(out, e.Expr, demand)
	case *ast.UnaryLenOpExpr:
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

// clonePointState deep-copies the mutable parts of a PointState so the transfer
// never aliases a predecessor's stored state (the solver compares states by
// value-domain Equal; mutating shared maps would corrupt the worklist).
func clonePointState(ps flow.PointState) flow.PointState {
	env := make(map[string]product.AbstractValue, len(ps.Env))
	for k, v := range ps.Env {
		env[k] = v
	}
	out := flow.PointState{Env: env, Cond: ps.Cond}
	if ps.Num != nil {
		out.Num = ps.Num.Clone()
	}
	return out
}

func itoa(v uint64) string {
	if v == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}

// zzEvalTableNoAny disables recording a table-literal field whose value does not
// resolve as a gradual `any` field, restoring the prior drop-the-field behavior.
// Debug toggle for attributing oracle deltas to the table-literal field-presence
// change.
func zzEvalTableNoAny() bool {
	return os.Getenv("ZZNOANY") != ""
}
