package program

import (
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/compiler/ast"
)

// paramInference accumulates, per callee function symbol, the join of every
// statically-visible call site's argument values per parameter index. The join
// is the sound seed for an unannotated parameter: it is a supertype of every
// actual argument, so the body sees the most precise type that holds across all
// call sites without ever assuming a type narrower than some caller provides.
type paramInference struct {
	reg      *axis.Registry
	enclosed map[symbol.ID]struct{}
	params   map[symbol.ID]*calleeArgs
	observed map[factflow.ExprRef]struct{}
}

// calleeArgs accumulates the per-parameter join across observed call sites along
// with how many sites supplied an argument at each index. A parameter is only
// seeded when every observed site supplied it, so an omitted argument (which is
// nil at runtime) never narrows the inferred parameter unsoundly.
type calleeArgs struct {
	sites    int
	joins    []product.Value
	presence []int
	heap     state.State
}

func newParamInference(reg *axis.Registry, enclosed map[symbol.ID]struct{}) *paramInference {
	return &paramInference{
		reg:      reg,
		enclosed: enclosed,
		params:   make(map[symbol.ID]*calleeArgs),
		observed: make(map[factflow.ExprRef]struct{}),
	}
}

// markObserved reports whether a call expression has already contributed its
// arguments. Overlapping prepasses re-walk shared function bodies, so each call
// site must be counted exactly once.
func (p *paramInference) markObserved(expr factflow.ExprRef) bool {
	if p == nil || expr == 0 {
		return false
	}
	if _, ok := p.observed[expr]; ok {
		return false
	}
	p.observed[expr] = struct{}{}
	return true
}

// candidate reports whether callee is an enclosed function whose parameters are
// eligible for call-site inference.
func (p *paramInference) candidate(callee symbol.ID) bool {
	if p == nil || callee == 0 {
		return false
	}
	_, ok := p.enclosed[callee]
	return ok
}

// observe joins one call site's argument values into the running per-parameter
// join for the callee function symbol. Arguments beyond the recorded length grow
// the slice; missing arguments at a recorded index are absent for this call and
// therefore join nil-presence Top, keeping the parameter at least as wide.
func (p *paramInference) observe(callee symbol.ID, args []product.Value, present []bool, caller state.State) {
	if p == nil || callee == 0 {
		return
	}
	if _, ok := p.enclosed[callee]; !ok {
		return
	}
	acc := p.params[callee]
	if acc == nil {
		acc = &calleeArgs{}
		p.params[callee] = acc
	}
	acc.sites++
	for i := range args {
		if i >= len(acc.joins) {
			acc.joins = append(acc.joins, product.Bottom(p.reg))
			acc.presence = append(acc.presence, 0)
		}
		if i < len(present) && !present[i] {
			continue
		}
		acc.joins[i] = product.Join(p.reg, acc.joins[i], args[i])
		acc.presence[i]++
		acc.heap = seedReachableHeapFromValue(p.reg, acc.heap, caller, args[i], map[identity.ID]struct{}{})
	}
}

func (p *paramInference) seedSource(callee symbol.ID) state.State {
	if p == nil {
		return state.State{}
	}
	acc := p.params[callee]
	if acc == nil {
		return state.State{}
	}
	return acc.heap.Snapshot()
}

// paramSeed binds one inferred parameter value to its symbol value slot.
type paramSeed struct {
	slot  statekey.Value
	value product.Value
}

