package callboundary

import (
	"strconv"
	"strings"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
)

// PathBindings resolves placeholder and return-slot paths in a call-boundary
// payload onto caller paths. It owns the boundary syntax used by NormalReturnFacts:
// $N paths bind to call arguments/receiver and ret[N] paths bind to call result
// targets.
type PathBindings struct {
	params  []pathdom.Path
	returns []pathdom.Path
}

// ParameterRoots returns the exact caller paths bound to parameter boundary
// roots. Empty entries preserve an unbound boundary ordinal.
func (b PathBindings) ParameterRoots() []pathdom.Path {
	return clonePaths(b.params)
}

// ReturnRoots returns the exact caller paths bound to return boundary roots.
// Empty entries preserve an unbound boundary ordinal.
func (b PathBindings) ReturnRoots() []pathdom.Path {
	return clonePaths(b.returns)
}

// NewPathBindings creates a call-boundary path resolver. The slices are copied
// so callers can build bindings incrementally without sharing mutable state.
func NewPathBindings(params, returns []pathdom.Path) PathBindings {
	return PathBindings{
		params:  clonePaths(params),
		returns: clonePaths(returns),
	}
}

// Substitute maps a boundary path to the caller path it denotes.
func (b PathBindings) Substitute(p pathdom.Path) (pathdom.Path, bool) {
	if index, ok := ReturnSlotIndex(p); ok {
		if index < 0 || index >= len(b.returns) || b.returns[index].IsEmpty() {
			return pathdom.Path{}, false
		}
		return b.returns[index].AppendSegments(p.Segments), true
	}
	return p.Substitute(b.params)
}

// IsConcreteSymbolPath reports whether p is an internal concrete symbol path,
// not a placeholder or return-slot boundary path.
func IsConcreteSymbolPath(p pathdom.Path) bool {
	if p.IsPlaceholder() || p.Symbol == 0 {
		return false
	}
	_, isReturnSlot := ReturnSlotIndex(p)
	return !isReturnSlot
}

// ReturnSlotIndex parses the root ret[N] syntax used by return-slot facts.
func ReturnSlotIndex(p pathdom.Path) (int, bool) {
	if p.Symbol != 0 || !strings.HasPrefix(p.Root, "ret[") || !strings.HasSuffix(p.Root, "]") {
		return 0, false
	}
	raw := strings.TrimSuffix(strings.TrimPrefix(p.Root, "ret["), "]")
	index, err := strconv.Atoi(raw)
	if err != nil || index < 0 || p.Root != "ret["+strconv.Itoa(index)+"]" {
		return 0, false
	}
	return index, true
}

// PathRootedInReturnSlots reports whether p is rooted in one of slots.
func PathRootedInReturnSlots(p pathdom.Path, slots map[int]struct{}) bool {
	if p.IsEmpty() || len(slots) == 0 {
		return false
	}
	index, ok := ReturnSlotIndex(p)
	if !ok {
		return false
	}
	_, ok = slots[index]
	return ok
}

func clonePaths(in []pathdom.Path) []pathdom.Path {
	if len(in) == 0 {
		return nil
	}
	out := make([]pathdom.Path, len(in))
	for i, p := range in {
		out[i] = p.Clone()
	}
	return out
}
