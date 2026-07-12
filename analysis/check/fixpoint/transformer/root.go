package transformer

import (
	"fmt"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
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
	rootKindCount
)

// Root is a dense boundary address. Index is zero based within Kind.
type Root struct {
	Kind  RootKind
	Index uint32
}

// Shape fixes every namespace width for one immutable transformer.
type Shape struct {
	Params        uint32
	Captures      uint32
	Globals       uint32
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
	case RootResult:
		return int(s.Params + s.Captures + s.Globals)
	case RootHeapTemplate:
		return int(s.Params + s.Captures + s.Globals + s.Results)
	default:
		return -1
	}
}

// ValueCount is the exact packed binding width.
func (s Shape) ValueCount() int {
	return int(s.Params + s.Captures + s.Globals + s.Results + s.HeapTemplates)
}

func (s Shape) validate(root Root) bool {
	return root.Kind > 0 && root.Kind < rootKindCount && root.Index < s.count(root.Kind)
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
	if len(values) != shape.ValueCount() {
		return BindingCursor{}, fmt.Errorf("transformer: got %d value bindings, want %d", len(values), shape.ValueCount())
	}
	if paths != nil && len(paths) != shape.ValueCount() {
		return BindingCursor{}, fmt.Errorf("transformer: got %d path bindings, want %d", len(paths), shape.ValueCount())
	}
	return BindingCursor{shape: shape, values: values, paths: paths}, nil
}

// Value reads one dense value without allocating.
func (c BindingCursor) Value(root Root) (product.Value, bool) {
	if !c.shape.validate(root) {
		return product.Value{}, false
	}
	return c.values[c.shape.offset(root.Kind)+int(root.Index)], true
}

// Path reads one borrowed dense path without allocating.
func (c BindingCursor) Path(root Root) (pathdom.Path, bool) {
	if c.paths == nil || !c.shape.validate(root) {
		return pathdom.Path{}, false
	}
	return c.paths[c.shape.offset(root.Kind)+int(root.Index)], true
}
