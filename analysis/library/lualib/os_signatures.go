package lualib

import (
	"github.com/wippyai/go-lua/analysis/domain/effect"
	"github.com/wippyai/go-lua/analysis/domain/type/normalize"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
	"github.com/wippyai/go-lua/analysis/module/signature"
)

// The authored typed application of every os-library export.
//
// os.clock is the case that shows why an effect row is carried rather than
// inferred: its result type is exact and its row is deliberately OPEN, because
// the absence of a label is not a proof that a host call is a closed operation.
// The other exports carry a closed empty row, which is the weaker and honest
// statement the model makes about them today.

// osClockEffectVariable names the open tail of os.clock's effect row. It is the
// row variable's identity, and it is written verbatim rather than rebuilt from
// the export's address: an effect variable is a name in the effect algebra, and
// renaming one renames the row.
const osClockEffectVariable = "stdlib.os.clock"

var osSignatures = map[string]signature.Function{
	"clock": {
		Type:   typ.Func().Returns(typ.Number).Build(),
		Effect: effect.Open(osClockEffectVariable),
	},
	"date": authored(typ.Func().
		OptParam("format", typ.String).
		OptParam("time", typ.Integer).
		Returns(typ.Any).
		Build()),
	"difftime": authored(typ.Func().
		Param("t2", typ.Number).
		Param("t1", typ.Number).
		Returns(typ.Number).
		Build()),
	"execute": authored(typ.Func().
		OptParam("command", typ.String).
		Returns(typ.Any).
		Build()),
	"exit": authored(typ.Func().
		OptParam("code", typ.Any).
		OptParam("close", typ.Boolean).
		Returns(typ.Never).
		Build()),
	"getenv": authored(typ.Func().
		Param("varname", typ.String).
		Returns(normalize.Optional(typ.String)).
		Build()),
	"remove": authored(typ.Func().
		Param("filename", typ.String).
		Returns(normalize.Optional(typ.Boolean), normalize.Optional(typ.String)).
		Build()),
	"rename": authored(typ.Func().
		Param("oldname", typ.String).
		Param("newname", typ.String).
		Returns(normalize.Optional(typ.Boolean), normalize.Optional(typ.String)).
		Build()),
	"time": authored(typ.Func().
		OptParam("t", typ.Any).
		Returns(typ.Integer).
		Build()),
	"tmpname": authored(typ.Func().
		Returns(typ.String).
		Build()),
}
