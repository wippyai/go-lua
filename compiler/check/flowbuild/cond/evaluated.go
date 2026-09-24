package cond

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/core"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// ReceiverRoots keeps dereference facts for values with a usable flow type.
// An unresolved root has no type that a NotNil fact can narrow; retaining such
// facts can repeatedly expand inferred unions at joins.
func ReceiverRoots(inputs *flow.Inputs, graph *cfg.Graph) (map[cfg.SymbolID]bool, map[cfg.SymbolID]bool, map[constraint.PathKey]bool) {
	if inputs == nil {
		return nil, nil, nil
	}
	roots := make(map[cfg.SymbolID]bool)
	nilable := make(map[cfg.SymbolID]bool)
	knownNonNil := make(map[constraint.PathKey]bool)
	add := func(sym cfg.SymbolID, t typ.Type) {
		if sym != 0 && t != nil && !typ.IsUnknown(t) && !typ.IsAny(t) {
			roots[sym] = true
			if unwrap.IsOptionalLike(t) {
				nilable[sym] = true
			}
		}
	}
	for sym, t := range inputs.DeclaredTypes {
		add(sym, t)
	}
	for sym, t := range inputs.SiblingTypes {
		add(sym, t)
	}
	// A successful dereference of an `any` parameter is still a runtime proof
	// about the argument. Keep it for the function's OnReturn summary, where it
	// can narrow a typed argument at a call site. Restrict this to parameters:
	// arbitrary unresolved locals have no stable flow type to refine.
	if graph != nil {
		for _, sym := range graph.ParamSymbols() {
			if typ.IsAny(inputs.DeclaredTypes[sym]) {
				roots[sym] = true
				nilable[sym] = true
			}
		}
	}
	for _, assignment := range inputs.Assignments {
		if len(assignment.TargetPath.Segments) == 0 {
			add(assignment.TargetPath.Symbol, assignment.Type)
		} else if assignment.Type != nil && !unwrap.IsOptionalLike(assignment.Type) {
			p := assignment.TargetPath
			p.Version = 0
			knownNonNil[p.Key()] = true
		}
	}
	return roots, nilable, knownNonNil
}

// CapturedReassignments finds locals that a nested function can rebind. A call
// can run such a function without adding an SSA assignment in the caller, so a
// preceding dereference cannot establish their value after the call.
func CapturedReassignments(graph *cfg.Graph) map[cfg.SymbolID]bool {
	if graph == nil || graph.Bindings() == nil {
		return nil
	}
	bindings := graph.Bindings()
	var reassigned map[cfg.SymbolID]bool
	var visit func(*cfg.Graph)
	visit = func(parent *cfg.Graph) {
		for _, nested := range parent.NestedFunctions() {
			if nested.Func == nil {
				continue
			}
			child := cfg.BuildWithBindings(nested.Func, bindings)
			captured := make(map[cfg.SymbolID]bool)
			for _, sym := range bindings.CapturedSymbols(nested.Func) {
				captured[sym] = true
			}
			child.EachAssign(func(_ cfg.Point, info *cfg.AssignInfo) {
				for _, target := range info.Targets {
					if target.Kind == cfg.TargetIdent && captured[target.Symbol] {
						if reassigned == nil {
							reassigned = make(map[cfg.SymbolID]bool)
						}
						reassigned[target.Symbol] = true
					}
				}
			})
			visit(child)
		}
	}
	visit(graph)
	return reassigned
}

func (ce *ConditionExtractor) canRetainPath(path constraint.Path) bool {
	if path.Symbol == 0 || ce.UnstableSymbols[path.Symbol] || !ce.ReceiverRoots[path.Symbol] {
		return false
	}
	if len(path.Segments) == 0 && !ce.NilableRoots[path.Symbol] {
		return false
	}
	unversioned := path
	unversioned.Version = 0
	if ce.KnownNonNilPaths[unversioned.Key()] {
		return false
	}
	if bindings := ce.bindings(); bindings != nil {
		if kind, ok := bindings.Kind(path.Symbol); ok && kind == cfg.SymbolGlobal {
			return false
		}
	}
	return true
}

