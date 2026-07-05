package transferfacts

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
	"github.com/wippyai/go-lua/analysis/lua/pathexpr"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	luatypeprojection "github.com/wippyai/go-lua/analysis/lua/typeprojection"
	"github.com/wippyai/go-lua/analysis/lua/valueexpr"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

func (l *lowerer) typeCastPostconditionRefinement(point cfg.Point, fact semantics.CallFact) (factflow.PostconditionRefinement, bool) {
	t, argPath, ok := l.directTypeCastCallAt(point, fact)
	if !ok {
		return factflow.PostconditionRefinement{}, false
	}
	return factflow.NewPostconditionRefinement(argPath, factflow.NewValueConstraint(l.untrustedTypeWitnessValue(t))), true
}

// addCastExposure records a covariant exposure for a cast (narrow as W) whose
// target type strictly widens the operand object. The cast value conforms at the
// VM at cast time, so the cast itself is not rejected; the unsoundness is that
// the cast result aliases the operand object, so a later write through the wider
// cast view can launder a wide value back. The operand object's witness is
// widened to the cast contract at the cast point. A read-only or width-only cast
// (no strictly-wider shared member) records no exposure and stays permissive.
func (l *lowerer) addCastExposure(input *factflow.FactsInput, point cfg.Point, expr ast.Expr) {
	cast, ok := castExpr(expr)
	if !ok {
		return
	}
	operandPath, ok := pathexpr.Resolve(cast.Expr, l.bindings)
	if !ok || operandPath.Symbol == 0 {
		return
	}
	target, ok := l.resolveType(cast.Type)
	if !ok || target == nil || typ.IsAny(target) || typ.IsUnknown(target) {
		return
	}
	sourceType, ok := l.aliasPathType(operandPath)
	if !ok {
		return
	}
	if !aliasStrictlyWidens(sourceType, target) {
		return
	}
	l.addCovariantExposureType(input, point, operandPath, target)
}

// castExpr matches a direct as/:: cast expression. A cast is itself a source
// provenance proof, so it is matched before unwrapping; ProofInner would unwrap
// it to its operand.
func castExpr(expr ast.Expr) (*ast.CastExpr, bool) {
	cast, ok := expr.(*ast.CastExpr)
	return cast, ok
}

// aliasPathType resolves the declared object type at a source path: the symbol's
// declared type for a bare path, or its structural projection for a sub-path.
func (l *lowerer) aliasPathType(p path.Path) (typ.Type, bool) {
	rootType, ok := l.symbolTypes[p.Symbol]
	if !ok {
		return nil, false
	}
	if len(p.Segments) == 0 {
		return rootType, true
	}
	projected, ok := luatypeprojection.ApplySegments(rootType, p.Segments)
	if !ok || projected == nil {
		return nil, false
	}
	return projected, true
}

func (l *lowerer) typeCastCallResultValue(fact semantics.CallFact) (factflow.CallResultValue, bool) {
	t, _, ok := l.directTypeCastCall(fact)
	if !ok {
		return factflow.CallResultValue{}, false
	}
	return factflow.NewCallResultValue(0, l.typeIsProofValue(t)), true
}

func (l *lowerer) directTypeCastCall(fact semantics.CallFact) (typ.Type, path.Path, bool) {
	call, ok := branchcond.TypeCall(fact.Call)
	if !ok {
		return nil, path.Path{}, false
	}
	t, ok := l.typeValueExpr(fact.Func)
	if !ok {
		return nil, path.Path{}, false
	}
	argPath, ok := pathexpr.Resolve(call.Args[0], l.bindings)
	if !ok || argPath.IsEmpty() {
		return nil, path.Path{}, false
	}
	return t, argPath, true
}

func (l *lowerer) directTypeCastCallAt(point cfg.Point, fact semantics.CallFact) (typ.Type, path.Path, bool) {
	t, argPath, ok := l.directTypeCastCall(fact)
	if !ok {
		return nil, path.Path{}, false
	}
	if wirPath, ok := l.typeCastArgumentPathFromWIR(point, fact); ok {
		argPath = wirPath
	}
	return t, argPath, true
}

func (l *lowerer) typeCastArgumentPathFromWIR(point cfg.Point, fact semantics.CallFact) (path.Path, bool) {
	if l == nil || l.wir == nil {
		return path.Path{}, false
	}
	if _, ok := branchcond.TypeCall(fact.Call); !ok {
		return path.Path{}, false
	}
	return l.callArgumentPathFromWIR(point, 0)
}

func (l *lowerer) typeValueExpr(expr ast.Expr) (typ.Type, bool) {
	if ident, ok := expr.(*ast.IdentExpr); ok && l.bindings != nil {
		decl, ok := l.bindings.TypeValueRef(ident)
		if ok {
			return l.resolveDecl(decl)
		}
		if sym, ok := l.bindings.SymbolOf(ident); ok {
			if t, ok := primitiveRuntimeCastType(ident.Value); ok && l.bindings.SymbolResolvesToGlobal(sym, ident.Value) {
				return t, true
			}
		}
	}
	parts, ok := valueexpr.TypeValueRefParts(expr)
	if !ok || l.typeResolver == nil {
		return nil, false
	}
	return l.typeResolver.ResolveTypeRef(parts)
}

func primitiveRuntimeCastType(name string) (typ.Type, bool) {
	switch name {
	case "boolean":
		return typ.Boolean, true
	case "number":
		return typ.Number, true
	case "integer":
		return typ.Integer, true
	case "string":
		return typ.String, true
	default:
		return nil, false
	}
}
