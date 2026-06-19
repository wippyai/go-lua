package typ

import (
	"github.com/wippyai/go-lua/analysis/internal/hash"
	"github.com/wippyai/go-lua/analysis/type/kind"
)

// Ref represents an unresolved reference to a named type definition.
//
// Refs are created during parsing when a type name is encountered before
// its definition. They are resolved to the actual type during semantic
// analysis using the Module path to locate the definition.
//
// Local refs (Module == "") refer to types in the current module.
// Cross-module refs include the module path for import resolution.
type Ref struct {
	Module string // Module path (empty for local references)
	Name   string // Type name
	hash   uint64
	str    string
}

// NewRef creates a type reference.
func NewRef(module, name string) *Ref {
	h := hash.MixHash(uint64(kind.Ref), hash.FnvString(module))
	h = hash.MixHash(h, hash.FnvString(name))

	str := name
	if module != "" {
		str = module + "." + name
	}
	return &Ref{Module: module, Name: name, hash: h, str: str}
}

func (r *Ref) Kind() kind.Kind { return kind.Ref }

func (r *Ref) String() string {
	return r.str
}

func (r *Ref) Hash() uint64 { return r.hash }

func (r *Ref) Equals(other Type) bool {
	if other.Kind() != kind.Ref {
		return false
	}

	or := other.(*Ref)

	return r.Module == or.Module && r.Name == or.Name
}

// Alias represents a named type alias.
//
// Aliases provide alternative names for types without creating new types.
// An Alias is structurally equivalent to its Target for subtyping and
// equality, but preserves the name for error messages and documentation.
//
// Example: type UserId = number creates Alias{Name: "UserId", Target: number}
type Alias struct {
	Name                  string // Alias name
	Target                Type   // Underlying type
	unaliased             Type
	hash                  uint64
	containsAny           bool
	containsNever         bool
	containsTypeParam     bool
	containsInstantiated  bool
	containsRecursive     bool
	containsOpenRecursive bool
}

// NewAlias creates a type alias.
func NewAlias(name string, target Type) *Alias {
	h := EqualityHash(target)

	return &Alias{
		Name:                  name,
		Target:                target,
		unaliased:             flattenAliasTarget(target),
		hash:                  h,
		containsAny:           knownContainsAny(target),
		containsNever:         knownContainsNever(target),
		containsTypeParam:     knownContainsTypeParam(target),
		containsInstantiated:  knownContainsInstantiated(target),
		containsRecursive:     knownContainsRecursive(target),
		containsOpenRecursive: knownContainsOpenRecursive(target),
	}
}

func (a *Alias) Kind() kind.Kind { return kind.Alias }
func (a *Alias) String() string  { return a.Name }
func (a *Alias) Hash() uint64 {
	if a == nil {
		return 0
	}
	return EqualityHash(a.UnaliasedTarget())
}

func (a *Alias) UnaliasedTarget() Type {
	if a == nil || a.unaliased == nil {
		return a.Target
	}
	return a.unaliased
}

// Equals compares structurally through the alias target.
func (a *Alias) Equals(other Type) bool {
	return typeEquals(a.Target, other)
}

func flattenAliasTarget(target Type) Type {
	current := target
	for depth := 0; depth < DefaultRecursionDepth; depth++ {
		alias, ok := current.(*Alias)
		if !ok || alias == nil {
			return current
		}
		next := alias.Target
		if alias.unaliased != nil {
			next = alias.unaliased
		}
		if next == nil || next == current {
			return current
		}
		current = next
	}
	return current
}

// Meta represents a metatype (the type of a type value).
//
// Meta types are used when types themselves are values, such as in
// type predicates or reflection. Meta{Of: T} represents the type of
// a runtime value that carries type T.
//
// Example: typeof(Point) has type Meta{Of: Point}
type Meta struct {
	Of                    Type // The type being wrapped
	hash                  uint64
	containsAny           bool
	containsNever         bool
	containsTypeParam     bool
	containsInstantiated  bool
	containsRecursive     bool
	containsOpenRecursive bool
	strCache              stringCache
}

// NewMeta creates a metatype.
func NewMeta(of Type) *Meta {
	h := hash.MixHash(uint64(kind.Meta), of.Hash())
	return &Meta{
		Of:                    of,
		hash:                  h,
		containsAny:           knownContainsAny(of),
		containsNever:         knownContainsNever(of),
		containsTypeParam:     knownContainsTypeParam(of),
		containsInstantiated:  knownContainsInstantiated(of),
		containsRecursive:     knownContainsRecursive(of),
		containsOpenRecursive: knownContainsOpenRecursive(of),
	}
}

func (m *Meta) Kind() kind.Kind { return kind.Meta }
func (m *Meta) String() string {
	return m.strCache.get(func() string {
		return "typeof(" + m.Of.String() + ")"
	})
}
func (m *Meta) Hash() uint64 { return m.hash }
func (m *Meta) Equals(other Type) bool {
	if other.Kind() != kind.Meta {
		return false
	}

	return m.Of.Equals(other.(*Meta).Of)
}
