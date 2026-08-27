package member

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema/carrier"
)

func selectionCatalog(t *testing.T, selection Selection) (Catalog, bool) {
	t.Helper()
	routes := baseRelation("heap/routes")
	routes.Addressing = Addressing{Address: "heap/route-key", Tag: "heap/route-tag"}
	base, ok := NewCatalog(testAuthorities("coordinate"), []carrier.Binding{}, []Relation{routes}, []Projection{
		projectionOf("heap/routes", "heap/route-key", Key),
		projectionOf("heap/routes", "heap/route-tag", Predicate),
	}, nil, nil)
	if !ok {
		t.Fatal("build base catalog")
	}
	return base.WithSelections([]Selection{selection})
}

// TestASelectionPublishesIntoADeclaredRelationUnderADeclaredTag states the
// whole contract: a produced row lands in a relation this catalog declares and
// carries a tag that is an ordinary projection over that same relation, so a
// reading rule joins produced rows exactly as it joins enumerated ones.
func TestASelectionPublishesIntoADeclaredRelationUnderADeclaredTag(t *testing.T) {
	catalog, ok := selectionCatalog(t, Selection{
		Key: "heap/route-selection", Relation: "heap/routes", Tag: "heap/route-tag",
	})
	if !ok || !catalog.Available() {
		t.Fatal("a selection over declared rows was refused")
	}
	resolved, found := catalog.Selection("heap/route-selection")
	if !found || resolved.Relation != "heap/routes" || resolved.Tag != "heap/route-tag" {
		t.Fatal("the catalog does not resolve the selection it admitted")
	}
	if catalog.SelectionCount() != 1 {
		t.Fatalf("selection count = %d, want one", catalog.SelectionCount())
	}
}

// TestASelectionIntoAnUndeclaredRelationRefuses states that a produced row
// cannot land somewhere this catalog does not declare.
func TestASelectionIntoAnUndeclaredRelationRefuses(t *testing.T) {
	if _, ok := selectionCatalog(t, Selection{
		Key: "heap/route-selection", Relation: "heap/absent", Tag: "heap/route-tag",
	}); ok {
		t.Fatal("a selection published into a relation no catalog declares")
	}
}

// TestASelectionTaggedByAForeignColumnRefuses states the tag is a column of
// the relation the selection publishes into, so a reading rule cannot
// correlate a produced row through a column another relation owns.
func TestASelectionTaggedByAForeignColumnRefuses(t *testing.T) {
	directory := baseRelation("heap/directory")
	routes := baseRelation("heap/routes")
	base, ok := NewCatalog(testAuthorities("coordinate"), []carrier.Binding{}, []Relation{directory, routes}, []Projection{
		projectionOf("heap/directory", "heap/directory-tag", Predicate),
		projectionOf("heap/routes", "heap/route-tag", Predicate),
	}, nil, nil)
	if !ok {
		t.Fatal("build base catalog")
	}
	if _, ok := base.WithSelections([]Selection{{
		Key: "heap/route-selection", Relation: "heap/routes", Tag: "heap/directory-tag",
	}}); ok {
		t.Fatal("a selection stamped its rows with a column another relation owns")
	}
}

// TestASelectionAndItsRelationNameOneTag states that where a relation also
// publishes its tag coordinate the two agree: one column, one authority.
func TestASelectionAndItsRelationNameOneTag(t *testing.T) {
	if _, ok := selectionCatalog(t, Selection{
		Key: "heap/route-selection", Relation: "heap/routes", Tag: "heap/route-key",
	}); ok {
		t.Fatal("a selection stamped a tag its relation contradicts")
	}
}

// TestAnIncompleteSelectionRefuses states the row is whole or absent: an
// operation that names no relation or no tag publishes rows nothing can read.
func TestAnIncompleteSelectionRefuses(t *testing.T) {
	for label, selection := range map[string]Selection{
		"no key":      {Relation: "heap/routes", Tag: "heap/route-tag"},
		"no relation": {Key: "heap/route-selection", Tag: "heap/route-tag"},
		"no tag":      {Key: "heap/route-selection", Relation: "heap/routes"},
	} {
		if _, ok := selectionCatalog(t, selection); ok {
			t.Fatalf("a selection with %s was admitted", label)
		}
	}
}

// TestSelectionsAreContentAndSurviveTheClone states that the operations an
// axis publishes rows through are part of its declaration, and that an axis
// publishing none keeps the exact stream it had.
func TestSelectionsAreContentAndSurviveTheClone(t *testing.T) {
	catalog, ok := selectionCatalog(t, Selection{
		Key: "heap/route-selection", Relation: "heap/routes", Tag: "heap/route-tag",
	})
	if !ok {
		t.Fatal("build catalog")
	}
	routes := baseRelation("heap/routes")
	routes.Addressing = Addressing{Address: "heap/route-key", Tag: "heap/route-tag"}
	silent, ok := NewCatalog(testAuthorities("coordinate"), []carrier.Binding{}, []Relation{routes}, []Projection{
		projectionOf("heap/routes", "heap/route-key", Key),
		projectionOf("heap/routes", "heap/route-tag", Predicate),
	}, nil, nil)
	if !ok {
		t.Fatal("build silent catalog")
	}
	if catalogStream(t, silent) == catalogStream(t, catalog) {
		t.Fatal("declaring a selection left the canonical stream unchanged")
	}
	if catalog.Clone().SelectionCount() != 1 {
		t.Fatal("cloning the catalog dropped the selections")
	}
	if catalog.MemberCount() != silent.MemberCount()+1 {
		t.Fatal("a selection is not counted as a member of its axis")
	}
}
