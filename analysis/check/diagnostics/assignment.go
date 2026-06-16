package diagnostics

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/diagnostics/internal/readmodel"
	"github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	"github.com/wippyai/go-lua/analysis/lua/typeannotation"
	"github.com/wippyai/go-lua/analysis/lua/valueexpr"
	"github.com/wippyai/go-lua/analysis/symbol"
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
	envs := guardEnvironments(result)
	defs := directCallDefinitions(result, nil)
	var out []diagnostic.Diagnostic
	for _, point := range graph.RPO() {
		if fact, ok := result.LocalAssignment(point); ok {
			if d, ok := p.localAssignment(result, point, fact, envs[point], defs); ok {
				out = append(out, d)
			}
		}
		if fact, ok := result.OrdinaryAssignment(point); ok {
			if d, ok := p.pathAssignment(result, point, fact, envs[point]); ok {
				out = append(out, d)
			}
		}
	}
	return out
}

func (p annotationAssignability) localAssignment(result *body.Result, point cfg.Point, fact semantics.LocalAssignmentFact, env guardEnv, directDefs map[symbol.ID]*ast.FunctionExpr) (diagnostic.Diagnostic, bool) {
	if fact.Type == nil || fact.Expr == nil {
		return diagnostic.Diagnostic{}, false
	}
	want, ok := lowerType(fact.Type, p.resolver)
	if !ok {
		return diagnostic.Diagnostic{}, false
	}
	if directCallResultAssignmentWouldReport(result, p.resolver, fact.Source, want, directDefs) {
		return p.objectLiteralAssignment(result, point, fact.Name, want, fact.Expr, fact.Type, env)
	}
	if directCallResultOwner(result, fact.Source) {
		if got, ok := callResultWitnessProvenMismatchType(result, point, fact.Source, want); ok {
			return assignmentDiagnostic(fact.Name, want, got, fact.Expr, fact.Type), true
		}
	}
	if directCallResultOwner(result, fact.Source) && !directCallSourceHasSignature(result, fact.Source) {
		if !callResultWitnessProvenMismatch(result, point, fact.Source, want) {
			return p.objectLiteralAssignment(result, point, fact.Name, want, fact.Expr, fact.Type, env)
		}
	}
	got, ok := valueexpr.LiteralType(fact.Expr)
	optionalIndexProjection := false
	if !ok {
		got, ok = projectedOptionalIndexType(result, p.resolver, point, fact.Expr)
		optionalIndexProjection = ok
	}
	presenceAwareSourceProjection := false
	untrustedTopLike := false
	if !ok {
		got, ok = untrustedTopLikeExpressionTypeAt(result, p.resolver, point, fact.Expr)
		untrustedTopLike = ok
	}
	if !ok {
		got, ok = untrustedAnnotatedIdentifierType(result, p.resolver, fact.Expr)
		untrustedTopLike = ok
	}
	if !ok {
		got, ok = inferredFunctionValueType(result, point, fact.Expr)
	}
	if !ok {
		got, ok = sourceExpressionTypeWithPresence(result, point, fact.Source)
		presenceAwareSourceProjection = ok
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
		got, ok = optionalMemberReadType(result, p.resolver, point, env, fact.Expr)
	}
	if !ok {
		return p.objectLiteralAssignment(result, point, fact.Name, want, fact.Expr, fact.Type, env)
	}
	if !optionalIndexProjection && !presenceAwareSourceProjection && !typ.IsAny(got) && !typ.IsUnknown(got) {
		got = refineAssignmentSourceType(result, point, fact.Expr, got)
	}
	readBoundary := boundaryValueFromASTSource(fact.Source)
	if optionalIndexProjection {
		readBoundary = nil
	}
	mismatch := boundaryTypeMismatch(result, point, got, want, readBoundary)
	if untrustedTopLike {
		mismatch = boundaryProofTypeMismatch(result, point, got, want, readBoundary)
	}
	if mismatch {
		return assignmentDiagnostic(fact.Name, want, got, fact.Expr, fact.Type), true
	}
	return p.objectLiteralAssignment(result, point, fact.Name, want, fact.Expr, fact.Type, env)
}

func (p annotationAssignability) pathAssignment(result *body.Result, point cfg.Point, fact semantics.OrdinaryAssignmentFact, env guardEnv) (diagnostic.Diagnostic, bool) {
	if fact.Target == nil || fact.Value == nil {
		return diagnostic.Diagnostic{}, false
	}
	want, ok := assignmentTargetType(result, p.resolver, point, fact)
	if !ok || topLikeType(want) || refinement.ContainsFreeTypeParam(want) {
		return diagnostic.Diagnostic{}, false
	}
	if mismatch, ok := objectLiteralMemberMismatch(result, point, fact.Value, want, env); ok {
		return pathAssignmentDiagnostic(fact.Target, mismatch.expr, mismatch.got, mismatch.want), true
	}
	got, ok := assignmentValueType(result, p.resolver, point, fact.Value, fact.Source)
	if !ok {
		got, ok = untrustedTopLikeExpressionTypeAt(result, p.resolver, point, fact.Value)
		if !ok {
			return diagnostic.Diagnostic{}, false
		}
	}
	readBoundary := boundaryValueFromASTSource(fact.Source)
	if _, untrusted := untrustedTopLikeExpressionTypeAt(result, p.resolver, point, fact.Value); untrusted {
		if !boundaryProofTypeMismatch(result, point, got, want, readBoundary) {
			return diagnostic.Diagnostic{}, false
		}
		return pathAssignmentDiagnostic(fact.Target, fact.Value, got, want), true
	}
	if !boundaryTypeMismatch(result, point, got, want, readBoundary) {
		return diagnostic.Diagnostic{}, false
	}
	return pathAssignmentDiagnostic(fact.Target, fact.Value, got, want), true
}

