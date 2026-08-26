package authorityprojection

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/authority"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/schema/region"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/rule/relcompile"
)

func projectionID(t *testing.T, label string) identity.ContentID {
	t.Helper()
	value, ok := identity.DeriveContentID("wippy.analysis/schema/rule/relcompile/authorityprojection/test", []byte(label))
	if !ok {
		t.Fatalf("derive %q", label)
	}
	return value
}

func projectionRegion(t *testing.T, label string) region.Region {
	t.Helper()
	atom, ok := region.NewAtom(projectionID(t, "scope-region/"+label))
	if !ok {
		t.Fatalf("issue scope region atom %q", label)
	}
	value, ok := region.FromAtom(atom)
	if !ok {
		t.Fatalf("construct scope region %q", label)
	}
	return value
}

type projectionFixture struct {
	owner     authority.Owner
	typeOwner authority.Owner
	typeID    model.TypeID
	typeName  relcompile.Name
	catalog   authority.Catalog
}

func newProjectionFixture(t *testing.T) projectionFixture {
	t.Helper()
	ownerEntry := schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: "projection-axis"}
	typeEntry := schema.EntryReference{Surface: schema.SurfaceKindStructure, Key: "projection-types"}
	owner, ok := authority.NewOwner(ownerEntry, projectionID(t, "owner"))
	if !ok {
		t.Fatal("owner unavailable")
	}
	typeOwner, ok := authority.NewOwner(typeEntry, projectionID(t, "type-owner"))
	if !ok {
		t.Fatal("type owner unavailable")
	}
	typeID, ok := model.IssueTypeID(typeOwner.ID(), projectionID(t, "type/value"))
	if !ok {
		t.Fatal("type unavailable")
	}
	typeName := relcompile.NewName(typeEntry, "value")
	relations := []authority.RelationSpec{{
		Name: "rows", Token: projectionID(t, "relation/rows"), Scope: "scope",
		Addressing: []authority.Address{
			{Coordinate: authority.CoordinateAddress, Column: "address"},
			{Coordinate: authority.CoordinateParent, Column: "parent"},
			{Coordinate: authority.CoordinateOrdinal, Column: "ordinal"},
			{Coordinate: authority.CoordinateTag, Column: "tag"},
			{Coordinate: authority.CoordinateDestination, Column: "destination"},
			{Coordinate: authority.CoordinateOccurrence, Column: "occurrence"},
		},
		Publication: "primary",
	}}
	columns := []authority.ColumnSpec{
		{Name: "address", Token: projectionID(t, "column/address"), Relation: "rows", Type: typeID},
		{Name: "parent", Token: projectionID(t, "column/parent"), Relation: "rows", Type: typeID},
		{Name: "ordinal", Token: projectionID(t, "column/ordinal"), Relation: "rows", Type: typeID},
		{Name: "tag", Token: projectionID(t, "column/tag"), Relation: "rows", Type: typeID},
		{Name: "destination", Token: projectionID(t, "column/destination"), Relation: "rows", Type: typeID},
		{Name: "occurrence", Token: projectionID(t, "column/occurrence"), Relation: "rows", Type: typeID},
	}
	keys := []authority.KeySpec{{Name: "primary", Token: projectionID(t, "key/primary"), Relation: "rows", Columns: []schema.Key{"address"}}}
	scopes := []authority.ScopeSpec{{Name: "scope", Token: projectionID(t, "scope"), Dimensions: []schema.Key{"address"}, Region: projectionRegion(t, "scope")}}
	denominators := []authority.DenominatorSpec{{Name: "rows/all", Relation: "rows", Key: "primary"}}
	declaration, ok := authority.NewDeclaration(relations, columns, keys, scopes, denominators)
	if !ok {
		t.Fatal("declaration unavailable")
	}
	catalog, ok := declaration.Seal(owner)
	if !ok {
		t.Fatal("catalog unavailable")
	}
	return projectionFixture{owner: owner, typeOwner: typeOwner, typeID: typeID, typeName: typeName, catalog: catalog}
}

