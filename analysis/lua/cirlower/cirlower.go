// Package cirlower lowers typed-Lua syntax into cir attached to the shared CFG
// that cfgbuild already produced (decision D1a). It creates no CFG topology of
// its own: cfgbuild is the point authority, and cirlower maps each Lua construct
// onto the pre-existing points it discovers through cfgbuild.Result.StmtPoints
// (statement -> points, in creation order) and cfgfacts.Metadata (short-circuit
// guard / expression-evaluation sidecars).
//
// It translates syntax and resolves bindings/types only. It computes no
// refinements, no narrowing, and no type conclusions: every value derivation is
// the transfer interpreter's job. Conditions are lowered through
// branchcond.Normalize (a closed descriptor) and attached to OpBranch; paths
// come from pathexpr/keyspace; nothing here concludes anything about values.
// That boundary is the whole point of cir.
//
// Point granularity follows cfgbuild: one OpCall per call (each on its own
// NodeCall point, in Lua evaluation order, before the owning statement's own
// points); one destination per NodeAssign point (multret result binding splits
// across the per-target assign points); loop headers split (OpIterate on the
// branch point, the loop-variable binding on the assign point(s)); joins carry
// no instruction. Short-circuit and/or follows the always-materialized
// short-circuit topology cfgbuild emits (decision D3, purity split).
package cirlower

import (
	"strconv"

	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/cir"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
	"github.com/wippyai/go-lua/analysis/lua/callorder"
	"github.com/wippyai/go-lua/analysis/lua/cfgbuild"
	"github.com/wippyai/go-lua/analysis/lua/cfgfacts"
	"github.com/wippyai/go-lua/analysis/lua/channelruntime"
	"github.com/wippyai/go-lua/analysis/lua/pathexpr"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	"github.com/wippyai/go-lua/analysis/lua/typeresolve"
	"github.com/wippyai/go-lua/compiler/ast"
)

// Lower lowers a bound statement chunk onto the CFG cfgbuild already built for
// the same AST. bindings must be the result of binding stmts (e.g.
// bind.BindChunk) and built must be cfgbuild.BuildChunk over the same stmts.
// Points in the returned Body index into built.Graph.
func Lower(name string, stmts []ast.Stmt, bindings *bind.Result, built *cfgbuild.Result) *cir.Body {
	return lowerInto(name, stmts, bindings, built, typeresolve.New(bindings))
}

// lowerInto lowers one function-scope statement list (a chunk or a nested
// function body) onto its shared graph. resolver is the shared lexical type
// resolver, threaded through nested protos so type identities and their caches
// stay consistent across the whole chunk.
func lowerInto(name string, stmts []ast.Stmt, bindings *bind.Result, built *cfgbuild.Result, resolver *typeresolve.Resolver) *cir.Body {
	b := &builder{
		body:        cir.NewBody(name),
		graph:       built.Graph,
		meta:        built.Meta,
		points:      built.StmtPoints,
		bindings:    bindings,
		resolver:    resolver,
		pointInstrs: make(map[cfg.Point][]cir.Instruction),
		callTemps:   make(map[*ast.FuncCallExpr]*callResult),
		guardByCond: make(map[ast.Expr]cfg.Point),
		evalByExpr:  make(map[ast.Expr]cfg.Point),
	}
	b.indexShortCircuits()

	b.curPoint = b.graph.Entry()
	b.emit(cir.Instruction{Op: cir.OpEntry})
	b.lowerStmts(stmts)
	b.curPoint = b.graph.Exit()
	b.emit(cir.Instruction{Op: cir.OpExit})

	b.flush()
	return b.body
}

type builder struct {
	body     *cir.Body
	graph    cfg.Graph
	meta     cfgfacts.Metadata
	points   cfgbuild.StmtPoints
	bindings *bind.Result
	resolver *typeresolve.Resolver

	curPoint    cfg.Point
	pointInstrs map[cfg.Point][]cir.Instruction
	tempSeq     uint32
	protoSeq    int

	// callTemps maps every call pre-lowered at its own NodeCall point to the
	// result temp(s) it binds. Expression lowering substitutes a call's head
	// temp in place of re-emitting the call; multret tail patching flips the
	// producing OpCall's ResultSpread through the recorded (point, index).
	callTemps map[*ast.FuncCallExpr]*callResult

	// guardByCond and evalByExpr index the short-circuit sidecar points cfgbuild
	// records outside StmtPoints, keyed by the AST identity cirlower matches on
	// (the guard condition = LogicalOpExpr.Lhs, the eval expr = the RHS).
	guardByCond map[ast.Expr]cfg.Point
	evalByExpr  map[ast.Expr]cfg.Point
}

