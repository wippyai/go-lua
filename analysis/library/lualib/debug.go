package lualib

import (
	"github.com/wippyai/go-lua/analysis/domain/type/normalize"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
	"github.com/wippyai/go-lua/analysis/library/contract"
	"github.com/wippyai/go-lua/analysis/module/signature"
	"github.com/wippyai/go-lua/analysis/schema/library"
)

// DebugRoot is the authored mount selector of the debug library.
const DebugRoot = "debug"

// debugExports is the authored export inventory of the debug library, in
// canonical order. It has one member, and the shortness is the statement: the
// rest of the Lua debug library reaches into frames, locals and metatables that
// a Wippy actor may not touch, so the host boots the aggregate with those
// members refused. A refusal is what the initial environment booted, so it is
// the environment contract that publishes it and not this one.
var debugExports = []string{"getupvalue"}

// DebugExports returns a copy of the authored export inventory.
func DebugExports() []string { return copyNames(debugExports) }

// debugSignatures is the authored typed application of the debug library's one
// export. debug.getupvalue answers with the name of an upvalue and the value it
// holds, and answers with nothing when the function has no upvalue at that
// index, so both slots are optional together.
var debugSignatures = map[string]signature.Function{
	"getupvalue": authored(typ.Func().
		Param("f", typ.Any).
		Param("up", typ.Integer).
		Returns(normalize.Optional(typ.String), typ.Any).
		Build()),
}

// DebugContract authors the debug library contract instance against one declared
// library kind. The root carries what the library is: a mutable aggregate.
// Nothing is deferred.
func DebugContract(kind *library.Entry) (*contract.Instance, bool) {
	return librarySpec{
		Root:       DebugRoot,
		Exports:    debugExports,
		Signatures: debugSignatures,
		Aggregate:  contract.Aggregate(contract.MutabilityMutable),
	}.instance(kind)
}
