package placement

// EscapeTransition is a placement-policy transition that moves a fresh graph
// beyond stack lifetime. Call-boundary events are translated into this neutral
// vocabulary by fact application.
type EscapeTransition uint8

const (
	EscapeTransitionNone EscapeTransition = iota
	EscapeTransitionReturn
	EscapeTransitionRetain
	EscapeTransitionStore
	EscapeTransitionSend
	EscapeTransitionExport
	EscapeTransitionOpaque
)

type escapePlacementPolicy struct {
	placement Value
	applies   bool
}

var escapeTransitionPolicy = [...]escapePlacementPolicy{
	EscapeTransitionReturn: {placement: OwnedHeap, applies: true},
	EscapeTransitionRetain: {placement: OwnedHeap, applies: true},
	EscapeTransitionStore:  {placement: OwnedHeap, applies: true},
	EscapeTransitionSend:   {placement: SharedHeap, applies: true},
	EscapeTransitionExport: {placement: SharedHeap, applies: true},
	EscapeTransitionOpaque: {placement: SharedHeap, applies: true},
}

// Placement returns the placement required by transition.
func (t EscapeTransition) Placement() (Value, bool) {
	if int(t) >= len(escapeTransitionPolicy) {
		return Bottom, false
	}
	policy := escapeTransitionPolicy[t]
	return policy.placement, policy.applies
}
