package body

import (
	"strconv"
	"strings"

	"github.com/wippyai/go-lua/analysis/check/body/internal/readexpr"
	"github.com/wippyai/go-lua/analysis/domain/constraint/decision"
	"github.com/wippyai/go-lua/analysis/domain/constraint/numeric"
	"github.com/wippyai/go-lua/analysis/domain/constraint/solver"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/typestate"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/identityvalue"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/proof"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/factquery"
	enginesourcevalue "github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/type/access"
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/refinement"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
	"github.com/wippyai/go-lua/compiler/ast"
)

// SourceValueAtBoundary resolves a lowered value source at the solved boundary
// for point. Node-local solved effects, such as call-result facts,
// postconditions, and assignments, are visible at that boundary. This is a
// projection of solved state only; read models that explain diagnostics may
// opt into SourceValueForExplanationAtBoundary.
func (r *Result) SourceValueAtBoundary(point cfg.Point, source factflow.ValueSource) (product.Value, bool) {
	return r.cachedSourceValue(sourceValueReadBoundary, point, source, func() (product.Value, bool) {
		return r.computeSourceValueAtBoundary(point, source)
	})
}

func (r *Result) computeSourceValueAtBoundary(point cfg.Point, source factflow.ValueSource) (product.Value, bool) {
	if r == nil || r.registry == nil {
		return product.Value{}, false
	}
	in, ok := r.boundaryStateAt(point)
	if !ok {
		return product.Value{}, false
	}
	if state.IsBottom(r.registry, in) {
		return product.Value{}, false
	}
	sources := r.boundarySources(sourceValueReadBoundary)
	if sources == nil {
		return product.Value{}, false
	}
	value, ok := sources.ValueOfSource(point, source, in, r.boundaryRead)
	if !ok || product.Equal(r.registry, value, product.Bottom(r.registry)) {
		return product.Value{}, false
	}
	return value, true
}

func readableConcreteType(reg *axis.Registry, typeValues *typevalue.Cache, value product.Value) bool {
	if reg == nil {
		return false
	}
	t, ok := typeValues.TypeOf(reg, value)
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
	return r.cachedSourceValue(sourceValueReadExplanationBoundary, point, source, func() (product.Value, bool) {
		return r.computeSourceValueForExplanationAtBoundary(point, source)
	})
}

