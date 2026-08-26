package plan

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
)

// Initial is one immutable owner declaration for a zero-input invocation.
// It is logical schema data: the operation and decision scope are both
// owner-issued identities, and neither a runtime request nor a physical
// address participates in the value.
//
// Initial declarations are checked against the signature and scope registries
// by relation/check.  Mount then exposes only the checked projection to the
// runtime admission seam.
type Initial struct {
	operation signature.Identity
	scope     model.ScopeID
	digest    identity.ContentID
}

// DefineInitial declares one zero-input invocation at a logical scope. Full
// signature/input/output and ownership validation belongs to the independent
// checker; this constructor only freezes the declaration and its identity.
func DefineInitial(operation signature.Identity, scope model.ScopeID) Initial {
	value := Initial{operation: operation, scope: scope}
	value.digest, _ = identity.DeriveContentID(
		"relation/schema/plan/initial/v1",
		contentBytes(operation.Operation.Owner().Content()),
		contentBytes(operation.Operation.Content()),
		uint64Bytes(operation.Version),
		contentBytes(scope.Owner().Content()),
		contentBytes(scope.Content()),
	)
	return value
}

// Available reports whether the declaration carries complete logical
// identities and a stable content identity.
func (value Initial) Available() bool {
	return value.operation.Available() && value.scope.Available() && value.digest.Available()
}

// Operation returns the exact owner-issued semantic signature identity.
func (value Initial) Operation() signature.Identity { return value.operation }

// Scope returns the exact owner-issued decision scope identity.
func (value Initial) Scope() model.ScopeID { return value.scope }

// Digest returns the stable identity of the declaration row.
func (value Initial) Digest() identity.ContentID { return value.digest }
