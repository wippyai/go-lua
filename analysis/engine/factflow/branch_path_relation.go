package factflow

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// BranchPathRelationKind identifies the relation enforced on selected branch
// edges.
type BranchPathRelationKind uint8

const (
	BranchPathRelationUnknown BranchPathRelationKind = iota
	BranchPathRelationEqual
	BranchPathRelationNotEqual
)

// BranchPathRelation describes a path-to-path relation that is active only on
// selected branch edges.
type BranchPathRelation struct {
	kind      BranchPathRelationKind
	leftPath  path.Path
	rightPath path.Path

	activeOnTrue  bool
	activeOnFalse bool
}

// BranchPathRelationSet groups branch path relations emitted at the same CFG
// branch point.
type BranchPathRelationSet struct {
	relations []BranchPathRelation
}

// NewBranchPathEquality creates a branch path equality relation.
func NewBranchPathEquality(
	leftPath path.Path,
	rightPath path.Path,
	activeOnTrue bool,
	activeOnFalse bool,
) BranchPathRelation {
	return BranchPathRelation{
		kind:          BranchPathRelationEqual,
		leftPath:      leftPath.Clone(),
		rightPath:     rightPath.Clone(),
		activeOnTrue:  activeOnTrue,
		activeOnFalse: activeOnFalse,
	}
}

// NewBranchPathInequality creates a branch path inequality relation.
func NewBranchPathInequality(
	leftPath path.Path,
	rightPath path.Path,
	activeOnTrue bool,
	activeOnFalse bool,
) BranchPathRelation {
	return BranchPathRelation{
		kind:          BranchPathRelationNotEqual,
		leftPath:      leftPath.Clone(),
		rightPath:     rightPath.Clone(),
		activeOnTrue:  activeOnTrue,
		activeOnFalse: activeOnFalse,
	}
}

// NewBranchPathRelationSet creates a relation set.
func NewBranchPathRelationSet(relations ...BranchPathRelation) BranchPathRelationSet {
	return BranchPathRelationSet{relations: copyBranchPathRelationSlice(relations)}
}

// Kind returns the relation kind.
func (r BranchPathRelation) Kind() BranchPathRelationKind { return r.kind }

// LeftPath returns the left relation path.
func (r BranchPathRelation) LeftPath() path.Path { return r.leftPath.Clone() }

// RightPath returns the right relation path.
func (r BranchPathRelation) RightPath() path.Path { return r.rightPath.Clone() }

// ActiveOnEdge reports whether this relation is active on a branch edge.
func (r BranchPathRelation) ActiveOnEdge(cond bool) bool {
	if cond {
		return r.activeOnTrue
	}
	return r.activeOnFalse
}

func (r BranchPathRelation) copy() BranchPathRelation {
	r.leftPath = r.leftPath.Clone()
	r.rightPath = r.rightPath.Clone()
	return r
}

// Relations returns the branch path relations in deterministic order.
func (s BranchPathRelationSet) Relations() []BranchPathRelation {
	return copyBranchPathRelationSlice(s.relations)
}

func (s BranchPathRelationSet) copy() BranchPathRelationSet {
	return BranchPathRelationSet{relations: copyBranchPathRelationSlice(s.relations)}
}

func copyBranchPathRelationMap(in map[cfg.Point]BranchPathRelationSet) map[cfg.Point]BranchPathRelationSet {
	if len(in) == 0 {
		return nil
	}
	out := make(map[cfg.Point]BranchPathRelationSet, len(in))
	for point, set := range in {
		out[point] = set.copy()
	}
	return out
}

func copyBranchPathRelationSlice(in []BranchPathRelation) []BranchPathRelation {
	if len(in) == 0 {
		return nil
	}
	out := make([]BranchPathRelation, len(in))
	for i, fact := range in {
		out[i] = fact.copy()
	}
	return out
}
