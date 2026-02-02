package siblings

import (
	"github.com/wippyai/go-lua/compiler/check/returns"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/join"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// MergeSiblingType merges two sibling types by precision.
//
// For function types, this prefers the more refined type (better return info).
// If neither refines the other, uses join.Two to create a union.
//
// This is used when updating sibling types during nested function processing:
// as functions are analyzed, their types become more precise, and those
// improvements are merged back into the sibling map.
func MergeSiblingType(prev, next typ.Type) typ.Type {
	if prev == nil {
		return next
	}
	if next == nil {
		return prev
	}
	pfn, _ := unwrap.Alias(prev).(*typ.Function)
	nfn, _ := unwrap.Alias(next).(*typ.Function)
	if pfn != nil && nfn != nil {
		if returns.ReturnTypesRefine(pfn.Returns, nfn.Returns) {
			return prev
		}
		if returns.ReturnTypesRefine(nfn.Returns, pfn.Returns) {
			return next
		}
	}
	return join.Two(prev, next)
}
