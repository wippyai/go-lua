// Package derive turns a function's type shape into effect labels through a
// registry of small, independent rules.
//
// Effect labels carry the contracts the checker reasons about (the value/error
// presence correlation, ownership, refinements, ...). For a function with a
// body the summarizer proves these from data flow. A function known only by its
// type - a built-in module signature - has no body to summarize, yet its type
// still encodes conventions (a trailing optional error means the other results
// are present exactly when the error is absent). A Rule recovers those labels
// from the type alone.
//
// Rules are ordered and composable: extending the system means appending a Rule,
// not editing a switch. Each rule is gated on Context, so nothing is recognized
// by name - only by the structural facts the caller supplies.
package derive

import (
	"github.com/wippyai/go-lua/analysis/domain/effect"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
)

// Context carries the structural facts rules are allowed to key on. It is the
// single point of designation: a rule never hardcodes a type or name, it asks
// Context.
type Context struct {
	// ErrorType is the canonical error type of the analyzed program (for Wippy,
	// the LuaError interface). Nil disables every rule that depends on it.
	ErrorType typ.Type
}

// Rule derives additional effect labels for fn from its type shape, given what
// is already known about its effect and the derivation context. A rule is pure
// and returns nil when it does not apply or when known already covers it.
type Rule func(fn *typ.Function, known effect.Row, ctx Context) []effect.Label

// ApplyDefault applies the package-owned standard derivation rule set.
//
// The standard set is deliberately not exposed as a mutable slice: rule
// selection is semantic policy owned by this package, while callers that need
// additional rules retain explicit ownership through Apply's variadic rules.
func ApplyDefault(fn *typ.Function, known effect.Row, ctx Context) effect.Row {
	return Apply(fn, known, ctx, ErrorReturnFromShape)
}

// Apply runs rules in order and returns known augmented with every derived
// label. Duplicate labels are dropped by Row.With, so a rule that restates an
// existing label is harmless.
func Apply(fn *typ.Function, known effect.Row, ctx Context, rules ...Rule) effect.Row {
	if fn == nil {
		return known
	}
	out := known
	for _, rule := range rules {
		if rule == nil {
			continue
		}
		out = out.With(rule(fn, out, ctx)...)
	}
	return out
}
