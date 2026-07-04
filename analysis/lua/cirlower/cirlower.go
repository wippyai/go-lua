// Package lower is a prototype AST->cir lowering for a subset of typed Lua.
//
// It translates syntax and resolves bindings/types only. It computes no
// refinements, no narrowing, and no type conclusions: every value derivation is
// the transfer interpreter's job. Conditions are lowered through
// branchcond.Normalize (a closed descriptor) and attached to OpBranch; paths
// come from pathexpr/keyspace; nothing here concludes anything about values.
// That boundary is the whole point of cir.
//
// Scope covered by this prototype: local and ordinary assignment (incl. static
// member and dynamic index writes), if/elseif/else, numeric and generic for,
// while, return, direct and method calls, table literals, binary/unary/concat
// operators, casts, non-nil assertions, and type annotations as claims. Out of
// scope constructs lower to a Noop point or an opaque assignment.
package cirlower

import (
	"strconv"

	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/cir"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
	"github.com/wippyai/go-lua/analysis/lua/channelruntime"
	"github.com/wippyai/go-lua/analysis/lua/pathexpr"
	"github.com/wippyai/go-lua/compiler/ast"
)

// Result pairs the lowered Body with the CFG that owns its topology. Points in
// the Body are indices into the same CFG.
type Result struct {
	Body  *cir.Body
	Graph *cfg.CFG
}

// Chunk lowers a bound statement chunk into cir. bindings must be the result of
// binding stmts (e.g. bind.BindChunk).
func Chunk(name string, stmts []ast.Stmt, bindings *bind.Result) *Result {
	return lowerBody(name, stmts, bindings, nil)
}

// lowerBody lowers one function-scope statement list (a chunk when fn is nil, a
// nested function body otherwise) into its own Body and CFG. Break/goto/label
// scopes are function-local and reset per call.
func lowerBody(name string, stmts []ast.Stmt, bindings *bind.Result, fn *ast.FunctionExpr) *Result {
	g := cfg.New()
	b := &builder{
		body:         cir.NewBody(name),
		graph:        g,
		bindings:     bindings,
		curFn:        fn,
		labels:       make(map[string]cfg.Point),
		pendingGotos: make(map[string][]cfg.Point),
	}

	entry := g.Entry()
	b.begin(entry)
	b.body.Emit(cir.Instruction{Op: cir.OpEntry, Point: entry})
	b.finish()
	b.pending = []pend{{p: entry, cond: false}}

	b.lowerStmts(stmts)

	exit := g.Exit()
	b.linkTo(exit)
	b.begin(exit)
	b.body.Emit(cir.Instruction{Op: cir.OpExit, Point: exit})
	b.finish()

	return &Result{Body: b.body, Graph: g}
}

// pend is an open control-flow end awaiting an edge to the next point.
type pend struct {
	p    cfg.Point
	cond bool
}

type builder struct {
	body     *cir.Body
	graph    *cfg.CFG
	bindings *bind.Result
	curFn    *ast.FunctionExpr

	pending  []pend
	curPoint cfg.Point
	curStart int
	tempSeq  uint32
	protoSeq int

	// breakStack holds, per enclosing loop, the open ends produced by `break`
	// statements. They merge into the loop's exit pending set when the loop closes.
	breakStack [][]pend
	// labels maps a resolved label name to its CFG point; pendingGotos holds goto
	// source points awaiting a forward label definition.
	labels       map[string]cfg.Point
	pendingGotos map[string][]cfg.Point
}

// begin starts an instruction window at point p.
func (b *builder) begin(p cfg.Point) {
	b.curPoint = p
	b.curStart = b.body.Len()
}

// finish records the window opened by begin.
func (b *builder) finish() {
	b.body.SetPointRange(b.curPoint, b.curStart, b.body.Len())
}

// linkTo connects every pending open end to p, then clears the pending set.
func (b *builder) linkTo(p cfg.Point) {
	for _, e := range b.pending {
		b.graph.AddEdge(e.p, p, e.cond)
	}
	b.pending = b.pending[:0]
}

