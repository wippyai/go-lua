package check

import (
	"sort"
	"strings"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/cfg/analysis"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/returns"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/typ"
)

// exportTruthyCallbackCalls summarizes calls through a module field only when
// every normal path returning a truthy first result goes through that call.
// The field may still be nil in the module; the caller must prove it installed
// a function before using the summary.
func (s *Session) exportTruthyCallbackCalls(manifest *io.Manifest) {
	if s == nil || manifest == nil || s.RootGraph() == nil {
		return
	}
	rec, ok := manifest.Export.(*typ.Record)
	if !ok {
		return
	}
	root := s.RootGraph()
	for _, nested := range root.NestedFunctions() {
		owner, name, ok := strings.Cut(root.NameOf(nested.Symbol), ".")
		if !ok || strings.Contains(name, ".") || !exportRootIs(root, owner) {
			continue
		}
		field := rec.GetField(name)
		if field == nil {
			continue
		}
		if _, ok := field.Type.(*typ.Function); !ok {
			continue
		}
		res := s.Results[nested.Func]
		if res == nil || res.Graph == nil {
			continue
		}
		g := res.Graph
		idom, _ := analysis.ComputeDominators(g.CFG())
		truthyReturns, classified := truthyReturnPoints(res)
		if !classified || len(truthyReturns) == 0 {
			continue
		}
		g.EachAssign(func(_ cfg.Point, assign *cfg.AssignInfo) {
			for i, target := range assign.Targets {
				if target.Kind != cfg.TargetIdent || target.Symbol == 0 || i >= len(assign.Sources) {
					continue
				}
				or, ok := assign.Sources[i].(*ast.LogicalOpExpr)
				if !ok || or.Operator != "or" {
					continue
				}
				attr, ok := or.Lhs.(*ast.AttrGetExpr)
				if !ok {
					continue
				}
				ident, ok := attr.Object.(*ast.IdentExpr)
				if !ok || ident.Value != owner {
					continue
				}
				key, ok := attr.Key.(*ast.StringExpr)
				if !ok || key.Value == "" || !singleLocalAssignment(g, target.Symbol) {
					continue
				}
				g.EachCallSite(func(callPoint cfg.Point, call *cfg.CallInfo) {
					if call == nil || call.CalleePath.Symbol != target.Symbol || len(call.CalleePath.Segments) != 0 || !returns.CallEvaluatedAtPoint(g, callPoint, call) {
						return
					}
					for _, retPoint := range truthyReturns {
						if !analysis.Dominates(idom, callPoint, retPoint) {
							return
						}
					}
					var nonNil []int
					for argIndex, arg := range call.Args {
						if nonNilArgumentAt(g, idom, callPoint, arg) {
							nonNil = append(nonNil, argIndex)
						}
					}
					prior := s.priorModuleCallbackFields(root, g, owner, key.Value, callPoint)
					if prior == nil {
						return
					}
					if manifest.TruthyCallbackCalls == nil {
						manifest.TruthyCallbackCalls = make(map[string][]io.CallbackCall)
					}
					manifest.TruthyCallbackCalls[name] = append(manifest.TruthyCallbackCalls[name], io.CallbackCall{Field: key.Value, NonNilArgs: nonNil, PriorFields: prior})
				})
			}
		})
	}
}

// A mutable callback reached before the summarized call can change the
// summarized field. Record it so the importer can verify the installed body.
// A write to the summarized field in this function invalidates the summary.
func (s *Session) priorModuleCallbackFields(root, g *cfg.Graph, owner, field string, callPoint cfg.Point) []string {
	prior := make(map[string]bool)
	valid := true
	g.EachAssign(func(p cfg.Point, assign *cfg.AssignInfo) {
		if p == callPoint || !mayReachPoint(g, p, callPoint) {
			return
		}
		for _, target := range assign.Targets {
			if target.Kind == cfg.TargetField && target.BaseName == owner && len(target.FieldPath) == 1 && target.FieldPath[0] == field {
				valid = false
			}
		}
	})
	visit := func(p cfg.Point, expr ast.Expr) {
		if p == callPoint || !mayReachPoint(g, p, callPoint) {
			return
		}
		callsInExpr(expr, func(call *ast.FuncCallExpr) {
			if name := moduleFieldCall(call, owner); name != "" {
				if name == field {
					valid = false
				} else {
					prior[name] = true
				}
				return
			}
			if !s.safePriorCall(root, g, p, call, owner, map[cfg.SymbolID]bool{}) {
				valid = false
			}
		})
	}
	g.EachAssign(func(p cfg.Point, assign *cfg.AssignInfo) {
		for _, source := range assign.Sources {
			visit(p, source)
		}
	})
	g.EachBranch(func(p cfg.Point, branch *cfg.BranchInfo) { visit(p, branch.Condition) })
	g.EachReturn(func(p cfg.Point, ret *cfg.ReturnInfo) {
		for _, expr := range ret.Exprs {
			visit(p, expr)
		}
	})
	g.EachStmtCall(func(p cfg.Point, call *cfg.CallInfo) {
		if call != nil {
			visit(p, call.Call)
		}
	})
	if !valid {
		return nil
	}
	fields := make([]string, 0, len(prior))
	for name := range prior {
		fields = append(fields, name)
	}
	sort.Strings(fields)
	return fields
}

