package body

import (
	"github.com/wippyai/go-lua/analysis/check/body/internal/readexpr"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	factapply "github.com/wippyai/go-lua/analysis/engine/factapply"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/factquery"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/type/refinement"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

// SourceValueAtBoundary resolves a lowered value source at the solved boundary
// for point. Node-local solved effects, such as call-result facts,
// postconditions, and assignments, are visible at that boundary. This is a
// projection of solved state only; read models that need declaration recovery
// must opt into SourceValueWithRootDeclarationRecoveryAtBoundary.
func (r *Result) SourceValueAtBoundary(point cfg.Point, source factflow.ValueSource) (product.Value, bool) {
	if r == nil || r.registry == nil {
		return product.Value{}, false
	}
	in, ok := r.boundaryStateAt(point)
	if !ok {
		return product.Value{}, false
	}
	sources := r.boundarySources()
	if sources == nil {
		return product.Value{}, false
	}
	value, ok := sources.ValueOfSource(point, source, in, r.boundaryRead)
	if !ok || product.Equal(r.registry, value, product.Bottom(r.registry)) {
		return product.Value{}, false
	}
	return value, true
}

func readableConcreteType(reg *axis.Registry, value product.Value) bool {
	if reg == nil {
		return false
	}
	t, ok := typevalue.TypeOf(reg, value)
	if !ok || t == nil || typ.IsAny(t) || typ.IsUnknown(t) || typ.IsNever(t) || refinement.ContainsFreeTypeParam(t) {
		return false
	}
	ev := product.Get(reg, value, evidence.Key)
	return !ev.IsExplicitTop() && !ev.IsGradualTop()
}

// LocalAssignmentSourceValueAtBoundary reads the lowered value source for the
// semantic local assignment at point when it corresponds to source.
func (r *Result) LocalAssignmentSourceValueAtBoundary(point cfg.Point, source sourceprovenance.ASTSource) (product.Value, bool) {
	return r.localAssignmentBoundaryValue(point, source, r.SourceValueAtBoundary)
}

// localAssignmentBoundaryValue resolves the lowered value source of the local
// assignment at point (when its AST source matches) through resolve.
func (r *Result) localAssignmentBoundaryValue(
	point cfg.Point,
	source sourceprovenance.ASTSource,
	resolve func(cfg.Point, factflow.ValueSource) (product.Value, bool),
) (product.Value, bool) {
	if r == nil {
		return product.Value{}, false
	}
	fact, ok := r.LocalAssignment(point)
	if !ok || fact.Source != source {
		return product.Value{}, false
	}
	lowered, ok := r.facts.LocalAssignment(point)
	if !ok {
		return product.Value{}, false
	}
	return resolve(point, lowered.Source())
}

// SourceValueWithRootDeclarationRecoveryAtBoundary resolves a lowered value
// source for read models that explain or contextualize code at a point. It
// first reads the solved boundary source. If that source is only weak
// top/unknown evidence, it may recover a stronger value from a dominating root
// declaration. Final solved-state projections should call SourceValueAtBoundary
// instead.
func (r *Result) SourceValueWithRootDeclarationRecoveryAtBoundary(point cfg.Point, source factflow.ValueSource) (product.Value, bool) {
	if r == nil || r.registry == nil {
		return product.Value{}, false
	}
	in, hasState := r.boundaryStateAt(point)
	value, ok := r.SourceValueAtBoundary(point, source)
	if ok && readableConcreteType(r.registry, value) {
		return value, true
	}
	if hasState {
		if declaration, declarationOK := r.rootDeclarationSourceForExpr(point, source.ExprRef); declarationOK {
			if recoveredValue, ok := r.rootDeclarationValue(declaration, in); ok {
				return recoveredValue, true
			}
		}
	}
	if ok {
		return value, true
	}
	return product.Value{}, false
}

// LocalAssignmentSourceValueWithRootDeclarationRecoveryAtBoundary is the
// declaration-recovering counterpart to LocalAssignmentSourceValueAtBoundary.
func (r *Result) LocalAssignmentSourceValueWithRootDeclarationRecoveryAtBoundary(point cfg.Point, source sourceprovenance.ASTSource) (product.Value, bool) {
	return r.localAssignmentBoundaryValue(point, source, r.SourceValueWithRootDeclarationRecoveryAtBoundary)
}

// ExpressionValueAtBoundary projects a Lua expression's product value at the
// diagnostic read boundary for point.
func (r *Result) ExpressionValueAtBoundary(point cfg.Point, expr ast.Expr) (product.Value, bool) {
	p, ok := r.ExpressionPath(expr)
	if !ok {
		return product.Value{}, false
	}
	return r.PathValueAtBoundary(point, p)
}

// PathValueAtBoundary projects a path's product value at the diagnostic read
// boundary for point.
func (r *Result) PathValueAtBoundary(point cfg.Point, p pathdom.Path) (product.Value, bool) {
	if r == nil || r.registry == nil || p.IsEmpty() {
		return product.Value{}, false
	}
	in, ok := r.boundaryStateAt(point)
	if !ok {
		return product.Value{}, false
	}
	value, ok := readexpr.Project(readexpr.Config{
		Registry:   r.registry,
		Facts:      r.facts,
		Visibility: r.visibility,
		TypeValues: r.typeValues,
	}, point, p, in)
	if !ok || product.Equal(r.registry, value, product.Bottom(r.registry)) {
		return product.Value{}, false
	}
	return value, true
}

// LengthFloorAtBoundary returns the proven length floor for array path p at the
// diagnostic read boundary for point: a returned (lo, true) asserts len(p) >= lo.
func (r *Result) LengthFloorAtBoundary(point cfg.Point, p pathdom.Path) (int64, bool) {
	if r == nil || r.visibility == nil || p.IsEmpty() {
		return 0, false
	}
	in, ok := r.boundaryStateAt(point)
	if !ok {
		return 0, false
	}
	pathKey := r.visibility.KeyAt(point, p)
	if pathKey == "" {
		return 0, false
	}
	return in.ReadLenFloor(pathKey)
}

// IndexInRangeAtBoundary reports whether the current boundary state proves
// indexPath <= len(arrayPath). Callers must pair this with a separate proof that
// indexPath is positive before dropping nil from a Lua array read.
func (r *Result) IndexInRangeAtBoundary(point cfg.Point, indexPath, arrayPath pathdom.Path) bool {
	if r == nil || r.visibility == nil || indexPath.IsEmpty() || arrayPath.IsEmpty() {
		return false
	}
	in, ok := r.boundaryStateAt(point)
	if !ok {
		return false
	}
	indexKey := r.visibility.KeyAt(point, indexPath)
	arrayKey := r.visibility.KeyAt(point, arrayPath)
	if indexKey == "" || arrayKey == "" {
		return false
	}
	return in.HasIndexInRangeProof(indexKey, arrayKey)
}

// NumericFloorAtBoundary returns the proven numeric lower bound for p at point:
// a returned (lo, true) asserts value(p) >= lo at that boundary.
func (r *Result) NumericFloorAtBoundary(point cfg.Point, p pathdom.Path) (int64, bool) {
	if r == nil || r.visibility == nil || p.IsEmpty() {
		return 0, false
	}
	in, ok := r.boundaryStateAt(point)
	if !ok {
		return 0, false
	}
	pathKey := visibility.RootOrVisibleKeyAt(r.visibility, point, p)
	if pathKey == "" {
		return 0, false
	}
	return in.ReadNumFloor(pathKey)
}

// SymbolValueAtBoundary reads a root symbol value at the diagnostic read
// boundary for point.
func (r *Result) SymbolValueAtBoundary(point cfg.Point, id symbol.ID) (product.Value, bool) {
	if id == 0 {
		return product.Value{}, false
	}
	return r.PathValueAtBoundary(point, pathdom.NewPath(id, r.SymbolName(id)))
}

// CallOutcomeAt resolves the configured call-boundary evidence for point.
func (r *Result) CallOutcomeAt(point cfg.Point) (factapply.CallOutcome, bool) {
	if r == nil || r.registry == nil || r.callOutcome == nil {
		return factapply.CallOutcome{}, false
	}
	site, ok := r.facts.CallSite(point)
	if !ok {
		return factapply.CallOutcome{}, false
	}
	in, ok := r.StateAt(point)
	if !ok {
		return factapply.CallOutcome{}, false
	}
	graph := r.Graph()
	ctx := transfer.NodeContext{
		Graph:    graph,
		Point:    point,
		Registry: r.registry,
		Read:     r.boundaryRead,
	}
	if graph != nil {
		ctx.Node = graph.Node(point)
	}
	return r.callOutcome(ctx, site.View(), in, r.boundaryRead), true
}

// CallExprResultValue resolves the product value of result slot resultIndex
// produced by a syntactic call expression. It locates the call's own CFG point
// and reads the solved call-result slot there, letting diagnostics type an
// inner call result (e.g. the container of make()[1]) that has no symbol path.
func (r *Result) CallExprResultValue(call *ast.FuncCallExpr, resultIndex int) (product.Value, bool) {
	if r == nil || r.registry == nil || call == nil || resultIndex < 0 {
		return product.Value{}, false
	}
	point, ok := r.callExprPoint(call)
	if !ok {
		return product.Value{}, false
	}
	source, ok := factflow.NewCallValueSource(0, factflow.NoValueSourceIndex, factflow.NoValueSourceIndex, resultIndex, point, factflow.ValueSourceShape{})
	if !ok {
		return product.Value{}, false
	}
	return r.SourceValueAtBoundary(point, source)
}

func (r *Result) callExprPoint(call *ast.FuncCallExpr) (cfg.Point, bool) {
	if r == nil || r.semantics == nil {
		return 0, false
	}
	if r.callExprPts == nil {
		graph := r.Graph()
		if graph == nil {
			return 0, false
		}
		r.callExprPts = make(map[*ast.FuncCallExpr]cfg.Point)
		for _, point := range graph.RPO() {
			if fact, ok := r.semantics.Call(point); ok && fact.Call != nil {
				r.callExprPts[fact.Call] = point
			}
		}
	}
	point, ok := r.callExprPts[call]
	return point, ok
}

func (r *Result) rootDeclarationValue(declaration factquery.RootDeclarationSource, fallbackState state.State) (product.Value, bool) {
	if r == nil || r.registry == nil || declaration.Symbol == 0 {
		return product.Value{}, false
	}
	declState, ok := r.boundaryStateAt(declaration.Point)
	if !ok {
		declState = fallbackState
	}
	v := declState.ReadValue(r.registry, key.SymbolValue(declaration.Symbol))
	if readableConcreteType(r.registry, v) {
		return v, true
	}
	if declaration.Source.Kind == 0 {
		return product.Value{}, false
	}
	if recoveredValue, ok := r.sourceValueAtPoint(declaration.Point, declaration.Source, declState, r.boundaryRead); ok {
		if readableConcreteType(r.registry, recoveredValue) {
			return recoveredValue, true
		}
	}
	return product.Value{}, false
}

func (r *Result) rootDeclarationSourceForExpr(point cfg.Point, expr factflow.ExprRef) (factquery.RootDeclarationSource, bool) {
	if r == nil || expr == 0 || point == 0 {
		return factquery.RootDeclarationSource{}, false
	}
	exprPath, ok := r.facts.ExpressionPath(expr)
	if !ok || exprPath.Symbol == 0 || len(exprPath.Segments) != 0 {
		return factquery.RootDeclarationSource{}, false
	}
	graph := r.Graph()
	if graph == nil {
		return factquery.RootDeclarationSource{}, false
	}
	return factquery.DominatingRootDeclarationSource(point, exprPath.Symbol, r.facts, graph)
}
