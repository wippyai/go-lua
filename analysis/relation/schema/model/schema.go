package model

// ColumnSchema is the immutable logical declaration for one nominal column.
// The column's owner and relation fences remain part of ColumnID; this shape
// carries its owner-issued semantic TypeID but no storage offset or physical
// ordinal.
type ColumnSchema struct {
	id     ColumnID
	typeID TypeID
}

// DefineColumnSchema freezes one nominal column declaration. Zero and foreign
// identities remain representable for the independent checker to reject.
func DefineColumnSchema(id ColumnID, typeID TypeID) ColumnSchema {
	return ColumnSchema{id: id, typeID: typeID}
}

// Available reports whether the declaration has non-zero column and type
// identities. Semantic ownership and membership are checked independently.
func (schema ColumnSchema) Available() bool {
	return schema.id.Available() && schema.typeID.Available()
}

// ID returns the column's nominal identity.
func (schema ColumnSchema) ID() ColumnID { return schema.id }

// Relation returns the relation that owns the column.
func (schema ColumnSchema) Relation() RelationID { return schema.id.Relation() }

// Type returns the column's owner-issued semantic type identity.
func (schema ColumnSchema) Type() TypeID { return schema.typeID }

// RelationSchema is the immutable logical shape of one relation. Columns,
// keys, and the canonical ScopeID are nominal references; no physical
// position is represented. Column order is retained as authored because it is
// part of a typed row contract, while identity itself remains owner-issued and
// order-independent. The ScopeSchema definition lives in the enclosing
// execution-schema registry.
type RelationSchema struct {
	id      RelationID
	columns []ColumnID
	keys    []KeyID
	scope   ScopeID
}

// DefineRelationSchema freezes one relation shape. It is intentionally a
// structural definition: zero, foreign, and duplicate references remain
// representable for the independent checker to reject.
func DefineRelationSchema(id RelationID, columns []ColumnID, keys []KeyID, scope ScopeID) RelationSchema {
	return RelationSchema{
		id:      id,
		columns: copyColumns(columns),
		keys:    copyKeys(keys),
		scope:   scope,
	}
}

// Available reports whether the declaration has a non-zero relation and scope
// identity. Semantic ownership, membership, and duplicate checks are
// independent.
func (schema RelationSchema) Available() bool {
	return schema.id.Available() && schema.scope.Available()
}

// ID returns the relation's nominal identity.
func (schema RelationSchema) ID() RelationID { return schema.id }

// Owner returns the relation's issuing owner.
func (schema RelationSchema) Owner() OwnerID { return schema.id.Owner() }

// Columns returns a copy of the authored logical column IDs.
func (schema RelationSchema) Columns() []ColumnID { return copyColumns(schema.columns) }

// Keys returns a copy of the authored logical key IDs.
func (schema RelationSchema) Keys() []KeyID { return copyKeys(schema.keys) }

// Scope returns the canonical decision-scope identity. The corresponding
// ScopeSchema is held once by the enclosing execution schema registry.
func (schema RelationSchema) Scope() ScopeID { return schema.scope }

// HasColumn reports whether column belongs to this relation schema.
func (schema RelationSchema) HasColumn(column ColumnID) bool {
	if !schema.Available() || !column.Available() || column.Relation() != schema.id {
		return false
	}
	for _, candidate := range schema.columns {
		if candidate == column {
			return true
		}
	}
	return false
}

// HasKey reports whether key belongs to this relation schema.
func (schema RelationSchema) HasKey(key KeyID) bool {
	if !schema.Available() || !key.Available() || key.Relation() != schema.id {
		return false
	}
	for _, candidate := range schema.keys {
		if candidate == key {
			return true
		}
	}
	return false
}

// Equal compares immutable logical schema content.  Authored column and key
// order is significant to the typed row contract; physical order never
// enters this comparison.
func (schema RelationSchema) Equal(other RelationSchema) bool {
	if schema.id != other.id || schema.scope != other.scope {
		return false
	}
	return equalColumns(schema.columns, other.columns) && equalKeys(schema.keys, other.keys)
}

func copyColumns(source []ColumnID) []ColumnID {
	if len(source) == 0 {
		return nil
	}
	copyOf := make([]ColumnID, len(source))
	copy(copyOf, source)
	return copyOf
}

func copyKeys(source []KeyID) []KeyID {
	if len(source) == 0 {
		return nil
	}
	copyOf := make([]KeyID, len(source))
	copy(copyOf, source)
	return copyOf
}

func equalColumns(left, right []ColumnID) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func equalKeys(left, right []KeyID) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

// KeySchema is the immutable logical vector definition for one key identity.
// Unlike scope dimensions, vector order is meaningful and therefore retained.
type KeySchema struct {
	id      KeyID
	columns []ColumnID
}

// DefineKeySchema freezes one ordered key vector. Zero, foreign, and
// duplicate references remain representable for the independent checker.
func DefineKeySchema(id KeyID, columns []ColumnID) KeySchema {
	return KeySchema{id: id, columns: copyColumns(columns)}
}

// Available reports whether schema passed construction.
func (schema KeySchema) Available() bool { return schema.id.Available() }

// ID returns the key's nominal identity.
func (schema KeySchema) ID() KeyID { return schema.id }

// Relation returns the relation that owns the key.
func (schema KeySchema) Relation() RelationID { return schema.id.Relation() }

// Columns returns a copy of the ordered logical key vector.
func (schema KeySchema) Columns() []ColumnID { return copyColumns(schema.columns) }

// Equal compares immutable key schema content.
func (schema KeySchema) Equal(other KeySchema) bool {
	if schema.id != other.id {
		return false
	}
	return equalColumns(schema.columns, other.columns)
}

// ScopeSchema is an immutable decision-scope schema. Scope dimensions form a
// conjunction; authored order is retained so an independent checker can
// inspect duplicate and foreign references. No mask, bitset, ordinal, or
// physical address is represented here.
type ScopeSchema struct {
	id         ScopeID
	dimensions []ColumnID
}

// DefineScopeSchema freezes one decision-scope definition. Dimensions retain
// authored order and malformed foreign/duplicate references remain visible to
// the independent checker.
func DefineScopeSchema(id ScopeID, dimensions []ColumnID) ScopeSchema {
	return ScopeSchema{id: id, dimensions: copyColumns(dimensions)}
}

// Available reports whether the declaration has a non-zero scope identity.
// Semantic ownership, membership, and duplicate checks are independent.
func (schema ScopeSchema) Available() bool { return schema.id.Available() }

// ID returns the scope schema's nominal identity.
func (schema ScopeSchema) ID() ScopeID { return schema.id }

// Owner returns the authority that issued the scope schema.
func (schema ScopeSchema) Owner() OwnerID { return schema.id.Owner() }

// Dimensions returns a copy of canonical scope dimensions.
func (schema ScopeSchema) Dimensions() []ColumnID { return copyColumns(schema.dimensions) }

// HasDimension reports whether dimension belongs to this scope schema.
func (schema ScopeSchema) HasDimension(dimension ColumnID) bool {
	if !schema.Available() || !dimension.Available() || dimension.Owner() != schema.Owner() {
		return false
	}
	for _, candidate := range schema.dimensions {
		if candidate == dimension {
			return true
		}
	}
	return false
}

// Equal compares immutable scope schema content.
func (schema ScopeSchema) Equal(other ScopeSchema) bool {
	if schema.id != other.id {
		return false
	}
	return equalColumns(schema.dimensions, other.dimensions)
}
