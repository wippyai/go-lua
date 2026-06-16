package diagnostics

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/diagnostics/internal/readmodel"
	"github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/assertion"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/escape"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/typewitness"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	"github.com/wippyai/go-lua/analysis/lua/typeannotation"
	"github.com/wippyai/go-lua/analysis/lua/valueexpr"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/type/subtype"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

type boundaryValueReader func(*body.Result, cfg.Point) (product.Value, bool)

func boundaryTypeMismatch(result *body.Result, point cfg.Point, got, want typ.Type, read boundaryValueReader) bool {
	if want == nil || typ.IsAny(want) || typ.IsUnknown(want) {
		return false
	}
	if read != nil {
		if value, ok := read(result, point); ok {
			reader := readmodel.New(result)
			if reader.ValueAdmissible(value, want) || freshValueAdmissible(result, value, want) {
				return false
			}
		}
	}
	if !topLikeType(got) {
		return clearMismatch(result, got, want)
	}
	return true
}

func freshValueAdmissible(result *body.Result, value product.Value, want typ.Type) bool {
	if result == nil || result.Registry() == nil || want == nil {
		return false
	}
	reg := result.Registry()
	if !presence.Equal(product.PresenceOf(value), presence.Present()) {
		return false
	}
	if gotEscape := product.Get(reg, value, escape.Key); !escape.Equal(gotEscape, escape.Fresh()) {
		return false
	}
	witness := product.Get(reg, value, typewitness.Key)
	if witness.IsTop() {
		return false
	}
	got, ok := witness.Type()
	return ok && subtype.IsFreshAssignable(got, want)
}

func boundaryProofTypeMismatch(result *body.Result, point cfg.Point, got, want typ.Type, read boundaryValueReader) bool {
	if want == nil || typ.IsAny(want) || typ.IsUnknown(want) {
		return false
	}
	if read != nil {
		if value, ok := read(result, point); ok && readmodel.New(result).ValueProofAdmissible(value, want) {
			return false
		}
	}
	if !topLikeType(got) {
		return clearMismatch(result, got, want)
	}
	return true
}

func boundaryExprType(result *body.Result, resolver typeannotation.Resolver, expr ast.Expr) (typ.Type, bool) {
	if t, ok := valueexpr.LiteralType(expr); ok {
		return t, true
	}
	return newExpressionTyper(result, resolver).typeOf(expr)
}

func explicitTopLikeExpressionType(result *body.Result, resolver typeannotation.Resolver, expr ast.Expr) (typ.Type, bool) {
	t, ok := newExpressionTyper(result, resolver).typeOf(expr)
	if !ok || !topLikeType(t) {
		return nil, false
	}
	return t, true
}

func explicitTopLikeCastType(resolver typeannotation.Resolver, expr ast.Expr) (typ.Type, bool) {
	cast, ok := expr.(*ast.CastExpr)
	if !ok || cast == nil || cast.Type == nil {
		return nil, false
	}
	t, ok := lowerType(cast.Type, resolver)
	if !ok || !topLikeType(t) {
		return nil, false
	}
	return t, true
}

func explicitTopLikeCastEvidence(span diagnostic.Span, want typ.Type, expr ast.Expr) []diagnostic.Evidence {
	if _, ok := expr.(*ast.CastExpr); !ok {
		return nil
	}
	out := diagnostic.AssertionEvidence(span, assertion.Any())
	out = append(out, diagnostic.Evidence{
		Kind:    diagnostic.EvidencePrecisionBoundary,
		Trust:   diagnostic.TrustUnknown,
		Span:    span,
		Message: "explicit any/unknown boundary has no structural proof for " + formatType(want),
	})
	out = append(out, diagnostic.Evidence{
		Kind:    diagnostic.EvidenceMissingProof,
		Trust:   diagnostic.TrustUnknown,
		Span:    span,
		Message: "no boundary proof establishes " + formatType(want),
	})
	return out
}

