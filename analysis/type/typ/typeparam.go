package typ

import (
	"github.com/wippyai/go-lua/analysis/internal/hash"
	"github.com/wippyai/go-lua/analysis/type/kind"
)

// TypeParam represents a type parameter in a generic type or function.
//
// Type parameters enable parametric polymorphism: a single definition that
// works with multiple types. The Constraint field bounds what types can
// instantiate this parameter (nil means any type is allowed).
//
// Example: In Array<T>, T is a TypeParam with Name="T" and Constraint=nil.
type TypeParam struct {
	Name                  string
	Constraint            Type // Upper bound (nil means any type)
	hash                  uint64
	containsAny           bool
	containsNever         bool
	containsTypeParam     bool
	containsInstantiated  bool
	containsRecursive     bool
	containsOpenRecursive bool
}

// NewTypeParam creates a type parameter.
func NewTypeParam(name string, constraint Type) *TypeParam {
	h := hash.MixHash(uint64(kind.TypeParam), hash.FnvString(name))
	if constraint != nil {
		h = hash.MixHash(h, constraint.Hash())
	}

	return &TypeParam{
		Name:                  name,
		Constraint:            constraint,
		hash:                  h,
		containsAny:           knownContainsAny(constraint),
		containsNever:         knownContainsNever(constraint),
		containsTypeParam:     true,
		containsInstantiated:  knownContainsInstantiated(constraint),
		containsRecursive:     knownContainsRecursive(constraint),
		containsOpenRecursive: knownContainsOpenRecursive(constraint),
	}
}

func (t *TypeParam) Kind() kind.Kind { return kind.TypeParam }
func (t *TypeParam) String() string {
	if t.Constraint != nil {
		return t.Name + " : " + t.Constraint.String()
	}

	return t.Name
}
func (t *TypeParam) Hash() uint64 { return t.hash }
func (t *TypeParam) Equals(other Type) bool {
	if other.Kind() != kind.TypeParam {
		return false
	}

	ot := other.(*TypeParam)
	if t.Name != ot.Name {
		return false
	}

	if (t.Constraint == nil) != (ot.Constraint == nil) {
		return false
	}

	if t.Constraint != nil && !t.Constraint.Equals(ot.Constraint) {
		return false
	}

	return true
}
