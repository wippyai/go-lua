// Package ref contains stable identifiers shared by flow-engine layers.
package ref

import (
	"slices"

	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow"
)

// FuncRef identifies one function in the canonical call graph.
//
// GraphID is the CFG's stable per-construction identity. ParentHash distinguishes
// distinct lexical instances of the same source analyzed under different captured
// environments.
type FuncRef struct {
	GraphID    uint64
	ParentHash uint64
}

// ToFlow converts a canonical function identity to the flow-domain mirror type.
// The flow package cannot import checker packages, so the canonical identity
// owner defines the boundary conversion.
func ToFlow(ref FuncRef) flow.FunctionRef {
	return flow.FunctionRef{GraphID: ref.GraphID, ParentHash: ref.ParentHash}
}

// FromFlow converts the flow-domain mirror identity back to the canonical
// function identity.
func FromFlow(ref flow.FunctionRef) FuncRef {
	return FuncRef{GraphID: ref.GraphID, ParentHash: ref.ParentHash}
}

// CompareFuncRef orders function identities deterministically by graph identity
// and then by lexical-context hash.
func CompareFuncRef(a, b FuncRef) int {
	switch {
	case a.GraphID < b.GraphID:
		return -1
	case a.GraphID > b.GraphID:
		return 1
	case a.ParentHash < b.ParentHash:
		return -1
	case a.ParentHash > b.ParentHash:
		return 1
	default:
		return 0
	}
}

// SortFuncRefs sorts function identities in canonical deterministic order.
func SortFuncRefs(refs []FuncRef) {
	slices.SortFunc(refs, CompareFuncRef)
}

// UniqueSortedFuncRefs returns a sorted, deduplicated copy of refs.
func UniqueSortedFuncRefs(in []FuncRef) []FuncRef {
	if len(in) == 0 {
		return nil
	}
	out := append([]FuncRef(nil), in...)
	SortFuncRefs(out)
	dst := out[:0]
	for _, ref := range out {
		if len(dst) > 0 && CompareFuncRef(dst[len(dst)-1], ref) == 0 {
			continue
		}
		dst = append(dst, ref)
	}
	return append([]FuncRef(nil), dst...)
}

// FromFlowPath reads the flow-domain function identities at path and converts the
// finite may-set to canonical function identities. A present Top set yields
// (nil, true), preserving the caller's "authoritative but unknown" distinction
// from an absent path, which yields (nil, false).
func FromFlowPath(refs flow.FunctionRefs, path constraint.PathKey) ([]FuncRef, bool) {
	addr, ok := flow.StableAddressFromKey(path)
	if !ok {
		return nil, false
	}
	return FromFlowAddress(refs, addr)
}

// FromFlowAddress reads the flow-domain function identities at addr and converts
// the finite may-set to canonical function identities.
func FromFlowAddress(refs flow.FunctionRefs, addr flow.StableAddress) ([]FuncRef, bool) {
	set, ok := flow.FunctionRefAtAddress(refs, addr)
	if !ok {
		return nil, false
	}
	flowRefs := set.Refs()
	if len(flowRefs) == 0 {
		return nil, true
	}
	out := make([]FuncRef, 0, len(flowRefs))
	for _, ref := range flowRefs {
		out = append(out, FromFlow(ref))
	}
	return out, true
}
