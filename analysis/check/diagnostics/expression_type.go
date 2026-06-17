package diagnostics

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/diagnostics/internal/readmodel"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/variant"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/typeannotation"
	"github.com/wippyai/go-lua/analysis/lua/typeoperator"
	luatypeprojection "github.com/wippyai/go-lua/analysis/lua/typeprojection"
	"github.com/wippyai/go-lua/analysis/lua/valueexpr"
	"github.com/wippyai/go-lua/analysis/type/access"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

type expressionTyper struct {
	result        *body.Result
	resolver      typeannotation.Resolver
	point         cfg.Point
	env           guardEnv
	flow          bool
	witnessRefine bool
}

func newExpressionTyper(result *body.Result, resolver typeannotation.Resolver) expressionTyper {
	return expressionTyper{result: result, resolver: resolver}
}

func newFlowExpressionTyper(result *body.Result, resolver typeannotation.Resolver, point cfg.Point, env guardEnv) expressionTyper {
	return expressionTyper{result: result, resolver: resolver, point: point, env: env, flow: true, witnessRefine: true}
}

// newStructuralFlowExpressionTyper types expressions under active flow narrowing
// but suppresses structural witness refinement, so the result reflects the
// declared/origin type narrowed only by sound discriminant narrowing. This keeps
// absence proofs from trusting a partial observed table-literal snapshot that may
// omit declared fields.
func newStructuralFlowExpressionTyper(result *body.Result, resolver typeannotation.Resolver, point cfg.Point, env guardEnv) expressionTyper {
	return expressionTyper{result: result, resolver: resolver, point: point, env: env, flow: true, witnessRefine: false}
}

func (p expressionTyper) typeOf(expr ast.Expr) (typ.Type, bool) {
	return p.typeOfDepth(expr, 0)
}

func (p expressionTyper) typeOfDepth(expr ast.Expr, depth int) (typ.Type, bool) {
	if expr == nil || depth > typ.DefaultRecursionDepth {
		return nil, false
	}
	if t, ok := valueexpr.LiteralType(expr); ok {
		return t, true
	}
	switch e := expr.(type) {
	case *ast.CastExpr:
		return p.typeOfDepth(e.Expr, depth+1)
	case *ast.NonNilAssertExpr:
		t, ok := p.typeOfDepth(e.Expr, depth+1)
		if !ok {
			return nil, false
		}
		return projectionWithoutNil(t), true
	case *ast.IdentExpr:
		return p.annotatedPathType(e)
	case *ast.FuncCallExpr:
		return p.callResultType(e)
	case *ast.AttrGetExpr:
		return p.attrType(e, depth+1)
	case *ast.LogicalOpExpr:
		return p.binaryType(e.Lhs, e.Operator, e.Rhs, depth+1)
	case *ast.RelationalOpExpr:
		return p.binaryType(e.Lhs, e.Operator, e.Rhs, depth+1)
	case *ast.StringConcatOpExpr:
		return p.binaryType(e.Lhs, "..", e.Rhs, depth+1)
	case *ast.ArithmeticOpExpr:
		return p.binaryType(e.Lhs, e.Operator, e.Rhs, depth+1)
	case *ast.UnaryMinusOpExpr:
		return p.unaryType("-", e.Expr, depth+1)
	case *ast.UnaryNotOpExpr:
		return p.unaryType("not", e.Expr, depth+1)
	case *ast.UnaryLenOpExpr:
		return p.unaryType("#", e.Expr, depth+1)
	case *ast.UnaryBNotOpExpr:
		return p.unaryType("~", e.Expr, depth+1)
	default:
		return nil, false
	}
}

