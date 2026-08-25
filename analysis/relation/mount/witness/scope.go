package witness

import "github.com/wippyai/go-lua/analysis/relation/semantic/binding"

// Scope is the only mounted representation of an authenticated formula.
// It carries only the opaque runtime-fenced token. The owning Mounted keeps
// the token-to-Region association in its private concurrency-safe arena.
type Scope struct {
	token binding.ScopeToken
}

func newScope(token binding.ScopeToken) (Scope, bool) {
	if !token.Available() {
		return Scope{}, false
	}
	return Scope{token: token}, true
}

// Available reports whether the scope carries one authenticated token.
func (scope Scope) Available() bool { return scope.token.Available() }

// ValidFor reports whether this immutable scope belongs to the exact runtime
// fence. It does not expose the issuer or allow reissuance.
func (scope Scope) ValidFor(fence binding.Fence) bool {
	return scope.Available() && scope.token.ValidFor(fence)
}

func (scope Scope) validFor(fence binding.Fence) bool { return scope.ValidFor(fence) }

// Same compares the exact authenticated formula and runtime fence.
func (scope Scope) Same(other Scope) bool {
	return scope.Available() && other.Available() && scope.token.Same(other.token)
}
