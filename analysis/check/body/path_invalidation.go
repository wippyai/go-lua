package body

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/type/subtype"
)

// PathInvalidatedBetween reports whether target loses path-stability after from
// and before to. The destination point is excluded because this query answers
// whether target survived up to that point; the start point is included because
// it may be the first point controlled by a proof edge. It considers descendant
// invalidation facts, lowered root/path assignments, and call invalidation
// summaries.
func (r *Result) PathInvalidatedBetween(from, to cfg.Point, target pathdom.Path) bool {
	if r == nil || target.IsEmpty() || from == to {
		return false
	}
	graph := r.Graph()
	if graph == nil {
		return false
	}
	for _, candidate := range graph.RPO() {
		if candidate == to {
			continue
		}
		if !r.PointCanReach(from, candidate) || !r.PointCanReach(candidate, to) {
			continue
		}
		if invalidation, ok := r.PathDescendantInvalidation(candidate); ok && target.HasStrictPrefix(invalidation.ContainerPath()) {
			return true
		}
		if r.assignmentInvalidatesMemberPathAt(candidate, target) {
			return true
		}
		if r.CallMayInvalidateGuardFact(candidate, target) {
			return true
		}
	}
	return false
}

// PathRefinementInvalidatedBetween reports whether target loses a proven value
// refinement between from and to. A root assignment that writes a value already
// satisfying the same refinement preserves the proof; every other invalidating
// write still kills it.
func (r *Result) PathRefinementInvalidatedBetween(from, to cfg.Point, target pathdom.Path, refinement product.Value) bool {
	if r == nil || target.IsEmpty() || from == to {
		return false
	}
	graph := r.Graph()
	if graph == nil {
		return false
	}
	for _, candidate := range graph.RPO() {
		if candidate == to {
			continue
		}
		if !r.PointCanReach(from, candidate) || !r.PointCanReach(candidate, to) {
			continue
		}
		if invalidation, ok := r.PathDescendantInvalidation(candidate); ok && target.HasStrictPrefix(invalidation.ContainerPath()) {
			return true
		}
		if r.assignmentInvalidatesMemberPathAt(candidate, target) {
			if r.rootAssignmentPreservesPathRefinementAt(candidate, target, refinement) {
				continue
			}
			return true
		}
		if r.CallMayInvalidateGuardFact(candidate, target) {
			return true
		}
	}
	return false
}

// PathPresenceInvalidatedBetween reports whether a proof that target was
// present is invalidated between from and to. Presence is weaker than full
// path-stability: mutating descendants may invalidate field/member facts, but
// it does not make the receiver path itself nil. This is used for required
// member-read proofs such as `ipairs(graph.items)` proving `graph` present in
// the loop body.
func (r *Result) PathPresenceInvalidatedBetween(from, to cfg.Point, target pathdom.Path) bool {
	if r == nil || target.IsEmpty() || from == to {
		return false
	}
	graph := r.Graph()
	if graph == nil {
		return false
	}
	for _, candidate := range graph.RPO() {
		if candidate == to {
			continue
		}
		if !r.PointCanReach(from, candidate) || !r.PointCanReach(candidate, to) {
			continue
		}
		if r.assignmentInvalidatesMemberPathAt(candidate, target) {
			return true
		}
		if r.CallMayInvalidatePathPresence(candidate, target) {
			return true
		}
	}
	return false
}

func (r *Result) assignmentInvalidatesMemberPathAt(point cfg.Point, target pathdom.Path) bool {
	if len(target.Segments) == 0 {
		return r.rootAssignmentTargetsPathRoot(point, target)
	}
	if r.rootAssignmentTargetsPathRoot(point, target) {
		return true
	}
	if fact, ok := r.PathAssignment(point); ok {
		assigned := fact.TargetPathRef()
		return pathHasPrefixStaticEquiv(target, assigned) ||
			r.PathsAliasWithSameSuffixAtBoundary(point, assigned, target)
	}
	return false
}

func (r *Result) rootAssignmentTargetsPathRoot(point cfg.Point, target pathdom.Path) bool {
	root, ok := r.RootAssignment(point)
	if !ok {
		return false
	}
	if root.Kind() != factflow.RootAssignmentOrdinaryRootWrite {
		return false
	}
	if root.TargetSymbol() != 0 && target.Symbol != 0 && root.TargetSymbol() == target.Symbol {
		return true
	}
	assigned := root.TargetPathRef()
	return !assigned.IsEmpty() && len(assigned.Segments) == 0 && pathHasPrefixStaticEquiv(target.RootOnly(), assigned)
}

func (r *Result) rootAssignmentPreservesPathRefinementAt(point cfg.Point, target pathdom.Path, refinement product.Value) bool {
	if r == nil || r.registry == nil || len(target.Segments) != 0 {
		return false
	}
	root, ok := r.RootAssignment(point)
	if !ok ||
		root.Kind() != factflow.RootAssignmentOrdinaryRootWrite ||
		root.TargetSymbol() == 0 ||
		root.TargetSymbol() != target.Symbol ||
		len(root.TargetPathRef().Segments) != 0 {
		return false
	}
	value, ok := r.SourceValueBeforeBoundary(point, root.Source())
	if !ok {
		return false
	}
	if product.LessOrEq(r.registry, value, refinement) {
		return true
	}
	valueType, valueTypeOK := typevalue.TypeOf(r.registry, value)
	refinementType, refinementTypeOK := typevalue.TypeOf(r.registry, refinement)
	return valueTypeOK && refinementTypeOK && subtype.IsSubtype(valueType, refinementType)
}

func pathHasPrefixStaticEquiv(candidate, prefix pathdom.Path) bool {
	if !candidate.SameRootIgnoringVersion(prefix) {
		return false
	}
	return address.SegmentsHasPrefix(candidate.Segments, prefix.Segments)
}

// CallMayInvalidatePathPresence reports whether a call can invalidate a
// presence proof for target. For root locals and params, ordinary open calls can
// mutate reachable objects but cannot rebind the caller's root variable; exact
// call summaries still win when they report a root rebinding.
func (r *Result) CallMayInvalidatePathPresence(point cfg.Point, target pathdom.Path) bool {
	if r == nil || target.IsEmpty() {
		return false
	}
	if len(target.Segments) != 0 {
		return r.CallMayInvalidateGuardFact(point, target)
	}
	site, hasSite := r.CallSite(point)
	if !hasSite {
		return false
	}
	outcome, hasOutcome := r.CallOutcomeAt(point)
	if !hasOutcome {
		if r.CallSiteHasExactEmptyGuardInvalidationSummary(site) {
			return false
		}
		if kind, ok := r.SymbolKind(target.Symbol); ok && (kind == symbol.Local || kind == symbol.Param) {
			return false
		}
		return r.CallMayInvalidateGuardFact(point, target)
	}
	if !r.CallOutcomeHasExactGuardInvalidationSummary(site, outcome, false) {
		if kind, ok := r.SymbolKind(target.Symbol); ok && (kind == symbol.Local || kind == symbol.Param) {
			return false
		}
		return r.CallMayInvalidateGuardFact(point, target)
	}
	if CallOutcomeHasGlobalGuardInvalidation(outcome) {
		return true
	}
	invalidated, ok := r.CallOutcomeGuardInvalidationPaths(site, outcome)
	if !ok {
		return true
	}
	for _, invalidation := range invalidated {
		if invalidation.RootRebinding && target.HasPrefix(invalidation.Path) {
			return true
		}
	}
	return false
}