// open allocates a new node of kind, links pending ends to it, and starts its
// window. The caller emits instructions then calls finish.
func (b *builder) open(kind cfg.NodeKind) cfg.Point {
	p := b.graph.AddNode(kind)
	b.linkTo(p)
	b.begin(p)
	return p
}

func (b *builder) newTemp() cir.Operand {
	op := cir.Operand{Kind: cir.OperandTemp, Ref: b.tempSeq}
	b.tempSeq++
	return op
}

// lowerStmts lowers a statement list in order.
func (b *builder) lowerStmts(stmts []ast.Stmt) {
	for _, stmt := range stmts {
		b.lowerStmt(stmt)
	}
}

func (b *builder) lowerStmt(stmt ast.Stmt) {
	switch s := stmt.(type) {
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
	case *ast.BreakStmt:
		b.lowerBreak()
	case *ast.LabelStmt:
		b.lowerLabel(s)
	case *ast.GotoStmt:
		b.lowerGoto(s)
	case *ast.DoBlockStmt:
		b.lowerStmts(s.Stmts)
	case *ast.TypeDefStmt, *ast.InterfaceDefStmt:
		// Type declarations resolve at bind time and cost zero instructions.
	default:
		// Unhandled statement occupies a structural point so topology stays intact.
		b.open(cfg.NodeNoop)
		b.body.Emit(cir.Instruction{Op: cir.OpNoop, Point: b.curPoint})
		b.finish()
		b.pending = append(b.pending, pend{p: b.curPoint, cond: false})
	}
}

// seq lowers a straight-line statement: opens a point of kind, runs emit, closes
// it, and leaves a single fall-through pending end.
func (b *builder) seq(kind cfg.NodeKind, emit func()) {
	b.open(kind)
	emit()
	b.finish()
	b.pending = append(b.pending, pend{p: b.curPoint, cond: false})
}

func (b *builder) lowerLocalAssign(s *ast.LocalAssignStmt) {
	b.seq(cfg.NodeAssign, func() {
		dsts := make([]cir.Operand, len(s.Names))
		for i := range s.Names {
			dsts[i] = b.localPath(s, i)
		}
		b.lowerBindingList(dsts, s.Exprs)
		b.emitAnnotations(s, dsts)
	})
}

// lowerBindingList binds a value list into a fixed list of simple destinations,
// preserving Lua's tail expansion: the final expression, when a call or vararg,
// fills every remaining destination (adjusted to that exact count); a shorter
// list nil-fills the rest. Excess expressions are still lowered for their value.
func (b *builder) lowerBindingList(dsts []cir.Operand, exprs []ast.Expr) {
	n := len(dsts)
	if len(exprs) == 0 {
		for _, d := range dsts {
			b.emitAssign(d, b.constNil())
		}
		return
	}
	last := len(exprs) - 1
	for i, e := range exprs {
		if i > last {
			break
		}
		if i >= n {
			// No destination: lower for its value/side effect and drop the result.
			if i == last && ast.CanProduceMultipleValues(e) {
				b.lowerMultiValue(e)
			} else {
				b.lowerExpr(e)
			}
			continue
		}
		if i == last {
			b.lowerBindingTail(dsts[i:], e)
			return
		}
		b.lowerExprInto(dsts[i], e)
	}
}

// lowerBindingTail binds the final value expression into the remaining
// destinations. A call adjusts its result count to len(dsts); a vararg expands
// across them; any other value binds the head and nil-fills the rest.
func (b *builder) lowerBindingTail(dsts []cir.Operand, e ast.Expr) {
	switch e := e.(type) {
	case *ast.FuncCallExpr:
		b.lowerCall(e, dsts, false)
	case *ast.Comma3Expr:
		for _, d := range dsts {
			b.emitAssign(d, cir.Operand{Kind: cir.OperandVararg})
		}
	default:
		b.lowerExprInto(dsts[0], e)
		for _, d := range dsts[1:] {
			b.emitAssign(d, b.constNil())
		}
	}
}

