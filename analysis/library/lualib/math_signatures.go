package lualib

import (
	"github.com/wippyai/go-lua/analysis/domain/type/normalize"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
	"github.com/wippyai/go-lua/analysis/module/signature"
)

// The authored typed application of every math-library export. No export
// carries an effect label: a math operation reads its numeric arguments and
// produces a number, so the row is empty because there is nothing to state.

// mathTypeName is the complete result domain of math.type: an argument that is
// a number is one of exactly two subkinds, and any other argument yields none.
// The names are literals so a caller keeps the exact contract instead of
// weakening the result to an arbitrary string.
var mathTypeName = typ.MaterializeUnion([]typ.Type{
	typ.LiteralString("integer"),
	typ.LiteralString("float"),
})

// mathExtremum is the shared application of math.max and math.min. Both answer
// with one of the arguments they were given rather than with a fresh number, so
// the result is the argument type parameter and not the number bound on it.
func mathExtremum() signature.Function {
	extremum := typ.NewTypeParam("T", typ.Number)
	return authored(typ.Func().
		TypeParamRef(extremum).
		Param("x", extremum).
		Variadic(extremum).
		Returns(extremum).
		Build())
}

var mathSignatures = map[string]signature.Function{
	"abs":   authored(typ.Func().Param("x", typ.Number).Returns(typ.Number).Build()),
	"acos":  authored(typ.Func().Param("x", typ.Number).Returns(typ.Number).Build()),
	"asin":  authored(typ.Func().Param("x", typ.Number).Returns(typ.Number).Build()),
	"atan":  authored(typ.Func().Param("y", typ.Number).OptParam("x", typ.Number).Returns(typ.Number).Build()),
	"atan2": authored(typ.Func().Param("y", typ.Number).Param("x", typ.Number).Returns(typ.Number).Build()),
	"ceil":  authored(typ.Func().Param("x", typ.Number).Returns(typ.Integer).Build()),
	"cos":   authored(typ.Func().Param("x", typ.Number).Returns(typ.Number).Build()),
	"cosh":  authored(typ.Func().Param("x", typ.Number).Returns(typ.Number).Build()),
	"deg":   authored(typ.Func().Param("x", typ.Number).Returns(typ.Number).Build()),
	"exp":   authored(typ.Func().Param("x", typ.Number).Returns(typ.Number).Build()),
	"floor": authored(typ.Func().Param("x", typ.Number).Returns(typ.Integer).Build()),
	"fmod":  authored(typ.Func().Param("x", typ.Number).Param("y", typ.Number).Returns(typ.Number).Build()),
	"frexp": authored(typ.Func().Param("x", typ.Number).Returns(typ.Number, typ.Integer).Build()),
	"ldexp": authored(typ.Func().Param("m", typ.Number).Param("e", typ.Integer).Returns(typ.Number).Build()),
	"log":   authored(typ.Func().Param("x", typ.Number).OptParam("base", typ.Number).Returns(typ.Number).Build()),
	"log10": authored(typ.Func().Param("x", typ.Number).Returns(typ.Number).Build()),
	"max":   mathExtremum(),
	"min":   mathExtremum(),
	"mod":   authored(typ.Func().Param("x", typ.Number).Param("y", typ.Number).Returns(typ.Number).Build()),
	"modf":  authored(typ.Func().Param("x", typ.Number).Returns(typ.Integer, typ.Number).Build()),
	"pow":   authored(typ.Func().Param("x", typ.Number).Param("y", typ.Number).Returns(typ.Number).Build()),
	"rad":   authored(typ.Func().Param("x", typ.Number).Returns(typ.Number).Build()),
	"random": authored(typ.Func().
		OptParam("m", typ.Integer).
		OptParam("n", typ.Integer).
		Returns(typ.Number).
		Build()),
	"randomseed": authored(typ.Func().
		OptParam("x", typ.Integer).
		OptParam("y", typ.Integer).
		Build()),
	"sin":       authored(typ.Func().Param("x", typ.Number).Returns(typ.Number).Build()),
	"sinh":      authored(typ.Func().Param("x", typ.Number).Returns(typ.Number).Build()),
	"sqrt":      authored(typ.Func().Param("x", typ.Number).Returns(typ.Number).Build()),
	"tan":       authored(typ.Func().Param("x", typ.Number).Returns(typ.Number).Build()),
	"tanh":      authored(typ.Func().Param("x", typ.Number).Returns(typ.Number).Build()),
	"tointeger": authored(typ.Func().Param("x", typ.Any).Returns(normalize.Optional(typ.Integer)).Build()),
	"type":      authored(typ.Func().Param("x", typ.Any).Returns(normalize.Optional(mathTypeName)).Build()),
	"ult":       authored(typ.Func().Param("m", typ.Integer).Param("n", typ.Integer).Returns(typ.Boolean).Build()),
}
