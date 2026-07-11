// Package symbolic defines the boundary vocabulary for function summary
// transformers: typed roots, symbolic paths, guarded value expressions, and
// the Transformer container solved once per lexical function and instantiated
// at call sites.
//
// Design invariants (see p3 plan review invariants):
//   - Roots are namespace-distinct until binding: a parameter can never be
//     confused with a capture, global, heap root, result slot, or allocation.
//   - No caller value, entry digest, or concrete caller heap participates in
//     any identity or cache key derived from these types.
//   - Complexity limits are expressed as sound widening (collapse toward Top
//     with an observable reason), never as truncation.
package symbolic

import (
	"strconv"
	"strings"

	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/symbol"
)

// RootKind is the namespace of a symbolic boundary root.
type RootKind uint8

const (
	// RootParam is the N-th declared parameter of the summarized function.
	RootParam RootKind = iota
	// RootCapture is an upvalue captured by the summarized function,
	// identified by its defining symbol.
	RootCapture
	// RootGlobal is a global read/written by the summarized function,
	// identified by its global symbol.
	RootGlobal
	// RootHeap is a heap region reachable at the boundary, identified by a
	// summary-local ordinal assigned during transformer construction.
	RootHeap
	// RootResult is the N-th result slot of the summarized function.
	RootResult
	// RootAllocation is an allocation site inside the summarized function,
	// identified by a summary-local ordinal. Allocations are fresh per
	// instantiation and never alias caller state.
	RootAllocation
)

func (k RootKind) String() string {
	switch k {
	case RootParam:
		return "param"
	case RootCapture:
		return "capture"
	case RootGlobal:
		return "global"
	case RootHeap:
		return "heap"
	case RootResult:
		return "ret"
	case RootAllocation:
		return "alloc"
	default:
		return "invalid"
	}
}

// Root identifies one symbolic boundary entity. Exactly one of Index or
// Symbol is meaningful, selected by Kind: Param/Result/Heap/Allocation use
// Index; Capture/Global use Symbol.
type Root struct {
	Kind   RootKind
	Index  int
	Symbol symbol.ID
}

// Valid reports whether the root uses the correct identity dimension for its
// kind and that dimension is in range.
func (r Root) Valid() bool {
	switch r.Kind {
	case RootParam, RootResult, RootHeap, RootAllocation:
		return r.Index >= 0 && r.Symbol == 0
	case RootCapture, RootGlobal:
		return r.Symbol != 0 && r.Index == 0
	default:
		return false
	}
}

// Less is the canonical strict order over roots: by kind, then by the
// kind-selected identity dimension. It gives every container a deterministic
// iteration order independent of construction history.
func (r Root) Less(o Root) bool {
	if r.Kind != o.Kind {
		return r.Kind < o.Kind
	}
	if r.Index != o.Index {
		return r.Index < o.Index
	}
	return r.Symbol < o.Symbol
}

// String renders the canonical spelling, e.g. "param[0]", "capture[#12]".
func (r Root) String() string {
	var b strings.Builder
	b.WriteString(r.Kind.String())
	b.WriteByte('[')
	switch r.Kind {
	case RootCapture, RootGlobal:
		b.WriteByte('#')
		b.WriteString(strconv.FormatUint(uint64(r.Symbol), 10))
	default:
		b.WriteString(strconv.Itoa(r.Index))
	}
	b.WriteByte(']')
	return b.String()
}

// Path is a symbolic access path: a boundary root plus a canonical suffix.
type Path struct {
	Root     Root
	Segments []segment.Segment
}

// Valid reports whether the root is valid; segments reuse the engine's
// canonical segment vocabulary and are valid by construction.
func (p Path) Valid() bool { return p.Root.Valid() }

// Less orders paths by root, then by the canonical suffix spelling.
func (p Path) Less(o Path) bool {
	if p.Root != o.Root {
		return p.Root.Less(o.Root)
	}
	return segment.FormatSegments(p.Segments) < segment.FormatSegments(o.Segments)
}

// Equal reports structural path equality.
func (p Path) Equal(o Path) bool {
	if p.Root != o.Root || len(p.Segments) != len(o.Segments) {
		return false
	}
	for i := range p.Segments {
		if p.Segments[i] != o.Segments[i] {
			return false
		}
	}
	return true
}

// String renders "root[.suffix]" with the engine's canonical suffix format.
func (p Path) String() string {
	return p.Root.String() + segment.FormatSegments(p.Segments)
}
