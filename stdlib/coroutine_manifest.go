package stdlib

import (
	"github.com/wippyai/go-lua/domain/effect/ownership"
	"github.com/wippyai/go-lua/domain/type/typ"
	"github.com/wippyai/go-lua/types/signature"
)

var coroutineStatus = typ.MaterializeUnion([]typ.Type{
	typ.LiteralString("suspended"), typ.LiteralString("dead"),
	typ.LiteralString("running"), typ.LiteralString("normal"),
})

func coroutineDeclaration() declaration {
	return declaration{signatures: map[string]signature.Function{
		"create": openAuthored("stdlib.coroutine.create.activation", typ.Func().
			Param("f", typ.Any).Returns(typ.Any).Build(), ownership.Send{FromParam: 0}),
		"resume": openAuthored("stdlib.coroutine.resume.control", typ.Func().
			Param("co", typ.Any).Variadic(typ.Any).Returns(typ.Boolean, typ.Any).Build(),
			ownership.Send{FromParam: 1}),
		"running": openAuthored("stdlib.coroutine.running.control", typ.Func().
			Returns(typ.Any).Build()),
		"spawn": openAuthored("stdlib.coroutine.spawn.system_yield", typ.Func().
			Param("f", typ.Any).Build(), ownership.Send{FromParam: 0}),
		"status": authored(typ.Func().
			Param("co", typ.Any).Returns(coroutineStatus).Build(), ownership.BorrowAll{}),
		"wrap": openAuthored("stdlib.coroutine.wrap.activation", typ.Func().
			Param("f", typ.Any).Returns(typ.Func().Variadic(typ.Any).Returns(typ.Any).Build()).Build(),
			ownership.Send{FromParam: 0}),
		"yield": openAuthored("stdlib.coroutine.yield.user", typ.Func().
			Variadic(typ.Any).Returns(typ.Any).Build(), ownership.Send{FromParam: 0}),
	}}
}
