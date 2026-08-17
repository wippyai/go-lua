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
type TypeParam struct {
	Name       string
	Constraint Type // Upper bound (nil means any type)
	hash       uint64
	typeProperties
}

// NewTypeParam creates a type parameter.
func NewTypeParam(name string, constraint Type) *TypeParam {
	h := hash.MixHash(uint64(kind.TypeParam), hash.FnvString(name))
	if constraint != nil {
		h = hash.MixHash(h, constraint.Hash())
	}
	props := typePropertiesOf(constraint)
	props.containsTypeParam = true

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