func mayReachPoint(g *cfg.Graph, from, to cfg.Point) bool {
	if from == to {
		return true
	}
	seen := map[cfg.Point]bool{from: true}
	work := []cfg.Point{from}
	for len(work) > 0 {
		p := work[len(work)-1]
		work = work[:len(work)-1]
		for _, next := range g.SuccessorsReadOnly(p) {
			if next == to {
				return true
			}
			if !seen[next] {
				seen[next] = true
				work = append(work, next)
			}
		}
	}
	return false
}

func callsInExpr(expr ast.Expr, visit func(*ast.FuncCallExpr)) {
	switch x := expr.(type) {
	case *ast.FuncCallExpr:
		visit(x)
		callsInExpr(x.Func, visit)
		callsInExpr(x.Receiver, visit)
		for _, arg := range x.Args {
			callsInExpr(arg, visit)
		}
	case *ast.LogicalOpExpr:
		callsInExpr(x.Lhs, visit)
		callsInExpr(x.Rhs, visit)
	case *ast.UnaryNotOpExpr:
		callsInExpr(x.Expr, visit)
	case *ast.AttrGetExpr:
		callsInExpr(x.Object, visit)
		callsInExpr(x.Key, visit)
	case *ast.TableExpr:
		for _, field := range x.Fields {
			callsInExpr(field.Key, visit)
			callsInExpr(field.Value, visit)
		}
	case *ast.RelationalOpExpr:
		callsInExpr(x.Lhs, visit)
		callsInExpr(x.Rhs, visit)
	case *ast.StringConcatOpExpr:
		callsInExpr(x.Lhs, visit)
		callsInExpr(x.Rhs, visit)
	case *ast.ArithmeticOpExpr:
		callsInExpr(x.Lhs, visit)
		callsInExpr(x.Rhs, visit)
	case *ast.UnaryMinusOpExpr:
		callsInExpr(x.Expr, visit)
	case *ast.UnaryLenOpExpr:
		callsInExpr(x.Expr, visit)
	case *ast.UnaryBNotOpExpr:
		callsInExpr(x.Expr, visit)
	case *ast.CastExpr:
		callsInExpr(x.Expr, visit)
	case *ast.NonNilAssertExpr:
		callsInExpr(x.Expr, visit)
	}
}

func moduleFieldCall(call *ast.FuncCallExpr, owner string) string {
	if call == nil {
		return ""
	}
	callee := call.Func
	if or, ok := callee.(*ast.LogicalOpExpr); ok && or.Operator == "or" {
		callee = or.Lhs
	}
	attr, ok := callee.(*ast.AttrGetExpr)
	if !ok {
		return ""
	}
	ident, ok := attr.Object.(*ast.IdentExpr)
	if !ok || ident.Value != owner {
		return ""
	}
	key, _ := attr.Key.(*ast.StringExpr)
	if key == nil {
		return ""
	}
	return key.Value
}

func (s *Session) safePriorCall(root, g *cfg.Graph, p cfg.Point, call *ast.FuncCallExpr, owner string, visiting map[cfg.SymbolID]bool) bool {
	if call.Method == "gsub" {
		cast, ok := call.Receiver.(*ast.CastExpr)
		if !ok {
			return false
		}
		annotation, ok := cast.Type.(*ast.PrimitiveTypeExpr)
		if !ok || annotation.Name != "string" {
			return false
		}
		ident, ok := cast.Expr.(*ast.IdentExpr)
		if !ok || g.Bindings() == nil {
			return false
		}
		sym, ok := g.Bindings().SymbolOf(ident)
		if !ok || s.Results[g.Func()] == nil {
			return false
		}
		t := s.Results[g.Func()].NarrowedTypeAt(p, constraint.Path{Root: ident.Value, Symbol: sym})
		return t != nil && t.Kind() == kind.String
	}
	callee, ok := call.Func.(*ast.IdentExpr)
	if !ok || g.Bindings() == nil {
		return false
	}
	sym, ok := g.Bindings().SymbolOf(callee)
	if !ok || sym == 0 {
		return false
	}
	if callee.Value == "type" {
		bindingKind, ok := g.Bindings().Kind(sym)
		return ok && bindingKind == cfg.SymbolGlobal
	}
	for _, nested := range root.NestedFunctions() {
		if nested.Symbol == sym && root.NameOf(sym) == callee.Value && !visiting[sym] {
			visiting[sym] = true
			defer delete(visiting, sym)
			return s.safeLocalHelper(root, s.Results[nested.Func], owner, visiting)
		}
	}
	return false
}

