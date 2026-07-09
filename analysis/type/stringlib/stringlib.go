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
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typeexpr"
)

var gsubReplacement = typeexpr.Union(
	typ.String,
	typetable.NewMap(typ.Any, typ.Any),
	typ.Func().
		Param("capture", captureValueType).
		Returns(typeexpr.Union(typ.String, typ.Number, typ.False, typ.Nil)).
		Build(),
)

var captureValueType = typeexpr.Union(typ.String, typ.Integer)
var optionalCaptureValueType = normalize.Optional(captureValueType)

const generalCaptureReturnSlots = 4

var methods = map[string]*typ.Function{
	"byte": typ.Func().
		Param("s", typ.String).
		OptParam("i", typ.Integer).
		OptParam("j", typ.Integer).
		Returns(normalize.Optional(typ.Integer)).
		Build(),
	"char": typ.Func().
		Variadic(typ.Integer).
		Returns(typ.String).
		Build(),
	"dump": typ.Func().
		Param("function", typ.Any).
		OptParam("strip", typ.Boolean).
		Returns(typ.Never).
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
	"gfind": typ.Func().
		Param("s", typ.String).
		Param("pattern", typ.String).
		Returns(gmatchIterator(nil, true), typ.Any).
		Build(),
	"gmatch": typ.Func().
		Param("s", typ.String).
		Param("pattern", typ.String).
		Returns(gmatchIterator(nil, true), typ.Any).
		Build(),
	"gsub": typ.Func().
		Param("s", typ.String).
		Param("pattern", typ.String).
		Param("repl", gsubReplacement).
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
		Returns(optionalCaptureValueType).
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

func gmatchIterator(captures []typ.Type, general bool) *typ.Function {
	returns := captureReturnTypes(captures, general)
	return typ.Func().Returns(returns...).Build()
}

func captureReturnTypes(captures []typ.Type, general bool) []typ.Type {
	if len(captures) == 0 {
		if !general {
			return []typ.Type{normalize.Optional(typ.String)}
		}
		captures = make([]typ.Type, generalCaptureReturnSlots)
		for i := range captures {
			captures[i] = captureValueType
		}
	}
	out := make([]typ.Type, len(captures))
	for i, capture := range captures {
		if capture == nil {
			capture = captureValueType
		}
		out[i] = normalize.Optional(capture)
	}
	return out
}

// CaptureTypes returns the capture result types implied by a Lua pattern
// literal. Empty captures "()" are position captures and produce integers; all
// other captures produce strings. It deliberately does not validate the pattern.
func CaptureTypes(pattern string) []typ.Type {
	var captures []typ.Type
	for i := 0; i < len(pattern); i++ {
		switch pattern[i] {
		case '%':
			if i+1 < len(pattern) {
				i++
			}
		case '[':
			i = skipPatternClass(pattern, i+1)
		case '(':
			if i+1 < len(pattern) && pattern[i+1] == ')' {
				captures = append(captures, typ.Integer)
				i++
			} else {
				captures = append(captures, typ.String)
			}
		}
	}
	return captures
}

func skipPatternClass(pattern string, i int) int {
	for i < len(pattern) {
		if pattern[i] == '%' && i+1 < len(pattern) {
			i += 2
			continue
		}
		if pattern[i] == ']' {
			return i
		}
		i++
	}
	return len(pattern) - 1
}

// GMatchIterator returns the iterator function type for a gmatch/gfind call.
// With no literal capture information it returns a conservative capture tuple.
func GMatchIterator(captures []typ.Type) *typ.Function {
	return gmatchIterator(captures, false)
}

// GeneralGMatchIterator returns a conservative iterator function type when the
// pattern is not statically known.
func GeneralGMatchIterator() *typ.Function {
	return gmatchIterator(nil, true)
}

// OptionalCaptureValue returns the nilable type for a single string-library
// capture result.
func OptionalCaptureValue(t typ.Type) typ.Type {
	if t == nil {
		t = captureValueType
	}
	return normalize.Optional(t)
}

// OptionalGeneralCaptureValue returns a nilable string-or-integer capture.
func OptionalGeneralCaptureValue() typ.Type {
	return optionalCaptureValueType
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
