package program

// Core algebra rows intentionally omit lexical owners. An attachment supplies
// the single authored root; recursive aliases can therefore be predeclared
// before their target tree is filled.
type typeParamRow struct {
	owner            Term
	name             Key
	constraint       Term
	constraintFilled bool
}

// PrimitiveKind is the complete parser-authored primitive type vocabulary.
// User spellings always travel through TypeRef, never an open primitive name.
type PrimitiveKind uint8

const (
	PrimitiveNil PrimitiveKind = iota + 1
	PrimitiveBoolean
	PrimitiveNumber
	PrimitiveInteger
	PrimitiveString
	PrimitiveFunction
	PrimitiveAny
	PrimitiveUnknown
	PrimitiveNever
	PrimitiveSelf
)

func (kind PrimitiveKind) valid() bool { return kind >= PrimitiveNil && kind <= PrimitiveSelf }

type primitiveTypeRow struct{ kind PrimitiveKind }

// LiteralKind is the closed parser-reachable literal type vocabulary. nil is
// deliberately absent: parser nil type syntax is Primitive("nil"), while a
// nil LiteralTypeExpr denotes malformed numeric source and must not be made a
// semantic nil type.
type LiteralKind uint8

const (
	LiteralBool LiteralKind = iota + 1
	LiteralInteger
	LiteralFloat
	LiteralString
)

type literalTypeRow struct {
	kind  LiteralKind
	exact Key
	bits  uint64
}
type arrayTypeRow struct {
	element  Term
	readonly bool
}
type mapTypeRow struct {
	key, value Term
	readonly   bool
}
type recordTypeRow struct {
	fields   recordFieldRange
	readonly bool
}
type recordFieldRange struct{ start, end uint32 }
type recordFieldRow struct {
	key      Key
	typ      Term
	nameSpan storedSpan
	optional bool
}

// RecordField is one exact authored record member. Key must be a TypeKey;
// it is metadata rather than an evaluated Lua key occurrence.
type RecordField struct {
	Key      Key
	Type     Term
	NameSpan Span
	Optional bool
}

// LiteralValue is a compact closed query result. FloatBits is the authored
// IEEE-754 payload rather than a normalized numeric key, retaining -0 and NaN
// payloads exactly.
type LiteralValue struct {
	Kind      LiteralKind
	Bool      bool
	Integer   int64
	FloatBits uint64
	String    string
}
type unaryTypeRow struct{ inner Term }
type termsTypeRow struct{ terms termRange }
type keyRange struct{ start, end uint32 }

// TypeRefState records resolution independently from the source spelling.
// A row can therefore preserve bare or qualified source for every state.
type TypeRefState uint8

const (
	TypeRefUnresolved TypeRefState = iota + 1
	TypeRefDeclaration
	TypeRefCanonicalPath
)

// target xor path: declaration resolution stores the exact declaration target;
// canonical module resolution stores its ordered Key path. pkg/name always
// retain the source spelling (pkg is zero for a bare source reference).
type typeRefRow struct {
	state     TypeRefState
	target    Term
	pkg, name Key
	path      keyRange
}
type genericTypeRow struct {
	base Term
	args termRange
}
