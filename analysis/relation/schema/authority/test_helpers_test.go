package authority

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/schema/region"
	"github.com/wippyai/go-lua/analysis/schema"
)

type catalogFixture struct {
	owner        Owner
	relations    []RelationSpec
	columns      []ColumnSpec
	keys         []KeySpec
	scopes       []ScopeSpec
	denominators []DenominatorSpec
	typeID       model.TypeID
}

func fixtureID(t *testing.T, label string) identity.ContentID {
	t.Helper()
	value, ok := identity.DeriveContentID("wippy.analysis/relation/schema/authority/test", []byte(label))
	if !ok {
		t.Fatalf("derive %q", label)
	}
	return value
}

func fixtureRegion(t *testing.T, label string) region.Region {
	t.Helper()
	atom, ok := region.NewAtom(fixtureID(t, "scope-region/"+label))
	if !ok {
		t.Fatalf("issue scope region atom %q", label)
	}
	value, ok := region.FromAtom(atom)
	if !ok {
		t.Fatalf("construct scope region %q", label)
	}
	return value
}

func newCatalogFixture(t *testing.T) catalogFixture {
	t.Helper()
	owner, ok := NewOwner(
		schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: "fixture-axis"},
		fixtureID(t, "owner"),
	)
	if !ok {
		t.Fatal("fixture owner unavailable")
	}
	foreignOwner, ok := model.IssueOwnerID(fixtureID(t, "foreign-owner"))
	if !ok {
		t.Fatal("fixture foreign owner unavailable")
	}
	typeID, ok := model.IssueTypeID(foreignOwner, fixtureID(t, "foreign-type"))
	if !ok {
		t.Fatal("fixture type unavailable")
	}
	return catalogFixture{
		owner: owner,
		relations: []RelationSpec{{
			Name: "orders", Token: fixtureID(t, "relation"), Scope: "order-scope",
			Addressing:  []Address{{Coordinate: CoordinateAddress, Column: "address"}},
			Publication: "primary",
		}},
		columns: []ColumnSpec{
			{Name: "address", Token: fixtureID(t, "column/address"), Relation: "orders", Type: typeID},
			{Name: "value", Token: fixtureID(t, "column/value"), Relation: "orders", Type: typeID},
		},
		keys: []KeySpec{{
			Name: "primary", Token: fixtureID(t, "key/primary"), Relation: "orders", Columns: []schema.Key{"address"},
		}},
		scopes: []ScopeSpec{{
			Name: "order-scope", Token: fixtureID(t, "scope"), Dimensions: []schema.Key{"address"}, Region: fixtureRegion(t, "order-scope"),
		}},
		denominators: []DenominatorSpec{{Name: "orders/all", Relation: "orders", Key: "primary"}},
		typeID:       typeID,
	}
}

func (fixture catalogFixture) seal(t *testing.T) (Catalog, bool) {
	t.Helper()
	declaration, ok := NewDeclaration(fixture.relations, fixture.columns, fixture.keys, fixture.scopes, fixture.denominators)
	if !ok {
		return Catalog{}, false
	}
	return declaration.Seal(fixture.owner)
}

func (fixture catalogFixture) declaration(t *testing.T) (Declaration, bool) {
	t.Helper()
	return NewDeclaration(fixture.relations, fixture.columns, fixture.keys, fixture.scopes, fixture.denominators)
}
