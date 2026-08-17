package lualib

import (
	"github.com/wippyai/go-lua/analysis/domain/effect/ownership"
	"github.com/wippyai/go-lua/analysis/domain/type/normalize"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
	"github.com/wippyai/go-lua/analysis/module/signature"
)

// The authored typed application of every coroutine-library export, and the
// ownership transfer each one performs. Creating, resuming, wrapping and
// yielding all send a value across a control boundary, and the row names the
// argument position that is sent.
//
// What these exports do to CONTROL - the point at which it leaves and may
// return - is the suspension form's business and is not stated here. That
// format has not landed, and nothing in this package fabricates it.

var coroutineSignatures = map[string]signature.Function{
	"close": authored(typ.Func().
		Param("co", typ.Any).
		Returns(typ.Boolean, normalize.Optional(typ.Any)).
		Build()),
	"create": authored(typ.Func().
		Param("f", typ.Any).
		Returns(typ.Any).
		Build(),
		ownership.Send{FromParam: 0}),
	"isyieldable": authored(typ.Func().
		OptParam("co", typ.Any).
		Returns(typ.Boolean).
		Build()),
	"resume": authored(typ.Func().
		Param("co", typ.Any).
		Variadic(typ.Any).
		Returns(typ.Boolean, typ.Any).
		Build(),
		ownership.Send{FromParam: 1}),
	"running": authored(typ.Func().
		Returns(typ.Any, typ.Boolean).
		Build()),
	// coroutine.spawn starts a detached activation of its first argument and
	// hands the caller nothing back: the values the activation produces reach
	// the caller through no result slot, which is what detached means. The
	// function and every argument after it cross the activation boundary, so the
	// transfer names the position the crossing starts at.
	"spawn": authored(typ.Func().
		Param("f", typ.Any).
		Variadic(typ.Any).
		Build(),
		ownership.Send{FromParam: 0}),
	"status": authored(typ.Func().
		Param("co", typ.Any).
		Returns(typ.String).
		Build()),
	"wrap": authored(typ.Func().
		Param("f", typ.Any).
		Returns(typ.Any).
		Build(),
		ownership.Send{FromParam: 0}),
	"yield": authored(typ.Func().
		Variadic(typ.Any).
		Returns(typ.Any).
		Build(),
		ownership.Send{FromParam: 0}),
}
