// Package wirlower lowers typed-Lua syntax into wir attached to the shared CFG
// that cfgbuild already produced (decision D1a). It creates no CFG topology of
// its own: cfgbuild is the point authority, and wirlower maps each Lua construct
// onto the pre-existing points it discovers through cfgbuild.Result.StmtPoints
// (statement -> points, in creation order) and cfgfacts.Metadata (short-circuit
// guard / expression-evaluation sidecars).
//
// It translates syntax and resolves bindings/types only. It computes no
// refinements, no narrowing, and no type conclusions: every value derivation is
// the transfer interpreter's job. Conditions are lowered through
// branchcond.Normalize (a closed descriptor) and attached to OpBranch; paths
// come from pathexpr/keyspace; nothing here concludes anything about values.
// That boundary is the whole point of wir.
//
// Point granularity follows cfgbuild: one OpCall per call (each on its own
// NodeCall point, in Lua evaluation order, before the owning statement's own
// points); one destination per NodeAssign point (multret result binding splits
// across the per-target assign points); loop headers split (OpIterate on the
// branch point, the loop-variable binding on the assign point(s)); joins carry
// no instruction. Short-circuit and/or follows the always-materialized
// short-circuit topology cfgbuild emits (decision D3, purity split).
package wirlower

import (
	"strconv"

	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
	"github.com/wippyai/go-lua/analysis/lua/callorder"
	"github.com/wippyai/go-lua/analysis/lua/cfgbuild"
	"github.com/wippyai/go-lua/analysis/lua/cfgfacts"
	"github.com/wippyai/go-lua/analysis/lua/channelruntime"
	"github.com/wippyai/go-lua/analysis/lua/expressionid"
	"github.com/wippyai/go-lua/analysis/lua/functiontype"
	"github.com/wippyai/go-lua/analysis/lua/pathexpr"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	luatypeprojection "github.com/wippyai/go-lua/analysis/lua/typeprojection"
	"github.com/wippyai/go-lua/analysis/lua/typeresolve"
	"github.com/wippyai/go-lua/analysis/lua/valueexpr"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/source"
)

// Lower lowers a bound statement chunk onto the CFG cfgbuild already built for
// the same AST. bindings must be the result of binding stmts (e.g.
// bind.BindChunk) and built must be cfgbuild.BuildChunk over the same stmts.
// Points in the returned Body index into built.Graph.
func Lower(name string, stmts []ast.Stmt, bindings *bind.Result, built *cfgbuild.Result) *wir.Body {
	return LowerWithResolver(name, stmts, bindings, built, typeresolve.New(bindings))
}

// LowerFunction lowers a bound function body and records function-level syntax
// metadata such as declared return slots on the returned WIR body.
func LowerFunction(name string, fn *ast.FunctionExpr, bindings *bind.Result, built *cfgbuild.Result) *wir.Body {
	return LowerFunctionWithResolver(name, fn, bindings, built, typeresolve.New(bindings))
}

// LowerWithResolver lowers with the caller's canonical type resolver. This is
// the production entry point when module/export type refs are in scope; WIR
// TypeRefs must match the resolver used by transfer facts.
func LowerWithResolver(name string, stmts []ast.Stmt, bindings *bind.Result, built *cfgbuild.Result, resolver *typeresolve.Resolver) *wir.Body {
	if resolver == nil {
		resolver = typeresolve.New(bindings)
	}
	return lowerInto(name, stmts, bindings, built, resolver)
}

// LowerFunctionWithResolver is the function-body form of LowerWithResolver. It
// keeps function-level metadata in WIR instead of requiring later stages to
// reach back into semantic AST sidecars.
func LowerFunctionWithResolver(name string, fn *ast.FunctionExpr, bindings *bind.Result, built *cfgbuild.Result, resolver *typeresolve.Resolver) *wir.Body {
	if resolver == nil {
		resolver = typeresolve.New(bindings)
	}
	var stmts []ast.Stmt
	if fn != nil {
		stmts = fn.Stmts
	}
	body := lowerInto(name, stmts, bindings, built, resolver)
	body.SetDeclaredReturnTypes(resolveDeclaredReturns(fn, resolver))
	return body
}

// lowerInto lowers one function-scope statement list (a chunk or a nested
// function body) onto its shared graph. resolver is the shared lexical type
// resolver, threaded through nested protos so type identities and their caches
// stay consistent across the whole chunk.
func lowerInto(name string, stmts []ast.Stmt, bindings *bind.Result, built *cfgbuild.Result, resolver *typeresolve.Resolver) *wir.Body {
	b := &builder{
		body:                wir.NewBody(name),
		graph:               built.Graph,
		meta:                built.Meta,
		points:              built.StmtPoints,
		bindings:            bindings,
		resolver:            resolver,
		pointInstrs:         make(map[cfg.Point][]wir.Instruction),
		callTemps:           make(map[*ast.FuncCallExpr]*callResult),
		guardByCond:         make(map[ast.Expr]cfg.Point),
		evalByExpr:          make(map[ast.Expr]cfg.Point),
		logicalGuardEmitted: make(map[*ast.LogicalOpExpr]bool),
		logicalValues:       make(map[*ast.LogicalOpExpr]wir.Operand),
	}
	b.indexShortCircuits()

	b.curPoint = b.graph.Entry()
	b.emit(wir.Instruction{Op: wir.OpEntry})
	b.lowerStmts(stmts)
	b.curPoint = b.graph.Exit()
	b.emit(wir.Instruction{Op: wir.OpExit})

	b.flush()
	return b.body
}

func resolveDeclaredReturns(fn *ast.FunctionExpr, resolver *typeresolve.Resolver) []typ.Type {
	if fn == nil {
		return nil
	}
	declared := functiontype.ReturnTypeExprs(fn.ReturnTypes)
	if len(declared) == 0 {
		return nil
	}
	out := make([]typ.Type, len(declared))
	if resolver == nil {
		return out
	}
	for i, expr := range declared {
		if expr == nil {
			continue
		}
		if t, ok := resolver.Type(expr); ok {
			out[i] = t
		}
	}
	return out
}

type builder struct {
	body     *wir.Body
	graph    cfg.Graph
	meta     cfgfacts.Metadata
	points   cfgbuild.StmtPoints
	bindings *bind.Result
	resolver *typeresolve.Resolver

	curPoint    cfg.Point
	pointInstrs map[cfg.Point][]wir.Instruction
	tempSeq     uint32
	protoSeq    int

	// callTemps maps every call pre-lowered at its own NodeCall point to the
	// result temp(s) it binds. Expression lowering substitutes a call's head
	// temp in place of re-emitting the call; multret tail patching flips the
	// producing OpCall's ResultSpread through the recorded (point, index).
	callTemps map[*ast.FuncCallExpr]*callResult

	// guardByCond and evalByExpr index the short-circuit sidecar points cfgbuild
	// records outside StmtPoints, keyed by the AST identity wirlower matches on
	// (the guard condition = LogicalOpExpr.Lhs, the eval expr = the RHS).
	guardByCond         map[ast.Expr]cfg.Point
	evalByExpr          map[ast.Expr]cfg.Point
	logicalGuardEmitted map[*ast.LogicalOpExpr]bool
	logicalValues       map[*ast.LogicalOpExpr]wir.Operand
}

// callResult records the result temps a pre-lowered call binds together with the
// slot to patch when the call is later found in a spread (multret) tail.
type callResult struct {
	head  wir.Operand
	temps []wir.Operand
	point cfg.Point
	index int
}

func (b *builder) indexShortCircuits() {
	for _, p := range b.meta.ShortCircuitGuardPoints() {
		if g, ok := b.meta.ShortCircuitGuard(p); ok && g.Condition != nil {
			b.guardByCond[g.Condition] = p
		}
	}
	for _, p := range b.graph.RPO() {
		if e, ok := b.meta.ExpressionEvaluation(p); ok && e.Expr != nil {
			b.evalByExpr[e.Expr] = p
		}
	}
}

// emit accumulates an instruction on the current point. Instructions are flushed
// to the Body in point order once lowering completes; accumulation lets the
// short-circuit RHS patch a producer's ResultSpread before flush.
func (b *builder) emit(inst wir.Instruction) {
	inst.Point = b.curPoint
	b.pointInstrs[b.curPoint] = append(b.pointInstrs[b.curPoint], inst)
}

// flush writes every point's accumulated instructions into the Body as a
// contiguous window, in point-id order.
func (b *builder) flush() {
	for p := 0; p < b.graph.Size(); p++ {
		point := cfg.Point(p)
		insts := b.pointInstrs[point]
		if len(insts) == 0 {
			continue
		}
		start := b.body.Len()
		for _, in := range insts {
			b.body.Emit(in)
		}
		b.body.SetPointRange(point, start, b.body.Len())
	}
}

func (b *builder) newTemp() wir.Operand {
	op := wir.Operand{Kind: wir.OperandTemp, Ref: b.tempSeq}
	b.tempSeq++
	return op
}

func (b *builder) callOrderOptions() callorder.Options {
	options := callorder.LuaOptions(b.bindings)
	options.AllowShortCircuitCalls = true
	return options
}

