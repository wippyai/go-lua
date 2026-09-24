package assign

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/cond"
	fbcore "github.com/wippyai/go-lua/compiler/check/flowbuild/core"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/guard"
	fbpath "github.com/wippyai/go-lua/compiler/check/flowbuild/path"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/predicate"
	"github.com/wippyai/go-lua/compiler/check/synth/ops"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
)

type narrowResolverAdapter struct {
	ctx *db.QueryContext
	ops core.TypeOps
}

var _ narrow.Resolver = (*narrowResolverAdapter)(nil)

func (r narrowResolverAdapter) Field(t typ.Type, name string) (typ.Type, bool) {
	if r.ops == nil {
		return nil, false
	}
	return r.ops.Field(r.ctx, t, name)
}

func (r narrowResolverAdapter) Index(t typ.Type, key typ.Type) (typ.Type, bool) {
	if r.ops == nil {
		return nil, false
	}
	return r.ops.Index(r.ctx, t, key)
}

// buildPreflowBranchSolution solves only branch/numeric edge facts that are
// already available before assignment extraction completes.
//
// This gives local inference access to canonical branch narrowing such as
// discriminant checks on parameters, without depending on later assignment-
// derived facts or full post-extraction solve. Every write in the graph is
// recorded without a type, so a branch fact about a value ends where the
// value is reassigned.
func buildPreflowBranchSolution(fc *fbcore.FlowContext, inputs *flow.Inputs) *flow.Solution {
	if fc == nil || inputs == nil || inputs.Graph == nil || fc.TypeOps == nil {
		return nil
	}

	temp := *inputs
	temp.Assignments = untypedWrites(fc.Graph)
	temp.EdgeConditions = nil
	temp.EdgeNumericConstraints = nil

	cond.ExtractEdgeConstraints(fc, &temp)
	cond.ExtractNumericConstraints(fc, &temp)

	return flow.Solve(&temp, narrowResolverAdapter{ctx: fc.CallCtx, ops: fc.TypeOps})
}

// untypedWrites lists the writes of graph as assignments without a type: a
// write to a variable, and a field or index write or a function definition
// through a variable, which writes that variable's value.
func untypedWrites(graph *cfg.Graph) []flow.UnifiedAssignment {
	if graph == nil {
		return nil
	}
	var writes []flow.UnifiedAssignment
	write := func(p cfg.Point, sym cfg.SymbolID, name string) {
		if sym == 0 {
			return
		}
		writes = append(writes, flow.UnifiedAssignment{
			Point:      p,
			TargetPath: constraint.Path{Root: name, Symbol: sym},
		})
	}
	graph.EachAssign(func(p cfg.Point, info *cfg.AssignInfo) {
		if info == nil {
			return
		}
		for _, target := range info.Targets {
			if target.Kind == cfg.TargetIdent {
				write(p, target.Symbol, target.Name)
			} else {
				write(p, target.BaseSymbol, target.BaseName)
			}
		}
	})
	graph.EachFuncDef(func(p cfg.Point, info *cfg.FuncDefInfo) {
		if info == nil {
			return
		}
		write(p, info.TargetPath.Symbol, info.TargetPath.Root)
	})
	return writes
}

