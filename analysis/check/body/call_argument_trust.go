package body

import (
	checkprojection "github.com/wippyai/go-lua/analysis/check/internal/projection"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
	"github.com/wippyai/go-lua/analysis/type/refinement"
	"github.com/wippyai/go-lua/analysis/type/subtype"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

// RootPathHasTrustedDominatingAssignmentSource reports whether p's root has a
// dominating assignment whose source carries a readable, trusted boundary value.
// Call-argument projections use this as body-owned proof that a boundary value
// is not merely a top/unknown fallback.
func (r *Result) RootPathHasTrustedDominatingAssignmentSource(point cfg.Point, p pathdom.Path) bool {
	if r == nil || p.IsEmpty() || p.Symbol == 0 || len(p.Segments) != 0 || r.Graph() == nil {
		return false
	}
	var source factflow.ValueSource
	var sourcePoint cfg.Point
	found := false
	for _, candidate := range r.Graph().RPO() {
		if candidate == point || !r.PointDominates(candidate, point) {
			continue
		}
		candidateSource, ok := r.rootPathAssignmentSourceAt(candidate, p)
		if !ok {
			continue
		}
		if !found || r.PointDominates(sourcePoint, candidate) {
			source = candidateSource
			sourcePoint = candidate
			found = true
		}
	}
	if !found {
		return false
	}
	value, ok := r.SourceValueBeforeBoundary(sourcePoint, source)
	if !ok {
		value, ok = r.SourceValueAtBoundary(sourcePoint, source)
	}
	return ok && r.callArgumentValueHasReadableNonNilType(value) && !r.ValueHasUntrustedTopOrigin(value)
}

func (r *Result) rootPathAssignmentSourceAt(point cfg.Point, p pathdom.Path) (factflow.ValueSource, bool) {
	if local, ok := r.LoweredLocalAssignment(point); ok && local.TargetSymbol() == p.Symbol && len(local.TargetPathRef().Segments) == 0 {
		return local.Source(), true
	}
	if root, ok := r.RootAssignment(point); ok &&
		root.Kind() == factflow.RootAssignmentOrdinaryRootWrite &&
		root.TargetSymbol() == p.Symbol &&
		len(root.TargetPathRef().Segments) == 0 {
		return root.Source(), true
	}
	return factflow.ValueSource{}, false
}

// RootPathHasTrustedNumericForVariable reports whether p's root is a numeric
// for-loop variable whose visible value at point has a readable type witness.
func (r *Result) RootPathHasTrustedNumericForVariable(point cfg.Point, p pathdom.Path) bool {
	if r == nil || p.IsEmpty() || p.Symbol == 0 || len(p.Segments) != 0 || r.Graph() == nil {
		return false
	}
	for _, candidate := range r.Graph().RPO() {
		fact, ok := r.NumericFor(candidate)
		if !ok || !fact.HasSymbol || fact.Symbol != p.Symbol {
			continue
		}
		value, ok := r.PathValueBeforeBoundary(point, p)
		return ok && r.callArgumentValueHasReadableType(value)
	}
	return false
}

// RootPathHasTrustedGenericForVariable reports whether p's root is a generic
// for-loop variable dominated by point with a trusted boundary value.
func (r *Result) RootPathHasTrustedGenericForVariable(point cfg.Point, p pathdom.Path) bool {
	if r == nil || p.IsEmpty() || p.Symbol == 0 || len(p.Segments) != 0 || r.Graph() == nil {
		return false
	}
	for _, candidate := range r.Graph().RPO() {
		if candidate == point || !r.PointDominates(candidate, point) {
			continue
		}
		fact, ok := r.GenericFor(candidate)
		if !ok || fact.Role != GenericForRoleVariable || !fact.HasSymbols {
			continue
		}
		for _, sym := range fact.Symbols {
			if sym != p.Symbol {
				continue
			}
			value, ok := r.PathValueAtBoundary(point, p)
			return ok && r.callArgumentValueHasReadableType(value) && !r.ValueHasUntrustedTopOrigin(value)
		}
	}
	return false
}

// RootPathHasTrustedEntryValue reports whether p's root was trusted at function
// entry. Entry reads are body-owned solved-state reads, not readmodel logic.
func (r *Result) RootPathHasTrustedEntryValue(p pathdom.Path) bool {
	if r == nil || p.IsEmpty() || p.Symbol == 0 || len(p.Segments) != 0 {
		return false
	}
	entry, ok := r.EntryState()
	if !ok {
		return false
	}
	value := entry.ReadValue(r.Registry(), statekey.SymbolValue(p.Symbol))
	return r.callArgumentValueHasReadableType(value) && !r.ValueHasUntrustedTopOrigin(value)
}

// RootPathHasDominatingRuntimeValidationAssignment reports whether a dominating
// declaration or root assignment validates p at runtime. Declared any/unknown
// roots remain explicit-top boundaries and are not treated as validation proof.
func (r *Result) RootPathHasDominatingRuntimeValidationAssignment(point cfg.Point, p pathdom.Path) bool {
	if r == nil || p.IsEmpty() || p.Symbol == 0 || len(p.Segments) != 0 || r.Graph() == nil {
		return false
	}
	if r.rootPathHasDeclaredTop(p) {
		return false
	}
	if declaration, ok := r.DominatingPathRootDeclarationSource(point, p); ok {
		if !declaration.Source.Adjusted && r.SourceHasRuntimeValidation(declaration.Source) {
			return true
		}
	}
	for _, candidate := range r.Graph().RPO() {
		if candidate == point || !r.PointDominates(candidate, point) {
			continue
		}
		source, ok := r.rootPathAssignmentSourceAt(candidate, p)
		if !ok {
			continue
		}
		if !source.Adjusted && r.SourceHasRuntimeValidation(source) {
			return true
		}
	}
	return false
}

func (r *Result) rootPathHasDeclaredTop(p pathdom.Path) bool {
	if r == nil || p.IsEmpty() || len(p.Segments) != 0 || p.Symbol == 0 {
		return false
	}
	typeExpr, ok := r.SymbolTypeAnnotation(p.Symbol)
	if !ok || typeExpr == nil || r.TypeResolver() == nil {
		return false
	}
	declared, ok := r.TypeResolver().Type(typeExpr)
	base := unwrap.Optional(declared)
	return ok && base != nil && (typ.IsAny(base) || typ.IsUnknown(base))
}

// ExplicitTopDeclaredPathType returns the declared any/unknown type projected
// to p. It is used when an explicit gradual annotation is the authority for an
// otherwise sharper value; callers must keep that untrusted boundary visible.
func (r *Result) ExplicitTopDeclaredPathType(p pathdom.Path) (typ.Type, bool) {
	if r == nil || p.IsEmpty() || p.Symbol == 0 {
		return nil, false
	}
	typeExpr, ok := r.SymbolTypeAnnotation(p.Symbol)
	if !ok || typeExpr == nil || r.TypeResolver() == nil {
		return nil, false
	}
	root, ok := r.TypeResolver().Type(typeExpr)
	base := unwrap.Optional(root)
	if !ok || base == nil || (!typ.IsAny(base) && !typ.IsUnknown(base)) {
		return nil, false
	}
	if len(p.Segments) == 0 {
		return root, true
	}
	return checkprojection.DeclaredPathType(root, p)
}

// RootPathArgumentUsesBoundary reports whether p has any body-owned proof that
// call-argument projection should prefer a boundary value over the raw
// pre-call/source value.
func (r *Result) RootPathArgumentUsesBoundary(point cfg.Point, p pathdom.Path) bool {
	if r == nil || p.IsEmpty() || len(p.Segments) != 0 {
		return false
	}
	if declared, ok := r.DeclaredPathTypeAt(point, p, true); ok && declared != nil {
		return true
	}
	if _, _, ok := r.DominatingBranchCheckForPath(point, p, func(_ cfg.Point, check branchcond.Check, _ bool) bool {
		return check.Kind != branchcond.CheckNone
	}); ok {
		return true
	}
	if _, ok := r.DominatingTruthyBranchForPath(point, p); ok {
		return true
	}
	if value, ok := r.DominatingBranchRefinementValueForPath(point, p); ok && r.callArgumentValueHasReadableType(value) {
		return true
	}
	return r.RootPathHasTrustedDominatingAssignmentSource(point, p) ||
		r.rootPathHasTrustedDeclarationSource(point, p) ||
		r.RootPathHasTrustedNumericForVariable(point, p) ||
		r.rootPathHasTrustedCurrentValue(point, p) ||
		r.RootPathHasTrustedEntryValue(p) ||
		r.RootPathHasTrustedGenericForVariable(point, p)
}

// CallerOwnedParameterSource reports whether a call argument source is derived
// from a caller-owned parameter. The proof may follow expression operations,
// dynamic-index table sources, and dominating root declarations, so it lives on
// body.Result instead of readmodel projection code.
func (r *Result) CallerOwnedParameterSource(point cfg.Point, source factflow.ValueSource) bool {
	return r.callerOwnedParameterSource(point, source, nil, 0)
}

// callerOwnedParameterGuard is the recursion state shared by
// callerOwnedParameterSource and callerOwnedParameterDeclarationSource.
//
// Expression-operand traversal (operator Left/Right, dynamic-index table
// source) cycles on ExprRef alone: ExpressionOperationRef and
// DynamicIndexExpressionRef are keyed by expression identity, not by the
// query point, so a repeated ExprRef always re-explores the same subtree.
//
// Declaration-source lookup cycles on the (point, path) pair it is re-entered
// with. DominatingPathRootDeclarationSource is a pure function of that pair
// for one Result (same dominator chain, same facts), so once a (point, path)
// pair repeats, the rest of the expansion is identical forever. This is the
// axis the ExprRef-only guard could not see: hopping through a dominating
// declaration changes the query point and swaps in a different source
// without ever registering the original ExprRef, so a cycle between the two
// functions never revisited a guarded ExprRef.
type callerOwnedParameterGuard struct {
	expr map[factflow.ExprRef]struct{}
	decl map[callerOwnedDeclarationVisit]struct{}
}

// callerOwnedDeclarationVisit identifies one declaration-source lookup. path
// is a comparable, allocation-free structural key (see keyspace.Key) so
// repeated visits to the same (point, path) pair cost a plain map probe.
type callerOwnedDeclarationVisit struct {
	point cfg.Point
	path  keyspace.Key
}

func newCallerOwnedDeclarationVisit(ks *keyspace.KeySpace, point cfg.Point, p pathdom.Path) (callerOwnedDeclarationVisit, bool) {
	if ks == nil || !ks.Valid() || p.IsEmpty() {
		return callerOwnedDeclarationVisit{}, false
	}
	key := ks.FromPath(p)
	if key.Kind == keyspace.KindInvalid {
		return callerOwnedDeclarationVisit{}, false
	}
	return callerOwnedDeclarationVisit{point: point, path: key}, true
}

// CallerOwnedRootParameterContract returns the exact finalized entry contract
// for a caller-owned root parameter in the canonical lexical application. A
// mismatching caller value is diagnosed on that incoming edge; body uses may
// therefore assume this contract instead of treating the incompatible meet as
// an any/unknown value. Annotated/unowned parameters and non-root paths cannot
// obtain this authority.
func (r *Result) CallerOwnedRootParameterContract(p pathdom.Path) (product.Value, bool) {
	if r == nil || p.IsEmpty() || p.Symbol == 0 || len(p.Segments) != 0 {
		return product.Value{}, false
	}
	if !r.callerOwnedParameterPath(p) {
		return product.Value{}, false
	}
	plan := r.operationPlan
	if plan == nil || !plan.BoundaryParamsValid() {
		return product.Value{}, false
	}
	params := plan.BoundaryParams()
	contracts := plan.BoundaryParamContracts()
	if len(params) != len(contracts) {
		return product.Value{}, false
	}
	for index, param := range params {
		if param != p.Symbol {
			continue
		}
		contract := contracts[index]
		if !r.callArgumentValueHasReadableType(contract) || r.ValueHasUntrustedTopOrigin(contract) {
			return product.Value{}, false
		}
		return contract, true
	}
	return product.Value{}, false
}

func (r *Result) callerOwnedParameterSource(point cfg.Point, source factflow.ValueSource, active *callerOwnedParameterGuard, depth int) bool {
	if r == nil {
		return false
	}
	if active == nil {
		active = &callerOwnedParameterGuard{}
	}
	if p, ok := r.callArgumentSourcePath(source); ok {
		if r.callerOwnedParameterPath(p) {
			return true
		}
		if r.callerOwnedParameterDeclarationSource(point, p, active, depth+1) {
			return true
		}
	}
	if !source.HasExpr || source.ExprRef == 0 {
		return false
	}
	if active.expr == nil {
		active.expr = make(map[factflow.ExprRef]struct{}, 1)
	}
	if _, seen := active.expr[source.ExprRef]; seen {
		return false
	}
	active.expr[source.ExprRef] = struct{}{}
	op, ok := r.ExpressionOperationRef(source.ExprRef)
	if ok {
		if r.callerOwnedParameterSource(point, op.Left(), active, depth+1) {
			return true
		}
		if op.Kind() == factflow.ExpressionOperationBinary && r.callerOwnedParameterSource(point, op.Right(), active, depth+1) {
			return true
		}
	}
	if dyn, ok := r.DynamicIndexExpressionRef(source.ExprRef); ok {
		if tableSource, ok := dyn.TableSource(); ok && r.callerOwnedParameterSource(point, tableSource, active, depth+1) {
			return true
		}
		if tablePath := dyn.TablePathRef(); !tablePath.IsEmpty() {
			if r.callerOwnedParameterPath(tablePath) || r.callerOwnedParameterDeclarationSource(point, tablePath, active, depth+1) {
				return true
			}
		}
	}
	return false
}

func (r *Result) callArgumentSourcePath(source factflow.ValueSource) (pathdom.Path, bool) {
	p, ok := r.ValueSourcePath(source)
	if ok && !p.IsEmpty() {
		return p, true
	}
	if source.Kind == factflow.ValueSourceExpression && source.HasExpr {
		return r.ExpressionRefPath(source.ExprRef)
	}
	return pathdom.Path{}, false
}

func (r *Result) callerOwnedParameterDeclarationSource(point cfg.Point, p pathdom.Path, active *callerOwnedParameterGuard, depth int) bool {
	if p.IsEmpty() || p.Symbol == 0 || point == 0 || r == nil || r.Graph() == nil {
		return false
	}
	if active == nil {
		active = &callerOwnedParameterGuard{}
	}
	if visit, ok := newCallerOwnedDeclarationVisit(r.KeySpace(), point, p); ok {
		if active.decl == nil {
			active.decl = make(map[callerOwnedDeclarationVisit]struct{}, 1)
		}
		if _, seen := active.decl[visit]; seen {
			return false
		}
		active.decl[visit] = struct{}{}
	}
	declaration, ok := r.DominatingPathRootDeclarationSource(point, p)
	if !ok || !declaration.Source.HasExpr {
		return false
	}
	return r.callerOwnedParameterSource(declaration.Point, declaration.Source, active, depth+1)
}

func (r *Result) callerOwnedParameterPath(p pathdom.Path) bool {
	if p.Symbol == 0 || r == nil {
		return false
	}
	fn := r.Function()
	if fn == nil {
		return false
	}
	for _, slot := range r.FunctionParamSlots(fn) {
		if slot.Symbol != p.Symbol {
			continue
		}
		if !r.SymbolHasTypeAnnotation(slot.Symbol) {
			return true
		}
		t, ok := r.SymbolDeclaredType(slot.Symbol)
		return ok && obligationTypeContainsFreeTypeParam(t)
	}
	return false
}

// PathHasRuntimeProof reports whether p has body-owned runtime/type evidence
// strong enough to let a call-argument boundary value replace an explicit-top
// declaration.
func (r *Result) PathHasRuntimeProof(point cfg.Point, p pathdom.Path) bool {
	if r == nil || p.IsEmpty() {
		return false
	}
	if r.PathHasRuntimeTypeGuard(point, p) {
		return true
	}
	if value, ok := r.DominatingBranchRefinementValueForPath(point, p); ok && r.callArgumentValueHasReadableType(value) {
		return true
	}
	if value, ok := r.PathValueAtBoundary(point, p); ok && r.ValueHasRuntimeValidationProof(value) {
		return true
	}
	return false
}

func (r *Result) PathHasPositiveRuntimeTypeGuard(point cfg.Point, p pathdom.Path) bool {
	if r == nil || p.IsEmpty() {
		return false
	}
	_, _, ok := r.DominatingBranchCheckForPath(point, p, func(_ cfg.Point, check branchcond.Check, edge bool) bool {
		return (check.Kind == branchcond.CheckTypeEqual && edge) ||
			(check.Kind == branchcond.CheckTypeNot && !edge)
	})
	return ok
}

func (r *Result) PathHasRuntimeTypeGuard(point cfg.Point, p pathdom.Path) bool {
	if r == nil || p.IsEmpty() {
		return false
	}
	_, _, ok := r.DominatingBranchCheckForPath(point, p, func(_ cfg.Point, check branchcond.Check, _ bool) bool {
		return check.Kind == branchcond.CheckTypeEqual || check.Kind == branchcond.CheckTypeNot
	})
	return ok
}

func (r *Result) rootPathHasTrustedDeclarationSource(point cfg.Point, p pathdom.Path) bool {
	if r == nil || p.IsEmpty() || len(p.Segments) != 0 {
		return false
	}
	declaration, ok := r.DominatingPathRootDeclarationSource(point, p)
	if !ok {
		return false
	}
	value, ok := r.SourceValueAtBoundary(declaration.Point, declaration.Source)
	return ok && r.callArgumentValueHasReadableType(value) && !r.ValueHasUntrustedTopOrigin(value)
}

func (r *Result) rootPathHasTrustedCurrentValue(point cfg.Point, p pathdom.Path) bool {
	if r == nil || p.IsEmpty() || len(p.Segments) != 0 {
		return false
	}
	if value, ok := r.PathValueBeforeBoundary(point, p); ok &&
		r.callArgumentValueHasReadableType(value) && !r.ValueHasUntrustedTopOrigin(value) {
		return true
	}
	if value, ok := r.PathValueAtBoundary(point, p); ok &&
		r.callArgumentValueHasReadableType(value) && !r.ValueHasUntrustedTopOrigin(value) {
		return true
	}
	return false
}

func (r *Result) callArgumentValueHasReadableType(value product.Value) bool {
	t, ok := r.ValueType(value)
	return ok && t != nil && !typ.IsAny(t) && !typ.IsUnknown(t) && !typ.IsNever(t)
}

func (r *Result) callArgumentValueHasReadableNonNilType(value product.Value) bool {
	t, ok := r.ValueTypeWithPresence(value)
	return ok && t != nil && !typ.IsAny(t) && !typ.IsUnknown(t) && !typ.IsNever(t) && !typevalue.TypeIncludesNil(t)
}

// CallArgumentFunctionTypeAdmissible reports whether a contextual function
// argument can satisfy the expected parameter type. It keeps subtype ownership
// inside the solved body instead of exposing a callback to readmodel planners.
func (r *Result) CallArgumentFunctionTypeAdmissible(fn *typ.Function, expected typ.Type) bool {
	return r != nil && fn != nil && expected != nil && r.IsSubtype(fn, expected)
}

// CallArgumentSolvedTypeProvenMismatch reports whether a trusted solved
// argument type contradicts the expected parameter type.
func (r *Result) CallArgumentSolvedTypeProvenMismatch(actual, expected typ.Type, untrustedTopOrigin bool) bool {
	if r == nil || untrustedTopOrigin || actual == nil || expected == nil {
		return false
	}
	if typ.IsAny(actual) ||
		typ.IsUnknown(actual) ||
		typ.IsNever(actual) ||
		typ.IsAny(expected) ||
		typ.IsUnknown(expected) {
		return false
	}
	return !r.IsSubtype(actual, expected)
}

// CallArgumentFunctionTypeProvenMismatch reports whether a contextual function
// argument concretely rejects the expected parameter type.
func (r *Result) CallArgumentFunctionTypeProvenMismatch(fn *typ.Function, expected typ.Type) bool {
	if r == nil || fn == nil || expected == nil || typ.IsAny(expected) || typ.IsUnknown(expected) {
		return false
	}
	return !r.IsSubtype(fn, expected)
}

// InterfaceMismatchKind classifies why a record value fails an interface
// contract.
type InterfaceMismatchKind uint8

const (
	InterfaceMismatchMissingMethod InterfaceMismatchKind = iota + 1
	InterfaceMismatchMethodType
)

// InterfaceMismatch explains the first direct record-to-interface structural
// mismatch for readmodels. Body owns this proof decision so presentation layers
// do not call subtype internals directly.
type InterfaceMismatch struct {
	Kind       InterfaceMismatchKind
	MethodName string
	Actual     typ.Type
	Expected   typ.Type
}

// RecordInterfaceMismatch explains the first direct record-to-interface
// mismatch, if the given types have that shape.
func (r *Result) RecordInterfaceMismatch(actual, expected typ.Type) (InterfaceMismatch, bool) {
	if r == nil {
		return InterfaceMismatch{}, false
	}
	mismatch, ok := subtype.RecordInterfaceMismatch(actual, expected)
	if !ok {
		return InterfaceMismatch{}, false
	}
	out := InterfaceMismatch{
		MethodName: mismatch.Method.Name,
		Actual:     mismatch.Actual,
		Expected:   mismatch.Expected,
	}
	switch mismatch.Kind {
	case subtype.InterfaceMismatchMissingMethod:
		out.Kind = InterfaceMismatchMissingMethod
	case subtype.InterfaceMismatchMethodType:
		out.Kind = InterfaceMismatchMethodType
	default:
		return InterfaceMismatch{}, false
	}
	return out, true
}

// CallArgumentNilabilityOnlyRefinement reports whether two projected types
// differ only by nilability. It is body-owned because it relies on the canonical
// proof-domain non-nil projection and subtype relation.
func (r *Result) CallArgumentNilabilityOnlyRefinement(beforeType, boundaryType typ.Type) bool {
	beforeNonNil := ProjectionWithoutNil(beforeType)
	boundaryNonNil := ProjectionWithoutNil(boundaryType)
	if typ.TypeEquals(beforeNonNil, boundaryNonNil) {
		return true
	}
	if typ.IsAny(beforeNonNil) || typ.IsUnknown(beforeNonNil) || typ.IsNever(beforeNonNil) {
		return false
	}
	return r.IsSubtype(boundaryNonNil, beforeNonNil) && r.IsSubtype(beforeNonNil, boundaryNonNil)
}

func (r *Result) callArgumentPresenceOnlyRefinement(before, boundary product.Value) bool {
	beforeType, beforeOK := r.ValueTypeWithPresence(before)
	boundaryType, boundaryOK := r.ValueTypeWithPresence(boundary)
	if !beforeOK || !boundaryOK {
		return true
	}
	return r.CallArgumentNilabilityOnlyRefinement(beforeType, boundaryType)
}

// CallArgumentRuntimeValidationCanAdoptBoundary reports whether a
// runtime-validated source can safely use its boundary value instead of the
// pre-boundary value.
func (r *Result) CallArgumentRuntimeValidationCanAdoptBoundary(before, boundary product.Value) bool {
	beforeType, beforeOK := r.ValueTypeWithPresence(before)
	boundaryType, boundaryOK := r.ValueTypeWithPresence(boundary)
	if !beforeOK || !boundaryOK {
		return true
	}
	if typ.TypeEquals(beforeType, boundaryType) {
		return true
	}
	if r.CallArgumentNilabilityOnlyRefinement(beforeType, boundaryType) {
		return true
	}
	beforeNonNil := ProjectionWithoutNil(beforeType)
	return typ.IsAny(beforeNonNil) || typ.IsUnknown(beforeNonNil) || typ.IsNever(beforeNonNil)
}

// CallArgumentBoundaryRefinementAccepted reports whether a boundary value may
// replace a pre-boundary value, given whether the source/path carries explicit
// runtime or type proof. The caller owns collecting the proof bit; body owns the
// semantic classification.
func (r *Result) CallArgumentBoundaryRefinementAccepted(before, boundary product.Value, hasProof bool) bool {
	if r.CallArgumentBoundaryCanRefine(before, boundary) {
		if r.CallArgumentBoundaryNarrowsOnlyNilability(before, boundary) ||
			r.CallArgumentBoundaryConcretizesTop(before, boundary) {
			return hasProof
		}
		return true
	}
	return hasProof && r.CallArgumentBoundaryConcretizesTop(before, boundary)
}

func (r *Result) CallArgumentBoundaryNarrowsOnlyNilability(before, boundary product.Value) bool {
	beforeType, beforeOK := r.ValueTypeWithPresence(before)
	boundaryType, boundaryOK := r.ValueTypeWithPresence(boundary)
	return beforeOK && boundaryOK &&
		TypeMayBeNilMismatch(beforeType, boundaryType) &&
		r.CallArgumentNilabilityOnlyRefinement(beforeType, boundaryType)
}

func (r *Result) CallArgumentBoundaryConcretizesTop(before, boundary product.Value) bool {
	beforeType, beforeOK := r.ValueTypeWithPresence(before)
	boundaryType, boundaryOK := r.ValueTypeWithPresence(boundary)
	if !beforeOK || !boundaryOK || typ.TypeEquals(beforeType, boundaryType) {
		return false
	}
	beforeNonNil := ProjectionWithoutNil(beforeType)
	boundaryNonNil := ProjectionWithoutNil(boundaryType)
	if !typ.IsAny(beforeNonNil) && !typ.IsUnknown(beforeNonNil) && !typ.IsNever(beforeNonNil) {
		return false
	}
	return !typ.IsAny(boundaryNonNil) && !typ.IsUnknown(boundaryNonNil) && !typ.IsNever(boundaryNonNil)
}

func (r *Result) CallArgumentBoundaryCanRefine(before, boundary product.Value) bool {
	if presence.Equal(product.PresenceOf(boundary), presence.Present()) &&
		!presence.Equal(product.PresenceOf(before), presence.Present()) {
		if r.callArgumentPresenceOnlyRefinement(before, boundary) {
			return true
		}
		return r.callArgumentGenericBoundaryProofCanRefine(before, boundary)
	}
	beforeType, beforeTypeOK := r.ValueTypeWithPresence(before)
	boundaryType, boundaryTypeOK := r.ValueTypeWithPresence(boundary)
	if beforeTypeOK && boundaryTypeOK && typ.TypeEquals(beforeType, boundaryType) {
		return true
	}
	if beforeTypeOK && boundaryTypeOK && TypeMayBeNilMismatch(beforeType, boundaryType) {
		if r.CallArgumentNilabilityOnlyRefinement(beforeType, boundaryType) {
			return true
		}
		if r.callArgumentGenericBoundaryProofCanRefine(before, boundary) {
			return true
		}
	}
	if beforeTypeOK && (typ.IsAny(beforeType) || typ.IsUnknown(beforeType) || typ.IsNever(beforeType)) {
		return true
	}
	if beforeTypeOK && boundaryTypeOK &&
		r.IsSubtype(boundaryType, beforeType) &&
		r.ValueProofAdmissible(boundary, boundaryType) {
		return true
	}
	if beforeTypeOK && boundaryTypeOK &&
		obligationTypeContainsFreeTypeParam(beforeType) &&
		r.ValueProofAdmissible(boundary, boundaryType) {
		return true
	}
	return false
}

func (r *Result) callArgumentGenericBoundaryProofCanRefine(before, boundary product.Value) bool {
	beforeType, beforeTypeOK := r.ValueTypeWithPresence(before)
	boundaryType, boundaryTypeOK := r.ValueTypeWithPresence(boundary)
	return beforeTypeOK && boundaryTypeOK &&
		obligationTypeContainsFreeTypeParam(beforeType) &&
		r.ValueProofAdmissible(boundary, boundaryType)
}

func obligationTypeContainsFreeTypeParam(t typ.Type) bool {
	if refinement.ContainsFreeTypeParam(t) {
		return true
	}
	nonNil := ProjectionWithoutNil(t)
	return nonNil != nil && !typ.TypeEquals(nonNil, t) && refinement.ContainsFreeTypeParam(nonNil)
}

// TypeMayBeNilMismatch reports whether got admits nil while want rejects nil.
func TypeMayBeNilMismatch(got, want typ.Type) bool {
	return got != nil && want != nil && typevalue.TypeIncludesNil(got) && !typevalue.TypeIncludesNil(want)
}
