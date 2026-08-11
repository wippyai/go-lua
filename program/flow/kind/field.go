package kind

// FieldKind is the sole closed Lua table-constructor field vocabulary.
type FieldKind uint8

const (
	FieldList FieldKind = iota + 1
	FieldName
	FieldExact
	FieldKey
)