// callResult records the result temps a pre-lowered call binds together with the
// slot to patch when the call is later found in a spread (multret) tail.
type callResult struct {
	head  cir.Operand
	temps []cir.Operand
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
func (b *builder) emit(inst cir.Instruction) {
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

func (b *builder) newTemp() cir.Operand {
	op := cir.Operand{Kind: cir.OperandTemp, Ref: b.tempSeq}
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
		b.bindInto(dst, values[i])
		if i < len(s.Types) && s.Types[i] != nil {
			b.emit(cir.Instruction{
				Op:    cir.OpClaim,
				Dst:   dst,
				A:     dst,
				Claim: cir.ClaimAnnotation,
				Type:  b.internType(s.Types[i]),
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
		b.lowerAssignTarget(target, values[i])
	}
}

// lowerAssignTarget writes a pre-planned value into one assignment target,
// choosing the write op by target shape.
func (b *builder) lowerAssignTarget(target ast.Expr, v binding) {
	switch t := target.(type) {
	case *ast.IdentExpr:
		dst, _ := b.targetOperand(t)
		b.bindInto(dst, v)
	case *ast.AttrGetExpr:
		if p, ok := pathexpr.Resolve(t, b.bindings); ok {
			b.emit(cir.Instruction{
				Op:  cir.OpStaticMemberWrite,
				Dst: b.pathOperand(p),
				A:   b.bindingOperand(v),
			})
			return
		}
		container, _ := pathexpr.ResolveMutationContainer(t, b.bindings)
		b.emit(cir.Instruction{
			Op:  cir.OpDynamicIndexWrite,
			Dst: b.pathOperand(container),
			A:   b.lowerExpr(t.Key),
			B:   b.bindingOperand(v),
		})
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
	op   cir.Operand
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
			out[i] = binding{kind: bindExpr, expr: exprs[i]}
			continue
		}
		// i == last (i > last is impossible: loop bound n, and when last >= n the
		// loop exits before reaching last). Bind the final expression.
		e := exprs[last]
		if call, ok := tailCall(e); ok {
			if cr, ok := b.callTemps[call]; ok {
				for j := last; j < n; j++ {
					out[j] = tempBinding(cr, j-last)
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
func tempBinding(cr *callResult, k int) binding {
	if k < len(cr.temps) {
		return binding{kind: bindOperand, op: cr.temps[k]}
	}
	return binding{kind: bindOperand, op: cr.head}
}

// bindInto writes a planned value into a destination that can receive a produced
// instruction directly (a path or temp), keeping the compact `dst = op a b` form
// for compound expressions.
func (b *builder) bindInto(dst cir.Operand, v binding) {
	switch v.kind {
	case bindExpr:
		b.lowerExprInto(dst, v.expr)
	case bindOperand:
		b.emitAssign(dst, v.op)
	case bindVararg:
		b.emitAssign(dst, cir.Operand{Kind: cir.OperandVararg})
	default:
		b.emitAssign(dst, b.constNil())
	}
}

// bindingOperand reduces a planned value to a single operand (for member/index
// writes, which take the value as an operand rather than a destination).
func (b *builder) bindingOperand(v binding) cir.Operand {
	switch v.kind {
	case bindExpr:
		return b.lowerExpr(v.expr)
	case bindOperand:
		return v.op
	case bindVararg:
		return cir.Operand{Kind: cir.OperandVararg}
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
		if occ.ExprIndex == last && last < targetCount && isTopLevelCall(exprs[last], occ.Call) {
			results = targetCount - last
		}
		b.emitCallAt(callPoints[i], occ.Call, results)
	}
}

// preLowerExprCalls emits an OpCall for every call under a single expression
// (condition, return value, iterator source) onto its NodeCall point, each
// binding one head temp. Tail spread is applied later by the value-list lowering.
func (b *builder) preLowerExprCalls(expr ast.Expr, callPoints []cfg.Point) {
	calls, ok := callorder.Expr(expr, b.callOrderOptions())
	if !ok || len(calls) != len(callPoints) {
		return
	}
	for i, occ := range calls {
		b.emitCallAt(callPoints[i], occ.Call, 1)
	}
}

// preLowerListCalls emits an OpCall for every call under a value list, each
// binding one head temp (returns / iterator sources / statement expressions).
func (b *builder) preLowerListCalls(exprs []ast.Expr, callPoints []cfg.Point) {
	calls, ok := callorder.ValueList(exprs, b.callOrderOptions())
	if !ok || len(calls) != len(callPoints) {
		return
	}
	for i, occ := range calls {
		b.emitCallAt(callPoints[i], occ.Call, 1)
	}
}

// emitCallAt lowers one call onto its dedicated point, binding results result
// temps. resultCount == 0 is a statement call whose results are discarded.
func (b *builder) emitCallAt(point cfg.Point, call *ast.FuncCallExpr, resultCount int) {
	prev := b.curPoint
	b.curPoint = point
	defer func() { b.curPoint = prev }()

	temps := make([]cir.Operand, resultCount)
	for i := range temps {
		temps[i] = b.newTemp()
	}
	head := cir.Operand{}
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
	inst := cir.Instruction{
		Op:         cir.OpCall,
		List:       b.body.AppendOperands(args),
		Results:    b.body.AppendOperands(temps),
		ListSpread: argSpread,
	}
	if call.Method != "" {
		inst.Call.Method = b.internString(call.Method)
		if call.Receiver != nil {
			inst.Call.Receiver = b.lowerExpr(call.Receiver)
		}
	} else {
		inst.Call.Callee = b.calleeOperand(call.Func)
	}
	cr.index = len(b.pointInstrs[point])
	b.emit(inst)
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
		if occ.Call == top || occ.Call == call {
			results = 0
		}
		b.emitCallAt(pts[i], occ.Call, results)
	}
}

// ---- return -------------------------------------------------------------

func (b *builder) lowerReturn(s *ast.ReturnStmt) {
	pts := b.stmtPoints(s)
	nCalls := b.countCalls(s.Exprs)
	if len(pts) != nCalls+1 {
		return
	}
	b.preLowerListCalls(s.Exprs, pts[:nCalls])
	b.curPoint = pts[nCalls]
	ops, spread := b.lowerValueList(s.Exprs)
	b.emit(cir.Instruction{
		Op:         cir.OpReturn,
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
	b.preLowerExprCalls(s.Condition, pts[:nCalls])
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
	b.preLowerExprCalls(s.Condition, pts[:nCalls])
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
	b.preLowerExprCalls(s.Condition, pts[:nCalls])
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
	inst := cir.Instruction{Op: cir.OpBranch, Check: b.body.InternCheck(lowerCheck(check))}
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
	b.preLowerListCalls(bounds, pts[:nCalls])
	// preheader (NodeAssign): the loop-variable binding (value derived by transfer
	// from the iterator header on the branch point).
	b.curPoint = pts[nCalls]
	b.emitAssign(b.numForOperand(s), cir.Operand{})
	// branch (NodeBranch): the numeric iterator header carrying the bounds.
	b.curPoint = pts[nCalls+1]
	list := []cir.Operand{b.lowerExpr(s.Init), b.lowerExpr(s.Limit)}
	if s.Step != nil {
		list = append(list, b.lowerExpr(s.Step))
	} else {
		list = append(list, b.constNumber("1"))
	}
	b.emit(cir.Instruction{
		Op:   cir.OpIterate,
		Iter: cir.IterNumeric,
		List: b.body.AppendOperands(list),
	})
	b.lowerStmts(s.Stmts)
}

func (b *builder) lowerGenericFor(s *ast.GenericForStmt) {
	pts := b.stmtPoints(s)
	nCalls := b.countCalls(s.Exprs)
	if len(pts) != nCalls+1+len(s.Names) {
		return
	}
	b.preLowerListCalls(s.Exprs, pts[:nCalls])
	// branch (NodeBranch): the generic iterator header carrying the sources.
	b.curPoint = pts[nCalls]
	srcOps, spread := b.lowerValueList(s.Exprs)
	b.emit(cir.Instruction{
		Op:         cir.OpIterate,
		Iter:       cir.IterGeneric,
		List:       b.body.AppendOperands(srcOps),
		ListSpread: spread,
	})
	// one NodeAssign per loop variable: the variable binding.
	varOps := b.genericForOperands(s)
	for i := range s.Names {
		b.curPoint = pts[nCalls+1+i]
		b.emitAssign(varOps[i], cir.Operand{})
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
		b.emitClosure(b.pathOperand(p), s.Func)
		return
	}
	tmp := b.newTemp()
	b.emitClosure(tmp, s.Func)
	if ok {
		b.emit(cir.Instruction{Op: cir.OpStaticMemberWrite, Dst: b.pathOperand(p), A: tmp})
	}
}

// emitClosure lowers fn into its own proto Body (built on its own cfgbuild
// graph) and emits an OpClosure into dst carrying the capture operands in bind
// order.
func (b *builder) emitClosure(dst cir.Operand, fn *ast.FunctionExpr) {
	name := b.body.Name + ".fn" + strconv.Itoa(b.protoSeq)
	b.protoSeq++
	childBuilt := cfgbuild.BuildFunction(fn, b.bindings)
	var ref cir.FuncRef
	if childBuilt != nil && childBuilt.Graph != nil {
		childBody := lowerInto(name, fn.Stmts, b.bindings, childBuilt, b.resolver)
		ref = b.body.AddProto(cir.FuncProto{Name: name, Body: childBody, Graph: childBuilt.Graph})
	}

	caps := b.bindings.DirectCaptures(fn)
	ops := make([]cir.Operand, 0, len(caps))
	for _, c := range caps {
		ops = append(ops, b.pathOperand(path.NewPath(c.Captured, c.CapturedName)))
	}
	b.emit(cir.Instruction{
		Op:   cir.OpClosure,
		Dst:  dst,
		Func: ref,
		List: b.body.AppendOperands(ops),
	})
}

// ---- channel select recognition ----------------------------------------

// maybeLowerSelect recognizes the ambient channel.select runtime call and emits
// an OpSelect over its recognized receive-case channels. It returns false when
// call is not a select so the caller lowers it as an ordinary call.
func (b *builder) maybeLowerSelect(dst cir.Operand, call *ast.FuncCallExpr) bool {
	if !channelruntime.IsSelectCall(call, b.bindings) {
		return false
	}
	table, ok := call.Args[0].(*ast.TableExpr)
	if !ok {
		return false
	}
	ops := make([]cir.Operand, 0, len(table.Fields))
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
		if !ok || !channelruntime.IsReceiveCaseCall(cc, b.bindings) {
			continue
		}
		if p, ok := pathexpr.Resolve(cc.Receiver, b.bindings); ok && !p.IsEmpty() {
			ops = append(ops, b.pathOperand(p))
		}
	}
	b.emit(cir.Instruction{
		Op:            cir.OpSelect,
		Dst:           dst,
		List:          b.body.AppendOperands(ops),
		SelectDefault: hasDefault,
	})
	return true
}

// ---- expression lowering ------------------------------------------------

// lowerExprInto lowers e so its value lands in dst, choosing the producing
// instruction by syntax. Calls are already pre-lowered at their own points, so
// a call expression here copies its head result temp.
func (b *builder) lowerExprInto(dst cir.Operand, e ast.Expr) {
	switch e := e.(type) {
	case *ast.NilExpr, *ast.TrueExpr, *ast.FalseExpr, *ast.NumberExpr, *ast.StringExpr:
		b.emitAssign(dst, b.constOperand(e))
	case *ast.IdentExpr:
		b.emitAssign(dst, b.readOperand(e))
	case *ast.AttrGetExpr:
		b.emitAssign(dst, b.readOperand(e))
	case *ast.ArithmeticOpExpr:
		b.emitBinOp(dst, arithOperator(e.Operator), e.Lhs, e.Rhs)
	case *ast.RelationalOpExpr:
		b.emitBinOp(dst, relOperator(e.Operator), e.Lhs, e.Rhs)
	case *ast.StringConcatOpExpr:
		ops := b.flattenConcat(e)
		b.emit(cir.Instruction{Op: cir.OpConcat, Dst: dst, List: b.body.AppendOperands(ops)})
	case *ast.UnaryMinusOpExpr:
		b.emitUnOp(dst, cir.UnNeg, e.Expr)
	case *ast.UnaryNotOpExpr:
		b.emitUnOp(dst, cir.UnNot, e.Expr)
	case *ast.UnaryLenOpExpr:
		b.emitUnOp(dst, cir.UnLen, e.Expr)
	case *ast.UnaryBNotOpExpr:
		b.emitUnOp(dst, cir.UnBNot, e.Expr)
	case *ast.FuncCallExpr:
		b.emitAssign(dst, b.callValue(e))
	case *ast.CastExpr:
		b.emit(cir.Instruction{Op: cir.OpClaim, Dst: dst, A: b.lowerExpr(e.Expr), Claim: cir.ClaimCast, Type: b.internType(e.Type)})
	case *ast.NonNilAssertExpr:
		b.emit(cir.Instruction{Op: cir.OpClaim, Dst: dst, A: b.lowerExpr(e.Expr), Claim: cir.ClaimAssert})
	case *ast.LogicalOpExpr:
		b.lowerLogicalInto(dst, e)
	case *ast.FunctionExpr:
		b.emitClosure(dst, e)
	case *ast.TableExpr:
		b.lowerTable(dst, e)
	case *ast.Comma3Expr:
		b.emitAssign(dst, cir.Operand{Kind: cir.OperandVararg})
	default:
		b.emitAssign(dst, cir.Operand{})
	}
}

// lowerExpr lowers e to an operand, allocating a temp for compound expressions.
func (b *builder) lowerExpr(e ast.Expr) cir.Operand {
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
			t := b.newTemp()
			b.emit(cir.Instruction{Op: cir.OpLogical, Dst: t, A: b.lowerExpr(e.Lhs), B: b.lowerExpr(e.Rhs), Operator: logicalOperator(e)})
			return t
		}
		return b.lowerLogicalValue(e)
	case *ast.Comma3Expr:
		return cir.Operand{Kind: cir.OperandVararg}
	default:
		t := b.newTemp()
		b.lowerExprInto(t, e)
		return t
	}
}

// callValue returns the head result temp of a call already lowered at its point.
func (b *builder) callValue(call *ast.FuncCallExpr) cir.Operand {
	if cr, ok := b.callTemps[call]; ok {
		return cr.head
	}
	return cir.Operand{}
}

// lowerMultiValue lowers a tail-position expression that expands to all its
// values, marking the producing call as multret. It returns the head operand.
func (b *builder) lowerMultiValue(e ast.Expr) cir.Operand {
	switch e := e.(type) {
	case *ast.FuncCallExpr:
		if cr, ok := b.callTemps[e]; ok {
			b.markSpread(cr)
			return cr.head
		}
		return cir.Operand{}
	case *ast.Comma3Expr:
		return cir.Operand{Kind: cir.OperandVararg}
	default:
		return b.lowerExpr(e)
	}
}

// markSpread flips a pre-lowered call's ResultSpread once it is discovered in a
// spread (open multret) tail position.
func (b *builder) markSpread(cr *callResult) {
	insts := b.pointInstrs[cr.point]
	if cr.index >= 0 && cr.index < len(insts) && insts[cr.index].Op == cir.OpCall {
		insts[cr.index].ResultSpread = true
	}
}

// lowerValueList lowers an expression list, expanding a final multi-value
// producer. It returns the operands and whether the tail is an open spread.
func (b *builder) lowerValueList(exprs []ast.Expr) ([]cir.Operand, bool) {
	if len(exprs) == 0 {
		return nil, false
	}
	ops := make([]cir.Operand, 0, len(exprs))
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
func (b *builder) lowerLogicalInto(dst cir.Operand, e *ast.LogicalOpExpr) {
	if b.logicalRHSPure(e.Rhs) {
		b.emit(cir.Instruction{Op: cir.OpLogical, Dst: dst, A: b.lowerExpr(e.Lhs), B: b.lowerExpr(e.Rhs), Operator: logicalOperator(e)})
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
func (b *builder) lowerLogicalValue(e *ast.LogicalOpExpr) cir.Operand {
	guard, hasGuard := b.guardByCond[e.Lhs]
	anchor, hasAnchor := b.rhsAnchorPoint(e.Rhs)
	if !hasGuard || !hasAnchor {
		// The topology cfgbuild expected is absent (e.g. a shape callorder
		// rejected): fall back to the value form so the result is still bound.
		t := b.newTemp()
		b.emit(cir.Instruction{Op: cir.OpLogical, Dst: t, A: b.lowerExpr(e.Lhs), B: b.lowerExpr(e.Rhs), Operator: logicalOperator(e)})
		return t
	}
	result := b.newTemp()
	prev := b.curPoint

	b.curPoint = guard
	b.emitAssign(result, b.lowerExpr(e.Lhs))
	check := branchcond.Normalize(e.Lhs, b.bindings)
	guardInst := cir.Instruction{Op: cir.OpBranch, Check: b.body.InternCheck(lowerCheck(check))}
	if check.Kind == branchcond.CheckNone {
		guardInst.A = b.lowerExpr(e.Lhs)
	}
	b.emit(guardInst)

	b.curPoint = anchor
	b.lowerExprInto(result, e.Rhs)

	b.curPoint = prev
	return result
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

func logicalOperator(e *ast.LogicalOpExpr) cir.Operator {
	if e.Operator == "or" {
		return cir.LogOr
	}
	return cir.LogAnd
}

// ---- table constructors -------------------------------------------------

// lowerTable lowers a table constructor over its array, hash, and trailing
// spread parts. Every field value becomes a List operand; a final keyless
// multi-value producer marks the list tail as an open spread. Field keys are
// structural syntax recovered by transfer from the constructor.
func (b *builder) lowerTable(dst cir.Operand, t *ast.TableExpr) {
	last := lastFieldIndex(t.Fields)
	ops := make([]cir.Operand, 0, len(t.Fields))
	spread := false
	for i, f := range t.Fields {
		if f == nil || f.Value == nil {
			continue
		}
		if i == last && f.Key == nil && ast.CanProduceMultipleValues(f.Value) {
			ops = append(ops, b.lowerMultiValue(f.Value))
			spread = true
			continue
		}
		ops = append(ops, b.lowerExpr(f.Value))
	}
	b.emit(cir.Instruction{Op: cir.OpMakeTable, Dst: dst, List: b.body.AppendOperands(ops), ListSpread: spread})
}

func lastFieldIndex(fields []*ast.Field) int {
	for i := len(fields) - 1; i >= 0; i-- {
		if fields[i] != nil && fields[i].Value != nil {
			return i
		}
	}
	return -1
}

func (b *builder) flattenConcat(e *ast.StringConcatOpExpr) []cir.Operand {
	var ops []cir.Operand
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

func (b *builder) emitAssign(dst, src cir.Operand) {
	b.emit(cir.Instruction{Op: cir.OpAssign, Dst: dst, A: src})
}

func (b *builder) emitBinOp(dst cir.Operand, op cir.Operator, lhs, rhs ast.Expr) {
	a := b.lowerExpr(lhs)
	c := b.lowerExpr(rhs)
	b.emit(cir.Instruction{Op: cir.OpBinOp, Dst: dst, A: a, B: c, Operator: op})
}

func (b *builder) emitUnOp(dst cir.Operand, op cir.Operator, operand ast.Expr) {
	a := b.lowerExpr(operand)
	b.emit(cir.Instruction{Op: cir.OpUnOp, Dst: dst, A: a, Operator: op})
}

// readOperand returns the operand for a value read: a path operand when the
// expression resolves to a static path, else a temp holding an opaque read.
func (b *builder) readOperand(e ast.Expr) cir.Operand {
	if p, ok := pathexpr.Resolve(e, b.bindings); ok {
		return b.pathOperand(p)
	}
	t := b.newTemp()
	b.emit(cir.Instruction{Op: cir.OpAssign, Dst: t, A: cir.Operand{}})
	return t
}

func (b *builder) calleeOperand(e ast.Expr) cir.Operand {
	if p, ok := pathexpr.Resolve(e, b.bindings); ok {
		return b.pathOperand(p)
	}
	if id, ok := e.(*ast.IdentExpr); ok {
		return b.pathOperand(path.Path{Root: id.Value})
	}
	return b.lowerExpr(e)
}

func (b *builder) constOperand(e ast.Expr) cir.Operand {
	switch e := e.(type) {
	case *ast.NilExpr:
		return b.pooledConst(cir.Const{Kind: cir.ConstNil})
	case *ast.TrueExpr:
		return b.pooledConst(cir.Const{Kind: cir.ConstBool, Bool: true})
	case *ast.FalseExpr:
		return b.pooledConst(cir.Const{Kind: cir.ConstBool, Bool: false})
	case *ast.NumberExpr:
		return b.pooledConst(cir.Const{Kind: cir.ConstNumber, Number: e.Value})
	case *ast.StringExpr:
		return b.pooledConst(cir.Const{Kind: cir.ConstString, Str: e.Value})
	default:
		return cir.Operand{}
	}
}

func (b *builder) constNil() cir.Operand {
	return b.pooledConst(cir.Const{Kind: cir.ConstNil})
}

func (b *builder) constNumber(raw string) cir.Operand {
	return b.pooledConst(cir.Const{Kind: cir.ConstNumber, Number: raw})
}

func (b *builder) pooledConst(c cir.Const) cir.Operand {
	return cir.Operand{Kind: cir.OperandConst, Ref: uint32(b.body.InternConst(c))}
}

func (b *builder) pathOperand(p path.Path) cir.Operand {
	ref := b.body.InternPath(p)
	if ref == 0 {
		return cir.Operand{}
	}
	return cir.Operand{Kind: cir.OperandPath, Ref: uint32(ref)}
}

func (b *builder) internString(s string) cir.ConstRef {
	return b.body.InternConst(cir.Const{Kind: cir.ConstString, Str: s})
}

// internType resolves an AST type expression to its typ.Type identity through
// the shared lexical resolver and interns it. An unresolved type expression
// yields the none ref; there is no syntactic-spelling fallback.
func (b *builder) internType(t ast.TypeExpr) cir.TypeRef {
	if t == nil {
		return 0
	}
	resolved, ok := b.resolver.Type(t)
	if !ok {
		return 0
	}
	return b.body.InternType(resolved)
}

// targetOperand returns the destination operand for an assignment target and
// whether it resolved to a path.
func (b *builder) targetOperand(target ast.Expr) (cir.Operand, bool) {
	if p, ok := pathexpr.Resolve(target, b.bindings); ok {
		return b.pathOperand(p), true
	}
	return b.newTemp(), false
}

func (b *builder) localPath(s *ast.LocalAssignStmt, i int) cir.Operand {
	if sym, ok := b.bindings.LocalSymbolAt(s, i); ok {
		return b.pathOperand(path.Path{Root: s.Names[i], Symbol: sym})
	}
	return b.pathOperand(path.Path{Root: s.Names[i]})
}

func (b *builder) numForOperand(s *ast.NumberForStmt) cir.Operand {
	if sym, ok := b.bindings.NumForSymbol(s); ok {
		return b.pathOperand(path.Path{Root: s.Name, Symbol: sym})
	}
	return b.pathOperand(path.Path{Root: s.Name})
}

func (b *builder) genericForOperands(s *ast.GenericForStmt) []cir.Operand {
	syms := b.bindings.GenericForSymbols(s)
	ops := make([]cir.Operand, len(s.Names))
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

func isVararg(e ast.Expr) bool {
	_, ok := sourceprovenance.AssertionInner(e).(*ast.Comma3Expr)
	return ok
}

func arithOperator(op string) cir.Operator {
	switch op {
	case "+":
		return cir.BinAdd
	case "-":
		return cir.BinSub
	case "*":
		return cir.BinMul
	case "/":
		return cir.BinDiv
	case "//":
		return cir.BinIDiv
	case "%":
		return cir.BinMod
	case "^":
		return cir.BinPow
	case "&":
		return cir.BinBAnd
	case "|":
		return cir.BinBOr
	case "~":
		return cir.BinBXor
	case "<<":
		return cir.BinShl
	case ">>":
		return cir.BinShr
	default:
		return cir.OperatorNone
	}
}

func relOperator(op string) cir.Operator {
	switch op {
	case "==":
		return cir.BinEq
	case "~=":
		return cir.BinNe
	case "<":
		return cir.BinLt
	case "<=":
		return cir.BinLe
	case ">":
		return cir.BinGt
	case ">=":
		return cir.BinGe
	default:
		return cir.OperatorNone
	}
}

// lowerCheck projects a normalized branch condition into the neutral cir.Check
// descriptor the IR stores. branchcond owns the syntax-facing normalization; the
// IR keeps only the resolved path and type identities. The kind enumerations are
// defined in lockstep, so the kind maps by position.
func lowerCheck(c branchcond.Check) cir.Check {
	return cir.Check{
		Kind:          cir.CheckKind(c.Kind),
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
