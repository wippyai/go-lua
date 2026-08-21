package placement

// Displace applies one escape requirement to an allocation's current
// placement. Placement displacement is deliberately monotone: an applying
// escape can only move a seeded analysis placement upward in the placement
// chain.
//
// Bottom is the absence of an allocation placement, rather than evidence that
// the allocation is stack-local. Consequently an applying escape over Bottom
// is conservatively Unknown. None and Borrow do not require a placement
// transition and preserve the current analysis value.
//
// Runtime/JIT and invalid values are outside the analysis vocabulary. Either
// side crossing that boundary is conservatively Unknown. Return is a valid
// analysis escape even though it has no standalone manifest spelling.
func Displace(current Placement, escape Escape) Placement {
	if !validAnalysisPlacement(current) || !validAnalysisEscape(escape) {
		return Unknown
	}

	required, applies := escape.Placement()
	if !applies {
		return current
	}
	if current == Bottom {
		return Unknown
	}
	return Join(current, required)
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
