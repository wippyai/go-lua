package ast

// TypeExpr represents a type annotation in the AST.
// Type expressions are used for type annotations, type declarations,
// and type-related operations in the typed Lua extension.
type TypeExpr interface {
	PositionHolder
	typeExprMarker()
}

// TypeExprBase provides the base implementation for all type expressions.
type TypeExprBase struct {
	Node
}

func (t *TypeExprBase) typeExprMarker() {}

// AnnotationExpr represents a type annotation like @min(0) or @pattern("^.+$")
type AnnotationExpr struct {
	Node
	Name string // "min", "max", "pattern", etc.
	Args []Expr // static argument expressions, in authored order
}

// AnnotatedTypeExpr decorates one authored type expression with runtime
// validation annotations. Its Inner remains the semantic type constructor;
// lowering emits annotation relations against that same inner Program term.
type AnnotatedTypeExpr struct {
	TypeExprBase
	Inner       TypeExpr
	Annotations []AnnotationExpr
}

// PrimitiveTypeExpr represents primitive types: number, string, boolean, nil, any, unknown, never, integer
type PrimitiveTypeExpr struct {
	TypeExprBase
	Name string // "number", "string", "boolean", "nil", "any", "unknown", "never", "integer"
}

// OptionalTypeExpr represents an optional type: T?
type OptionalTypeExpr struct {
	TypeExprBase
	Inner TypeExpr
}

// UnionTypeExpr represents a union type: A | B | C
type UnionTypeExpr struct {
	TypeExprBase
	Types []TypeExpr
}

// IntersectionTypeExpr represents an intersection type: A & B
type IntersectionTypeExpr struct {
	TypeExprBase
	Types []TypeExpr
}

// ArrayTypeExpr represents an array type: {T}
type ArrayTypeExpr struct {
	TypeExprBase
	Element  TypeExpr
	Readonly bool
}

// MapTypeExpr represents a map type: {K: V}
type MapTypeExpr struct {
	TypeExprBase
	Key      TypeExpr
	Value    TypeExpr
	Readonly bool
}

// RecordFieldExpr represents a field in a record type.
type RecordFieldExpr struct {
	Name         string
	NamePosition Position // Exact parser-owned field-name token position.
	Type         TypeExpr
	Optional     bool
}

// RecordTypeExpr represents a record/table type: {name: string, age: number}
type RecordTypeExpr struct {
	TypeExprBase
	Fields   []RecordFieldExpr
	Readonly bool
}

// TypeParamExpr represents a type parameter in generic types.
type TypeParamExpr struct {
	Name         string
	NamePosition Position // Exact parser-owned type-parameter declaration token.
	Constraint   TypeExpr // nil = any
}

// FunctionParamExpr represents a function parameter with optional name and type.
type FunctionParamExpr struct {
	Name         string   // parameter name (may be empty for anonymous)
	NamePosition Position // Exact parser-owned name token; zero for anonymous parameters.
	Type         TypeExpr // parameter type
}

// FunctionTypeExpr represents a function type: (A, B) -> (C, D)
type FunctionTypeExpr struct {
	TypeExprBase
	TypeParams       []TypeParamExpr
	Params           []FunctionParamExpr // fixed parameters only, in authored order
	Variadic         TypeExpr            // nil if the signature has no variadic tail
	VariadicPosition Position            // exact `...` token; zero when Variadic is nil
	Returns          []TypeExpr
}

// AssertsTypeExpr represents an assertion type expression: asserts x is T.
// Contextual validity is decided after parsing.
type AssertsTypeExpr struct {
	TypeExprBase
	ParamName     string   // the parameter being asserted
	ParamPosition Position // Exact parser-owned asserted-parameter token.
	NarrowTo      TypeExpr // the type to narrow to (nil means truthy/non-nil)
}

// TypeRefExpr represents a type reference: User, http.Request
type TypeRefExpr struct {
	TypeExprBase
	Path         []string // ["User"] or ["http", "Request"]
	RootPosition Position // Exact parser-owned first path token; zero for manually assembled AST.
}