// stmtPoints returns the CFG points cfgbuild recorded for stmt, in creation
// order: the statement's call points (Lua evaluation order) followed by the
// statement's own points.
func (b *builder) stmtPoints(stmt ast.Stmt) []cfg.Point {
	return b.points.PointsFor(stmt)
}

func (b *builder) lowerStmts(stmts []ast.Stmt) {
	for _, stmt := range stmts {
		b.lowerStmt(stmt)
	}
}

func (b *builder) lowerStmt(stmt ast.Stmt) {
	switch s := stmt.(type) {
	case nil:
		return
	case *ast.LocalAssignStmt:
		b.lowerLocalAssign(s)
	case *ast.AssignStmt:
		b.lowerAssign(s)
	case *ast.FuncCallStmt:
		b.lowerCallStmt(s)
	case *ast.IfStmt:
		b.lowerIf(s)
	case *ast.NumberForStmt:
		b.lowerNumberFor(s)
	case *ast.GenericForStmt:
		b.lowerGenericFor(s)
	case *ast.WhileStmt:
		b.lowerWhile(s)
	case *ast.RepeatStmt:
		b.lowerRepeat(s)
	case *ast.ReturnStmt:
		b.lowerReturn(s)
	case *ast.FuncDefStmt:
		b.lowerFuncDef(s)
	case *ast.DoBlockStmt:
		b.lowerStmts(s.Stmts)
	case *ast.BreakStmt, *ast.LabelStmt, *ast.GotoStmt, *ast.TypeDefStmt, *ast.InterfaceDefStmt:
		// break carries no recorded point; goto/label/type declarations occupy a
		// structural point that carries no value effect (prints as noop).
	}
}

// ---- assignments --------------------------------------------------------

func (b *builder) lowerLocalAssign(s *ast.LocalAssignStmt) {
	pts := b.stmtPoints(s)
	nCalls := b.countCalls(s.Exprs)
	if len(pts) != nCalls+len(s.Names) {
		return
	}
	b.preLowerAssignCalls(s.Exprs, pts[:nCalls], len(s.Names))
	assignPoints := pts[nCalls:]
	values := b.planValues(len(s.Names), s.Exprs)
	for i := range s.Names {
		b.curPoint = assignPoints[i]
		dst := b.localPath(s, i)
		var declared wir.TypeRef
		if i < len(s.Types) && s.Types[i] != nil {
			declared = b.internType(s.Types[i])
			values[i] = values[i].withContextType(declared)
		}
		b.bindInto(dst, values[i].withTarget(wir.CallResultTargetLocalAssignment, i))
		if declared != 0 {
			b.emit(wir.Instruction{
				Op:    wir.OpClaim,
				Dst:   dst,
				A:     dst,
				Claim: wir.ClaimAnnotation,
				Type:  declared,
			})
		}
	}
}

func (b *builder) lowerAssign(s *ast.AssignStmt) {
	pts := b.stmtPoints(s)
	nCalls := b.countCalls(s.Rhs)
	if len(pts) != nCalls+len(s.Lhs) {
		return
	}
	b.preLowerAssignCalls(s.Rhs, pts[:nCalls], len(s.Lhs))
	assignPoints := pts[nCalls:]
	values := b.planValues(len(s.Lhs), s.Rhs)
	for i, target := range s.Lhs {
		b.curPoint = assignPoints[i]
		b.lowerAssignTarget(i, target, values[i])
	}
}

// lowerAssignTarget writes a pre-planned value into one assignment target,
// choosing the write op by target shape.
func (b *builder) lowerAssignTarget(index int, target ast.Expr, v binding) {
	v = v.withTarget(wir.CallResultTargetOrdinaryAssignment, index)
	switch t := target.(type) {
	case *ast.IdentExpr:
		dst, _ := b.targetOperand(t)
		b.bindInto(dst, v)
	case *ast.AttrGetExpr:
		if p, ok := pathexpr.Resolve(t, b.bindings); ok {
			b.emit(wir.Instruction{
				Op:  wir.OpStaticMemberWrite,
				Dst: b.pathOperand(p),
				A:   b.bindingOperand(v),
			})
			if v.hasCallResult {
				b.recordCallResultTargetPath(v, p)
			}
			return
		}
		target, ok := pathexpr.ResolveDynamicMutationTarget(t, b.bindings)
		if !ok {
			container, _ := pathexpr.ResolveMutationContainer(t, b.bindings)
			target.Table = container
			target.Key = t.Key
		}
		if context := b.dynamicIndexWriteContextType(target); context != 0 {
			v = v.withContextType(context)
		}
		b.emit(wir.Instruction{
			Op:            wir.OpDynamicIndexWrite,
			Dst:           b.pathOperand(target.Table),
			A:             b.lowerExpr(target.Key),
			B:             b.bindingOperand(v),
			DynamicSuffix: b.body.AppendSegments(target.Suffix),
		})
		if v.hasCallResult {
			b.recordCallResultTargetPath(v, path.Path{})
		}
	default:
		dst := b.newTemp()
		b.bindInto(dst, v)
	}
}

// ---- value binding plan -------------------------------------------------

type bindingKind uint8

const (
	bindNil bindingKind = iota
	bindExpr
	bindOperand
	bindVararg
)

// binding is one target's planned value: a direct expression to lower into the
// destination, a pre-lowered operand (a call result temp), a nil fill, or a
// vararg expansion.
type binding struct {
	kind bindingKind
	expr ast.Expr
	op   wir.Operand

	hasCallResult bool
	callPoint     cfg.Point
	resultIndex   int
	targetKind    wir.CallResultTargetKind
	targetIndex   int

	claim     wir.ClaimKind
	claimType wir.TypeRef

	contextType wir.TypeRef
}

func (v binding) withTarget(kind wir.CallResultTargetKind, index int) binding {
	v.targetKind = kind
	v.targetIndex = index
	return v
}

func (v binding) withContextType(t wir.TypeRef) binding {
	v.contextType = t
	return v
}

// planValues maps n destinations to their source values, preserving Lua tail
// expansion: the final expression, when a call or vararg, fills every remaining
// destination; a shorter list nil-fills the rest. Calls are already pre-lowered
// at their own points, so a tail call's targets bind to its result temps.
func (b *builder) planValues(n int, exprs []ast.Expr) []binding {
	out := make([]binding, n)
	if len(exprs) == 0 {
		return out // all bindNil
	}
	last := len(exprs) - 1
	for i := 0; i < n; i++ {
		if i < last {
			if call, ok := tailCall(exprs[i]); ok {
				if cr, ok := b.callTemps[call]; ok {
					out[i] = b.tempBinding(cr, 0, exprs[i])
					continue
				}
			}
			out[i] = binding{kind: bindExpr, expr: exprs[i]}
			continue
		}
		// i == last (i > last is impossible: loop bound n, and when last >= n the
		// loop exits before reaching last). Bind the final expression.
		e := exprs[last]
		if call, ok := tailCall(e); ok {
			if cr, ok := b.callTemps[call]; ok {
				for j := last; j < n; j++ {
					out[j] = b.tempBinding(cr, j-last, e)
				}
				return out
			}
		}
		if isVararg(e) {
			for j := last; j < n; j++ {
				out[j] = binding{kind: bindVararg}
			}
			return out
		}
		out[last] = binding{kind: bindExpr, expr: e}
		// remaining targets (last+1 .. n-1) stay bindNil
		return out
	}
	return out
}

// tempBinding returns the operand for the k-th result of a pre-lowered call,
// falling back to the head when the call bound fewer explicit temps than k.
func (b *builder) tempBinding(cr *callResult, k int, expr ast.Expr) binding {
	out := binding{kind: bindOperand, callPoint: cr.point, resultIndex: k, hasCallResult: true}
	if k < len(cr.temps) {
		out.op = cr.temps[k]
	} else {
		out.op = cr.head
	}
	b.addBindingClaim(&out, expr)
	return out
}

func (b *builder) addBindingClaim(out *binding, expr ast.Expr) {
	if out == nil || expr == nil {
		return
	}
	switch e := expr.(type) {
	case *ast.CastExpr:
		out.claim = wir.ClaimCast
		out.claimType = b.internType(e.Type)
	case *ast.NonNilAssertExpr:
		out.claim = wir.ClaimAssert
	}
}

// bindInto writes a planned value into a destination that can receive a produced
// instruction directly (a path or temp), keeping the compact `dst = op a b` form
// for compound expressions.
func (b *builder) bindInto(dst wir.Operand, v binding) {
	point := b.curPoint
	start := len(b.pointInstrs[point])
	assignKind := b.rootAssignKind(dst, v)
	switch v.kind {
	case bindExpr:
		b.lowerExprInto(dst, v.expr)
		b.attachContextTypeToConstructors(point, start, v.expr, v.contextType)
	case bindOperand:
		b.emitAssign(dst, v.op)
		b.emitBindingClaim(dst, v)
		b.recordCallResultTarget(dst, v)
	case bindVararg:
		b.emitAssign(dst, wir.Operand{Kind: wir.OperandVararg})
	default:
		b.emitAssign(dst, b.constNil())
	}
	b.markRootAssignKind(point, start, dst, assignKind)
}

