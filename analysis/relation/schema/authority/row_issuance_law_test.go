package authority

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
)

func TestCatalogAloneIssuesRowsThroughItsNamedRelation(t *testing.T) {
	catalog, ok := newCatalogFixture(t).seal(t)
	if !ok {
		t.Fatal("fixture catalog unavailable")
	}
	token := fixtureID(t, "row/site")
	row, ok := catalog.IssueRow("orders", token)
	if !ok || !row.Available() || row.Content() != token {
		t.Fatalf("owner catalog refused its row: row=%+v ok=%v", row, ok)
	}
	relation, relationOK := catalog.RelationByName("orders")
	if !relationOK || row.Relation() != relation.ID() || row.Owner() != catalog.OwnerID() {
		t.Fatalf("row escaped its relation fence: row=%+v relation=%+v", row, relation)
	}
}

func TestCatalogRefusesUnknownForeignAndUnavailableRowClaims(t *testing.T) {
	catalog, catalogOK := newCatalogFixture(t).seal(t)
	if !catalogOK {
		t.Fatal("fixture catalog unavailable")
	}
	if row, ok := catalog.IssueRow("relation/absent", fixtureID(t, "row/absent")); ok || row.Available() {
		t.Fatal("unknown relation issued a row")
	}
	if row, ok := catalog.IssueRow("orders", identity.ContentID{}); ok || row.Available() {
		t.Fatal("unavailable content issued a row")
	}
	if row, ok := (Catalog{}).IssueRow("orders", fixtureID(t, "row/zero")); ok || row.Available() {
		t.Fatal("unavailable catalog issued a row")
	}
	if relation, ok := catalog.RelationByName(schema.Key("")); ok || relation.Available() {
		t.Fatal("unavailable relation name resolved")
	}
}
