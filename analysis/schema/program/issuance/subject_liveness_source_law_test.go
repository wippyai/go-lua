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

// livenessFixture seals one publication holding count subject-liveness spans
// and the executable occurrence view of each, in the same order the compiler
// emits them.
func livenessFixture(t *testing.T, count int) (Rows, schemaissuance.Table, []identity.ContentID) {
	t.Helper()
	table := programIssuanceTable(t)
	builder := NewBuilder()
	publication := &programpublication.Publication{}
	ids := make([]identity.ContentID, 0, count)
	for index := 0; index < count; index++ {
		call, subject := issuanceRowID(byte(60+index)), issuanceRowID(byte(80+index))
		id, idOK := lifecycle.SubjectLivenessSpanIdentity(lifecycle.SubjectLivenessCell, subject, uint32(index), uint32(index))
		if !idOK {
			t.Fatal("liveness span identity unavailable")
		}
		row, rowOK := lifecycle.NewSubjectLivenessSpan(id, subject, lifecycle.SubjectLivenessCell, uint32(index), uint32(index), lifecycle.SubjectLivenessLive)
		occurrence, occurrenceOK := programschema.NewOccurrence(
			programschema.OccurrenceSubjectLiveness, id, identity.ContentID{}, 0,
			uint32(index), 1, uint32(index), 1, keyspace.FamilyInvalid, keyspace.LiteralValue{}, false,
		)
		point, pointOK := programschema.NewOccurrencePoint(issuanceRowID(byte(90 + index)))
		input, inputOK := programschema.NewOccurrenceInput(call)
		if !rowOK || !occurrenceOK || !pointOK || !inputOK {
			t.Fatalf("liveness fixture row %d unavailable", index)
		}
		publication.Lifecycle.SubjectSpans = append(publication.Lifecycle.SubjectSpans, row)
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
	count, supported := rows.Count(RowSubjectLivenessSpan)
	if !supported || count != len(ids) {
		t.Fatalf("liveness space count=%d supported=%t, want %d", count, supported, len(ids))
	}
	for index, want := range ids {
		row, rowOK := rows.At(RowSubjectLivenessSpan, index)
		field, fieldOK := rows.Read(row, FieldSubjectLivenessSpanID)
		if !rowOK || !fieldOK || field.Kind != ScalarIdentity || field.Identity != want {
			t.Fatalf("liveness row %d = %+v/%t, want %v", index, field, fieldOK, want)
		}
	}
	if _, rowOK := rows.At(RowSubjectLivenessSpan, len(ids)); rowOK {
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
		if targets[0].Space != RowSubjectLivenessSpan || targets[0].Index != index {
			t.Fatalf("occurrence %d reached %+v, want liveness ordinal %d", index, targets[0], index)
		}
		field, fieldOK := rows.Read(targets[0], FieldSubjectLivenessSpanID)
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
	subject := issuanceRowID(53)
	id, idOK := lifecycle.SubjectLivenessSpanIdentity(lifecycle.SubjectLivenessCell, subject, 0, 0)
	if !idOK {
		t.Fatal("liveness span identity unavailable")
	}
	liveness, livenessOK := lifecycle.NewSubjectLivenessSpan(id, subject, lifecycle.SubjectLivenessCell, 0, 0, lifecycle.SubjectLivenessLive)
	if !livenessOK {
		t.Fatal("liveness span unavailable")
	}
	publication := &programpublication.Publication{Occurrences: []programschema.Occurrence{unary}}
	publication.Lifecycle.SubjectSpans = []lifecycle.SubjectLivenessSpan{liveness}
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
