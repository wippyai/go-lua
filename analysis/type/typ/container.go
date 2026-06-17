package typ

import (
	"strings"

	"github.com/wippyai/go-lua/analysis/internal/hash"
	"github.com/wippyai/go-lua/analysis/type/kind"
)

// Array represents a homogeneous sequence type: T[].
//
// The Element type describes what each element contains.
type Array struct {
	Element               Type
	hash                  uint64
	containsAny           bool
	containsNever         bool
	containsTypeParam     bool
	containsInstantiated  bool
	containsRecursive     bool
	containsOpenRecursive bool
	strCache              stringCache
}

// NewArray creates an array type.
func NewArray(elem Type) *Array {
	if elem == nil {
		elem = Unknown
	}
	h := hash.MixHash(uint64(kind.Array), elem.Hash())
	return &Array{
		Element:               elem,
		hash:                  h,
		containsAny:           knownContainsAny(elem),
		containsNever:         knownContainsNever(elem),
		containsTypeParam:     knownContainsTypeParam(elem),
		containsInstantiated:  knownContainsInstantiated(elem),
		containsRecursive:     knownContainsRecursive(elem),
		containsOpenRecursive: knownContainsOpenRecursive(elem),
	}
}

func (a *Array) Kind() kind.Kind { return kind.Array }
func (a *Array) String() string {
	return a.strCache.get(func() string {
		if a.Element == nil {
			return "unknown[]"
		}
		return a.Element.String() + "[]"
	})
}
func (a *Array) Hash() uint64 { return a.hash }
func (a *Array) Equals(o Type) bool {
	return typeEquals(a, o)
}

// Map represents a homogeneous key-value mapping: {[K]: V}.
//
// Maps have uniform types for all entries rather than named fields with
// potentially different types.
type Map struct {
	Key                   Type
	Value                 Type
	hash                  uint64
	containsAny           bool
	containsNever         bool
	containsTypeParam     bool
	containsInstantiated  bool
	containsRecursive     bool
	containsOpenRecursive bool
	strCache              stringCache
}

// NewMap creates a map type.
func NewMap(key, value Type) *Map {
	return RebuildMap(key, value)
}

// RebuildMap rebuilds a hash-stable map node from already-computed key/value types.
func RebuildMap(key, value Type) *Map {
	if key == nil {
		key = Unknown
	}
	if value == nil {
		value = Unknown
	}
	h := hash.MixHash(uint64(kind.Map), key.Hash())
	h = hash.MixHash(h, value.Hash())

	return &Map{
		Key:                   key,
		Value:                 value,
		hash:                  h,
		containsAny:           knownAny(key, value),
		containsNever:         knownNever(key, value),
		containsTypeParam:     knownTypeParam(key, value),
		containsInstantiated:  knownInstantiated(key, value),
		containsRecursive:     knownRecursive(key, value),
		containsOpenRecursive: knownOpenRecursive(key, value),
	}
}

func (m *Map) Kind() kind.Kind { return kind.Map }
func (m *Map) String() string {
	return mapString(&m.strCache, m.Key, m.Value, "")
}

// mapString renders a map/readonly-map type and caches the result; prefix
// carries the readonly spelling.
func mapString(cache *stringCache, key, value Type, prefix string) string {
	return cache.get(func() string {
		ks, vs := "unknown", "unknown"
		if key != nil {
			ks = key.String()
		}
		if value != nil {
			vs = value.String()
		}
		return prefix + "{[" + ks + "]: " + vs + "}"
	})
}
func (m *Map) Hash() uint64 { return m.hash }
func (m *Map) Equals(o Type) bool {
	return typeEquals(m, o)
}

// ReadonlyMap represents a covariant read-only view of key/value table entries.
//
// It is used for obligations such as pairs(t): the body only enumerates present
// keys/values and does not gain the right to write arbitrary entries back through
// the value. Mutable Map remains invariant; ReadonlyMap is covariant in both key
// and value because it exposes reads only.
type ReadonlyMap struct {
	Key                   Type
	Value                 Type
	hash                  uint64
	containsAny           bool
	containsNever         bool
	containsTypeParam     bool
	containsInstantiated  bool
	containsRecursive     bool
	containsOpenRecursive bool
	strCache              stringCache
}

