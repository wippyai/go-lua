package transfer

import (
	"github.com/wippyai/go-lua/types/flow"
)

type referenceReducer struct{}

var references = referenceReducer{}

// Reference facts are address-indexed closure/function identities. Transfer
// decides when a write is authoritative; this reducer owns the canonical
// clear-then-publish operations on the two reference carriers.
func (referenceReducer) replaceFunctionSubtree(out *flow.PointState, addr flow.StableAddress, refs flow.FunctionRefs) bool {
	if out == nil {
		return false
	}
	before := out.FunctionRefs
	out.FunctionRefs = flow.WithoutFunctionRefSubtreeAddress(out.FunctionRefs, addr)
	out.FunctionRefs = flow.FunctionRefsDomain.Join(out.FunctionRefs, refs)
	return !flow.FunctionRefsDomain.Equal(before, out.FunctionRefs)
}

func (referenceReducer) replaceClosureSubtree(out *flow.PointState, addr flow.StableAddress, refs flow.ClosureRefs) bool {
	if out == nil {
		return false
	}
	before := out.ClosureRefs
	out.ClosureRefs = flow.WithoutClosureRefSubtreeAddress(out.ClosureRefs, addr)
	out.ClosureRefs = flow.ClosureRefsDomain.Join(out.ClosureRefs, refs)
	return !flow.ClosureRefsDomain.Equal(before, out.ClosureRefs)
}

func (referenceReducer) clearFunctionSubtree(out *flow.PointState, addr flow.StableAddress) bool {
	if out == nil {
		return false
	}
	before := out.FunctionRefs
	out.FunctionRefs = flow.WithoutFunctionRefSubtreeAddress(out.FunctionRefs, addr)
	return !flow.FunctionRefsDomain.Equal(before, out.FunctionRefs)
}

func (referenceReducer) clearClosureSubtree(out *flow.PointState, addr flow.StableAddress) bool {
	if out == nil {
		return false
	}
	before := out.ClosureRefs
	out.ClosureRefs = flow.WithoutClosureRefSubtreeAddress(out.ClosureRefs, addr)
	return !flow.ClosureRefsDomain.Equal(before, out.ClosureRefs)
}

func (referenceReducer) joinFunctions(out *flow.PointState, refs flow.FunctionRefs) bool {
	if out == nil {
		return false
	}
	before := out.FunctionRefs
	out.FunctionRefs = flow.FunctionRefsDomain.Join(out.FunctionRefs, refs)
	return !flow.FunctionRefsDomain.Equal(before, out.FunctionRefs)
}

func (referenceReducer) joinClosures(out *flow.PointState, refs flow.ClosureRefs) bool {
	if out == nil {
		return false
	}
	before := out.ClosureRefs
	out.ClosureRefs = flow.ClosureRefsDomain.Join(out.ClosureRefs, refs)
	return !flow.ClosureRefsDomain.Equal(before, out.ClosureRefs)
}

func (referenceReducer) setFunction(out *flow.PointState, addr flow.StableAddress, set flow.FunctionRefSet) bool {
	if out == nil {
		return false
	}
	before := out.FunctionRefs
	out.FunctionRefs = flow.WithFunctionRefAddress(out.FunctionRefs, addr, set)
	return !flow.FunctionRefsDomain.Equal(before, out.FunctionRefs)
}

func (referenceReducer) setClosure(out *flow.PointState, addr flow.StableAddress, set flow.ClosureRefSet) bool {
	if out == nil {
		return false
	}
	before := out.ClosureRefs
	out.ClosureRefs = flow.WithClosureRefAddress(out.ClosureRefs, addr, set)
	return !flow.ClosureRefsDomain.Equal(before, out.ClosureRefs)
}

func (referenceReducer) applyClosureCellEffects(out *flow.PointState, addr flow.StableAddress, effects flow.CaptureEffects) bool {
	if out == nil {
		return false
	}
	before := out.ClosureRefs
	out.ClosureRefs = flow.ApplyClosureRefCellEffectsAddress(out.ClosureRefs, addr, effects)
	return !flow.ClosureRefsDomain.Equal(before, out.ClosureRefs)
}