func (p expressionTyper) annotatedPathType(expr ast.Expr) (typ.Type, bool) {
	accessPath, ok := p.result.ExpressionPath(expr)
	if !ok || accessPath.Symbol == 0 {
		return nil, false
	}
	annotation, ok := p.result.SymbolTypeAnnotation(accessPath.Symbol)
	var t typ.Type
	if ok {
		lowered, loweredOK := lowerType(annotation, p.resolver)
		if !loweredOK {
			return nil, false
		}
		t = transparentComparableType(p.result, lowered)
	} else if p.flow {
		lowered, loweredOK := p.flowOriginType(accessPath)
		if !loweredOK {
			return nil, false
		}
		t = lowered
	} else {
		return nil, false
	}
	if p.flow {
		t = p.flowRootType(t, accessPath)
	}
	for _, seg := range accessPath.Segments {
		next, ok := expressionSegmentType(t, seg)
		if !ok {
			return nil, false
		}
		t = next
	}
	return p.refineFlowExpressionType(expr, t), true
}

// broadType returns the un-narrowed declared shape of a path expression: the
// lowered root annotation when present, otherwise the full variant-origin
// family, projected through the path suffix. It reflects what the receiver
// could be before any flow narrowing, so callers can tell discriminant collapse
// apart from a single-shape observed snapshot.
func (p expressionTyper) broadType(expr ast.Expr) (typ.Type, bool) {
	accessPath, ok := p.result.ExpressionPath(expr)
	if !ok || accessPath.Symbol == 0 {
		return nil, false
	}
	var t typ.Type
	if annotation, ok := p.result.SymbolTypeAnnotation(accessPath.Symbol); ok {
		lowered, loweredOK := lowerType(annotation, p.resolver)
		if !loweredOK {
			return nil, false
		}
		t = transparentComparableType(p.result, lowered)
	} else {
		value, ok := p.result.SymbolValueAtBoundary(p.point, accessPath.Symbol)
		if !ok {
			return nil, false
		}
		full, ok := readmodel.New(p.result).FullVariantOriginType(value)
		if !ok {
			return nil, false
		}
		t = full
	}
	for _, seg := range accessPath.Segments {
		next, ok := expressionSegmentType(t, seg)
		if !ok {
			return nil, false
		}
		t = next
	}
	return t, t != nil
}

func (p expressionTyper) flowOriginType(accessPath pathdom.Path) (typ.Type, bool) {
	if p.result == nil || accessPath.Symbol == 0 {
		return nil, false
	}
	value, ok := p.result.SymbolValueAtBoundary(p.point, accessPath.Symbol)
	if !ok {
		return nil, false
	}
	return readmodel.New(p.result).VariantOriginType(value)
}

func (p expressionTyper) flowRootType(t typ.Type, accessPath pathdom.Path) typ.Type {
	root := rootPath(accessPath)
	if root.Symbol == 0 {
		return t
	}
	if p.witnessRefine {
		if value, ok := p.result.SymbolValueAtBoundary(p.point, root.Symbol); ok {
			if refined, ok := readmodel.New(p.result).RefineDeclaredType(t, value); ok {
				t = refined
			}
		}
	} else if value, ok := p.result.SymbolValueAtBoundary(p.point, root.Symbol); ok {
		if refined, ok := readmodel.New(p.result).NarrowDeclaredByOrigin(t, value); ok {
			t = refined
		}
	}
	if narrowed, ok := applyLiteralPathNarrowing(t, root, p.env); ok {
		t = narrowed
	}
	return t
}

func applyLiteralPathNarrowing(base typ.Type, receiver pathdom.Path, env guardEnv) (typ.Type, bool) {
	if base == nil || len(env.constraints) == 0 {
		return base, false
	}
	out := base
	changed := false
	for _, c := range env.constraints {
		suffix, ok := suffixFromReceiver(receiver, c.target)
		if !ok {
			continue
		}
		lit := typ.LiteralString(c.value)
		if c.negated {
			if narrowed, ok := variant.NarrowByPathLiteralNot(out, suffix, lit); ok {
				out = narrowed
				changed = true
			}
		} else {
			if narrowed, ok := variant.NarrowByPathLiteral(out, suffix, lit); ok {
				out = narrowed
				changed = true
			}
		}
	}
	return out, changed
}