// NewReadonlyMap creates a read-only key/value view type.
func NewReadonlyMap(key, value Type) *ReadonlyMap {
	return RebuildReadonlyMap(key, value)
}

// RebuildReadonlyMap rebuilds a hash-stable read-only map node from already-computed key/value types.
func RebuildReadonlyMap(key, value Type) *ReadonlyMap {
	if key == nil {
		key = Unknown
	}
	if value == nil {
		value = Unknown
	}
	h := hash.MixHash(uint64(kind.ReadonlyMap), key.Hash())
	h = hash.MixHash(h, value.Hash())

	return &ReadonlyMap{
		Key:                   key,
		Value:                 value,
		hash:                  h,
		containsAny:           knownAny(key, value),
		containsNever:         knownNever(key, value),
		containsTypeParam:     knownTypeParam(key, value),
		containsInstantiated:  knownInstantiated(key, value),
		containsRecursive:     knownRecursive(key, value),
		containsOpenRecursive: knownOpenRecursive(key, value),
	}
}

func (m *ReadonlyMap) Kind() kind.Kind { return kind.ReadonlyMap }
func (m *ReadonlyMap) String() string {
	return mapString(&m.strCache, m.Key, m.Value, "readonly ")
}
func (m *ReadonlyMap) Hash() uint64 { return m.hash }
func (m *ReadonlyMap) Equals(o Type) bool {
	return typeEquals(m, o)
}

// Tuple represents a fixed-length heterogeneous sequence: (T1, T2, ...).
//
// Tuples are used for multi-value returns and destructuring assignments.
// Unlike Arrays, each position can have a different type and the length
// is fixed at compile time.
type Tuple struct {
	Elements              []Type
	hash                  uint64
	containsAny           bool
	containsNever         bool
	containsTypeParam     bool
	containsInstantiated  bool
	containsRecursive     bool
	containsOpenRecursive bool
	strCache              stringCache
}

// NewTuple creates a tuple type.
func NewTuple(elems ...Type) *Tuple {
	h := uint64(kind.Tuple)
	cleaned := make([]Type, len(elems))
	containsAny := false
	containsNever := false
	containsTypeParam := false
	containsInstantiated := false
	containsRecursive := false
	containsOpenRecursive := false
	for i, e := range elems {
		if e == nil {
			e = Unknown
		}
		cleaned[i] = e
		h = hash.MixHash(h, e.Hash())
		if !containsAny && knownContainsAny(e) {
			containsAny = true
		}
		if !containsNever && knownContainsNever(e) {
			containsNever = true
		}
		if !containsTypeParam && knownContainsTypeParam(e) {
			containsTypeParam = true
		}
		if !containsInstantiated && knownContainsInstantiated(e) {
			containsInstantiated = true
		}
		if !containsRecursive && knownContainsRecursive(e) {
			containsRecursive = true
		}
		if !containsOpenRecursive && knownContainsOpenRecursive(e) {
			containsOpenRecursive = true
		}
	}

	return &Tuple{
		Elements:              cleaned,
		hash:                  h,
		containsAny:           containsAny,
		containsNever:         containsNever,
		containsTypeParam:     containsTypeParam,
		containsInstantiated:  containsInstantiated,
		containsRecursive:     containsRecursive,
		containsOpenRecursive: containsOpenRecursive,
	}
}

func (t *Tuple) Kind() kind.Kind { return kind.Tuple }

func (t *Tuple) String() string {
	return t.strCache.get(func() string {
		parts := make([]string, len(t.Elements))
		for i, e := range t.Elements {
			if e == nil {
				parts[i] = "unknown"
			} else {
				parts[i] = e.String()
			}
		}
		return "(" + strings.Join(parts, ", ") + ")"
	})
}

func (t *Tuple) Hash() uint64 { return t.hash }

func (t *Tuple) Equals(o Type) bool {
	return typeEquals(t, o)
}