func (b *builder) attachContextTypeToConstructors(point cfg.Point, start int, expr ast.Expr, typeref wir.TypeRef) {
	if typeref == 0 || expr == nil {
		return
	}
	inner := sourceprovenance.AssertionInner(expr)
	switch e := inner.(type) {
	case *ast.TableExpr:
		b.setTableConstructorType(point, start, expressionid.Of(e), typeref)
	case *ast.LogicalOpExpr:
		if !b.logicalRHSPure(e.Rhs) {
			if guard, hasGuard := b.guardByCond[e.Lhs]; hasGuard {
				b.attachContextTypeToConstructors(guard, 0, e.Lhs, typeref)
			}
			if anchor, hasAnchor := b.rhsAnchorPoint(e.Rhs); hasAnchor {
				b.attachContextTypeToConstructors(anchor, 0, e.Rhs, typeref)
			}
			return
		}
		b.attachContextTypeToConstructors(point, start, e.Lhs, typeref)
		b.attachContextTypeToConstructors(point, start, e.Rhs, typeref)
	case *ast.CastExpr:
		b.attachContextTypeToConstructors(point, start, e.Expr, typeref)
	case *ast.NonNilAssertExpr:
		b.attachContextTypeToConstructors(point, start, e.Expr, typeref)
	}
}

func (b *builder) setTableConstructorType(point cfg.Point, start int, id wir.ExpressionID, typeref wir.TypeRef) {
	if id == 0 || typeref == 0 {
		return
	}
	insts := b.pointInstrs[point]
	for i := start; i < len(insts); i++ {
		if insts[i].Op == wir.OpMakeTable && insts[i].ExprID == id {
			insts[i].Type = typeref
		}
	}
	b.pointInstrs[point] = insts
}

func (b *builder) dynamicIndexWriteContextType(target pathexpr.DynamicMutationTarget) wir.TypeRef {
	container, ok := b.declaredPathType(target.Table)
	if !ok {
		return 0
	}
	value, ok := luatypeprojection.DynamicWriteValueType(container, b.dynamicIndexKeyType(target.Key))
	if !ok || value == nil {
		return 0
	}
	if len(target.Suffix) != 0 {
		projected, ok := luatypeprojection.ApplyWriteSegments(value, target.Suffix)
		if !ok || projected == nil {
			return 0
		}
		value = projected
	}
	return b.body.InternType(value)
}

func (b *builder) declaredPathType(p path.Path) (typ.Type, bool) {
	if b == nil || b.bindings == nil || b.resolver == nil || p.Symbol == 0 {
		return nil, false
	}
	decl, ok := b.bindings.SymbolTypeAnnotation(p.Symbol)
	if !ok || decl == nil {
		return nil, false
	}
	root, ok := b.resolver.Type(decl)
	if !ok || root == nil {
		return nil, false
	}
	if len(p.Segments) == 0 {
		return root, true
	}
	return luatypeprojection.ApplySegments(root, p.Segments)
}

func (b *builder) dynamicIndexKeyType(expr ast.Expr) typ.Type {
	if key, ok := valueexpr.LiteralType(expr); ok {
		return key
	}
	return nil
}

func (b *builder) rootAssignKind(dst wir.Operand, v binding) wir.AssignKind {
	if dst.Kind != wir.OperandPath || b == nil || b.body == nil {
		return wir.AssignNone
	}
	p := b.body.Path(wir.PathRef(dst.Ref))
	if p.IsEmpty() || len(p.Segments) != 0 {
		return wir.AssignNone
	}
	switch v.targetKind {
	case wir.CallResultTargetLocalAssignment:
		return wir.AssignLocalDeclaration
	case wir.CallResultTargetOrdinaryAssignment:
		return wir.AssignOrdinaryRootWrite
	default:
		return wir.AssignNone
	}
}

func (b *builder) markRootAssignKind(point cfg.Point, start int, dst wir.Operand, kind wir.AssignKind) {
	if kind == wir.AssignNone || dst.Kind != wir.OperandPath {
		return
	}
	insts := b.pointInstrs[point]
	for i := start; i < len(insts); i++ {
		if rootAssignmentSourceInstruction(insts[i]) && insts[i].Dst == dst && insts[i].WritesAssignmentPoint() {
			insts[i].Assign = kind
		}
	}
	b.pointInstrs[point] = insts
}

func rootAssignmentSourceInstruction(inst wir.Instruction) bool {
	if _, ok := inst.AssignmentSourceOperand(); ok {
		return true
	}
	switch inst.Op {
	case wir.OpDynamicIndexRead, wir.OpMakeTable, wir.OpBinOp, wir.OpUnOp, wir.OpConcat, wir.OpClaim, wir.OpSelect, wir.OpLogical, wir.OpClosure:
		return true
	default:
		return false
	}
}

func (b *builder) emitBindingClaim(dst wir.Operand, v binding) {
	if v.claim == wir.ClaimNone {
		return
	}
	b.emit(wir.Instruction{
		Op:    wir.OpClaim,
		Dst:   dst,
		A:     v.op,
		Claim: v.claim,
		Type:  v.claimType,
	})
}

func (b *builder) recordCallResultTarget(dst wir.Operand, v binding) {
	if !v.hasCallResult || dst.Kind != wir.OperandPath {
		return
	}
	b.recordCallResultTargetPath(v, b.body.Path(wir.PathRef(dst.Ref)))
}

func (b *builder) recordCallResultTargetPath(v binding, p path.Path) {
	if !v.hasCallResult {
		return
	}
	b.body.SetCallResultTarget(v.callPoint, wir.CallResultTarget{
		Kind:        v.targetKind,
		Index:       v.targetIndex,
		ResultIndex: v.resultIndex,
		Path:        p,
	})
}

// bindingOperand reduces a planned value to a single operand (for member/index
// writes, which take the value as an operand rather than a destination).
func (b *builder) bindingOperand(v binding) wir.Operand {
	switch v.kind {
	case bindExpr:
		point := b.curPoint
		start := len(b.pointInstrs[point])
		op := b.lowerExpr(v.expr)
		b.attachContextTypeToConstructors(point, start, v.expr, v.contextType)
		return op
	case bindOperand:
		return v.op
	case bindVararg:
		return wir.Operand{Kind: wir.OperandVararg}
	default:
		return b.constNil()
	}
}

// ---- call pre-lowering --------------------------------------------------

func (b *builder) countCalls(exprs []ast.Expr) int {
	calls, ok := callorder.ValueList(exprs, b.callOrderOptions())
	if !ok {
		return -1
	}
	return len(calls)
}

// preLowerAssignCalls emits an OpCall for every call under a value list (in Lua
// evaluation order) onto its NodeCall point. The top-level tail call binds one
// result temp per remaining destination (adjusted multret); every other call
// binds a single head temp.
func (b *builder) preLowerAssignCalls(exprs []ast.Expr, callPoints []cfg.Point, targetCount int) {
	calls, ok := callorder.ValueList(exprs, b.callOrderOptions())
	if !ok || len(calls) != len(callPoints) {
		return
	}
	last := len(exprs) - 1
	for i, occ := range calls {
		results := 1
		topLevel := isValueListRootCall(exprs, occ)
		context := valueListCallContext(exprs, occ, wir.CallContextAssignmentSource)
		meta := b.callMetadata(context, exprs, occ.ExprIndex, occ.Call, false)
		if occ.ExprIndex == last && last < targetCount && topLevel {
			results = targetCount - last
		}
		b.emitCallAt(callPoints[i], occ.Call, results, meta)
	}
}

// preLowerExprCalls emits an OpCall for every call under a single expression
// (condition, return value, iterator source) onto its NodeCall point, each
// binding one head temp. Tail spread is applied later by the value-list lowering.
func (b *builder) preLowerExprCalls(expr ast.Expr, callPoints []cfg.Point, context wir.CallContextKind) {
	calls, ok := callorder.Expr(expr, b.callOrderOptions())
	if !ok || len(calls) != len(callPoints) {
		return
	}
	for i, occ := range calls {
		callContext := exprCallContext(expr, occ.Call, context)
		b.emitCallAt(callPoints[i], occ.Call, 1, b.callMetadata(callContext, []ast.Expr{expr}, 0, occ.Call, false))
	}
}

// preLowerListCalls emits an OpCall for every call under a value list, each
// binding one head temp (returns / iterator sources / statement expressions).
func (b *builder) preLowerListCalls(exprs []ast.Expr, callPoints []cfg.Point, context wir.CallContextKind, openTail bool) {
	calls, ok := callorder.ValueList(exprs, b.callOrderOptions())
	if !ok || len(calls) != len(callPoints) {
		return
	}
	for i, occ := range calls {
		callContext := valueListCallContext(exprs, occ, context)
		b.emitCallAt(callPoints[i], occ.Call, 1, b.callMetadata(callContext, exprs, occ.ExprIndex, occ.Call, openTail))
	}
}

type callMetadata struct {
	context          wir.CallContextKind
	expr             int
	final            bool
	expanded         bool
	adjusted         bool
	openTail         bool
	conditionNegated bool
}

