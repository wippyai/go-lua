package body

import (
	"github.com/wippyai/go-lua/analysis/check/body/internal/readexpr"
	"github.com/wippyai/go-lua/analysis/domain/constraint/decision"
	"github.com/wippyai/go-lua/analysis/domain/constraint/numeric"
	"github.com/wippyai/go-lua/analysis/domain/constraint/solver"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/typestate"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
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
// projection of solved state only; read models that explain diagnostics may
// opt into SourceValueForExplanationAtBoundary.
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

// SourceValueForExplanationAtBoundary resolves a lowered value source for read
// models that explain or contextualize code at a point. It first reads the
// solved boundary source. If that source is only weak top/unknown evidence, it
// may recover a stronger value from a dominating root declaration. Final
// solved-state projections should call SourceValueAtBoundary instead.
func (r *Result) SourceValueForExplanationAtBoundary(point cfg.Point, source factflow.ValueSource) (product.Value, bool) {
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

// LocalAssignmentSourceValueForExplanationAtBoundary is the explanatory
// counterpart to LocalAssignmentSourceValueAtBoundary.
func (r *Result) LocalAssignmentSourceValueForExplanationAtBoundary(point cfg.Point, source sourceprovenance.ASTSource) (product.Value, bool) {
	return r.localAssignmentBoundaryValue(point, source, r.SourceValueForExplanationAtBoundary)
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

// PathKeyAtBoundary returns the canonical path key used by fact application at
// point. It is exposed for diagnostics that need to match solved state lanes
// back to call-boundary facts without re-deriving visibility policy.
func (r *Result) PathKeyAtBoundary(point cfg.Point, p pathdom.Path) (pathdom.PathKey, bool) {
	if r == nil || r.visibility == nil || p.IsEmpty() {
		return "", false
	}
	key := r.visibility.KeyAt(point, p)
	if key == "" {
		return "", false
	}
	return key, true
}

func (r *Result) rootOrVisibleStateKeyAtBoundary(point cfg.Point, p pathdom.Path) (pathaddr.StateKey, bool) {
	if r == nil || r.visibility == nil || p.IsEmpty() {
		return "", false
	}
	return visibility.RootOrVisibleStateKeyAt(r.visibility, point, p)
}

func (r *Result) relationGraphKeyAtBoundary(point cfg.Point, p pathdom.Path, length bool) (pathdom.PathKey, bool) {
	stateKey, ok := r.rootOrVisibleStateKeyAtBoundary(point, p)
	if !ok {
		return "", false
	}
	key := stateKey.PathKey()
	if length {
		return state.LengthRelKey(key), true
	}
	return key, true
}

// TypestateResourceKeyAtBoundary returns the canonical resource key used by the
// typestate lane at point. It folds proven path equality, matching the
// call-boundary application semantics.
func (r *Result) TypestateResourceKeyAtBoundary(point cfg.Point, p pathdom.Path) (pathaddr.StateKey, bool) {
	key, ok := r.PathKeyAtBoundary(point, p)
	if !ok {
		return "", false
	}
	stateKey, ok := pathaddr.StateKeyFromPathKey(key)
	if !ok {
		return "", false
	}
	in, ok := r.boundaryStateAt(point)
	if !ok {
		return stateKey, true
	}
	return in.CanonicalTypestateResourceKey(r.visibility.KeySpace(), stateKey), true
}

// TypestateResourceAtBoundary returns the canonical typestate resource for a
// protocol target at point. This keeps the conversion from state keys to
// typestate resource IDs inside the analysis boundary instead of diagnostics.
func (r *Result) TypestateResourceAtBoundary(point cfg.Point, p pathdom.Path, protocol typestate.Protocol) (typestate.Resource, bool) {
	key, ok := r.TypestateResourceKeyAtBoundary(point, p)
	if !ok {
		return typestate.Resource{}, false
	}
	return state.TypestateResourceFromCanonicalKey(key, protocol), true
}

// PathsEquivalentAtBoundary reports whether the solved boundary state proves
// left and right are equivalent access paths at point.
func (r *Result) PathsEquivalentAtBoundary(point cfg.Point, left, right pathdom.Path) bool {
	if r == nil || r.visibility == nil || left.IsEmpty() || right.IsEmpty() {
		return false
	}
	leftKey, leftOK := r.PathKeyAtBoundary(point, left)
	rightKey, rightOK := r.PathKeyAtBoundary(point, right)
	if !leftOK || !rightOK {
		return false
	}
	if leftKey == rightKey {
		return true
	}
	in, ok := r.boundaryStateAt(point)
	if !ok {
		return false
	}
	for _, equivalent := range in.EquivalentPathKeys(r.visibility.KeySpace(), leftKey) {
		if equivalent == rightKey {
			return true
		}
	}
	for _, equivalent := range in.EquivalentPathKeys(r.visibility.KeySpace(), rightKey) {
		if equivalent == leftKey {
			return true
		}
	}
	return false
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
	pathKey, keyOK := r.visibility.StateKeyAt(point, p)
	if !keyOK {
		return 0, false
	}
	return in.ReadLenFloor(r.visibility.KeySpace(), pathKey)
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
	indexKey, indexOK := r.PathKeyAtBoundary(point, indexPath)
	arrayKey, arrayOK := r.PathKeyAtBoundary(point, arrayPath)
	if !indexOK || !arrayOK {
		return false
	}
	return in.HasIndexInRangeProof(r.visibility.KeySpace(), indexKey, arrayKey)
}

// DiffProvesIndexLELength reports whether the difference-logic constraints proven
// at point entail indexCoeff*value(indexPath) + indexOffset <= len(arrayPath), the
// upper half of an in-range proof for a possibly-scaled arithmetic index. It runs
// the difference-logic solver over the constraint set, deriving transitive and
// cross-variable bounds (i < j <= #xs, #a == #b, i + 1 <= #xs, 2*i <= #xs) the
// simple index-in-range proof cannot. Callers pair it with a positive-index proof.
func (r *Result) DiffProvesIndexLELength(point cfg.Point, indexPath pathdom.Path, indexCoeff int64, indexOffset int64, arrayPath pathdom.Path) bool {
	if r == nil || r.visibility == nil || indexPath.IsEmpty() || arrayPath.IsEmpty() {
		return false
	}
	in, ok := r.boundaryStateAt(point)
	if !ok {
		return false
	}
	indexKey, indexOK := r.relationGraphKeyAtBoundary(point, indexPath, false)
	arrayLenKey, arrayOK := r.relationGraphKeyAtBoundary(point, arrayPath, true)
	if !indexOK || !arrayOK {
		return false
	}
	snap := in.RelConstraints()
	if snap.Bottom || len(snap.Constraints) == 0 {
		return false
	}
	asserted := make([]numeric.NumericConstraint, 0, len(snap.Constraints))
	floorKeys := make(map[pathdom.PathKey]struct{})
	noteKey := func(k pathdom.PathKey) {
		if k == "" {
			return
		}
		if _, isLength := state.ArrayKeyOfLengthRel(k); isLength {
			return
		}
		floorKeys[k] = struct{}{}
	}
	for _, c := range snap.Constraints {
		asserted = append(asserted, numeric.NewScaledLe(c.CoA, c.A, c.CoB, c.B, c.C, c.K))
		noteKey(c.A)
		noteKey(c.B)
		noteKey(c.C)
	}
	// A value operand's proven numeric floor strengthens the system: a sum bound
	// i + j <= #xs only proves i <= #xs once j >= 0 is known.
	for k := range floorKeys {
		stateKey, keyOK := pathaddr.StateKeyFromPathKey(k)
		if !keyOK {
			continue
		}
		if lo, ok := in.ReadNumFloor(r.visibility.KeySpace(), stateKey); ok {
			asserted = append(asserted, numeric.GeConst{X: k, C: lo})
		}
	}
	// indexCoeff*index + offset <= len is proven when indexCoeff*index - len <=
	// -offset, the scaled goal entailed by the lane constraints.
	goal := numeric.NewScaledLe(indexCoeff, indexKey, 0, "", arrayLenKey, -indexOffset)
	return solver.DefaultPortfolio().Entails(asserted, goal) == decision.Valid
}

// IndexReadSafeAtBoundary reports whether the diagnostic boundary proves a Lua
// array index expression is both positive and within array bounds:
// indexCoeff*value(indexPath)+indexOffset >= 1 and <= len(arrayPath).
func (r *Result) IndexReadSafeAtBoundary(point cfg.Point, indexPath pathdom.Path, indexCoeff int64, indexOffset int64, arrayPath pathdom.Path) bool {
	if indexCoeff <= 0 {
		return false
	}
	inRange := r.DiffProvesIndexLELength(point, indexPath, indexCoeff, indexOffset, arrayPath)
	if !inRange && indexCoeff == 1 && indexOffset == 0 {
		inRange = r.IndexInRangeAtBoundary(point, indexPath, arrayPath)
	}
	if !inRange {
		return false
	}
	floor, ok := r.NumericFloorAtBoundary(point, indexPath)
	return ok && indexCoeff*floor+indexOffset >= 1
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
	pathKey, keyOK := r.rootOrVisibleStateKeyAtBoundary(point, p)
	if !keyOK {
		return 0, false
	}
	return in.ReadNumFloor(r.visibility.KeySpace(), pathKey)
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
func (r *Result) CallOutcomeAt(point cfg.Point) (callpayload.CallOutcome, bool) {
	if r == nil || r.registry == nil || r.callOutcome == nil {
		return callpayload.CallOutcome{}, false
	}
	site, ok := r.facts.CallSite(point)
	if !ok {
		return callpayload.CallOutcome{}, false
	}
	in, ok := r.StateAt(point)
	if !ok {
		return callpayload.CallOutcome{}, false
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

// CallOutcomeForExpr returns the lowered call site and computed call outcome for
// a source call expression. Diagnostic producers use it when an AST-local scan
// needs to honor the same mutation/invalidation facts as boundary reads.
func (r *Result) CallOutcomeForExpr(call *ast.FuncCallExpr) (factflow.CallSite, callpayload.CallOutcome, bool) {
	if r == nil || call == nil {
		return factflow.CallSite{}, callpayload.CallOutcome{}, false
	}
	point, ok := r.callExprPoint(call)
	if !ok {
		return factflow.CallSite{}, callpayload.CallOutcome{}, false
	}
	site, ok := r.CallSite(point)
	if !ok {
		return factflow.CallSite{}, callpayload.CallOutcome{}, false
	}
	outcome, ok := r.CallOutcomeAt(point)
	if !ok {
		return factflow.CallSite{}, callpayload.CallOutcome{}, false
	}
	return site, outcome, true
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
