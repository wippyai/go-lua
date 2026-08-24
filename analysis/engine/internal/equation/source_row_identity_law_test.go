package equation

import "testing"

// sourceRowIdentityFixture admits one complete open source topology: two
// ordinary Sites, one formal port Site, two Occurrences and one Operand. The
// rows are returned open so a law can observe an identity before the phase
// barrier.
func sourceRowIdentityFixture(t testing.TB, batch *Batch, base byte) (Site, Occurrence, Operand) {
	t.Helper()
	left, leftOK := batch.AdmitSite(boundaryKey(base), EmptyScope(), FalseExpr(), InitAbsent)
	right, rightOK := batch.AdmitSite(boundaryKey(base+1), EmptyScope(), FalseExpr(), InitAbsent)
	_, portOK := batch.AdmitFormalPort(boundaryKey(base+2), PortExport, nil)
	occurrence, occurrenceOK := batch.At(left)
	_, secondOK := batch.From(right, boundaryKey(base+3))
	operand, operandOK := batch.AdmitOperand(occurrence, boundaryKey(base+4))
	if !leftOK || !rightOK || !portOK || !occurrenceOK || !secondOK || !operandOK {
		t.Fatal("source row fixture")
	}
	return left, occurrence, operand
}

// sourceRowIdentityFixtureRows is the fixture's row population: three Sites,
// two Occurrences and one Operand.
const sourceRowIdentityFixtureRows = 6

// A source row's portable identity is issued exactly once, at the moment the
// row is admitted. Deferring the mint to seal forces every open observer to
// rebuild the row's whole ancestor chain, so the cost of reading one identity
// scales with the number of surfaces that anchor it rather than with the
// number of rows. The count is the proof: admission owes one derivation per
// row, observation owes none, and seal owes one per row to authenticate the
// identity it already stores.
func TestSourceRowIdentityIsDerivedOnceAtAdmissionAndOnceToAuthenticate(t *testing.T) {
	before := sourceRowIdentityDerivations.Load()
	batch := NewBatch()
	_, occurrence, operand := sourceRowIdentityFixture(t, batch, 71)
	if admitted := sourceRowIdentityDerivations.Load() - before; admitted != sourceRowIdentityFixtureRows {
		t.Fatalf("admission derived %d row identities, want %d", admitted, sourceRowIdentityFixtureRows)
	}

	mark := sourceRowIdentityDerivations.Load()
	for anchor := 0; anchor < 5; anchor++ {
		if !occurrence.IdentityKey().Available() || !operand.IdentityKey().Available() {
			t.Fatal("open row identity")
		}
	}
	if anchored := sourceRowIdentityDerivations.Load() - mark; anchored != 0 {
		t.Fatalf("anchoring an open row derived its identity %d more times, want 0", anchored)
	}

	mark = sourceRowIdentityDerivations.Load()
	if !batch.Seal() {
		t.Fatal("seal")
	}
	if authenticated := sourceRowIdentityDerivations.Load() - mark; authenticated != sourceRowIdentityFixtureRows {
		t.Fatalf("seal derived %d row identities to authenticate, want %d", authenticated, sourceRowIdentityFixtureRows)
	}
}

// The identity a row carries while the Batch is open is the identity it
// carries after seal. A deferred surface binds to its pre-seal source through
// IdentityKey, so an open observation that disagreed with the sealed row would
// split one row into two identities.
func TestOpenSourceRowIdentityEqualsItsSealedIdentity(t *testing.T) {
	batch := NewBatch()
	_, occurrence, operand := sourceRowIdentityFixture(t, batch, 81)
	openOccurrence, openOperand := occurrence.IdentityKey(), operand.IdentityKey()
	if !openOccurrence.Available() || !openOperand.Available() {
		t.Fatal("open row identity")
	}
	if !batch.Seal() {
		t.Fatal("seal")
	}
	if occurrence.Key() != openOccurrence || occurrence.IdentityKey() != openOccurrence {
		t.Fatal("an Occurrence sealed under an identity other than the one it published while open")
	}
	if operand.Key() != openOperand || operand.IdentityKey() != openOperand {
		t.Fatal("an Operand sealed under an identity other than the one it published while open")
	}
}

