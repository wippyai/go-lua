package authority

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/schema"
)

func TestCatalogRejectsUnavailableDuplicateAndForeignDeclarations(t *testing.T) {
	base := newCatalogFixture(t)
	if catalog, ok := base.seal(t); !ok || !catalog.Available() {
		t.Fatal("valid fixture was rejected")
	}

	tests := []struct {
		name  string
		build func(*catalogFixture)
	}{
		{name: "unavailable owner", build: func(value *catalogFixture) { value.owner = Owner{} }},
		{name: "duplicate relation name", build: func(value *catalogFixture) {
			value.relations = append(value.relations, RelationSpec{Name: "orders", Token: fixtureID(t, "relation/duplicate"), Scope: "order-scope"})
		}},
		{name: "duplicate relation token", build: func(value *catalogFixture) {
			value.relations[0].Token = value.owner.Token
		}},
		{name: "duplicate column name", build: func(value *catalogFixture) {
			value.columns[1].Name = value.columns[0].Name
		}},
		{name: "foreign column relation", build: func(value *catalogFixture) {
			value.columns[0].Relation = "missing-relation"
		}},
		{name: "unavailable column type", build: func(value *catalogFixture) {
			value.columns[0].Type = model.TypeID{}
		}},
		{name: "duplicate key vector member", build: func(value *catalogFixture) {
			value.keys[0].Columns = []schema.Key{"address", "address"}
		}},
		{name: "foreign key column", build: func(value *catalogFixture) {
			value.keys[0].Columns = []schema.Key{"missing-column"}
		}},
		{name: "duplicate key name", build: func(value *catalogFixture) {
			value.keys = append(value.keys, KeySpec{Name: "primary", Token: fixtureID(t, "key/duplicate"), Relation: "orders", Columns: []schema.Key{"address"}})
		}},
		{name: "foreign scope dimension", build: func(value *catalogFixture) {
			value.scopes[0].Dimensions = []schema.Key{"missing-column"}
		}},
		{name: "duplicate address coordinate", build: func(value *catalogFixture) {
			value.relations[0].Addressing = append(value.relations[0].Addressing, Address{Coordinate: CoordinateAddress, Column: "value"})
		}},
		{name: "duplicate address column", build: func(value *catalogFixture) {
			value.relations[0].Addressing = append(value.relations[0].Addressing, Address{Coordinate: CoordinateParent, Column: "address"})
		}},
		{name: "foreign publication key", build: func(value *catalogFixture) {
			value.relations[0].Publication = "missing-key"
		}},
		{name: "foreign denominator key", build: func(value *catalogFixture) {
			value.denominators[0].Key = "missing-key"
		}},
		{name: "foreign denominator relation", build: func(value *catalogFixture) {
			value.denominators[0].Relation = "missing-relation"
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value := newCatalogFixture(t)
			test.build(&value)
			if catalog, ok := value.seal(t); ok || catalog.Available() {
				t.Fatalf("invalid declaration accepted: %+v", catalog)
			}
		})
	}
}

func TestCatalogEmptyAttachmentIsValid(t *testing.T) {
	fixture := newCatalogFixture(t)
	declaration, ok := NewDeclaration(nil, nil, nil, nil, nil)
	if !ok {
		t.Fatal("empty declaration unavailable")
	}
	catalog, ok := declaration.Seal(fixture.owner)
	if !ok || !catalog.Available() {
		t.Fatal("empty attachment rejected")
	}
	if catalog.RelationCount() != 0 || catalog.ColumnCount() != 0 || catalog.KeyCount() != 0 || catalog.ScopeCount() != 0 || catalog.DenominatorCount() != 0 {
		t.Fatalf("empty counts = %d/%d/%d/%d/%d", catalog.RelationCount(), catalog.ColumnCount(), catalog.KeyCount(), catalog.ScopeCount(), catalog.DenominatorCount())
	}
}

func TestCoordinateVocabularyIsClosed(t *testing.T) {
	for coordinate := CoordinateAddress; coordinate <= CoordinateOccurrence; coordinate++ {
		if !coordinate.Available() || coordinate.String() == "invalid" {
			t.Fatalf("coordinate %d unavailable", coordinate)
		}
	}
	if CoordinateInvalid.Available() || (Coordinate(255)).Available() {
		t.Fatal("coordinate vocabulary accepted an unavailable value")
	}
}