// evaluatedReceiverFacts describes dereferences that must have completed when
// an expression returns normally. A nil receiver would have raised before that
// point. A logical right operand is excluded because it can be skipped.
func (ce *ConditionExtractor) evaluatedReceiverFacts(expr ast.Expr) constraint.Condition {
	var facts []constraint.Constraint
	var visit func(ast.Expr)
	visit = func(expr ast.Expr) {
		switch e := expr.(type) {
		case *ast.AttrGetExpr:
			visit(e.Object)
			visit(e.Key)
			if p := ce.pathFromExpr(e.Object); ce.canRetainPath(p) {
				facts = append(facts, constraint.NotNil{Path: p})
			}
		case *ast.FuncCallExpr:
			visit(e.Func)
			visit(e.Receiver)
			for _, arg := range e.Args {
				visit(arg)
			}
			if e.Receiver != nil {
				if p := ce.pathFromExpr(e.Receiver); ce.canRetainPath(p) {
					facts = append(facts, constraint.NotNil{Path: p})
				}
			}
		case *ast.LogicalOpExpr:
			visit(e.Lhs)
		case *ast.RelationalOpExpr:
			visit(e.Lhs)
			visit(e.Rhs)
			switch e.Operator {
			case "<", "<=", ">", ">=":
				// A completed ordered comparison cannot have compared nil.
				for _, operand := range []ast.Expr{e.Lhs, e.Rhs} {
					if p := ce.pathFromExpr(operand); ce.canRetainPath(p) {
						facts = append(facts, constraint.NotNil{Path: p})
					}
				}
			}
		case *ast.ArithmeticOpExpr:
			visit(e.Lhs)
			visit(e.Rhs)
		case *ast.StringConcatOpExpr:
			visit(e.Lhs)
			visit(e.Rhs)
		case *ast.UnaryNotOpExpr:
			visit(e.Expr)
		case *ast.UnaryMinusOpExpr:
			visit(e.Expr)
		case *ast.UnaryLenOpExpr:
			visit(e.Expr)
		case *ast.UnaryBNotOpExpr:
			visit(e.Expr)
		case *ast.CastExpr:
			visit(e.Expr)
		case *ast.NonNilAssertExpr:
			visit(e.Expr)
		case *ast.TableExpr:
			for _, field := range e.Fields {
				visit(field.Key)
				visit(field.Value)
			}
		}
	}
	visit(expr)
	return constraint.FromConstraints(facts...)
}

func (ce *ConditionExtractor) conditionsFromEvaluatedExpr(expr ast.Expr) BranchConditions {
	bc := ce.ConstraintsFromConditionExpr(expr)
	facts := ce.evaluatedReceiverFacts(expr)
	if facts.HasConstraints() {
		bc.OnTrue = constraint.And(bc.OnTrue, facts)
		bc.OnFalse = constraint.And(bc.OnFalse, facts)
	}
	return bc
}

// ExtractEvaluationConstraints puts normal-return receiver facts on outgoing
// edges of statements. The statement itself is checked before these facts hold.
func ExtractEvaluationConstraints(fc *core.FlowContext, inputs *flow.Inputs) {
	if fc == nil || fc.Graph == nil || fc.Derived == nil || inputs == nil {
		return
	}
	emit := func(p cfg.Point, exprs []ast.Expr, assigned map[cfg.SymbolID]bool) {
		ce := newConditionExtractor(fc, inputs, p)
		var must []constraint.Constraint
		for _, expr := range exprs {
			for _, fact := range ce.evaluatedReceiverFacts(expr).MustConstraints() {
				if n, ok := fact.(constraint.NotNil); ok && !assigned[n.Path.Symbol] {
					must = append(must, n)
				}
			}
		}
		if len(must) == 0 {
			return
		}
		condition := constraint.FromConstraints(must...)
		for _, succ := range fc.Graph.Successors(p) {
			inputs.EdgeConditions = append(inputs.EdgeConditions, flow.EdgeCondition{From: p, To: succ, Condition: condition})
		}
	}
	fc.Graph.EachAssign(func(p cfg.Point, info *cfg.AssignInfo) {
		assigned := make(map[cfg.SymbolID]bool)
		for _, target := range info.Targets {
			if target.Kind == cfg.TargetIdent && target.Symbol != 0 {
				assigned[target.Symbol] = true
			}
		}
		emit(p, info.Sources, assigned)
	})
	fc.Graph.EachStmtCall(func(p cfg.Point, info *cfg.CallInfo) {
		if info != nil && info.Call != nil {
			emit(p, []ast.Expr{info.Call}, nil)
		}
	})
}
