package registry_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/check/registry"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/schema/region"
	"github.com/wippyai/go-lua/analysis/relation/schema/plan"
)

// Registry accessors must preserve the order sealed by ExecutionSchema. A
// diagnostic path is content-only, while column/key identity is relation-first;
// crossing those contents exposes a map/path sort that disagrees with mount.
func TestViewPreservesRelationFirstColumnAndKeyOrder(t *testing.T) {
	owner, ok := model.IssueOwnerID(identity.ContentID{0xA1})
	if !ok {
		t.Fatal("owner")
	}
	lowRelation, ok := model.IssueRelationID(owner, identity.ContentID{0x01})
	if !ok {
		t.Fatal("low relation")
	}
	highRelation, ok := model.IssueRelationID(owner, identity.ContentID{0xFE})
	if !ok {
		t.Fatal("high relation")
	}
	lowColumn, ok := model.IssueColumnID(lowRelation, identity.ContentID{0xFE})
	if !ok {
		t.Fatal("low column")
	}
	highColumn, ok := model.IssueColumnID(highRelation, identity.ContentID{0x01})
	if !ok {
		t.Fatal("high column")
	}
	lowKey, ok := model.IssueKeyID(lowRelation, identity.ContentID{0xFE})
	if !ok {
		t.Fatal("low key")
	}
	highKey, ok := model.IssueKeyID(highRelation, identity.ContentID{0x01})
	if !ok {
		t.Fatal("high key")
	}
	typeID, ok := model.IssueTypeID(owner, identity.ContentID{0x02})
	if !ok {
		t.Fatal("type")
	}
	scopeID, ok := model.IssueScopeID(owner, identity.ContentID{0x03})
	if !ok {
		t.Fatal("scope")
	}
	schemaID, ok := model.IssueSchemaID(owner, identity.ContentID{0x04})
	if !ok {
		t.Fatal("schema")
	}
	lowRelationSchema := model.DefineRelationSchema(lowRelation, []model.ColumnID{lowColumn}, []model.KeyID{lowKey}, scopeID)
	highRelationSchema := model.DefineRelationSchema(highRelation, []model.ColumnID{highColumn}, []model.KeyID{highKey}, scopeID)
	lowColumnSchema := model.DefineColumnSchema(lowColumn, typeID)
	highColumnSchema := model.DefineColumnSchema(highColumn, typeID)
	lowKeySchema := model.DefineKeySchema(lowKey, []model.ColumnID{lowColumn})
	highKeySchema := model.DefineKeySchema(highKey, []model.ColumnID{highColumn})
	scope := model.DefineScopeSchema(scopeID, []model.ColumnID{lowColumn, highColumn}, region.True())
	builder := plan.NewBuilder(schemaID)
	for _, relation := range []model.RelationSchema{highRelationSchema, lowRelationSchema} {
		if !builder.AddRelation(relation) {
			t.Fatal("relation")
		}
	}
	for _, column := range []model.ColumnSchema{highColumnSchema, lowColumnSchema} {
		if !builder.AddColumn(column) {
			t.Fatal("column")
		}
	}
	for _, key := range []model.KeySchema{highKeySchema, lowKeySchema} {
		if !builder.AddKey(key) {
			t.Fatal("key")
		}
	}
	if !builder.AddScope(scope) {
		t.Fatal("scope")
	}
	schema, ok := builder.Build()
	if !ok {
		t.Fatal("schema")
	}
	view := registry.Build(schema)
	if !view.Valid() {
		t.Fatalf("registry refused valid schema: %v", view.Issues())
	}
	columns := view.Columns()
	keys := view.Keys()
	if len(columns) != 2 || columns[0].ID() != lowColumn || columns[1].ID() != highColumn {
		t.Fatalf("columns=%v, want relation-first low/high", columns)
	}
	if len(keys) != 2 || keys[0].ID() != lowKey || keys[1].ID() != highKey {
		t.Fatalf("keys=%v, want relation-first low/high", keys)
	}
}
