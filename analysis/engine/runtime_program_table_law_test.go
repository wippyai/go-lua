package engine

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
)

func TestProgramQueryTableResolvesEveryDeclaredQueryToOnePublishedRow(t *testing.T) {
	fixture := newReceiptQueryMatrixFixture(t, 5, nil, nil)
	program := fixture.graph
	bound := make(map[composition.Key]runtimeQuery, len(fixture.solver.runtime.queries))
	for _, row := range fixture.solver.runtime.queries {
		if row == nil || !row.query().Key().Available() {
			t.Fatal("sealed query row is unavailable")
		}
		bound[row.query().Key()] = row
	}
	rows, ok := bindProgramQueryTable(fixture.addressed, program.graph, bound)
	if !ok || len(rows) != program.graph.QueryCount() {
		t.Fatalf("query table rows=%d ok=%t graph=%d", len(rows), ok, program.graph.QueryCount())
	}
	seen := make(map[composition.Key]struct{}, len(rows))
	for _, row := range rows {
		if row == nil || row.query().Key() == (composition.Key{}) {
			t.Fatal("query table published an empty row")
		}
		if _, duplicate := seen[row.query().Key()]; duplicate {
			t.Fatal("two query ordinals resolve to one row")
		}
		seen[row.query().Key()] = struct{}{}
	}
	if _, ok := bindProgramQueryTable(append([]composition.Key(nil), fixture.addressed[:len(fixture.addressed)-1]...), program.graph, bound); ok {
		t.Fatal("query table accepted a missing published key")
	}
	foreign := append([]composition.Key(nil), fixture.addressed...)
	foreign[0] = composition.Key{}
	if _, ok := bindProgramQueryTable(foreign, program.graph, bound); ok {
		t.Fatal("query table accepted an unavailable published key")
	}
}

func TestProgramObservationTableResolvesEveryIdentityToOneDenseOrdinal(t *testing.T) {
	fixture := newObservedReceiptQueryMatrixFixture(t, 3, nil, nil)
	bound := append([]runtimeObservation(nil), fixture.solver.runtime.observations...)
	rows, ok := bindProgramObservationTable(bound, len(bound))
	if !ok || len(rows) != len(fixture.observations) {
		t.Fatalf("observation table rows=%d ok=%t admissions=%d", len(rows), ok, len(fixture.observations))
	}
	for index, row := range rows {
		if row == nil || row.observationID() != fixture.observationIDs[index] {
			t.Fatalf("observation ordinal %d resolved the wrong row", index)
		}
	}
	if _, ok := bindProgramObservationTable(bound[:len(bound)-1], len(bound)); ok {
		t.Fatal("observation table accepted a truncated dense inventory")
	}
	if _, ok := bindProgramObservationTable(append(bound, nil), len(bound)+1); ok {
		t.Fatal("observation table accepted a nil row")
	}
}

func TestCommittedProgramSealRefusesDuplicateObservationIdentity(t *testing.T) {
	fixture := newObservedReceiptQueryMatrixFixture(t, 1, nil, nil)
	duplicate := append([]ProgramObservationAdmission(nil), fixture.observations...)
	duplicate = append(duplicate, fixture.observations[0])
	if _, failure, ok := fixture.graph.Seal(duplicate); ok || !failure.Available() {
		t.Fatalf("duplicate observation inventory sealed: ok=%t failure=%v", ok, failure)
	}
}

func TestConstructProgramPublishesConstructedGeometry(t *testing.T) {
	fixture := newReceiptQueryMatrixFixture(t, 4, nil, nil)
	program := fixture.graph
	if !program.valid() || program.graph == nil || program.topology == nil || program.directory == nil || !program.graph.OwnsComposition(program.state.schema.cold) || !program.topology.OwnsGraph(program.graph) {
		t.Fatal("ConstructProgram did not publish owned sealed geometry")
	}
	members := 0
	for index := 0; index < program.graph.GroupCount(); index++ {
		group, ok := program.graph.HyperedgeAt(index)
		if !ok {
			t.Fatal("published group")
		}
		members += group.MemberCount()
	}
	if len(program.members) != members || len(program.queries) != program.graph.QueryCount() {
		t.Fatalf("published rows members=%d/%d queries=%d/%d", len(program.members), members, len(program.queries), program.graph.QueryCount())
	}
}
