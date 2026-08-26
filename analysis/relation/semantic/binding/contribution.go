package binding

import "github.com/wippyai/go-lua/analysis/relation/schema/model"

// ContributionSide is one transport side of a contribution transition. A
// zero side means that this event did not supply that side; it makes no claim
// about whether a contribution exists in state and is never an authenticated
// absence. A present side retains the exact owner-issued value, presence, and
// proof lineage together.
//
// Invocation provenance and output identity are deliberately not stored here:
// those belong to semantic/invocation and the schema output declaration. This
// package owns only the fenced payload that can cross the engine boundary.
type ContributionSide struct {
	value    ValueToken
	presence model.Presence
	lineage  model.LineageRef
	present  bool
}

// NewContributionSide adopts one complete authenticated output payload. The
// caller supplies the exact lineage; this constructor never joins, derives,
// or substitutes proof state.
func NewContributionSide(value ValueToken, presence model.Presence, lineage model.LineageRef) (ContributionSide, bool) {
	if !presence.Available() || presence.Is(model.Refused) || !lineage.Available() || !valueMatches(value, presence, value.Fence()) {
		return ContributionSide{}, false
	}
	// Non-value statuses must not carry a stale or fabricated value token.
	if (presence.Is(model.Present) || presence.Is(model.AuthenticatedOpaque)) && !value.Available() {
		return ContributionSide{}, false
	}
	if !presence.Is(model.Present) && !presence.Is(model.AuthenticatedOpaque) && value.Available() {
		return ContributionSide{}, false
	}
	return ContributionSide{value: value, presence: presence, lineage: lineage, present: true}, true
}

// NoContributionSide returns the omitted-side marker used when an event
// supplies only Before or only After. It is not an authenticated sparse row,
// ProvenAbsent, or a state deletion and must never be written as one.
func NoContributionSide() ContributionSide { return ContributionSide{} }

// Available reports whether this side is either omitted from the event or a
// complete retained payload. An omitted side contributes no state assertion;
// the enclosing transition requires at least one supplied side.
func (side ContributionSide) Available() bool {
	if !side.present {
		return !side.value.Available() && !side.presence.Available() && !side.lineage.Available()
	}
	return side.presence.Available() && !side.presence.Is(model.Refused) && side.lineage.Available() && valueMatches(side.value, side.presence, side.value.Fence())
}

// ValidFor redeems a present value against the exact runtime fence. Omitted
// sides carry no runtime token and therefore impose no payload fence check.
func (side ContributionSide) ValidFor(fence Fence) bool {
	if !side.Available() || !fence.Available() {
		return false
	}
	if !side.present {
		return true
	}
	return valueMatches(side.value, side.presence, fence)
}

// Present distinguishes a retained payload from an omitted event side.
func (side ContributionSide) Present() bool { return side.present && side.Available() }

// Value returns the exact owner-issued payload, if this side is present.
func (side ContributionSide) Value() ValueToken {
	if !side.Present() {
		return ValueToken{}
	}
	return side.value
}

// Presence returns the exact declared status, if this side is present.
func (side ContributionSide) Presence() model.Presence {
	if !side.Present() {
		return model.Presence{}
	}
	return side.presence
}

// Lineage returns the exact proof sidecar, if this side is present.
func (side ContributionSide) Lineage() model.LineageRef {
	if !side.Present() {
		return model.LineageRef{}
	}
	return side.lineage
}

// Same compares the complete retained side without deriving an identity.
func (side ContributionSide) Same(other ContributionSide) bool {
	if !side.Available() || !other.Available() {
		return false
	}
	if !side.present || !other.present {
		return !side.present && !other.present
	}
	return side.presence == other.presence && side.lineage == other.lineage && side.value.Same(other.value)
}