func (b *builder) callMetadata(context wir.CallContextKind, exprs []ast.Expr, exprIndex int, call *ast.FuncCallExpr, openTailFinal bool) callMetadata {
	meta := callMetadata{context: context, expr: exprIndex}
	final := true
	allowExpansion := false
	openTail := false
	switch context {
	case wir.CallContextAssignmentSource, wir.CallContextReturnSource, wir.CallContextIteratorSource:
		final = exprIndex >= 0 && exprIndex == len(exprs)-1
		allowExpansion = true
		openTail = context == wir.CallContextReturnSource && openTailFinal && final
	case wir.CallContextStatement, wir.CallContextCondition, wir.CallContextExpressionProducer:
		final = true
	default:
		final = false
	}
	expanded, adjusted, shapedOpenTail := sourceprovenance.ValueShape(call, final, allowExpansion, openTail)
	meta.final = final
	meta.expanded = expanded
	meta.adjusted = adjusted
	meta.openTail = shapedOpenTail
	if context == wir.CallContextExpressionProducer {
		meta.expr = exprIndex
	}
	if context == wir.CallContextCondition && meta.expr < 0 {
		meta.expr = 0
	}
	if context == wir.CallContextCondition && len(exprs) != 0 {
		predicate, negated, ok := branchcond.PredicateCall(exprs[0])
		meta.conditionNegated = ok && predicate == call && negated
	}
	return meta
}

// emitCallAt lowers one call onto its dedicated point, binding results result
// temps. resultCount == 0 is a statement call whose results are discarded.
func (b *builder) emitCallAt(point cfg.Point, call *ast.FuncCallExpr, resultCount int, meta callMetadata) {
	prev := b.curPoint
	b.curPoint = point
	defer func() { b.curPoint = prev }()

	temps := make([]wir.Operand, resultCount)
	for i := range temps {
		temps[i] = b.newTemp()
	}
	head := wir.Operand{}
	if len(temps) > 0 {
		head = temps[0]
	}
	cr := &callResult{head: head, temps: temps, point: point}
	b.callTemps[call] = cr

	if resultCount >= 1 && b.maybeLowerSelect(head, call) {
		cr.index = len(b.pointInstrs[point]) - 1
		return
	}

	args, argSpread := b.lowerValueList(call.Args)
	inst := wir.Instruction{
		Op:                   wir.OpCall,
		List:                 b.body.AppendOperands(args),
		Results:              b.body.AppendOperands(temps),
		ListSpread:           argSpread,
		ExprID:               expressionid.Of(call),
		CallContext:          meta.context,
		CallExpr:             meta.expr,
		CallFinal:            meta.final,
		CallExpanded:         meta.expanded,
		CallAdjusted:         meta.adjusted,
		CallOpenTail:         meta.openTail,
		CallConditionNegated: meta.conditionNegated,
		CallSpan:             tableEntryValueSpan(call),
		CalleeSpan:           callCalleeSourceSpan(call),
		CallArgs:             b.body.AppendCallArgumentMeta(callArgumentMeta(call.Args)),
		CallTypeArgs:         b.body.AppendTypeRefs(b.internTypes(call.TypeArgs)),
	}
	if call.Method != "" {
		inst.Call.Method = b.internString(call.Method)
		if call.Receiver != nil {
			inst.Call.Receiver = b.lowerExpr(call.Receiver)
			if receiverType, ok := b.typeValueRef(call.Receiver); ok {
				inst.Type = receiverType
			}
		}
	} else {
		inst.Call.Callee = b.calleeOperand(call.Func)
		if calleeType, ok := b.typeValueRef(call.Func); ok {
			inst.Type = calleeType
		}
	}
	if check, ok := b.assertCallCheck(call, meta); ok {
		inst.Check = b.body.InternCheck(lowerCheck(check))
	}
	cr.index = len(b.pointInstrs[point])
	b.emit(inst)
	b.recordContextCallResultTarget(point, resultCount, meta)
}

func (b *builder) assertCallCheck(call *ast.FuncCallExpr, meta callMetadata) (branchcond.Check, bool) {
	if b == nil || b.bindings == nil || call == nil || meta.context != wir.CallContextStatement ||
		call.Method != "" || len(call.Args) == 0 {
		return branchcond.Check{}, false
	}
	callee, ok := call.Func.(*ast.IdentExpr)
	if !ok || !b.bindings.ResolvesToGlobal(callee, "assert") {
		return branchcond.Check{}, false
	}
	check := branchcond.Normalize(call.Args[0], b.bindings)
	if check.Kind == branchcond.CheckNone {
		return branchcond.Check{}, false
	}
	return check, true
}

func (b *builder) recordContextCallResultTarget(point cfg.Point, resultCount int, meta callMetadata) {
	if resultCount == 0 {
		return
	}
	target := wir.CallResultTarget{
		Index:       meta.expr,
		ResultIndex: 0,
	}
	switch meta.context {
	case wir.CallContextReturnSource:
		target.Kind = wir.CallResultTargetReturn
	case wir.CallContextExpressionProducer:
		target.Kind = wir.CallResultTargetExpression
	default:
		return
	}
	b.body.SetCallResultTarget(point, target)
}

// ---- calls, statement position -----------------------------------------

func (b *builder) lowerCallStmt(s *ast.FuncCallStmt) {
	pts := b.stmtPoints(s)
	call, ok := s.Expr.(*ast.FuncCallExpr)
	if !ok {
		return
	}
	calls, ok := callorder.Expr(s.Expr, b.callOrderOptions())
	if !ok || len(calls) != len(pts) {
		return
	}
	top, _ := sourceprovenance.Call(s.Expr)
	for i, occ := range calls {
		results := 1
		context := wir.CallContextExpressionProducer
		if occ.Call == top || occ.Call == call {
			context = wir.CallContextStatement
			results = 0
		}
		b.emitCallAt(pts[i], occ.Call, results, b.callMetadata(context, []ast.Expr{s.Expr}, 0, occ.Call, false))
	}
}

// ---- return -------------------------------------------------------------

func (b *builder) lowerReturn(s *ast.ReturnStmt) {
	pts := b.stmtPoints(s)
	nCalls := b.countCalls(s.Exprs)
	if len(pts) != nCalls+1 {
		return
	}
	b.preLowerListCalls(s.Exprs, pts[:nCalls], wir.CallContextReturnSource, true)
	b.curPoint = pts[nCalls]
	ops, spread := b.lowerValueList(s.Exprs)
	b.emit(wir.Instruction{
		Op:         wir.OpReturn,
		List:       b.body.AppendOperands(ops),
		ListSpread: spread,
	})
}

// ---- branches -----------------------------------------------------------

func (b *builder) lowerIf(s *ast.IfStmt) {
	pts := b.stmtPoints(s)
	nCalls := b.condCallCount(s.Condition)
	if len(pts) != nCalls+1 {
		return
	}
	b.preLowerExprCalls(s.Condition, pts[:nCalls], wir.CallContextCondition)
	b.curPoint = pts[nCalls]
	b.emitBranch(s.Condition)
	b.lowerStmts(s.Then)
	b.lowerStmts(s.Else)
}

func (b *builder) lowerWhile(s *ast.WhileStmt) {
	pts := b.stmtPoints(s)
	nCalls := b.condCallCount(s.Condition)
	if len(pts) != nCalls+1 {
		return
	}
	b.preLowerExprCalls(s.Condition, pts[:nCalls], wir.CallContextCondition)
	b.curPoint = pts[nCalls]
	b.emitBranch(s.Condition)
	b.lowerStmts(s.Stmts)
}

func (b *builder) lowerRepeat(s *ast.RepeatStmt) {
	pts := b.stmtPoints(s)
	nCalls := b.condCallCount(s.Condition)
	if len(pts) != nCalls+1 {
		return
	}
	b.lowerStmts(s.Stmts)
	b.preLowerExprCalls(s.Condition, pts[:nCalls], wir.CallContextCondition)
	b.curPoint = pts[nCalls]
	b.emitBranch(s.Condition)
}

func (b *builder) condCallCount(cond ast.Expr) int {
	calls, ok := callorder.Expr(cond, b.callOrderOptions())
	if !ok {
		return -1
	}
	return len(calls)
}

func (b *builder) emitBranch(cond ast.Expr) {
	check := branchcond.Normalize(cond, b.bindings)
	inst := wir.Instruction{
		Op:              wir.OpBranch,
		Check:           b.body.InternCheck(lowerCheck(check)),
		ImpliedChecks:   b.body.AppendImpliedChecks(lowerImpliedChecks(branchcond.ImpliedChecksOnBothEdges(cond, b.bindings))),
		DiffConstraints: b.body.AppendBranchDiffConstraints(lowerBranchDiffConstraints(branchcond.BranchDiffConstraintsOnBothEdges(cond, b.bindings))),
	}
	if check.Kind == branchcond.CheckNone {
		inst.A = b.lowerExpr(cond)
	}
	b.emit(inst)
}

// ---- loops --------------------------------------------------------------

