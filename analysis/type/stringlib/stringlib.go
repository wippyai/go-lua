// Package stringlib holds the canonical standard-library string function
// signatures. Each is the global form (string.upper, string.sub, ...) whose
// first parameter is the string operand, so it serves two callers from one
// source: the stdlib signature registry resolves the global call string.m(s, ...),
// and member-call resolution resolves the colon method s:m(...) by binding the
// receiver as that first operand. Keeping a single table avoids a second,
// divergent model of the string library.
package stringlib

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/type/normalize"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

var methods = map[string]*typ.Function{
	"byte": typ.Func().
		Param("s", typ.String).
		OptParam("i", typ.Integer).
		OptParam("j", typ.Integer).
		Returns(typ.Integer).
		Build(),
	"char": typ.Func().
		Variadic(typ.Integer).
		Returns(typ.String).
		Build(),
	"dump": typ.Func().
		Param("function", typ.Any).
		OptParam("strip", typ.Boolean).
		Returns(typ.String).
		Build(),
	"find": typ.Func().
		Param("s", typ.String).
		Param("pattern", typ.String).
		OptParam("init", typ.Integer).
		OptParam("plain", typ.Boolean).
		Returns(normalize.Optional(typ.Integer), normalize.Optional(typ.Integer)).
		Build(),
	"format": typ.Func().
		Param("formatstring", typ.String).
		Variadic(typ.Any).
		Returns(typ.String).
		Build(),
	"gmatch": typ.Func().
		Param("s", typ.String).
		Param("pattern", typ.String).
		Returns(typ.Any).
		Build(),
	"gsub": typ.Func().
		Param("s", typ.String).
		Param("pattern", typ.String).
		Param("repl", typ.Any).
		OptParam("n", typ.Integer).
		Returns(typ.String, typ.Integer).
		Build(),
	"len": typ.Func().
		Param("s", typ.String).
		Returns(typ.Integer).
		Build(),
	"lower": typ.Func().
		Param("s", typ.String).
		Returns(typ.String).
		Build(),
	"match": typ.Func().
		Param("s", typ.String).
		Param("pattern", typ.String).
		OptParam("init", typ.Integer).
		Returns(normalize.Optional(typ.String)).
		Build(),
	"pack": typ.Func().
		Param("fmt", typ.String).
		Variadic(typ.Any).
		Returns(typ.String).
		Build(),
	"packsize": typ.Func().
		Param("fmt", typ.String).
		Returns(typ.Integer).
		Build(),
	"rep": typ.Func().
		Param("s", typ.String).
		Param("n", typ.Integer).
		OptParam("sep", typ.String).
		Returns(typ.String).
		Build(),
	"reverse": typ.Func().
		Param("s", typ.String).
		Returns(typ.String).
		Build(),
	"sub": typ.Func().
		Param("s", typ.String).
		Param("i", typ.Integer).
		OptParam("j", typ.Integer).
		Returns(typ.String).
		Build(),
	"unpack": typ.Func().
		Param("fmt", typ.String).
		Param("s", typ.String).
		OptParam("pos", typ.Integer).
		Returns(typ.Any).
		Build(),
	"upper": typ.Func().
		Param("s", typ.String).
		Returns(typ.String).
		Build(),
}

// Method returns the signature of the named string-library function, with its
// first parameter being the string operand.
func Method(name string) (*typ.Function, bool) {
	fn, ok := methods[name]
	return fn, ok
}

// Names returns the string-library function names in a stable order.
func Names() []string {
	names := make([]string, 0, len(methods))
	for name := range methods {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
