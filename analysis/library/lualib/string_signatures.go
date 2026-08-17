package lualib

import (
	"github.com/wippyai/go-lua/analysis/domain/type/normalize"
	typetable "github.com/wippyai/go-lua/analysis/domain/type/table"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
	"github.com/wippyai/go-lua/analysis/domain/type/typeexpr"
	"github.com/wippyai/go-lua/analysis/module/signature"
)

// The authored typed application of every string-library export. Each is the
// global form - string.upper(s), string.sub(s, i, j) - whose first parameter is
// the string operand, which is what makes the colon-method form need no
// signature of its own: the metatable edge reaches the same exported value, and
// the receiver binds as that first operand.
//
// No export carries an effect label. A string operation reads its operand and
// produces a new string; it stores nothing, mutates nothing, and sends nothing
// across a boundary, so the row is empty because there is nothing to state and
// not because the row was left unwritten.

// stringCaptureValue is what one Lua pattern capture produces: a position
// capture "()" yields an integer, every other capture yields a string.
var stringCaptureValue = typeexpr.Union(typ.String, typ.Integer)

// stringGeneralCaptureSlots is the capture arity a NON-literal pattern is
// modeled with. A pattern known only at runtime can produce arbitrarily many
// captures, and a function type has a finite result row, so the general iterator
// publishes this many optional slots. A literal pattern is not modeled here at
// all: how many slots it has and what each carries is caller data, which is why
// the pattern exports delegate their result selection to a rule.
const stringGeneralCaptureSlots = 4

// stringGsubReplacement is the replacement argument of string.gsub: a literal
// replacement string, a table looked up by the capture, or a function called
// with the capture.
var stringGsubReplacement = typeexpr.Union(
	typ.String,
	typetable.NewMap(typ.Any, typ.Any),
	typ.Func().
		Param("capture", stringCaptureValue).
		Returns(typeexpr.Union(typ.String, typ.Number, typ.LiteralBool(false), typ.Nil)).
		Build(),
)

// stringGeneralIterator is the iterator string.gmatch and string.gfind return
// for a pattern that is not a literal.
func stringGeneralIterator() *typ.Function {
	slots := make([]typ.Type, stringGeneralCaptureSlots)
	for index := range slots {
		slots[index] = normalize.Optional(stringCaptureValue)
	}
	return typ.Func().Returns(slots...).Build()
}

var stringSignatures = map[string]signature.Function{
	"byte": authored(typ.Func().
		Param("s", typ.String).
		OptParam("i", typ.Integer).
		OptParam("j", typ.Integer).
		Returns(normalize.Optional(typ.Integer)).
		Build()),
	"char": authored(typ.Func().
		Variadic(typ.Integer).
		Returns(typ.String).
		Build()),
	"dump": authored(typ.Func().
		Param("function", typ.Any).
		OptParam("strip", typ.Boolean).
		Returns(typ.Never).
		Build()),
	"find": authored(typ.Func().
		Param("s", typ.String).
		Param("pattern", typ.String).
		OptParam("init", typ.Integer).
		OptParam("plain", typ.Boolean).
		Returns(normalize.Optional(typ.Integer), normalize.Optional(typ.Integer)).
		Build()),
	"format": authored(typ.Func().
		Param("formatstring", typ.String).
		Variadic(typ.Any).
		Returns(typ.String).
		Build()),
	"gfind": authored(typ.Func().
		Param("s", typ.String).
		Param("pattern", typ.String).
		Returns(stringGeneralIterator(), typ.Any).
		Build()),
	"gmatch": authored(typ.Func().
		Param("s", typ.String).
		Param("pattern", typ.String).
		Returns(stringGeneralIterator()).
		Build()),
	"gsub": authored(typ.Func().
		Param("s", typ.String).
		Param("pattern", typ.String).
		Param("repl", stringGsubReplacement).
		OptParam("n", typ.Integer).
		Returns(typ.String, typ.Integer).
		Build()),
	"len": authored(typ.Func().
		Param("s", typ.String).
		Returns(typ.Integer).
		Build()),
	"lower": authored(typ.Func().
		Param("s", typ.String).
		Returns(typ.String).
		Build()),
	"match": authored(typ.Func().
		Param("s", typ.String).
		Param("pattern", typ.String).
		OptParam("init", typ.Integer).
		Returns(normalize.Optional(stringCaptureValue)).
		Build()),
	"pack": authored(typ.Func().
		Param("fmt", typ.String).
		Variadic(typ.Any).
		Returns(typ.String).
		Build()),
	"packsize": authored(typ.Func().
		Param("fmt", typ.String).
		Returns(typ.Integer).
		Build()),
	"rep": authored(typ.Func().
		Param("s", typ.String).
		Param("n", typ.Integer).
		OptParam("sep", typ.String).
		Returns(typ.String).
		Build()),
	"reverse": authored(typ.Func().
		Param("s", typ.String).
		Returns(typ.String).
		Build()),
	"sub": authored(typ.Func().
		Param("s", typ.String).
		Param("i", typ.Integer).
		OptParam("j", typ.Integer).
		Returns(typ.String).
		Build()),
	"unpack": authored(typ.Func().
		Param("fmt", typ.String).
		Param("s", typ.String).
		OptParam("pos", typ.Integer).
		Returns(typ.Any).
		Build()),
	"upper": authored(typ.Func().
		Param("s", typ.String).
		Returns(typ.String).
		Build()),
}
