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
