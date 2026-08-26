package snapshot

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
)

func snapshotLawContent(label string) identity.ContentID {
	value, ok := identity.DeriveContentID("runtime/snapshot/opaque-law/v1", []byte(label))
	if !ok {
		panic("snapshot law content")
	}
	return value
}

func snapshotOpaqueCell(t *testing.T) (ObservationCell, model.LineageRef) {
	t.Helper()
	owner, ok := model.IssueOwnerID(snapshotLawContent("owner"))
	if !ok {
		t.Fatal("owner")
	}
	relation, ok := model.IssueRelationID(owner, snapshotLawContent("relation"))
	if !ok {
		t.Fatal("relation")
	}
	column, ok := model.IssueColumnID(relation, snapshotLawContent("column"))
	if !ok {
		t.Fatal("column")
	}
	typeID, ok := model.IssueTypeID(owner, snapshotLawContent("type"))
	if !ok {
		t.Fatal("type")
	}
	key, ok := model.IssueKeyID(relation, snapshotLawContent("key"))
	if !ok {
		t.Fatal("key")
	}
	denominator, ok := model.NewDenominatorRef(relation, key)
	if !ok {
		t.Fatal("denominator")
	}
	row, ok := model.IssueRowID(relation, snapshotLawContent("row"))
	if !ok {
		t.Fatal("row")
	}
	schema, ok := model.IssueSchemaID(owner, snapshotLawContent("schema"))
	if !ok {
		t.Fatal("schema")
	}
	fence, ok := binding.NewFence(schema, identity.MountID{9}, identity.Generation(1))
	if !ok {
		t.Fatal("fence")
	}
	issuer, ok := binding.NewIssuer(fence)
	if !ok {
		t.Fatal("issuer")
	}
	scope, ok := issuer.IssueScope(snapshotLawContent("scope"))
	if !ok {
		t.Fatal("scope")
	}
	membership, ok := binding.NewMembershipView(relation, []model.RowID{row})
	if !ok {
		t.Fatal("membership")
	}
	witness, ok := issuer.IssueDenominator(denominator, membership, snapshotLawContent("witness"))
	if !ok {
		t.Fatal("witness")
	}
	destination, ok := issuer.IssueCell(witness, scope, column, row)
	if !ok {
		t.Fatal("destination")
	}
	value, ok := issuer.IssueValue(typeID, snapshotLawContent("value"))
	if !ok {
		t.Fatal("value")
	}
	presence, ok := model.NewPresence(model.AuthenticatedOpaque)
	if !ok {
		t.Fatal("presence")
	}
	lineage, ok := model.IssueLineageRef(owner, row.Content())
	if !ok {
		t.Fatal("lineage")
	}
	return ObservationCell{Destination: destination, Column: column, Type: typeID, Presence: presence, Value: value}, lineage
}

// TestObservationRetainsOpaqueRows proves the neutral runtime observation
// keeps an authenticated opaque output instead of reducing it to a terminal
// empty observation. Non-publishing outcomes remain explicitly zero-row.
func TestObservationRetainsOpaqueRows(t *testing.T) {
	cell, lineage := snapshotOpaqueCell(t)
	opaque, ok := outcome.NewResult(outcome.Opaque, model.RefusalID{})
	if !ok {
		t.Fatal("opaque outcome")
	}
	observation, ok := newObservation(opaque, lineage, []ObservationCell{cell})
	if !ok || !observation.Available() || len(observation.Outputs()) != 1 {
		t.Fatalf("opaque observation available=%v ok=%v outputs=%d", observation.Available(), ok, len(observation.Outputs()))
	}
	if output, outputOK := observation.Output(cell.Column); !outputOK || !output.Presence.Is(model.AuthenticatedOpaque) || !output.Value.Available() {
		t.Fatal("opaque output was not retained")
	}
	for _, code := range []outcome.Code{outcome.NoCandidate, outcome.NoSelection} {
		terminal, terminalOK := outcome.NewResult(code, model.RefusalID{})
		if !terminalOK {
			t.Fatal("terminal outcome")
		}
		value, valueOK := newObservation(terminal, lineage, []ObservationCell{})
		if !valueOK || !value.Available() || value.Outcome().Code != code || len(value.Outputs()) != 0 {
			t.Fatalf("%v empty observation available=%v ok=%v outputs=%d", code, value.Available(), valueOK, len(value.Outputs()))
		}
		if rejected, rejectedOK := newObservation(terminal, lineage, []ObservationCell{cell}); rejectedOK || rejected.Available() {
			t.Fatalf("%v accepted a row", code)
		}
	}
}
