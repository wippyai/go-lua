package static

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema/plane"
	"github.com/wippyai/go-lua/analysis/schema/query"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/schema/structure/structuretest"
)

var typeSummaryLawLayout = sealTypeSummaryLawLayout()

func sealTypeSummaryLawLayout() *plane.Sealed {
	table, tableOK := structuretest.Table(structure.PublicationPlaneSpecs())
	if !tableOK {
		return nil
	}
	shape, shapeOK := query.NewShape(SummaryResultFamily, query.FoldDistributive)
	if !shapeOK {
		return nil
	}
	sealed, _ := plane.Seal(shape, table, SummaryResultStates, SummaryResultColumns())
	return sealed
}

func TestTypeSummaryCodecPublishesCanonicalClassIdentitiesOnSuppliedValuePlane(t *testing.T) {
	fixture := newTypeFactFixture(t, 151)
	classes := fixture.classes
	fact, ok := classes.TypeFactForOwnTarget(fixture.result)
	if !ok {
		t.Fatal("fixture Target fact unavailable")
	}
	observation := BeginTypeSummary(classes, 3)
	observation.Values[0], observation.Present[0] = fact, true
	observation.Values[2], observation.Present[2] = classes.TypeTop(), true
	observation.Rows = 1
	coordinates := []identity.ContentID{typeSummaryLawID(90), typeSummaryLawID(20), typeSummaryLawID(60)}
	present, rows, payload, encoded := EncodeSummaryResult(typeSummaryLawLayout, observation, coordinates)
	if !encoded || !present || rows != 1 {
		t.Fatal("canonical TypeFact summary was not encoded")
	}
	view, refusal := plane.Admit(typeSummaryLawLayout, present, rows, string(payload))
	if refusal.Available() || !view.Available() || view.Owner() != classes.linkID || view.RowCount() != len(coordinates) {
		t.Fatalf("admitted TypeFact summary = refusal:%s owner:%v rows:%d", refusal, view.Owner(), view.RowCount())
	}
	classID, classIDOK := classes.Identity(fact.class)
	topID, topIDOK := classes.Identity(classes.AnyValue())
	if !classIDOK || !topIDOK {
		t.Fatal("ClassSet identities unavailable")
	}
	wantIDs := []identity.ContentID{coordinates[1], coordinates[2], coordinates[0]}
	wantClasses := []identity.ContentID{{}, topID, classID}
	for index := range wantIDs {
		row, found := view.At(index)
		if !found || row.ID() != wantIDs[index] {
			t.Fatalf("wire coordinate %d = %v/%v, want %v", index, row.ID(), found, wantIDs[index])
		}
		if index == 0 {
			if row.Written() {
				t.Fatal("absent TypeFact coordinate was published")
			}
			continue
		}
		got, gotOK := row.Identity(SummaryColumnClass)
		if !row.Written() || !gotOK || got != wantClasses[index] {
			t.Fatalf("wire class %d = %v/%v written:%v, want %v", index, got, gotOK, row.Written(), wantClasses[index])
		}
	}
}

func TestTypeSummaryCodecIsIndependentOfValueDenseOrder(t *testing.T) {
	fixture := newTypeFactFixture(t, 152)
	classes := fixture.classes
	fact, ok := classes.TypeFactForOwnTarget(fixture.result)
	if !ok {
		t.Fatal("fixture Target fact unavailable")
	}
	firstIDs := []identity.ContentID{typeSummaryLawID(5), typeSummaryLawID(3)}
	first := BeginTypeSummary(classes, 2)
	first.Values[0], first.Present[0] = fact, true
	first.Values[1], first.Present[1] = classes.TypeTop(), true
	first.Rows = 1
	secondIDs := []identity.ContentID{firstIDs[1], firstIDs[0]}
	second := BeginTypeSummary(classes, 2)
	second.Values[0], second.Present[0] = classes.TypeTop(), true
	second.Values[1], second.Present[1] = fact, true
	second.Rows = 1
	_, _, leftPayload, leftOK := EncodeSummaryResult(typeSummaryLawLayout, first, firstIDs)
	_, _, rightPayload, rightOK := EncodeSummaryResult(typeSummaryLawLayout, second, secondIDs)
	if !leftOK || !rightOK || string(leftPayload) != string(rightPayload) {
		t.Fatal("TypeFact summary payload depended on Value dense declaration order")
	}
}

