package differential

import (
	"github.com/wippyai/go-lua/analysis/engine/relation/apply"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
	"github.com/wippyai/go-lua/analysis/relation/semantic/invocation"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
)

// Differential is one immutable signed transition between two semantic
// Apply observations.  Before and After are independent applications: each
// keeps its own proposal lease and lineage authority.  The sides are joined
// only by their exact runtime fence, operation identity, and invocation
// address.
//
// A zero apply.Application argument to New denotes an omitted side.  An
// omitted side is not interpreted as an empty result, ProvenAbsent value, or
// replacement application.  At least one side must be authenticated.
type Differential struct {
	before     apply.Application
	after      apply.Application
	beforeOK   bool
	afterOK    bool
	fence      binding.Fence
	operation  signature.Identity
	invocation invocation.InvocationAddress
	sealed     bool
}

// New validates and seals one signed Apply differential.  The supplied
// applications are retained by value, not reconstructed from their proposal
// batches; this is important because ProposalBatch is a lease whose validity
// is owned by the original Apply buffer.
//
// Either argument may be the zero apply.Application value, but both may not
// be omitted.  When both sides are present, they must name the exact same
// solve-local runtime fence, operation identity, and structural invocation
// address.  Proposal destinations are deliberately not compared: a move is
// a later delete-plus-insert classification, not a transport rejection.
func New(before, after apply.Application) (Differential, bool) {
	beforeOK := before.Available()
	afterOK := after.Available()
	if !beforeOK && !afterOK {
		return Differential{}, false
	}

	anchor := before
	if !beforeOK {
		anchor = after
	}
	fence := anchor.Fence()
	operation := anchor.Operation()
	address := anchor.Invocation()
	if !fence.Available() || !operation.Available() || !address.Available() {
		return Differential{}, false
	}
	if beforeOK && !sameEnvelope(before, fence, operation, address) {
		return Differential{}, false
	}
	if afterOK && !sameEnvelope(after, fence, operation, address) {
		return Differential{}, false
	}

	result := Differential{
		before:     before,
		after:      after,
		beforeOK:   beforeOK,
		afterOK:    afterOK,
		fence:      fence,
		operation:  operation,
		invocation: address,
		sealed:     true,
	}
	if !result.Available() {
		return Differential{}, false
	}
	return result, true
}

func sameEnvelope(application apply.Application, fence binding.Fence, operation signature.Identity, address invocation.InvocationAddress) bool {
	return application.Available() && application.Fence().Same(fence) && application.Operation() == operation && application.Invocation().Same(address)
}

// Available reports whether this signed transport still has at least one
// live authenticated side and both sides, when present, still satisfy the
// captured common envelope.  In particular, invalidation of either retained
// proposal lease invalidates the Differential; no stale copy is exposed.
func (value Differential) Available() bool {
	if !value.sealed || (!value.beforeOK && !value.afterOK) || !value.fence.Available() || !value.operation.Available() || !value.invocation.Available() {
		return false
	}
	if value.beforeOK && !sameEnvelope(value.before, value.fence, value.operation, value.invocation) {
		return false
	}
	if value.afterOK && !sameEnvelope(value.after, value.fence, value.operation, value.invocation) {
		return false
	}
	return true
}

// Before returns the exact retained predecessor Application.  A false
// result means the side was omitted, or that the signed transport has become
// unavailable (for example because its proposal lease was reset).
func (value Differential) Before() (apply.Application, bool) {
	if !value.Available() || !value.beforeOK {
		return apply.Application{}, false
	}
	return value.before, true
}

// After returns the exact retained successor Application.  A false result
// means the side was omitted, or that the signed transport has become
// unavailable (for example because its proposal lease was reset).
func (value Differential) After() (apply.Application, bool) {
	if !value.Available() || !value.afterOK {
		return apply.Application{}, false
	}
	return value.after, true
}

// Fence returns the exact solve-local runtime common to the retained sides.
func (value Differential) Fence() binding.Fence {
	if !value.Available() {
		return binding.Fence{}
	}
	return value.fence
}

// Operation returns the exact operation identity common to the retained
// sides.
func (value Differential) Operation() signature.Identity {
	if !value.Available() {
		return signature.Identity{}
	}
	return value.operation
}

// Invocation returns the exact structural invocation address common to the
// retained sides.  It is provenance, not an evaluation ordinal or generated
// destination key.
func (value Differential) Invocation() invocation.InvocationAddress {
	if !value.Available() {
		return invocation.InvocationAddress{}
	}
	return value.invocation
}