func (b *builder) lowerNumberFor(s *ast.NumberForStmt) {
	pts := b.stmtPoints(s)
	bounds := numericForBounds(s)
	nCalls := b.countCalls(bounds)
	if len(pts) != nCalls+2 {
		return
	}
	b.preLowerListCalls(bounds, pts[:nCalls], wir.CallContextIteratorSource, false)
	// preheader (NodeAssign): the loop-variable binding (value derived by transfer
	// from the iterator header on the branch point).
	b.curPoint = pts[nCalls]
	b.emitAssign(b.numForOperand(s), wir.Operand{})
	// branch (NodeBranch): the numeric iterator header carrying the bounds.
	b.curPoint = pts[nCalls+1]
	list := []wir.Operand{b.lowerExpr(s.Init), b.lowerExpr(s.Limit)}
	if s.Step != nil {
		list = append(list, b.lowerExpr(s.Step))
	} else {
		list = append(list, b.constNumber("1"))
	}
	b.emit(wir.Instruction{
		Op:      wir.OpIterate,
		Iter:    wir.IterNumeric,
		Results: b.body.AppendOperands([]wir.Operand{b.numForOperand(s)}),
		List:    b.body.AppendOperands(list),
	})
	b.lowerStmts(s.Stmts)
}

func (b *builder) lowerGenericFor(s *ast.GenericForStmt) {
	pts := b.stmtPoints(s)
	nCalls := b.countCalls(s.Exprs)
	if len(pts) != nCalls+1+len(s.Names) {
		return
	}
	b.preLowerListCalls(s.Exprs, pts[:nCalls], wir.CallContextIteratorSource, false)
	// branch (NodeBranch): the generic iterator header carrying the sources.
	b.curPoint = pts[nCalls]
	srcOps, spread := b.lowerValueList(s.Exprs)
	b.emit(wir.Instruction{
		Op:         wir.OpIterate,
		Iter:       wir.IterGeneric,
		List:       b.body.AppendOperands(srcOps),
		ListSpread: spread,
	})
	// one NodeAssign per loop variable: the variable binding.
	varOps := b.genericForOperands(s)
	for i := range s.Names {
		b.curPoint = pts[nCalls+1+i]
		b.emitAssign(varOps[i], wir.Operand{})
	}
	b.lowerStmts(s.Stmts)
}

// ---- function definitions ----------------------------------------------

func (b *builder) lowerFuncDef(s *ast.FuncDefStmt) {
	pts := b.stmtPoints(s)
	if len(pts) != 1 {
		return
	}
	b.curPoint = pts[0]
	p, ok := pathexpr.ResolveFuncName(s.Name, b.bindings)
	if ok && len(p.Segments) == 0 {
		dst := b.pathOperand(p)
		start := len(b.pointInstrs[b.curPoint])
		b.emitClosure(dst, s.Func)
		b.markRootAssignKind(b.curPoint, start, dst, wir.AssignOrdinaryRootWrite)
		return
	}
	tmp := b.newTemp()
	b.emitClosure(tmp, s.Func)
	if ok {
		b.emit(wir.Instruction{Op: wir.OpStaticMemberWrite, Dst: b.pathOperand(p), A: tmp})
	}
}

// emitClosure lowers fn into its own proto Body (built on its own cfgbuild
// graph) and emits an OpClosure into dst carrying the capture operands in bind
// order.
func (b *builder) emitClosure(dst wir.Operand, fn *ast.FunctionExpr) {
	name := b.body.Name + ".fn" + strconv.Itoa(b.protoSeq)
	b.protoSeq++
	childBuilt := cfgbuild.BuildFunction(fn, b.bindings)
	var ref wir.FuncRef
	if childBuilt != nil && childBuilt.Graph != nil {
		childBody := lowerInto(name, fn.Stmts, b.bindings, childBuilt, b.resolver)
		sym, _ := b.bindings.FunctionSymbol(fn)
		fnType, _ := functiontype.ValueExpression(fn, b.bindings, b.resolver)
		ref = b.body.AddProto(wir.FuncProto{Name: name, Body: childBody, Graph: childBuilt.Graph, Symbol: wir.FunctionSymbolID(sym), Type: fnType})
	}

	caps := b.bindings.DirectCaptures(fn)
	ops := make([]wir.Operand, 0, len(caps))
	for _, c := range caps {
		ops = append(ops, b.pathOperand(path.NewPath(c.Captured, c.CapturedName)))
	}
	b.emit(wir.Instruction{
		Op:     wir.OpClosure,
		Dst:    dst,
		Func:   ref,
		List:   b.body.AppendOperands(ops),
		ExprID: expressionid.Of(fn),
	})
}

// ---- channel select recognition ----------------------------------------

// maybeLowerSelect recognizes the ambient channel.select runtime call and emits
// an OpSelect over its recognized receive-case channels. It returns false when
// call is not a select so the caller lowers it as an ordinary call.
func (b *builder) maybeLowerSelect(dst wir.Operand, call *ast.FuncCallExpr) bool {
	if !channelruntime.IsSelectCall(call, b.bindings) {
		return false
	}
	table, ok := call.Args[0].(*ast.TableExpr)
	if !ok {
		return false
	}
	ops := make([]wir.Operand, 0, len(table.Fields))
	hasDefault := false
	for _, f := range table.Fields {
		if f == nil {
			continue
		}
		if f.Key != nil && ast.KeyName(f.Key) == "default" {
			hasDefault = true
			continue
		}
		cc, ok := f.Value.(*ast.FuncCallExpr)
		if !ok || !channelruntime.IsReceiveCaseCandidate(cc, b.bindings) {
			continue
		}
		if p, ok := pathexpr.Resolve(cc.Receiver, b.bindings); ok && !p.IsEmpty() {
			ops = append(ops, b.pathOperand(p))
		}
	}
	b.emit(wir.Instruction{
		Op:            wir.OpSelect,
		Dst:           dst,
		List:          b.body.AppendOperands(ops),
		SelectDefault: hasDefault,
		ExprID:        expressionid.Of(call),
	})
	return true
}

// ---- expression lowering ------------------------------------------------

// lowerExprInto lowers e so its value lands in dst, choosing the producing
// instruction by syntax. Calls are already pre-lowered at their own points, so
// a call expression here copies its head result temp.
func (b *builder) lowerExprInto(dst wir.Operand, e ast.Expr) {
	switch e := e.(type) {
	case *ast.NilExpr, *ast.TrueExpr, *ast.FalseExpr, *ast.NumberExpr, *ast.StringExpr:
		b.emitAssign(dst, b.constOperand(e))
	case *ast.IdentExpr:
		b.emitAssign(dst, b.readOperand(e))
	case *ast.AttrGetExpr:
		if p, ok := pathexpr.Resolve(e, b.bindings); ok {
			b.emitAssign(dst, b.pathOperand(p))
			return
		}
		b.emitDynamicIndexRead(dst, e)
	case *ast.ArithmeticOpExpr:
		b.emitBinOp(dst, arithOperator(e.Operator), e.Lhs, e.Rhs)
	case *ast.RelationalOpExpr:
		b.emitRelOp(dst, e)
	case *ast.StringConcatOpExpr:
		ops := b.flattenConcat(e)
		b.emit(wir.Instruction{Op: wir.OpConcat, Dst: dst, List: b.body.AppendOperands(ops)})
	case *ast.UnaryMinusOpExpr:
		b.emitUnOp(dst, wir.UnNeg, e.Expr)
	case *ast.UnaryNotOpExpr:
		b.emitUnOp(dst, wir.UnNot, e.Expr)
	case *ast.UnaryLenOpExpr:
		b.emitUnOp(dst, wir.UnLen, e.Expr)
	case *ast.UnaryBNotOpExpr:
		b.emitUnOp(dst, wir.UnBNot, e.Expr)
	case *ast.FuncCallExpr:
		b.emitAssign(dst, b.callValue(e))
	case *ast.CastExpr:
		b.emit(wir.Instruction{Op: wir.OpClaim, Dst: dst, A: b.lowerExpr(e.Expr), Claim: wir.ClaimCast, Type: b.internType(e.Type)})
	case *ast.NonNilAssertExpr:
		b.emit(wir.Instruction{Op: wir.OpClaim, Dst: dst, A: b.lowerExpr(e.Expr), Claim: wir.ClaimAssert})
	case *ast.LogicalOpExpr:
		b.lowerLogicalInto(dst, e)
	case *ast.FunctionExpr:
		b.emitClosure(dst, e)
	case *ast.TableExpr:
		b.lowerTable(dst, e)
	case *ast.Comma3Expr:
		b.emitAssign(dst, wir.Operand{Kind: wir.OperandVararg})
	default:
		b.emitAssign(dst, wir.Operand{})
	}
}

// lowerExpr lowers e to an operand, allocating a temp for compound expressions.
func (b *builder) lowerExpr(e ast.Expr) wir.Operand {
	switch e := e.(type) {
	case *ast.NilExpr, *ast.TrueExpr, *ast.FalseExpr, *ast.NumberExpr, *ast.StringExpr:
		return b.constOperand(e)
	case *ast.IdentExpr:
		return b.readOperand(e)
	case *ast.AttrGetExpr:
		if p, ok := pathexpr.Resolve(e, b.bindings); ok {
			return b.pathOperand(p)
		}
		t := b.newTemp()
		b.lowerExprInto(t, e)
		return t
	case *ast.FuncCallExpr:
		return b.callValue(e)
	case *ast.LogicalOpExpr:
		if b.logicalRHSPure(e.Rhs) {
			if value, ok := b.logicalValues[e]; ok {
				return value
			}
			t := b.newTemp()
			b.emitLogicalGuard(e)
			b.emit(wir.Instruction{Op: wir.OpLogical, Dst: t, A: b.lowerExpr(e.Lhs), B: b.lowerExpr(e.Rhs), Operator: logicalOperator(e)})
			b.logicalValues[e] = t
			return t
		}
		return b.lowerLogicalValue(e)
	case *ast.Comma3Expr:
		return wir.Operand{Kind: wir.OperandVararg}
	default:
		t := b.newTemp()
		b.lowerExprInto(t, e)
		return t
	}
}

