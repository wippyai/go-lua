package relcompile

import (
	"errors"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/schema/region"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
	"github.com/wippyai/go-lua/analysis/schema"
)

// axisEntry is the owner-issued surface address a domain axis is declared at.
func axisEntry(key schema.Key) schema.EntryReference {
	return schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: key}
}

// issue stands in for the declaration surface that owns one entry: it derives
// the token under the surface's own domain separation, exactly as the owning
// surface does, and hands it to the registry. The registry itself derives
// nothing.
func issue(t *testing.T, label string) identity.ContentID {
	t.Helper()
	value, ok := identity.DeriveContentID("relcompile-law/v1", []byte(label))
	if !ok {
		t.Fatalf("derive %q", label)
	}
	return value
}

func refusalOf(t *testing.T, err error) Refusal {
	t.Helper()
	var refusal Refusal
	if !errors.As(err, &refusal) {
		t.Fatalf("error %v is not a Refusal", err)
	}
	return refusal
}

// TestResolvedNameRoundTripsToTheOwnerIssuedIdentity states that a name the
// owner installed resolves to exactly the identity the owner issued, and that
// the schema row the registry projects carries the same identity.
func TestResolvedNameRoundTripsToTheOwnerIssuedIdentity(t *testing.T) {
	registry := NewRegistry()
	heap := axisEntry("heap")
	if err := registry.InstallOwner(heap, issue(t, "axis/heap")); err != nil {
		t.Fatalf("install owner: %v", err)
	}
	scope := EntryName(schema.SurfaceKindAxis, "heap")
	if err := registry.InstallScope(scope, issue(t, "scope/heap"), region.True()); err != nil {
		t.Fatalf("install scope: %v", err)
	}
	if err := registry.InstallType(NewName(heap, "type/route"), issue(t, "type/route")); err != nil {
		t.Fatalf("install type: %v", err)
	}
	relation := NewName(heap, "heap/formal-freeze-routes")
	relationToken := issue(t, "relation/heap/formal-freeze-routes")
	if err := registry.InstallRelation(relation, relationToken, scope); err != nil {
		t.Fatalf("install relation: %v", err)
	}
	column := NewName(heap, "heap/formal-freeze-route-key")
	columnToken := issue(t, "column/heap/formal-freeze-route-key")
	if err := registry.InstallColumn(column, columnToken, relation, NewName(heap, "type/route")); err != nil {
		t.Fatalf("install column: %v", err)
	}

	site := Site{Rule: "heap-formal-freeze", Path: "program.joins[0].relation"}
	resolved, err := registry.Relation(site, relation)
	if err != nil {
		t.Fatalf("resolve relation: %v", err)
	}
	if resolved.Content() != relationToken {
		t.Fatal("the resolved relation carries an identity the owner did not issue")
	}
	resolvedColumn, err := registry.Column(site, column)
	if err != nil {
		t.Fatalf("resolve column: %v", err)
	}
	if resolvedColumn.Content() != columnToken || resolvedColumn.Relation() != resolved {
		t.Fatal("the resolved column is not the owner-issued column of its relation")
	}

	owner, err := registry.Owner(site, heap)
	if err != nil {
		t.Fatalf("resolve owner: %v", err)
	}
	schemaID, ok := model.IssueSchemaID(owner, issue(t, "schema/heap"))
	if !ok {
		t.Fatal("issue schema identity")
	}
	declaration := registry.Declaration(schemaID)
	if len(declaration.Relations) != 1 || declaration.Relations[0].ID() != resolved {
		t.Fatal("the projected relation schema does not carry the resolved identity")
	}
	if !declaration.Relations[0].HasColumn(resolvedColumn) {
		t.Fatal("the projected relation schema does not hold its installed column")
	}
	if len(declaration.Columns) != 1 || declaration.Columns[0].ID() != resolvedColumn {
		t.Fatal("the projected column schema does not carry the resolved identity")
	}
}

