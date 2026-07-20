package transformer

import (
	"fmt"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
)

// RootKind gives each boundary namespace a distinct dense address space.
// Equal numeric indices in two namespaces never alias.
type RootKind uint8

const (
	RootParam RootKind = iota + 1
	RootCapture
	RootGlobal
	RootResult
	RootHeapTemplate
	// RootAmbient is appended rather than inserted into the historical enum.
	// Existing root fingerprints therefore remain byte-stable for bodies which
	// do not need closure-conversion carrier inputs.
	RootAmbient
	// RootMiddle names one stable invocation-local register. The formal-region
	// cell is the version/program-point dimension; the root itself never changes
	// when the register is written again. Middle roots are arena-owned and are
	// therefore validated by Arena rather than caller-supplied Shape.
	RootMiddle
	rootKindCount
)

// Root is a dense boundary address. Index is zero based within Kind.
type Root struct {
	Kind  RootKind
	Index uint32
}

// AmbientRoot is one closure-conversion input which an intermediate lexical
// body carries for a descendant without claiming it as that body's capture or
// global. Mutable means the same ordinal is also an output of the callable
// transformer and must be written back through the linked frame.
//
// Inventories are canonical only when sorted by Symbol with no duplicates.
// Keeping Symbol and Mutable in one value makes their alignment structural;
// callers must not recreate parallel symbol and mutability slices.
type AmbientRoot struct {
	Symbol  symbol.ID
	Mutable bool
}

func (r AmbientRoot) valid() bool { return r.Symbol != 0 }

func validAmbientRoots(roots []AmbientRoot) bool {
	for index, root := range roots {
		if !root.valid() || index != 0 && roots[index-1].Symbol >= root.Symbol {
			return false
		}
	}
	return true
}

// ResultRoot identifies one function-local temporary call-result slot. Point
// is part of the identity: slot zero produced by two call sites is two distinct
// roots and cannot leak through a later call boundary.
type ResultRoot struct {
	Point cfg.Point
	Slot  uint32
}

func (r ResultRoot) Valid(pointCount int) bool {
	return uint64(r.Point) < uint64(pointCount)
}

// Shape fixes every namespace width for one immutable transformer.
type Shape struct {
	Params        uint32
	Captures      uint32
	Globals       uint32
	Ambients      uint32
	Results       uint32
	HeapTemplates uint32
}

func (s Shape) count(kind RootKind) uint32 {
	switch kind {
	case RootParam:
		return s.Params
	case RootCapture:
		return s.Captures
	case RootGlobal:
		return s.Globals
	case RootAmbient:
		return s.Ambients
	case RootResult:
		return s.Results
	case RootHeapTemplate:
		return s.HeapTemplates
	default:
		return 0
	}
}

func (s Shape) offset(kind RootKind) int {
	switch kind {
	case RootParam:
		return 0
	case RootCapture:
		return int(s.Params)
	case RootGlobal:
		return int(s.Params + s.Captures)
	case RootAmbient:
		return int(s.Params + s.Captures + s.Globals)
	case RootResult:
		return int(s.Params + s.Captures + s.Globals + s.Ambients)
	case RootHeapTemplate:
		return int(s.Params + s.Captures + s.Globals + s.Ambients + s.Results)
	default:
		return -1
	}
}

// InputCount is the exact caller-supplied boundary width. Result and heap
// template roots are owner-local existentials: Apply alpha-renames them in the
// lexical call-frame namespace and never asks the caller to manufacture them.
func (s Shape) InputCount() int {
	return int(s.Params + s.Captures + s.Globals + s.Ambients)
}

// ExistentialCount is the exact owner-local namespace width.
func (s Shape) ExistentialCount() int {
	return int(s.Results + s.HeapTemplates)
}

// ValueCount is the complete owner namespace width. It is not a call-boundary
// arity; use InputCount when accepting caller bindings.
func (s Shape) ValueCount() int {
	return s.InputCount() + s.ExistentialCount()
}

func (s Shape) validate(root Root) bool {
	return root.Kind > 0 && root.Kind < rootKindCount && root.Index < s.count(root.Kind)
}

func (s Shape) validateInput(root Root) bool {
	switch root.Kind {
	case RootParam, RootCapture, RootGlobal, RootAmbient:
		return root.Index < s.count(root.Kind)
	default:
		return false
	}
}

// BindingCursor is a zero-allocation view over caller-owned dense bindings.
// It contains no maps and is safe to copy by value.
type BindingCursor struct {
	shape  Shape
	values []product.Value
	paths  []pathdom.Path
}

// NewBindingCursor validates and borrows the supplied packed slices. Paths may
// be nil when a relation has no path terms.
func NewBindingCursor(shape Shape, values []product.Value, paths []pathdom.Path) (BindingCursor, error) {
	if len(values) != shape.InputCount() {
		return BindingCursor{}, fmt.Errorf("transformer: got %d value bindings, want %d", len(values), shape.InputCount())
	}
	if paths != nil && len(paths) != shape.InputCount() {
		return BindingCursor{}, fmt.Errorf("transformer: got %d path bindings, want %d", len(paths), shape.InputCount())
	}
	return BindingCursor{shape: shape, values: values, paths: paths}, nil
}

// Value reads one dense value without allocating.
func (c BindingCursor) Value(root Root) (product.Value, bool) {
	if !c.shape.validateInput(root) {
		return product.Value{}, false
	}
	return c.values[c.shape.offset(root.Kind)+int(root.Index)], true
}

// Path reads one borrowed dense path without allocating.
func (c BindingCursor) Path(root Root) (pathdom.Path, bool) {
	if c.paths == nil || !c.shape.validateInput(root) {
		return pathdom.Path{}, false
	}
	return c.paths[c.shape.offset(root.Kind)+int(root.Index)], true
}