// callValue returns the head result temp of a call already lowered at its point.
func (b *builder) callValue(call *ast.FuncCallExpr) wir.Operand {
	if cr, ok := b.callTemps[call]; ok {
		return cr.head
	}
	return wir.Operand{}
}

// lowerMultiValue lowers a tail-position expression that expands to all its
// values, marking the producing call as multret. It returns the head operand.
func (b *builder) lowerMultiValue(e ast.Expr) wir.Operand {
	switch e := e.(type) {
	case *ast.FuncCallExpr:
		if cr, ok := b.callTemps[e]; ok {
			b.markSpread(cr)
			return cr.head
		}
		return wir.Operand{}
	case *ast.Comma3Expr:
		return wir.Operand{Kind: wir.OperandVararg}
	default:
		return b.lowerExpr(e)
	}
}

// markSpread flips a pre-lowered call's ResultSpread once it is discovered in a
// spread (open multret) tail position.
func (b *builder) markSpread(cr *callResult) {
	insts := b.pointInstrs[cr.point]
	if cr.index >= 0 && cr.index < len(insts) && insts[cr.index].Op == wir.OpCall {
		insts[cr.index].ResultSpread = true
	}
}

// lowerValueList lowers an expression list, expanding a final multi-value
// producer. It returns the operands and whether the tail is an open spread.
func (b *builder) lowerValueList(exprs []ast.Expr) ([]wir.Operand, bool) {
	if len(exprs) == 0 {
		return nil, false
	}
	ops := make([]wir.Operand, 0, len(exprs))
	spread := false
	last := len(exprs) - 1
	for i, e := range exprs {
		if i == last && ast.CanProduceMultipleValues(e) {
			ops = append(ops, b.lowerMultiValue(e))
			spread = true
			continue
		}
		ops = append(ops, b.lowerExpr(e))
	}
	return ops, spread
}

// ---- short-circuit and/or (decision D3) ---------------------------------

// lowerLogicalInto lowers a short-circuit and/or into dst. A conservatively pure
// right operand (literal or plain identifier read) keeps the single OpLogical
// value form; an effectful right operand maps onto the branch topology cfgbuild
// already materialized (guard branch + RHS-eval point + join).
func (b *builder) lowerLogicalInto(dst wir.Operand, e *ast.LogicalOpExpr) {
	if b.logicalRHSPure(e.Rhs) {
		b.emitLogicalGuard(e)
		if value, ok := b.logicalValues[e]; ok {
			b.emitAssign(dst, value)
			return
		}
		b.emit(wir.Instruction{Op: wir.OpLogical, Dst: dst, A: b.lowerExpr(e.Lhs), B: b.lowerExpr(e.Rhs), Operator: logicalOperator(e)})
		b.logicalValues[e] = dst
		return
	}
	b.emitAssign(dst, b.lowerLogicalValue(e))
}

// lowerLogicalValue lowers a logical to a result operand. The effectful form
// threads the short-circuit result through one temp: the guard point assigns the
// left operand and branches on it; the taken edge (the RHS-eval or last RHS call
// point) overwrites the temp with the right operand; the CFG join merges. The
// bypass edge carries no point, so retaining the left operand's assignment before
// the branch models effect gating without a phi.
func (b *builder) lowerLogicalValue(e *ast.LogicalOpExpr) wir.Operand {
	guard, hasGuard := b.guardByCond[e.Lhs]
	anchor, hasAnchor := b.rhsAnchorPoint(e.Rhs)
	if !hasGuard || !hasAnchor {
		// The topology cfgbuild expected is absent (e.g. a shape callorder
		// rejected): fall back to the value form so the result is still bound.
		t := b.newTemp()
		b.emit(wir.Instruction{Op: wir.OpLogical, Dst: t, A: b.lowerExpr(e.Lhs), B: b.lowerExpr(e.Rhs), Operator: logicalOperator(e)})
		return t
	}
	result := b.newTemp()
	prev := b.curPoint

	b.curPoint = guard
	left := b.lowerExpr(e.Lhs)
	b.emitAssign(result, left)
	b.emitLogicalGuardAtOperand(e, guard, left)

	b.curPoint = anchor
	b.lowerExprInto(result, e.Rhs)

	b.curPoint = prev
	b.logicalValues[e] = result
	return result
}

func (b *builder) emitLogicalGuard(e *ast.LogicalOpExpr) {
	if e == nil {
		return
	}
	guard, ok := b.guardByCond[e.Lhs]
	if !ok {
		return
	}
	prev := b.curPoint
	b.curPoint = guard
	b.emitLogicalGuardAt(e, guard)
	b.curPoint = prev
}

func (b *builder) emitLogicalGuardAt(e *ast.LogicalOpExpr, guard cfg.Point) {
	b.emitLogicalGuardAtOperand(e, guard, wir.Operand{})
}

func (b *builder) emitLogicalGuardAtOperand(e *ast.LogicalOpExpr, guard cfg.Point, operand wir.Operand) {
	if e == nil || b.logicalGuardEmitted[e] {
		return
	}
	b.logicalGuardEmitted[e] = true
	check := branchcond.Normalize(e.Lhs, b.bindings)
	guardInst := wir.Instruction{
		Op:              wir.OpBranch,
		Check:           b.body.InternCheck(lowerCheck(check)),
		ImpliedChecks:   b.body.AppendImpliedChecks(lowerImpliedChecks(branchcond.ImpliedChecksOnBothEdges(e.Lhs, b.bindings))),
		DiffConstraints: b.body.AppendBranchDiffConstraints(lowerBranchDiffConstraints(branchcond.BranchDiffConstraintsOnBothEdges(e.Lhs, b.bindings))),
	}
	if check.Kind == branchcond.CheckNone {
		if operand.Kind != wir.OperandNone {
			guardInst.A = operand
		} else {
			guardInst.A = b.lowerExpr(e.Lhs)
		}
	}
	b.curPoint = guard
	b.emit(guardInst)
}

// rhsAnchorPoint returns the taken-edge point where a short-circuit right operand
// is evaluated: the last call point when the RHS carries calls, otherwise the
// structural expression-evaluation point cfgbuild recorded for it.
func (b *builder) rhsAnchorPoint(rhs ast.Expr) (cfg.Point, bool) {
	if calls, ok := callorder.Expr(rhs, b.callOrderOptions()); ok && len(calls) > 0 {
		if cr, ok := b.callTemps[calls[len(calls)-1].Call]; ok {
			return cr.point, true
		}
	}
	if p, ok := b.evalByExpr[rhs]; ok {
		return p, true
	}
	return 0, false
}

// logicalRHSPure reports whether a logical operator's short-circuited right
// operand is conservatively pure: only literals and plain identifier reads. A
// member read is impure (its __index can be an arbitrary metamethod); index
// reads, calls, nested logicals, and any compound expression are impure.
func (b *builder) logicalRHSPure(rhs ast.Expr) bool {
	switch sourceprovenance.AssertionInner(rhs).(type) {
	case *ast.NilExpr, *ast.TrueExpr, *ast.FalseExpr, *ast.NumberExpr, *ast.StringExpr:
		return true
	case *ast.IdentExpr:
		return true
	default:
		return false
	}
}

func logicalOperator(e *ast.LogicalOpExpr) wir.Operator {
	if e.Operator == "or" {
		return wir.LogOr
	}
	return wir.LogAnd
}

// ---- table constructors -------------------------------------------------

// lowerTable lowers a table constructor over its array, hash, and trailing
// spread parts. Every field value becomes a List operand; a final keyless
// multi-value producer marks the list tail as an open spread. Static entry
// suffixes are carried in WIR as constructor metadata so transfer does not need
// to rediscover field keys from the AST.
func (b *builder) lowerTable(dst wir.Operand, t *ast.TableExpr) {
	last := lastFieldIndex(t.Fields)
	ops := make([]wir.Operand, 0, len(t.Fields))
	entryByField := map[*ast.Field]path.Path{}
	for _, entry := range pathexpr.ObjectEntries(t) {
		if entry.Field != nil {
			entryByField[entry.Field] = entry.Suffix
		}
	}
	entries := make([]wir.TableEntry, 0, len(entryByField))
	spread := false
	for i, f := range t.Fields {
		if f == nil || f.Value == nil {
			continue
		}
		var value wir.Operand
		if i == last && f.Key == nil && ast.CanProduceMultipleValues(f.Value) {
			value = b.lowerMultiValue(f.Value)
			spread = true
		} else {
			value = b.lowerExpr(f.Value)
		}
		ops = append(ops, value)
		if suffix, ok := entryByField[f]; ok {
			entries = append(entries, wir.TableEntry{
				Suffix:     suffix,
				Value:      value,
				ValueSpan:  tableEntryValueSpan(f.Value),
				ValueLabel: tableEntryValueLabel(f.Value),
			})
			for _, nested := range b.tableEntriesForProducedOperand(value) {
				entries = append(entries, wir.TableEntry{
					Suffix:     suffix.AppendPathSuffix(nested.Suffix),
					Value:      nested.Value,
					ValueSpan:  nested.ValueSpan,
					ValueLabel: nested.ValueLabel,
				})
			}
		}
	}
	b.emit(wir.Instruction{
		Op:           wir.OpMakeTable,
		Dst:          dst,
		List:         b.body.AppendOperands(ops),
		TableEntries: b.body.AppendTableEntries(entries),
		ListSpread:   spread,
		ExprID:       expressionid.Of(t),
	})
}

