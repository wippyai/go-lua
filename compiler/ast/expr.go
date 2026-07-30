package ast

// Expr is the interface implemented by all expression nodes.
type Expr interface {
	PositionHolder
	exprMarker()
}

// ExprBase is the base struct embedded in all expression nodes.
type ExprBase struct {
	Node
}

func (expr *ExprBase) exprMarker() {}

// ConstExpr is the interface for compile-time constant expressions.
type ConstExpr interface {
	Expr
	constExprMarker()
}

// ConstExprBase is the base struct for constant expression nodes.
type ConstExprBase struct {
	ExprBase
}

func (expr *ConstExprBase) constExprMarker() {}

// TrueExpr represents the boolean literal true.
type TrueExpr struct {
	ConstExprBase
}

// FalseExpr represents the boolean literal false.
type FalseExpr struct {
	ConstExprBase
}

// NilExpr represents the nil literal.
type NilExpr struct {
	ConstExprBase
}

// NumberExpr represents a numeric literal.
type NumberExpr struct {
	ConstExprBase
	Value string // Raw numeric string from source
}

// StringExpr represents a string literal.
type StringExpr struct {
	ConstExprBase
	Value string // Parsed string value
}

// Comma3Expr represents the vararg expression (...).
type Comma3Expr struct {
	ExprBase
	AdjustRet bool // Whether return count should be adjusted
}

// IdentExpr represents an identifier reference.
type IdentExpr struct {
	ExprBase
	Value string // Identifier name
}

// AttrKeySyntax records how an AttrGetExpr key appeared in source.
type AttrKeySyntax uint8

const (
	// AttrKeyUnknown records unspecified syntax for hand-built or manual AST nodes.
	AttrKeyUnknown AttrKeySyntax = iota
	// AttrKeyDot is source dot syntax: obj.key.
	AttrKeyDot
	// AttrKeyIndex is source bracket syntax: obj[key].
	AttrKeyIndex
)

// AttrGetExpr represents a table field access (obj.key or obj[key]).
type AttrGetExpr struct {
	ExprBase
	Object    Expr          // Table expression
	Key       Expr          // Key expression
	KeySyntax AttrKeySyntax // Dot, bracket, or unspecified syntax for hand-built/manual ASTs
}

// TableExpr represents a table constructor ({...}).
type TableExpr struct {
	ExprBase
	Fields []*Field // Table fields
}

// FuncCallExpr represents a function call expression.
type FuncCallExpr struct {
	ExprBase
	Func     Expr   // Function to call
	Receiver Expr   // Method receiver (nil for regular calls)
	Method   string // Method name (empty for regular calls)
	// MethodPosition is the exact parser token position for Method.
	// It is zero for regular calls and manually-constructed method ASTs.
	MethodPosition Position
	Args           []Expr     // Call arguments
	TypeArgs       []TypeExpr // Explicit type arguments for generic calls
	AdjustRet      bool       // Whether return count should be adjusted
}

// CanProduceMultipleValues reports whether expr can expand to multiple Lua values
// when it appears in the final slot of an expression list.
func CanProduceMultipleValues(expr Expr) bool {
	switch expr.(type) {
	case *FuncCallExpr, *Comma3Expr:
		return true
	default:
		return false
	}
}

// LogicalOpExpr represents a logical operator (and, or).
type LogicalOpExpr struct {
	ExprBase
	Operator string // "and" or "or"
	Lhs      Expr   // Left operand
	Rhs      Expr   // Right operand
}

// RelationalOpExpr represents a relational operator (<, >, <=, >=, ==, ~=).
type RelationalOpExpr struct {
	ExprBase
	Operator string // Comparison operator
	Lhs      Expr   // Left operand
	Rhs      Expr   // Right operand
}

// StringConcatOpExpr represents string concatenation (..).
type StringConcatOpExpr struct {
	ExprBase
	Lhs Expr // Left operand
	Rhs Expr // Right operand
}

// ArithmeticOpExpr represents an arithmetic operator (+, -, *, /, //, %, ^, &, |, ~, <<, >>).
type ArithmeticOpExpr struct {
	ExprBase
	Operator string // Arithmetic operator
	Lhs      Expr   // Left operand
	Rhs      Expr   // Right operand
}

// UnaryMinusOpExpr represents unary negation (-x).
type UnaryMinusOpExpr struct {
	ExprBase
	Expr Expr // Operand
}

// UnaryNotOpExpr represents logical negation (not x).
type UnaryNotOpExpr struct {
	ExprBase
	Expr Expr // Operand
}

// UnaryLenOpExpr represents the length operator (#x).
type UnaryLenOpExpr struct {
	ExprBase
	Expr Expr // Operand
}

// UnaryBNotOpExpr represents bitwise negation (~x).
type UnaryBNotOpExpr struct {
	ExprBase
	Expr Expr // Operand
}

// FunctionExpr represents a function expression (function(...) ... end).
type FunctionExpr struct {
	ExprBase
	TypeParams  []TypeParamExpr // Generic type parameters <T, U>
	ParList     *ParList        // Parameter list
	ReturnTypes []TypeExpr      // Declared return types (nil = inferred)
	// ReturnsKnown distinguishes an omitted return annotation from an authored
	// empty `: ()` return clause.
	ReturnsKnown bool
	Stmts        []Stmt // Function body
}

// KeyName extracts a string key name from an expression.
// Returns the string value for StringExpr and IdentExpr, empty string otherwise.
func KeyName(key Expr) string {
	switch k := key.(type) {
	case *StringExpr:
		return k.Value
	case *IdentExpr:
		return k.Value
	default:
		return ""
	}
}