// synthWithOverlayAndPreflow wraps base synthesis with overlay lookup and a
// preflow branch-narrowing view for identifiers and attribute/index reads.
//
// This keeps assignment inference on the canonical synthesis path while letting
// recursive field/index expressions observe already-provable branch facts.
// The operands of `and`/`or` are synthesized the same way, and the right
// operand sees the facts its left operand establishes, as in the check phase.
func synthWithOverlayAndPreflow(
	overlay map[cfg.SymbolID]typ.Type,
	bindings *bind.BindingTable,
	inputs *flow.Inputs,
	callCtx *db.QueryContext,
	typeOps core.TypeOps,
	preflow *flow.Solution,
	base func(ast.Expr, cfg.Point) typ.Type,
) func(ast.Expr, cfg.Point) typ.Type {
	var synth func(ast.Expr, cfg.Point, []operandFact) typ.Type

	pathOf := func(expr ast.Expr, p cfg.Point) constraint.Path {
		if bindings == nil {
			return constraint.Path{}
		}
		return fbpath.FromExprWithBindings(expr, predicate.BuildConstResolver(inputs, p), bindings)
	}

	// leftOperandFact returns the fact the right operand of ex observes about
	// a path read by its left operand: a type() test of the path, or the
	// truthiness of the left operand itself.
	leftOperandFact := func(ex *ast.LogicalOpExpr, left typ.Type, p cfg.Point, facts []operandFact) (operandFact, bool) {
		if rel, ok := ex.Lhs.(*ast.RelationalOpExpr); ok {
			if arg, key, holdsOnEqual, ok := guard.TypeGuardOperand(rel); ok {
				holds := (ex.Operator == "and") == holdsOnEqual
				if path := pathOf(arg, p); holds && !path.IsEmpty() {
					if narrowed := narrow.ByTypeKey(synth(arg, p, facts), key, nil); narrowed != nil && !narrowed.Kind().IsNever() {
						return operandFact{path: path, t: narrowed}, true
					}
				}
				return operandFact{}, false
			}
		}
		path := pathOf(ex.Lhs, p)
		if path.IsEmpty() || !ops.CanBeFalsy(left) {
			return operandFact{}, false
		}
		var narrowed typ.Type
		switch ex.Operator {
		case "and":
			narrowed = narrow.ToTruthy(left)
		case "or":
			narrowed = narrow.ToFalsy(left)
		}
		if narrowed == nil || narrowed.Kind().IsNever() {
			return operandFact{}, false
		}
		return operandFact{path: path, t: narrowed}, true
	}

	synth = func(expr ast.Expr, p cfg.Point, facts []operandFact) typ.Type {
		if expr == nil {
			return nil
		}

		if len(facts) > 0 {
			if path := pathOf(expr, p); !path.IsEmpty() {
				for i := len(facts) - 1; i >= 0; i-- {
					if facts[i].path.Equal(path) {
						return facts[i].t
					}
				}
			}
		}

		if ident, ok := expr.(*ast.IdentExpr); ok && bindings != nil {
			if sym, ok := bindings.SymbolOf(ident); ok && sym != 0 {
				if t, exists := overlay[sym]; exists {
					return t
				}
			}
		}

		if preflow != nil && bindings != nil && inputs != nil {
			if path := pathOf(expr, p); !path.IsEmpty() {
				if narrowed := preflow.NarrowedTypeAt(p, path); !typ.IsAbsentOrUnknown(narrowed) {
					return narrowed
				}
			}
		}

		switch ex := expr.(type) {
		case *ast.AttrGetExpr:
			if typeOps == nil {
				break
			}
			objType := synth(ex.Object, p, facts)
			if !typ.IsAbsentOrUnknown(objType) {
				switch key := ex.Key.(type) {
				case *ast.StringExpr:
					if ft, ok := typeOps.Field(callCtx, objType, key.Value); ok && !typ.IsAbsentOrUnknown(ft) {
						return ft
					}
					if it, ok := typeOps.Index(callCtx, objType, typ.LiteralString(key.Value)); ok && !typ.IsAbsentOrUnknown(it) {
						return it
					}
				default:
					keyType := synth(ex.Key, p, facts)
					if !typ.IsAbsentOrUnknown(keyType) {
						if it, ok := typeOps.Index(callCtx, objType, keyType); ok && !typ.IsAbsentOrUnknown(it) {
							return it
						}
					}
				}
			}
		case *ast.LogicalOpExpr:
			left := synth(ex.Lhs, p, facts)
			rightFacts := facts
			if fact, ok := leftOperandFact(ex, left, p, facts); ok {
				rightFacts = append(append(make([]operandFact, 0, len(facts)+1), facts...), fact)
			}
			right := synth(ex.Rhs, p, rightFacts)
			switch ex.Operator {
			case "and":
				return ops.LogicalAndTyped(left, right)
			case "or":
				return ops.LogicalOrTyped(left, right)
			}
		}

		if base == nil {
			return nil
		}
		return base(expr, p)
	}

	return func(expr ast.Expr, p cfg.Point) typ.Type {
		return synth(expr, p, nil)
	}
}

// operandFact is the type a logical operator's left operand establishes for
// a path read by its right operand.
type operandFact struct {
	path constraint.Path
	t    typ.Type
}
