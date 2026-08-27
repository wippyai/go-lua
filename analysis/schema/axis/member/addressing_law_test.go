package member

import (
	"bytes"
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/carrier"
	"github.com/wippyai/go-lua/internal/framing"
)

func baseRelation(key schema.Key) Relation {
	return Relation{
		Key:     key,
		Subject: "coordinate",
		CandidateProvider: AxisRelationCandidate(RelationRef{
			Axis: axisEntry("heap"), Member: "heap/directory",
		}),
	}
}

func projectionOf(relation schema.Key, key schema.Key, role Role) Projection {
	return Projection{
		Key: key, Relation: relation, Role: role, Result: "coordinate",
		CandidateProvider: AxisRelationCandidate(RelationRef{
			Axis: axisEntry("heap"), Member: "heap/directory",
		}),
	}
}

// TestAnAddressingCoordinateIsAColumnOfItsOwnRelation states the whole point
// of the declaration: a coordinate a relation is addressed by is an ordinary
// projection over that same relation, so a reader resolves it by name.
func TestAnAddressingCoordinateIsAColumnOfItsOwnRelation(t *testing.T) {
	directory := baseRelation("heap/directory")
	routes := baseRelation("heap/routes")
	routes.Addressing = Addressing{Address: "heap/route-key", Tag: "heap/route-tag"}
	catalog, ok := NewCatalog(
		testAuthorities("coordinate"),
		[]carrier.Binding{},
		[]Relation{directory, routes},
		[]Projection{
			projectionOf("heap/routes", "heap/route-key", Key),
			projectionOf("heap/routes", "heap/route-tag", Predicate),
		}, nil, nil)
	if !ok || !catalog.Available() {
		t.Fatal("a relation addressed by its own declared columns was refused")
	}
	if got := routes.Addressing.Columns(); len(got) != 2 {
		t.Fatalf("declared addressing columns = %d, want the address and the tag", len(got))
	}
}

// TestAForeignAddressingColumnRefuses states that a relation may not be
// addressed through a column another relation owns, which is the defect
// naming a coordinate by role instead of by column would hide.
func TestAForeignAddressingColumnRefuses(t *testing.T) {
	directory := baseRelation("heap/directory")
	routes := baseRelation("heap/routes")
	routes.Addressing = Addressing{Address: "heap/directory-key"}
	catalog, ok := NewCatalog(
		testAuthorities("coordinate"),
		[]carrier.Binding{},
		[]Relation{directory, routes},
		[]Projection{projectionOf("heap/directory", "heap/directory-key", Key)},
		nil, nil)
	if ok && catalog.Available() {
		t.Fatal("a relation was addressed through a column another relation owns")
	}
}

// TestAnUndeclaredAddressingColumnRefuses states that naming a column no
// projection declares is refused rather than resolved later.
func TestAnUndeclaredAddressingColumnRefuses(t *testing.T) {
	routes := baseRelation("heap/routes")
	routes.Addressing = Addressing{Address: "heap/absent-column"}
	catalog, ok := NewCatalog(testAuthorities("coordinate"), []carrier.Binding{}, []Relation{routes}, nil, nil, nil)
	if ok && catalog.Available() {
		t.Fatal("a relation was addressed through a column no projection declares")
	}
}

// TestParentAndOrdinalColumnsStandOrFallTogether states the nesting
// biconditional over columns, matching the one the ordinal carrier already
// answers to: a parent column with no ordinal gives its members no address,
// and an ordinal with no parent keys nothing.
func TestParentAndOrdinalColumnsStandOrFallTogether(t *testing.T) {
	nested := baseRelation("heap/members")
	nested.Parent = RelationRef{Axis: axisEntry("heap"), Member: "heap/directory"}
	nested.Ordinal = "ordinal"
	nested.Addressing = Addressing{Parent: "heap/member-parent"}
	if nested.Available() {
		t.Fatal("a parent column with no ordinal column was admitted")
	}
	nested.Addressing = Addressing{Ordinal: "heap/member-ordinal"}
	if nested.Available() {
		t.Fatal("an ordinal column with no parent column was admitted")
	}
	nested.Addressing = Addressing{Parent: "heap/member-parent", Ordinal: "heap/member-ordinal"}
	if !nested.Available() {
		t.Fatal("a nested member set naming both columns was refused")
	}

	flat := baseRelation("heap/routes")
	flat.Addressing = Addressing{Parent: "heap/route-parent", Ordinal: "heap/route-ordinal"}
	if flat.Available() {
		t.Fatal("a relation that is not nested was admitted with parent columns")
	}
}

