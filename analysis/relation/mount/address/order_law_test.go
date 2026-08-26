package address_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/check/certificate"
	"github.com/wippyai/go-lua/analysis/relation/mount/address"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/schema/region"
	"github.com/wippyai/go-lua/analysis/relation/schema/plan"
)

// Bind must retain the certificate's canonical identity order verbatim. The
// crossed member contents make a content-only or physical re-sort observable
// across two different relation authorities.
func TestBindPreservesCertificateOrderAcrossCrossedRelations(t *testing.T) {
	owner, ok := model.IssueOwnerID(identity.ContentID{0xA1})
	if !ok {
		t.Fatal("owner")
	}
	schemaID, ok := model.IssueSchemaID(owner, identity.ContentID{0x04})
	if !ok {
		t.Fatal("schema")
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
	lowRelationSchema := model.DefineRelationSchema(lowRelation, []model.ColumnID{lowColumn}, []model.KeyID{lowKey}, scopeID)
	highRelationSchema := model.DefineRelationSchema(highRelation, []model.ColumnID{highColumn}, []model.KeyID{highKey}, scopeID)
	builder := plan.NewBuilder(schemaID)
	for _, relation := range []model.RelationSchema{highRelationSchema, lowRelationSchema} {
		if !builder.AddRelation(relation) {
			t.Fatal("relation")
		}
	}
	for _, column := range []model.ColumnSchema{
		model.DefineColumnSchema(highColumn, typeID),
		model.DefineColumnSchema(lowColumn, typeID),
	} {
		if !builder.AddColumn(column) {
			t.Fatal("column")
		}
	}
	for _, key := range []model.KeySchema{
		model.DefineKeySchema(highKey, []model.ColumnID{highColumn}),
		model.DefineKeySchema(lowKey, []model.ColumnID{lowColumn}),
	} {
		if !builder.AddKey(key) {
			t.Fatal("key")
		}
	}
	if !builder.AddScope(model.DefineScopeSchema(scopeID, []model.ColumnID{lowColumn, highColumn}, region.True())) {
		t.Fatal("scope")
	}
	schema, ok := builder.Build()
	if !ok {
		t.Fatal("schema")
	}
	cert, refusal := certificate.Check(schema)
	if refusal != nil {
		t.Fatalf("certificate: %v", refusal)
	}
	store, ok := identity.IssueStore()
	if !ok {
		t.Fatal("store")
	}
	fence, ok := address.NewFence(cert.SchemaID(), cert.Digest(), store, identity.MountID{0: 7}, identity.Generation(1))
	if !ok {
		t.Fatal("fence")
	}
	inventory := &inventory{
		fence:     fence,
		relations: map[model.RelationID]uint64{lowRelation: 10, highRelation: 11},
		columns:   map[model.ColumnID]uint64{lowColumn: 20, highColumn: 21},
		keys:      map[model.KeyID]uint64{lowKey: 30, highKey: 31},
		scopes:    map[model.ScopeID]uint64{scopeID: 40},
		calls:     make(map[string]int),
	}
	book, ok := address.Bind(cert, inventory)
	if !ok || !book.Available() {
		t.Fatal("bind")
	}
	if got := book.RelationIDs(); len(got) != 2 || got[0] != cert.Relations()[0].ID() || got[1] != cert.Relations()[1].ID() {
		t.Fatalf("relations=%v, certificate=%v", got, cert.Relations())
	}
	if got := book.ColumnIDs(); len(got) != 2 || got[0] != cert.Columns()[0].ID() || got[1] != cert.Columns()[1].ID() {
		t.Fatalf("columns=%v, certificate=%v", got, cert.Columns())
	}
	if got := book.KeyIDs(); len(got) != 2 || got[0] != cert.Keys()[0].ID() || got[1] != cert.Keys()[1].ID() {
		t.Fatalf("keys=%v, certificate=%v", got, cert.Keys())
	}
}