// emitAnnotations records declared local type annotations as annotation claims.
func (b *builder) emitAnnotations(s *ast.LocalAssignStmt, dsts []cir.Operand) {
	for i := range s.Names {
		if i >= len(s.Types) || s.Types[i] == nil {
			continue
		}
		b.body.Emit(cir.Instruction{
			Op:    cir.OpClaim,
			Point: b.curPoint,
			Dst:   dsts[i],
			A:     dsts[i],
			Claim: cir.ClaimAnnotation,
			Type:  b.internType(s.Types[i]),
		})
	}
}

func (b *builder) lowerAssign(s *ast.AssignStmt) {
	b.seq(cfg.NodeAssign, func() {
		n := len(s.Lhs)
		last := len(s.Rhs) - 1
		for i, rhs := range s.Rhs {
			if i >= n {
				// Extra value with no target: lower for its side effect and drop it.
				if i == last && ast.CanProduceMultipleValues(rhs) {
					b.lowerMultiValue(rhs)
				} else {
					b.lowerExpr(rhs)
				}
				continue
			}
			if i == last {
				b.lowerAssignTail(s.Lhs[i:], rhs)
				return
			}
			b.lowerAssignTarget(s.Lhs[i], rhs)
		}
	})
}

// lowerAssignTail binds the final value into the remaining targets. A call over
// two or more simple (identifier or static-member) targets adjusts its result
// count to fill them; otherwise the head target takes the value.
func (b *builder) lowerAssignTail(targets []ast.Expr, value ast.Expr) {
	if call, ok := value.(*ast.FuncCallExpr); ok && len(targets) > 1 {
		if dsts, simple := b.resultDsts(targets); simple {
			b.lowerCall(call, dsts, false)
			return
		}
	}
	b.lowerAssignTarget(targets[0], value)
}

// resultDsts maps assignment targets to call-result destination operands, and
// reports whether every target is a simple path (identifier or static member)
// that can receive a call result directly.
func (b *builder) resultDsts(targets []ast.Expr) ([]cir.Operand, bool) {
	dsts := make([]cir.Operand, len(targets))
	for i, tgt := range targets {
		switch tgt.(type) {
		case *ast.IdentExpr, *ast.AttrGetExpr:
			p, ok := pathexpr.Resolve(tgt, b.bindings)
			if !ok {
				return nil, false
			}
			dsts[i] = b.pathOperand(p)
		default:
			return nil, false
		}
	}
	return dsts, true
}

// lowerAssignTarget lowers a single target = value pair to the right write op.
func (b *builder) lowerAssignTarget(target, value ast.Expr) {
	switch t := target.(type) {
	case *ast.IdentExpr:
		dst, _ := b.targetOperand(t)
		b.lowerExprInto(dst, value)
	case *ast.AttrGetExpr:
		if p, ok := pathexpr.Resolve(t, b.bindings); ok {
			// Statically known member: container.field <- value.
			b.body.Emit(cir.Instruction{
				Op:    cir.OpStaticMemberWrite,
				Point: b.curPoint,
				Dst:   b.pathOperand(p),
				A:     b.lowerExpr(value),
			})
			return
		}
		// Dynamic index: container[key] <- value.
		container, _ := pathexpr.ResolveMutationContainer(t, b.bindings)
		b.body.Emit(cir.Instruction{
			Op:    cir.OpDynamicIndexWrite,
			Point: b.curPoint,
			Dst:   b.pathOperand(container),
			A:     b.lowerExpr(t.Key),
			B:     b.lowerExpr(value),
		})
	default:
		dst := b.newTemp()
		b.lowerExprInto(dst, value)
	}
}

func (b *builder) lowerCallStmt(s *ast.FuncCallStmt) {
	call, ok := s.Expr.(*ast.FuncCallExpr)
	if !ok {
		b.seq(cfg.NodeCall, func() {})
		return
	}
	b.seq(cfg.NodeCall, func() {
		b.lowerCall(call, nil, false)
	})
}

