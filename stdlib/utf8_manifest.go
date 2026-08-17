package stdlib

import (
	"github.com/wippyai/go-lua/domain/effect"
	"github.com/wippyai/go-lua/domain/effect/iteration"
	"github.com/wippyai/go-lua/domain/effect/ownership"
	"github.com/wippyai/go-lua/domain/type/normalize"
	"github.com/wippyai/go-lua/domain/type/typ"
)

func utf8Declaration() declaration {
	return declaration{detached: utf8DetachedFunctions(),
		signatures: map[string]declaredFunction{
			"char": authored(typ.Func().Variadic(typ.Integer).Returns(typ.String).Build()),
			"codepoint": withResultTail(authored(typ.Func().
				Param("s", typ.String).OptParam("i", typ.Integer).OptParam("j", typ.Integer).
				Build(), ownership.BorrowAll{}), typ.Integer).operational(utf8CodepointOperationLaw()),
			"codes": authored(typ.Func().
				Param("s", typ.String).Returns(typ.Any, typ.String, typ.Integer).Build(),
				ownership.BorrowAll{},
				iteration.Iterator{Source: effect.ParamRef{Index: 0}, Kind: iteration.IterateIndexed}).operational(utf8CodesOperationLaw()),
			"len": authored(typ.Func().
				Param("s", typ.String).OptParam("i", typ.Integer).OptParam("j", typ.Integer).
				Returns(normalize.Optional(typ.Integer), normalize.Optional(typ.Integer)).Build(),
				ownership.BorrowAll{}).operational(utf8LenOperationLaw()),
			"offset": authored(typ.Func().
				Param("s", typ.String).Param("n", typ.Integer).OptParam("i", typ.Integer).
				Returns(normalize.Optional(typ.Integer)).Build(), ownership.BorrowAll{}).operational(utf8OffsetOperationLaw()),
		},
		values: map[string]typ.Type{
			"charpattern": typ.LiteralString("[\x00-\x7F\xC2-\xFD][\x80-\xBF]*"),
		},
	}
}
