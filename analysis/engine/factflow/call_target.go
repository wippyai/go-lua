package factflow

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/symbol"
)

// CallResultTargetKind classifies where a call result is consumed.
type CallResultTargetKind uint8

const (
	CallResultTargetUnknown CallResultTargetKind = iota
	CallResultTargetLocalAssignment
	CallResultTargetOrdinaryAssignment
	CallResultTargetReturn
	CallResultTargetExpression
)

// CallResultTarget describes one target that consumes a call result.
type CallResultTarget struct {
	kind         CallResultTargetKind
	index        int
	resultIndex  int
	targetSymbol symbol.ID
	targetPath   path.Path
	targetKey    path.PathKey
}

// CallResultTargetView provides read-only access to a call result target
// without exposing mutable internal path storage.
type CallResultTargetView struct {
	target CallResultTarget
}

// NewCallResultTarget creates a call result target descriptor.
func NewCallResultTarget(kind CallResultTargetKind, index, resultIndex int, targetSymbol symbol.ID, targetPath path.Path) CallResultTarget {
	ownedPath := targetPath.Clone()
	targetKey := path.PathKey("")
	if !ownedPath.IsEmpty() {
		targetKey = ownedPath.Key()
	}
	return CallResultTarget{
		kind:         kind,
		index:        index,
		resultIndex:  resultIndex,
		targetSymbol: targetSymbol,
		targetPath:   ownedPath,
		targetKey:    targetKey,
	}
}

// Kind returns the target category.
func (t CallResultTarget) Kind() CallResultTargetKind { return t.kind }

// Index returns the target's value-list index.
func (t CallResultTarget) Index() int { return t.index }

// ResultIndex returns the consumed result slot from the producing call.
func (t CallResultTarget) ResultIndex() int { return t.resultIndex }

// TargetSymbol returns the target's symbol identity.
func (t CallResultTarget) TargetSymbol() symbol.ID { return t.targetSymbol }

// TargetPath returns the target's path identity.
func (t CallResultTarget) TargetPath() path.Path { return t.targetPath.Clone() }

// Kind returns the target category.
func (v CallResultTargetView) Kind() CallResultTargetKind { return v.target.kind }

// Index returns the target's value-list index.
func (v CallResultTargetView) Index() int { return v.target.index }

// ResultIndex returns the consumed result slot from the producing call.
func (v CallResultTargetView) ResultIndex() int { return v.target.resultIndex }

// TargetSymbol returns the target's symbol identity.
func (v CallResultTargetView) TargetSymbol() symbol.ID { return v.target.targetSymbol }

// TargetPath returns a defensive copy of the target's path identity.
func (v CallResultTargetView) TargetPath() path.Path { return v.target.targetPath.Clone() }

// TargetPathRef returns the target path for immediate read-only use.
// Callers must not mutate or retain the returned path.
func (v CallResultTargetView) TargetPathRef() path.Path { return v.target.targetPath }

// TargetPathKey returns the target path's structural key.
func (v CallResultTargetView) TargetPathKey() path.PathKey { return v.target.targetKey }

// TargetPathEmpty reports whether the target path has no identity.
func (v CallResultTargetView) TargetPathEmpty() bool { return v.target.targetPath.IsEmpty() }

// TargetPathSegmentCount returns the number of path segments below the root.
func (v CallResultTargetView) TargetPathSegmentCount() int {
	return len(v.target.targetPath.Segments)
}

// TargetPathEqual reports whether p matches the target path.
func (v CallResultTargetView) TargetPathEqual(p path.Path) bool {
	return v.target.targetPath.Equal(p)
}

// CallResultTarget returns a defensive copy of the target descriptor.
func (v CallResultTargetView) CallResultTarget() CallResultTarget {
	return v.target.copy()
}

func (t CallResultTarget) copy() CallResultTarget {
	t.targetPath = t.targetPath.Clone()
	return t
}

func copyCallResultTargets(in []CallResultTarget) []CallResultTarget {
	if len(in) == 0 {
		return nil
	}
	out := make([]CallResultTarget, len(in))
	for i := range in {
		out[i] = in[i].copy()
	}
	return out
}