func (b *builder) lowerReturn(s *ast.ReturnStmt) {
	b.open(cfg.NodeReturn)
	ops, spread := b.lowerValueList(s.Exprs)
	b.body.Emit(cir.Instruction{
		Op:         cir.OpReturn,
		Point:      b.curPoint,
		List:       b.body.AppendOperands(ops),
		ListSpread: spread,
	})
	b.finish()
	// Return terminates the path: edge straight to Exit, nothing falls through.
	b.graph.AddEdge(b.curPoint, b.graph.Exit(), false)
}

func (b *builder) lowerIf(s *ast.IfStmt) {
	b.open(cfg.NodeBranch)
	check := branchcond.Normalize(s.Condition, b.bindings)
	inst := cir.Instruction{Op: cir.OpBranch, Point: b.curPoint, Check: b.body.InternCheck(lowerCheck(check))}
	if check.Kind == branchcond.CheckNone {
		inst.A = b.lowerExpr(s.Condition)
	}
	b.body.Emit(inst)
	b.finish()
	branch := b.curPoint

	// then edge.
	b.pending = []pend{{p: branch, cond: true}}
	b.lowerStmts(s.Then)
	thenEnds := b.pending

	// else edge (may hold an elseif chain as a nested IfStmt).
	b.pending = []pend{{p: branch, cond: false}}
	b.lowerStmts(s.Else)
	elseEnds := b.pending

	b.pending = append(append([]pend{}, thenEnds...), elseEnds...)
}

func (b *builder) lowerWhile(s *ast.WhileStmt) {
	b.open(cfg.NodeBranch)
	check := branchcond.Normalize(s.Condition, b.bindings)
	inst := cir.Instruction{Op: cir.OpBranch, Point: b.curPoint, Check: b.body.InternCheck(lowerCheck(check))}
	if check.Kind == branchcond.CheckNone {
		inst.A = b.lowerExpr(s.Condition)
	}
	b.body.Emit(inst)
	b.finish()
	header := b.curPoint

	b.pushBreak()
	b.pending = []pend{{p: header, cond: true}}
	b.lowerStmts(s.Stmts)
	// Body ends loop back to the header.
	for _, e := range b.pending {
		b.graph.AddEdge(e.p, header, e.cond)
	}
	b.pending = append([]pend{{p: header, cond: false}}, b.popBreak()...)
}

// lowerRepeat lowers `repeat body until cond`. The body always runs once, so it
// precedes the condition branch; the branch exits the loop when cond is true and
// loops back to the body header when false.
func (b *builder) lowerRepeat(s *ast.RepeatStmt) {
	header := b.open(cfg.NodeNoop)
	b.body.Emit(cir.Instruction{Op: cir.OpNoop, Point: header})
	b.finish()

	b.pushBreak()
	b.pending = []pend{{p: header, cond: false}}
	b.lowerStmts(s.Stmts)

	b.open(cfg.NodeBranch)
	check := branchcond.Normalize(s.Condition, b.bindings)
	inst := cir.Instruction{Op: cir.OpBranch, Point: b.curPoint, Check: b.body.InternCheck(lowerCheck(check))}
	if check.Kind == branchcond.CheckNone {
		inst.A = b.lowerExpr(s.Condition)
	}
	b.body.Emit(inst)
	b.finish()
	branch := b.curPoint

	// until cond: true edge exits the loop, false edge repeats the body.
	b.graph.AddEdge(branch, header, false)
	b.pending = append([]pend{{p: branch, cond: true}}, b.popBreak()...)
}

func (b *builder) lowerNumberFor(s *ast.NumberForStmt) {
	b.open(cfg.NodeBranch)
	loopVar := b.numForOperand(s)
	list := []cir.Operand{b.lowerExpr(s.Init), b.lowerExpr(s.Limit)}
	if s.Step != nil {
		list = append(list, b.lowerExpr(s.Step))
	} else {
		list = append(list, b.constNumber("1"))
	}
	b.body.Emit(cir.Instruction{
		Op:      cir.OpIterate,
		Point:   b.curPoint,
		Iter:    cir.IterNumeric,
		Results: b.body.AppendOperands([]cir.Operand{loopVar}),
		List:    b.body.AppendOperands(list),
	})
	b.finish()
	header := b.curPoint

	b.pushBreak()
	b.pending = []pend{{p: header, cond: true}}
	b.lowerStmts(s.Stmts)
	for _, e := range b.pending {
		b.graph.AddEdge(e.p, header, e.cond)
	}
	b.pending = append([]pend{{p: header, cond: false}}, b.popBreak()...)
}

