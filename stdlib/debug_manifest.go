package stdlib

import (
	"github.com/wippyai/go-lua/domain/effect"
	"github.com/wippyai/go-lua/domain/effect/mutation"
	"github.com/wippyai/go-lua/domain/effect/ownership"
	"github.com/wippyai/go-lua/domain/effect/returns"
	"github.com/wippyai/go-lua/domain/type/normalize"
	typetable "github.com/wippyai/go-lua/domain/type/table"
	"github.com/wippyai/go-lua/domain/type/typ"
)

func debugDeclaration() declaration {
	info := typetable.NewRecord().
		OptField("name", typ.String).
		Field("what", typ.String).
		Field("source", typ.String).
		Field("currentline", typ.Number).
		Field("nups", typ.Number).
		Field("linedefined", typ.Number).
		Field("lastlinedefined", typ.Number).
		Field("func", typ.Any).
		Build()
	return declaration{signatures: map[string]declaredFunction{
		"getinfo": openAuthored("stdlib.debug.getinfo.stack", typ.Func().
			Param("function_or_level", typ.Any).OptParam("what", typ.String).
			Returns(normalize.Optional(info)).Build(), ownership.BorrowAll{}),
		"getlocal": openAuthored("stdlib.debug.getlocal.stack", typ.Func().
			Param("level", typ.Integer).Param("index", typ.Integer).
			Returns(normalize.Optional(typ.String), normalize.Optional(typ.Any)).Build()),
		"getmetatable": authored(typ.Func().
			Param("object", typ.Any).Returns(normalize.Optional(typ.Any)).Build(),
			ownership.BorrowAll{}),
		"getupvalue": openAuthored("stdlib.debug.getupvalue.stack", typ.Func().
			Param("function", typ.Any).Param("index", typ.Integer).
			Returns(normalize.Optional(typ.String), normalize.Optional(typ.Any)).Build()).operational(debugGetUpvalueOperationLaw()),
		"setlocal": openAuthored("stdlib.debug.setlocal.stack", typ.Func().
			Param("level", typ.Integer).Param("index", typ.Integer).Param("value", typ.Any).
			Returns(normalize.Optional(typ.String)).Build(),
			ownership.Store{Param: effect.ParamRef{Index: 2}}),
		"setmetatable": debugSetMetatableSignature(),
		"setupvalue": openAuthored("stdlib.debug.setupvalue.stack", typ.Func().
			Param("function", typ.Any).Param("index", typ.Integer).Param("value", typ.Any).
			Returns(normalize.Optional(typ.String)).Build(),
			ownership.Store{Param: effect.ParamRef{Index: 2}}),
		"traceback": openAuthored("stdlib.debug.traceback.stack", typ.Func().
			OptParam("thread_or_message", typ.Any).OptParam("level", typ.Integer).
			Returns(typ.String).Build(), ownership.BorrowAll{}),
	}}
}

func debugSetMetatableSignature() declaredFunction {
	subject := typ.NewTypeParam("T", nil)
	return authored(typ.Func().TypeParamRef(subject).
		Param("object", subject).Param("metatable", normalize.Optional(typ.Any)).
		Returns(subject).Build(),
		mutation.Mutate{Target: effect.ParamRef{Index: 0}, Transform: mutation.Unchanged{}},
		ownership.Retain{Param: effect.ParamRef{Index: 1}},
		returns.Return{ReturnIndex: 0, Transform: returns.SameAs{Source: effect.ParamRef{Index: 0}}})
}
