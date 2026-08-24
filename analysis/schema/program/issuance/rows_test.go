package issuance

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	schemaissuance "github.com/wippyai/go-lua/analysis/schema/issuance"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	programpublication "github.com/wippyai/go-lua/analysis/schema/program/publication"
)

func TestRowsDistinguishSupportedEmptySpaceFromUnknownSpace(t *testing.T) {
	rows := Rows{}
	if count, supported := rows.Count(RowOccurrence); !supported || count != 0 {
		t.Fatalf("supported empty occurrence space count=%d supported=%v", count, supported)
	}
	if count, supported := rows.Count("row/not-owned-by-program-issuance"); supported || count != 0 {
		t.Fatalf("unknown row space count=%d supported=%v", count, supported)
	}
}

func issuanceRowID(value byte) identity.ContentID {
	var id identity.ContentID
	id[0], id[1] = 0x91, value
	return id
}

func issuanceOccurrence(t testing.TB, kind programschema.OccurrenceKind, id identity.ContentID) programschema.Occurrence {
	t.Helper()
	row, ok := programschema.NewOccurrence(kind, id, identity.ContentID{}, 0, 0, 0, 0, 0, keyspace.FamilyInvalid, keyspace.LiteralValue{}, false)
	if !ok {
		t.Fatal("test occurrence unavailable")
	}
	return row
}

func TestRowsSealRefusesOrphanOwnerReceipts(t *testing.T) {
	table := programIssuanceTable(t)
	for name, add := range map[string]func(*Builder) bool{
		"geometry": func(builder *Builder) bool {
			return builder.AddGeometry(programschema.OccurrenceUnary, issuanceRowID(1), nil, []identity.ContentID{issuanceRowID(2)})
		},
		"closure": func(builder *Builder) bool { return builder.AddClosureProof(issuanceRowID(1)) },
		"predecessor": func(builder *Builder) bool {
			return builder.AddPredecessor(issuanceRowID(1), issuanceRowID(2), issuanceRowID(3))
		},
	} {
		t.Run(name, func(t *testing.T) {
			builder := NewBuilder()
			if !add(builder) {
				t.Fatal("owner receipt refused before seal")
			}
			if rows, ok := builder.Seal(table, &programpublication.Publication{}); ok || rows.publication != nil {
				t.Fatal("orphan owner receipt sealed")
			}
		})
	}
}

func TestRowsSealRefusesAmbiguousReceiptOccurrenceIdentity(t *testing.T) {
	id := issuanceRowID(1)
	publication := &programpublication.Publication{Occurrences: []programschema.Occurrence{
		issuanceOccurrence(t, programschema.OccurrenceUnary, id),
		issuanceOccurrence(t, programschema.OccurrenceSelect, id),
	}}
	builder := NewBuilder()
	if !builder.AddClosureProof(id) {
		t.Fatal("closure proof refused before owner census")
	}
	if rows, ok := builder.Seal(programIssuanceTable(t), publication); ok || rows.publication != nil {
		t.Fatal("receipt for an ambiguous occurrence identity sealed")
	}
}

// TestRowsGeometryRelationIsolatesSharedOccurrenceIdentity pins the
// owner-issued family discriminator in the geometry relation. Call and
// CallActivation deliberately share one Call identity, but their geometry
// remains two distinct occurrence-owned rows.
func TestRowsGeometryRelationIsolatesSharedOccurrenceIdentity(t *testing.T) {
	id := issuanceRowID(10)
	firstPoint := issuanceRowID(11)
	secondPoint := issuanceRowID(12)
	first, firstOK := programschema.NewOccurrence(programschema.OccurrenceCall, id, identity.ContentID{}, 0, 0, 1, 0, 0, keyspace.FamilyInvalid, keyspace.LiteralValue{}, false)
	second, secondOK := programschema.NewOccurrence(programschema.OccurrenceCallActivation, id, identity.ContentID{}, 0, 1, 1, 0, 0, keyspace.FamilyInvalid, keyspace.LiteralValue{}, false)
	firstPointRow, firstPointOK := programschema.NewOccurrencePoint(firstPoint)
	secondPointRow, secondPointOK := programschema.NewOccurrencePoint(secondPoint)
	if !firstOK || !secondOK || !firstPointOK || !secondPointOK {
		t.Fatal("shared-identity occurrence rows unavailable")
	}
	builder := NewBuilder()
	if !builder.AddGeometry(programschema.OccurrenceCall, id, nil, []identity.ContentID{firstPoint}) ||
		!builder.AddGeometry(programschema.OccurrenceCallActivation, id, nil, []identity.ContentID{secondPoint}) {
		t.Fatal("owner-issued geometry refused")
	}
	publication := &programpublication.Publication{
		Occurrences:      []programschema.Occurrence{first, second},
		OccurrencePoints: []programschema.OccurrencePoint{firstPointRow, secondPointRow},
	}
	rows, rowsOK := builder.Seal(programIssuanceTable(t), publication)
	if !rowsOK {
		t.Fatal("shared-identity Program rows refused sealing")
	}
	relation, relationOK := programIssuanceTable(t).Entry(RelationOccurrenceFinishGeometry, schemaissuance.KindRelation)
	if !relationOK {
		t.Fatal("finish geometry relation unavailable")
	}
	for index, want := range []identity.ContentID{firstPoint, secondPoint} {
		source, sourceOK := rows.At(RowOccurrence, index)
		if !sourceOK {
			t.Fatalf("occurrence row %d unavailable", index)
		}
		targets, followOK := rows.Follow(source, relation)
		if !followOK || len(targets) != 1 {
			t.Fatalf("occurrence row %d geometry targets = %d, want one", index, len(targets))
		}
		point, pointOK := rows.Read(targets[0], FieldGeometryPointID)
		if !pointOK || point.Kind != ScalarIdentity || point.Identity != want {
			t.Fatalf("occurrence row %d geometry point = %+v, want %v", index, point, want)
		}
	}
}

