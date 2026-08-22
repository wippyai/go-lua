package stdlib

import (
	"github.com/wippyai/go-lua/domain/effect"
	"github.com/wippyai/go-lua/domain/effect/iteration"
	"github.com/wippyai/go-lua/domain/effect/mutation"
	"github.com/wippyai/go-lua/domain/effect/ownership"
	"github.com/wippyai/go-lua/domain/effect/postcondition"
	"github.com/wippyai/go-lua/domain/effect/returns"
	"github.com/wippyai/go-lua/domain/runtimekind"
	"github.com/wippyai/go-lua/domain/type/normalize"
	"github.com/wippyai/go-lua/domain/type/typ"
	moduleio "github.com/wippyai/go-lua/manifest/wire"
)

var luaRuntimeTypeName = luaRuntimeTypeNameUnion()

func luaRuntimeTypeNameUnion() typ.Type {
	members := make([]typ.Type, 0, runtimekind.All.Members())
	for index := 0; ; index++ {
		kind, ok := runtimekind.All.MemberAt(index)
		if !ok {
			return typ.MaterializeUnion(members)
		}
		members = append(members, typ.LiteralString(kind.Spelling()))
	}
}

func baseDeclaration() declaration {
	return declaration{detached: baseDetachedFunctions(),
		signatures: map[string]declaredFunction{
			"assert": authored(typ.Func().
				Param("v", typ.Any).OptParam("message", typ.Any).
				Returns(typ.Any).Build(),
				postcondition.NormalReturnRefinement{
					Target: effect.ParamRef{Index: 0}, Refinement: postcondition.Present{},
				}),
			"error": authored(typ.Func().
				Param("message", typ.Any).OptParam("level", typ.Integer).
				Returns(typ.Never).Build()),
			"getmetatable": authored(typ.Func().
				Param("object", typ.Any).Returns(normalize.Optional(typ.Any)).Build(),
				ownership.BorrowAll{}),
			"ipairs": authored(typ.Func().
				Param("t", typ.Any).Returns(typ.Any, typ.Any, typ.Integer).Build(),
				iteration.Iterator{Source: effect.ParamRef{Index: 0}, Kind: iteration.IterateIndexed}).operational(ipairsOperationLaw()),
			"next": authored(typ.Func().
				Param("table", typ.Any).OptParam("index", typ.Any).
				Returns(normalize.Optional(typ.Any), normalize.Optional(typ.Any)).Build(),
				ownership.BorrowAll{}),
			"pairs": authored(typ.Func().
				Param("t", typ.Any).Returns(typ.Any, typ.Any, typ.Nil).Build(),
				iteration.Iterator{Source: effect.ParamRef{Index: 0}, Kind: iteration.IterateKeyed}).operational(pairsOperationLaw()),
			"pcall": openAuthored("stdlib.base.pcall.callback", typ.Func().
				Param("f", typ.Any).Variadic(typ.Any).
				Returns(typ.Boolean, typ.Any).Build(),
				ownership.BorrowAll{},
				returns.Return{ReturnIndex: 1, Transform: returns.CallbackReturn{
					CallbackParam: effect.ParamRef{Index: 0},
				}}).operational(replacement(pcallProfile(nativeBuiltin("pcall")))),
			"print": openAuthored("stdlib.base.print.io", typ.Func().
				Variadic(typ.Any).Build(), ownership.BorrowAll{}).operational(replacement(printProfile())),
			"rawequal": authored(typ.Func().
				Param("v1", typ.Any).Param("v2", typ.Any).Returns(typ.Boolean).Build(),
				ownership.BorrowAll{}),
			"rawget": authored(typ.Func().
				Param("table", typ.Any).Param("index", typ.Any).Returns(typ.Any).Build(),
				ownership.BorrowAll{}),
			"rawset": authored(typ.Func().
				Param("table", typ.Any).Param("index", typ.Any).Param("value", typ.Any).
				Returns(typ.Any).Build(),
				ownership.Store{Param: effect.ParamRef{Index: 2}, Into: effect.ParamRef{Index: 0}}).operational(amendment(aliasAmendment(0, 0, 0))),
			"select": authored(typ.Func().
				Param("index", typ.Any).Variadic(typ.Any).Returns(typ.Any).Build()),
			"setmetatable": baseSetMetatableSignature(),
			"tonumber": authored(typ.Func().
				Param("v", typ.Any).OptParam("base", typ.Integer).
				Returns(normalize.Optional(typ.Number)).Build(), ownership.BorrowAll{}),
			"tostring": openAuthored("stdlib.base.tostring.metamethod", typ.Func().
				Param("v", typ.Any).Returns(typ.String).Build(), ownership.BorrowAll{}).operational(replacement(tostringProfile())),
			"type": authored(typ.Func().
				Param("v", typ.Any).Returns(luaRuntimeTypeName).Build(), ownership.BorrowAll{}).operational(moduleio.Operation{
				Behavior: &moduleio.OperationBehavior{
					Results: []moduleio.OperationResult{{
						Outcome:  0,
						Result:   0,
						Source:   moduleio.InputSource{Kind: moduleio.InputSourceValue, Ordinal: 0},
						Relation: string(runtimekind.RuntimeKindResultRelationKey),
					}},
					Predicates: []moduleio.OperationPredicate{{
						Outcome:  0,
						Result:   0,
						Subject:  moduleio.InputSource{Kind: moduleio.InputSourceValue, Ordinal: 0},
						Relation: string(runtimekind.RuntimeKindPredicateRelationKey),
					}},
				},
			}),
			"unpack": authored(typ.Func().
				Param("list", typ.Any).OptParam("i", typ.Integer).OptParam("j", typ.Integer).
				Returns(typ.Any).Build(), ownership.BorrowAll{}).operational(replacement(tableUnpackProfile())),
			"xpcall": openAuthored("stdlib.base.xpcall.callback", typ.Func().
				Param("f", typ.Any).Param("msgh", typ.Any).
				Returns(typ.Boolean, typ.Any).Build(),
				ownership.BorrowAll{},
				returns.Return{ReturnIndex: 1, Transform: returns.CallbackReturn{
					CallbackParam: effect.ParamRef{Index: 0},
				}}).operational(replacement(xpcallProfile(nativeBuiltin("xpcall")))),
		},
		values: map[string]typ.Type{
			"_G":                  typ.BuiltinTableTopMarker(),
			"_VERSION":            typ.LiteralString(LanguageVersion),
			"_GOPHER_LUA_VERSION": typ.LiteralString(ImplementationString),
		},
	}
}

func baseSetMetatableSignature() declaredFunction {
	subject := typ.NewTypeParam("T", nil)
	return authored(typ.Func().
		TypeParamRef(subject).
		Param("object", subject).
		Param("metatable", normalize.Optional(typ.Any)).
		Returns(subject).Build(),
		mutation.Mutate{Target: effect.ParamRef{Index: 0}, Transform: mutation.Unchanged{}},
		ownership.Retain{Param: effect.ParamRef{Index: 1}},
		returns.Return{ReturnIndex: 0, Transform: returns.SameAs{Source: effect.ParamRef{Index: 0}}}).operational(amendment(aliasAmendment(0, 0, 0)))
}
