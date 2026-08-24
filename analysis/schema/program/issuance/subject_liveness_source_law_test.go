package issuance

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	schemaissuance "github.com/wippyai/go-lua/analysis/schema/issuance"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	"github.com/wippyai/go-lua/analysis/schema/program/lifecycle"
	programpublication "github.com/wippyai/go-lua/analysis/schema/program/publication"
)

// livenessFixture seals one publication holding count subject-liveness rows
// and the executable occurrence view of each, in the same order the compiler
// emits them.
func livenessFixture(t *testing.T, count int) (Rows, schemaissuance.Table, []identity.ContentID) {
	t.Helper()
	table := programIssuanceTable(t)
	builder := NewBuilder()
	publication := &programpublication.Publication{}
	ids := make([]identity.ContentID, 0, count)
	for index := 0; index < count; index++ {
		call, route, subject := issuanceRowID(byte(60+index)), issuanceRowID(byte(70+index)), issuanceRowID(byte(80+index))
		id, idOK := lifecycle.SubjectLivenessIdentity(call, route, lifecycle.SubjectLivenessCell, subject)
		if !idOK {
			t.Fatal("liveness identity unavailable")
		}
		row, rowOK := lifecycle.NewSubjectLiveness(id, call, route, identity.ContentID{}, identity.ContentID{}, subject, lifecycle.SubjectLivenessCell, lifecycle.SubjectLivenessLive)
		occurrence, occurrenceOK := programschema.NewOccurrence(
			programschema.OccurrenceSubjectLiveness, id, identity.ContentID{}, 0,
			uint32(index), 1, uint32(index), 1, keyspace.FamilyInvalid, keyspace.LiteralValue{}, false,
		)
		point, pointOK := programschema.NewOccurrencePoint(issuanceRowID(byte(90 + index)))
		input, inputOK := programschema.NewOccurrenceInput(call)
		if !rowOK || !occurrenceOK || !pointOK || !inputOK {
			t.Fatalf("liveness fixture row %d unavailable", index)
		}
		publication.Lifecycle.SubjectLifetimes = append(publication.Lifecycle.SubjectLifetimes, row)
		publication.Occurrences = append(publication.Occurrences, occurrence)
		publication.OccurrencePoints = append(publication.OccurrencePoints, point)
		publication.OccurrenceInputs = append(publication.OccurrenceInputs, input)
		ids = append(ids, id)
	}
	rows, sealed := builder.Seal(table, publication)
	if !sealed {
		t.Fatal("liveness publication refused sealing")
	}
	return rows, table, ids
}

// TestSubjectLivenessIsAnAddressableRowSpace states that the liveness family
// is reachable by ordinal, not only by identity. A candidate ordinal is only
// an address if the space it indexes is declared.
func TestSubjectLivenessIsAnAddressableRowSpace(t *testing.T) {
	rows, _, ids := livenessFixture(t, 3)
	count, supported := rows.Count(RowSubjectLiveness)
	if !supported || count != len(ids) {
		t.Fatalf("liveness space count=%d supported=%t, want %d", count, supported, len(ids))
	}
	for index, want := range ids {
		row, rowOK := rows.At(RowSubjectLiveness, index)
		field, fieldOK := rows.Read(row, FieldSubjectLivenessID)
		if !rowOK || !fieldOK || field.Kind != ScalarIdentity || field.Identity != want {
			t.Fatalf("liveness row %d = %+v/%t, want %v", index, field, fieldOK, want)
		}
	}
	if _, rowOK := rows.At(RowSubjectLiveness, len(ids)); rowOK {
		t.Fatal("liveness space admitted an ordinal past its census")
	}
}

// TestOccurrenceReachesExactlyOneLivenessRow is the join the issuance machine
// resolves a candidate through. It must be total and single-valued over the
// executable views, because a rule issued on that family runs once per row and
// an ambiguous source would leave the ordinal undetermined.
func TestOccurrenceReachesExactlyOneLivenessRow(t *testing.T) {
	rows, table, ids := livenessFixture(t, 3)
	relation, relationOK := table.Entry(RelationOccurrenceSubjectLiveness, schemaissuance.KindRelation)
	if !relationOK {
		t.Fatal("subject-liveness source relation undeclared")
	}
	for index, want := range ids {
		source, sourceOK := rows.At(RowOccurrence, index)
		targets, followed := rows.Follow(source, relation)
		if !sourceOK || !followed || len(targets) != 1 {
			t.Fatalf("occurrence %d reached %d liveness rows, want one", index, len(targets))
		}
		if targets[0].Space != RowSubjectLiveness || targets[0].Index != index {
			t.Fatalf("occurrence %d reached %+v, want liveness ordinal %d", index, targets[0], index)
		}
		field, fieldOK := rows.Read(targets[0], FieldSubjectLivenessID)
		if !fieldOK || field.Identity != want {
			t.Fatalf("occurrence %d reached identity %+v, want %v", index, field, want)
		}
	}
}

// TestNonLivenessOccurrenceReachesNoLivenessRow keeps the relation honest at
// its other end: an occurrence of any other kind shares no identity with a
// liveness row, so the source resolves to nothing rather than to a neighbour.
func TestNonLivenessOccurrenceReachesNoLivenessRow(t *testing.T) {
	table := programIssuanceTable(t)
	builder := NewBuilder()
	unary := issuanceOccurrence(t, programschema.OccurrenceUnary, issuanceRowID(50))
	call, route, subject := issuanceRowID(51), issuanceRowID(52), issuanceRowID(53)
	id, idOK := lifecycle.SubjectLivenessIdentity(call, route, lifecycle.SubjectLivenessCell, subject)
	if !idOK {
		t.Fatal("liveness identity unavailable")
	}
	liveness, livenessOK := lifecycle.NewSubjectLiveness(id, call, route, identity.ContentID{}, identity.ContentID{}, subject, lifecycle.SubjectLivenessCell, lifecycle.SubjectLivenessLive)
	if !livenessOK {
		t.Fatal("liveness row unavailable")
	}
	publication := &programpublication.Publication{Occurrences: []programschema.Occurrence{unary}}
	publication.Lifecycle.SubjectLifetimes = []lifecycle.SubjectLiveness{liveness}
	rows, sealed := builder.Seal(table, publication)
	relation, relationOK := table.Entry(RelationOccurrenceSubjectLiveness, schemaissuance.KindRelation)
	source, sourceOK := rows.At(RowOccurrence, 0)
	if !sealed || !relationOK || !sourceOK {
		t.Fatal("mismatched fixture refused sealing")
	}
	targets, followed := rows.Follow(source, relation)
	if !followed || len(targets) != 0 {
		t.Fatalf("unary occurrence reached %d liveness rows, want none", len(targets))
	}
}