// paramSeeds builds the inferred parameter seeds for a function from the joined
// call-site arguments. Only parameters whose declared slot is unannotated are
// seeded; an annotated parameter keeps its declared type. A joined value that is
// Top (an argument was itself any, or call sites disagreed up to Top) is dropped
// so the parameter stays at the Top base-summary seed soundly.
func (p *paramInference) paramSeeds(bindings *bind.Result, fn *ast.FunctionExpr, callee symbol.ID) []paramSeed {
	if p == nil || p.reg == nil || bindings == nil || fn == nil {
		return nil
	}
	acc, ok := p.params[callee]
	if !ok || acc == nil || acc.sites == 0 {
		return nil
	}
	slots := bindings.ParamSlots(fn)
	var out []paramSeed
	top := product.Set(p.reg, product.Top(), evidence.Key, evidence.GradualTop())
	for i, slot := range slots {
		if slot.Symbol == 0 || slot.Vararg || slot.Type != nil {
			continue
		}
		if i >= len(acc.joins) {
			continue
		}
		// Every observed call site must have supplied this argument; otherwise an
		// omitted argument is nil at runtime and the joined value would be a
		// narrower type than the parameter can actually receive.
		if acc.presence[i] != acc.sites {
			continue
		}
		value := acc.joins[i]
		if product.Equal(p.reg, value, product.Bottom(p.reg)) {
			continue
		}
		if product.Equal(p.reg, value, product.Top()) || product.Equal(p.reg, value, top) {
			continue
		}
		if !inferableParamValue(p.reg, value) {
			continue
		}
		valueSlot := statekey.SymbolValue(slot.Symbol)
		if valueSlot == "" {
			continue
		}
		out = append(out, paramSeed{slot: valueSlot, value: value})
	}
	return out
}

// applyParamSeeds writes inferred parameter seeds onto a clone of base. An
// existing non-Bottom slot value is preserved so a caller-supplied entry state
// is never overwritten by inference. When a written seed carries a singleton
// table identity, reachable heap table objects are copied from source into the
// callee entry state's heap sidecar without changing any summary or context key.
func applyParamSeeds(reg *axis.Registry, base, source state.State, seeds []paramSeed) state.State {
	if reg == nil || len(seeds) == 0 {
		return base
	}
	bottom := product.Bottom(reg)
	out := base.Snapshot()
	for _, seed := range seeds {
		if seed.slot == "" {
			continue
		}
		if !product.Equal(reg, out.ReadValue(reg, seed.slot), bottom) {
			continue
		}
		out = out.WriteValue(reg, seed.slot, seed.value)
		out = seedReachableHeapFromValue(reg, out, source, seed.value, map[identity.ID]struct{}{})
	}
	return out
}

func seedReachableHeapFromValue(
	reg *axis.Registry,
	dst, src state.State,
	value product.Value,
	seen map[identity.ID]struct{},
) state.State {
	if reg == nil {
		return dst
	}
	id, ok := product.Get(reg, value, identity.Key).ID()
	if !ok || id == (identity.ID{}) {
		return dst
	}
	return seedReachableHeapFromID(reg, dst, src, id, seen)
}

func seedReachableHeapFromID(
	reg *axis.Registry,
	dst, src state.State,
	id identity.ID,
	seen map[identity.ID]struct{},
) state.State {
	if reg == nil || id == (identity.ID{}) {
		return dst
	}
	if _, ok := seen[id]; ok {
		return dst
	}
	seen[id] = struct{}{}
	object := src.ReadHeapTableObject(reg, id)
	objectDomain := heapidentity.ObjectDomain(reg)
	if objectDomain.Equal(object, objectDomain.Bottom()) {
		return dst
	}
	if existing := dst.ReadHeapTableObject(reg, id); !objectDomain.Equal(existing, objectDomain.Bottom()) {
		object = objectDomain.Join(existing, object)
	}
	dst = dst.WriteHeapTableObject(reg, id, object)
	dst = seedReachableHeapFromValue(reg, dst, src, object.Root(), seen)
	for _, member := range object.StaticMembers() {
		dst = seedReachableHeapFromValue(reg, dst, src, member, seen)
	}
	for _, fact := range object.DynamicIndexFacts() {
		dst = seedReachableHeapFromValue(reg, dst, src, fact.KeyValue, seen)
		dst = seedReachableHeapFromValue(reg, dst, src, fact.Value, seen)
	}
	return dst
}

