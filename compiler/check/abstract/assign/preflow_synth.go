package assign

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/abstract/cond"
	abstractcore "github.com/wippyai/go-lua/compiler/check/abstract/core"
	"github.com/wippyai/go-lua/compiler/check/abstract/predicate"
	fbpath "github.com/wippyai/go-lua/compiler/check/domain/path"
	"github.com/wippyai/go-lua/compiler/check/synth/callarg"
	"github.com/wippyai/go-lua/compiler/check/synth/ops"
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
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

type overlayTypeAt func(cfg.SymbolID, cfg.Point) (typ.Type, bool)

func mapOverlayTypeAt(overlay map[cfg.SymbolID]typ.Type) overlayTypeAt {
	return func(sym cfg.SymbolID, _ cfg.Point) (typ.Type, bool) {
		t, ok := overlay[sym]
		return t, ok
	}
}

// buildPreflowBranchSolution solves only branch/numeric edge facts that are
// already available before assignment extraction completes.
//
// This gives local inference access to canonical branch narrowing such as
// discriminant checks on parameters, without depending on later assignment-
// derived facts or full post-extraction solve.
func buildPreflowBranchSolution(fc *abstractcore.FlowContext, inputs *flow.Inputs) *flow.Solution {
	if fc == nil || inputs == nil || inputs.Graph == nil || fc.TypeOps == nil {
		return nil
	}

	temp := *inputs
	temp.EdgeConditions = nil
	temp.EdgeNumericConstraints = nil

	cond.ExtractEdgeConstraints(fc, &temp)
	cond.ExtractNumericConstraints(fc, &temp)

	return flow.Solve(&temp, narrowResolverAdapter{ctx: fc.CallCtx, ops: fc.TypeOps})
}

// synthWithOverlayAndPreflow wraps base synthesis with overlay lookup and a
// preflow branch-narrowing view for identifiers and attribute/index reads.
//
// This keeps assignment inference on the canonical synthesis path while letting
// recursive field/index expressions observe already-provable branch facts.
func synthWithOverlayAndPreflow(
	overlay overlayTypeAt,
	bindings *bind.BindingTable,
	inputs *flow.Inputs,
	callCtx *db.QueryContext,
	typeOps core.TypeOps,
	preflow *flow.Solution,
	base func(ast.Expr, cfg.Point) typ.Type,
) func(ast.Expr, cfg.Point) typ.Type {
	var synth func(ast.Expr, cfg.Point) typ.Type

	synth = func(expr ast.Expr, p cfg.Point) typ.Type {
		if expr == nil {
			return nil
		}

		if ident, ok := expr.(*ast.IdentExpr); ok && bindings != nil {
			if sym, ok := bindings.SymbolOf(ident); ok && sym != 0 {
				if overlay != nil {
					if t, exists := overlay(sym, p); exists {
						return t
					}
				}
			}
		}

		if preflow != nil && bindings != nil && inputs != nil {
			constResolver := predicate.BuildConstResolver(inputs, p)
			if path := fbpath.FromExprWithBindings(expr, constResolver, bindings); !path.IsEmpty() {
				if narrowed := preflow.NarrowedTypeAt(p, path); !typ.IsAbsentOrUnknown(narrowed) {
					if attr, ok := expr.(*ast.AttrGetExpr); ok && typeOps != nil {
						if objType := synth(attr.Object, p); !typ.IsAbsentOrUnknown(objType) {
							if refined := refinePreflowLengthIndex(attr, objType, narrowed, p, bindings, inputs, preflow); refined != nil {
								return refined
							}
						}
						if declared := declaredAttrReadType(attr, p, synth, callCtx, typeOps); declared != nil {
							refined, ok := refinePathFactWithDeclaredType(narrowed, declared, callCtx, typeOps)
							if !ok {
								goto skipPreflowPathFact
							}
							narrowed = refined
						}
					}
					return narrowed
				}
			}
		}

	skipPreflowPathFact:

		if attr, ok := expr.(*ast.AttrGetExpr); ok && typeOps != nil {
			objType := synth(attr.Object, p)
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
					keyType := synth(attr.Key, p)
					if !typ.IsAbsentOrUnknown(keyType) {
						if it, ok := typeOps.Index(callCtx, objType, keyType); ok && !typ.IsAbsentOrUnknown(it) {
							if refined := refinePreflowLengthIndex(attr, objType, it, p, bindings, inputs, preflow); refined != nil {
								return refined
							}
							return it
						}
					}
				}
			}
		}

		if call, ok := expr.(*ast.FuncCallExpr); ok && typeOps != nil {
			if result := evalOverlayCallFirstResult(call, p, synth, callCtx, typeOps); !typ.IsAbsentOrUnknown(result) {
				return result
			}
			if base != nil {
				if direct := base(expr, p); !typ.IsAbsentOrUnknown(direct) {
					return direct
				}
			}
			return typ.Unknown
		}

		if logical, ok := expr.(*ast.LogicalOpExpr); ok {
			left := synth(logical.Lhs, p)
			right := synth(logical.Rhs, p)
			var result typ.Type
			switch logical.Operator {
			case "and":
				result = ops.LogicalAndTyped(left, right)
			case "or":
				result = ops.LogicalOrTyped(left, right)
			default:
				result = typ.Unknown
			}
			if (typ.IsAbsentOrUnknown(result) || typ.IsAny(result)) && base != nil {
				if direct := base(expr, p); !typ.IsAbsentOrUnknown(direct) && !typ.IsAny(direct) {
					return direct
				}
			}
			return result
		}

		if base == nil {
			return nil
		}
		return base(expr, p)
	}

	return synth
}

