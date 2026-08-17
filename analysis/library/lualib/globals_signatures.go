package lualib

import (
	"github.com/wippyai/go-lua/analysis/domain/effect"
	"github.com/wippyai/go-lua/analysis/domain/effect/dispatch"
	"github.com/wippyai/go-lua/analysis/domain/effect/iteration"
	"github.com/wippyai/go-lua/analysis/domain/effect/ownership"
	"github.com/wippyai/go-lua/analysis/domain/effect/postcondition"
	"github.com/wippyai/go-lua/analysis/domain/effect/returns"
	"github.com/wippyai/go-lua/analysis/domain/type/normalize"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
	"github.com/wippyai/go-lua/analysis/module/signature"
)

// The authored typed application of every environment slot whose initial value
// is a callable, and the effect row each one exercises.
//
// The refinement assert publishes is one of those effect labels and rides this
// envelope. It is stated ONCE, in the row: a refinement predicated on a call
// returning normally is a postcondition of the application, so publishing a
// second member form beside the envelope would be one fact under two authorities
// with nothing to keep them agreeing.

// globalsTypeName is the complete Lua type-name result domain of type(v). It is
// kept as literals so a call-result consumer retains the exact contract instead
// of weakening the result to an arbitrary string. The operation's own identity -
// the fact that its result is the runtime family of a caller value, which no
// signature can state - is published separately as the intrinsic marker.
var globalsTypeName = typ.MaterializeUnion([]typ.Type{
	typ.LiteralString("nil"),
	typ.LiteralString("boolean"),
	typ.LiteralString("number"),
	typ.LiteralString("string"),
	typ.LiteralString("table"),
	typ.LiteralString("function"),
	typ.LiteralString("thread"),
	typ.LiteralString("userdata"),
})

var globalsSignatures = map[string]signature.Function{
	"assert": authored(typ.Func().
		Param("v", typ.Any).
		OptParam("message", typ.Any).
		Returns(typ.Any).
		Build(),
		postcondition.NormalReturnRefinement{
			Target:     effect.ParamRef{Index: 0},
			Refinement: postcondition.Present{},
		}),
	"collectgarbage": authored(typ.Func().
		OptParam("opt", typ.String).
		OptParam("arg", typ.Any).
		Returns(typ.Any).
		Build()),
	"error": authored(typ.Func().
		Param("message", typ.Any).
		OptParam("level", typ.Integer).
		Returns(typ.Never).
		Build()),
	"getmetatable": authored(typ.Func().
		Param("object", typ.Any).
		Returns(normalize.Optional(typ.Any)).
		Build()),
	"ipairs": authored(typ.Func().
		Param("t", typ.Any).
		Returns(typ.Any, typ.Any, typ.Integer).
		Build(),
		iteration.Iterator{
			Source: effect.ParamRef{Index: 0},
			Kind:   iteration.IterateIndexed,
		}),
	"next": authored(typ.Func().
		Param("table", typ.Any).
		OptParam("index", typ.Any).
		Returns(normalize.Optional(typ.Any), normalize.Optional(typ.Any)).
		Build()),
	"pairs": authored(typ.Func().
		Param("t", typ.Any).
		Returns(typ.Any, typ.Any, typ.Nil).
		Build(),
		iteration.Iterator{
			Source: effect.ParamRef{Index: 0},
			Kind:   iteration.IterateKeyed,
		}),
	"pcall": authored(typ.Func().
		Param("f", typ.Any).
		Variadic(typ.Any).
		Returns(typ.Boolean, typ.Any).
		Build(),
		ownership.BorrowAll{},
		returns.Return{
			ReturnIndex: 1,
			Transform:   returns.CallbackReturn{CallbackParam: effect.ParamRef{Index: 0}},
		}),
	"print": authored(typ.Func().
		Variadic(typ.Any).
		Build(),
		ownership.BorrowAll{}),
	"rawequal": authored(typ.Func().
		Param("v1", typ.Any).
		Param("v2", typ.Any).
		Returns(typ.Boolean).
		Build(),
		ownership.BorrowAll{}),
	"rawget": authored(typ.Func().
		Param("table", typ.Any).
		Param("index", typ.Any).
		Returns(typ.Any).
		Build(),
		ownership.BorrowAll{}),
	"rawlen": authored(typ.Func().
		Param("v", typ.Any).
		Returns(typ.Integer).
		Build(),
		ownership.BorrowAll{}),
	"rawset": authored(typ.Func().
		Param("table", typ.Any).
		Param("index", typ.Any).
		Param("value", typ.Any).
		Returns(typ.Any).
		Build(),
		ownership.Store{
			Param: effect.ParamRef{Index: 2},
			Into:  effect.ParamRef{Index: 0},
		}),
	"require": authored(typ.Func().
		Param("modname", typ.String).
		Returns(typ.Any).
		Build(),
		dispatch.ModuleLoad{}),
	// select(index, ...) answers with the count of its variadic tail when index
	// is the literal "#", and with a member of that tail otherwise. The dynamic
	// half is all a type can say; the literal half is published as the result
	// refinement this slot carries.
	"select": authored(typ.Func().
		Param("index", typ.Any).
		Variadic(typ.Any).
		Returns(typ.Any).
		Build()),
	"setmetatable": globalsSetMetatable(),
	"tonumber": authored(typ.Func().
		Param("v", typ.Any).
		OptParam("base", typ.Integer).
		Returns(normalize.Optional(typ.Number)).
		Build(),
		ownership.BorrowAll{}),
	"tostring": authored(typ.Func().
		Param("v", typ.Any).
		Returns(typ.String).
		Build(),
		ownership.BorrowAll{}),
	"type": authored(typ.Func().
		Param("v", typ.Any).
		Returns(globalsTypeName).
		Build(),
		ownership.BorrowAll{}),
	"unpack": globalsUnpack(),
	"xpcall": authored(typ.Func().
		Param("f", typ.Any).
		Param("msgh", typ.Any).
		Variadic(typ.Any).
		Returns(typ.Boolean, typ.Any).
		Build(),
		ownership.BorrowAll{},
		returns.Return{
			ReturnIndex: 1,
			Transform:   returns.CallbackReturn{CallbackParam: effect.ParamRef{Index: 0}},
		}),
}

// globalsSetMetatable answers with the table it was handed rather than with a
// fresh one, so the result is the argument's own type parameter and the row
// states that the returned value IS the first argument.
func globalsSetMetatable() signature.Function {
	subject := typ.NewTypeParam("T", nil)
	return authored(typ.Func().
		TypeParamRef(subject).
		Param("table", subject).
		Param("metatable", normalize.Optional(typ.Any)).
		Returns(subject).
		Build(),
		ownership.Retain{Param: effect.ParamRef{Index: 1}},
		returns.Return{ReturnIndex: 0, Transform: returns.SameAs{Source: effect.ParamRef{Index: 0}}})
}

func globalsUnpack() signature.Function {
	elem := typ.NewTypeParam("T", nil)
	return authored(typ.Func().
		TypeParamRef(elem).
		Param("list", typ.NewArray(elem)).
		OptParam("i", typ.Integer).
		OptParam("j", typ.Integer).
		Returns(normalize.Optional(elem)).
		Build(),
		ownership.BorrowAll{})
}
