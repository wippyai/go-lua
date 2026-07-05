package body

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/symbol"
)

// PathInvalidatedBetween reports whether target loses path-stability after from
// and before to. The destination point is excluded because this query answers
// whether target survived up to that point; the start point is included because
// it may be the first point controlled by a proof edge. It considers descendant
// invalidation facts, ordinary root/member assignments, and call invalidation
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
		if fact, ok := r.OrdinaryAssignment(candidate); ok && r.ordinaryAssignmentInvalidatesMemberPathAt(candidate, fact, target) {
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
		if fact, ok := r.OrdinaryAssignment(candidate); ok && r.ordinaryAssignmentInvalidatesMemberPathAt(candidate, fact, target) {
			return true
		}
		if r.CallMayInvalidatePathPresence(candidate, target) {
			return true
		}
	}
	return false
}

func (r *Result) ordinaryAssignmentInvalidatesMemberPathAt(point cfg.Point, fact semantics.OrdinaryAssignmentFact, target pathdom.Path) bool {
	if fact.HasPath {
		return pathHasPrefixStaticEquiv(target, fact.Path) ||
			r.PathsAliasWithSameSuffixAtBoundary(point, fact.Path, target)
	}
	return fact.HasSymbol && target.Symbol != 0 && fact.Symbol == target.Symbol
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
