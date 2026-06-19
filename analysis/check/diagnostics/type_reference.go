package diagnostics

import (
	"strings"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/lua/cfgfacts"
	"github.com/wippyai/go-lua/analysis/lua/typeresolve"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

// unresolvedTypeReferences reports annotation references that do not resolve in
// the lexical type namespace or parent function scopes.
type unresolvedTypeReferences producerContext

func (p unresolvedTypeReferences) Produce(result *body.Result) []diagnostic.Diagnostic {
	if result == nil {
		return nil
	}
	resolver, ok := p.resolver.(*resultResolver)
	if !ok || resolver == nil {
		return nil
	}
	var out []diagnostic.Diagnostic
	seenRefs := make(map[*ast.TypeRefExpr]struct{})
	seenPrimitives := make(map[*ast.PrimitiveTypeExpr]struct{})
	emitExpr := func(expr ast.TypeExpr) {
		out = append(out, unresolvedTypeRefs(expr, resolver, seenRefs, seenPrimitives)...)
	}
	emitExprs := func(exprs []ast.TypeExpr) {
		for _, expr := range exprs {
			emitExpr(expr)
		}
	}

	if fn := result.Function(); fn != nil {
		for _, param := range fn.TypeParams {
			emitExpr(param.Constraint)
		}
		if fn.ParList != nil {
			emitExprs(fn.ParList.Types)
			emitExpr(fn.ParList.VarargType)
		}
		emitExprs(fn.ReturnTypes)
	}

	graph := result.Graph()
	if graph == nil {
		return out
	}
	for _, point := range graph.RPO() {
		if fact, ok := result.LocalAssignment(point); ok {
			emitExpr(fact.Type)
		}
		if fact, ok := result.TypeDefinition(point); ok {
			emitTypeDefinitionRefs(fact, emitExpr)
		}
		if fact, ok := result.Call(point); ok && fact.Call != nil {
			emitExprs(fact.Call.TypeArgs)
		}
	}
	return out
}

func emitTypeDefinitionRefs(fact cfgfacts.TypeDefinitionFact, emitExpr func(ast.TypeExpr)) {
	switch fact.Kind {
	case cfgfacts.TypeDefinitionAlias:
		if fact.Type == nil {
			return
		}
		for _, param := range fact.Type.TypeParams {
			emitExpr(param.Constraint)
		}
		emitExpr(fact.Type.Type)
	case cfgfacts.TypeDefinitionInterface:
		if fact.Interface == nil {
			return
		}
		for _, ref := range fact.Interface.Extends {
			emitExpr(ref)
		}
		for _, field := range fact.Interface.Fields {
			emitExpr(field.Type)
		}
		for _, method := range fact.Interface.Methods {
			if method.Type != nil {
				emitExpr(method.Type)
			}
		}
	}
}

func unresolvedTypeRefs(
	expr ast.TypeExpr,
	resolver *resultResolver,
	seenRefs map[*ast.TypeRefExpr]struct{},
	seenPrimitives map[*ast.PrimitiveTypeExpr]struct{},
) []diagnostic.Diagnostic {
	if expr == nil || resolver == nil {
		return nil
	}
	var out []diagnostic.Diagnostic
	typeresolve.WalkTypeNameExpr(expr, func(ref *ast.TypeRefExpr) bool {
		if ref == nil {
			return true
		}
		if _, ok := seenRefs[ref]; ok {
			return true
		}
		seenRefs[ref] = struct{}{}
		if resolver.TypeRefResolved(ref) {
			return true
		}
		out = append(out, unresolvedTypeDiagnostic(ref, typeRefName(ref)))
		return true
	}, func(prim *ast.PrimitiveTypeExpr) bool {
		if prim == nil || typ.BuiltinPrimitiveName(prim.Name) {
			return true
		}
		if _, ok := seenPrimitives[prim]; ok {
			return true
		}
		seenPrimitives[prim] = struct{}{}
		if resolver.PrimitiveTypeResolved(prim) {
			return true
		}
		out = append(out, unresolvedTypeDiagnostic(prim, prim.Name))
		return true
	})
	return out
}

func unresolvedTypeDiagnostic(node ast.PositionHolder, name string) diagnostic.Diagnostic {
	span := ast.SpanOf(node)
	return diagnostic.New(diagnostic.DiagnosticSpec{
		Span:     span,
		Code:     CodeUnresolvedTypeReference,
		Severity: diagnostic.SeverityError,
		Message:  unresolvedTypeMessage(name),
		Labels:   []diagnostic.Label{sourceLabel(span, labelUnknownType)},
		Explanation: diagnostic.NewExplanation(
			diagnostic.Evidence{
				Kind:    diagnostic.EvidenceAbstractFact,
				Trust:   diagnostic.TrustProven,
				Span:    span,
				Message: unresolvedTypeEvidence(name),
			},
		),
		Help: unresolvedTypeHelp(),
	})
}

func typeRefName(ref *ast.TypeRefExpr) string {
	if ref == nil || len(ref.Path) == 0 {
		return "<missing>"
	}
	return strings.Join(ref.Path, ".")
}