func (b *builder) tableEntriesForProducedOperand(op wir.Operand) []wir.TableEntry {
	if op.Kind != wir.OperandTemp {
		return nil
	}
	insts := b.pointInstrs[b.curPoint]
	for i := len(insts) - 1; i >= 0; i-- {
		inst := insts[i]
		if inst.Op != wir.OpMakeTable || inst.Dst != op {
			continue
		}
		return b.body.TableEntries(inst.TableEntries)
	}
	return nil
}

func tableEntryValueSpan(expr ast.Expr) wir.Span {
	if expr == nil {
		return wir.Span{}
	}
	span := ast.SpanOf(expr)
	if ident, ok := expr.(*ast.IdentExpr); ok && span.Valid() && span.EndLine == span.StartLine && span.EndCol <= span.StartCol && ident.Value != "" {
		span.EndCol = span.StartCol + len(ident.Value)
	}
	return wirSpanFromSource(span)
}

func callCalleeSourceSpan(call *ast.FuncCallExpr) wir.Span {
	if call == nil {
		return wir.Span{}
	}
	if span := tableEntryValueSpan(call.Func); span.Valid() {
		return span
	}
	return tableEntryValueSpan(call.Receiver)
}

func wirSpanFromSource(span source.Span) wir.Span {
	return wir.Span{
		StartLine: span.StartLine,
		StartCol:  span.StartCol,
		EndLine:   span.EndLine,
		EndCol:    span.EndCol,
	}
}

func callArgumentMeta(exprs []ast.Expr) []wir.CallArgumentMeta {
	if len(exprs) == 0 {
		return nil
	}
	out := make([]wir.CallArgumentMeta, len(exprs))
	for i, expr := range exprs {
		out[i] = wir.CallArgumentMeta{
			Span:  tableEntryValueSpan(expr),
			Label: tableEntryValueLabel(expr),
		}
	}
	return out
}

func tableEntryValueLabel(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.IdentExpr:
		return e.Value
	case *ast.AttrGetExpr:
		object := tableEntryValueLabel(e.Object)
		key := tableEntryAttrKeyLabel(e)
		if object == "" || key == "" {
			return object
		}
		return object + key
	case *ast.CastExpr:
		return tableEntryValueLabel(e.Expr)
	case *ast.NonNilAssertExpr:
		return tableEntryValueLabel(e.Expr)
	case *ast.FuncCallExpr:
		if tableEntryUnpackCallLabel(e) {
			return "unpack(...)"
		}
		return ""
	default:
		return ""
	}
}

func tableEntryAttrKeyLabel(expr *ast.AttrGetExpr) string {
	if expr == nil {
		return ""
	}
	switch expr.KeySyntax {
	case ast.AttrKeyDot:
		if name := ast.KeyName(expr.Key); name != "" {
			return "." + name
		}
	case ast.AttrKeyIndex:
		switch key := expr.Key.(type) {
		case *ast.StringExpr:
			return "[" + strconv.Quote(key.Value) + "]"
		case *ast.NumberExpr:
			return "[" + key.Value + "]"
		case *ast.IdentExpr:
			return "[" + key.Value + "]"
		}
	}
	if name := ast.KeyName(expr.Key); name != "" {
		return "." + name
	}
	return ""
}

func tableEntryUnpackCallLabel(call *ast.FuncCallExpr) bool {
	if call == nil || call.Method != "" || call.Receiver != nil {
		return false
	}
	if ident, ok := call.Func.(*ast.IdentExpr); ok {
		return ident.Value == "unpack"
	}
	attr, ok := call.Func.(*ast.AttrGetExpr)
	if !ok {
		return false
	}
	obj, ok := attr.Object.(*ast.IdentExpr)
	if !ok || obj.Value != "table" {
		return false
	}
	return ast.KeyName(attr.Key) == "unpack"
}

func lastFieldIndex(fields []*ast.Field) int {
	for i := len(fields) - 1; i >= 0; i-- {
		if fields[i] != nil && fields[i].Value != nil {
			return i
		}
	}
	return -1
}

func (b *builder) flattenConcat(e *ast.StringConcatOpExpr) []wir.Operand {
	var ops []wir.Operand
	var walk func(x ast.Expr)
	walk = func(x ast.Expr) {
		if c, ok := x.(*ast.StringConcatOpExpr); ok {
			walk(c.Lhs)
			walk(c.Rhs)
			return
		}
		ops = append(ops, b.lowerExpr(x))
	}
	walk(e.Lhs)
	walk(e.Rhs)
	return ops
}

// ---- operand encoding ---------------------------------------------------

func (b *builder) emitAssign(dst, src wir.Operand) {
	b.emit(wir.Instruction{Op: wir.OpAssign, Dst: dst, A: src})
}

func (b *builder) emitBinOp(dst wir.Operand, op wir.Operator, lhs, rhs ast.Expr) {
	a := b.lowerExpr(lhs)
	c := b.lowerExpr(rhs)
	b.emit(wir.Instruction{Op: wir.OpBinOp, Dst: dst, A: a, B: c, Operator: op})
}

func (b *builder) emitRelOp(dst wir.Operand, expr *ast.RelationalOpExpr) {
	if expr == nil {
		b.emitBinOp(dst, wir.BinEq, nil, nil)
		return
	}
	a := b.lowerExpr(expr.Lhs)
	c := b.lowerExpr(expr.Rhs)
	inst := wir.Instruction{Op: wir.OpBinOp, Dst: dst, A: a, B: c, Operator: relOperator(expr.Operator)}
	if check := branchcond.Normalize(expr, b.bindings); check.Kind != branchcond.CheckNone {
		inst.Check = b.body.InternCheck(lowerCheck(check))
	}
	b.emit(inst)
}

func (b *builder) emitUnOp(dst wir.Operand, op wir.Operator, operand ast.Expr) {
	a := b.lowerExpr(operand)
	b.emit(wir.Instruction{Op: wir.OpUnOp, Dst: dst, A: a, Operator: op})
}

func (b *builder) emitDynamicIndexRead(dst wir.Operand, attr *ast.AttrGetExpr) {
	if attr == nil {
		b.emitAssign(dst, wir.Operand{})
		return
	}
	b.emit(wir.Instruction{
		Op:     wir.OpDynamicIndexRead,
		Dst:    dst,
		A:      b.lowerExpr(attr.Object),
		B:      b.attrKeyOperand(attr),
		ExprID: expressionid.Of(attr),
	})
}

// readOperand returns the operand for a value read: a path operand when the
// expression resolves to a static path, else a temp holding an opaque read.
func (b *builder) readOperand(e ast.Expr) wir.Operand {
	if p, ok := pathexpr.Resolve(e, b.bindings); ok {
		return b.pathOperand(p)
	}
	t := b.newTemp()
	b.emit(wir.Instruction{Op: wir.OpAssign, Dst: t, A: wir.Operand{}})
	return t
}

func (b *builder) attrKeyOperand(attr *ast.AttrGetExpr) wir.Operand {
	if attr == nil || attr.Key == nil {
		return wir.Operand{}
	}
	if attr.KeySyntax == ast.AttrKeyDot {
		if name := ast.KeyName(attr.Key); name != "" {
			return b.pooledConst(wir.Const{Kind: wir.ConstString, Str: name})
		}
	}
	switch key := attr.Key.(type) {
	case *ast.StringExpr:
		return b.constOperand(key)
	case *ast.NumberExpr:
		return b.constOperand(key)
	}
	return b.lowerExpr(attr.Key)
}

func (b *builder) calleeOperand(e ast.Expr) wir.Operand {
	if p, ok := pathexpr.Resolve(e, b.bindings); ok {
		return b.pathOperand(p)
	}
	if id, ok := e.(*ast.IdentExpr); ok {
		return b.pathOperand(path.Path{Root: id.Value})
	}
	return b.lowerExpr(e)
}

func (b *builder) typeValueRef(e ast.Expr) (wir.TypeRef, bool) {
	if b == nil || b.resolver == nil || e == nil {
		return 0, false
	}
	if ident, ok := e.(*ast.IdentExpr); ok && b.bindings != nil {
		if decl, ok := b.bindings.TypeValueRef(ident); ok {
			if t, ok := b.resolver.Decl(decl); ok {
				ref := b.body.InternType(t)
				return ref, ref != 0
			}
		}
	}
	parts, ok := valueexpr.TypeValueRefParts(e)
	if !ok {
		return 0, false
	}
	t, ok := b.resolver.ResolveTypeRef(parts)
	if !ok {
		return 0, false
	}
	ref := b.body.InternType(t)
	return ref, ref != 0
}

