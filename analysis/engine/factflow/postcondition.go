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

// PostconditionPathRelationSet groups node-local postcondition path relations
// emitted at the same CFG point.
type PostconditionPathRelationSet struct {
	relations []PostconditionPathRelation
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

// NewPostconditionPathRelationSet creates a path relation set.
func NewPostconditionPathRelationSet(relations ...PostconditionPathRelation) PostconditionPathRelationSet {
	return PostconditionPathRelationSet{relations: copyPostconditionPathRelationSlice(relations)}
}

// TargetPath returns the refined path.
func (r PostconditionRefinement) TargetPath() path.Path { return r.targetPath.Clone() }

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

// Relations returns the postcondition path relations in deterministic order.
func (s PostconditionPathRelationSet) Relations() []PostconditionPathRelation {
	return copyPostconditionPathRelationSlice(s.relations)
}

func (s PostconditionPathRelationSet) copy() PostconditionPathRelationSet {
	return PostconditionPathRelationSet{relations: copyPostconditionPathRelationSlice(s.relations)}
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

func copyPostconditionPathRelationMap(in map[cfg.Point]PostconditionPathRelationSet) map[cfg.Point]PostconditionPathRelationSet {
	if len(in) == 0 {
		return nil
	}
	out := make(map[cfg.Point]PostconditionPathRelationSet, len(in))
	for point, set := range in {
		out[point] = set.copy()
	}
	return out
}

func mergePostconditionRefinementMap(
	base map[cfg.Point]PostconditionRefinementSet,
	add map[cfg.Point]PostconditionRefinementSet,
) map[cfg.Point]PostconditionRefinementSet {
	return mergePostconditionMap(base, add, copyPostconditionRefinementMap,
		PostconditionRefinementSet.Refinements, NewPostconditionRefinementSet)
}

func mergePostconditionPathRelationMap(
	base map[cfg.Point]PostconditionPathRelationSet,
	add map[cfg.Point]PostconditionPathRelationSet,
) map[cfg.Point]PostconditionPathRelationSet {
	return mergePostconditionMap(base, add, copyPostconditionPathRelationMap,
		PostconditionPathRelationSet.Relations, NewPostconditionPathRelationSet)
}

// mergePostconditionMap appends each point's add-set elements onto a copy of the
// base map's per-point set, rebuilding each set with construct.
func mergePostconditionMap[S any, E any](
	base, add map[cfg.Point]S,
	copyMap func(map[cfg.Point]S) map[cfg.Point]S,
	elems func(S) []E,
	construct func(...E) S,
) map[cfg.Point]S {
	out := copyMap(base)
	if len(add) == 0 {
		return out
	}
	if out == nil {
		out = make(map[cfg.Point]S, len(add))
	}
	for point, set := range add {
		merged := append(elems(out[point]), elems(set)...)
		out[point] = construct(merged...)
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
