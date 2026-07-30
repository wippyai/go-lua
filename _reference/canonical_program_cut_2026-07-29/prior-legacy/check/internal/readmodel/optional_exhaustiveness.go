package readmodel

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	readapi "github.com/wippyai/go-lua/analysis/check/readmodel"
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
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
		candidate, ok := r.optionalCandidateForCheck(branch.Point, branch.Check)
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
	t, ok := r.result.OptionalExhaustivenessPathTypeAt(point, check.Path)
	if !ok || !readapi.OptionalTypeHasConcreteValue(t) {
		return optionalBranchCandidate{}, false
	}
	switch check.Kind {
	case branchcond.CheckNil:
		return optionalBranchCandidate{target: check.Path, handlesNil: true}, true
	case branchcond.CheckNotNil:
		return optionalBranchCandidate{target: check.Path, handlesValue: true}, true
	case branchcond.CheckTruthy:
		if readapi.OptionalTruthinessPartitionsNilValue(t) {
			return optionalBranchCandidate{target: check.Path, handlesValue: true}, true
		}
	case branchcond.CheckFalsy:
		if readapi.OptionalTruthinessPartitionsNilValue(t) {
			return optionalBranchCandidate{target: check.Path, handlesNil: true}, true
		}
	}
	return optionalBranchCandidate{}, false
}
