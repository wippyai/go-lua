package relationoracle

import (
	"bytes"

	"github.com/wippyai/go-lua/analysis/identity"
)

// Scope is the canonical identity of a decision formula.  The oracle never
// decodes a formula: callers provide the content identity and, when needed,
// the small entailment/conjunction algebra used by a test.
type Scope struct {
	formula identity.ContentID
}

// NewScope adopts an already canonical formula identity.
func NewScope(formula identity.ContentID) (Scope, bool) {
	if !formula.Available() {
		return Scope{}, false
	}
	return Scope{formula: formula}, true
}

// Formula returns the opaque canonical formula identity.
func (scope Scope) Formula() identity.ContentID { return scope.formula }

// Available reports whether the scope carries a canonical formula identity.
func (scope Scope) Available() bool { return scope.formula.Available() }

// Equal compares canonical formula identities.
func (scope Scope) Equal(other Scope) bool {
	return scope.Available() && other.Available() && scope.formula == other.formula
}

// ScopeAlgebra is the only authority consulted for scope operations.  Its
// implementation is test-owned; the oracle has no masks, dimensions, or
// physical scope coordinates.
type ScopeAlgebra interface {
	// Entails reports whether available scope entails the requested formula.
	Entails(available Scope, requested Scope) bool
	// Conjoin returns the canonical identity for conjunction.  It must return
	// an unavailable scope only when the supplied formulas are unavailable.
	Conjoin(left Scope, right Scope) Scope
}

// ExactScope is the smallest useful scope algebra.  Selection retains rows
// whose formula is exactly the requested identity, and conjunction is a
// deterministic opaque identity derived from its two operands.
type ExactScope struct{}

func (ExactScope) Entails(available Scope, requested Scope) bool {
	return available.Equal(requested)
}

func (ExactScope) Conjoin(left Scope, right Scope) Scope {
	if !left.Available() || !right.Available() {
		return Scope{}
	}
	if left.Equal(right) {
		return left
	}
	first, second := left.formula, right.formula
	if bytes.Compare(first[:], second[:]) > 0 {
		first, second = second, first
	}
	formula, ok := identity.DeriveContentID("internal/relationoracle/scope-conjunction/v1", first[:], second[:])
	if !ok {
		return Scope{}
	}
	return Scope{formula: formula}
}

// ScopeEntailmentFunc is a test-only scope entailment function.
type ScopeEntailmentFunc func(available Scope, requested Scope) bool

// ScopeConjunctionFunc is a test-only scope conjunction function.
type ScopeConjunctionFunc func(left Scope, right Scope) Scope

// Entails makes ScopeEntailmentFunc implement ScopeAlgebra for tests that do
// not need conjunction.  It uses exact conjunction because the callback is
// intentionally only an entailment witness.
func (fn ScopeEntailmentFunc) Entails(available Scope, requested Scope) bool {
	return fn != nil && fn(available, requested)
}

func (fn ScopeEntailmentFunc) Conjoin(left Scope, right Scope) Scope {
	if fn == nil {
		return Scope{}
	}
	return ExactScope{}.Conjoin(left, right)
}

// Conjoin makes ScopeConjunctionFunc implement ScopeAlgebra for tests that
// use exact entailment and inject only conjunction.
func (fn ScopeConjunctionFunc) Entails(available Scope, requested Scope) bool {
	return ExactScope{}.Entails(available, requested)
}

func (fn ScopeConjunctionFunc) Conjoin(left Scope, right Scope) Scope {
	if fn == nil {
		return Scope{}
	}
	return fn(left, right)
}
