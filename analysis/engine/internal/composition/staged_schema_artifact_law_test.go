package composition

import "testing"

// TestStagedReadSchemaArtifactReplaysCanonicalIdentity exercises the narrow
// cold artifact boundary. Seal hashes the complete immutable schema and
// Rules returns a detached replay row; neither operation needs a live Ref,
// Unit route, candidate vector, or candidate-by-root surface.
func TestStagedReadSchemaArtifactReplaysCanonicalIdentity(t *testing.T) {
	const stagedOrdinal = 2
	const stagedInput = 3
	locatorLaw := coldKey(81)
	target := coldKey(82)
	base := stagedSchemaArtifactCandidate(locatorLaw, target, []uint64{0, 1})

	sealed, ok := Seal(base)
	if !ok || sealed == nil {
		t.Fatal("seal staged schema artifact")
	}
	rules := sealed.Rules()
	if len(rules) != 1 || rules[0].Key != locatorLaw || len(rules[0].Reads) != stagedOrdinal+1 {
		t.Fatalf("owning Rule/read ordinal = %#v", rules)
	}
	read := rules[0].Reads[stagedOrdinal]
	if read.Kind != ReadSelect || read.Input != stagedInput || read.Factor != target || read.Semantic != target || read.Normalizer.Available() ||
		len(read.Dependencies) != 2 || read.Dependencies[0] != 0 || read.Dependencies[1] != 1 {
		t.Fatalf("staged surface = %#v", read)
	}
	if rules[0].Reads[0].Kind != ReadExact || rules[0].Reads[1].Kind != ReadExact {
		t.Fatalf("staged predecessors = %#v", rules[0].Reads)
	}

	// Replaying the detached canonical rows produces the same artifact
	// identity, including the owning locator law, ordinal, target exact form,
	// and ordered predecessor projection.
	replayed, replayedOK := Seal(Candidate{Factors: sealed.Factors(), Rules: rules, Queries: sealed.Queries()})
	if !replayedOK || replayed == nil || replayed.ID() != sealed.ID() {
		t.Fatal("staged schema artifact identity did not replay")
	}

	changedLaw := stagedSchemaArtifactCandidate(coldKey(83), target, []uint64{0, 1})
	changedLawSealed, changedLawOK := Seal(changedLaw)
	if !changedLawOK || changedLawSealed == nil || changedLawSealed.ID() == sealed.ID() {
		t.Fatal("owning Rule locator law was omitted from staged artifact identity")
	}

	changedTarget := stagedSchemaArtifactCandidate(locatorLaw, coldKey(84), []uint64{0, 1})
	changedTargetSealed, changedTargetOK := Seal(changedTarget)
	if !changedTargetOK || changedTargetSealed == nil || changedTargetSealed.ID() == sealed.ID() {
		t.Fatal("staged target Factor/exact form was omitted from artifact identity")
	}

	changedDependencies := stagedSchemaArtifactCandidate(locatorLaw, target, []uint64{0})
	changedDependenciesSealed, changedDependenciesOK := Seal(changedDependencies)
	if !changedDependenciesOK || changedDependenciesSealed == nil || changedDependenciesSealed.ID() == sealed.ID() {
		t.Fatal("staged predecessor dependencies were omitted from artifact identity")
	}

	unordered := stagedSchemaArtifactCandidate(locatorLaw, target, []uint64{1, 0})
	if rejected, accepted := Seal(unordered); accepted || rejected != nil {
		t.Fatal("unordered staged predecessor dependencies were accepted")
	}

	// The Rule semantic owns the locator/coverage law. A staged surface is the
	// exact target Factor, so it cannot carry an independent semantic or a
	// summary normalizer.
	foreignSurface := stagedSchemaArtifactCandidate(locatorLaw, target, []uint64{0, 1})
	foreignSurface.Rules[0].Reads[stagedOrdinal].Semantic = locatorLaw
	if rejected, accepted := Seal(foreignSurface); accepted || rejected != nil {
		t.Fatal("staged surface admitted a second semantic identity")
	}
	normalized := stagedSchemaArtifactCandidate(locatorLaw, target, []uint64{0, 1})
	normalized.Rules[0].Reads[stagedOrdinal].Normalizer = target
	if rejected, accepted := Seal(normalized); accepted || rejected != nil {
		t.Fatal("staged exact surface admitted a normalizer")
	}
}

func stagedSchemaArtifactCandidate(locatorLaw, target Key, dependencies []uint64) Candidate {
	output, left, right := coldKey(78), coldKey(79), coldKey(80)
	return Candidate{
		// Both admissible targets remain in the Factor schema so changing the
		// staged target below changes the read surface, not the Factor universe.
		Factors: []Factor{{Key: output}, {Key: left}, {Key: right}, {Key: coldKey(82)}, {Key: coldKey(84)}},
		Rules: []Rule{{
			Key: locatorLaw, OperandFamily: coldKey(249), OutputKind: FactorOutput, Output: output, Inputs: 4,
			Reads: []Read{
				{Kind: ReadExact, Input: 0, Factor: left},
				{Kind: ReadExact, Input: 2, Factor: right},
				{Kind: ReadSelect, Input: 3, Factor: target, Semantic: target, Dependencies: append([]uint64(nil), dependencies...)},
			},
			Writes: []Write{{Kind: WriteExact, Factor: output}},
		}},
	}
}
