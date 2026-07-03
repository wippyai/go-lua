package body

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
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
