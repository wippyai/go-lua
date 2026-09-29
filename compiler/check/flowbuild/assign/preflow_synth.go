package assign

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/cond"
	fbcore "github.com/wippyai/go-lua/compiler/check/flowbuild/core"
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

// preflowFacts is what assignment inference knows about conditions before
// the full solve: the branch facts reaching each point, and the conditions an
// expression establishes, extracted as for branch edges.
type preflowFacts struct {
	graph      *cfg.Graph
	solution   *flow.Solution
	conditions api.ConditionFromExprFunc
}

// buildPreflowFacts solves only branch/numeric edge facts that are already
// available before assignment extraction completes.
//
// This gives local inference access to canonical branch narrowing such as
// discriminant checks on parameters, without depending on later assignment-
// derived facts or full post-extraction solve. Every write in the graph is
// recorded without a type, so a branch fact about a value ends where the
// value is reassigned.
func buildPreflowFacts(fc *fbcore.FlowContext, inputs *flow.Inputs) *preflowFacts {
	if fc == nil || inputs == nil || inputs.Graph == nil || fc.TypeOps == nil {
		return nil
	}
	return &preflowFacts{
		graph:      fc.Graph,
		solution:   buildPreflowBranchSolution(fc, inputs),
		conditions: cond.ConditionsFunc(fc, inputs),
	}
}

// narrowTypeAssuming narrows t, the type of path read by an expression
// evaluated at p, by the branch facts reaching p conjoined with extra. An
// assignment at p evaluates its sources before it writes its targets, so a
// read of a target observes the facts on entry to p.
func (f *preflowFacts) narrowTypeAssuming(p cfg.Point, path constraint.Path, t typ.Type, extra constraint.Condition) typ.Type {
	if f == nil || f.solution == nil {
		return t
	}
	if f.writesAt(p, path.Symbol) {
		return f.solution.NarrowTypeBeforeAssuming(p, path, t, extra)
	}
	return f.solution.NarrowTypeAssuming(p, path, t, extra)
}

// writesAt reports whether the assignment at p writes the variable sym.
func (f *preflowFacts) writesAt(p cfg.Point, sym cfg.SymbolID) bool {
	if f.graph == nil || sym == 0 {
		return false
	}
	info := f.graph.Assign(p)
	if info == nil {
		return false
	}
	for _, target := range info.Targets {
		if target.Kind == cfg.TargetIdent && target.Symbol == sym {
			return true
		}
	}
	return false
}

// narrowedTypeAt returns the type of path at p the branch facts give.
func (f *preflowFacts) narrowedTypeAt(p cfg.Point, path constraint.Path) typ.Type {
	if f == nil || f.solution == nil {
		return nil
	}
	return f.solution.NarrowedTypeAt(p, path)
}

// operandCondition returns the condition under which the right operand of ex
// is evaluated: the left operand's truthy condition for `and`, its falsy
// condition for `or`.
func (f *preflowFacts) operandCondition(ex *ast.LogicalOpExpr, p cfg.Point) constraint.Condition {
	if f == nil || f.conditions == nil {
		return constraint.TrueCondition()
	}
	onTrue, onFalse := f.conditions(p, ex.Lhs)
	switch ex.Operator {
	case "and":
		return onTrue
	case "or":
		return onFalse
	}
	return constraint.TrueCondition()
}

