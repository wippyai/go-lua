package typ

import (
	"github.com/wippyai/go-lua/domain/type/kind"
	"github.com/wippyai/go-lua/internal/hash"
)

// TypeParam represents a type parameter in a generic type or function.
//
// Type parameters enable parametric polymorphism: a single definition that
// works with multiple types. The Constraint field bounds what types can
// instantiate this parameter (nil means any type is allowed).
//
// Example: In Array<T>, T is a TypeParam with Name="T" and Constraint=nil.
//
// Identity:
//
// A formal is identified by its position in the binder that introduces it, the
// pair (binder, ordinal) the canonical encoder writes. Name is the spelling a
// declaration chose and is presentation only, so it is part of neither the
// structural hash nor structural equality. An occurrence that no binder in the
// comparison introduces is free; a free occurrence keeps its lexical name as
// its identity, and an anonymous free occurrence is only ever itself.
type TypeParam struct {
	Name       string
	Constraint Type // Upper bound (nil means any type)
	hash       uint64
	typeProperties
}

// NewTypeParam creates a type parameter.
func NewTypeParam(name string, constraint Type) *TypeParam {
	h := uint64(kind.TypeParam)
	if constraint != nil {
		h = hash.MixHash(h, constraint.Hash())
	}
	props := typePropertiesOf(constraint)
	props.containsTypeParam = true

	zzProbeConstruct(uint64(kind.TypeParam), h) // ZZPROBE
	return &TypeParam{
		Name:           name,
		Constraint:     constraint,
		hash:           h,
		typeProperties: props,
	}
}

func (t *TypeParam) Kind() kind.Kind { return kind.TypeParam }
func (t *TypeParam) String() string {
	return renderTypeString(t)
}
func (t *TypeParam) Hash() uint64 { return t.hash }
func (t *TypeParam) Equals(other Type) bool {
	return typeEquals(t, other)
}