func projectionRegistry(t *testing.T, fixture projectionFixture) *relcompile.Registry {
	t.Helper()
	registry := relcompile.NewRegistry()
	if err := registry.InstallOwner(fixture.owner.Entry, fixture.owner.Token); err != nil {
		t.Fatalf("install catalog owner: %v", err)
	}
	if err := registry.InstallOwner(fixture.typeOwner.Entry, fixture.typeOwner.Token); err != nil {
		t.Fatalf("install type owner: %v", err)
	}
	if err := registry.InstallType(fixture.typeName, fixture.typeID.Content()); err != nil {
		t.Fatalf("install type: %v", err)
	}
	return registry
}

func projectionResolver(fixture projectionFixture) TypeNameResolver {
	return func(typeID model.TypeID) (relcompile.Name, bool) {
		if typeID != fixture.typeID {
			return relcompile.Name{}, false
		}
		return fixture.typeName, true
	}
}

func assertRefusal(t *testing.T, err error, reason relcompile.ReasonKind) {
	t.Helper()
	refusal, ok := err.(relcompile.Refusal)
	if !ok || refusal.Reason != reason {
		t.Fatalf("error = %T %v, want refusal reason %s", err, err, reason)
	}
}

func TestProjectCatalogInstallsExactOwnerFencedDeclaration(t *testing.T) {
	fixture := newProjectionFixture(t)
	registry := projectionRegistry(t, fixture)
	if err := Project(registry, fixture.catalog, projectionResolver(fixture)); err != nil {
		t.Fatalf("project: %v", err)
	}

	ownerID, err := registry.Owner(relcompile.Site{Path: "test"}, fixture.owner.Entry)
	if err != nil || ownerID != fixture.catalog.OwnerID() {
		t.Fatalf("owner = %v/%v, want %v", ownerID, err, fixture.catalog.OwnerID())
	}
	relation, _ := fixture.catalog.RelationAt(0)
	column, _ := fixture.catalog.ColumnAt(0)
	key, _ := fixture.catalog.KeyAt(0)
	scope, _ := fixture.catalog.ScopeAt(0)
	denominator, _ := fixture.catalog.DenominatorAt(0)
	ownerName := fixture.owner.Entry
	if got, err := registry.Relation(relcompile.Site{Path: "test"}, relcompile.NewName(ownerName, relation.Name())); err != nil || got != relation.ID() {
		t.Fatalf("relation = %v/%v, want %v", got, err, relation.ID())
	}
	if got, err := registry.Column(relcompile.Site{Path: "test"}, relcompile.NewName(ownerName, column.Name())); err != nil || got != column.ID() {
		t.Fatalf("column = %v/%v, want %v", got, err, column.ID())
	}
	if got, err := registry.Key(relcompile.Site{Path: "test"}, relcompile.NewName(ownerName, key.Name())); err != nil || got != key.ID() {
		t.Fatalf("key = %v/%v, want %v", got, err, key.ID())
	}
	if got, err := registry.Scope(relcompile.Site{Path: "test"}, relcompile.NewName(ownerName, scope.Name())); err != nil || got != scope.ID() {
		t.Fatalf("scope = %v/%v, want %v", got, err, scope.ID())
	}
	if got, err := registry.Denominator(relcompile.Site{Path: "test"}, relcompile.NewName(ownerName, denominator.Name())); err != nil || got != denominator.Reference() {
		t.Fatalf("denominator = %v/%v, want %v", got, err, denominator.Reference())
	}
	if got, err := registry.Addressed(relcompile.Site{Path: "test"}, relcompile.NewName(ownerName, relation.Name()), relcompile.CoordinateAddress); err != nil || got != column.ID() {
		t.Fatalf("address = %v/%v, want %v", got, err, column.ID())
	}
	if got, err := registry.RelationPublicationKey(relcompile.Site{Path: "test"}, relcompile.NewName(ownerName, relation.Name())); err != nil || got != key.ID() {
		t.Fatalf("publication = %v/%v, want %v", got, err, key.ID())
	}

	declaration := registry.Declaration(model.SchemaID{})
	if len(declaration.Relations) != 1 || declaration.Relations[0].ID() != relation.ID() {
		t.Fatalf("relation declaration = %+v", declaration.Relations)
	}
	if len(declaration.Scopes) != 1 || len(declaration.Scopes[0].Dimensions()) != 1 || declaration.Scopes[0].Dimensions()[0] != column.ID() {
		t.Fatalf("scope dimensions = %+v", declaration.Scopes)
	}
}