func (b *builder) constOperand(e ast.Expr) wir.Operand {
	switch e := e.(type) {
	case *ast.NilExpr:
		return b.pooledConst(wir.Const{Kind: wir.ConstNil})
	case *ast.TrueExpr:
		return b.pooledConst(wir.Const{Kind: wir.ConstBool, Bool: true})
	case *ast.FalseExpr:
		return b.pooledConst(wir.Const{Kind: wir.ConstBool, Bool: false})
	case *ast.NumberExpr:
		return b.pooledConst(wir.Const{Kind: wir.ConstNumber, Number: e.Value})
	case *ast.StringExpr:
		return b.pooledConst(wir.Const{Kind: wir.ConstString, Str: e.Value})
	default:
		return wir.Operand{}
	}
}

func (b *builder) constNil() wir.Operand {
	return b.pooledConst(wir.Const{Kind: wir.ConstNil})
}

func (b *builder) constNumber(raw string) wir.Operand {
	return b.pooledConst(wir.Const{Kind: wir.ConstNumber, Number: raw})
}

func (b *builder) pooledConst(c wir.Const) wir.Operand {
	return wir.Operand{Kind: wir.OperandConst, Ref: uint32(b.body.InternConst(c))}
}

func (b *builder) pathOperand(p path.Path) wir.Operand {
	ref := b.body.InternPath(p)
	if ref == 0 {
		return wir.Operand{}
	}
	return wir.Operand{Kind: wir.OperandPath, Ref: uint32(ref)}
}

func (b *builder) internString(s string) wir.ConstRef {
	return b.body.InternConst(wir.Const{Kind: wir.ConstString, Str: s})
}

// internType resolves an AST type expression to its typ.Type identity through
// the shared lexical resolver and interns it. An unresolved type expression
// yields the none ref; there is no syntactic-spelling fallback.
func (b *builder) internType(t ast.TypeExpr) wir.TypeRef {
	if t == nil {
		return 0
	}
	resolved, ok := b.resolver.Type(t)
	if !ok {
		return 0
	}
	return b.body.InternType(resolved)
}

func (b *builder) internTypes(types []ast.TypeExpr) []wir.TypeRef {
	if len(types) == 0 {
		return nil
	}
	out := make([]wir.TypeRef, 0, len(types))
	for _, expr := range types {
		if ref := b.internType(expr); ref != 0 {
			out = append(out, ref)
		}
	}
	return out
}

// targetOperand returns the destination operand for an assignment target and
// whether it resolved to a path.
func (b *builder) targetOperand(target ast.Expr) (wir.Operand, bool) {
	if p, ok := pathexpr.Resolve(target, b.bindings); ok {
		return b.pathOperand(p), true
	}
	return b.newTemp(), false
}

func (b *builder) localPath(s *ast.LocalAssignStmt, i int) wir.Operand {
	if sym, ok := b.bindings.LocalSymbolAt(s, i); ok {
		return b.pathOperand(path.Path{Root: s.Names[i], Symbol: sym})
	}
	return b.pathOperand(path.Path{Root: s.Names[i]})
}

func (b *builder) numForOperand(s *ast.NumberForStmt) wir.Operand {
	if sym, ok := b.bindings.NumForSymbol(s); ok {
		return b.pathOperand(path.Path{Root: s.Name, Symbol: sym})
	}
	return b.pathOperand(path.Path{Root: s.Name})
}

func (b *builder) genericForOperands(s *ast.GenericForStmt) []wir.Operand {
	syms := b.bindings.GenericForSymbols(s)
	ops := make([]wir.Operand, len(s.Names))
	for i, name := range s.Names {
		p := path.Path{Root: name}
		if i < len(syms) {
			p.Symbol = syms[i]
		}
		ops[i] = b.pathOperand(p)
	}
	return ops
}

// ---- shared free helpers ------------------------------------------------

// numericForBounds returns the numeric-for control expressions in Lua
// evaluation order (init, limit, optional step), matching cfgbuild so bound-call
// points stay positionally aligned with the numeric-for points.
func numericForBounds(stmt *ast.NumberForStmt) []ast.Expr {
	bounds := []ast.Expr{stmt.Init, stmt.Limit}
	if stmt.Step != nil {
		bounds = append(bounds, stmt.Step)
	}
	return bounds
}

// tailCall reports the call expression when e is a direct call (the last
// value-list expression that expands its results across remaining targets).
func tailCall(e ast.Expr) (*ast.FuncCallExpr, bool) {
	inner := sourceprovenance.AssertionInner(e)
	call, ok := inner.(*ast.FuncCallExpr)
	return call, ok
}

// isTopLevelCall reports whether call is the direct call value of expr.
func isTopLevelCall(expr ast.Expr, call *ast.FuncCallExpr) bool {
	top, ok := sourceprovenance.Call(expr)
	return ok && top == call
}

func isValueListRootCall(exprs []ast.Expr, occ callorder.Occurrence) bool {
	return occ.ExprIndex >= 0 && occ.ExprIndex < len(exprs) && isTopLevelCall(exprs[occ.ExprIndex], occ.Call)
}

func valueListCallContext(exprs []ast.Expr, occ callorder.Occurrence, rootContext wir.CallContextKind) wir.CallContextKind {
	if isValueListRootCall(exprs, occ) {
		return rootContext
	}
	return wir.CallContextExpressionProducer
}

func exprCallContext(expr ast.Expr, call *ast.FuncCallExpr, rootContext wir.CallContextKind) wir.CallContextKind {
	if rootContext == wir.CallContextCondition {
		predicate, _, ok := branchcond.PredicateCall(expr)
		if ok && predicate == call {
			return rootContext
		}
		return wir.CallContextExpressionProducer
	}
	if isTopLevelCall(expr, call) {
		return rootContext
	}
	return wir.CallContextExpressionProducer
}

func isVararg(e ast.Expr) bool {
	_, ok := sourceprovenance.AssertionInner(e).(*ast.Comma3Expr)
	return ok
}

func arithOperator(op string) wir.Operator {
	switch op {
	case "+":
		return wir.BinAdd
	case "-":
		return wir.BinSub
	case "*":
		return wir.BinMul
	case "/":
		return wir.BinDiv
	case "//":
		return wir.BinIDiv
	case "%":
		return wir.BinMod
	case "^":
		return wir.BinPow
	case "&":
		return wir.BinBAnd
	case "|":
		return wir.BinBOr
	case "~":
		return wir.BinBXor
	case "<<":
		return wir.BinShl
	case ">>":
		return wir.BinShr
	default:
		return wir.OperatorNone
	}
}

func relOperator(op string) wir.Operator {
	switch op {
	case "==":
		return wir.BinEq
	case "~=":
		return wir.BinNe
	case "<":
		return wir.BinLt
	case "<=":
		return wir.BinLe
	case ">":
		return wir.BinGt
	case ">=":
		return wir.BinGe
	default:
		return wir.OperatorNone
	}
}

// lowerCheck projects a normalized branch condition into the neutral wir.Check
// descriptor the IR stores. branchcond owns the syntax-facing normalization; the
// IR keeps only the resolved path and type identities. The kind enumerations are
// defined in lockstep, so the kind maps by position.
func lowerCheck(c branchcond.Check) wir.Check {
	return wir.Check{
		Kind:          wir.CheckKind(c.Kind),
		Path:          c.Path,
		OtherPath:     c.OtherPath,
		TypeName:      c.TypeName,
		Literal:       c.Literal,
		LiteralString: c.LiteralString,
		LenFloor:      c.LenFloor,
		NumFloor:      c.NumFloor,
		Negated:       c.Negated,
	}
}

func lowerImpliedChecks(in []branchcond.ImpliedCheck) []wir.ImpliedCheck {
	if len(in) == 0 {
		return nil
	}
	out := make([]wir.ImpliedCheck, 0, len(in))
	for _, check := range in {
		out = append(out, wir.ImpliedCheck{
			Check:    lowerCheck(check.Check),
			Edge:     check.Edge,
			Polarity: check.Polarity,
		})
	}
	return out
}

func lowerBranchDiffConstraints(in []branchcond.BranchDiffConstraint) []wir.BranchDiffConstraint {
	if len(in) == 0 {
		return nil
	}
	out := make([]wir.BranchDiffConstraint, 0, len(in))
	for _, d := range in {
		out = append(out, wir.BranchDiffConstraint{
			CoHi:     d.CoHi,
			HiPath:   d.HiPath,
			HiIsLen:  d.HiIsLen,
			CoHi2:    d.CoHi2,
			Hi2Path:  d.Hi2Path,
			Hi2IsLen: d.Hi2IsLen,
			HasHi2:   d.HasHi2,
			LoPath:   d.LoPath,
			LoIsLen:  d.LoIsLen,
			C:        d.C,
			Edge:     d.Edge,
		})
	}
	return out
}
