package invocation

import (
	"github.com/wippyai/go-lua/analysis/relation/schema/semantic/output"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
)

// ContributionTransition is the exact transport value for one schema
// contribution output. Its key is retained as the already-authenticated
// InvocationAddress plus the sealed output port and owner-issued destination
// CellToken. The state owner may intern Address into its fence-local Handle;
// this transport value never invents a digest, ordinal, or alternate key.
//
// Before and After are independent supplied sides. An omitted side is
// represented by binding.NoContributionSide and makes no claim about state;
// it is not replaced by ProvenAbsent. Both sides may be supplied for an
// atomic replacement.
type ContributionTransition struct {
	address     InvocationAddress
	spec        output.ContributionSpec
	destination binding.CellToken
	before      binding.ContributionSide
	after       binding.ContributionSide
	fence       binding.Fence
	sealed      bool
}

// NewContributionTransition validates and seals one exact contribution
// transition. The schema declaration is the sole authority for output type
// and port identity; runtime inputs only provide already-issued provenance
// and payloads.
func NewContributionTransition(
	spec output.ContributionSpec,
	address InvocationAddress,
	destination binding.CellToken,
	fence binding.Fence,
	before binding.ContributionSide,
	after binding.ContributionSide,
) (ContributionTransition, bool) {
	if !spec.Available() || spec.Reducer() != output.Contributions || !address.Available() || !destination.Available() || !fence.Available() || !address.ValidFor(fence) || !destination.ValidFor(fence) || !before.Available() || !after.Available() || (!before.Present() && !after.Present()) || destination.Column() != spec.Column() {
		return ContributionTransition{}, false
	}
	if !before.ValidFor(fence) || !after.ValidFor(fence) {
		return ContributionTransition{}, false
	}
	for _, side := range []binding.ContributionSide{before, after} {
		if !side.Present() {
			continue
		}
		value := side.Value()
		if !value.Available() || value.Type() != spec.ValueType() || !value.ValidFor(fence) {
			return ContributionTransition{}, false
		}
	}
	value := ContributionTransition{address: address, spec: spec, destination: destination, fence: fence, before: before, after: after, sealed: true}
	return value, value.Available()
}

// Available authenticates the complete immutable transition envelope.
func (transition ContributionTransition) Available() bool {
	return transition.sealed && transition.address.Available() && transition.spec.Available() && transition.destination.Available() && transition.fence.Available() && transition.address.ValidFor(transition.fence) && transition.destination.ValidFor(transition.fence) && transition.destination.Column() == transition.spec.Column() && transition.before.Available() && transition.after.Available() && (transition.before.Present() || transition.after.Present()) && transition.before.ValidFor(transition.fence) && transition.after.ValidFor(transition.fence)
}

// ValidFor redeems the transition against one exact mounted runtime fence.
func (transition ContributionTransition) ValidFor(fence binding.Fence) bool {
	return transition.Available() && transition.fence.Same(fence)
}

// Address returns the exact invocation provenance retained by this value.
// The state contribution directory is the only owner allowed to intern it.
func (transition ContributionTransition) Address() InvocationAddress {
	if !transition.Available() {
		return InvocationAddress{}
	}
	return transition.address
}

// Spec returns the exact schema declaration that admitted this transition.
func (transition ContributionTransition) Spec() output.ContributionSpec {
	if !transition.Available() {
		return output.ContributionSpec{}
	}
	return transition.spec
}

// Port returns the schema-owned output port.
func (transition ContributionTransition) Port() output.OutputPort {
	if !transition.Available() {
		return output.OutputPort{}
	}
	return transition.spec.Port()
}

// Destination returns the exact owner-issued destination cell. Its scope and
// denominator witness are part of the authenticated output address and must
// be retained through reduction; Row is only a projection for state indexes.
func (transition ContributionTransition) Destination() binding.CellToken {
	if !transition.Available() {
		return binding.CellToken{}
	}
	return transition.destination
}

// Before returns the supplied predecessor side. A false result means the
// event omitted Before, not that state is ProvenAbsent.
func (transition ContributionTransition) Before() (binding.ContributionSide, bool) {
	if !transition.Available() || !transition.before.Present() {
		return binding.ContributionSide{}, false
	}
	return transition.before, true
}

// After returns the supplied successor side. A false result means the event
// omitted After, not that state is ProvenAbsent.
func (transition ContributionTransition) After() (binding.ContributionSide, bool) {
	if !transition.Available() || !transition.after.Present() {
		return binding.ContributionSide{}, false
	}
	return transition.after, true
}

// Replacement reports the atomic two-sided form.
func (transition ContributionTransition) Replacement() bool {
	return transition.Available() && transition.before.Present() && transition.after.Present()
}

// Same compares exact signed transport content. It is not a generated key.
func (transition ContributionTransition) Same(other ContributionTransition) bool {
	if !transition.Available() || !other.Available() || transition.fence != other.fence || !transition.address.Same(other.address) || !transition.spec.Equal(other.spec) || !transition.destination.Same(other.destination) {
		return false
	}
	return transition.before.Same(other.before) && transition.after.Same(other.after)
}