func TestProjectRequiresTheCatalogOwnerAlreadyInstalled(t *testing.T) {
	fixture := newProjectionFixture(t)
	registry := relcompile.NewRegistry()
	err := Project(registry, fixture.catalog, projectionResolver(fixture))
	assertRefusal(t, err, relcompile.ReasonUnknown)
}

func TestProjectRejectsForeignInstalledOwner(t *testing.T) {
	fixture := newProjectionFixture(t)
	registry := relcompile.NewRegistry()
	foreignToken := projectionID(t, "foreign-owner")
	if err := registry.InstallOwner(fixture.owner.Entry, foreignToken); err != nil {
		t.Fatalf("install foreign owner: %v", err)
	}
	err := Project(registry, fixture.catalog, projectionResolver(fixture))
	assertRefusal(t, err, relcompile.ReasonForeign)
}

func TestProjectRejectsMissingAndWrongTypeMappings(t *testing.T) {
	fixture := newProjectionFixture(t)
	registry := projectionRegistry(t, fixture)
	err := Project(registry, fixture.catalog, func(model.TypeID) (relcompile.Name, bool) { return relcompile.Name{}, false })
	assertRefusal(t, err, relcompile.ReasonUnknown)

	wrongEntry := schema.EntryReference{Surface: schema.SurfaceKindStructure, Key: "projection-types-wrong"}
	wrongOwner, ok := authority.NewOwner(wrongEntry, projectionID(t, "wrong-type-owner"))
	if !ok {
		t.Fatal("wrong owner unavailable")
	}
	if err := registry.InstallOwner(wrongOwner.Entry, wrongOwner.Token); err != nil {
		t.Fatalf("install wrong owner: %v", err)
	}
	wrongID, ok := model.IssueTypeID(wrongOwner.ID(), projectionID(t, "wrong-type"))
	if !ok {
		t.Fatal("wrong type unavailable")
	}
	wrongName := relcompile.NewName(wrongEntry, "value")
	if err := registry.InstallType(wrongName, wrongID.Content()); err != nil {
		t.Fatalf("install wrong type: %v", err)
	}
	err = Project(registry, fixture.catalog, func(model.TypeID) (relcompile.Name, bool) { return wrongName, true })
	assertRefusal(t, err, relcompile.ReasonForeign)
}

func TestProjectRejectsDuplicateAndUnavailableCatalog(t *testing.T) {
	fixture := newProjectionFixture(t)
	registry := projectionRegistry(t, fixture)
	resolver := projectionResolver(fixture)
	if err := Project(registry, fixture.catalog, resolver); err != nil {
		t.Fatalf("first project: %v", err)
	}
	assertRefusal(t, Project(registry, fixture.catalog, resolver), relcompile.ReasonDuplicateName)
	assertRefusal(t, Project(relcompile.NewRegistry(), authority.Catalog{}, resolver), relcompile.ReasonUnavailable)
}

func TestCoordinateProjectionIsClosedAndMalformedValuesRefuse(t *testing.T) {
	for coordinate := authority.CoordinateAddress; coordinate <= authority.CoordinateOccurrence; coordinate++ {
		mapped, ok := mapCoordinate(coordinate)
		if !ok || mapped == relcompile.CoordinateInvalid {
			t.Fatalf("coordinate %d did not map", coordinate)
		}
	}
	if mapped, ok := mapCoordinate(authority.CoordinateInvalid); ok || mapped != relcompile.CoordinateInvalid {
		t.Fatalf("invalid coordinate mapped to %v/%t", mapped, ok)
	}
	if mapped, ok := mapCoordinate(authority.Coordinate(255)); ok || mapped != relcompile.CoordinateInvalid {
		t.Fatalf("unknown coordinate mapped to %v/%t", mapped, ok)
	}
}