func TestSubjectLivenessOccurrenceJoinsCanonicalCallAndFinishGeometry(t *testing.T) {
	rowID, callID, pointID := issuanceRowID(30), issuanceRowID(31), issuanceRowID(32)
	occurrence, occurrenceOK := programschema.NewOccurrence(
		programschema.OccurrenceSubjectLiveness, rowID, identity.ContentID{}, 0,
		0, 1, 0, 1, keyspace.FamilyInvalid, keyspace.LiteralValue{}, false,
	)
	point, pointOK := programschema.NewOccurrencePoint(pointID)
	input, inputOK := programschema.NewOccurrenceInput(callID)
	call, callOK := programschema.NewCall(
		callID, issuanceRowID(33), issuanceRowID(34), issuanceRowID(35), issuanceRowID(36), issuanceRowID(37), issuanceRowID(38),
		issuanceRowID(39), issuanceRowID(40), identity.ContentID{}, identity.ContentID{}, identity.ContentID{},
		programschema.CallFormPlain, 0, 0, 0, 0, 0, 0, false, false,
	)
	if !occurrenceOK || !pointOK || !inputOK || !callOK {
		t.Fatal("subject-liveness issuance fixture unavailable")
	}
	builder := NewBuilder()
	if !builder.AddGeometry(programschema.OccurrenceSubjectLiveness, rowID, nil, []identity.ContentID{pointID}) {
		t.Fatal("subject-liveness finish geometry refused")
	}
	table := programIssuanceTable(t)
	rows, rowsOK := builder.Seal(table, &programpublication.Publication{
		Occurrences: []programschema.Occurrence{occurrence}, OccurrencePoints: []programschema.OccurrencePoint{point},
		OccurrenceInputs: []programschema.OccurrenceInput{input}, Calls: []programschema.Call{call},
	})
	if !rowsOK {
		t.Fatal("subject-liveness issuance rows refused sealing")
	}
	source, sourceOK := rows.At(RowOccurrence, 0)
	callRelation, callRelationOK := table.Entry(RelationOccurrenceCall, schemaissuance.KindRelation)
	geometryRelation, geometryRelationOK := table.Entry(RelationOccurrenceFinishGeometry, schemaissuance.KindRelation)
	calls, callsOK := rows.Follow(source, callRelation)
	points, pointsOK := rows.Follow(source, geometryRelation)
	if !sourceOK || !callRelationOK || !geometryRelationOK || !callsOK || len(calls) != 1 || !pointsOK || len(points) != 1 {
		t.Fatalf("subject-liveness joins source=%t call=%t/%d geometry=%t/%d", sourceOK, callsOK, len(calls), pointsOK, len(points))
	}
	joinedCall, joinedCallOK := rows.Read(calls[0], FieldCallID)
	joinedPoint, joinedPointOK := rows.Read(points[0], FieldGeometryPointID)
	if !joinedCallOK || joinedCall.Identity != callID || !joinedPointOK || joinedPoint.Identity != pointID {
		t.Fatalf("subject-liveness joins call=%+v/%t point=%+v/%t", joinedCall, joinedCallOK, joinedPoint, joinedPointOK)
	}
}