func TestTypeSummaryCodecPreservesZeroRowAbsenceAndRejectsNearestNegatives(t *testing.T) {
	fixture := newTypeFactFixture(t, 153)
	classes := fixture.classes
	coordinates := []identity.ContentID{typeSummaryLawID(10), typeSummaryLawID(11)}
	absent := BeginTypeSummary(classes, len(coordinates))
	present, rows, payload, encoded := EncodeSummaryResult(typeSummaryLawLayout, absent, coordinates)
	if !encoded || present || rows != 0 {
		t.Fatal("all-absent TypeFact summary did not retain zero-row cardinality")
	}
	view, refusal := plane.Admit(typeSummaryLawLayout, present, rows, string(payload))
	if refusal.Available() || view.Present() || view.RowCount() != len(coordinates) {
		t.Fatalf("zero-row TypeFact payload admission = refusal:%s present:%v rows:%d", refusal, view.Present(), view.RowCount())
	}
	for index := range coordinates {
		row, found := view.At(index)
		classID, classFound := row.Identity(SummaryColumnClass)
		if !found || row.Written() || classFound || classID != (identity.ContentID{}) {
			t.Fatalf("zero-row coordinate %d carried a Static class", index)
		}
	}

	foreign := newTypeFactFixture(t, 154)
	foreignFact, factOK := foreign.classes.TypeFactForOwnTarget(foreign.result)
	if !factOK {
		t.Fatal("foreign Target fact unavailable")
	}
	bad := BeginTypeSummary(classes, len(coordinates))
	bad.Values[0], bad.Present[0], bad.Rows = foreignFact, true, 1
	if _, _, _, ok := EncodeSummaryResult(typeSummaryLawLayout, bad, coordinates); ok {
		t.Fatal("codec accepted a summary containing a foreign TypeFact")
	}
	bad = BeginTypeSummary(classes, len(coordinates))
	bad.Rows = 1
	if _, _, _, ok := EncodeSummaryResult(typeSummaryLawLayout, bad, coordinates); ok {
		t.Fatal("codec accepted row cardinality inconsistent with absence")
	}
	bad = BeginTypeSummary(classes, len(coordinates))
	if _, _, _, ok := EncodeSummaryResult(typeSummaryLawLayout, bad, []identity.ContentID{coordinates[0], {}}); ok {
		t.Fatal("codec accepted an unavailable supplied Value coordinate identity")
	}
}

func TestTypeSummaryCodecAllocatesOnlyItsPlanePayload(t *testing.T) {
	fixture := newTypeFactFixture(t, 155)
	fact, ok := fixture.classes.TypeFactForOwnTarget(fixture.result)
	if !ok {
		t.Fatal("fixture Target fact unavailable")
	}
	observation := BeginTypeSummary(fixture.classes, 2)
	observation.Values[0], observation.Present[0] = fact, true
	observation.Values[1], observation.Present[1], observation.Rows = fact, true, 1
	coordinates := []identity.ContentID{typeSummaryLawID(21), typeSummaryLawID(22)}
	var sink []byte
	allocations := testing.AllocsPerRun(100, func() {
		_, _, sink, ok = EncodeSummaryResult(typeSummaryLawLayout, observation, coordinates)
		if !ok || len(sink) == 0 {
			t.Fatal("TypeFact summary codec failed in allocation law")
		}
	})
	if allocations != 1 {
		t.Fatalf("TypeFact summary codec allocations = %v, want one payload allocation", allocations)
	}
}
