package placement

// AuthenticateFactCell validates one value read from Placement's canonical
// product factor. Sparse transport is lawful only for the exact owner-issued
// default. A reachable cell must carry an explicit retain verdict; component
// Bottom/Absent values are lattice machinery, not solved allocation facts.
func AuthenticateFactCell(fact Fact, present, available bool) (Fact, bool) {
	if !available || !fact.Valid() || fact.Class == Bottom || fact.RetainEscape == EvidenceAbsent ||
		!present && fact != DefaultFact() {
		return invalidFact(), false
	}
	return fact, true
}

// displaceClassChecked is the class component of the canonical Fact
// transition. It is private so no producer can update placement while omitting
// retain provenance.
func displaceClassChecked(current Placement, escape Escape) (Placement, bool) {
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

// DisplaceFactChecked is the canonical Placement transition. The same
// operation that raises Class also records whether a retaining boundary has
// been crossed, so provenance cannot diverge from placement or be reconstructed
// by a downstream consumer. None and Borrow preserve both components. Every
// applying escape establishes a retain proof on the successor point.
func DisplaceFactChecked(current Fact, escape Escape) (Fact, bool) {
	if !current.Valid() || current.Class == Bottom || current.RetainEscape == EvidenceAbsent || !validAnalysisEscape(escape) {
		return invalidFact(), false
	}
	if escape == None || escape == Borrow {
		return current, true
	}
	class, ok := displaceClassChecked(current.Class, escape)
	if !ok {
		return invalidFact(), false
	}
	return Fact{Class: class, RetainEscape: EvidenceProven}, true
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
