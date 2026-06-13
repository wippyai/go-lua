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
}

// NewCallResultTarget creates a call result target descriptor.
func NewCallResultTarget(kind CallResultTargetKind, index, resultIndex int, targetSymbol symbol.ID, targetPath path.Path) CallResultTarget {
	return CallResultTarget{
		kind:         kind,
		index:        index,
		resultIndex:  resultIndex,
		targetSymbol: targetSymbol,
		targetPath:   copyPath(targetPath),
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
func (t CallResultTarget) TargetPath() path.Path { return copyPath(t.targetPath) }

func (t CallResultTarget) copy() CallResultTarget {
	t.targetPath = copyPath(t.targetPath)
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