// TestOneColumnMayNotFillTwoAddressingRoles states that the coordinates stay
// distinct: a column that were both the address and the tag would make a
// selection indistinguishable from the row it selected.
func TestOneColumnMayNotFillTwoAddressingRoles(t *testing.T) {
	routes := baseRelation("heap/routes")
	routes.Addressing = Addressing{Address: "heap/route-key", Tag: "heap/route-key"}
	if routes.Available() {
		t.Fatal("one column filled two addressing roles")
	}
}

// TestARelationDeclaringNoAddressingRemainsAdmissible states that silence is
// a verdict and not a defect: a relation publishes the coordinates it has, and
// a reader that needs one it did not publish has nothing to pair against.
func TestARelationDeclaringNoAddressingRemainsAdmissible(t *testing.T) {
	routes := baseRelation("heap/routes")
	if !routes.Available() {
		t.Fatal("a relation declaring no addressing column was refused")
	}
	if routes.Addressing.Declared() {
		t.Fatal("an undeclared addressing reports itself declared")
	}
}

func catalogStream(t *testing.T, catalog Catalog) string {
	t.Helper()
	var buffer bytes.Buffer
	var writer framing.Writer
	if err := writer.Reset(&buffer, "member-addressing-law", 1); err != nil {
		t.Fatalf("reset writer: %v", err)
	}
	if err := catalog.WriteContent(&writer); err != nil {
		t.Fatalf("write catalog: %v", err)
	}
	if err := writer.Finish(); err != nil {
		t.Fatalf("finish stream: %v", err)
	}
	return buffer.String()
}

// TestAddressingEntersTheCanonicalStream states that the coordinates a
// relation is addressed by are content: two catalogs that differ only in which
// column addresses a relation are two different declarations, and a relation
// that names none is byte-identical to the declaration it had before the
// coordinates could be named.
func TestAddressingEntersTheCanonicalStream(t *testing.T) {
	projections := []Projection{
		projectionOf("heap/routes", "heap/route-key", Key),
		projectionOf("heap/routes", "heap/route-tag", Predicate),
	}
	bare := baseRelation("heap/routes")
	silent, ok := NewCatalog(testAuthorities("coordinate"), []carrier.Binding{}, []Relation{bare}, projections, nil, nil)
	if !ok {
		t.Fatal("build silent catalog")
	}
	addressed := baseRelation("heap/routes")
	addressed.Addressing = Addressing{Address: "heap/route-key"}
	first, ok := NewCatalog(testAuthorities("coordinate"), []carrier.Binding{}, []Relation{addressed}, projections, nil, nil)
	if !ok {
		t.Fatal("build addressed catalog")
	}
	tagged := baseRelation("heap/routes")
	tagged.Addressing = Addressing{Address: "heap/route-tag"}
	second, ok := NewCatalog(testAuthorities("coordinate"), []carrier.Binding{}, []Relation{tagged}, projections, nil, nil)
	if !ok {
		t.Fatal("build re-addressed catalog")
	}

	if catalogStream(t, first) == catalogStream(t, second) {
		t.Fatal("two relations addressed by different columns share one canonical stream")
	}
	if catalogStream(t, silent) == catalogStream(t, first) {
		t.Fatal("declaring an address column left the canonical stream unchanged")
	}
}

// TestAddressingSurvivesTheCatalogClone states that the immutable copy the
// catalog stores carries the coordinates, so a relation cannot be addressed at
// declaration and unaddressed once sealed.
func TestAddressingSurvivesTheCatalogClone(t *testing.T) {
	addressed := baseRelation("heap/routes")
	addressed.Addressing = Addressing{Address: "heap/route-key", Tag: "heap/route-tag"}
	catalog, ok := NewCatalog(testAuthorities("coordinate"), []carrier.Binding{}, []Relation{addressed},
		[]Projection{
			projectionOf("heap/routes", "heap/route-key", Key),
			projectionOf("heap/routes", "heap/route-tag", Predicate),
		}, nil, nil)
	if !ok {
		t.Fatal("build catalog")
	}
	stored, ok := catalog.RelationAt(0)
	if !ok {
		t.Fatal("read stored relation")
	}
	if stored.Addressing != addressed.Addressing {
		t.Fatal("the stored relation lost the coordinates it was declared with")
	}
	if stored.Addressing != catalog.Clone().Relations[0].Addressing {
		t.Fatal("cloning the catalog dropped the coordinates")
	}
}
