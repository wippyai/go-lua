package typ

import (
	"github.com/wippyai/go-lua/domain/type/kind"
	"github.com/wippyai/go-lua/internal/hash"
)

// Instantiated represents a generic type with concrete type arguments applied.
//
// When a generic like Array<T> is used as Array<number>, an Instantiated
// type is created with Generic=Array and TypeArgs=[number]. The body can
// be expanded by substituting type parameters with arguments.
type Instantiated struct {
	Generic  *Generic // The generic being instantiated
	TypeArgs []Type   // Concrete types for each type parameter
	hash     uint64
	// equalityHashCache memoizes the refreshed structural hash used while a
	// generic declaration is being completed. The construction hash above is
	// intentionally cheap and can predate Generic.SetBody; EqualityHash uses
	// this revision-validated cache instead of rebuilding the recursive generic
	// graph for every interning comparison.
	equalityHashCache *equalityHashCache
	typeProperties
	strCache stringCache
}

// Instantiate creates an instantiated generic type with the given arguments.
func Instantiate(g *Generic, args ...Type) *Instantiated {
	h := hash.MixHash(uint64(kind.Instantiated), g.Hash())
	props := typePropertiesOf(g)
	props.containsInstantiated = true
	for _, a := range args {
		h = hash.MixHash(h, a.Hash())
		props.include(a)
	}

	return &Instantiated{
		Generic:           g,
		TypeArgs:          args,
		hash:              h,
		equalityHashCache: &equalityHashCache{},
		typeProperties:    props,
	}
}

func (i *Instantiated) Kind() kind.Kind { return kind.Instantiated }
func (i *Instantiated) String() string {
	return i.strCache.get(func() string { return renderTypeString(i) })
}
func (i *Instantiated) Hash() uint64 { return i.hash }
func (i *Instantiated) Equals(other Type) bool {
	return typeEquals(i, other)
}