// inferableParamValue rejects a joined argument value that carries no usable
// type witness: seeding a parameter with such a value cannot improve body
// diagnostics and risks masking the unannotated-parameter signal.
func inferableParamValue(reg *axis.Registry, value product.Value) bool {
	t, ok := typevalue.TypeOf(reg, value)
	if !ok || t == nil {
		return false
	}
	return true
}

// collectEnclosedFunctions returns the set of function symbols whose every
// reference is the direct callee of a call. Such a function cannot reach an
// unknown consumer, so the prepass call-site enumeration observes every one of
// its calls and the per-parameter join is complete. Any other reference (stored
// in a table, returned, passed as an argument, assigned, indexed, used as a
// receiver) is treated as an escape and excludes the function: inferring from a
// partial call-site set would be unsound.
func collectEnclosedFunctions(bindings *bind.Result, roots ...[]ast.Stmt) map[symbol.ID]struct{} {
	if bindings == nil {
		return nil
	}
	functionSymbols := make(map[symbol.ID]struct{})
	// targetToFunction maps a function's local binding symbol to the function
	// symbol, so an escaping reference to the binding (which is what call sites
	// and value reads resolve to) excludes the function it names.
	targetToFunction := make(map[symbol.ID]symbol.ID)
	for _, origin := range bindings.FunctionOrigins() {
		if origin.Symbol == 0 || origin.Func == nil {
			continue
		}
		// A method definition stores the function value into a receiver table,
		// reaching callers outside the analyzed call-site set.
		if origin.Kind == bind.FunctionOriginMethod {
			continue
		}
		// A function bound to a non-local target (a global or a dotted module
		// field) is reachable through that target by external callers.
		if origin.HasTargetSymbol {
			kind, ok := bindings.Kind(origin.TargetSymbol)
			if !ok || (kind != symbol.Local && kind != symbol.Param) {
				continue
			}
			targetToFunction[origin.TargetSymbol] = origin.Symbol
		}
		functionSymbols[origin.Symbol] = struct{}{}
	}
	if len(functionSymbols) == 0 {
		return nil
	}
	walker := &escapeWalker{
		bindings:         bindings,
		candidate:        functionSymbols,
		targetToFunction: targetToFunction,
		escaped:          make(map[symbol.ID]struct{}),
	}
	for _, stmts := range roots {
		walker.walkStmts(stmts)
	}
	for _, origin := range bindings.FunctionOrigins() {
		if origin.Func == nil {
			continue
		}
		walker.walkStmts(origin.Func.Stmts)
	}
	out := make(map[symbol.ID]struct{}, len(functionSymbols))
	for sym := range functionSymbols {
		if _, ok := walker.escaped[sym]; ok {
			continue
		}
		out[sym] = struct{}{}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

type escapeWalker struct {
	bindings         *bind.Result
	candidate        map[symbol.ID]struct{}
	targetToFunction map[symbol.ID]symbol.ID
	escaped          map[symbol.ID]struct{}
}

func (w *escapeWalker) walkStmts(stmts []ast.Stmt) {
	for _, stmt := range stmts {
		w.walkStmt(stmt)
	}
}

func (w *escapeWalker) walkStmt(stmt ast.Stmt) {
	switch stmt := stmt.(type) {
	case *ast.AssignStmt:
		for _, expr := range stmt.Lhs {
			w.walkExpr(expr)
		}
		for _, expr := range stmt.Rhs {
			w.walkExpr(expr)
		}
	case *ast.LocalAssignStmt:
		for _, expr := range stmt.Exprs {
			w.walkExpr(expr)
		}
	case *ast.FuncCallStmt:
		w.walkExpr(stmt.Expr)
	case *ast.DoBlockStmt:
		w.walkStmts(stmt.Stmts)
	case *ast.WhileStmt:
		w.walkExpr(stmt.Condition)
		w.walkStmts(stmt.Stmts)
	case *ast.RepeatStmt:
		w.walkStmts(stmt.Stmts)
		w.walkExpr(stmt.Condition)
	case *ast.IfStmt:
		w.walkExpr(stmt.Condition)
		w.walkStmts(stmt.Then)
		w.walkStmts(stmt.Else)
	case *ast.NumberForStmt:
		w.walkExpr(stmt.Init)
		w.walkExpr(stmt.Limit)
		w.walkExpr(stmt.Step)
		w.walkStmts(stmt.Stmts)
	case *ast.GenericForStmt:
		for _, expr := range stmt.Exprs {
			w.walkExpr(expr)
		}
		w.walkStmts(stmt.Stmts)
	case *ast.FuncDefStmt:
		// The definition body is walked through FunctionOrigins; the name is a
		// declaration, not a value read.
	case *ast.ReturnStmt:
		// Returning a function value exposes it to callers outside the analyzed
		// scope. Each returned expression is walked as a value read.
		for _, expr := range stmt.Exprs {
			w.walkExpr(expr)
		}
	}
}

// walkExpr classifies every identifier read inside expr. A direct callee
// identifier is excluded from the escape set; every other identifier read of a
// candidate function symbol marks it escaped.
func (w *escapeWalker) walkExpr(expr ast.Expr) {
	switch expr := expr.(type) {
	case nil:
		return
	case *ast.IdentExpr:
		w.markIdentEscape(expr)
	case *ast.FuncCallExpr:
		w.walkCall(expr)
	case *ast.AttrGetExpr:
		w.walkExpr(expr.Object)
		w.walkExpr(expr.Key)
	case *ast.TableExpr:
		for _, field := range expr.Fields {
			if field == nil {
				continue
			}
			w.walkExpr(field.Key)
			w.walkExpr(field.Value)
		}
	case *ast.LogicalOpExpr:
		w.walkExpr(expr.Lhs)
		w.walkExpr(expr.Rhs)
	case *ast.RelationalOpExpr:
		w.walkExpr(expr.Lhs)
		w.walkExpr(expr.Rhs)
	case *ast.StringConcatOpExpr:
		w.walkExpr(expr.Lhs)
		w.walkExpr(expr.Rhs)
	case *ast.ArithmeticOpExpr:
		w.walkExpr(expr.Lhs)
		w.walkExpr(expr.Rhs)
	case *ast.UnaryMinusOpExpr:
		w.walkExpr(expr.Expr)
	case *ast.UnaryNotOpExpr:
		w.walkExpr(expr.Expr)
	case *ast.UnaryLenOpExpr:
		w.walkExpr(expr.Expr)
	case *ast.UnaryBNotOpExpr:
		w.walkExpr(expr.Expr)
	case *ast.CastExpr:
		w.walkExpr(expr.Expr)
	case *ast.NonNilAssertExpr:
		w.walkExpr(expr.Expr)
	case *ast.FunctionExpr:
		// Nested function bodies are walked through FunctionOrigins; descending
		// here as well would not change the escape verdict.
	}
}

func (w *escapeWalker) walkCall(call *ast.FuncCallExpr) {
	if call == nil {
		return
	}
	// A direct identifier callee is a call, not an escape. A non-identifier
	// callee (an indexed field, a returned value) is walked as a value read so
	// any function identifier inside it is treated as escaped.
	if _, ok := call.Func.(*ast.IdentExpr); !ok {
		w.walkExpr(call.Func)
	}
	w.walkExpr(call.Receiver)
	for _, arg := range call.Args {
		w.walkExpr(arg)
	}
}

func (w *escapeWalker) markIdentEscape(ident *ast.IdentExpr) {
	if ident == nil {
		return
	}
	sym, ok := w.bindings.SymbolOf(ident)
	if !ok || sym == 0 {
		return
	}
	if _, ok := w.candidate[sym]; ok {
		w.escaped[sym] = struct{}{}
	}
	// A reference to a function's local binding (the value the call site resolves
	// through) in a non-call position lets the value reach an unknown consumer.
	if fnSym, ok := w.targetToFunction[sym]; ok {
		w.escaped[fnSym] = struct{}{}
	}
}