func untrustedTopLikeExpressionType(result *body.Result, resolver typeannotation.Resolver, expr ast.Expr) (typ.Type, bool) {
	if result == nil || expr == nil {
		return nil, false
	}
	accessPath, ok := result.ExpressionPath(expr)
	if !ok || accessPath.Symbol == 0 {
		return nil, false
	}
	root, ok := explicitTopLikePathRootType(result, resolver, accessPath.Symbol)
	if !ok {
		return nil, false
	}
	t, ok := newExpressionTyper(result, resolver).typeOf(expr)
	if ok && topLikeType(t) {
		return t, true
	}
	return root, true
}

func untrustedTopLikeExpressionTypeAt(result *body.Result, resolver typeannotation.Resolver, point cfg.Point, expr ast.Expr) (typ.Type, bool) {
	if result == nil || expr == nil {
		return nil, false
	}
	if value, ok := result.ExpressionValueAtBoundary(point, expr); ok && readmodel.New(result).ValueHasUntrustedTopOrigin(value) {
		return typ.Any, true
	}
	return untrustedTopLikeExpressionType(result, resolver, expr)
}

func explicitTopLikeCallSourceType(result *body.Result, resolver typeannotation.Resolver, expr ast.Expr) (typ.Type, bool) {
	call, ok := expr.(*ast.FuncCallExpr)
	if !ok || call == nil {
		return nil, false
	}
	if call.Method != "" && call.Receiver != nil {
		if t, ok := explicitTopLikeExpressionType(result, resolver, call.Receiver); ok {
			return t, true
		}
	}
	if call.Func != nil {
		if t, ok := explicitTopLikeExpressionType(result, resolver, call.Func); ok {
			return t, true
		}
	}
	return nil, false
}

func explicitTopLikeCallFactSourceType(result *body.Result, resolver typeannotation.Resolver, source sourceprovenance.ASTSource) (typ.Type, bool) {
	if result == nil || source.Kind != sourceprovenance.SourceCall || !source.HasCallPoint {
		return nil, false
	}
	fact, ok := result.Call(source.CallPoint)
	if !ok {
		return nil, false
	}
	if fact.Receiver != nil {
		if t, ok := explicitTopLikeExpressionType(result, resolver, fact.Receiver); ok {
			return t, true
		}
	}
	if fact.HasReceiverPath {
		if t, ok := explicitTopLikePathRootType(result, resolver, fact.ReceiverPath.Symbol); ok {
			return t, true
		}
	}
	if fact.Func != nil {
		if t, ok := explicitTopLikeExpressionType(result, resolver, fact.Func); ok {
			return t, true
		}
	}
	if fact.HasCalleePath {
		if t, ok := explicitTopLikePathRootType(result, resolver, fact.CalleePath.Symbol); ok {
			return t, true
		}
	}
	if fact.Call != nil {
		if t, ok := explicitTopLikeCallSourceType(result, resolver, fact.Call); ok {
			return t, true
		}
	}
	return nil, false
}

func explicitTopLikePathRootType(result *body.Result, resolver typeannotation.Resolver, id symbol.ID) (typ.Type, bool) {
	if result == nil || id == 0 {
		return nil, false
	}
	expr, ok := result.SymbolTypeAnnotation(id)
	if !ok {
		return nil, false
	}
	t, ok := lowerType(expr, resolver)
	if !ok || !topLikeType(t) {
		return nil, false
	}
	return t, true
}

func boundaryValueFromExpr(expr ast.Expr) boundaryValueReader {
	return func(result *body.Result, point cfg.Point) (product.Value, bool) {
		if result == nil || expr == nil {
			return product.Value{}, false
		}
		return result.ExpressionValueAtBoundary(point, expr)
	}
}

func boundaryValueFromASTSource(source sourceprovenance.ASTSource) boundaryValueReader {
	return func(result *body.Result, point cfg.Point) (product.Value, bool) {
		return readmodel.New(result).SourceValue(point, source)
	}
}

func topLikeType(t typ.Type) bool {
	return t == nil || typ.IsAny(t) || typ.IsUnknown(t)
}
