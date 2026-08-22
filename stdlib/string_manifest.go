package stdlib

import (
	"github.com/wippyai/go-lua/domain/effect/ownership"
	"github.com/wippyai/go-lua/domain/type/normalize"
	typetable "github.com/wippyai/go-lua/domain/type/table"
	"github.com/wippyai/go-lua/domain/type/typ"
	"github.com/wippyai/go-lua/domain/type/typeexpr"
	moduleio "github.com/wippyai/go-lua/manifest/wire"
)

const stringMetatableRoot = "StringMetatableRoot"

var stringCapture = typeexpr.Union(typ.String, typ.Number)

var stringReplacement = typeexpr.Union(
	typ.String,
	typetable.NewMap(typ.Any, typ.Any),
	// gsub applies the replacement function to the captures of each match, or
	// to the whole match when the pattern declares none, so the arm always
	// receives one capture and may receive more.
	typ.Func().Param("capture", stringCapture).Variadic(stringCapture).
		Returns(typeexpr.Union(typ.String, typ.Number, typ.LiteralBool(false), typ.Nil)).Build(),
)

func stringDeclaration() declaration {
	iterator := typ.Func().Returns(
		normalize.Optional(stringCapture), normalize.Optional(stringCapture),
		normalize.Optional(stringCapture), normalize.Optional(stringCapture),
	).Build()
	return declaration{aliases: map[string]string{"gfind": "string.gmatch"}, detached: stringDetachedFunctions(), signatures: map[string]declaredFunction{
		"byte": withResultTail(authored(typ.Func().
			Param("s", typ.String).OptParam("i", typ.Integer).OptParam("j", typ.Integer).
			Build(), ownership.BorrowAll{}), typ.Integer).operational(stringByteOperationLaw()),
		"char": authored(typ.Func().Variadic(typ.Integer).Returns(typ.String).Build()).operational(stringCharOperationLaw()),
		"dump": authored(typ.Func().
			Param("function", typ.Any).Returns(typ.Never).Build()),
		"find": withResultTail(authored(typ.Func().
			Param("s", typ.String).Param("pattern", typ.String).
			OptParam("init", typ.Integer).OptParam("plain", typ.Boolean).
			Returns(normalize.Optional(typ.Integer), normalize.Optional(typ.Integer)).Build(),
			ownership.BorrowAll{}), typ.Any).operational(stringFindOperationLaw()),
		"format": authored(typ.Func().
			Param("format", typ.String).Variadic(typ.Any).Returns(typ.String).Build(),
			ownership.BorrowAll{}).operational(replacement(formatProfile())),
		"gmatch": openAuthored("stdlib.string.gmatch.iterator", typ.Func().
			Param("s", typ.String).Param("pattern", typ.String).
			Returns(iterator, typ.Any).Build(), ownership.BorrowAll{}).operational(stringGmatchOperationLaw()),
		"gsub": openAuthored("stdlib.string.gsub.replacement", typ.Func().
			Param("s", typ.String).Param("pattern", typ.String).
			Param("repl", stringReplacement).OptParam("n", typ.Integer).
			Returns(typ.String, typ.Number).Build(), ownership.BorrowAll{}).operational(replacement(callbackGsubProfile())),
		"len": authored(typ.Func().Param("s", typ.String).Returns(typ.Number).Build(),
			ownership.BorrowAll{}),
		"lower": authored(typ.Func().Param("s", typ.String).Returns(typ.String).Build(),
			ownership.BorrowAll{}),
		"match": withResultTail(authored(typ.Func().
			Param("s", typ.String).Param("pattern", typ.String).OptParam("init", typ.Integer).
			Build(), ownership.BorrowAll{}), typ.Any).operational(stringMatchOperationLaw()),
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
		// unpack answers the values the format describes followed by the index
		// of the first unread byte. That trailing integer is end-anchored
		// behind an open tail, and the sealed result vocabulary addresses a
		// result by its fixed ordinal only, so the declaration states the tail
		// it can carry; the seal refuses a suffix in this position by name.
		"unpack": withResultTail(authored(typ.Func().
			Param("fmt", typ.String).Param("s", typ.String).OptParam("pos", typ.Integer).
			Build(), ownership.BorrowAll{}), typ.Any),
		"upper": authored(typ.Func().Param("s", typ.String).Returns(typ.String).Build(),
			ownership.BorrowAll{}),
	}, values: map[string]typ.Type{
		// OpenString publishes the module itself under this visible field while
		// also using it as the string metatable's index target.
		"__index": typ.Any,
	}, initialRoots: []moduleio.InitialRoot{
		{Identity: stringMetatableRoot, Aggregate: moduleio.InitialAggregateMetatable},
	}, initialEntries: []moduleio.InitialEntry{
		{
			Root: moduleio.DeclaredInitialRoot(stringMetatableRoot), Key: "__index",
			Value: moduleio.InitialRootValue(moduleio.ProviderModuleRoot()), Mutability: moduleio.InitialMutable,
		},
		{
			Root: moduleio.ProviderModuleRoot(), Key: "__index",
			Value: moduleio.InitialRootValue(moduleio.ProviderModuleRoot()), Mutability: moduleio.InitialMutable,
		},
	}, initialMetatables: []moduleio.InitialMetatableAttachment{
		{Primitive: moduleio.InitialPrimitiveString, Metatable: moduleio.DeclaredInitialRoot(stringMetatableRoot)},
	}}
}
