package selectop

import (
	"github.com/wippyai/go-lua/analysis/engine/relation/state/geometry"
	"github.com/wippyai/go-lua/analysis/engine/relation/tuple"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
)

// Execute retains tuples whose authenticated input scope entails the scope
// declared by the sealed SelectBinding. The returned slice contains the
// source range partition or one authenticated empty partition carrying the
// same cofiber. It is ordered transport, not a callback stream or a hidden
// regrouping operation. The concrete cofiber authority is the exact physical
// scope authority for Select; no unrelated relation Reader is fabricated
// merely to ask an entailment question.
// Select never reopens Mounted's neutral scope geometry itself.
func Execute(binding arrangement.SelectBinding, mounted witness.Mounted, authority geometry.Geometry, source tuple.Batch) ([]tuple.Batch, bool) {
	if !binding.ValidFor(mounted.Fence()) || !mounted.Available() || !authority.ValidFor(mounted) || !source.ValidFor(mounted) {
		return nil, false
	}
	selected, ok := mounted.Scope(binding.Scope())
	if !ok || !selected.ValidFor(mounted.RuntimeFence()) {
		return nil, false
	}
	if !authority.Entails(source.Scope(), selected) {
		empty, ok := tuple.PreserveRange(mounted, source, source.Scope(), []tuple.Tuple{})
		if !ok {
			return nil, false
		}
		return []tuple.Batch{empty}, true
	}
	return []tuple.Batch{source}, true
}
