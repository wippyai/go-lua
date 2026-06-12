package factflow

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// PostconditionRefinement describes a value refinement that holds after a node
// completes normally.
type PostconditionRefinement struct {
	targetPath path.Path
	value      ValueRefinement
}

// PostconditionRefinementSet groups node-local postcondition refinements emitted
// at the same CFG point.
type PostconditionRefinementSet struct {
	refinements []PostconditionRefinement
}

// NewPostconditionRefinement creates a node-local postcondition refinement.
func NewPostconditionRefinement(targetPath path.Path, value ValueRefinement) PostconditionRefinement {
	return PostconditionRefinement{
		targetPath: copyPath(targetPath),
		value:      value,
	}
}

// NewPostconditionRefinementSet creates a postcondition refinement set.
func NewPostconditionRefinementSet(refinements ...PostconditionRefinement) PostconditionRefinementSet {
	return PostconditionRefinementSet{refinements: copyPostconditionRefinementSlice(refinements)}
}

// TargetPath returns the refined path.
func (r PostconditionRefinement) TargetPath() path.Path { return copyPath(r.targetPath) }

// Value returns the postcondition value refinement.
func (r PostconditionRefinement) Value() ValueRefinement { return r.value }

func (r PostconditionRefinement) copy() PostconditionRefinement {
	r.targetPath = copyPath(r.targetPath)
	return r
}

// Refinements returns the postcondition refinements in deterministic order.
func (s PostconditionRefinementSet) Refinements() []PostconditionRefinement {
	return copyPostconditionRefinementSlice(s.refinements)
}

func (s PostconditionRefinementSet) copy() PostconditionRefinementSet {
	return PostconditionRefinementSet{refinements: copyPostconditionRefinementSlice(s.refinements)}
}

func copyPostconditionRefinementMap(in map[cfg.Point]PostconditionRefinementSet) map[cfg.Point]PostconditionRefinementSet {
	if len(in) == 0 {
		return nil
	}
	out := make(map[cfg.Point]PostconditionRefinementSet, len(in))
	for point, set := range in {
		out[point] = set.copy()
	}
	return out
}

func copyPostconditionRefinementSlice(in []PostconditionRefinement) []PostconditionRefinement {
	if len(in) == 0 {
		return nil
	}
	out := make([]PostconditionRefinement, len(in))
	for i, fact := range in {
		out[i] = fact.copy()
	}
	return out
}
