package readmodel

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	readapi "github.com/wippyai/go-lua/analysis/check/readmodel"
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

type OptionalExhaustiveness = readapi.OptionalExhaustiveness

type optionalBranchCandidate struct {
	target       path.Path
	handlesNil   bool
	handlesValue bool
}

// ForEachOptionalExhaustiveness visits branch chains that handle and consume an
// optional value case but continue without proving the nil case handled.
func (r Reader) ForEachOptionalExhaustiveness(visit func(OptionalExhaustiveness) bool) bool {
	if r.result == nil || visit == nil || r.result.Graph() == nil {
		return false
	}
	visited := false
	for _, chain := range r.result.IfBranchChains() {
		item, ok := r.optionalChain(chain)
		if !ok {
			continue
		}
		visited = true
		if !visit(item) {
			return true
		}
	}
	return visited
}

func (r Reader) optionalChain(chain body.IfBranchChain) (OptionalExhaustiveness, bool) {
	if chain.HasDefaultElse {
		return OptionalExhaustiveness{}, false
	}
	var selected path.Path
	selectedSet := false
	handlesNil := false
	handlesValue := false
	consumesValue := false
	var point cfg.Point
	for _, branch := range chain.Branches {
		if branch.Fact.If == nil {
			return OptionalExhaustiveness{}, false
		}
		candidate, ok := r.optionalCandidateForCheck(branch.Point, branch.Fact.Check)
		if !ok {
			return OptionalExhaustiveness{}, false
		}
		if !selectedSet {
			selected = candidate.target
			selectedSet = true
			point = branch.Point
		} else if !selected.Equal(candidate.target) {
			return OptionalExhaustiveness{}, false
		}
		handlesNil = handlesNil || candidate.handlesNil
		handlesValue = handlesValue || candidate.handlesValue
		if candidate.handlesValue &&
			r.result.IfBranchConsumesPath(branch.Point, branch, candidate.target) &&
			!r.result.IfBranchTerminates(branch) {
			consumesValue = true
		}
	}
	if !selectedSet || !handlesValue || !consumesValue || handlesNil {
		return OptionalExhaustiveness{}, false
	}
	missing := []string{selected.String() + " == nil"}
	return OptionalExhaustiveness{
		Point:   point,
		Span:    sourceSpanFromBody(chain.Head.ConditionSpan),
		Target:  selected.String(),
		Missing: missing,
	}, true
}

func (r Reader) optionalCandidateForCheck(point cfg.Point, check branchcond.Check) (optionalBranchCandidate, bool) {
	if check.Path.IsEmpty() {
		return optionalBranchCandidate{}, false
	}
	t, ok := r.optionalPathType(point, check.Path)
	if !ok || !optionalTypeHasValue(t) {
		return optionalBranchCandidate{}, false
	}
	switch check.Kind {
	case branchcond.CheckNil:
		return optionalBranchCandidate{target: check.Path, handlesNil: true}, true
	case branchcond.CheckNotNil:
		return optionalBranchCandidate{target: check.Path, handlesValue: true}, true
	case branchcond.CheckTruthy:
		if optionalTruthyPartitionsNilValue(t) {
			return optionalBranchCandidate{target: check.Path, handlesValue: true}, true
		}
	case branchcond.CheckFalsy:
		if optionalTruthyPartitionsNilValue(t) {
			return optionalBranchCandidate{target: check.Path, handlesNil: true}, true
		}
	}
	return optionalBranchCandidate{}, false
}

func (r Reader) optionalPathType(point cfg.Point, target path.Path) (typ.Type, bool) {
	direct, directOK := r.optionalDirectPathType(point, target)
	if directOK && optionalTypeHasValue(direct) {
		return direct, true
	}
	if t, ok := r.optionalDominatingAliasSourceType(point, target); ok {
		return t, true
	}
	return direct, directOK
}

func (r Reader) optionalDirectPathType(point cfg.Point, target path.Path) (typ.Type, bool) {
	if target.Symbol == 0 {
		return nil, false
	}
	root := target.RootOnly()
	t, ok := r.discriminatedUnionRootType(point, root)
	if !ok || t == nil {
		return nil, false
	}
	for _, seg := range target.Segments {
		next, ok := body.TypeAtSegment(t, seg)
		if !ok {
			return nil, false
		}
		t = next
	}
	return t, true
}

func (r Reader) optionalDominatingAliasSourceType(point cfg.Point, target path.Path) (typ.Type, bool) {
	if target.Symbol == 0 {
		return nil, false
	}
	fact, _, ok := r.result.DominatingRootLocalAssignment(point, target.Symbol)
	if !ok || fact.Expr == nil || fact.Type != nil {
		return nil, false
	}
	source, ok := r.result.ExpressionPath(fact.Expr)
	if !ok || source.IsEmpty() {
		return nil, false
	}
	return r.optionalDirectPathType(point, source.AppendSegments(target.Segments))
}

func optionalTypeHasValue(t typ.Type) bool {
	if t == nil || typ.IsAny(t) || typ.IsUnknown(t) || typ.IsNever(t) || !typevalue.ProjectionHasNil(t) {
		return false
	}
	value := readmodelProjectionWithoutNil(t)
	return value != nil && !typ.IsNever(value)
}

func optionalTruthyPartitionsNilValue(t typ.Type) bool {
	value := readmodelProjectionWithoutNil(t)
	return value != nil && !typ.IsNever(value) && !optionalTypeAdmitsFalse(value)
}

func optionalTypeAdmitsFalse(t typ.Type) bool {
	switch v := t.(type) {
	case nil:
		return false
	case *typ.Alias:
		return optionalTypeAdmitsFalse(v.UnaliasedTarget())
	case *typ.Union:
		for _, member := range v.Members {
			if optionalTypeAdmitsFalse(member) {
				return true
			}
		}
		return false
	default:
		return typ.TypeEquals(t, typ.Boolean) || typ.TypeEquals(t, typ.False)
	}
}
