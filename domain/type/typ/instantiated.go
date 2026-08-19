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
	// equalityHashCache memoizes the structural hash once every Generic
	// declaration reachable from this application has a body. A self
	// application (e.g. List<T> = {..., tail: List<T>}) is lawfully built
	// while its own Generic.Body is still nil, so the hash cannot be sealed at
	// construction; Hash reads through this cache exactly as EqualityHash
	// does, and both publish the same value once the graph closes.
	equalityHashCache *equalityHashCache
	// typeProperties.containsAny/containsNever/containsRecursive are a
	// construction-time snapshot and are never read for this kind: the same
	// staleness that motivates equalityHashCache applies to them (the
	// snapshot can predate Generic.SetBody), so knownContainsAny/Never/
	// Recursive read the Generic's own self-healing flags plus TypeArgs live
	// instead. containsTypeParam/containsGeneric/containsInstantiated/
	// containsOpenRecursive are unaffected: they are either always-true for
	// this kind or already answered from a live scan.
	typeProperties
	strCache stringCache
}

// Instantiate creates an instantiated generic type with the given arguments.
func Instantiate(g *Generic, args ...Type) *Instantiated {
	props := typePropertiesOf(g)
	props.containsInstantiated = true
	h := hash.MixHash(uint64(kind.Instantiated), g.Hash())
	for _, a := range args {
		props.include(a)
		h = hash.MixHash(h, a.Hash())
	}

	inst := &Instantiated{
		Generic:           g,
		TypeArgs:          args,
		equalityHashCache: &equalityHashCache{},
		typeProperties:    props,
	}
	// h is computed eagerly from g's and each argument's CURRENT hash - cheap,
	// since a closed child's Hash is an O(1) cache read. It is published here
	// only when g's declaration (and everything else inst reaches) is already
	// closed: an Instantiated is immutable once built, so a closed graph here
	// can never later become open, and a value published now is therefore
	// permanent and safe to reuse from any position. When g is still open
	// (the self-application case), h is left unpublished and Hash instead
	// falls back to the close-gated recompute once the graph does close.
	cacheEqualityHash(inst, h, true)
	zzProbeConstructLazy(uint64(kind.Instantiated), inst.Hash) // ZZPROBE
	return inst
}

func (i *Instantiated) Kind() kind.Kind { return kind.Instantiated }
func (i *Instantiated) String() string {
	return i.strCache.get(func() string { return renderTypeString(i) })
}

// Hash returns the structural hash. It is derived lazily rather than sealed
// at construction: an Instantiated built during a self application can
// predate its own Generic's SetBody, so the value is read through the same
// close-gated cache as EqualityHash instead of a field frozen at construction.
func (i *Instantiated) Hash() uint64 {
	if i == nil {
		return 0
	}
	return closeGatedHash(i)
}
func (i *Instantiated) Equals(other Type) bool {
	return typeEquals(i, other)
}
