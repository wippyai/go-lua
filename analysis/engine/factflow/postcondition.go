package factflow

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
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

// PostconditionPathRelationKind identifies a path relation that holds after a
// node completes normally.
type PostconditionPathRelationKind uint8

const (
	PostconditionPathRelationUnknown PostconditionPathRelationKind = iota
	PostconditionPathRelationEqual
)

// PostconditionPathRelation describes a path-to-path relation that holds after
// a node completes normally.
type PostconditionPathRelation struct {
	kind      PostconditionPathRelationKind
	leftPath  path.Path
	rightPath path.Path
}

// NewPostconditionRefinement creates a node-local postcondition refinement.
func NewPostconditionRefinement(targetPath path.Path, value ValueRefinement) PostconditionRefinement {
	return PostconditionRefinement{
		targetPath: targetPath.Clone(),
		value:      value,
	}
}

// NewPostconditionRefinementSet creates a postcondition refinement set.
func NewPostconditionRefinementSet(refinements ...PostconditionRefinement) PostconditionRefinementSet {
	return PostconditionRefinementSet{refinements: copyPostconditionRefinementSlice(refinements)}
}

// NewPostconditionPathEquality creates a node-local path equality relation.
func NewPostconditionPathEquality(leftPath path.Path, rightPath path.Path) PostconditionPathRelation {
	return PostconditionPathRelation{
		kind:      PostconditionPathRelationEqual,
		leftPath:  leftPath.Clone(),
		rightPath: rightPath.Clone(),
	}
}

// TargetPath returns the refined path.
func (r PostconditionRefinement) TargetPath() path.Path { return r.targetPath.Clone() }

// TargetPathRef returns the refined path for immediate read-only use.
// Callers must not mutate or retain the returned path.
func (r PostconditionRefinement) TargetPathRef() path.Path { return r.targetPath }

// Value returns the postcondition value refinement.
func (r PostconditionRefinement) Value() ValueRefinement { return r.value }

func (r PostconditionRefinement) copy() PostconditionRefinement {
	r.targetPath = r.targetPath.Clone()
	return r
}

// Refinements returns the postcondition refinements in deterministic order.
func (s PostconditionRefinementSet) Refinements() []PostconditionRefinement {
	return copyPostconditionRefinementSlice(s.refinements)
}

func (s PostconditionRefinementSet) copy() PostconditionRefinementSet {
	return PostconditionRefinementSet{refinements: copyPostconditionRefinementSlice(s.refinements)}
}

// Kind returns the relation kind.
func (r PostconditionPathRelation) Kind() PostconditionPathRelationKind { return r.kind }

// LeftPath returns the left relation path.
func (r PostconditionPathRelation) LeftPath() path.Path { return r.leftPath.Clone() }

// RightPath returns the right relation path.
func (r PostconditionPathRelation) RightPath() path.Path { return r.rightPath.Clone() }

func (r PostconditionPathRelation) copy() PostconditionPathRelation {
	r.leftPath = r.leftPath.Clone()
	r.rightPath = r.rightPath.Clone()
	return r
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

func copyPostconditionPathRelationSlice(in []PostconditionPathRelation) []PostconditionPathRelation {
	if len(in) == 0 {
		return nil
	}
	out := make([]PostconditionPathRelation, len(in))
	for i, fact := range in {
		out[i] = fact.copy()
	}
	return out
}
