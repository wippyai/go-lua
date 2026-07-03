package diagnostics

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/assertion"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/escape"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/typewitness"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/castsem"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	"github.com/wippyai/go-lua/analysis/lua/typeannotation"
	"github.com/wippyai/go-lua/analysis/lua/valueexpr"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/type/subtype"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
	"github.com/wippyai/go-lua/compiler/ast"
)

type boundaryValueReader func(*body.Result, cfg.Point) (product.Value, bool)

func boundaryTypeMismatch(result *body.Result, point cfg.Point, got, want typ.Type, read boundaryValueReader) bool {
	if topLikeTarget(want) {
		return false
	}
	if read != nil {
		if value, ok := read(result, point); ok {
			query := newDiagnosticQuery(result)
			if query.ValueAdmissible(value, want) || freshValueAdmissible(result, value, want) {
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
	if topLikeTarget(want) {
		return false
	}
	if read != nil {
		if value, ok := read(result, point); ok {
			query := newDiagnosticQuery(result)
			if query.ValueProofAdmissible(value, want) {
				return false
			}
			if query.ValueHasUntrustedTopOrigin(value) {
				return true
			}
		}
	}
	if !topLikeType(got) {
		return clearMismatch(result, got, want)
	}
	return true
}

func topLikeTarget(want typ.Type) bool {
	if want == nil {
		return true
	}
	want = unwrap.Alias(want)
	if typ.IsAny(want) || typ.IsUnknown(want) {
		return true
	}
	opt, ok := want.(*typ.Optional)
	if !ok || opt == nil {
		return false
	}
	inner := unwrap.Alias(opt.Inner)
	return typ.IsAny(inner) || typ.IsUnknown(inner)
}

func boundaryValueHasUntrustedTopOrigin(result *body.Result, point cfg.Point, read boundaryValueReader) bool {
	if result == nil || read == nil {
		return false
	}
	value, ok := read(result, point)
	if !ok {
		return false
	}
	return newDiagnosticQuery(result).ValueHasUntrustedTopOrigin(value)
}

func boundaryValueNeedsValidationProof(result *body.Result, point cfg.Point, read boundaryValueReader, want typ.Type) bool {
	if result == nil || result.Registry() == nil || read == nil || want == nil {
		return false
	}
	value, ok := read(result, point)
	if !ok {
		return false
	}
	query := newDiagnosticQuery(result)
	if query.ValueProofAdmissible(value, want) {
		return false
	}
	if query.ValueHasUntrustedTopOrigin(value) {
		return true
	}
	claims := product.Get(result.Registry(), value, assertion.Key)
	return claims.Has(assertion.AnyClaim)
}

func expressionValueHasUntrustedTopOrigin(result *body.Result, point cfg.Point, expr ast.Expr) bool {
	if result == nil || expr == nil {
		return false
	}
	query := newDiagnosticQuery(result)
	value, ok := query.ExpressionValueAtBoundary(point, expr)
	if !ok {
		return false
	}
	return query.ValueHasUntrustedTopOrigin(value)
}

func staticExpressionType(result *body.Result, resolver typeannotation.Resolver, expr ast.Expr) (typ.Type, bool) {
	if t, ok := valueexpr.LiteralType(expr); ok {
		return t, true
	}
	if t, ok := immutableLocalLiteralType(result, expr); ok {
		return t, true
	}
	return newExpressionTyper(result, resolver).typeOf(expr)
}

func immutableLocalLiteralType(result *body.Result, expr ast.Expr) (typ.Type, bool) {
	ident, ok := expr.(*ast.IdentExpr)
	if !ok || result == nil || ident == nil {
		return nil, false
	}
	p, ok := result.ExpressionPath(ident)
	if !ok || p.Symbol == 0 || len(p.Segments) != 0 {
		return nil, false
	}
	kind, ok := result.SymbolKind(p.Symbol)
	if !ok || kind != symbol.Local {
		return nil, false
	}
	if result.SymbolHasWrite(p.Symbol) {
		return nil, false
	}
	origin, ok := result.LocalOrigin(p.Symbol)
	if !ok || origin.Stmt == nil || origin.Index < 0 || origin.Index >= len(origin.Stmt.Exprs) {
		return nil, false
	}
	return valueexpr.LiteralType(origin.Stmt.Exprs[origin.Index])
}

// concreteCastObligationType reports the type to check against an imposed
// obligation when the operand is a direct cast to a concrete type. Concrete
// non-top casts are executable runtime validations in this dialect, so their
// target type is the checked result on the normal path.
func concreteCastObligationType(result *body.Result, resolver typeannotation.Resolver, point cfg.Point, env guardEnv, expr ast.Expr) (typ.Type, bool) {
	return concreteRuntimeCastTarget(resolver, expr)
}

func concreteRuntimeCastTarget(resolver typeannotation.Resolver, expr ast.Expr) (typ.Type, bool) {
	cast, ok := expr.(*ast.CastExpr)
	if !ok || cast == nil || cast.Type == nil || topLikeCastTargetExpr(cast) {
		return nil, false
	}
	target, ok := lowerType(cast.Type, resolver)
	if !ok || target == nil {
		return nil, false
	}
	return target, true
}

func topLikeCastTargetExpr(cast *ast.CastExpr) bool {
	primitive, ok := cast.Type.(*ast.PrimitiveTypeExpr)
	return ok && primitive != nil && castsem.IsTopLikeTarget(primitive.Name)
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
	if primitive, ok := cast.Type.(*ast.PrimitiveTypeExpr); ok && primitive != nil {
		switch {
		case castsem.IsAnyTarget(primitive.Name):
			return typ.Any, true
		case castsem.IsUnknownTarget(primitive.Name):
			return typ.Unknown, true
		}
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
	subject := exprEvidenceName(expr)
	out := diagnostic.AssertionEvidence(span, assertion.Any())
	out = append(out, diagnostic.Evidence{
		Kind:    diagnostic.EvidencePrecisionBoundary,
		Trust:   diagnostic.TrustUnknown,
		Reason:  diagnostic.EvidenceReasonExplicitBoundaryValidation,
		Span:    span,
		Message: explicitBoundaryProofMessageForSubject(subject, want),
	})
	out = append(out, diagnostic.Evidence{
		Kind:    diagnostic.EvidenceMissingProof,
		Trust:   diagnostic.TrustUnknown,
		Reason:  diagnostic.EvidenceReasonBoundaryValidationMissing,
		Span:    span,
		Message: missingBoundaryProofMessageForSubject(subject, want),
	})
	return out
}

func untrustedAnyBoundaryReader(read boundaryValueReader) boundaryValueReader {
	if read == nil {
		return nil
	}
	return func(result *body.Result, point cfg.Point) (product.Value, bool) {
		value, ok := read(result, point)
		if !ok || result == nil || result.Registry() == nil {
			return value, ok
		}
		reg := result.Registry()
		claims := product.Get(reg, value, assertion.Key)
		if claims.Has(assertion.TypeClaim) {
			return value, true
		}
		value = product.Set(reg, value, evidence.Key, evidence.ExplicitTop())
		value = product.Set(reg, value, assertion.Key, assertionWithAnyClaim(claims))
		return value, true
	}
}

func assertionWithAnyClaim(claims assertion.Value) assertion.Value {
	flags := claims.Flags()
	flags = append(flags, assertion.AnyClaim)
	return assertion.Of(flags...)
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
	query := newDiagnosticQuery(result)
	if value, ok := query.ExpressionValueAtBoundary(point, expr); ok {
		if query.ValueHasUntrustedTopOrigin(value) {
			if got, typeOK := query.ValueType(value); typeOK && !topLikeType(got) && query.ValueProofAdmissible(value, got) {
				return nil, false
			}
			return typ.Any, true
		}
		if got, typeOK := query.ValueType(value); typeOK && !topLikeType(got) {
			return nil, false
		}
	}
	if call, ok := expr.(*ast.FuncCallExpr); ok {
		if value, ok := query.CallExprResultValue(call, 0); ok && query.ValueHasUntrustedTopOrigin(value) {
			return typ.Any, true
		}
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
	site, _ := result.CallSite(source.CallPoint)
	if fact.Receiver != nil {
		if t, ok := explicitTopLikeExpressionType(result, resolver, fact.Receiver); ok {
			return t, true
		}
	}
	if receiver, ok := site.ReceiverPath(); ok {
		if t, ok := explicitTopLikePathRootType(result, resolver, receiver.Symbol); ok {
			return t, true
		}
	}
	if fact.Func != nil {
		if t, ok := explicitTopLikeExpressionType(result, resolver, fact.Func); ok {
			return t, true
		}
	}
	if callee := site.CalleePathRef(); !callee.IsEmpty() {
		if t, ok := explicitTopLikePathRootType(result, resolver, callee.Symbol); ok {
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
		return newDiagnosticQuery(result).ExpressionValueAtBoundary(point, expr)
	}
}

func boundaryValueFromASTSource(source sourceprovenance.ASTSource) boundaryValueReader {
	return func(result *body.Result, point cfg.Point) (product.Value, bool) {
		return newDiagnosticQuery(result).SourceValue(point, source)
	}
}

func boundaryValueFromValueSource(source factflow.ValueSource) boundaryValueReader {
	return func(result *body.Result, point cfg.Point) (product.Value, bool) {
		if result == nil {
			return product.Value{}, false
		}
		return newDiagnosticQuery(result).ValueSourceForExplanationAtBoundary(point, source)
	}
}

func topLikeType(t typ.Type) bool {
	return t == nil || typ.IsAny(t) || typ.IsUnknown(t)
}