func (b *builder) lowerGenericFor(s *ast.GenericForStmt) {
	b.open(cfg.NodeBranch)
	vars := b.genericForOperands(s)
	srcOps, spread := b.lowerValueList(s.Exprs)
	b.body.Emit(cir.Instruction{
		Op:         cir.OpIterate,
		Point:      b.curPoint,
		Iter:       cir.IterGeneric,
		Results:    b.body.AppendOperands(vars),
		List:       b.body.AppendOperands(srcOps),
		ListSpread: spread,
	})
	b.finish()
	header := b.curPoint

	b.pushBreak()
	b.pending = []pend{{p: header, cond: true}}
	b.lowerStmts(s.Stmts)
	for _, e := range b.pending {
		b.graph.AddEdge(e.p, header, e.cond)
	}
	b.pending = append([]pend{{p: header, cond: false}}, b.popBreak()...)
}

// pushBreak opens a break scope for an enclosing loop.
func (b *builder) pushBreak() {
	b.breakStack = append(b.breakStack, nil)
}

// popBreak closes the innermost break scope and returns its collected open ends.
func (b *builder) popBreak() []pend {
	n := len(b.breakStack) - 1
	ends := b.breakStack[n]
	b.breakStack = b.breakStack[:n]
	return ends
}

// lowerBreak routes the current path to the innermost loop exit and terminates
// straight-line flow (nothing falls through after a break).
func (b *builder) lowerBreak() {
	b.open(cfg.NodeNoop)
	b.body.Emit(cir.Instruction{Op: cir.OpNoop, Point: b.curPoint})
	b.finish()
	if n := len(b.breakStack); n > 0 {
		b.breakStack[n-1] = append(b.breakStack[n-1], pend{p: b.curPoint, cond: false})
	}
	b.pending = nil
}

// lowerLabel materializes a label point, linking both fall-through flow and any
// forward gotos targeting it, then becomes the new open end.
func (b *builder) lowerLabel(s *ast.LabelStmt) {
	gotos := b.pendingGotos[s.Name]
	delete(b.pendingGotos, s.Name)
	p := b.graph.AddNode(cfg.NodeNoop)
	b.linkTo(p)
	for _, from := range gotos {
		b.graph.AddEdge(from, p, false)
	}
	b.begin(p)
	b.body.Emit(cir.Instruction{Op: cir.OpNoop, Point: p})
	b.finish()
	b.labels[s.Name] = p
	b.pending = []pend{{p: p, cond: false}}
}

// lowerGoto emits a jump point: an edge straight to a resolved label, or a
// pending entry for a forward label. Flow does not fall through a goto.
func (b *builder) lowerGoto(s *ast.GotoStmt) {
	b.open(cfg.NodeNoop)
	b.body.Emit(cir.Instruction{Op: cir.OpNoop, Point: b.curPoint})
	b.finish()
	if target, ok := b.labels[s.Label]; ok {
		b.graph.AddEdge(b.curPoint, target, false)
	} else {
		b.pendingGotos[s.Label] = append(b.pendingGotos[s.Label], b.curPoint)
	}
	b.pending = nil
}

// lowerFuncDef lowers a function definition statement (function a.b.c() ... end)
// as a closure value written to its resolved name path.
func (b *builder) lowerFuncDef(s *ast.FuncDefStmt) {
	b.seq(cfg.NodeAssign, func() {
		p, ok := pathexpr.ResolveFuncName(s.Name, b.bindings)
		if ok && len(p.Segments) == 0 {
			b.emitClosure(b.pathOperand(p), s.Func)
			return
		}
		tmp := b.newTemp()
		b.emitClosure(tmp, s.Func)
		if ok {
			// Member/method target: write the closure into the resolved member path.
			b.body.Emit(cir.Instruction{
				Op:    cir.OpStaticMemberWrite,
				Point: b.curPoint,
				Dst:   b.pathOperand(p),
				A:     tmp,
			})
		}
	})
}