func buildPreflowBranchSolution(fc *fbcore.FlowContext, inputs *flow.Inputs) *flow.Solution {

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
// operand is narrowed under the condition its left operand establishes, the
// condition a branch on it would carry.
func synthWithOverlayAndPreflow(
	overlay map[cfg.SymbolID]typ.Type,
	bindings *bind.BindingTable,
	inputs *flow.Inputs,
	callCtx *db.QueryContext,
	typeOps core.TypeOps,
	preflow *preflowFacts,
	base func(ast.Expr, cfg.Point) typ.Type,
) func(ast.Expr, cfg.Point) typ.Type {
	var synth func(ast.Expr, cfg.Point, constraint.Condition) typ.Type

	pathOf := func(expr ast.Expr, p cfg.Point) constraint.Path {
		if bindings == nil {
			return constraint.Path{}
		}
		return fbpath.FromExprWithBindings(expr, predicate.BuildConstResolver(inputs, p), bindings)
	}

	// read types an identifier or attribute read, before the operand
	// condition applies.
	read := func(expr ast.Expr, p cfg.Point, assumed constraint.Condition) typ.Type {
		if ident, ok := expr.(*ast.IdentExpr); ok && bindings != nil {
			if sym, ok := bindings.SymbolOf(ident); ok && sym != 0 {
				if t, exists := overlay[sym]; exists {
					return t
				}
			}
		}

		if bindings != nil && inputs != nil {
			if path := pathOf(expr, p); !path.IsEmpty() {
				if narrowed := preflow.narrowedTypeAt(p, path); !typ.IsAbsentOrUnknown(narrowed) {
					return narrowed
				}
			}
		}

		if attr, ok := expr.(*ast.AttrGetExpr); ok && typeOps != nil {
			objType := synth(attr.Object, p, assumed)
			if !typ.IsAbsentOrUnknown(objType) {
				switch key := attr.Key.(type) {
				case *ast.StringExpr:
					if ft, ok := typeOps.Field(callCtx, objType, key.Value); ok && !typ.IsAbsentOrUnknown(ft) {
						return ft
					}
					if it, ok := typeOps.Index(callCtx, objType, typ.LiteralString(key.Value)); ok && !typ.IsAbsentOrUnknown(it) {
						return it
					}
				default:
					keyType := synth(attr.Key, p, assumed)
					if !typ.IsAbsentOrUnknown(keyType) {
						if it, ok := typeOps.Index(callCtx, objType, keyType); ok && !typ.IsAbsentOrUnknown(it) {
							return it
						}
					}
				}
			}
		}

		if base == nil {
			return nil
		}
		return base(expr, p)
	}

	synth = func(expr ast.Expr, p cfg.Point, assumed constraint.Condition) typ.Type {
		if expr == nil {
			return nil
		}
		if ex, ok := expr.(*ast.LogicalOpExpr); ok && (ex.Operator == "and" || ex.Operator == "or") {
			left := synth(ex.Lhs, p, assumed)
			right := synth(ex.Rhs, p, constraint.And(assumed, preflow.operandCondition(ex, p)))
			if ex.Operator == "and" {
				return ops.LogicalAndTyped(left, right)
			}
			return ops.LogicalOrTyped(left, right)
		}
		t := read(expr, p, assumed)
		if t == nil {
			return t
		}
		switch expr.(type) {
		case *ast.IdentExpr, *ast.AttrGetExpr:
			if path := pathOf(expr, p); !path.IsEmpty() {
				return preflow.narrowTypeAssuming(p, path, t, assumed)
			}
		}
		return t
	}

	return func(expr ast.Expr, p cfg.Point) typ.Type {
		return synth(expr, p, constraint.TrueCondition())
	}
}

// narrowTableFieldsAtPoint narrows the fields of recType, the type of the table
// literal source, whose values read a path, by the branch facts reaching p:
// `{from = event.from}` under `if event.from then` has a present from.
func narrowTableFieldsAtPoint(recType typ.Type, source ast.Expr, p cfg.Point, bindings *bind.BindingTable, inputs *flow.Inputs, preflow *preflowFacts) typ.Type {
	tbl, ok := source.(*ast.TableExpr)
	if !ok || bindings == nil || preflow == nil {
		return recType
	}
	rec, ok := recType.(*typ.Record)
	if !ok || len(rec.Fields) == 0 {
		return recType
	}
	constResolver := predicate.BuildConstResolver(inputs, p)
	fieldPaths := make(map[string]constraint.Path)
	for _, field := range tbl.Fields {
		if field == nil || field.Key == nil {
			continue
		}
		name := ast.KeyName(field.Key)
		if name == "" {
			continue
		}
		if path := fbpath.FromExprWithBindings(field.Value, constResolver, bindings); !path.IsEmpty() {
			fieldPaths[name] = path
		}
	}
	if len(fieldPaths) == 0 {
		return recType
	}
	out := rec
	for _, f := range rec.Fields {
		path, ok := fieldPaths[f.Name]
		if !ok {
			continue
		}
		narrowed := preflow.narrowTypeAssuming(p, path, f.Type, constraint.TrueCondition())
		if narrowed == nil || typ.IsNever(narrowed) || typ.TypeEquals(narrowed, f.Type) {
			continue
		}
		f.Type = narrowed
		out = out.WithField(f)
	}
	return out
}

// tableValueFieldPaths records paths read by a table literal stored through a
// dynamic index. Its fields can be resolved again during the full flow solve,
// after call result types have replaced preflow placeholders.
func tableValueFieldPaths(source ast.Expr, p cfg.Point, bindings *bind.BindingTable, inputs *flow.Inputs) []flow.IndexerValueFieldPath {
	tbl, ok := source.(*ast.TableExpr)
	if !ok || bindings == nil || inputs == nil {
		return nil
	}
	constResolver := predicate.BuildConstResolver(inputs, p)
	var paths []flow.IndexerValueFieldPath
	for _, field := range tbl.Fields {
		if field == nil || field.Key == nil {
			continue
		}
		name := ast.KeyName(field.Key)
		if name == "" {
			continue
		}
		if valuePath := fbpath.FromExprWithBindings(field.Value, constResolver, bindings); !valuePath.IsEmpty() {
			paths = append(paths, flow.IndexerValueFieldPath{Name: name, Path: valuePath})
		}
	}
	return paths
}
