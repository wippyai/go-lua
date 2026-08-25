package unary

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
)

func unaryLawContent(t testing.TB, label string) identity.ContentID {
	t.Helper()
	value, ok := identity.DeriveContentID("analysis/engine/relation/operator/unary/law/v1", []byte(label))
	if !ok {
		t.Fatalf("derive content %q", label)
	}
	return value
}

func TestProjectedCellRejectsRefusedPresence(t *testing.T) {
	owner, ok := model.IssueOwnerID(unaryLawContent(t, "owner"))
	if !ok {
		t.Fatal("owner")
	}
	relation, ok := model.IssueRelationID(owner, unaryLawContent(t, "relation"))
	if !ok {
		t.Fatal("relation")
	}
	column, ok := model.IssueColumnID(relation, unaryLawContent(t, "column"))
	if !ok {
		t.Fatal("column")
	}
	typeID, ok := model.IssueTypeID(owner, unaryLawContent(t, "type"))
	if !ok {
		t.Fatal("type")
	}
	reason, ok := model.IssueRefusalID(owner, unaryLawContent(t, "refusal"))
	if !ok {
		t.Fatal("refusal")
	}
	presence, ok := model.NewRefused(reason)
	if !ok {
		t.Fatal("presence")
	}
	manager, err := guard.New(nil)
	if err != nil {
		t.Fatal("guard manager: ", err)
	}
	region, ok := support.True(manager)
	if !ok {
		t.Fatal("region")
	}
	lineage, ok := model.IssueLineageRef(owner, unaryLawContent(t, "lineage"))
	if !ok {
		t.Fatal("lineage")
	}
	cell := ProjectedCell{target: column, typeID: typeID, presence: presence, region: region, lineage: lineage}
	if cell.Available() {
		t.Fatal("refused presence crossed the projection boundary")
	}
}
