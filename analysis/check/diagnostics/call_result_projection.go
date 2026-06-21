package diagnostics

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/internal/readmodel"
	"github.com/wippyai/go-lua/analysis/diagnostic"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/dominance"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	"github.com/wippyai/go-lua/analysis/lua/typeannotation"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/type/normalize"
	"github.com/wippyai/go-lua/analysis/type/refinement"
	"github.com/wippyai/go-lua/analysis/type/subtype"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

func dominatingCallResultPathType(result *body.Result, resolver typeannotation.Resolver, flow *diagnosticFlowCache, point cfg.Point, expr ast.Expr, defs map[symbol.ID]*ast.FunctionExpr) (typ.Type, bool) {
	if result == nil || expr == nil {
		return nil, false
	}
	accessPath, ok := result.ExpressionPath(expr)
	if !ok || accessPath.Symbol == 0 || len(accessPath.Segments) == 0 {
		return nil, false
	}
	if !allFieldSegments(accessPath.Segments) {
		return nil, false
	}
	root, ok := dominatingCallResultRootType(result, resolver, flow, point, accessPath.Symbol, defs)
	if !ok {
		return nil, false
	}
	got, ok := expectedTypeAtSegments(root, accessPath.Segments)
	if !ok || got == nil || typ.IsAny(got) || typ.IsUnknown(got) || projectionHasNil(got) {
		return nil, false
	}
	return got, true
}

func dominatingCallResultFieldSourceType(result *body.Result, flow *diagnosticFlowCache, point cfg.Point, expr ast.Expr, source sourceprovenance.ASTSource) (typ.Type, bool) {
	if result == nil || expr == nil || source.Kind != sourceprovenance.SourceExpression {
		return nil, false
	}
	accessPath, ok := result.ExpressionPath(expr)
	if !ok || accessPath.Symbol == 0 || len(accessPath.Segments) == 0 || !allFieldSegments(accessPath.Segments) {
		return nil, false
	}
	if !rootInitializedByDominatingCall(result, flow, point, accessPath.Symbol) {
		return nil, false
	}
	got, ok := readmodel.New(result).SourceType(point, source)
	if !ok || got == nil || typ.IsAny(got) || typ.IsUnknown(got) || projectionHasNil(got) {
		return nil, false
	}
	return got, true
}

func reassignedCallResultFieldBoundaryType(result *body.Result, resolver typeannotation.Resolver, flow *diagnosticFlowCache, point cfg.Point, expr ast.Expr, defs map[symbol.ID]*ast.FunctionExpr) (typ.Type, bool) {
	if result == nil || expr == nil {
		return nil, false
	}
	accessPath, ok := result.ExpressionPath(expr)
	if !ok || accessPath.Symbol == 0 || len(accessPath.Segments) == 0 || !allFieldSegments(accessPath.Segments) {
		return nil, false
	}
	source, assignPoint, ok := dominatingCallSourceForRootUnchecked(result, flow, point, accessPath.Symbol)
	if !ok || !callResultRootReassignedBeforeUse(result, flow, assignPoint, point, accessPath.Symbol) {
		return nil, false
	}
	members := make([]typ.Type, 0, 2)
	if root, ok := directCallReturnSourceType(result, resolver, source, defs); ok {
		if got, ok := expectedTypeAtSegments(root, accessPath.Segments); ok && readableProjectedType(got) {
			members = append(members, got)
		}
	}
	for _, got := range reachingRootReassignmentFieldTypes(result, resolver, flow, assignPoint, point, accessPath.Symbol, accessPath.Segments) {
		if readableProjectedType(got) {
			members = append(members, got)
		}
	}
	if len(members) == 0 {
		return nil, false
	}
	return normalize.UnionForEvidence(members...), true
}

func reachingRootReassignmentFieldTypes(
	result *body.Result,
	resolver typeannotation.Resolver,
	flow *diagnosticFlowCache,
	assignPoint, usePoint cfg.Point,
	target symbol.ID,
	segments []segment.Segment,
) []typ.Type {
	graph := result.Graph()
	if graph == nil || assignPoint == 0 || usePoint == 0 || target == 0 {
		return nil
	}
	var out []typ.Type
	for _, candidate := range graph.RPO() {
		if candidate == assignPoint {
			continue
		}
		fact, ok := result.OrdinaryAssignment(candidate)
		if !ok || !fact.HasSymbol || fact.Symbol != target || !ordinaryAssignmentInvalidatesRootCallResult(fact) {
			continue
		}
		if !diagnosticCanReach(flow, graph, assignPoint, candidate) || !diagnosticCanReach(flow, graph, candidate, usePoint) {
			continue
		}
		if got, ok := rootReassignmentFieldType(result, resolver, candidate, fact, segments); ok {
			out = append(out, got)
		}
	}
	return out
}

