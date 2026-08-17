package typeauthority

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/domain/type/subtype"
	typetable "github.com/wippyai/go-lua/domain/type/table"
	"github.com/wippyai/go-lua/domain/type/typ"
)

// TestRuntimeSealedRelationDerivesOncePerClosedUniverse is the derive-once law
// of the subtype relation. The relation is a pure function of the sealed closed
// rows, so a second Runtime over the same closed universe must answer every
// ordered pair exactly as the first and must not put a single pair to the
// canonical prover again. The judged-pair counter is the observation: it counts
// prover work, so an equal count across the second seal is the proof that the
// second seal derived nothing.
func TestRuntimeSealedRelationDerivesOncePerClosedUniverse(t *testing.T) {
	corpus := runtimeRelationCorpus()
	first, _, _ := runtimeRelationFixture(corpus)
	if len(first.closedRows) == 0 {
		t.Fatal("sealed relation universe is empty")
	}
	judged := runtimeRelationJudgedPairs.Load()
	if judged == 0 {
		t.Fatal("the first seal of a closed universe judged no ordered pairs; the observation is vacuous")
	}
	seals := runtimeRelationSeals.Load()
	materializations := runtimeRelationMaterializations.Load()
	second, _, _ := runtimeRelationFixture(corpus)
	if repeated := runtimeRelationJudgedPairs.Load(); repeated != judged {
		t.Fatalf("second seal of one closed universe judged %d ordered pairs, want none", repeated-judged)
	}
	if built := runtimeRelationMaterializations.Load(); built != materializations {
		t.Fatalf("second seal of one closed universe materialized %d relations, want none", built-materializations)
	}
	if asked := runtimeRelationSeals.Load(); asked != seals+1 {
		t.Fatalf("the second seal asked for %d relations, want 1", asked-seals)
	}
	if len(second.closedRows) != len(first.closedRows) || second.subtypeStride != first.subtypeStride {
		t.Fatalf("second seal universe = %d rows/stride %d, first = %d rows/stride %d", len(second.closedRows), second.subtypeStride, len(first.closedRows), first.subtypeStride)
	}
	for leftPosition, leftRow := range first.closedRows {
		if second.closedRows[leftPosition] != leftRow {
			t.Fatalf("closed universe position %d = row %d, first seal row %d", leftPosition, second.closedRows[leftPosition], leftRow)
		}
		firstLeft, leftOK := first.InnerAtIndex(leftRow)
		secondLeft, secondLeftOK := second.InnerAtIndex(leftRow)
		if !leftOK || !secondLeftOK {
			t.Fatalf("closed universe position %d is not an owned row in both seals", leftPosition)
		}
		for _, rightRow := range first.closedRows {
			firstRight, rightOK := first.InnerAtIndex(rightRow)
			secondRight, secondRightOK := second.InnerAtIndex(rightRow)
			if !rightOK || !secondRightOK {
				t.Fatalf("closed universe row %d is not an owned row in both seals", rightRow)
			}
			expected, decided := first.Subtype(firstLeft, firstRight)
			answer, secondDecided := second.Subtype(secondLeft, secondRight)
			if answer != expected || secondDecided != decided {
				t.Fatalf("memoized relation row %d <: row %d = %v/%v, first seal %v/%v", leftRow, rightRow, answer, secondDecided, expected, decided)
			}
		}
	}
}

// TestRuntimeSealedRelationSeparatesDistinctClosedUniverses is the other half
// of the same law: the memo key is the closed universe, so a universe that
// gained a row is a different universe and is materialized on its own. A memo
// that collapsed the two would answer the larger universe out of the smaller
// one's bitset.
func TestRuntimeSealedRelationSeparatesDistinctClosedUniverses(t *testing.T) {
	base, _, _ := runtimeRelationFixture(runtimeRelationCorpus())
	extended, _, sources := runtimeRelationFixture(append(runtimeRelationCorpus(), runtimeRelationFixtureType{
		name:  "relation-memo-extension",
		value: typetable.NewRecord().Field("relationMemoExtension", typ.String).Build(),
	}))
	if len(extended.closedRows) <= len(base.closedRows) {
		t.Fatalf("extended universe holds %d closed rows, base holds %d", len(extended.closedRows), len(base.closedRows))
	}
	for leftPosition, leftRow := range extended.closedRows {
		left, leftOK := extended.InnerAtIndex(leftRow)
		if !leftOK {
			t.Fatalf("extended universe position %d is not an owned row", leftPosition)
		}
		for _, rightRow := range extended.closedRows {
			right, rightOK := extended.InnerAtIndex(rightRow)
			if !rightOK {
				t.Fatalf("extended universe row %d is not an owned row", rightRow)
			}
			expected := subtype.IsSubtype(sources[leftRow-1], sources[rightRow-1])
			answer, decided := extended.Subtype(left, right)
			if !decided || answer != expected {
				t.Fatalf("extended relation row %d <: row %d = %v/%v, canonical %v", leftRow, rightRow, answer, decided, expected)
			}
		}
	}
}