// TestTypeCapabilityIsAnExplicitOwnerStatement proves the registry carries a
// type policy only when its owner declares one. Lowering transports the exact
// sealed policy into ExecutionSchema; it never guesses from a column or a
// signature contract.
func TestTypeCapabilityIsAnExplicitOwnerStatement(t *testing.T) {
	registry := NewRegistry()
	heap := axisEntry("heap")
	if err := registry.InstallOwner(heap, issue(t, "capability/axis/heap")); err != nil {
		t.Fatalf("install owner: %v", err)
	}
	typeName := NewName(heap, "type/candidate")
	if err := registry.InstallType(typeName, issue(t, "capability/type/candidate")); err != nil {
		t.Fatalf("install type: %v", err)
	}
	if _, err := registry.TypeCapability(Site{Path: "test.capability"}, typeName); err == nil || refusalOf(t, err).Reason != ReasonUndeclared {
		t.Fatalf("undeclared capability refusal = %v, want undeclared", err)
	}
	if err := registry.InstallTypeCapability(typeName, model.DecodeOnly); err != nil {
		t.Fatalf("install decode-only capability: %v", err)
	}
	if err := registry.InstallTypeCapability(typeName, model.DecodeOnly); err == nil || refusalOf(t, err).Reason != ReasonDuplicateName {
		t.Fatalf("duplicate capability refusal = %v, want duplicate name", err)
	}
	capability, err := registry.TypeCapability(Site{Path: "test.capability"}, typeName)
	if err != nil || !capability.DecodeOnly() {
		t.Fatalf("resolved capability = %v/%v, want DecodeOnly", capability, err)
	}
	owner, err := registry.Owner(Site{Path: "test.owner"}, heap)
	if err != nil {
		t.Fatalf("resolve owner: %v", err)
	}
	schemaID, ok := model.IssueSchemaID(owner, issue(t, "capability/schema"))
	if !ok {
		t.Fatal("issue schema")
	}
	declaration := registry.Declaration(schemaID)
	if len(declaration.TypeCapabilities) != 1 || !declaration.TypeCapabilities[0].Equal(capability) {
		t.Fatalf("declaration capabilities = %+v, want one exact policy", declaration.TypeCapabilities)
	}
	compiled, err := Compile(declaration)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if capabilities := compiled.TypeCapabilities(); len(capabilities) != 1 || !capabilities[0].Equal(capability) {
		t.Fatalf("compiled capabilities = %+v, want one exact policy", capabilities)
	}
}

// TestUnknownNameRefusesWithTheRuleAndSiteNamed states that a reference no
// owner installed is refused where it is authored, not as an anonymous
// compile failure and never by minting an identity for it.
func TestUnknownNameRefusesWithTheRuleAndSiteNamed(t *testing.T) {
	registry := NewRegistry()
	heap := axisEntry("heap")
	if err := registry.InstallOwner(heap, issue(t, "axis/heap")); err != nil {
		t.Fatalf("install owner: %v", err)
	}
	missing := NewName(heap, "heap/absent-relation")
	site := Site{Rule: "heap-formal-freeze", Path: "program.joins[2].relation"}

	_, err := registry.Relation(site, missing)
	if err == nil {
		t.Fatal("an uninstalled relation name resolved")
	}
	refusal := refusalOf(t, err)
	if refusal.Reason != ReasonUnknown {
		t.Fatalf("reason = %v, want unknown", refusal.Reason)
	}
	if refusal.Site.Rule != "heap-formal-freeze" || refusal.Site.Path != "program.joins[2].relation" {
		t.Fatalf("refusal site = %v, want the authoring rule and declaration path", refusal.Site)
	}
	if refusal.Name != missing {
		t.Fatalf("refusal name = %v, want the unresolved reference", refusal.Name)
	}
}

// TestSignatureRefusesUntilOwnerInstallsOperation states that an otherwise
// owner-issued semantic identity cannot enter the declaration merely because
// a caller can construct it. The owner must publish the operation through the
// one registry before its signature is accepted.
func TestSignatureRefusesUntilOwnerInstallsOperation(t *testing.T) {
	registry := NewRegistry()
	heap := axisEntry("heap")
	if err := registry.InstallOwner(heap, issue(t, "axis/heap")); err != nil {
		t.Fatalf("install owner: %v", err)
	}
	owner, err := registry.Owner(Site{Path: "test.operation-owner"}, heap)
	if err != nil {
		t.Fatalf("resolve owner: %v", err)
	}
	operation, operationOK := model.IssueOperationID(owner, issue(t, "operation/uninstalled"))
	if !operationOK {
		t.Fatal("issue owner operation")
	}
	semantic, semanticOK := signature.Seal(signature.Spec{
		Identity: signature.Identity{Operation: operation, Version: 1},
	})
	if !semanticOK {
		t.Fatal("seal semantic signature")
	}
	name := NewName(heap, "operation/uninstalled")
	err = registry.InstallSignature(name, semantic)
	if err == nil {
		t.Fatal("signature accepted without an installed owner operation")
	}
	refusal := refusalOf(t, err)
	if refusal.Kind != KindOperation || refusal.Reason != ReasonUnknown || refusal.Name != name {
		t.Fatalf("refusal = %+v, want unknown operation %v", refusal, name)
	}
}

// TestTwoDeclarationsOfOneIdentityRefuse states the canonical registry is one
// registry: a second name may not claim a token another entry already holds,
// and one name may not be installed twice.
func TestTwoDeclarationsOfOneIdentityRefuse(t *testing.T) {
	registry := NewRegistry()
	heap := axisEntry("heap")
	if err := registry.InstallOwner(heap, issue(t, "axis/heap")); err != nil {
		t.Fatalf("install owner: %v", err)
	}
	scope := EntryName(schema.SurfaceKindAxis, "heap")
	if err := registry.InstallScope(scope, issue(t, "scope/heap"), region.True()); err != nil {
		t.Fatalf("install scope: %v", err)
	}
	token := issue(t, "relation/heap/routes")
	first := NewName(heap, "heap/routes")
	if err := registry.InstallRelation(first, token, scope); err != nil {
		t.Fatalf("install first relation: %v", err)
	}

	second := NewName(heap, "heap/other-routes")
	err := registry.InstallRelation(second, token, scope)
	if err == nil {
		t.Fatal("a second relation claimed an identity another relation holds")
	}
	if reason := refusalOf(t, err).Reason; reason != ReasonDuplicateIdentity {
		t.Fatalf("reason = %v, want duplicate identity", reason)
	}

	err = registry.InstallRelation(first, issue(t, "relation/heap/routes-again"), scope)
	if err == nil {
		t.Fatal("one relation name was installed twice")
	}
	if reason := refusalOf(t, err).Reason; reason != ReasonDuplicateName {
		t.Fatalf("reason = %v, want duplicate name", reason)
	}
}