func rootReassignmentFieldType(result *body.Result, resolver typeannotation.Resolver, point cfg.Point, fact semantics.OrdinaryAssignmentFact, segments []segment.Segment) (typ.Type, bool) {
	if fact.Value == nil {
		return nil, false
	}
	if object, ok := result.ObjectLiteral(fact.Value); ok {
		return expectedTypeAtSegments(objectLiteralType(nil, object), segments)
	}
	root, ok := assignmentValueType(result, resolver, point, fact.Value, fact.Source)
	if !ok {
		return nil, false
	}
	return expectedTypeAtSegments(root, segments)
}

func readableProjectedType(t typ.Type) bool {
	return t != nil && !typ.IsAny(t) && !typ.IsUnknown(t)
}

func reassignedCallResultFieldEvidence(result *body.Result, resolver typeannotation.Resolver, flow *diagnosticFlowCache, point cfg.Point, expr ast.Expr) []diagnostic.Evidence {
	if result == nil || expr == nil {
		return nil
	}
	accessPath, ok := result.ExpressionPath(expr)
	if !ok || accessPath.Symbol == 0 || len(accessPath.Segments) == 0 || !allFieldSegments(accessPath.Segments) {
		return nil
	}
	_, assignPoint, ok := dominatingCallSourceForRootUnchecked(result, flow, point, accessPath.Symbol)
	if !ok {
		return nil
	}
	graph := result.Graph()
	if graph == nil {
		return nil
	}
	for _, candidate := range graph.RPO() {
		if candidate == assignPoint {
			continue
		}
		fact, ok := result.OrdinaryAssignment(candidate)
		if !ok || !fact.HasSymbol || fact.Symbol != accessPath.Symbol || !ordinaryAssignmentInvalidatesRootCallResult(fact) {
			continue
		}
		if !diagnosticCanReach(flow, graph, assignPoint, candidate) || !diagnosticCanReach(flow, graph, candidate, point) {
			continue
		}
		root := accessPath
		root.Segments = nil
		span := ast.SpanOf(fact.Value)
		if !span.Valid() {
			span = ast.SpanOf(fact.Target)
		}
		replacement, _ := rootReassignmentFieldType(result, resolver, candidate, fact, accessPath.Segments)
		return []diagnostic.Evidence{{
			Kind:  diagnostic.EvidenceAbstractFact,
			Trust: diagnostic.TrustProven,
			Span:  span,
			Message: reassignedCallResultFieldEvidenceMessage(
				root.String(),
				accessPath.String(),
				replacement,
			),
		}}
	}
	return nil
}

func callResultFieldOptionalImprecision(result *body.Result, flow *diagnosticFlowCache, point cfg.Point, expr ast.Expr, got, want typ.Type) bool {
	if result == nil || expr == nil || got == nil || want == nil {
		return false
	}
	accessPath, ok := result.ExpressionPath(expr)
	if !ok || accessPath.Symbol == 0 || len(accessPath.Segments) == 0 || !allFieldSegments(accessPath.Segments) {
		return false
	}
	if !rootInitializedByDominatingCall(result, flow, point, accessPath.Symbol) {
		return false
	}
	source, ok := dominatingCallSourceForRoot(result, flow, point, accessPath.Symbol)
	if !ok || !wrapperProviderReplacementDominatesCall(result, source) {
		return false
	}
	inner := projectionWithoutNil(got)
	return inner != nil && !typ.IsNever(inner) && subtype.IsSubtype(transparentComparableType(result, inner), transparentComparableType(result, want))
}

