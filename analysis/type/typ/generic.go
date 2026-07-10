package typ

import (
	"strings"

	"github.com/wippyai/go-lua/analysis/internal/hash"
	"github.com/wippyai/go-lua/analysis/type/kind"
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
	hash       uint64
	typeProperties
	strCache stringCache
}

// NewGeneric creates a generic type definition identified by name + type params
// + body structure.
func NewGeneric(name string, params []*TypeParam, body Type) *Generic {
	h := hash.MixHash(uint64(kind.Generic), hash.FnvString(name))
	for _, p := range params {
		h = hash.MixHash(h, p.Hash())
	}

	// The body participates in identity so two same-named declarations with
	// different bodies stay distinct, while two declarations of the same body
	// are one type regardless of which compilation produced them. A self-recursive
	// forward-reference body (nil at the time the placeholder is hashed) is left
	// out of the hash; structural equality still distinguishes such generics
	// through the coinductive body comparison.
	if body != nil {
		h = hash.MixHash(h, body.Hash())
	}

	copied := make([]*TypeParam, len(params))
	copy(copied, params)
	props := typePropertiesOfTypeParams(copied)
	props.include(body)

	return &Generic{
		Name:           name,
		TypeParams:     copied,
		Body:           body,
		hash:           h,
		typeProperties: props,
	}
}

// SetBody back-patches the body of a generic that was created as a forward
// reference (nil body) so the body can refer to the generic itself. The same
// node carries the declaration identity throughout body resolution, so a
// self-referential body and the top-level generic are the same node and the
// hash is finalized against the completed body. Intended for construction
// before the generic escapes into any interner; a no-op once a body is set.
func (g *Generic) SetBody(body Type) {
	if g == nil || g.Body != nil || body == nil {
		return
	}
	g.Body = body

	h := hash.MixHash(uint64(kind.Generic), hash.FnvString(g.Name))
	for _, p := range g.TypeParams {
		h = hash.MixHash(h, p.Hash())
	}
	h = hash.MixHash(h, body.Hash())
	g.hash = h

	g.include(body)
	g.strCache = stringCache{}
}

func (g *Generic) Kind() kind.Kind { return kind.Generic }
func (g *Generic) String() string {
	return g.strCache.get(func() string {
		var sb strings.Builder

		sb.WriteString(g.Name)
		sb.WriteString("<")

		for i, p := range g.TypeParams {
			if i > 0 {
				sb.WriteString(", ")
			}

			sb.WriteString(p.String())
		}

		sb.WriteString(">")

		return sb.String()
	})
}
func (g *Generic) Hash() uint64 { return g.hash }
func (g *Generic) Equals(other Type) bool {
	return typeEquals(g, other)
}