// TestAnIdentityMayNotBeSharedAcrossKinds states that the one-token-one-entry
// law spans the whole registry, so a column cannot be issued the token its
// relation already holds.
func TestAnIdentityMayNotBeSharedAcrossKinds(t *testing.T) {
	registry := NewRegistry()
	heap := axisEntry("heap")
	if err := registry.InstallOwner(heap, issue(t, "axis/heap")); err != nil {
		t.Fatalf("install owner: %v", err)
	}
	scope := EntryName(schema.SurfaceKindAxis, "heap")
	if err := registry.InstallScope(scope, issue(t, "scope/heap"), region.True()); err != nil {
		t.Fatalf("install scope: %v", err)
	}
	if err := registry.InstallType(NewName(heap, "type/route"), issue(t, "type/route")); err != nil {
		t.Fatalf("install type: %v", err)
	}
	relation := NewName(heap, "heap/routes")
	token := issue(t, "relation/heap/routes")
	if err := registry.InstallRelation(relation, token, scope); err != nil {
		t.Fatalf("install relation: %v", err)
	}
	err := registry.InstallColumn(NewName(heap, "heap/route-key"), token, relation, NewName(heap, "type/route"))
	if err == nil {
		t.Fatal("a column claimed the identity its relation holds")
	}
	if reason := refusalOf(t, err).Reason; reason != ReasonDuplicateIdentity {
		t.Fatalf("reason = %v, want duplicate identity", reason)
	}
}

// TestAddressAndPublicationKeyAreOwnerStatements states that the columns and
// keys a join and a publication pair against are declared by the relation's
// owner and refuse until they are.
func TestAddressAndPublicationKeyAreOwnerStatements(t *testing.T) {
	registry := NewRegistry()
	heap := axisEntry("heap")
	if err := registry.InstallOwner(heap, issue(t, "axis/heap")); err != nil {
		t.Fatalf("install owner: %v", err)
	}
	scope := EntryName(schema.SurfaceKindAxis, "heap")
	if err := registry.InstallScope(scope, issue(t, "scope/heap"), region.True()); err != nil {
		t.Fatalf("install scope: %v", err)
	}
	if err := registry.InstallType(NewName(heap, "type/route"), issue(t, "type/route")); err != nil {
		t.Fatalf("install type: %v", err)
	}
	relation := NewName(heap, "heap/routes")
	if err := registry.InstallRelation(relation, issue(t, "relation/heap/routes"), scope); err != nil {
		t.Fatalf("install relation: %v", err)
	}
	column := NewName(heap, "heap/route-key")
	if err := registry.InstallColumn(column, issue(t, "column/heap/route-key"), relation, NewName(heap, "type/route")); err != nil {
		t.Fatalf("install column: %v", err)
	}

	site := Site{Rule: "heap-formal-freeze", Path: "program.joins[0].sources[0]"}
	if _, err := registry.Addressed(site, relation, CoordinateAddress); err == nil {
		t.Fatal("a relation with no declared address column was joined onto")
	} else if reason := refusalOf(t, err).Reason; reason != ReasonUndeclared {
		t.Fatalf("reason = %v, want undeclared", reason)
	}
	if err := registry.DeclareCoordinate(relation, CoordinateAddress, column); err != nil {
		t.Fatalf("declare address: %v", err)
	}
	address, err := registry.Addressed(site, relation, CoordinateAddress)
	if err != nil {
		t.Fatalf("resolve address: %v", err)
	}
	if address.Relation() != mustRelation(t, registry, relation) {
		t.Fatal("the declared address column is not owned by its relation")
	}

	if _, err := registry.PublicationKey(site, column); err == nil {
		t.Fatal("a relation with no declared publication key was published to")
	} else if reason := refusalOf(t, err).Reason; reason != ReasonUndeclared {
		t.Fatalf("reason = %v, want undeclared", reason)
	}
	key := NewName(heap, "heap/route-key-vector")
	if err := registry.InstallKey(key, issue(t, "key/heap/routes"), relation, column); err != nil {
		t.Fatalf("install key: %v", err)
	}
	if err := registry.DeclarePublicationKey(relation, key); err != nil {
		t.Fatalf("declare publication key: %v", err)
	}
	if _, err := registry.PublicationKey(site, column); err != nil {
		t.Fatalf("resolve publication key: %v", err)
	}
}

func mustRelation(t *testing.T, registry *Registry, name Name) model.RelationID {
	t.Helper()
	id, err := registry.Relation(Site{Path: "test"}, name)
	if err != nil {
		t.Fatalf("resolve relation: %v", err)
	}
	return id
}