func rootPath(p pathdom.Path) pathdom.Path {
	p.Segments = nil
	return p
}

func expressionSegmentType(t typ.Type, seg segment.Segment) (typ.Type, bool) {
	switch seg.Kind {
	case segment.SegmentField:
		if field, ok := access.Field(t, seg.Name); ok {
			return field, true
		}
		if access.MissingFieldReadsNil(t) {
			return typ.Nil, true
		}
		return nil, false
	case segment.SegmentIndexString, segment.SegmentIndexInt:
		key, ok := luatypeprojection.SegmentKeyType(seg)
		if !ok {
			return nil, false
		}
		return access.RuntimeIndex(t, key)
	default:
		return nil, false
	}
}

// callResultType types the first result of a call expression from its solved
// call-result slot, preserving boundary presence so an optional result reads as
// optional. It lets index/field projections over a call result (make()[1]) be
// typed without a symbol path.
func (p expressionTyper) callResultType(call *ast.FuncCallExpr) (typ.Type, bool) {
	if p.result == nil || call == nil {
		return nil, false
	}
	value, ok := p.result.CallExprResultValue(call, 0)
	if !ok {
		return nil, false
	}
	return readmodel.New(p.result).ValueTypeWithPresence(value)
}

func (p expressionTyper) attrType(expr *ast.AttrGetExpr, depth int) (typ.Type, bool) {
	if expr == nil {
		return nil, false
	}
	container, ok := p.typeOfDepth(expr.Object, depth+1)
	if !ok {
		return nil, false
	}
	if expr.KeySyntax == ast.AttrKeyDot {
		name := ast.KeyName(expr.Key)
		if name == "" {
			return nil, false
		}
		t, ok := expressionSegmentType(container, segment.Segment{Kind: segment.SegmentField, Name: name})
		if !ok {
			return nil, false
		}
		return p.refineFlowExpressionType(expr, t), true
	}
	key, ok := p.typeOfDepth(expr.Key, depth+1)
	if !ok {
		return nil, false
	}
	t, ok := access.RuntimeIndex(container, key)
	if !ok {
		return nil, false
	}
	return p.refineFlowExpressionType(expr, t), true
}

func (p expressionTyper) refineFlowExpressionType(expr ast.Expr, t typ.Type) typ.Type {
	if !p.flow || !p.witnessRefine || p.result == nil || expr == nil || t == nil {
		return t
	}
	if value, ok := p.result.ExpressionValueAtBoundary(p.point, expr); ok {
		if refined, ok := readmodel.New(p.result).RefineDeclaredType(t, value); ok {
			t = refined
		}
	}
	if accessPath, ok := p.result.ExpressionPath(expr); ok && p.env.hasPresent(accessPath) {
		if withoutNil := projectionWithoutNil(t); withoutNil != nil && !typ.IsNever(withoutNil) {
			return withoutNil
		}
	}
	return t
}

func (p expressionTyper) unaryType(op string, expr ast.Expr, depth int) (typ.Type, bool) {
	operand, ok := p.typeOfDepth(expr, depth+1)
	if !ok {
		return nil, false
	}
	return typeoperator.UnaryOp(op, operand)
}

func (p expressionTyper) binaryType(lhs ast.Expr, op string, rhs ast.Expr, depth int) (typ.Type, bool) {
	left, ok := p.typeOfDepth(lhs, depth+1)
	if !ok {
		return nil, false
	}
	right, ok := p.typeOfDepth(rhs, depth+1)
	if !ok {
		return nil, false
	}
	return typeoperator.BinaryOp(left, op, right)
}

func projectionHasNil(t typ.Type) bool {
	return readmodel.ProjectionHasNil(t)
}

func projectionWithoutNil(t typ.Type) typ.Type {
	return typetable.PresentReadonlyEntryValue(t)
}
