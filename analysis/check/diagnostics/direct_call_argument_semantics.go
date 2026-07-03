package diagnostics

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/lua/typeannotation"
	"github.com/wippyai/go-lua/analysis/lua/typecall"
	"github.com/wippyai/go-lua/analysis/type/subtype"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
	"github.com/wippyai/go-lua/compiler/ast"
)

func objectLiteralMemberMismatchForCallArgument(
	result *body.Result,
	resolver typeannotation.Resolver,
	point cfg.Point,
	fact semantics.CallFact,
	index int,
	arg ast.Expr,
	want typ.Type,
	env guardEnv,
) (objectLiteralTypeMismatch, bool) {
	if result == nil || arg == nil {
		return objectLiteralTypeMismatch{}, false
	}
	if site, ok := result.CallSite(point); ok {
		if source, ok := site.ArgumentSourceAt(index); ok && source.HasExpr {
			if literal, ok := result.ObjectLiteralExpr(source.ExprRef); ok {
				return objectLiteralMemberMismatchWithValueSources(result, resolver, point, arg, want, env, literal)
			}
		}
	}
	return objectLiteralMemberMismatch(result, resolver, point, arg, want, env)
}

func displayAliasDescribesFlowType(result *body.Result, got, declared typ.Type) bool {
	if got == nil || declared == nil {
		return false
	}
	if typ.TypeEquals(got, declared) {
		return true
	}
	got = transparentComparableType(result, got)
	declared = transparentComparableType(result, declared)
	return subtype.IsSubtype(got, declared) && subtype.IsSubtype(declared, got)
}

func containsTypeParamSyntax(t typ.Type) bool {
	return containsTypeParamSyntaxDepth(t, nil, 0)
}

func containsTypeParamSyntaxDepth(t typ.Type, seen map[typ.Type]bool, depth int) bool {
	if t == nil || depth > typ.DefaultRecursionDepth {
		return false
	}
	t = unwrap.Annotated(t)
	if t == nil {
		return false
	}
	switch t.(type) {
	case *typ.TypeParam, *typ.Ref:
		return true
	}
	if seen == nil {
		seen = make(map[typ.Type]bool)
	}
	if seen[t] {
		return false
	}
	seen[t] = true
	return typ.WalkChildren(t, func(child typ.Type) bool {
		return containsTypeParamSyntaxDepth(child, seen, depth+1)
	})
}

func declaredArgumentExprType(result *body.Result, resolver typeannotation.Resolver, expr ast.Expr) (typ.Type, bool) {
	switch expr.(type) {
	case *ast.IdentExpr, *ast.CastExpr, *ast.NonNilAssertExpr:
		return staticExpressionType(result, resolver, expr)
	default:
		return nil, false
	}
}

func directCallArgumentDisplayType(result *body.Result, flow *diagnosticFlowCache, point cfg.Point, expr ast.Expr) (string, bool) {
	if result == nil || expr == nil {
		return "", false
	}
	if _, ok := expr.(*ast.IdentExpr); !ok {
		return "", false
	}
	accessPath, ok := result.ExpressionPath(expr)
	if !ok || accessPath.Symbol == 0 || len(accessPath.Segments) != 0 {
		return "", false
	}
	fact, _, ok := dominatingRootLocalAssignment(result, flow, point, accessPath.Symbol)
	if !ok || fact.Type == nil {
		return "", false
	}
	if rendered := display.AnnotationOrType(fact.Type, nil); rendered != "" {
		return rendered, true
	}
	return "", false
}

func genericObjectLiteralArgTypeMismatch(result *body.Result, arg ast.Expr, actual typ.Type, formal typ.Type) (objectLiteralTypeMismatch, bool) {
	if result == nil || arg == nil || actual == nil || formal == nil {
		return objectLiteralTypeMismatch{}, false
	}
	fact, ok := result.ObjectLiteral(arg)
	if !ok {
		return objectLiteralTypeMismatch{}, false
	}
	for _, entry := range fact.Entries {
		got, gotOK := expectedTypeAtSegments(actual, entry.Suffix.Segments)
		want, wantOK := expectedTypeAtSegments(formal, entry.Suffix.Segments)
		if !gotOK || !wantOK || got == nil || want == nil ||
			typ.IsAny(got) || typ.IsUnknown(got) || typ.IsAny(want) || typ.IsUnknown(want) {
			continue
		}
		if typecall.InstantiatedArgumentAssignable(got, want) {
			continue
		}
		return objectLiteralTypeMismatch{
			expr:     entry.Value,
			got:      got,
			want:     want,
			suffix:   segment.FormatSegments(entry.Suffix.Segments),
			segments: entry.Suffix.Segments,
		}, true
	}
	return objectLiteralTypeMismatch{}, false
}

func genericObjectLiteralMissingFieldEvidence(result *body.Result, arg ast.Expr, formal typ.Type) []diagnostic.Evidence {
	if result == nil || arg == nil || formal == nil {
		return nil
	}
	fact, ok := result.ObjectLiteral(arg)
	if !ok {
		return nil
	}
	field, ok := missingRequiredRecordField(formal, fact)
	if ok {
		return []diagnostic.Evidence{{
			Kind:    diagnostic.EvidenceMissingProof,
			Trust:   diagnostic.TrustUnknown,
			Span:    ast.SpanOf(arg),
			Message: missingRequiredFieldEvidence(field.Name),
		}}
	}
	method, ok := missingRequiredInterfaceMethod(formal, fact)
	if ok {
		return []diagnostic.Evidence{{
			Kind:    diagnostic.EvidenceMissingProof,
			Trust:   diagnostic.TrustUnknown,
			Span:    ast.SpanOf(arg),
			Message: missingRequiredMethodTypeEvidence(formal, method),
		}}
	}
	return nil
}
