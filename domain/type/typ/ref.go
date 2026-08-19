package typ

import (
	"sync/atomic"

	"github.com/wippyai/go-lua/domain/type/kind"
	"github.com/wippyai/go-lua/internal/hash"
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
	zzProbeConstruct(uint64(kind.Ref), h) // ZZPROBE
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
	Name string // Alias name
	// Underlying type.
	Target Type
	// unaliasedMemo caches the fully-flattened target so UnaliasedTarget
	// never re-walks a chain it has already resolved. Aliases built through
	// NewAlias populate it at construction; an Alias assembled some other
	// way resolves and publishes it on first use. Either way the memo is
	// write-once: duplicate first-use computation from concurrent readers is
	// harmless since flattening is a pure function of Target, and only the
	// atomic publish matters for visibility.
	unaliasedMemo atomic.Pointer[Type]
	hash          uint64
	typeProperties
}

// NewAlias creates a type alias.
func NewAlias(name string, target Type) *Alias {
	h := EqualityHash(target)

	a := &Alias{
		Name:           name,
		Target:         target,
		hash:           h,
		typeProperties: typePropertiesOf(target),
	}
	resolved := flattenAliasTarget(target)
	a.unaliasedMemo.Store(&resolved)
	zzProbeConstruct(uint64(kind.Alias), h) // ZZPROBE
	return a
}

func (a *Alias) Kind() kind.Kind { return kind.Alias }
func (a *Alias) String() string  { return a.Name }
func (a *Alias) Hash() uint64 {
	if a == nil {
		return 0
	}
	return EqualityHash(a.UnaliasedTarget())
}

// UnaliasedTarget returns the type Target chains to once every intermediate
// Alias layer is peeled away. The result is cached: repeated calls, including
// the ones subtype checking makes once per recursion level, cost O(1) after
// the chain has been resolved the first time, regardless of chain length.
func (a *Alias) UnaliasedTarget() Type {
	if a == nil {
		return nil
	}
	if cached := a.unaliasedMemo.Load(); cached != nil {
		return *cached
	}
	resolved := flattenAliasTarget(a.Target)
	a.unaliasedMemo.Store(&resolved)
	return resolved
}

// Equals compares structurally through the alias target.
func (a *Alias) Equals(other Type) bool {
	return typeEquals(a.Target, other)
}

// flattenAliasTarget walks target through its Alias layers to the first
// non-Alias type (or the point a cycle re-enters), consulting each
// intermediate Alias's own memo (an atomic load, never a computation trigger)
// so a chain already resolved from another entry point is not re-walked.
func flattenAliasTarget(target Type) Type {
	current := target
	var seen typePath
	for {
		alias, ok := current.(*Alias)
		if !ok || alias == nil {
			return current
		}
		if !seen.enter(alias) {
			return current
		}
		next := alias.Target
		if cached := alias.unaliasedMemo.Load(); cached != nil {
			next = *cached
		}
		if next == nil || next == current {
			return current
		}
		current = next
	}
}

// Meta represents a metatype (the type of a type value).
//
// Meta types are used when types themselves are values, such as in
// type predicates or reflection. Meta{Of: T} represents the type of
// a runtime value that carries type T.
//
// Example: typeof(Point) has type Meta{Of: Point}
type Meta struct {
	// The type being wrapped.
	Of   Type
	hash uint64
	typeProperties
	strCache stringCache
}

// NewMeta creates a metatype.
func NewMeta(of Type) *Meta {
	h := hash.MixHash(uint64(kind.Meta), of.Hash())
	zzProbeConstruct(uint64(kind.Meta), h) // ZZPROBE
	return &Meta{
		Of:             of,
		hash:           h,
		typeProperties: typePropertiesOf(of),
	}
}

func (m *Meta) Kind() kind.Kind { return kind.Meta }
func (m *Meta) String() string {
	return m.strCache.get(func() string { return renderTypeString(m) })
}
func (m *Meta) Hash() uint64 { return m.hash }
func (m *Meta) Equals(other Type) bool {
	if other.Kind() != kind.Meta {
		return false
	}

	return m.Of.Equals(other.(*Meta).Of)
}
