package witness

import (
	"bytes"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
)

// Scope is the only mounted representation of an authenticated formula.
// Its token and neutral Region are deliberately private: callers can carry,
// compare, and ask the owning Mounted to combine scopes, but cannot mint a
// token for an arbitrary formula or replace the formula behind one.
type Scope struct {
	token  binding.ScopeToken
	region Region
}

func newScope(token binding.ScopeToken, region Region) (Scope, bool) {
	if !token.Available() || !regionAvailable(region) {
		return Scope{}, false
	}
	return Scope{token: token, region: region}, true
}

// Available reports whether the scope carries one authenticated token and a
// complete neutral formula.
func (scope Scope) Available() bool {
	return scope.token.Available() && regionAvailable(scope.region)
}

// ValidFor reports whether this immutable scope belongs to the exact runtime
// fence. It does not expose the issuer or allow reissuance.
func (scope Scope) ValidFor(fence binding.Fence) bool {
	return scope.Available() && scope.token.ValidFor(fence)
}

func (scope Scope) validFor(fence binding.Fence) bool { return scope.ValidFor(fence) }

// Same compares the exact authenticated formula and runtime fence.
func (scope Scope) Same(other Scope) bool {
	if !scope.Available() || !other.Available() || !scope.token.Same(other.token) {
		return false
	}
	left, leftOK := scope.region.Identity()
	right, rightOK := other.region.Identity()
	return leftOK && rightOK && left == right
}

// identity returns the canonical formula identity used to issue token.
func (scope Scope) identity() (identity.ContentID, bool) {
	if !scope.Available() {
		return identity.ContentID{}, false
	}
	return scope.region.Identity()
}

func (scope Scope) less(other Scope) bool {
	left, leftOK := scope.identity()
	right, rightOK := other.identity()
	if !leftOK || !rightOK {
		return false
	}
	return bytes.Compare(left[:], right[:]) < 0
}