func directCallResultAssignmentWouldReport(result *body.Result, resolver typeannotation.Resolver, source sourceprovenance.ASTSource, want typ.Type, defs map[symbol.ID]*ast.FunctionExpr) bool {
	if result == nil || source.Kind != sourceprovenance.SourceCall || !source.HasCallPoint ||
		want == nil || typ.IsAny(want) || typ.IsUnknown(want) || refinement.ContainsFreeTypeParam(want) {
		return false
	}
	fact, ok := result.Call(source.CallPoint)
	if !ok || fact.Call == nil {
		return false
	}
	site, ok := result.CallSite(source.CallPoint)
	if !ok {
		return false
	}
	if _, _, _, member := callMemberAccess(fact); member {
		if _, hasSignature := result.CallSignature(site); !hasSignature {
			return false
		}
	}
	var def *ast.FunctionExpr
	if site.CalleeSymbol() != 0 {
		def = defs[site.CalleeSymbol()]
	}
	contract, _, ok := directCallResultContract(result, source.CallPoint, fact, site, def, defs, resolver)
	if !ok {
		return false
	}
	got, ok := contract.returnType(source.ResultIndex)
	if !ok {
		got, ok = contract.declaredReturnType(source.ResultIndex)
	}
	if !ok || refinement.ContainsFreeTypeParam(got) {
		return false
	}
	return boundaryTypeMismatch(result, source.CallPoint, got, want, boundaryCallResultReader(source.CallPoint, source.ResultIndex))
}

// callResultWitnessProvenMismatch reports whether the converged result value of
// a call source carries a concrete type witness that provably contradicts want.
// A call to a body-defined function without an explicit return annotation has no
// declared contract, so this is the proof path for assigning its inferred result
// to an annotated local: the result type is taken from the summary's converged
// return value, and only a non-gradual witness contradiction reports.
func callResultWitnessProvenMismatch(result *body.Result, point cfg.Point, source sourceprovenance.ASTSource, want typ.Type) bool {
	_, ok := callResultWitnessProvenMismatchType(result, point, source, want)
	return ok
}

func callResultWitnessProvenMismatchType(result *body.Result, point cfg.Point, source sourceprovenance.ASTSource, want typ.Type) (typ.Type, bool) {
	if result == nil || want == nil {
		return nil, false
	}
	reader := readmodel.New(result)
	value, ok := reader.SourceValue(point, source)
	if !ok {
		return nil, false
	}
	if !reader.ValueWitnessProvenMismatch(value, want) {
		return nil, false
	}
	got, ok := reader.ValueType(value)
	return got, ok
}

func directCallSourceHasSignature(result *body.Result, source sourceprovenance.ASTSource) bool {
	if result == nil || source.Kind != sourceprovenance.SourceCall || !source.HasCallPoint {
		return false
	}
	site, ok := result.CallSite(source.CallPoint)
	if !ok {
		return false
	}
	_, ok = result.CallSignature(site)
	return ok
}

func (p annotationAssignability) objectLiteralAssignment(result *body.Result, point cfg.Point, name string, want typ.Type, expr ast.Expr, annotation ast.TypeExpr, env guardEnv) (diagnostic.Diagnostic, bool) {
	fact, ok := result.ObjectLiteral(expr)
	if !ok {
		return diagnostic.Diagnostic{}, false
	}
	if arms, ok := closedRecordUnionArms(want); ok {
		if objectLiteralAdmissibleToAnyArm(result, point, arms, fact, env) {
			return diagnostic.Diagnostic{}, false
		}
		return assignmentDiagnostic(name, want, objectLiteralType(want, fact), expr, annotation), true
	}
	for _, entry := range fact.Entries {
		expected, ok := expectedTypeAtSegments(want, entry.Suffix.Segments)
		if !ok {
			continue
		}
		got, ok := objectLiteralEntryMismatchType(result, point, entry, expected, env)
		if !ok {
			continue
		}
		return assignmentDiagnostic(name, expected, got, entry.Value, annotation), true
	}
	if field, ok := missingRequiredRecordField(want, fact); ok {
		return missingFieldAssignmentDiagnostic(name, want, field, expr, annotation), true
	}
	return diagnostic.Diagnostic{}, false
}