func directFunctionCurrentReturnPathType(result *body.Result, resolver typeannotation.Resolver, source sourceprovenance.ASTSource, defs map[symbol.ID]*ast.FunctionExpr) (typ.Type, bool) {
	if result == nil || source.Kind != sourceprovenance.SourceCall || !source.HasCallPoint || source.ResultIndex < 0 {
		return nil, false
	}
	fact, ok := result.Call(source.CallPoint)
	if !ok || fact.Call == nil {
		return nil, false
	}
	site, ok := result.CallSite(source.CallPoint)
	if !ok || site.CalleeSymbol() == 0 {
		return nil, false
	}
	fn := defs[site.CalleeSymbol()]
	if fn == nil {
		fn = dominatingFunctionDefinitionForPath(result, source.CallPoint, site.CalleePathRef())
	}
	if fn == nil {
		return nil, false
	}
	expr, ok := singleReturnExpr(fn, source.ResultIndex)
	if !ok {
		return nil, false
	}
	retPath, ok := result.ExpressionPath(expr)
	if !ok || retPath.IsEmpty() {
		return nil, false
	}
	value, ok := result.PathValueAtBoundary(source.CallPoint, retPath)
	if ok {
		if got, ok := readmodel.New(result).ValueTypeWithPresence(value); ok && got != nil && !typ.IsAny(got) && !typ.IsUnknown(got) {
			if refinement.ContainsFreeTypeParam(got) {
				return nil, false
			}
			return got, true
		}
	}
	got, ok := newFlowExpressionTyper(result, resolver, source.CallPoint, cachedGuardEnvironments(result)[source.CallPoint]).typeOf(expr)
	if !ok || refinement.ContainsFreeTypeParam(got) {
		return nil, false
	}
	return got, true
}

func singleReturnExpr(fn *ast.FunctionExpr, index int) (ast.Expr, bool) {
	if fn == nil || index < 0 || len(fn.Stmts) != 1 {
		return nil, false
	}
	ret, ok := fn.Stmts[0].(*ast.ReturnStmt)
	if !ok || index >= len(ret.Exprs) {
		return nil, false
	}
	return ret.Exprs[index], ret.Exprs[index] != nil
}

func allFieldSegments(segments []segment.Segment) bool {
	for _, seg := range segments {
		if seg.Kind != segment.SegmentField {
			return false
		}
	}
	return true
}

func rootInitializedByDominatingCall(result *body.Result, flow *diagnosticFlowCache, point cfg.Point, target symbol.ID) bool {
	_, ok := dominatingCallSourceForRoot(result, flow, point, target)
	return ok
}

func dominatingCallSourceForRoot(result *body.Result, flow *diagnosticFlowCache, point cfg.Point, target symbol.ID) (sourceprovenance.ASTSource, bool) {
	graph := result.Graph()
	if graph == nil || target == 0 {
		return sourceprovenance.ASTSource{}, false
	}
	source, assignPoint, ok := dominatingCallSourceForRootUnchecked(result, flow, point, target)
	if !ok || callResultRootReassignedBeforeUse(result, flow, assignPoint, point, target) {
		return sourceprovenance.ASTSource{}, false
	}
	return source, true
}

func dominatingCallSourceForRootUnchecked(result *body.Result, flow *diagnosticFlowCache, point cfg.Point, target symbol.ID) (sourceprovenance.ASTSource, cfg.Point, bool) {
	graph := result.Graph()
	if graph == nil || target == 0 {
		return sourceprovenance.ASTSource{}, 0, false
	}
	var idom map[cfg.Point]cfg.Point
	if flow != nil && flow.graph == graph {
		idom = flow.immediateDominators()
	} else {
		idom = dominance.ComputeImmediateDominatorInfo(graph).Map()
	}
	visited := make(map[cfg.Point]struct{}, graph.Size())
	for cursor := point; ; {
		if _, ok := visited[cursor]; ok {
			return sourceprovenance.ASTSource{}, 0, false
		}
		visited[cursor] = struct{}{}
		if fact, ok := result.OrdinaryAssignment(cursor); ok && fact.HasSymbol && fact.Symbol == target && (!fact.HasPath || len(fact.Path.Segments) == 0) {
			return sourceprovenance.ASTSource{}, 0, false
		}
		if fact, ok := result.LocalAssignment(cursor); ok && fact.HasSymbol && fact.Symbol == target {
			if fact.Source.Kind == sourceprovenance.SourceCall {
				return fact.Source, cursor, true
			}
			return sourceprovenance.ASTSource{}, 0, false
		}
		parent, ok := idom[cursor]
		if !ok || parent == cursor {
			return sourceprovenance.ASTSource{}, 0, false
		}
		cursor = parent
	}
}

