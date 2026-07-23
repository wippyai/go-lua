package transferfacts

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
)

// branchDisjunctiveRefinementsFromWIR lowers the per-arm sufficiency groups
// wirlower records on an OpBranch (SufficientCheckArmsTrue for a top-level
// `or`, SufficientCheckArmsFalse for a top-level `and`) into branch
// refinements. Each arm proves its own leaf checks when it alone forced the
// edge, but the edge does not reveal which arm actually held. The sound
// conclusion for a path is therefore the join of every arm's own conclusion
// about that path, and only when every arm has one: an arm silent about a
// path, or an arm with no recognized leaf check at all, means the edge could
// have been forced by something that says nothing about that path, so no
// conclusion follows.
func (l *lowerer) branchDisjunctiveRefinementsFromWIR(point cfg.Point) []factflow.BranchRefinement {
	var out []factflow.BranchRefinement
	for _, inst := range l.wir.PointInstructions(point) {
		if inst.Op != wir.OpBranch {
			continue
		}
		out = append(out, l.disjunctiveRefinementsForArmRange(inst.SufficientCheckArmsTrue, true)...)
		out = append(out, l.disjunctiveRefinementsForArmRange(inst.SufficientCheckArmsFalse, false)...)
	}
	return out
}

func (l *lowerer) disjunctiveRefinementsForArmRange(r wir.ArmRange, edge bool) []factflow.BranchRefinement {
	wirArms := l.wir.SufficientCheckArms(r)
	if len(wirArms) < 2 {
		return nil
	}
	arms := make([][]branchcond.ImpliedCheck, len(wirArms))
	for i, wirArm := range wirArms {
		arm := make([]branchcond.ImpliedCheck, 0, len(wirArm))
		for _, wirCheck := range wirArm {
			check := branchcond.ImpliedCheck{
				Check:    branchCheckFromWIR(wirCheck.Check),
				Edge:     wirCheck.Edge,
				Polarity: wirCheck.Polarity,
			}
			// Type predicates require the same binding-owned authority as
			// every other consumer of a WIR type check: an arm's syntax can
			// look like `type(x) == "table"` while `type` is shadowed, and
			// only the seal proves it still has canonical Lua semantics.
			if !l.branchCheckAuthorized(check.Check) {
				continue
			}
			arm = append(arm, check)
		}
		arms[i] = arm
	}
	return l.joinedDisjunctiveRefinements(arms, edge)
}

// joinedDisjunctiveRefinements computes, for every path any arm mentions, the
// join across arms of each arm's own meet of its checks about that path. A
// path missing a mappable check in even one arm is dropped rather than
// narrowed, which is what keeps the join sound.
func (l *lowerer) joinedDisjunctiveRefinements(arms [][]branchcond.ImpliedCheck, edge bool) []factflow.BranchRefinement {
	var order []path.Path
	seen := make(map[path.PathKey]bool)
	for _, arm := range arms {
		for _, check := range arm {
			p := check.Check.Path
			if p.IsEmpty() || seen[p.Key()] {
				continue
			}
			seen[p.Key()] = true
			order = append(order, p)
		}
	}
	if len(order) == 0 {
		return nil
	}
	var out []factflow.BranchRefinement
	for _, target := range order {
		joined, ok := l.joinArmsForPath(arms, target)
		if !ok {
			continue
		}
		// An arm that cannot say anything about a path relevant to the other
		// axes (for example a bare truthiness arm like `not kind`, which
		// constrains only presence) joins the axes it is silent on straight
		// to Top. When every axis ends up Top the join carries no
		// information at all; publishing it would just be inert noise on
		// the fact stream.
		if constraint, hasConstraint := joined.Constraint(); !hasConstraint || product.Equal(l.registry, constraint, product.Top()) {
			continue
		}
		if edge {
			out = append(out, factflow.NewBranchRefinement(target, joined, true, factflow.ValueRefinement{}, false))
		} else {
			out = append(out, factflow.NewBranchRefinement(target, factflow.ValueRefinement{}, false, joined, true))
		}
	}
	return out
}

// joinArmsForPath returns the value every arm proves about target when it
// alone forces the edge, joined across arms. ok is false when some arm has no
// mappable check about target, which makes any conclusion about it unsound.
func (l *lowerer) joinArmsForPath(arms [][]branchcond.ImpliedCheck, target path.Path) (factflow.ValueRefinement, bool) {
	var joined factflow.ValueRefinement
	hasJoined := false
	for _, arm := range arms {
		armValue, ok := l.meetArmForPath(arm, target)
		if !ok {
			return factflow.ValueRefinement{}, false
		}
		if !hasJoined {
			joined = armValue
			hasJoined = true
			continue
		}
		joinedConstraint, _ := joined.Constraint()
		armConstraint, _ := armValue.Constraint()
		joined = factflow.NewValueConstraint(product.Join(l.registry, joinedConstraint, armConstraint))
	}
	return joined, hasJoined
}

// meetArmForPath returns the single value one arm proves about target,
// meeting together every mappable check the arm carries about it (an arm can
// itself be an `and` of several checks about the same path). ok is false when
// the arm carries no mappable check about target at all.
func (l *lowerer) meetArmForPath(arm []branchcond.ImpliedCheck, target path.Path) (factflow.ValueRefinement, bool) {
	var acc factflow.ValueRefinement
	has := false
	for _, check := range arm {
		if !check.Check.Path.Equal(target) {
			continue
		}
		refinement, ok := l.branchValueRefinementForCheck(check.Check)
		if !ok {
			continue
		}
		value, ok := refinement.ValueForEdge(check.Polarity)
		if !ok {
			continue
		}
		// FalsyAbsent/NegatedLiteral mark a value whose applicability is
		// conditional on state the applicator checks at apply time (e.g. a
		// bare truthiness guard's Absent conclusion holds only when the
		// subject's type can never be boolean false). Meet/Join here treat
		// every input as unconditional data, so folding a conditional value
		// in would silently drop the condition it depends on. Skipping it
		// here is the safe default: it just means this arm did not clearly
		// narrow this path, which the exhaustiveness check above already
		// treats as "no conclusion" if it was the arm's only mappable check.
		if value.FalsyAbsent() || value.NegatedLiteral() {
			continue
		}
		constraint, ok := value.Constraint()
		if !ok {
			continue
		}
		if !has {
			acc = value
			has = true
			continue
		}
		accConstraint, _ := acc.Constraint()
		acc = factflow.NewValueConstraint(product.Meet(l.registry, accConstraint, constraint))
	}
	return acc, has
}
