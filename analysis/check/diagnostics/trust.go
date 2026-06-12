package diagnostics

import (
	"github.com/wippyai/go-lua/analysis/check"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/typewitness"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/variantorigin"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	"github.com/wippyai/go-lua/analysis/lua/typeannotation"
	"github.com/wippyai/go-lua/analysis/lua/valueexpr"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/type/discriminant"
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/subtype"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

type boundaryValueReader func(*check.Result, cfg.Point) (product.Value, bool)

func boundaryTypeMismatch(result *check.Result, point cfg.Point, got, want typ.Type, read boundaryValueReader) bool {
	if !topLikeType(got) {
		return clearMismatch(got, want)
	}
	if want == nil || typ.IsAny(want) || typ.IsUnknown(want) {
		return false
	}
	if read != nil {
		if value, ok := read(result, point); ok && boundaryValueAdmissible(result, value, want) {
			return false
		}
	}
	return true
}

func boundarySourceType(result *check.Result, point cfg.Point, source sourceprovenance.ASTSource) (typ.Type, bool) {
	value, ok := boundaryValueFromSource(result, point, source)
	if !ok {
		return nil, false
	}
	if t, ok := concreteBoundaryType(result, value); ok {
		return t, true
	}
	return nil, false
}

func boundaryExprType(result *check.Result, resolver typeannotation.Resolver, expr ast.Expr) (typ.Type, bool) {
	if t, ok := valueexpr.LiteralType(expr); ok {
		return t, true
	}
	return newExpressionTyper(result, resolver).typeOf(expr)
}

func explicitTopLikeExpressionType(result *check.Result, resolver typeannotation.Resolver, expr ast.Expr) (typ.Type, bool) {
	t, ok := newExpressionTyper(result, resolver).typeOf(expr)
	if !ok || !topLikeType(t) {
		return nil, false
	}
	return t, true
}

func explicitTopLikeCallSourceType(result *check.Result, resolver typeannotation.Resolver, expr ast.Expr) (typ.Type, bool) {
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

func explicitTopLikeCallFactSourceType(result *check.Result, resolver typeannotation.Resolver, source sourceprovenance.ASTSource) (typ.Type, bool) {
	if result == nil || source.Kind != factflow.ValueSourceCall || !source.HasCallPoint {
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

func explicitTopLikePathRootType(result *check.Result, resolver typeannotation.Resolver, id symbol.ID) (typ.Type, bool) {
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
	return func(result *check.Result, point cfg.Point) (product.Value, bool) {
		if result == nil || expr == nil {
			return product.Value{}, false
		}
		return result.ExpressionValueAtBoundary(point, expr)
	}
}

func boundaryValueFromASTSource(source sourceprovenance.ASTSource) boundaryValueReader {
	return func(result *check.Result, point cfg.Point) (product.Value, bool) {
		return boundaryValueFromSource(result, point, source)
	}
}

func boundaryValueFromSource(result *check.Result, point cfg.Point, source sourceprovenance.ASTSource) (product.Value, bool) {
	if result == nil {
		return product.Value{}, false
	}
	if source.Kind == factflow.ValueSourceCall {
		return result.SourceValueAtBoundary(point, factflow.ValueSource{
			Kind:         source.Kind,
			ResultIndex:  source.ResultIndex,
			CallPoint:    source.CallPoint,
			HasCallPoint: source.HasCallPoint,
			Final:        source.Final,
			Expanded:     source.Expanded,
			Adjusted:     source.Adjusted,
			OpenTail:     source.OpenTail,
		})
	}
	if source.Expr != nil {
		return result.ExpressionValueAtBoundary(point, source.Expr)
	}
	return product.Value{}, false
}

func boundaryValueAdmissible(result *check.Result, value product.Value, want typ.Type) bool {
	if result == nil || result.Registry() == nil || want == nil {
		return false
	}
	reg := result.Registry()
	if gotEvidence := product.Get(reg, value, evidence.Key); evidence.Equal(gotEvidence, evidence.GradualTop()) {
		return true
	}
	if witness := product.Get(reg, value, typewitness.Key); !witness.IsTop() {
		if t, ok := witness.Type(); ok && subtype.IsSubtype(t, want) {
			return true
		}
	}
	if projected, ok := concreteBoundaryType(result, value); ok && subtype.IsSubtype(projected, want) {
		return true
	}
	return false
}

func concreteBoundaryType(result *check.Result, value product.Value) (typ.Type, bool) {
	if result == nil || result.Registry() == nil {
		return nil, false
	}
	reg := result.Registry()
	if witness := product.Get(reg, value, typewitness.Key); !witness.IsTop() {
		return witness.Type()
	}
	origin := product.Get(reg, value, variantorigin.Key)
	if !origin.IsBottom() && !origin.IsTop() {
		if t, ok := discriminant.TypeFromOrigin(origin.Family(), origin.Cases()); ok {
			return t, true
		}
	}
	return scalarRuntimeKindType(reg, value)
}

func scalarRuntimeKindType(reg *axis.Registry, value product.Value) (typ.Type, bool) {
	kinds := product.Get(reg, value, runtimekind.Key)
	if kinds.IsBottom() || kinds.IsTop() {
		return nil, false
	}
	var members []typ.Type
	for _, tag := range kinds.Tags() {
		switch tag {
		case runtimekind.Nil:
			members = append(members, typ.Nil)
		case runtimekind.Boolean:
			members = append(members, typ.Boolean)
		case runtimekind.Number:
			members = append(members, typ.Number)
		case runtimekind.String:
			members = append(members, typ.String)
		default:
			return nil, false
		}
	}
	if len(members) == 0 {
		return nil, false
	}
	t := typ.NewUnion(members...)
	if presence.Equal(product.PresenceOf(value), presence.Maybe()) && !typeIncludesNil(t) {
		t = typ.NewOptional(t)
	}
	if normalized := typ.NormalizeNilType(t); normalized != nil && normalized.Kind() == kind.Nil {
		return typ.Nil, true
	}
	return t, true
}

func topLikeType(t typ.Type) bool {
	return t == nil || typ.IsAny(t) || typ.IsUnknown(t)
}
