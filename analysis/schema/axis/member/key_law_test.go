package member

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
)

// keyedCatalog builds one two-relation catalog whose "heap/routes" relation
// declares the supplied key vectors over its own two declared columns.
func keyedCatalog(keys ...KeyVector) (Catalog, bool) {
	directory := baseRelation("heap/directory")
	routes := baseRelation("heap/routes")
	routes.Keys = keys
	return NewCatalog(
		[]Relation{directory, routes},
		[]Projection{
			projectionOf("heap/routes", "heap/route-key", Key),
			projectionOf("heap/routes", "heap/route-tag", Predicate),
		}, nil, nil)
}

// TestAKeyVectorIsAColumnVectorOfItsOwnRelation states the whole point of the
// declaration: the columns a row is published under are columns of the
// relation that publishes it, so a reader resolves the vector by name instead
// of reconstructing it from a role.
func TestAKeyVectorIsAColumnVectorOfItsOwnRelation(t *testing.T) {
	catalog, ok := keyedCatalog(KeyVector{
		Name:    "heap/routes/publication",
		Columns: []schema.Key{"heap/route-key", "heap/route-tag"},
	})
	if !ok || !catalog.Available() {
		t.Fatal("a relation keyed by its own declared columns was refused")
	}
}

// TestAForeignKeyVectorColumnRefuses states that a relation may not publish
// its rows under a column another relation owns. That is the defect a key
// named by role rather than by column would hide.
func TestAForeignKeyVectorColumnRefuses(t *testing.T) {
	directory := baseRelation("heap/directory")
	routes := baseRelation("heap/routes")
	routes.Keys = []KeyVector{{
		Name:    "heap/routes/publication",
		Columns: []schema.Key{"heap/directory-key"},
	}}
	catalog, ok := NewCatalog(
		[]Relation{directory, routes},
		[]Projection{
			projectionOf("heap/directory", "heap/directory-key", Key),
			projectionOf("heap/routes", "heap/route-key", Key),
		}, nil, nil)
	if ok || catalog.Available() {
		t.Fatal("a relation was keyed through a column another relation owns")
	}
}

// TestAKeyVectorOrderIsItsContent states that the vector is a sequence. Two
// keys over the same columns in a different order address rows differently, so
// they are different declarations and cannot share one canonical stream.
func TestAKeyVectorOrderIsItsContent(t *testing.T) {
	forward, ok := keyedCatalog(KeyVector{
		Name:    "heap/routes/publication",
		Columns: []schema.Key{"heap/route-key", "heap/route-tag"},
	})
	if !ok {
		t.Fatal("the forward key was refused")
	}
	reverse, ok := keyedCatalog(KeyVector{
		Name:    "heap/routes/publication",
		Columns: []schema.Key{"heap/route-tag", "heap/route-key"},
	})
	if !ok {
		t.Fatal("the reversed key was refused")
	}
	if catalogStream(t, forward) == catalogStream(t, reverse) {
		t.Fatal("two key vectors differing only in column order share one canonical stream")
	}
}

// TestAKeyVectorNameJoinsTheOneMemberNamespace states that a key is a named
// member like any other: it cannot take the name of a relation, a projection,
// or a second key, because a consumer resolves all of them from one namespace.
func TestAKeyVectorNameJoinsTheOneMemberNamespace(t *testing.T) {
	if catalog, ok := keyedCatalog(KeyVector{
		Name: "heap/routes", Columns: []schema.Key{"heap/route-key"},
	}); ok || catalog.Available() {
		t.Fatal("a key took the name of a relation")
	}
	if catalog, ok := keyedCatalog(KeyVector{
		Name: "heap/route-key", Columns: []schema.Key{"heap/route-key"},
	}); ok || catalog.Available() {
		t.Fatal("a key took the name of a projection")
	}
	if catalog, ok := keyedCatalog(
		KeyVector{Name: "heap/routes/publication", Columns: []schema.Key{"heap/route-key"}},
		KeyVector{Name: "heap/routes/publication", Columns: []schema.Key{"heap/route-tag"}},
	); ok || catalog.Available() {
		t.Fatal("one relation declared two keys under one name")
	}
}

// TestARelationWithoutAKeyVectorEmitsTheStreamItAlwaysHad states that the
// declaration is a tagged trailing extension. A catalog that names no key
// emits exactly the stream it emitted before keys could be named, so adding
// the statement remints no declaration that does not use it.
func TestARelationWithoutAKeyVectorEmitsTheStreamItAlwaysHad(t *testing.T) {
	silent, ok := keyedCatalog()
	if !ok {
		t.Fatal("a catalog naming no key was refused")
	}
	keyed, ok := keyedCatalog(KeyVector{
		Name: "heap/routes/publication", Columns: []schema.Key{"heap/route-key"},
	})
	if !ok {
		t.Fatal("a catalog naming one key was refused")
	}
	if catalogStream(t, silent) == catalogStream(t, keyed) {
		t.Fatal("a declared key left the canonical stream unchanged")
	}
	var undeclared KeyVector
	if undeclared.Declared() || undeclared.Available() {
		t.Fatal("the zero key vector reports itself declared")
	}
}

// TestAMalformedKeyVectorRefuses states that a key addresses something or it
// is not a key: an empty vector addresses nothing, and a column repeated
// within one key would give a row two positions in its own address.
func TestAMalformedKeyVectorRefuses(t *testing.T) {
	if catalog, ok := keyedCatalog(KeyVector{Name: "heap/routes/publication"}); ok || catalog.Available() {
		t.Fatal("a key with no column was admitted")
	}
	if catalog, ok := keyedCatalog(KeyVector{
		Name: "heap/routes/publication", Columns: []schema.Key{"heap/route-key", "heap/route-key"},
	}); ok || catalog.Available() {
		t.Fatal("a key repeated one column")
	}
	if catalog, ok := keyedCatalog(KeyVector{
		Name: "", Columns: []schema.Key{"heap/route-key"},
	}); ok || catalog.Available() {
		t.Fatal("an unnamed key was admitted")
	}
	if catalog, ok := keyedCatalog(KeyVector{
		Name: "heap/routes/publication", Columns: []schema.Key{""},
	}); ok || catalog.Available() {
		t.Fatal("a key named an unavailable column")
	}
}
