package factapply

import (
	"sort"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// BranchAlgebra is a borrowed, immutable view of the canonical factflow
// branch vocabulary at one CFG point. It is shared by concrete edge execution
// and symbolic guard lowering so edge polarity and sidecar ownership have one
// semantic source. It does not apply or reinterpret refinements.
type BranchAlgebra struct {
	facts factflow.Facts
	point cfg.Point
}

func NewBranchAlgebra(facts factflow.Facts, point cfg.Point) BranchAlgebra {
	return BranchAlgebra{facts: facts, point: point}
}

func (a BranchAlgebra) Point() cfg.Point { return a.point }

func (a BranchAlgebra) ConditionSource() (factflow.ValueSource, bool) {
	return a.facts.BranchConditionSource(a.point)
}

func (a BranchAlgebra) Refinements() []factflow.BranchRefinement {
	return a.facts.BranchRefinements(a.point)
}

// ActiveBranchRefinement is one edge-selected refinement. TargetPathRef and
// Refinement borrow immutable fact payload; callers must not retain or mutate
// the path.
type ActiveBranchRefinement struct {
	targetPath pathdom.Path
	refinement factflow.ValueRefinement
}

func (r ActiveBranchRefinement) TargetPathRef() pathdom.Path          { return r.targetPath }
func (r ActiveBranchRefinement) Refinement() factflow.ValueRefinement { return r.refinement }

// ActiveRefinements centralizes edge polarity and the shallow-before-deep
// application order required by the concrete engine.
func (a BranchAlgebra) ActiveRefinements(cond bool) []ActiveBranchRefinement {
	refinements := a.Refinements()
	if len(refinements) == 0 {
		return nil
	}
	out := make([]ActiveBranchRefinement, 0, len(refinements))
	for _, fact := range refinements {
		refinement, ok := fact.ValueForEdge(cond)
		if !ok {
			continue
		}
		out = append(out, ActiveBranchRefinement{targetPath: fact.TargetPathRef(), refinement: refinement})
	}
	sort.SliceStable(out, func(i, j int) bool {
		return len(out[i].targetPath.Segments) < len(out[j].targetPath.Segments)
	})
	return out
}

func (a BranchAlgebra) PathRelations() []factflow.BranchPathRelation {
	return a.facts.BranchPathRelations(a.point)
}

func (a BranchAlgebra) ActivePathRelations(cond bool) []factflow.BranchPathRelation {
	relations := a.PathRelations()
	out := relations[:0]
	for _, relation := range relations {
		if relation.ActiveOnEdge(cond) {
			out = append(out, relation)
		}
	}
	return out
}

func (a BranchAlgebra) ForEachPathEvidence(fn func(factflow.BranchPathEvidence) bool) {
	a.facts.ForEachBranchPathEvidence(a.point, fn)
}

func (a BranchAlgebra) SufficientLiteralCases() []factflow.BranchSufficientLiteralCase {
	return a.facts.BranchSufficientLiteralCases(a.point)
}

// GuardOnly reports whether this point has no edge effect beyond selecting a
// truthy/falsy condition. It is the initial exact symbolic slice. Every family
// with persistent or refinement meaning remains a blocker until represented.
func (a BranchAlgebra) GuardOnly() (bool, string) {
	reasons := a.GuardOnlyBlockers()
	if len(reasons) != 0 {
		return false, reasons[0]
	}
	return true, ""
}

// GuardOnlyBlockers returns every unrepresented branch family in canonical
// concrete application order. Capability censuses can therefore measure the
// next slice without changing execution or hiding secondary blockers.
func (a BranchAlgebra) GuardOnlyBlockers() []string {
	var out []string
	if _, ok := a.ConditionSource(); !ok {
		out = append(out, "branch:missing-condition-source")
	}
	if len(a.Refinements()) != 0 {
		out = append(out, "branch:refinement")
	}
	if len(a.PathRelations()) != 0 {
		out = append(out, "branch:path-relation")
	}
	hasEvidence := false
	a.ForEachPathEvidence(func(factflow.BranchPathEvidence) bool {
		hasEvidence = true
		return false
	})
	if hasEvidence {
		out = append(out, "branch:path-evidence")
	}
	if len(a.SufficientLiteralCases()) != 0 {
		out = append(out, "branch:sufficient-literal-case")
	}
	return out
}
