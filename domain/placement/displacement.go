package placement

// AuthenticateFactorCell validates one value read from Placement's solved
// allocation factor. Sparse transport is lawful only when the engine returns
// the owner-issued Stack default; Bottom is outside the factor and JIT-only or
// malformed values are never normalized into a semantic class.
func AuthenticateFactorCell(value Placement, present, available bool) (Placement, bool) {
	if !available || !validAnalysisPlacement(value) || value == Bottom || !present && value != Stack {
		return invalidPlacementResult, false
	}
	return value, true
}

// DisplaceChecked applies one escape requirement to an allocation's current
// placement. Placement displacement is deliberately monotone: an applying
// escape can only move an allocation's Stack baseline upward in the placement
// chain.
//
// The boolean is part of the contract, rather than a convenience status. A
// Bottom is outside the allocation factor, not evidence that the allocation is
// stack-local; an applying escape therefore refuses it. Likewise, runtime/JIT
// values and invalid enum values are outside the Placement analysis domain
// and are refused. In particular, malformed input must never be normalized to
// Unknown: Unknown is an authenticated semantic top, not an error sentinel.
// None and Borrow do not require a placement transition and preserve a valid
// current analysis value. Return is a valid analysis escape even though it has
// no standalone manifest spelling.
func DisplaceChecked(current Placement, escape Escape) (Placement, bool) {
	if !validAnalysisPlacement(current) || !validAnalysisEscape(escape) {
		return invalidPlacementResult, false
	}

	required, applies := escape.Placement()
	if !applies {
		return current, true
	}
	if current == Bottom {
		return invalidPlacementResult, false
	}
	return JoinChecked(current, required)
}

func validAnalysisPlacement(value Placement) bool {
	switch value {
	case Bottom, Stack, OwnedHeap, SharedHeap, Unknown:
		return true
	default:
		return false
	}
}

func validAnalysisEscape(value Escape) bool {
	switch value {
	case None, Borrow, Retain, Store, Send, Export, Opaque, Return:
		return true
	default:
		return false
	}
}