func callResultRootReassignedBeforeUse(result *body.Result, flow *diagnosticFlowCache, assignPoint, usePoint cfg.Point, target symbol.ID) bool {
	graph := result.Graph()
	if graph == nil || assignPoint == 0 || usePoint == 0 || target == 0 {
		return true
	}
	for _, candidate := range graph.RPO() {
		if candidate == assignPoint {
			continue
		}
		fact, ok := result.OrdinaryAssignment(candidate)
		if !ok || !fact.HasSymbol || fact.Symbol != target {
			continue
		}
		if !ordinaryAssignmentInvalidatesRootCallResult(fact) {
			continue
		}
		if diagnosticCanReach(flow, graph, assignPoint, candidate) && diagnosticCanReach(flow, graph, candidate, usePoint) {
			return true
		}
	}
	return false
}

func ordinaryAssignmentInvalidatesRootCallResult(fact semantics.OrdinaryAssignmentFact) bool {
	return !fact.HasPath || fact.Path.Symbol == fact.Symbol
}

func wrapperProviderReplacementDominatesCall(result *body.Result, source sourceprovenance.ASTSource) bool {
	if result == nil || source.Kind != sourceprovenance.SourceCall || !source.HasCallPoint {
		return false
	}
	fact, ok := result.Call(source.CallPoint)
	if !ok || fact.Call == nil {
		return false
	}
	site, ok := result.CallSite(source.CallPoint)
	if !ok || site.CalleeSymbol() == 0 {
		return false
	}
	defs := directCallDefinitions(result, nil)
	fn := defs[site.CalleeSymbol()]
	if fn == nil {
		fn = dominatingFunctionDefinitionForPath(result, source.CallPoint, site.CalleePathRef())
	}
	retExpr, ok := singleReturnExpr(fn, source.ResultIndex)
	if !ok {
		return false
	}
	retCall, ok := retExpr.(*ast.FuncCallExpr)
	if !ok || retCall.Func == nil {
		return false
	}
	calleePath, ok := result.ExpressionPath(retCall.Func)
	if !ok || len(calleePath.Segments) == 0 {
		return false
	}
	return pathAssignmentDominates(result, source.CallPoint, calleePath.Parent())
}

func dominatingFunctionDefinitionForPath(result *body.Result, point cfg.Point, target pathdom.Path) *ast.FunctionExpr {
	fn, _, ok := dominatingFunctionDefinitionForPathWithPoint(result, point, target)
	if !ok {
		return nil
	}
	return fn
}

func dominatingFunctionDefinitionForPathWithPoint(result *body.Result, point cfg.Point, target pathdom.Path) (*ast.FunctionExpr, cfg.Point, bool) {
	graph := result.Graph()
	if graph == nil || target.IsEmpty() {
		return nil, 0, false
	}
	dom := dominance.ComputeImmediateDominatorInfo(graph)
	var out *ast.FunctionExpr
	var outPoint cfg.Point
	for _, candidate := range graph.RPO() {
		if !dom.StrictlyDominates(candidate, point) {
			continue
		}
		fact, ok := result.FunctionDefinition(candidate)
		if !ok || !fact.HasTargetPath || fact.Func == nil {
			continue
		}
		if fact.TargetPath.Equal(target) {
			out = fact.Func
			outPoint = candidate
		}
	}
	if out == nil {
		return nil, 0, false
	}
	return out, outPoint, true
}

func pathAssignmentDominates(result *body.Result, point cfg.Point, target pathdom.Path) bool {
	graph := result.Graph()
	if graph == nil || target.IsEmpty() {
		return false
	}
	dom := dominance.ComputeImmediateDominatorInfo(graph)
	for _, candidate := range graph.RPO() {
		if !dom.StrictlyDominates(candidate, point) {
			continue
		}
		fact, ok := result.OrdinaryAssignment(candidate)
		if !ok || !fact.HasPath {
			continue
		}
		if fact.Path.Equal(target) {
			return true
		}
	}
	return false
}

func dominatingCallResultRootType(result *body.Result, resolver typeannotation.Resolver, flow *diagnosticFlowCache, point cfg.Point, target symbol.ID, defs map[symbol.ID]*ast.FunctionExpr) (typ.Type, bool) {
	source, ok := dominatingCallSourceForRoot(result, flow, point, target)
	if !ok {
		return nil, false
	}
	return directCallReturnSourceType(result, resolver, source, defs)
}
