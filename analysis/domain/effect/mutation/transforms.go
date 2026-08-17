package mutation

import (
	"fmt"
	"reflect"

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
	aNil := isNilTypeTransform(a)
	bNil := isNilTypeTransform(b)
	if aNil || bNil {
		return aNil && bNil
	}
	kind := KindOfTransform(a)
	if !kind.Valid() || kind != KindOfTransform(b) {
		return false
	}
	switch kind {
	case TransformUnchanged:
		return true
	case TransformElementUnion:
		aa, aok := AsElementUnion(a)
		bb, bok := AsElementUnion(b)
		return aok && bok && aa.Source.Index == bb.Source.Index
	case TransformContainerElementUnion:
		aa, aok := AsContainerElementUnion(a)
		bb, bok := AsContainerElementUnion(b)
		return aok && bok &&
			aa.Container.Index == bb.Container.Index &&
			aa.Value.Index == bb.Value.Index
	case TransformToArray:
		aa, aok := AsToArray(a)
		bb, bok := AsToArray(b)
		return aok && bok && aa.Element.Index == bb.Element.Index
	default:
		return false
	}
}

// isNilTypeTransform reports whether transform is absent, including typed nil
// pointer values stored behind the TypeTransform interface.
func isNilTypeTransform(transform TypeTransform) bool {
	if transform == nil {
		return true
	}
	v := reflect.ValueOf(transform)
	return v.Kind() == reflect.Pointer && v.IsNil()
}
