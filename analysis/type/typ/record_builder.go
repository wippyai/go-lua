package typ

// RecordBuilder provides a fluent API for constructing record types.
//
// Example:
//
//	rec := typ.NewRecord().
//	    Field("name", typ.String).
//	    OptField("age", typ.Integer).
//	    Build()
type RecordBuilder struct {
	fields        []Field
	staticMembers []StaticMember
	metatable     Type
	mapKey        Type
	mapValue      Type
	open          bool
}

// NewRecord starts building a record type.
func NewRecord() *RecordBuilder {
	return &RecordBuilder{}
}

// Metatable sets the metatable type.
func (b *RecordBuilder) Metatable(t Type) *RecordBuilder {
	b.metatable = t
	return b
}

// SetOpen marks the record as open (unknown field access returns unknown).
func (b *RecordBuilder) SetOpen(open bool) *RecordBuilder {
	b.open = open
	return b
}

// MapComponent sets the map component key and value types.
func (b *RecordBuilder) MapComponent(key, value Type) *RecordBuilder {
	b.mapKey = key
	b.mapValue = value
	return b
}

// Build creates the record type.
func (b *RecordBuilder) Build() *Record {
	return buildRecordType(b.fields, b.staticMembers, b.metatable, b.mapKey, b.mapValue, b.open, false, false)
}

// NewFreshEmptyRecord creates the transient empty-table-literal seed ({}).
//
// It is a zero-field record (so under IsSubtype it is exactly the closed empty
// record), but Fresh is folded into hash/equals so it is distinct from an
// ordinary empty record. As the gradual bottom of the table lattice it is
// admitted by subtype.Consistent against any table-like target an empty table
// can satisfy (see emptyTableSatisfies).
func NewFreshEmptyRecord() *Record {
	return buildRecordType(nil, nil, nil, nil, nil, false, true, true)
}
