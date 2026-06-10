package typ

import (
	"strings"

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
	h := hash.HashCombine(uint64(kind.TypeParam), hash.FnvString(name))
	if constraint != nil {
		h = hash.HashCombine(h, constraint.Hash())
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
	Name                  string       // Type name (empty for anonymous generics)
	TypeParams            []*TypeParam // Type parameters to be substituted
	Body                  Type         // Template type with TypeParam references
	hash                  uint64
	containsAny           bool
	containsNever         bool
	containsTypeParam     bool
	containsInstantiated  bool
	containsRecursive     bool
	containsOpenRecursive bool
	strCache              stringCache
}

// NewGeneric creates a generic type definition identified by name + type params
// + body structure.
func NewGeneric(name string, params []*TypeParam, body Type) *Generic {
	h := hash.HashCombine(uint64(kind.Generic), hash.FnvString(name))
	for _, p := range params {
		h = hash.HashCombine(h, p.Hash())
	}

	// The body participates in identity so two same-named declarations with
	// different bodies stay distinct, while two declarations of the same body
	// are one type regardless of which compilation produced them. A self-recursive
	// forward-reference body (nil at the time the placeholder is hashed) is left
	// out of the hash; structural equality still distinguishes such generics
	// through the coinductive body comparison.
	if body != nil {
		h = hash.HashCombine(h, body.Hash())
	}

	copied := make([]*TypeParam, len(params))
	copy(copied, params)

	return &Generic{
		Name:                  name,
		TypeParams:            copied,
		Body:                  body,
		hash:                  h,
		containsAny:           knownAnyTypeParams(copied) || knownContainsAny(body),
		containsNever:         knownNeverTypeParams(copied) || knownContainsNever(body),
		containsTypeParam:     knownTypeParamTypeParams(copied) || knownContainsTypeParam(body),
		containsInstantiated:  knownInstantiatedTypeParams(copied) || knownContainsInstantiated(body),
		containsRecursive:     knownRecursiveTypeParams(copied) || knownContainsRecursive(body),
		containsOpenRecursive: knownOpenRecursiveTypeParams(copied) || knownContainsOpenRecursive(body),
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

	h := hash.HashCombine(uint64(kind.Generic), hash.FnvString(g.Name))
	for _, p := range g.TypeParams {
		h = hash.HashCombine(h, p.Hash())
	}
	h = hash.HashCombine(h, body.Hash())
	g.hash = h

	g.containsAny = g.containsAny || knownContainsAny(body)
	g.containsNever = g.containsNever || knownContainsNever(body)
	g.containsTypeParam = g.containsTypeParam || knownContainsTypeParam(body)
	g.containsInstantiated = g.containsInstantiated || knownContainsInstantiated(body)
	g.containsRecursive = g.containsRecursive || knownContainsRecursive(body)
	g.containsOpenRecursive = g.containsOpenRecursive || knownContainsOpenRecursive(body)
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
	return TypeEquals(g, other)
}

// Instantiated represents a generic type with concrete type arguments applied.
//
// When a generic like Array<T> is used as Array<number>, an Instantiated
// type is created with Generic=Array and TypeArgs=[number]. The body can
// be expanded by substituting type parameters with arguments.
type Instantiated struct {
	Generic               *Generic // The generic being instantiated
	TypeArgs              []Type   // Concrete types for each type parameter
	hash                  uint64
	softPrunable          bool
	containsAny           bool
	containsNever         bool
	containsTypeParam     bool
	containsInstantiated  bool
	containsRecursive     bool
	containsOpenRecursive bool
	strCache              stringCache
}

// Instantiate creates an instantiated generic type with the given arguments.
func Instantiate(g *Generic, args ...Type) *Instantiated {
	h := hash.HashCombine(uint64(kind.Instantiated), g.Hash())
	softPrunable := false
	containsAny := knownContainsAny(g)
	containsNever := knownContainsNever(g)
	containsTypeParam := knownContainsTypeParam(g)
	containsInstantiated := true
	containsRecursive := knownContainsRecursive(g)
	containsOpenRecursive := knownContainsOpenRecursive(g)
	for _, a := range args {
		h = hash.HashCombine(h, a.Hash())
		if !softPrunable && softPruneMayRewrite(a) {
			softPrunable = true
		}
		if !containsAny && knownContainsAny(a) {
			containsAny = true
		}
		if !containsNever && knownContainsNever(a) {
			containsNever = true
		}
		if !containsTypeParam && knownContainsTypeParam(a) {
			containsTypeParam = true
		}
		if !containsInstantiated && knownContainsInstantiated(a) {
			containsInstantiated = true
		}
		if !containsRecursive && knownContainsRecursive(a) {
			containsRecursive = true
		}
		if !containsOpenRecursive && knownContainsOpenRecursive(a) {
			containsOpenRecursive = true
		}
	}

	return &Instantiated{
		Generic:               g,
		TypeArgs:              args,
		hash:                  h,
		softPrunable:          softPrunable,
		containsAny:           containsAny,
		containsNever:         containsNever,
		containsTypeParam:     containsTypeParam,
		containsInstantiated:  containsInstantiated,
		containsRecursive:     containsRecursive,
		containsOpenRecursive: containsOpenRecursive,
	}
}

func (i *Instantiated) Kind() kind.Kind { return kind.Instantiated }
func (i *Instantiated) String() string {
	return i.strCache.get(func() string {
		var sb strings.Builder

		sb.WriteString(i.Generic.Name)
		sb.WriteString("<")

		for j, a := range i.TypeArgs {
			if j > 0 {
				sb.WriteString(", ")
			}

			sb.WriteString(a.String())
		}

		sb.WriteString(">")

		return sb.String()
	})
}
func (i *Instantiated) Hash() uint64 { return i.hash }
func (i *Instantiated) Equals(other Type) bool {
	return TypeEquals(i, other)
}

// TypeVar represents an inference variable during type checking.
//
// Type variables are placeholders created during generic instantiation
// and constraint solving. They are unified with concrete types as the
// checker gathers information. ID distinguishes different variables.
type TypeVar struct {
	ID   int
	hash uint64
}

// NewTypeVar creates a type variable with the given ID.
func NewTypeVar(id int) *TypeVar {
	h := hash.HashCombine(uint64(kind.TypeVar), uint64(id))
	return &TypeVar{ID: id, hash: h}
}

func (t *TypeVar) Kind() kind.Kind { return kind.TypeVar }
func (t *TypeVar) String() string  { return "$" + string(rune('a'+t.ID%26)) }
func (t *TypeVar) Hash() uint64    { return t.hash }
func (t *TypeVar) Equals(other Type) bool {
	if other.Kind() != kind.TypeVar {
		return false
	}

	return t.ID == other.(*TypeVar).ID
}
