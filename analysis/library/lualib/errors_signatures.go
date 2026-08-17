package lualib

import (
	"github.com/wippyai/go-lua/analysis/domain/effect/ownership"
	"github.com/wippyai/go-lua/analysis/domain/type/normalize"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
	"github.com/wippyai/go-lua/analysis/module/signature"
)

// The authored typed application of the errors library: the three callables the
// library root exports, and the six the error family publishes - two metamethods
// of the family itself and four methods an error object reaches through it.
//
// A method takes the value it is called on at its first position. Nothing here
// names a receiver TYPE beyond that: the family is the metatable of every error
// object, and an error object is produced by errors.new rather than written as a
// literal, so the contract states the application and leaves the identity of the
// value to whatever produced it.

var errorsSignatures = map[string]signature.Function{
	"is": authored(typ.Func().
		Param("err", typ.Any).
		Param("kind", typ.String).
		Returns(typ.Boolean).
		Build(),
		ownership.BorrowAll{}),
	"new": authored(typ.Func().
		Param("message", typ.Any).
		Returns(typ.Any).
		Build()),
	// errors.wrap carries an existing error inside a new one, so the wrapped
	// value is stored into the value the call produces rather than borrowed for
	// the duration of the call.
	"wrap": authored(typ.Func().
		Param("err", typ.Any).
		Param("message", typ.Any).
		Returns(typ.Any).
		Build()),
}

// errorsMethods are the callables the errors library publishes below its root, in
// authored order: the family's own metamethods, then the methods an error reaches
// through the family's index.
var errorsMethods = []methodExport{
	{
		Path: exportPath(ErrorFamilyKey, "__concat"),
		Signature: authored(typ.Func().
			Param("self", typ.Any).
			Param("other", typ.Any).
			Returns(typ.String).
			Build(),
			ownership.BorrowAll{}),
	},
	{
		Path: exportPath(ErrorFamilyKey, "__tostring"),
		Signature: authored(typ.Func().
			Param("self", typ.Any).
			Returns(typ.String).
			Build(),
			ownership.BorrowAll{}),
	},
	// details answers with the table an error was given and with nothing when it
	// was given none, so the slot is optional over the table top rather than over
	// any value: an error's details are a table or they are absent.
	{
		Path: exportPath(ErrorFamilyKey, ErrorMethodKey, "details"),
		Signature: authored(typ.Func().
			Param("self", typ.Any).
			Returns(normalize.Optional(typ.BuiltinTableTopMarker())).
			Build()),
	},
	{
		Path: exportPath(ErrorFamilyKey, ErrorMethodKey, "kind"),
		Signature: authored(typ.Func().
			Param("self", typ.Any).
			Returns(typ.String).
			Build(),
			ownership.BorrowAll{}),
	},
	{
		Path: exportPath(ErrorFamilyKey, ErrorMethodKey, "message"),
		Signature: authored(typ.Func().
			Param("self", typ.Any).
			Returns(typ.String).
			Build(),
			ownership.BorrowAll{}),
	},
	// retryable answers with a boolean when the error carries the judgment and
	// with nothing when it does not, which is a third answer rather than a false
	// one: an error that says nothing about retrying is not an error that says
	// no.
	{
		Path: exportPath(ErrorFamilyKey, ErrorMethodKey, "retryable"),
		Signature: authored(typ.Func().
			Param("self", typ.Any).
			Returns(normalize.Optional(typ.Boolean)).
			Build(),
			ownership.BorrowAll{}),
	},
}
