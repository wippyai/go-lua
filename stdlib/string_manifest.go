package stdlib

import (
	"github.com/wippyai/go-lua/domain/effect/ownership"
	"github.com/wippyai/go-lua/domain/type/normalize"
	typetable "github.com/wippyai/go-lua/domain/type/table"
	"github.com/wippyai/go-lua/domain/type/typ"
	"github.com/wippyai/go-lua/domain/type/typeexpr"
	"github.com/wippyai/go-lua/types/signature"
)

var stringCapture = typeexpr.Union(typ.String, typ.Number)

var stringReplacement = typeexpr.Union(
	typ.String,
	typetable.NewMap(typ.Any, typ.Any),
	typ.Func().Param("capture", stringCapture).
		Returns(typeexpr.Union(typ.String, typ.Number, typ.LiteralBool(false), typ.Nil)).Build(),
)

func stringDeclaration() declaration {
	iterator := typ.Func().Returns(
		normalize.Optional(stringCapture), normalize.Optional(stringCapture),
		normalize.Optional(stringCapture), normalize.Optional(stringCapture),
	).Build()
	return declaration{aliases: map[string]string{"gfind": "string.gmatch"}, signatures: map[string]signature.Function{
		"byte": withResultTail(authored(typ.Func().
			Param("s", typ.String).OptParam("i", typ.Integer).OptParam("j", typ.Integer).
			Build(), ownership.BorrowAll{}), typ.Integer),
		"char": authored(typ.Func().Variadic(typ.Integer).Returns(typ.String).Build()),
		"dump": authored(typ.Func().
			Param("function", typ.Any).Returns(typ.Never).Build()),
		"find": withResultTail(authored(typ.Func().
			Param("s", typ.String).Param("pattern", typ.String).
			OptParam("init", typ.Integer).OptParam("plain", typ.Boolean).
			Returns(normalize.Optional(typ.Integer), normalize.Optional(typ.Integer)).Build(),
			ownership.BorrowAll{}), typ.Any),
		"format": authored(typ.Func().
			Param("format", typ.String).Variadic(typ.Any).Returns(typ.String).Build(),
			ownership.BorrowAll{}),
		"gfind": openAuthored("stdlib.string.gmatch.iterator", typ.Func().
			Param("s", typ.String).Param("pattern", typ.String).
			Returns(iterator, typ.Any).Build(), ownership.BorrowAll{}),
		"gmatch": openAuthored("stdlib.string.gmatch.iterator", typ.Func().
			Param("s", typ.String).Param("pattern", typ.String).
			Returns(iterator, typ.Any).Build(), ownership.BorrowAll{}),
		"gsub": openAuthored("stdlib.string.gsub.replacement", typ.Func().
			Param("s", typ.String).Param("pattern", typ.String).
			Param("repl", stringReplacement).OptParam("n", typ.Integer).
			Returns(typ.String, typ.Number).Build(), ownership.BorrowAll{}),
		"len": authored(typ.Func().Param("s", typ.String).Returns(typ.Number).Build(),
			ownership.BorrowAll{}),
		"lower": authored(typ.Func().Param("s", typ.String).Returns(typ.String).Build(),
			ownership.BorrowAll{}),
		"match": withResultTail(authored(typ.Func().
			Param("s", typ.String).Param("pattern", typ.String).OptParam("init", typ.Integer).
			Build(), ownership.BorrowAll{}), typ.Any),
		"pack": authored(typ.Func().
			Param("fmt", typ.String).Variadic(typ.Any).Returns(typ.String).Build(),
			ownership.BorrowAll{}),
		"packsize": authored(typ.Func().
			Param("fmt", typ.String).Returns(typ.Integer).Build(), ownership.BorrowAll{}),
		"rep": authored(typ.Func().
			Param("s", typ.String).Param("n", typ.Integer).Returns(typ.String).Build(),
			ownership.BorrowAll{}),
		"reverse": authored(typ.Func().Param("s", typ.String).Returns(typ.String).Build(),
			ownership.BorrowAll{}),
		"sub": authored(typ.Func().
			Param("s", typ.String).Param("i", typ.Integer).OptParam("j", typ.Integer).
			Returns(typ.String).Build(), ownership.BorrowAll{}),
		"unpack": withResults(authored(typ.Func().
			Param("fmt", typ.String).Param("s", typ.String).OptParam("pos", typ.Integer).
			Build(), ownership.BorrowAll{}), typ.Any, typ.Integer),
		"upper": authored(typ.Func().Param("s", typ.String).Returns(typ.String).Build(),
			ownership.BorrowAll{}),
	}, values: map[string]typ.Type{
		// OpenString publishes the module itself under this visible field while
		// also using it as the string metatable's index target.
		"__index": typ.Any,
	}}
}
