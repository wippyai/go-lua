package plan_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/schema/plan"
)

func TestTypeCapabilitiesAreSealedIntoExecutionSchemaDigest(t *testing.T) {
	ownerToken, ok := identity.DeriveContentID("plan-type-capability-law/v1", []byte("owner"))
	if !ok {
		t.Fatal("owner token")
	}
	owner, ok := model.IssueOwnerID(ownerToken)
	if !ok {
		t.Fatal("owner")
	}
	typeToken, ok := identity.DeriveContentID("plan-type-capability-law/v1", []byte("type"))
	if !ok {
		t.Fatal("type token")
	}
	typeID, ok := model.IssueTypeID(owner, typeToken)
	if !ok {
		t.Fatal("type")
	}
	ascending, ok := model.NewAscendingCapability(typeID)
	if !ok {
		t.Fatal("ascending capability")
	}
	decodeOnly, ok := model.NewDecodeOnlyCapability(typeID)
	if !ok {
		t.Fatal("decode-only capability")
	}
	firstBuilder := plan.NewBuilder(mustSchema(t, owner, "schema"))
	if !firstBuilder.AddTypeCapability(ascending) {
		t.Fatal("add ascending capability")
	}
	first, ok := firstBuilder.Build()
	if !ok {
		t.Fatal("build ascending schema")
	}
	secondBuilder := plan.NewBuilder(first.SchemaID())
	if !secondBuilder.AddTypeCapability(decodeOnly) {
		t.Fatal("add decode-only capability")
	}
	second, ok := secondBuilder.Build()
	if !ok {
		t.Fatal("build decode-only schema")
	}
	if first.Digest() == second.Digest() {
		t.Fatal("capability policy mutation did not change schema digest")
	}
	capabilities := first.TypeCapabilities()
	if len(capabilities) != 1 || !capabilities[0].Equal(ascending) {
		t.Fatalf("schema lost sealed capability: %+v", capabilities)
	}
	capabilities[0] = model.TypeCapability{}
	if got := first.TypeCapabilities(); len(got) != 1 || !got[0].Equal(ascending) {
		t.Fatal("schema exposed mutable capability storage")
	}
}

func mustSchema(t *testing.T, owner model.OwnerID, label string) model.SchemaID {
	t.Helper()
	token, ok := identity.DeriveContentID("plan-type-capability-law/v1", []byte(label))
	if !ok {
		t.Fatal("schema token")
	}
	schema, ok := model.IssueSchemaID(owner, token)
	if !ok {
		t.Fatal("schema")
	}
	return schema
}
