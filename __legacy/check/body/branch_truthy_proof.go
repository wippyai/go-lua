package body

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
)

// PathProvenTruthyByDominatingBranch reports whether lowered branch evidence
// proves p truthy before point and no invalidation on a path from the proof edge
// to point invalidates that path. It is the canonical post-solve query for
// consumers that need a guard-origin truthiness proof without rebuilding a
// diagnostics-local dominance scan.
func (r *Result) PathProvenTruthyByDominatingBranch(point cfg.Point, p pathdom.Path) bool {
	_, ok := r.DominatingTruthyBranchForPath(point, p)
	return ok
}

// DominatingTruthyBranchForPath returns the branch point whose active edge proves
// p truthy before point, with no invalidating write or call in between. It is the
// origin-bearing form of PathProvenTruthyByDominatingBranch for diagnostics and
// read models that need to attach evidence to the proving guard.
func (r *Result) DominatingTruthyBranchForPath(point cfg.Point, p pathdom.Path) (cfg.Point, bool) {
	if r == nil || p.IsEmpty() || point == 0 {
		return 0, false
	}
	graph := r.Graph()
	if graph == nil {
		return 0, false
	}
	for _, branch := range graph.RPO() {
		if branch == point {
			continue
		}
		if !r.branchHasTruthyEvidenceForPath(branch, p) {
			continue
		}
		successors := cfg.SuccessorsReadOnly(graph, branch)
		conditions := cfg.SuccessorConditionsReadOnly(graph, branch)
		if len(conditions) != len(successors) {
			continue
		}
		for index, succ := range successors {
			cond := conditions[index]
			if !r.branchTruthyEvidenceActiveOnEdge(branch, p, cond) {
				continue
			}
			if !r.proofEdgeDominatesPoint(graph, branch, succ, point) {
				continue
			}
			if !r.PathInvalidatedBetween(succ, point, p) {
				return branch, true
			}
		}
	}
	return 0, false
}

// DominatingBranchCheckForPath returns a branch point whose selected edge
// dominates point, carries a direct branch condition for p accepted by accepts,
// and has no invalidation of p before point.
func (r *Result) DominatingBranchCheckForPath(
	point cfg.Point,
	p pathdom.Path,
	accepts func(branch cfg.Point, check branchcond.Check, cond bool) bool,
) (cfg.Point, bool, bool) {
	if r == nil || p.IsEmpty() || point == 0 || accepts == nil {
		return 0, false, false
	}
	graph := r.Graph()
	if graph == nil {
		return 0, false, false
	}
	for _, branch := range graph.RPO() {
		if branch == point {
			continue
		}
		check, ok := r.BranchConditionCheck(branch)
		if !ok || !check.Path.Equal(p) {
			continue
		}
		successors := cfg.SuccessorsReadOnly(graph, branch)
		conditions := cfg.SuccessorConditionsReadOnly(graph, branch)
		if len(conditions) != len(successors) {
			continue
		}
		for index, succ := range successors {
			cond := conditions[index]
			if !accepts(branch, check, cond) {
				continue
			}
			if !r.proofEdgeDominatesPoint(graph, branch, succ, point) {
				continue
			}
			if !r.PathInvalidatedBetween(succ, point, p) {
				return branch, cond, true
			}
		}
	}
	return 0, false, false
}

// DominatingLiteralBranchForPath returns a direct branch proof that p has a
// concrete literal value before point, with no invalidation between the proof
// edge and point. It covers literal equality/inequality checks and boolean
// truthy/falsy guards; callers that use the boolean guard form should only do
// so for paths whose domain is known to be boolean-literal.
func (r *Result) DominatingLiteralBranchForPath(point cfg.Point, p pathdom.Path) (typ.Type, cfg.Point, bool) {
	if r == nil || p.IsEmpty() || point == 0 {
		return nil, 0, false
	}
	graph := r.Graph()
	if graph == nil {
		return nil, 0, false
	}
	for _, branch := range graph.RPO() {
		if branch == point {
			continue
		}
		check, ok := r.BranchConditionCheck(branch)
		if !ok || !r.branchCheckPathMatchesAt(point, check.Path, p) {
			continue
		}
		successors := cfg.SuccessorsReadOnly(graph, branch)
		conditions := cfg.SuccessorConditionsReadOnly(graph, branch)
		if len(conditions) != len(successors) {
			continue
		}
		for index, succ := range successors {
			cond := conditions[index]
			proven, ok := literalProofFromBranchCheck(check, cond)
			if !ok {
				continue
			}
			if !r.proofEdgeDominatesPoint(graph, branch, succ, point) {
				continue
			}
			if !r.PathInvalidatedBetween(succ, point, p) &&
				(check.Path.Equal(p) || !r.PathInvalidatedBetween(succ, point, check.Path)) {
				return proven, branch, true
			}
		}
	}
	return nil, 0, false
}

// DominatingBranchProvesLiteral reports whether a direct dominating guard proves
// p equals lit at point.
func (r *Result) DominatingBranchProvesLiteral(point cfg.Point, p pathdom.Path, lit typ.Type) bool {
	if lit == nil {
		return false
	}
	proven, _, ok := r.DominatingLiteralBranchForPath(point, p)
	return ok && typ.TypeEquals(proven, lit)
}

func literalProofFromBranchCheck(check branchcond.Check, cond bool) (typ.Type, bool) {
	switch check.Kind {
	case branchcond.CheckLiteralEqual:
		if lit, ok := check.LiteralValue(); ok && cond {
			return lit, true
		}
	case branchcond.CheckLiteralNot:
		if lit, ok := check.LiteralValue(); ok && !cond {
			return lit, true
		}
	case branchcond.CheckTruthy:
		if cond {
			return typ.True, true
		}
	case branchcond.CheckFalsy:
		if cond {
			return typ.False, true
		}
	}
	return nil, false
}

func (r *Result) branchCheckPathMatchesAt(point cfg.Point, checkPath, target pathdom.Path) bool {
	return checkPath.Equal(target) || r.PathsAliasWithSameSuffixAtBoundary(point, checkPath, target)
}

func (r *Result) proofEdgeDominatesPoint(graph cfg.Graph, branch, succ, point cfg.Point) bool {
	if !r.PointDominates(succ, point) {
		return false
	}
	for _, pred := range cfg.PredecessorsReadOnly(graph, succ) {
		if pred == branch {
			continue
		}
		if !r.PointDominates(succ, pred) {
			return false
		}
	}
	return true
}

func (r *Result) branchHasTruthyEvidenceForPath(point cfg.Point, p pathdom.Path) bool {
	found := false
	r.facts.ForEachBranchPathEvidence(point, func(proof factflow.BranchPathEvidence) bool {
		if proof.Kind() == factflow.BranchPathEvidenceTruthy && proof.PathRef().Equal(p) {
			found = true
			return false
		}
		return true
	})
	return found
}

func (r *Result) branchTruthyEvidenceActiveOnEdge(point cfg.Point, p pathdom.Path, cond bool) bool {
	found := false
	r.facts.ForEachBranchPathEvidence(point, func(proof factflow.BranchPathEvidence) bool {
		if proof.Kind() == factflow.BranchPathEvidenceTruthy && proof.PathRef().Equal(p) && proof.ActiveOnEdge(cond) {
			found = true
			return false
		}
		return true
	})
	return found
}