// emitClosure lowers fn into its own proto Body and emits an OpClosure into dst
// carrying the capture (upvalue) operands in bind order.
func (b *builder) emitClosure(dst cir.Operand, fn *ast.FunctionExpr) {
	name := b.body.Name + ".fn" + strconv.Itoa(b.protoSeq)
	b.protoSeq++
	child := lowerBody(name, fn.Stmts, b.bindings, fn)
	ref := b.body.AddProto(cir.FuncProto{Name: name, Body: child.Body, Graph: child.Graph})

	caps := b.bindings.DirectCaptures(fn)
	ops := make([]cir.Operand, 0, len(caps))
	for _, c := range caps {
		ops = append(ops, b.pathOperand(path.NewPath(c.Captured, c.CapturedName)))
	}
	b.body.Emit(cir.Instruction{
		Op:    cir.OpClosure,
		Point: b.curPoint,
		Dst:   dst,
		Func:  ref,
		List:  b.body.AppendOperands(ops),
	})
}

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
	b.body.Emit(cir.Instruction{
		Op:            cir.OpSelect,
		Point:         b.curPoint,
		Dst:           dst,
		List:          b.body.AppendOperands(ops),
		SelectDefault: hasDefault,
	})
	return true
}

// lowerExprInto lowers e so its value lands in dst, choosing the producing
// instruction by syntax.
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
		b.body.Emit(cir.Instruction{
			Op:    cir.OpConcat,
			Point: b.curPoint,
			Dst:   dst,
			List:  b.body.AppendOperands(ops),
		})
	case *ast.UnaryMinusOpExpr:
		b.emitUnOp(dst, cir.UnNeg, e.Expr)
	case *ast.UnaryNotOpExpr:
		b.emitUnOp(dst, cir.UnNot, e.Expr)
	case *ast.UnaryLenOpExpr:
		b.emitUnOp(dst, cir.UnLen, e.Expr)
	case *ast.UnaryBNotOpExpr:
		b.emitUnOp(dst, cir.UnBNot, e.Expr)
	case *ast.FuncCallExpr:
		b.lowerCall(e, []cir.Operand{dst}, false)
	case *ast.CastExpr:
		src := b.lowerExpr(e.Expr)
		b.body.Emit(cir.Instruction{
			Op:    cir.OpClaim,
			Point: b.curPoint,
			Dst:   dst,
			A:     src,
			Claim: cir.ClaimCast,
			Type:  b.internType(e.Type),
		})
	case *ast.NonNilAssertExpr:
		src := b.lowerExpr(e.Expr)
		b.body.Emit(cir.Instruction{
			Op:    cir.OpClaim,
			Point: b.curPoint,
			Dst:   dst,
			A:     src,
			Claim: cir.ClaimAssert,
		})
	case *ast.LogicalOpExpr:
		b.emitLogical(dst, e)
	case *ast.FunctionExpr:
		b.emitClosure(dst, e)
	case *ast.TableExpr:
		b.lowerTable(dst, e)
	case *ast.Comma3Expr:
		b.emitAssign(dst, cir.Operand{Kind: cir.OperandVararg})
	default:
		// Out of subset: opaque source.
		b.emitAssign(dst, cir.Operand{})
	}
}

// emitLogical lowers a short-circuit and/or into an OpLogical value form. The
// operands are lowered eagerly; the short-circuit result and the guard narrowing
// the right operand inherits are derived by transfer, not concluded here.
func (b *builder) emitLogical(dst cir.Operand, e *ast.LogicalOpExpr) {
	op := cir.LogAnd
	if e.Operator == "or" {
		op = cir.LogOr
	}
	a := b.lowerExpr(e.Lhs)
	c := b.lowerExpr(e.Rhs)
	b.body.Emit(cir.Instruction{Op: cir.OpLogical, Point: b.curPoint, Dst: dst, A: a, B: c, Operator: op})
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
	case *ast.Comma3Expr:
		return cir.Operand{Kind: cir.OperandVararg}
	default:
		t := b.newTemp()
		b.lowerExprInto(t, e)
		return t
	}
}