// TestRuntimeRelationUniverseIDReadsOnlyCanonicalRowBytes pins the memo key to
// content identity. Two independently constructed universes over equal rows
// share no address, so an equal key is the statement that nothing
// address-shaped reached the digest.
func TestRuntimeRelationUniverseIDReadsOnlyCanonicalRowBytes(t *testing.T) {
	first, _, _ := runtimeRelationFixture(runtimeRelationCorpus())
	second, _, _ := runtimeRelationFixture(runtimeRelationCorpus())
	firstID, firstErr := runtimeRelationUniverseID(first)
	secondID, secondErr := runtimeRelationUniverseID(second)
	if firstErr != nil || secondErr != nil {
		t.Fatalf("relation universe identity errors: %v / %v", firstErr, secondErr)
	}
	if firstID != secondID {
		t.Fatalf("equal closed universes produced distinct relation identities %v / %v", firstID, secondID)
	}
	extended, _, _ := runtimeRelationFixture(append(runtimeRelationCorpus(), runtimeRelationFixtureType{
		name:  "relation-identity-extension",
		value: typetable.NewRecord().Field("relationIdentityExtension", typ.Number).Build(),
	}))
	extendedID, extendedErr := runtimeRelationUniverseID(extended)
	if extendedErr != nil {
		t.Fatal(extendedErr)
	}
	if extendedID == firstID {
		t.Fatal("a universe with an additional closed row shares the base universe's relation identity")
	}
}

// TestRuntimeRelationMemoRejectsRowsWithoutCanonicalIdentity keeps the key
// total. A closed row without canonical bytes has no content identity, so it
// cannot be keyed, and the seal must refuse rather than publish a relation
// under an ambiguous key.
func TestRuntimeRelationMemoRejectsRowsWithoutCanonicalIdentity(t *testing.T) {
	runtime, _, _ := runtimeRelationFixture(runtimeRelationCorpus())
	stripped := &Runtime{rows: make([]runtimeRow, len(runtime.rows)), closedRows: runtime.closedRows}
	copy(stripped.rows, runtime.rows)
	for index := range stripped.rows {
		if stripped.rows[index].closed {
			stripped.rows[index].encoded = nil
			break
		}
	}
	if _, err := runtimeRelationUniverseID(stripped); err == nil {
		t.Fatal("a closed row without canonical bytes produced a relation identity")
	}
}

// TestRuntimeRelationMemoIsKeyedByValue proves the published bitset is reached
// by content identity alone, with no per-shard aliasing: an identity absent
// from the memo must miss even when its shard already holds an entry.
func TestRuntimeRelationMemoIsKeyedByValue(t *testing.T) {
	runtime, _, _ := runtimeRelationFixture(runtimeRelationCorpus())
	universe, err := runtimeRelationUniverseID(runtime)
	if err != nil {
		t.Fatal(err)
	}
	bits, materialized := loadRuntimeRelation(universe)
	if !materialized {
		t.Fatal("a sealed universe is absent from the relation memo")
	}
	if len(bits) != len(runtime.closedRows)*runtime.subtypeStride {
		t.Fatalf("memoized relation holds %d words, want %d", len(bits), len(runtime.closedRows)*runtime.subtypeStride)
	}
	neighbour := universe
	neighbour[len(neighbour)-1] ^= 0xFF
	if _, present := loadRuntimeRelation(neighbour); present {
		t.Fatal("an unmaterialized relation identity resolved inside its shard")
	}
	var absent identity.ContentID
	absent[0] = universe[0]
	absent[1] = universe[1] ^ 0x5A
	if _, present := loadRuntimeRelation(absent); present {
		t.Fatal("an unmaterialized relation identity resolved by shard alone")
	}
}