func refinePreflowLengthIndex(attr *ast.AttrGetExpr, objType, indexResult typ.Type, p cfg.Point, bindings *bind.BindingTable, inputs *flow.Inputs, preflow *flow.Solution) typ.Type {
	if attr == nil || bindings == nil || inputs == nil || preflow == nil {
		return nil
	}
	constResolver := predicate.BuildConstResolver(inputs, p)
	tablePath := fbpath.FromExprWithBindings(attr.Object, constResolver, bindings)
	if tablePath.IsEmpty() {
		return nil
	}
	lenPath, offset, ok := lengthIndexPathFromExpr(attr.Key, constResolver, bindings)
	if !ok || !lenPath.Equal(tablePath) {
		return nil
	}
	lower, _, ok := preflow.LengthBoundsAt(p, tablePath)
	if !ok {
		return nil
	}
	return narrow.RefineLengthIndex(objType, indexResult, lower, offset)
}

// evalOverlayCallFirstResult evaluates a call expression inside assignment
// transfer using the shared call domain. It is a local value evaluator only:
// facts and diagnostics are still published by the canonical call/evidence
// consumers after the abstract state is solved.
func evalOverlayCallFirstResult(
	call *ast.FuncCallExpr,
	p cfg.Point,
	synth func(ast.Expr, cfg.Point) typ.Type,
	callCtx *db.QueryContext,
	typeOps core.TypeOps,
) typ.Type {
	if call == nil || synth == nil || typeOps == nil {
		return nil
	}
	args := make([]typ.Type, len(call.Args))
	for i, arg := range call.Args {
		args[i] = synth(arg, p)
	}
	def := ops.CallDef{
		Args:  args,
		Query: typeOps,
	}
	if call.Method != "" {
		def.IsMethod = true
		def.MethodName = call.Method
		def.Receiver = synth(call.Receiver, p)
	} else {
		def.Callee = synth(call.Func, p)
	}
	result := ops.NewCallPipeline(callCtx, def, len(call.Args)).
		WithReSynth(assignmentCallArgReSynth(call.Args, synth, p)).
		Run()
	if len(result.Returns) > 0 {
		return result.Returns[0]
	}
	return ops.ExtractFirstValue(result.Type)
}

func assignmentCallArgReSynth(args []ast.Expr, synth func(ast.Expr, cfg.Point) typ.Type, p cfg.Point) ops.ArgReSynth {
	if synth == nil {
		return nil
	}
	return callarg.ForArgs(args, callarg.Full(
		func(arg ast.Expr, _ cfg.Point, _ typ.Type) typ.Type {
			return synth(arg, p)
		},
		nil,
		p,
	))
}

func declaredAttrReadType(
	attr *ast.AttrGetExpr,
	p cfg.Point,
	synth func(ast.Expr, cfg.Point) typ.Type,
	callCtx *db.QueryContext,
	typeOps core.TypeOps,
) typ.Type {
	if attr == nil || synth == nil || typeOps == nil {
		return nil
	}
	objType := synth(attr.Object, p)
	if typ.IsAbsentOrUnknown(objType) {
		return nil
	}
	switch key := attr.Key.(type) {
	case *ast.StringExpr:
		if ft, ok := typeOps.Field(callCtx, objType, key.Value); ok {
			return ft
		}
		if it, ok := typeOps.Index(callCtx, objType, typ.LiteralString(key.Value)); ok {
			return it
		}
	default:
		keyType := synth(attr.Key, p)
		if !typ.IsAbsentOrUnknown(keyType) {
			if it, ok := typeOps.Index(callCtx, objType, keyType); ok {
				return it
			}
		}
	}
	return nil
}

func refinePathFactWithDeclaredType(narrowed, declared typ.Type, callCtx *db.QueryContext, typeOps core.TypeOps) (typ.Type, bool) {
	if narrowed == nil || declared == nil {
		return narrowed, true
	}
	narrowed = unwrap.Alias(narrowed)
	declared = unwrap.Alias(declared)
	if narrowed == nil || declared == nil || declared.Kind().IsPlaceholder() {
		return narrowed, true
	}
	if typeOps == nil {
		return nil, false
	}
	if typeOps.IsSubtype(callCtx, narrowed, declared) {
		return narrowed, true
	}
	declaredNonNil := narrow.RemoveNil(declared)
	if !typ.IsNever(declaredNonNil) {
		if typeOps.IsSubtype(callCtx, declaredNonNil, narrowed) {
			return declaredNonNil, true
		}
		if unwrap.Function(declaredNonNil) != nil && unwrap.Function(narrowed) != nil {
			return declaredNonNil, true
		}
	}
	return nil, false
}