// lowerMultiValue lowers a tail-position expression that expands to all its
// values, marking the producing call as multret. It returns the head operand.
func (b *builder) lowerMultiValue(e ast.Expr) cir.Operand {
	switch e := e.(type) {
	case *ast.FuncCallExpr:
		head := b.newTemp()
		b.lowerCall(e, []cir.Operand{head}, true)
		return head
	case *ast.Comma3Expr:
		return cir.Operand{Kind: cir.OperandVararg}
	default:
		return b.lowerExpr(e)
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

// lowerCall emits an OpCall. results are the bound destinations (nil for a
// statement call); resultSpread marks the call as multret (open result count).
func (b *builder) lowerCall(call *ast.FuncCallExpr, results []cir.Operand, resultSpread bool) {
	// A recognized channel.select bound to a single result lowers to OpSelect.
	if len(results) == 1 && b.maybeLowerSelect(results[0], call) {
		return
	}
	args, argSpread := b.lowerValueList(call.Args)
	inst := cir.Instruction{
		Op:           cir.OpCall,
		Point:        b.curPoint,
		List:         b.body.AppendOperands(args),
		Results:      b.body.AppendOperands(results),
		ListSpread:   argSpread,
		ResultSpread: resultSpread,
	}
	if call.Method != "" {
		// Method-call sugar recv:m(): keep receiver binding explicit so a codegen
		// backend emits SELF without re-parsing a folded member-call blob.
		inst.Call.Method = b.internString(call.Method)
		if call.Receiver != nil {
			inst.Call.Receiver = b.lowerExpr(call.Receiver)
		}
	} else {
		inst.Call.Callee = b.calleeOperand(call.Func)
	}
	b.body.Emit(inst)
}

// lowerTable lowers a table constructor over its array, hash, and trailing
// spread parts. Every field value becomes a List operand; a final keyless
// multi-value producer ({..., f()}) marks the list tail as an open spread. Field
// keys are structural syntax recovered by transfer from the constructor, not
// lowered as operands.
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
	b.body.Emit(cir.Instruction{
		Op:         cir.OpMakeTable,
		Point:      b.curPoint,
		Dst:        dst,
		List:       b.body.AppendOperands(ops),
		ListSpread: spread,
	})
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

func (b *builder) emitAssign(dst, src cir.Operand) {
	b.body.Emit(cir.Instruction{Op: cir.OpAssign, Point: b.curPoint, Dst: dst, A: src})
}

func (b *builder) emitBinOp(dst cir.Operand, op cir.Operator, lhs, rhs ast.Expr) {
	a := b.lowerExpr(lhs)
	c := b.lowerExpr(rhs)
	b.body.Emit(cir.Instruction{Op: cir.OpBinOp, Point: b.curPoint, Dst: dst, A: a, B: c, Operator: op})
}

func (b *builder) emitUnOp(dst cir.Operand, op cir.Operator, operand ast.Expr) {
	a := b.lowerExpr(operand)
	b.body.Emit(cir.Instruction{Op: cir.OpUnOp, Point: b.curPoint, Dst: dst, A: a, Operator: op})
}

// readOperand returns the operand for a value read: a path operand when the
// expression resolves to a static path, else a temp holding an opaque read.
func (b *builder) readOperand(e ast.Expr) cir.Operand {
	if p, ok := pathexpr.Resolve(e, b.bindings); ok {
		return b.pathOperand(p)
	}
	t := b.newTemp()
	b.body.Emit(cir.Instruction{Op: cir.OpAssign, Point: b.curPoint, Dst: t, A: cir.Operand{}})
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

func (b *builder) internType(t ast.TypeExpr) cir.TypeRef {
	return b.body.InternType(spellType(t))
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
// IR keeps only the resolved path and type identities, so the descriptor crosses
// into the IR layer without carrying any syntax dependency. The kind enumerations
// are defined in lockstep, so the kind maps by position.
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
