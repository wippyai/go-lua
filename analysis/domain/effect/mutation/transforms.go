package mutation

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/effect"
)

// TypeTransform describes a local type change for Mutate.
type TypeTransform interface {
	transform()
	String() string
}

type ElementUnion struct {
	Source effect.ParamRef
}

func (ElementUnion) transform() {}
func (e ElementUnion) String() string {
	return fmt.Sprintf("union_elem(%s)", e.Source)
}

type ContainerElementUnion struct {
	Container effect.ParamRef
	Value     effect.ParamRef
}

func (ContainerElementUnion) transform() {}
func (c ContainerElementUnion) String() string {
	return fmt.Sprintf("union_elem(%s, %s)", c.Container, c.Value)
}

type ToArray struct {
	Element effect.ParamRef
}

func (ToArray) transform() {}
func (t ToArray) String() string {
	return fmt.Sprintf("to_array(%s)", t.Element)
}

type Unchanged struct{}

func (Unchanged) transform()     {}
func (Unchanged) String() string { return "unchanged" }

func transformEquals(a, b TypeTransform) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	switch av := a.(type) {
	case Unchanged:
		return unchangedEquals(av, b)
	case *Unchanged:
		return unchangedEquals(*av, b)
	case ElementUnion:
		return elementUnionEquals(av, b)
	case *ElementUnion:
		return elementUnionEquals(*av, b)
	case ContainerElementUnion:
		return containerElementUnionEquals(av, b)
	case *ContainerElementUnion:
		return containerElementUnionEquals(*av, b)
	case ToArray:
		return toArrayEquals(av, b)
	case *ToArray:
		return toArrayEquals(*av, b)
	default:
		return false
	}
}

func unchangedEquals(_ Unchanged, b TypeTransform) bool {
	switch b.(type) {
	case Unchanged, *Unchanged:
		return true
	default:
		return false
	}
}

func elementUnionEquals(a ElementUnion, b TypeTransform) bool {
	bb, ok := normalizeElementUnion(b)
	return ok && a.Source.Index == bb.Source.Index
}

func containerElementUnionEquals(a ContainerElementUnion, b TypeTransform) bool {
	bb, ok := normalizeContainerElementUnion(b)
	return ok &&
		a.Container.Index == bb.Container.Index &&
		a.Value.Index == bb.Value.Index
}

func toArrayEquals(a ToArray, b TypeTransform) bool {
	bb, ok := normalizeToArray(b)
	return ok && a.Element.Index == bb.Element.Index
}

func normalizeElementUnion(t TypeTransform) (ElementUnion, bool) {
	switch tt := t.(type) {
	case ElementUnion:
		return tt, true
	case *ElementUnion:
		if tt != nil {
			return *tt, true
		}
	}
	return ElementUnion{}, false
}

func normalizeContainerElementUnion(t TypeTransform) (ContainerElementUnion, bool) {
	switch tt := t.(type) {
	case ContainerElementUnion:
		return tt, true
	case *ContainerElementUnion:
		if tt != nil {
			return *tt, true
		}
	}
	return ContainerElementUnion{}, false
}

func normalizeToArray(t TypeTransform) (ToArray, bool) {
	switch tt := t.(type) {
	case ToArray:
		return tt, true
	case *ToArray:
		if tt != nil {
			return *tt, true
		}
	}
	return ToArray{}, false
}