// GenericTypeExpr represents an instantiated generic type: Array<T>, Map<K, V>
type GenericTypeExpr struct {
	TypeExprBase
	Base *TypeRefExpr
	Args []TypeExpr
}

// LiteralTypeExpr represents a literal type: "red" | 42 | true
type LiteralTypeExpr struct {
	TypeExprBase
	Value interface{} // string, float64, bool
}

// TypeOfExpr represents typeof(expr) - captures the inferred type of an expression.
//
// Unlike TypeScript which restricts typeof to identifiers, we follow Luau's approach
// allowing any expression. This enables patterns like:
//
//	type Config = typeof(defaultConfig)           -- capture variable type
//	type Person = typeof({ name = "", age = 0 })  -- capture table literal type
//	type API = typeof(require("api"))             -- capture module type
//
// During type checking, the Expr is synthesized and its type becomes the result.
type TypeOfExpr struct {
	TypeExprBase
	Expr Expr // the expression whose type to capture
}

// KeyOfExpr represents keyof T - extracts record keys as a union of string literals.
//
//	type Person = {name: string, age: number}
//	type Keys = keyof Person  -- "name" | "age"
//
// For non-record types, keyof returns never.
type KeyOfExpr struct {
	TypeExprBase
	Inner TypeExpr
}

// IndexAccessExpr represents T[K] - extracts the type of a field by key.
//
//	type Person = {name: string, age: number}
//	type NameType = Person["name"]  -- string
//
// Supports records, tuples (numeric index), and arrays.
type IndexAccessExpr struct {
	TypeExprBase
	Object TypeExpr // the type being indexed
	Index  TypeExpr // the key (usually a literal type)
}

// ConditionalTypeExpr represents T extends U ? A : B - type-level conditional.
//
//	type IsString<T> = T extends string ? true : false
//
// When T is a union, the conditional distributes over each member.
type ConditionalTypeExpr struct {
	TypeExprBase
	Check   TypeExpr // the type to check
	Extends TypeExpr // the constraint type
	Then    TypeExpr // result if Check extends Extends
	Else    TypeExpr // result if not
}

// TypeDefStmt represents a type alias declaration: type User = {name: string}
type TypeDefStmt struct {
	StmtBase
	Name         string
	NamePosition Position // Exact parser-owned declaration-name token.
	TypeParams   []TypeParamExpr
	Type         TypeExpr
}

// InterfaceMemberKind is the closed source vocabulary inside an interface
// declaration.  The ordered Members sequence is the sole member order
// authority; fields and methods are not held in parallel projections.
type InterfaceMemberKind uint8

const (
	InterfaceFieldMember InterfaceMemberKind = iota + 1
	InterfaceMethodMember
)

// InterfaceMember is one authored member of an interface declaration.
// Type is any parser-valid TypeExpr for a field and is exactly a
// *FunctionTypeExpr for a method. Optional is field-only.
type InterfaceMember struct {
	Kind         InterfaceMemberKind
	Name         string
	NamePosition Position // Exact parser-owned member-name token position.
	Type         TypeExpr
	Optional     bool
}

// InterfaceDefStmt represents an interface declaration: interface Serializable ... end
type InterfaceDefStmt struct {
	StmtBase
	Name         string
	NamePosition Position // Exact parser-owned declaration-name token.
	Extends      []*TypeRefExpr
	Members      []InterfaceMember
}

type CastSyntax uint8

const (
	// CastSyntaxUnknown records unspecified syntax for hand-built or manual AST nodes.
	CastSyntaxUnknown CastSyntax = iota
	// CastSyntaxAs records source `as` syntax.
	CastSyntaxAs
	// CastSyntaxColonColon records source `::` syntax.
	CastSyntaxColonColon
)

// CastExpr represents an unsafe type cast: expr as Type or expr :: Type.
type CastExpr struct {
	ExprBase
	Expr   Expr
	Type   TypeExpr
	Syntax CastSyntax
}

// NonNilAssertExpr represents a non-nil assertion: expr!
type NonNilAssertExpr struct {
	ExprBase
	Expr Expr
}