func (s *Session) safeLocalHelper(root *cfg.Graph, res *api.FuncResult, owner string, visiting map[cfg.SymbolID]bool) bool {
	if res == nil || res.Graph == nil {
		return false
	}
	g := res.Graph
	safe := true
	check := func(p cfg.Point, expr ast.Expr) {
		callsInExpr(expr, func(call *ast.FuncCallExpr) {
			if moduleFieldCall(call, owner) != "" || !s.safePriorCall(root, g, p, call, owner, visiting) {
				safe = false
			}
		})
	}
	g.EachAssign(func(p cfg.Point, assign *cfg.AssignInfo) {
		for _, target := range assign.Targets {
			if !assign.IsLocal || target.Kind != cfg.TargetIdent {
				safe = false
			}
		}
		for _, source := range assign.Sources {
			check(p, source)
		}
	})
	g.EachBranch(func(p cfg.Point, branch *cfg.BranchInfo) { check(p, branch.Condition) })
	g.EachReturn(func(p cfg.Point, ret *cfg.ReturnInfo) {
		for _, expr := range ret.Exprs {
			check(p, expr)
		}
	})
	g.EachStmtCall(func(p cfg.Point, call *cfg.CallInfo) {
		if call != nil {
			check(p, call.Call)
		}
	})
	return safe
}

func exportRootIs(g *cfg.Graph, owner string) bool {
	found := false
	valid := true
	g.EachReturn(func(_ cfg.Point, ret *cfg.ReturnInfo) {
		if len(ret.Exprs) == 0 {
			valid = false
			return
		}
		ident, ok := ret.Exprs[0].(*ast.IdentExpr)
		if !ok || ident.Value != owner {
			valid = false
			return
		}
		found = true
	})
	return found && valid
}

func truthyReturnPoints(res *api.FuncResult) ([]cfg.Point, bool) {
	var points []cfg.Point
	classified := true
	res.Graph.EachReturn(func(p cfg.Point, ret *cfg.ReturnInfo) {
		if len(ret.Exprs) == 0 {
			return
		}
		switch first := ret.Exprs[0].(type) {
		case *ast.NilExpr, *ast.FalseExpr:
			return
		case *ast.StringExpr, *ast.NumberExpr, *ast.TrueExpr, *ast.TableExpr, *ast.FunctionExpr:
			points = append(points, p)
		case *ast.IdentExpr:
			sym, ok := res.Graph.SymbolAt(p, first.Value)
			if !ok || sym == 0 {
				classified = false
				return
			}
			t := res.NarrowedTypeAt(p, constraint.Path{Root: first.Value, Symbol: sym})
			if t == nil {
				classified = false
				return
			}
			switch t.Kind() {
			case kind.String, kind.Integer, kind.Number, kind.Record, kind.Array, kind.Map, kind.Function:
				points = append(points, p)
			case kind.Nil:
				return
			default:
				classified = false
			}
		default:
			classified = false
		}
	})
	return points, classified
}

func singleLocalAssignment(g *cfg.Graph, symbol cfg.SymbolID) bool {
	count := 0
	g.EachAssign(func(_ cfg.Point, assign *cfg.AssignInfo) {
		for _, target := range assign.Targets {
			if target.Kind == cfg.TargetIdent && target.Symbol == symbol {
				count++
			}
		}
	})
	return count == 1
}

func nonNilArgumentAt(g *cfg.Graph, idom map[cfg.Point]cfg.Point, p cfg.Point, expr ast.Expr) bool {
	switch expr.(type) {
	case *ast.TableExpr, *ast.FunctionExpr, *ast.StringExpr, *ast.NumberExpr, *ast.TrueExpr:
		return true
	}
	ident, ok := expr.(*ast.IdentExpr)
	if !ok || g.Bindings() == nil {
		return false
	}
	sym, ok := g.Bindings().SymbolOf(ident)
	if !ok || sym == 0 || !singleLocalAssignment(g, sym) {
		return false
	}
	proved := false
	g.EachAssign(func(ap cfg.Point, assign *cfg.AssignInfo) {
		for i, target := range assign.Targets {
			if target.Kind != cfg.TargetIdent || target.Symbol != sym || i >= len(assign.Sources) {
				continue
			}
			if _, ok := assign.Sources[i].(*ast.TableExpr); ok && analysis.Dominates(idom, ap, p) {
				proved = true
			}
		}
	})
	return proved
}
