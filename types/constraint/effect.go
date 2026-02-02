package constraint

import (
	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/typ"
)

// EffectLookupBySym retrieves a function's inferred effect by symbol ID.
//
// Used during call site analysis to determine what type refinements a
// function call produces. Returns nil if the function has no recorded effect.
type EffectLookupBySym func(sym cfg.SymbolID) *FunctionEffect

// FunctionEffect describes the type refinements a function produces.
//
// Effects encode how a function call narrows types based on its return value.
// Three categories are supported:
//   - OnReturn: constraints that hold when the function returns normally
//     (used for assert-style functions that error() on failure)
//   - OnTrue: constraints that hold when the function returns truthy
//     (used for type predicate functions like isString(x))
//   - OnFalse: constraints that hold when the function returns falsy
//
// Placeholder roots ($0, $1, ...) reference parameters by position.
// At call sites, placeholders are substituted with actual argument paths.
type FunctionEffect struct {
	// Row is the effect label set (IO, Mutate, Throw, etc.).
	// Stored as typ.EffectInfo to avoid circular import with effect package.
	// The concrete type is effect.Row.
	Row typ.EffectInfo

	// OnReturn: constraints that hold when function returns normally.
	// Used for assert-style functions that error() on failure.
	OnReturn Condition

	// OnTrue: constraints that hold when function returns truthy.
	// Used for type predicate functions like isString(x).
	OnTrue Condition

	// OnFalse: constraints that hold when function returns falsy.
	OnFalse Condition

	// Terminates indicates the function never returns normally.
	// Used for functions that always call error() or similar.
	Terminates bool
}

// NewEffect creates a FunctionEffect from constraint slices.
func NewEffect(onReturn, onTrue, onFalse []Constraint) *FunctionEffect {
	return &FunctionEffect{
		OnReturn: FromConstraints(onReturn...),
		OnTrue:   FromConstraints(onTrue...),
		OnFalse:  FromConstraints(onFalse...),
	}
}

// IsEmpty returns true if the effect has no constraints, no row, and doesn't terminate.
func (e *FunctionEffect) IsEmpty() bool {
	if e == nil {
		return true
	}

	return e.Row == nil && !e.OnReturn.HasConstraints() && !e.OnTrue.HasConstraints() && !e.OnFalse.HasConstraints() && !e.Terminates
}

// HasAssertSemantics returns true if function has assert-style semantics.
func (e *FunctionEffect) HasAssertSemantics() bool {
	return e != nil && e.OnReturn.HasConstraints()
}

// HasPredicateSemantics returns true if function has predicate semantics.
func (e *FunctionEffect) HasPredicateSemantics() bool {
	return e != nil && (e.OnTrue.HasConstraints() || e.OnFalse.HasConstraints())
}

// Equals returns true if two function effects are structurally equal.
// Implements internal.Equaler interface for use in typ.Function.
func (e *FunctionEffect) Equals(other any) bool {
	if other == nil {
		return e == nil
	}

	o, ok := other.(*FunctionEffect)

	if !ok {
		return false
	}

	if e == nil && o == nil {
		return true
	}

	if e == nil || o == nil {
		return false
	}

	if !effectRowEquals(e.Row, o.Row) {
		return false
	}

	return e.OnReturn.Equals(o.OnReturn) &&
		e.OnTrue.Equals(o.OnTrue) &&
		e.OnFalse.Equals(o.OnFalse) &&
		e.Terminates == o.Terminates
}

// effectRowEquals compares two effect rows stored as typ.EffectInfo.
func effectRowEquals(a, b typ.EffectInfo) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return a.Equals(b)
}

// IsRefinementInfo implements typ.RefinementInfo.
func (e *FunctionEffect) IsRefinementInfo() {}

// Substitute returns a new FunctionEffect with placeholder paths replaced.
//
// At a call site, parameter placeholders ($0, $1, ...) are replaced with
// the actual argument paths, producing concrete constraints that can be
// applied to narrow types at the call location.
func (e *FunctionEffect) Substitute(args []Path) *FunctionEffect {
	if e == nil || e.IsEmpty() {
		return nil
	}

	result := &FunctionEffect{
		OnReturn: e.OnReturn.Substitute(args),
		OnTrue:   e.OnTrue.Substitute(args),
		OnFalse:  e.OnFalse.Substitute(args),
	}
	if result.IsEmpty() {
		return nil
	}

	return result
}

// KeysCollectorParamIndex checks if the function returns keys of a parameter.
//
// Detects patterns like `for k in pairs(t)` where the function returns keys
// from a table parameter. Returns the parameter index (0-based) if found,
// or -1 if the function doesn't have this behavior.
//
// Detection looks for KeyOf constraints in OnReturn where the table is a
// parameter placeholder ($N) and the key is a return path.
func (e *FunctionEffect) KeysCollectorParamIndex() int {
	if e == nil || !e.OnReturn.HasConstraints() {
		return -1
	}

	for _, disj := range e.OnReturn.Disjuncts {
		for _, c := range disj {
			if keyOf, ok := c.(KeyOf); ok {
				if keyOf.Table.IsPlaceholder() && IsReturnPath(keyOf.Key) {
					return keyOf.Table.PlaceholderIndex()
				}
			}
		}
	}
	return -1
}
