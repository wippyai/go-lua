package typ

import "github.com/wippyai/go-lua/analysis/type/kind"

// Record represents a product type with named fields: {field1: T1, field2: T2, ...}.
//
// Records support both structural typing (field presence/type matching) and
// optional map components for tables with dynamic indexing.
//
// Features:
//   - Open: When true, unknown field access returns Unknown instead of error
//   - MapKey/MapValue: Optional map component for {foo: T, [K]: V} patterns
//   - Metatable: Optional metatable type for metamethod resolution
//
// Fields are sorted by name for deterministic hashing and comparison.
type Record struct {
	Fields        []Field
	StaticMembers []StaticMember
	Metatable     Type // Metatable type for metamethod lookup
	MapKey        Type // Map component key type (nil if no map component)
	MapValue      Type // Map component value type (nil if no map component)
	Open          bool // Allow access to undefined fields
	// Fresh marks a transient empty-table-literal seed ({}). It is the gradual
	// bottom of the table lattice: invisible to IsSubtype (a Fresh empty record
	// behaves exactly as a closed empty record under <:) but admitted by
	// subtype.Consistent against any table-like target. Fresh is set only via
	// NewFreshEmptyRecord; rebuilt/merged records drop it.
	Fresh                 bool
	sorted                bool
	hash                  uint64
	containsAny           bool
	containsNever         bool
	containsTypeParam     bool
	containsInstantiated  bool
	containsRecursive     bool
	containsOpenRecursive bool
	strCache              stringCache
}

func (r *Record) Kind() kind.Kind { return kind.Record }

func (r *Record) Hash() uint64 { return r.hash }

func (r *Record) Equals(other Type) bool {
	return TypeEquals(r, other)
}
