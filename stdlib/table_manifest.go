package stdlib

import (
	"github.com/wippyai/go-lua/domain/effect"
	"github.com/wippyai/go-lua/domain/effect/mutation"
	"github.com/wippyai/go-lua/domain/effect/ownership"
	"github.com/wippyai/go-lua/domain/effect/returns"
	"github.com/wippyai/go-lua/domain/type/normalize"
	typetable "github.com/wippyai/go-lua/domain/type/table"
	"github.com/wippyai/go-lua/domain/type/typ"
	"github.com/wippyai/go-lua/types/signature"
)

func tableDeclaration() declaration {
	return declaration{signatures: map[string]signature.Function{
		"concat": authored(typ.Func().
			Param("list", typ.Any).OptParam("sep", typ.String).
			OptParam("i", typ.Integer).OptParam("j", typ.Integer).
			Returns(typ.String).Build(), ownership.BorrowAll{}),
		"create": openAuthored("stdlib.table.create.allocate", typ.Func().
			Param("narray", typ.Integer).Param("nhash", typ.Integer).
			Returns(typetable.NewRecord().Build()).Build()),
		"freeze": tableFreezeSignature(),
		"getn": authored(typ.Func().
			Param("list", typ.Any).Returns(typ.Number).Build(), ownership.BorrowAll{}),
		"insert": authored(typ.Func().
			Param("list", typ.Any).Param("pos_or_value", typ.Any).OptParam("value", typ.Any).Build(),
			mutation.TableMutator{Target: effect.ParamRef{Index: 0}, Value: effect.ParamRef{Index: -1}},
			mutation.LengthChange{Target: effect.ParamRef{Index: 0}, Delta: 1},
			ownership.Store{Param: effect.ParamRef{Index: -1}, Into: effect.ParamRef{Index: 0}}),
		"isfrozen": authored(typ.Func().
			Param("table", typ.Any).Returns(typ.Boolean).Build(), ownership.BorrowAll{}),
		"maxn": authored(typ.Func().
			Param("list", typ.Any).Returns(typ.Number).Build(), ownership.BorrowAll{}),
		"remove": tableRemoveSignature(),
		"sort": openAuthored("stdlib.table.sort.callback", typ.Func().
			Param("list", typ.Any).OptParam("comp", typ.Any).Build(),
			mutation.Mutate{Target: effect.ParamRef{Index: 0}, Transform: mutation.Unchanged{}}),
		"unpack": authored(typ.Func().
			Param("list", typ.Any).OptParam("i", typ.Integer).OptParam("j", typ.Integer).
			Returns(typ.Any).Build(), ownership.BorrowAll{}),
	}}
}

func tableFreezeSignature() signature.Function {
	subject := typ.NewTypeParam("T", nil)
	return openAuthored("stdlib.table.freeze.ownership", typ.Func().
		TypeParamRef(subject).Param("table", subject).Returns(subject).Build(),
		mutation.Mutate{Target: effect.ParamRef{Index: 0}, Transform: mutation.Unchanged{}},
		returns.Return{ReturnIndex: 0, Transform: returns.SameAs{Source: effect.ParamRef{Index: 0}}})
}

func tableRemoveSignature() signature.Function {
	element := typ.NewTypeParam("T", nil)
	return authored(typ.Func().
		TypeParamRef(element).Param("list", typ.NewArray(element)).
		OptParam("pos", typ.Integer).Returns(normalize.Optional(element)).Build(),
		mutation.Mutate{Target: effect.ParamRef{Index: 0}, Transform: mutation.Unchanged{}},
		mutation.LengthChange{Target: effect.ParamRef{Index: 0}, Delta: -1})
}
