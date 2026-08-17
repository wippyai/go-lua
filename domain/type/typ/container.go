package typ

import (
	"github.com/wippyai/go-lua/domain/type/kind"
	"github.com/wippyai/go-lua/internal/hash"
)

// Array represents a homogeneous sequence type: T[].
//
// The Element type describes what each element contains.
type Array struct {
	Element Type
	hash    uint64
	typeProperties
	strCache stringCache
}

// NewArray creates an array type.
func NewArray(elem Type) *Array {
	if elem == nil {
		elem = Unknown
	}
	h := hash.MixHash(uint64(kind.Array), elem.Hash())
	return &Array{
		Element:        elem,
		hash:           h,
		typeProperties: typePropertiesOf(elem),
	}
}

func (a *Array) Kind() kind.Kind { return kind.Array }
func (a *Array) String() string {
	return a.strCache.get(func() string { return renderTypeString(a) })
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
	Key   Type
	Value Type
	hash  uint64
	typeProperties
	strCache stringCache
}

// NewMap creates a map type.
func NewMap(key, value Type) *Map {
	return RebuildMap(key, value)
}

// RebuildMap rebuilds a hash-stable map node from already-computed key/value types.
func RebuildMap(key, value Type) *Map {
	key, value, h, props := canonicalMapParts(kind.Map, key, value)

	return &Map{
		Key:            key,
		Value:          value,
		hash:           h,
		typeProperties: props,
	}
}

func (m *Map) Kind() kind.Kind { return kind.Map }
func (m *Map) String() string {
	return m.strCache.get(func() string { return renderTypeString(m) })
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
	Key   Type
	Value Type
	hash  uint64
	typeProperties
	strCache stringCache
}

// NewReadonlyMap creates a read-only key/value view type.
func NewReadonlyMap(key, value Type) *ReadonlyMap {
	return RebuildReadonlyMap(key, value)
}

// RebuildReadonlyMap rebuilds a hash-stable read-only map node from already-computed key/value types.
func RebuildReadonlyMap(key, value Type) *ReadonlyMap {
	key, value, h, props := canonicalMapParts(kind.ReadonlyMap, key, value)

	return &ReadonlyMap{
		Key:            key,
		Value:          value,
		hash:           h,
		typeProperties: props,
	}
}

func canonicalMapParts(k kind.Kind, key, value Type) (Type, Type, uint64, typeProperties) {
	if key == nil {
		key = Unknown
	}
	if value == nil {
		value = Unknown
	}
	h := hash.MixHash(uint64(k), key.Hash())
	h = hash.MixHash(h, value.Hash())
	return key, value, h, typePropertiesOf(key, value)
}

func (m *ReadonlyMap) Kind() kind.Kind { return kind.ReadonlyMap }
func (m *ReadonlyMap) String() string {
	return m.strCache.get(func() string { return renderTypeString(m) })
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
	Elements []Type
	hash     uint64
	typeProperties
	strCache stringCache
}

// NewTuple creates a tuple type.
func NewTuple(elems ...Type) *Tuple {
	h := uint64(kind.Tuple)
	cleaned := make([]Type, len(elems))
	var props typeProperties
	for i, e := range elems {
		if e == nil {
			e = Unknown
		}
		cleaned[i] = e
		h = hash.MixHash(h, e.Hash())
		props.include(e)
	}

	return &Tuple{
		Elements:       cleaned,
		hash:           h,
		typeProperties: props,
	}
}

func (t *Tuple) Kind() kind.Kind { return kind.Tuple }

func (t *Tuple) String() string {
	return t.strCache.get(func() string { return renderTypeString(t) })
}

func (t *Tuple) Hash() uint64 { return t.hash }

func (t *Tuple) Equals(o Type) bool {
	return typeEquals(t, o)
}
