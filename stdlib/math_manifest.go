package stdlib

import (
	"math"

	"github.com/wippyai/go-lua/domain/type/normalize"
	"github.com/wippyai/go-lua/domain/type/typ"
)

var mathNumberKind = typ.MaterializeUnion([]typ.Type{
	typ.LiteralString("integer"), typ.LiteralString("float"),
})

func mathDeclaration() declaration {
	unary := func(param string) declaredFunction {
		return authored(typ.Func().Param(param, typ.Number).Returns(typ.Number).Build())
	}
	binary := func(left, right string) declaredFunction {
		return authored(typ.Func().Param(left, typ.Number).Param(right, typ.Number).
			Returns(typ.Number).Build())
	}
	return declaration{
		signatures: map[string]declaredFunction{
			"abs": unary("x"), "acos": unary("x"), "asin": unary("x"),
			"atan":  unary("x"),
			"atan2": binary("y", "x"),
			"ceil":  unary("x"), "cos": unary("x"), "cosh": unary("x"),
			"deg": unary("x"), "exp": unary("x"), "floor": unary("x"),
			"fmod": binary("x", "y"),
			"frexp": authored(typ.Func().Param("x", typ.Number).
				Returns(typ.Number, typ.Number).Build()),
			"ldexp": authored(typ.Func().Param("m", typ.Number).Param("e", typ.Integer).
				Returns(typ.Number).Build()),
			"log": unary("x"), "log10": unary("x"),
			"max": mathExtremumSignature("max"), "min": mathExtremumSignature("min"),
			"mod": binary("x", "y"),
			"modf": authored(typ.Func().Param("x", typ.Number).
				Returns(typ.Number, typ.Number).Build()),
			"pow": binary("x", "y"),
			"rad": unary("x"),
			"random": openAuthored("stdlib.math.random.nondeterministic", typ.Func().
				OptParam("m", typ.Integer).OptParam("n", typ.Integer).
				Returns(typ.Number).Build()).operational(mathRandomOperationLaw()),
			// The rand/v2-backed implementation intentionally accepts and ignores
			// every argument: it is an observable no-op retained for compatibility.
			"randomseed": authored(typ.Func().Variadic(typ.Any).Build()),
			"sin":        unary("x"), "sinh": unary("x"), "sqrt": unary("x"),
			"tan": unary("x"), "tanh": unary("x"),
			"tointeger": authored(typ.Func().Param("x", typ.Any).
				Returns(normalize.Optional(typ.Integer)).Build()).operational(mathToIntegerOperationLaw()),
			"type": authored(typ.Func().Param("x", typ.Any).
				Returns(normalize.Optional(mathNumberKind)).Build()).operational(mathTypeOperationLaw()),
			"ult": authored(typ.Func().Param("m", typ.Integer).Param("n", typ.Integer).
				Returns(typ.Boolean).Build()),
		},
		values: map[string]typ.Type{
			"pi":         typ.LiteralNumber(math.Pi),
			"huge":       typ.LiteralNumber(math.MaxFloat64),
			"maxinteger": typ.LiteralInt(math.MaxInt64),
			"mininteger": typ.LiteralInt(math.MinInt64),
		},
	}
}

func mathExtremumSignature(name string) declaredFunction {
	number := typ.NewTypeParam("T", typ.Number)
	return authored(typ.Func().TypeParamRef(number).
		Param("x", number).Variadic(number).Returns(number).Build()).operational(replacement(minMaxProfile(module("math", name))))
}