func (r *Result) computeSourceValueForExplanationAtBoundary(point cfg.Point, source factflow.ValueSource) (product.Value, bool) {
	if r == nil || r.registry == nil {
		return product.Value{}, false
	}
	in, hasState := r.boundaryStateAt(point)
	if hasState && state.IsBottom(r.registry, in) {
		return product.Value{}, false
	}
	value, ok := r.SourceValueAtBoundary(point, source)
	if ok && readableConcreteType(r.registry, r.typeValues, value) {
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

// OrdinaryAssignmentSourceValueForExplanationAtBoundary is the explanatory
// counterpart for ordinary assignment sources.
func (r *Result) OrdinaryAssignmentSourceValueForExplanationAtBoundary(point cfg.Point, source sourceprovenance.ASTSource) (product.Value, bool) {
	if r == nil {
		return product.Value{}, false
	}
	fact, ok := r.OrdinaryAssignment(point)
	if !ok || fact.Source != source {
		return product.Value{}, false
	}
	lowered, ok := r.facts.OrdinaryAssignment(point)
	if !ok {
		return product.Value{}, false
	}
	return r.SourceValueForExplanationAtBoundary(point, lowered.Source())
}

// ExpressionValueAtBoundary projects a Lua expression's product value at the
// diagnostic read boundary for point.
func (r *Result) ExpressionValueAtBoundary(point cfg.Point, expr ast.Expr) (product.Value, bool) {
	p, ok := r.ExpressionPath(expr)
	if ok {
		if value, ok := r.PathValueAtBoundary(point, p); ok {
			return value, true
		}
	}
	if value, ok := r.attributeExpressionValueBeforeBoundary(point, expr); ok {
		return value, true
	}
	if value, ok := r.returnExpressionValueAtBoundary(point, expr); ok {
		return value, true
	}
	return r.expressionAssignmentSourceValueAtBoundary(point, expr, r.SourceValueAtBoundary)
}

// ExpressionValueBeforeBoundary projects a Lua expression's product value at
// the solved input to point, before same-node assignment/call materialization.
// Assignment diagnostics use this for RHS reads so the target write cannot make
// its own source look stronger than it was.
func (r *Result) ExpressionValueBeforeBoundary(point cfg.Point, expr ast.Expr) (product.Value, bool) {
	p, ok := r.ExpressionPath(expr)
	if ok {
		if value, ok := r.PathValueBeforeBoundary(point, p); ok {
			return value, true
		}
	}
	if value, ok := r.attributeExpressionValueBeforeBoundary(point, expr); ok {
		return value, true
	}
	return r.expressionAssignmentSourceValueAtBoundary(point, expr, r.SourceValueBeforeBoundary)
}

// SourceReadProvenPresentBeforeBoundary reports whether source is a dynamic
// indexed read whose solved facts prove the selected slot is present before the
// boundary at point. It is the public read-boundary wrapper around readexpr's
// in-range/key-membership proof so diagnostics do not duplicate that logic.
func (r *Result) SourceReadProvenPresentBeforeBoundary(point cfg.Point, source factflow.ValueSource) bool {
	if r == nil || source.Kind != factflow.ValueSourceExpression || !source.HasExpr {
		return false
	}
	in, ok := r.solvedStateAt(point)
	if !ok || state.IsBottom(r.registry, in) {
		return false
	}
	return readexpr.DynamicIndexReadProvenPresent(r.readExprConfig(sourceValueReadBeforeBoundary), point, source.ExprRef, in)
}

// ExpressionReadProvenPresentBeforeBoundary reports whether expr's lowered
// source is a dynamic indexed read proven present before the boundary at point.
func (r *Result) ExpressionReadProvenPresentBeforeBoundary(point cfg.Point, expr ast.Expr) bool {
	if r == nil || expr == nil {
		return false
	}
	if attr, ok := expr.(*ast.AttrGetExpr); ok && attr.KeySyntax == ast.AttrKeyIndex && attr.Object != nil && attr.Key != nil {
		if containerPath, ok := r.ExpressionPath(attr.Object); ok && r.IndexReadSafeForExpressionAtBoundary(point, attr.Key, containerPath) {
			return true
		}
	}
	if p, ok := r.ExpressionPath(expr); ok && r.PathProvenPresentBeforeBoundary(point, p) {
		return true
	} else if ok && r.requiredPathDescendantProvenPresentBeforeBoundary(point, p) {
		return true
	} else if ok && r.DominatingRequiredMemberReadProvesPathPresent(point, p) {
		return true
	}
	if value, ok := r.ExpressionValueBeforeBoundary(point, expr); ok &&
		presence.Equal(product.PresenceOf(value), presence.Present()) {
		return true
	}
	if r.requiredMemberReadProvenPresentBeforeBoundary(point, expr) {
		return true
	}
	if fact, ok := r.LocalAssignment(point); ok && fact.Expr == expr {
		if lowered, ok := r.facts.LocalAssignment(point); ok {
			return r.SourceReadProvenPresentBeforeBoundary(point, lowered.Source())
		}
	}
	if fact, ok := r.OrdinaryAssignment(point); ok && fact.Value == expr {
		if lowered, ok := r.facts.OrdinaryAssignment(point); ok {
			return r.SourceReadProvenPresentBeforeBoundary(point, lowered.Source())
		}
		if lowered, ok := r.facts.PathAssignment(point); ok {
			return r.SourceReadProvenPresentBeforeBoundary(point, lowered.Source())
		}
	}
	return false
}

func (r *Result) requiredPathDescendantProvenPresentBeforeBoundary(point cfg.Point, p pathdom.Path) bool {
	if r == nil || p.IsEmpty() || len(p.Segments) == 0 {
		return false
	}
	for n := len(p.Segments) - 1; n >= 0; n-- {
		prefix := pathdom.Path{Root: p.Root, Symbol: p.Symbol, Version: p.Version}
		if n > 0 {
			prefix.Segments = append([]segment.Segment(nil), p.Segments[:n]...)
		}
		if !r.PathProvenPresentBeforeBoundary(point, prefix) {
			continue
		}
		prefixType, ok := r.pathTypeBeforeBoundary(point, prefix)
		if !ok || prefixType == nil {
			continue
		}
		if withoutNil := ProjectionWithoutNil(prefixType); withoutNil != nil && !typ.IsNever(withoutNil) {
			prefixType = withoutNil
		}
		if requiredSegmentsPresent(prefixType, p.Segments[n:]) {
			return true
		}
	}
	return false
}

func (r *Result) pathTypeBeforeBoundary(point cfg.Point, p pathdom.Path) (typ.Type, bool) {
	if value, ok := r.PathValueBeforeBoundary(point, p); ok {
		if t, typeOK := r.valueTypeWithPresence(value); typeOK && t != nil {
			return t, true
		}
	}
	return r.DeclaredPathTypeAt(point, p, !p.IsEmpty())
}

func requiredSegmentsPresent(root typ.Type, suffix []segment.Segment) bool {
	current := root
	for _, seg := range suffix {
		keyType, ok := segmentLiteralType(seg)
		if !ok || current == nil {
			return false
		}
		next, ok := access.RuntimeIndex(current, keyType)
		if !ok || next == nil || typevalue.TypeIncludesNil(next) {
			return false
		}
		current = next
	}
	return true
}

func segmentLiteralType(seg segment.Segment) (typ.Type, bool) {
	switch seg.Kind {
	case segment.SegmentField, segment.SegmentIndexString:
		if seg.Name == "" {
			return nil, false
		}
		return typ.LiteralString(seg.Name), true
	case segment.SegmentIndexInt:
		return typ.LiteralInt(int64(seg.Index)), true
	default:
		return nil, false
	}
}

func (r *Result) requiredMemberReadProvenPresentBeforeBoundary(point cfg.Point, expr ast.Expr) bool {
	attr, ok := expr.(*ast.AttrGetExpr)
	if !ok || attr.Object == nil || attr.Key == nil {
		return false
	}
	if !r.ExpressionReadProvenPresentBeforeBoundary(point, attr.Object) {
		return false
	}
	container, ok := r.ExpressionTypeBeforeBoundary(point, attr.Object)
	if !ok || container == nil {
		container, ok = r.DeclaredExpressionTypeAt(point, attr.Object)
	}
	if !ok || container == nil {
		return false
	}
	if withoutNil := ProjectionWithoutNil(container); withoutNil != nil && !typ.IsNever(withoutNil) {
		container = withoutNil
	}
	key, ok := memberReadKeyTypeBeforeBoundary(r, point, attr)
	if !ok || key == nil {
		return false
	}
	member, ok := access.RuntimeIndex(container, key)
	return ok && member != nil && !typevalue.TypeIncludesNil(member)
}

func memberReadKeyTypeBeforeBoundary(r *Result, point cfg.Point, attr *ast.AttrGetExpr) (typ.Type, bool) {
	if attr == nil || attr.Key == nil {
		return nil, false
	}
	if attr.KeySyntax != ast.AttrKeyIndex {
		name := ast.KeyName(attr.Key)
		if name == "" {
			return nil, false
		}
		return typ.LiteralString(name), true
	}
	if key, ok := r.ExpressionTypeBeforeBoundary(point, attr.Key); ok {
		return key, true
	}
	return LiteralExpressionType(attr.Key)
}

// PathProvenPresentBeforeBoundary reports whether the solved branch evidence
// proves p present before the boundary at point. It is the static-path sibling
// of SourceReadProvenPresentBeforeBoundary's dynamic-index proof.
func (r *Result) PathProvenPresentBeforeBoundary(point cfg.Point, p pathdom.Path) bool {
	if r == nil || r.visibility == nil || p.IsEmpty() {
		return false
	}
	ks := r.visibility.KeySpace()
	if ks == nil {
		return false
	}
	in, ok := r.solvedStateAt(point)
	if !ok || state.IsBottom(r.registry, in) {
		return false
	}
	snapshot := in.BranchProofsSnapshot(ks)
	if snapshot.Bottom || snapshot.Top || len(snapshot.Proofs) == 0 {
		return false
	}
	address := visibility.AddressAt(r.visibility, point, p)
	found := false
	address.ForEachStateKey(func(stateKey pathaddr.StateKey) bool {
		key, ok := ks.InternStateKey(stateKey)
		if !ok {
			return true
		}
		for _, proof := range snapshot.Proofs {
			if proof.Kind != pathevidence.BranchProofPathPresence ||
				proof.Path != key ||
				!presence.Equal(proof.Presence, presence.Present()) {
				continue
			}
			found = true
			return false
		}
		return true
	}, visibility.StateKeyVisible, visibility.StateKeyRootOrVisible, visibility.StateKeyStructural)
	return found
}

func (r *Result) attributeExpressionValueBeforeBoundary(point cfg.Point, expr ast.Expr) (product.Value, bool) {
	attr, ok := expr.(*ast.AttrGetExpr)
	if !ok || attr.Object == nil || attr.Key == nil {
		return product.Value{}, false
	}
	object, ok := r.attributeObjectValueBeforeBoundary(point, attr.Object)
	if !ok {
		return product.Value{}, false
	}
	objectType, ok := r.typeValues.TypeOf(r.registry, object)
	if !ok || objectType == nil {
		return product.Value{}, false
	}
	var projected typ.Type
	switch attr.KeySyntax {
	case ast.AttrKeyDot:
		name := ast.KeyName(attr.Key)
		if name == "" {
			return product.Value{}, false
		}
		var fieldOK bool
		projected, fieldOK = access.Field(objectType, name)
		if !fieldOK {
			if !typ.TypeEquals(objectType, typ.Nil) {
				return product.Value{}, false
			}
			projected = typ.Nil
		}
	default:
		keyType, keyOK := attrKeyLiteralType(attr.Key)
		if !keyOK || keyType == nil {
			return product.Value{}, false
		}
		var indexOK bool
		projected, indexOK = access.RuntimeIndex(objectType, keyType)
		if !indexOK {
			if !typ.TypeEquals(objectType, typ.Nil) {
				return product.Value{}, false
			}
			projected = typ.Nil
		}
	}
	value := typevalue.WithWitness(r.registry, r.typeValues.FromType(r.registry, projected), projected)
	value = enginesourcevalue.InheritTopOriginEvidence(r.registry, value, object)
	return value, true
}

func (r *Result) attributeObjectValueBeforeBoundary(point cfg.Point, expr ast.Expr) (product.Value, bool) {
	switch object := expr.(type) {
	case *ast.FuncCallExpr:
		return r.CallExprResultValue(object, 0)
	case *ast.AttrGetExpr:
		return r.attributeExpressionValueBeforeBoundary(point, object)
	default:
		return r.ExpressionValueBeforeBoundary(point, expr)
	}
}

func attrKeyLiteralType(expr ast.Expr) (typ.Type, bool) {
	switch key := expr.(type) {
	case *ast.StringExpr:
		return typ.LiteralString(key.Value), true
	case *ast.NumberExpr:
		if strings.ContainsAny(key.Value, ".eE") {
			v, err := strconv.ParseFloat(key.Value, 64)
			if err != nil {
				return nil, false
			}
			return typ.LiteralNumber(v), true
		}
		v, err := strconv.ParseInt(key.Value, 10, 64)
		if err != nil {
			return nil, false
		}
		return typ.LiteralInt(v), true
	default:
		return nil, false
	}
}

// StaticStringExprValueAtBoundary returns the statically-known string literal
// for expr at point. Literal syntax is accepted directly; non-literal
// expressions must be proven by the solved product value at the boundary.
func (r *Result) StaticStringExprValueAtBoundary(point cfg.Point, expr ast.Expr) (string, bool) {
	if key, ok := staticStringLiteralExpr(expr); ok {
		return key, true
	}
	if r == nil || expr == nil || r.registry == nil || r.typeValues == nil {
		return "", false
	}
	value, ok := r.ExpressionValueAtBoundary(point, expr)
	if !ok {
		return "", false
	}
	t, ok := proof.New(r.registry, r.typeValues).ValueTypeWithPresence(value)
	if !ok {
		return "", false
	}
	lit, ok := unwrap.Annotated(t).(*typ.Literal)
	if !ok || lit.Base != kind.String {
		return "", false
	}
	name, ok := lit.Value.(string)
	return name, ok
}

// PathLiteralTypeAtBoundary returns the current solved literal type for p at
// point. It is the canonical post-solve query for consumers that need to know
// whether flow facts have collapsed a path to one concrete literal value.
func (r *Result) PathLiteralTypeAtBoundary(point cfg.Point, p pathdom.Path) (typ.Type, bool) {
	if r == nil || p.IsEmpty() || r.registry == nil || r.typeValues == nil {
		return nil, false
	}
	value, ok := r.PathValueAtBoundary(point, p)
	if !ok {
		return nil, false
	}
	t, ok := proof.New(r.registry, r.typeValues).ValueTypeWithPresence(value)
	if !ok {
		return nil, false
	}
	lit, ok := unwrap.Annotated(t).(*typ.Literal)
	if !ok {
		return nil, false
	}
	return lit, true
}

func staticStringLiteralExpr(expr ast.Expr) (string, bool) {
	lit, ok := expr.(*ast.StringExpr)
	if !ok || lit == nil {
		return "", false
	}
	return lit.Value, true
}

func (r *Result) returnExpressionValueAtBoundary(point cfg.Point, expr ast.Expr) (product.Value, bool) {
	if r == nil || r.registry == nil || expr == nil {
		return product.Value{}, false
	}
	fact, ok := r.ReturnFact(point)
	if !ok {
		return product.Value{}, false
	}
	for i, returned := range fact.Exprs {
		if returned != expr {
			continue
		}
		in, ok := r.boundaryStateAt(point)
		if !ok {
			return product.Value{}, false
		}
		value := in.ReadReturnSlot(r.registry, i)
		if product.Equal(r.registry, value, product.Bottom(r.registry)) {
			return product.Value{}, false
		}
		return value, true
	}
	return product.Value{}, false
}

func (r *Result) expressionAssignmentSourceValueAtBoundary(
	point cfg.Point,
	expr ast.Expr,
	resolve func(cfg.Point, factflow.ValueSource) (product.Value, bool),
) (product.Value, bool) {
	if r == nil || expr == nil || resolve == nil {
		return product.Value{}, false
	}
	if fact, ok := r.LocalAssignment(point); ok && fact.Expr == expr {
		if lowered, ok := r.facts.LocalAssignment(point); ok {
			return resolve(point, lowered.Source())
		}
	}
	if fact, ok := r.OrdinaryAssignment(point); ok && fact.Value == expr {
		if lowered, ok := r.facts.OrdinaryAssignment(point); ok {
			return resolve(point, lowered.Source())
		}
		if lowered, ok := r.facts.PathAssignment(point); ok {
			return resolve(point, lowered.Source())
		}
	}
	return product.Value{}, false
}

// SourceValueBeforeBoundary resolves a lowered value source from the solved
// input to point, without same-node boundary effects.
func (r *Result) SourceValueBeforeBoundary(point cfg.Point, source factflow.ValueSource) (product.Value, bool) {
	return r.cachedSourceValue(sourceValueReadBeforeBoundary, point, source, func() (product.Value, bool) {
		return r.computeSourceValueBeforeBoundary(point, source)
	})
}

func (r *Result) computeSourceValueBeforeBoundary(point cfg.Point, source factflow.ValueSource) (product.Value, bool) {
	if r == nil || r.registry == nil {
		return product.Value{}, false
	}
	in, ok := r.solvedStateAt(point)
	if !ok {
		return product.Value{}, false
	}
	if state.IsBottom(r.registry, in) {
		return product.Value{}, false
	}
	value, ok := r.sourceValueAtPoint(sourceValueReadBeforeBoundary, point, source, in, r.beforeBoundarySourceRead(point))
	if !ok || product.Equal(r.registry, value, product.Bottom(r.registry)) {
		return product.Value{}, false
	}
	return value, true
}

func (r *Result) beforeBoundarySourceRead(point cfg.Point) func(cfg.Point) state.State {
	return func(sourcePoint cfg.Point) state.State {
		if sourcePoint == point {
			return r.stateRead(sourcePoint)
		}
		return r.boundaryRead(sourcePoint)
	}
}

// PathValueAtBoundary projects a path's product value at the diagnostic read
// boundary for point.
func (r *Result) PathValueAtBoundary(point cfg.Point, p pathdom.Path) (product.Value, bool) {
	return r.cachedPathValue(sourceValueReadBoundary, point, p, func() (product.Value, bool) {
		return r.computePathValueAtBoundary(point, p)
	})
}

func (r *Result) computePathValueAtBoundary(point cfg.Point, p pathdom.Path) (product.Value, bool) {
	return r.computePathValue(sourceValueReadBoundary, point, p, r.boundaryStateAt)
}

// PathValueBeforeBoundary projects a path from the solved point input, without
// applying same-node boundary transfer effects.
func (r *Result) PathValueBeforeBoundary(point cfg.Point, p pathdom.Path) (product.Value, bool) {
	return r.cachedPathValue(sourceValueReadBeforeBoundary, point, p, func() (product.Value, bool) {
		return r.computePathValueBeforeBoundary(point, p)
	})
}

func (r *Result) computePathValueBeforeBoundary(point cfg.Point, p pathdom.Path) (product.Value, bool) {
	return r.computePathValue(sourceValueReadBeforeBoundary, point, p, r.solvedStateAt)
}

func (r *Result) computePathValue(
	mode sourceValueReadMode,
	point cfg.Point,
	p pathdom.Path,
	stateAt func(cfg.Point) (state.State, bool),
) (product.Value, bool) {
	if r == nil || r.registry == nil || p.IsEmpty() {
		return product.Value{}, false
	}
	in, ok := stateAt(point)
	if !ok {
		return product.Value{}, false
	}
	if state.IsBottom(r.registry, in) {
		return product.Value{}, false
	}
	value, ok := readexpr.Project(r.readExprConfig(mode), point, p, in)
	if !ok || product.Equal(r.registry, value, product.Bottom(r.registry)) {
		return product.Value{}, false
	}
	return value, true
}

// StateKeyAtBoundary returns the typed point-visible state key for p at point.
// It is the canonical boundary vocabulary for state lanes that use visible
// source paths; PathKeyAtBoundary exists only for compatibility with lanes that
// still expose a path-key string carrier.
func (r *Result) StateKeyAtBoundary(point cfg.Point, p pathdom.Path) (pathaddr.StateKey, bool) {
	address, ok := r.boundaryAddress(point, p)
	if !ok {
		return "", false
	}
	return address.VisibleStateKey()
}

// PathKeyAtBoundary returns the canonical path key used by fact application at
// point. It is exposed for diagnostics that need to match solved state lanes
// back to call-boundary facts without re-deriving visibility policy.
func (r *Result) PathKeyAtBoundary(point cfg.Point, p pathdom.Path) (pathdom.PathKey, bool) {
	key, ok := r.StateKeyAtBoundary(point, p)
	if !ok {
		return "", false
	}
	return key.PathKey(), true
}

func (r *Result) rootOrVisibleStateKeyAtBoundary(point cfg.Point, p pathdom.Path) (pathaddr.StateKey, bool) {
	address, ok := r.boundaryAddress(point, p)
	if !ok {
		return "", false
	}
	return address.RootOrVisibleStateKey()
}

func (r *Result) boundaryAddress(point cfg.Point, p pathdom.Path) (visibility.Address, bool) {
	if r == nil || r.visibility == nil || p.IsEmpty() {
		return visibility.Address{}, false
	}
	return visibility.AddressAt(r.visibility, point, p), true
}

func (r *Result) relationGraphKeyAtBoundary(point cfg.Point, p pathdom.Path, length bool) (state.RelOperand, bool) {
	stateKey, ok := r.rootOrVisibleStateKeyAtBoundary(point, p)
	if !ok {
		return state.RelOperand{}, false
	}
	if length {
		return state.RelLengthOperand(stateKey), true
	}
	return state.RelValueOperand(stateKey), true
}

// TypestateResourceKeyAtBoundary returns the canonical resource key used by the
// typestate lane at point. It folds proven path equality, matching the
// call-boundary application semantics.
func (r *Result) TypestateResourceKeyAtBoundary(point cfg.Point, p pathdom.Path) (pathaddr.StateKey, bool) {
	stateKey, ok := r.StateKeyAtBoundary(point, p)
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
	leftKey, leftOK := r.StateKeyAtBoundary(point, left)
	rightKey, rightOK := r.StateKeyAtBoundary(point, right)
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
	leftPathKey := leftKey.PathKey()
	rightPathKey := rightKey.PathKey()
	for _, equivalent := range in.EquivalentPathKeys(r.visibility.KeySpace(), leftPathKey) {
		if equivalent == rightPathKey {
			return true
		}
	}
	for _, equivalent := range in.EquivalentPathKeys(r.visibility.KeySpace(), rightPathKey) {
		if equivalent == leftPathKey {
			return true
		}
	}
	return false
}

// DistinctPathsShareExactIdentityAtBoundary reports whether two different
// paths project to values with the same singleton runtime identity at point.
// Path equality and path-relation equality are handled by
// PathsEquivalentAtBoundary; this query owns the value-identity fallback.
func (r *Result) DistinctPathsShareExactIdentityAtBoundary(point cfg.Point, left, right pathdom.Path) bool {
	if r == nil || left.IsEmpty() || right.IsEmpty() || left.Equal(right) {
		return false
	}
	if r.PathsEquivalentAtBoundary(point, left, right) {
		return true
	}
	leftValue, leftOK := r.PathValueAtBoundary(point, left)
	rightValue, rightOK := r.PathValueAtBoundary(point, right)
	if !leftOK || !rightOK {
		return false
	}
	leftID, leftOK := identityvalue.ExactID(r.registry, leftValue)
	rightID, rightOK := identityvalue.ExactID(r.registry, rightValue)
	return leftOK && rightOK && leftID == rightID
}

// PathsAliasAtBoundary reports whether the solved boundary model proves two
// paths denote the same value path. It owns the fallback ladder diagnostics
// used to spell locally: exact path, solved path-equivalence, exact runtime
// identity, and dominating local alias declarations.
func (r *Result) PathsAliasAtBoundary(point cfg.Point, left, right pathdom.Path) bool {
	if r == nil || left.IsEmpty() || right.IsEmpty() {
		return false
	}
	return left.Equal(right) ||
		r.PathsEquivalentAtBoundary(point, left, right) ||
		r.DistinctPathsShareExactIdentityAtBoundary(point, left, right) ||
		r.dominatingAliasPathAtBoundary(point, left, right) ||
		r.dominatingAliasPathAtBoundary(point, right, left)
}

// PathsAliasWithSameSuffixAtBoundary reports whether two paths with the same
// field/index suffix denote the same projected value at point. It is stricter
// than PathsAliasAtBoundary: different suffixes are not considered aliases even
// if another fact lane could relate the complete paths.
func (r *Result) PathsAliasWithSameSuffixAtBoundary(point cfg.Point, left, right pathdom.Path) bool {
	if r == nil || left.IsEmpty() || right.IsEmpty() {
		return false
	}
	return r.pathsAliasWithExactSuffixAtBoundary(point, left, right) ||
		r.pathsAliasWithProjectedSuffixAtBoundary(point, left, right)
}

func (r *Result) pathsAliasWithExactSuffixAtBoundary(point cfg.Point, left, right pathdom.Path) bool {
	if !samePathSuffix(left, right) {
		return false
	}
	if left.Equal(right) || r.PathsEquivalentAtBoundary(point, left, right) {
		return true
	}
	if r.DistinctPathsShareExactIdentityAtBoundary(point, left, right) {
		return true
	}
	return len(left.Segments) > 0 &&
		r.DistinctPathsShareExactIdentityAtBoundary(point, left.RootOnly(), right.RootOnly())
}

func (r *Result) pathsAliasWithProjectedSuffixAtBoundary(point cfg.Point, left, right pathdom.Path) bool {
	maxSuffix := len(left.Segments)
	if len(right.Segments) < maxSuffix {
		maxSuffix = len(right.Segments)
	}
	for suffixLen := 1; suffixLen <= maxSuffix; suffixLen++ {
		leftPrefixLen := len(left.Segments) - suffixLen
		rightPrefixLen := len(right.Segments) - suffixLen
		if !sameSegments(left.Segments[leftPrefixLen:], right.Segments[rightPrefixLen:]) {
			continue
		}
		leftPrefix := pathPrefix(left, leftPrefixLen)
		rightPrefix := pathPrefix(right, rightPrefixLen)
		if r.PathsAliasAtBoundary(point, leftPrefix, rightPrefix) {
			return true
		}
	}
	return false
}

func samePathSuffix(left, right pathdom.Path) bool {
	return sameSegments(left.Segments, right.Segments)
}

func pathPrefix(p pathdom.Path, segments int) pathdom.Path {
	if segments <= 0 {
		return p.RootOnly()
	}
	if segments >= len(p.Segments) {
		return p.Clone()
	}
	prefix := p.RootOnly()
	prefix.Segments = append(prefix.Segments, p.Segments[:segments]...)
	return prefix
}

func (r *Result) dominatingAliasPathAtBoundary(point cfg.Point, alias, target pathdom.Path) bool {
	if r == nil || alias.Symbol == 0 || target.Symbol == 0 {
		return false
	}
	fact, _, ok := r.DominatingRootLocalAssignment(point, alias.Symbol)
	if !ok || fact.Expr == nil {
		return false
	}
	source, ok := r.ExpressionPath(fact.Expr)
	if !ok || source.IsEmpty() {
		return false
	}
	source = source.AppendSegments(alias.Segments)
	return source.Equal(target) ||
		r.PathsEquivalentAtBoundary(point, source, target) ||
		r.DistinctPathsShareExactIdentityAtBoundary(point, source, target)
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
	stateKey, keyOK := r.StateKeyAtBoundary(point, p)
	if !keyOK {
		return 0, false
	}
	return in.ReadLenFloor(r.visibility.KeySpace(), stateKey)
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
	indexKey, indexOK := r.StateKeyAtBoundary(point, indexPath)
	arrayKey, arrayOK := r.StateKeyAtBoundary(point, arrayPath)
	if !indexOK || !arrayOK {
		return false
	}
	return in.HasIndexInRangeProofForStateKeys(r.visibility.KeySpace(), indexKey, arrayKey)
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
	floorKeys := make(map[pathaddr.StateKey]struct{})
	valueKeys := make([]pathaddr.StateKey, 0, 3)
	for _, c := range snap.Constraints {
		asserted = append(asserted, c.NumericConstraint())
		valueKeys = c.AppendValueStateKeys(valueKeys[:0])
		for _, key := range valueKeys {
			floorKeys[key] = struct{}{}
		}
	}
	// A value operand's proven numeric floor strengthens the system: a sum bound
	// i + j <= #xs only proves i <= #xs once j >= 0 is known.
	for k := range floorKeys {
		if lo, ok := in.ReadNumFloor(r.visibility.KeySpace(), k); ok {
			asserted = append(asserted, numeric.GeConst{X: k.PathKey(), C: lo})
		}
	}
	// indexCoeff*index + offset <= len is proven when indexCoeff*index - len <=
	// -offset, the scaled goal entailed by the lane constraints.
	goal := numeric.NewScaledLe(indexCoeff, indexKey.NumericKey(), 0, "", arrayLenKey.NumericKey(), -indexOffset)
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
	stateKey, keyOK := r.rootOrVisibleStateKeyAtBoundary(point, p)
	if !keyOK {
		return 0, false
	}
	return in.ReadNumFloor(r.visibility.KeySpace(), stateKey)
}

// SymbolValueAtBoundary reads a root symbol value at the diagnostic read
// boundary for point.
func (r *Result) SymbolValueAtBoundary(point cfg.Point, id symbol.ID) (product.Value, bool) {
	if id == 0 {
		return product.Value{}, false
	}
	return r.PathValueAtBoundary(point, pathdom.NewPath(id, r.SymbolName(id)))
}

// UninitializedLocalDeclarationValueAtBoundary returns Lua's implicit nil for a
// local declaration without an initializer when that declaration still owns the
// root symbol at point.
func (r *Result) UninitializedLocalDeclarationValueAtBoundary(point cfg.Point, id symbol.ID) (product.Value, bool) {
	if r == nil || r.registry == nil || id == 0 {
		return product.Value{}, false
	}
	declaration, ok := r.DominatingPathRootDeclarationSource(point, pathdom.NewPath(id, r.SymbolName(id)))
	if !ok {
		return product.Value{}, false
	}
	fact, ok := r.LocalAssignment(declaration.Point)
	if !ok || !fact.HasSymbol || fact.Symbol != id || fact.Expr != nil || fact.Source.Kind != sourceprovenance.SourceNil {
		return product.Value{}, false
	}
	return typevalue.Nil(r.registry), true
}

// CallOutcomeAt resolves the configured call-boundary evidence for point.
func (r *Result) CallOutcomeAt(point cfg.Point) (callpayload.CallOutcome, bool) {
	if r == nil || r.registry == nil || r.callOutcome == nil {
		return callpayload.CallOutcome{}, false
	}
	if outcome, ok := r.queries.callOutcome(point); ok {
		return outcome, true
	}
	site, ok := r.facts.CallSiteView(point)
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
	outcome := r.callOutcome(ctx, site, in, r.boundaryRead)
	r.queries.rememberCallOutcome(point, outcome)
	return outcome, true
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
	if outcome, ok := r.CallOutcomeAt(point); ok {
		for _, result := range outcome.Results {
			if result.Index == resultIndex && !product.Equal(r.registry, result.Value, product.Bottom(r.registry)) {
				return result.Value, true
			}
		}
	}
	if value, ok := r.callExprSignatureResultValue(point, resultIndex); ok {
		return value, true
	}
	source, ok := factflow.NewCallValueSource(0, factflow.NoValueSourceIndex, factflow.NoValueSourceIndex, resultIndex, point, factflow.ValueSourceShape{})
	if !ok {
		return product.Value{}, false
	}
	return r.SourceValueAtBoundary(point, source)
}

func (r *Result) callExprSignatureResultValue(point cfg.Point, resultIndex int) (product.Value, bool) {
	site, ok := r.CallSite(point)
	if !ok {
		return product.Value{}, false
	}
	if fn, ok := r.CallSignatureType(site); ok {
		return r.functionReturnValue(fn, resultIndex)
	}
	if fn, ok := r.FunctionValueTypeForCallSiteAtBoundary(point, site); ok {
		return r.functionReturnValue(fn, resultIndex)
	}
	return product.Value{}, false
}

func (r *Result) functionReturnValue(fn *typ.Function, resultIndex int) (product.Value, bool) {
	if fn == nil || resultIndex < 0 || resultIndex >= len(fn.Returns) || fn.Returns[resultIndex] == nil {
		return product.Value{}, false
	}
	t := fn.Returns[resultIndex]
	return typevalue.WithWitness(r.registry, r.typeValues.FromType(r.registry, t), t), true
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

// CallExprPoint returns the CFG point for a syntactic call expression.
func (r *Result) CallExprPoint(call *ast.FuncCallExpr) (cfg.Point, bool) {
	return r.callExprPoint(call)
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
	if readableConcreteType(r.registry, r.typeValues, v) {
		return v, true
	}
	if declaration.Source.Kind == 0 {
		return product.Value{}, false
	}
	if recoveredValue, ok := r.sourceValueAtPoint(sourceValueReadExplanationBoundary, declaration.Point, declaration.Source, declState, r.boundaryRead); ok {
		if readableConcreteType(r.registry, r.typeValues, recoveredValue) {
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
