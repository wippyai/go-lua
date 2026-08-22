package typ

import (
	"github.com/wippyai/go-lua/domain/type/kind"
	"github.com/wippyai/go-lua/internal/hash"
)

// Generic represents a parameterized type definition awaiting instantiation.
//
// Generics have type parameters that are substituted with concrete types
// when instantiated. The Body contains the type with TypeParam references.
//
// Identity:
//
// A generic is identified by its declaration content: name + type params + the
// structure of its body. Two declarations of the same name with the same body
// (e.g. one exported generic imported into two modules) are the same type, even
// across independent compilations; two same-named declarations with different
// bodies (a record `Box<T>` vs an interface `Box<T>`) stay distinct. This makes
// identity stable across the export/import round-trip without depending on an
// ephemeral per-compilation counter.
type Generic struct {
	Name       string       // Type name (empty for anonymous generics)
	TypeParams []*TypeParam // Type parameters to be substituted
	Body       Type         // Template type with TypeParam references
	// equalityHashCache memoizes the structural hash once every Generic
	// declaration reachable from this one (including itself) has a body. A
	// forward-declared generic is lawfully hashed with a nil Body before
	// SetBody runs; Hash reads through this cache exactly as EqualityHash
	// does, so both self-heal to the same value once the declaration and any
	// mutually recursive peers all close.
	equalityHashCache *equalityHashCache
	typeProperties
	strCache stringCache
}

// NewGeneric creates a generic type definition identified by name + type params
// + body structure.
func NewGeneric(name string, params []*TypeParam, body Type) *Generic {
	copied := make([]*TypeParam, len(params))
	copy(copied, params)
	props := typePropertiesOfTypeParams(copied)
	props.include(body)

	h := hash.MixHash(uint64(kind.Generic), hash.FnvString(name))
	for _, p := range copied {
		h = hash.MixHash(h, p.Hash())
	}
	if body != nil {
		h = hash.MixHash(h, body.Hash())
	}

	g := &Generic{
		Name:              name,
		TypeParams:        copied,
		Body:              body,
		equalityHashCache: &equalityHashCache{},
		typeProperties:    props,
	}
	// h is computed eagerly and, when body is non-nil, published immediately:
	// body cannot reference g (g's pointer does not exist until this literal
	// returns), so a body supplied here directly can never make g
	// self-referential, and a body-less placeholder is caught by the canonical
	// columns closure proof and left unpublished until SetBody. A
	// self-referential declaration is only reachable through SetBody, whose
	// own comment covers why publishing there is instead left to Hash's
	// close-gated fallback.
	cacheEqualityHash(g, h, true)
	zzProbeConstructLazy(uint64(kind.Generic), g.Hash) // ZZPROBE
	return g
}

// SetBody back-patches the body of a generic that was created as a forward
// reference (nil body) so the body can refer to the generic itself. The same
// node carries the declaration identity throughout body resolution, so a
// self-referential body and the top-level generic are the same node and the
// hash is finalized against the completed body once it is read. Intended for
// construction before the generic escapes into any interner. The body is a
// sealed fact and is written once; a second call with a non-nil body panics.
func (g *Generic) SetBody(body Type) {
	if g == nil || body == nil {
		return
	}
	if g.Body != nil {
		panic("typ: Generic.SetBody: body already sealed")
	}
	g.Body = body
	g.typeProperties.invalidateColumns()
	g.typeProperties.include(body)
}

func (g *Generic) Kind() kind.Kind { return kind.Generic }
func (g *Generic) String() string {
	return g.strCache.get(func() string { return renderTypeString(g) })
}

// Hash returns the structural hash. It is derived lazily rather than sealed
// at construction or at SetBody: a mutually recursive declaration can still
// be open elsewhere in the graph when this generic's own body closes, so the
// value is read through the same close-gated cache as EqualityHash instead of
// a field written once and never revisited.
func (g *Generic) Hash() uint64 {
	if g == nil {
		return 0
	}
	return closeGatedHash(g)
}
func (g *Generic) Equals(other Type) bool {
	return typeEquals(g, other)
}
