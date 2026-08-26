package authority

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
)

func TestDeclarationDigestIsOwnerIndependentAndSealsExactOwner(t *testing.T) {
	fixture := newCatalogFixture(t)
	declaration, ok := fixture.declaration(t)
	if !ok || !declaration.Available() {
		t.Fatal("valid declaration was rejected")
	}
	otherOwner, ok := NewOwner(schema.EntryReference{Surface: schema.SurfaceKindQuery, Key: "fixture-query"}, fixtureID(t, "other-owner"))
	if !ok {
		t.Fatal("second owner unavailable")
	}
	otherDeclaration, ok := newCatalogFixture(t).declaration(t)
	if !ok || declaration.Digest() != otherDeclaration.Digest() {
		t.Fatal("same authored declaration did not have stable owner-independent digest")
	}
	first, ok := declaration.Seal(fixture.owner)
	if !ok {
		t.Fatal("declaration did not seal under first owner")
	}
	second, ok := declaration.Seal(otherOwner)
	if !ok {
		t.Fatal("declaration did not seal under second owner")
	}
	if first.Owner() != fixture.owner || second.Owner() != otherOwner || first.Digest() == second.Digest() {
		t.Fatal("sealed owner fence or owner-bound digest was not applied")
	}
}

func TestDeclarationCopiesRawSpecsAndSealRejectsCrossRowReferences(t *testing.T) {
	fixture := newCatalogFixture(t)
	declaration, ok := fixture.declaration(t)
	if !ok {
		t.Fatal("valid declaration was rejected")
	}
	wantDigest := declaration.Digest()
	fixture.relations[0].Addressing[0].Column = "value"
	fixture.keys[0].Columns[0] = "value"
	fixture.scopes[0].Dimensions[0] = "value"
	if declaration.Digest() != wantDigest {
		t.Fatal("raw input mutation changed declaration digest")
	}
	relations := declaration.Relations()
	relations[0].Addressing[0].Column = "tampered"
	keys := declaration.Keys()
	keys[0].Columns[0] = "tampered"
	scopes := declaration.Scopes()
	scopes[0].Dimensions[0] = "tampered"
	if declaration.Relations()[0].Addressing[0].Column != "address" || declaration.Keys()[0].Columns[0] != "address" || declaration.Scopes()[0].Dimensions[0] != "address" {
		t.Fatal("mutation of returned raw specs changed declaration")
	}

	bad := newCatalogFixture(t)
	bad.relations[0].Scope = "missing-scope"
	badDeclaration, ok := bad.declaration(t)
	if ok || badDeclaration.Available() {
		t.Fatal("cross-row foreign scope reference was accepted by declaration validation")
	}

	if !reflect.DeepEqual(declaration.Columns(), newCatalogFixture(t).columns) {
		t.Fatal("declaration changed its complete raw column authority")
	}
}

func TestEmptyDeclarationIsValidBeforeAndAfterOwnerAttachment(t *testing.T) {
	declaration, ok := NewDeclaration(nil, nil, nil, nil, nil)
	if !ok || !declaration.Available() {
		t.Fatal("empty declaration rejected")
	}
	fixture := newCatalogFixture(t)
	catalog, ok := declaration.Seal(fixture.owner)
	if !ok || !catalog.Available() || catalog.RelationCount() != 0 {
		t.Fatal("empty declaration did not seal as empty catalog")
	}
}
