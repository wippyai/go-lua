package stdlib

import (
	"github.com/wippyai/go-lua/domain/effect/ownership"
	"github.com/wippyai/go-lua/domain/type/typ"
)

var coroutineStatus = typ.MaterializeUnion([]typ.Type{
	typ.LiteralString("suspended"), typ.LiteralString("dead"),
	typ.LiteralString("running"), typ.LiteralString("normal"),
})

func coroutineDeclaration() declaration {
	return declaration{detached: coroutineDetachedFunctions(), signatures: map[string]declaredFunction{
		"create": openAuthored("stdlib.coroutine.create.activation", typ.Func().
			Param("f", typ.Any).Returns(typ.Any).Build(), ownership.Send{FromParam: 0}).operational(coroutineCreateOperationLaw()),
		"resume": openAuthored("stdlib.coroutine.resume.control", typ.Func().
			Param("co", typ.Any).Variadic(typ.Any).Returns(typ.Boolean, typ.Any).Build(),
			ownership.Send{FromParam: 1}).operational(replacement(resumeEnvelope())),
		"running": openAuthored("stdlib.coroutine.running.control", typ.Func().
			Returns(typ.Any).Build()),
		"spawn": openAuthored("stdlib.coroutine.spawn.system_yield", typ.Func().
			Param("f", typ.Any).Build(), ownership.Send{FromParam: 0}).operational(replacement(callbackSpawn())),
		"status": authored(typ.Func().
			Param("co", typ.Any).Returns(coroutineStatus).Build(), ownership.BorrowAll{}),
		"wrap": openAuthored("stdlib.coroutine.wrap.activation", typ.Func().
			Param("f", typ.Any).Returns(typ.Func().Variadic(typ.Any).Returns(typ.Any).Build()).Build(),
			ownership.Send{FromParam: 0}).operational(coroutineWrapOperationLaw()),
		"yield": openAuthored("stdlib.coroutine.yield.user", typ.Func().
			Variadic(typ.Any).Returns(typ.Any).Build(), ownership.Send{FromParam: 0}).operational(coroutineYieldOperationLaw()),
	}}
}