// Seal authenticates the identity each row already stores. The stored value is
// the source of truth for every consumer, so seal re-derives only to prove the
// row and its identity still agree and refuses the whole topology when they do
// not.
func TestSealRefusesASourceRowWhoseStoredIdentityDisagreesWithItsRow(t *testing.T) {
	for _, law := range []struct {
		name    string
		corrupt func(batch *Batch)
		refusal SealFailure
	}{
		{"site", func(batch *Batch) { batch.sites[0].key = boundaryKey(99) }, sealRefused(SealFailureFamilySource, "site-identity")},
		{"occurrence", func(batch *Batch) { batch.occurrences[0].key = boundaryKey(99) }, sealRefused(SealFailureFamilySource, "occurrence-identity")},
		{"operand", func(batch *Batch) { batch.operands[0].key = boundaryKey(99) }, sealRefused(SealFailureFamilySource, "operand-identity")},
	} {
		t.Run(law.name, func(t *testing.T) {
			batch := NewBatch()
			sourceRowIdentityFixture(t, batch, 91)
			law.corrupt(batch)
			failure := batch.SealWithFailure()
			if !failure.Available() {
				t.Fatal("a row whose stored identity disagrees with its own fields sealed")
			}
			if failure != law.refusal {
				t.Fatalf("seal refused at %s, want %s", failure, law.refusal)
			}
		})
	}
}

// Portable row identity is a function of the row, never of its ordinal. Two
// Batches that admit the same rows in opposite order therefore seal to the
// same Batch identity and the same per-row identities, even though the rows
// occupy different ordinals.
func TestSourceRowIdentitySurvivesAdmissionOrder(t *testing.T) {
	forward := NewBatch()
	forwardFirst, forwardFirstOK := forward.AdmitSite(boundaryKey(101), EmptyScope(), FalseExpr(), InitAbsent)
	forwardSecond, forwardSecondOK := forward.AdmitSite(boundaryKey(102), EmptyScope(), FalseExpr(), InitAbsent)

	reversed := NewBatch()
	reversedSecond, reversedSecondOK := reversed.AdmitSite(boundaryKey(102), EmptyScope(), FalseExpr(), InitAbsent)
	reversedFirst, reversedFirstOK := reversed.AdmitSite(boundaryKey(101), EmptyScope(), FalseExpr(), InitAbsent)

	if !forwardFirstOK || !forwardSecondOK || !reversedFirstOK || !reversedSecondOK {
		t.Fatal("ordered admission")
	}
	if forwardFirst.row == reversedFirst.row {
		t.Fatal("reversed admission did not move the rows it admits")
	}
	if !forward.Seal() || !reversed.Seal() {
		t.Fatal("seal")
	}
	if forward.Key() != reversed.Key() {
		t.Fatal("admission order changed the sealed Batch identity")
	}
	if forwardFirst.Key() != reversedFirst.Key() || forwardSecond.Key() != reversedSecond.Key() {
		t.Fatal("admission order changed a portable row identity")
	}
}

// An unrelated row shifts the ordinals of the rows admitted after it and no
// identity at all. Row addressing is Batch-local; identity is portable.
func TestAnUnrelatedSiteShiftsOrdinalsAndNoIdentity(t *testing.T) {
	plain := NewBatch()
	plainSite, plainOccurrence, plainOperand := sourceRowIdentityFixture(t, plain, 111)

	extended := NewBatch()
	if _, ok := extended.AdmitSite(boundaryKey(200), EmptyScope(), FalseExpr(), InitAbsent); !ok {
		t.Fatal("unrelated site")
	}
	extendedSite, extendedOccurrence, extendedOperand := sourceRowIdentityFixture(t, extended, 111)

	if plainSite.row == extendedSite.row {
		t.Fatal("the unrelated site did not shift the ordinals that follow it")
	}
	if !plain.Seal() || !extended.Seal() {
		t.Fatal("seal")
	}
	if plainSite.Key() != extendedSite.Key() || plainOccurrence.Key() != extendedOccurrence.Key() ||
		plainOperand.Key() != extendedOperand.Key() {
		t.Fatal("an unrelated row changed a portable row identity")
	}
}
