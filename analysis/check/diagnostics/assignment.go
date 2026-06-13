package diagnostics

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/body/readmodel"
	"github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/type/refinement"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

// annotationAssignability reports clear contradictions between a local
// annotation and a syntactically known source literal or scalar operator
// expression. Broader flow-to-type projection belongs in later producers once
// the relevant value axes own it.
type annotationAssignability producerContext

func (p annotationAssignability) Produce(result *body.Result) []diagnostic.Diagnostic {
	if result == nil {
		return nil
	}
	graph := result.Graph()
	if graph == nil {
		return nil
	}
	envs := literalEnvironments(result)
	var out []diagnostic.Diagnostic
	for _, point := range graph.RPO() {
		if fact, ok := result.LocalAssignment(point); ok {
			if d, ok := p.localAssignment(result, point, fact, envs[point]); ok {
				out = append(out, d)
			}
		}
		if fact, ok := result.OrdinaryAssignment(point); ok {
			if d, ok := p.pathAssignment(result, point, fact); ok {
				out = append(out, d)
			}
		}
	}
	return out
}

func (p annotationAssignability) localAssignment(result *body.Result, point cfg.Point, fact semantics.LocalAssignmentFact, env literalEnv) (diagnostic.Diagnostic, bool) {
	if fact.Type == nil || fact.Expr == nil {
		return diagnostic.Diagnostic{}, false
	}
	want, ok := lowerType(fact.Type, p.resolver)
	if !ok {
		return diagnostic.Diagnostic{}, false
	}
	if directCallResultOwner(result, fact.Source) || directCallExpressionOwner(result, fact.Expr) {
		return p.objectLiteralAssignment(result, fact.Name, want, fact.Expr, fact.Type)
	}
	got, ok := literalType(fact.Expr)
	optionalIndexProjection := false
	if !ok {
		got, ok = projectedOptionalIndexType(result, p.resolver, fact.Expr)
		optionalIndexProjection = ok
	}
	if !ok {
		got, ok = readmodel.New(result).SourceType(point, fact.Source)
	}
	if !ok {
		got, ok = localScalarOperatorSourceType(result, p.resolver, fact.Expr)
	}
	if !ok {
		got, ok = projectedFlowSourceType(result, p.resolver, point, env, fact.Expr)
	}
	if !ok {
		got, ok = annotatedIdentifierType(result, p.resolver, point, fact.Expr)
	}
	if !ok {
		got, ok = explicitTopLikeExpressionType(result, p.resolver, fact.Expr)
	}
	if !ok {
		got, ok = explicitTopLikeCallFactSourceType(result, p.resolver, fact.Source)
	}
	if !ok {
		return p.objectLiteralAssignment(result, fact.Name, want, fact.Expr, fact.Type)
	}
	if !optionalIndexProjection {
		got = refineAssignmentSourceType(result, point, fact.Expr, got)
	}
	readBoundary := boundaryValueFromASTSource(fact.Source)
	if optionalIndexProjection {
		readBoundary = nil
	}
	if boundaryTypeMismatch(result, point, got, want, readBoundary) {
		return assignmentDiagnostic(fact.Name, want, got, fact.Expr, fact.Type), true
	}
	return p.objectLiteralAssignment(result, fact.Name, want, fact.Expr, fact.Type)
}

func (p annotationAssignability) pathAssignment(result *body.Result, point cfg.Point, fact semantics.OrdinaryAssignmentFact) (diagnostic.Diagnostic, bool) {
	if fact.Target == nil || fact.Value == nil {
		return diagnostic.Diagnostic{}, false
	}
	want, ok := assignmentTargetType(result, p.resolver, fact)
	if !ok || topLikeType(want) || refinement.ContainsFreeTypeParam(want) {
		return diagnostic.Diagnostic{}, false
	}
	got, ok := assignmentValueType(result, p.resolver, point, fact.Value, fact.Source)
	if !ok {
		return diagnostic.Diagnostic{}, false
	}
	if !boundaryTypeMismatch(result, point, got, want, boundaryValueFromASTSource(fact.Source)) {
		return diagnostic.Diagnostic{}, false
	}
	return pathAssignmentDiagnostic(fact.Target, fact.Value, got, want), true
}

func (p annotationAssignability) objectLiteralAssignment(result *body.Result, name string, want typ.Type, expr ast.Expr, annotation ast.TypeExpr) (diagnostic.Diagnostic, bool) {
	fact, ok := result.ObjectLiteral(expr)
	if !ok {
		return diagnostic.Diagnostic{}, false
	}
	for _, entry := range fact.Entries {
		expected, ok := expectedTypeAtSegments(want, entry.Suffix.Segments)
		if !ok {
			continue
		}
		got, ok := literalType(entry.Value)
		if !ok || !clearMismatch(got, expected) {
			continue
		}
		return assignmentDiagnostic(name, expected, got, entry.Value, annotation), true
	}
	if field, ok := missingRequiredRecordField(want, fact); ok {
		return missingFieldAssignmentDiagnostic(name, want, field, expr, annotation), true
	}
	return diagnostic.Diagnostic{}, false
}
