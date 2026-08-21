package programschema_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	programcatalog "github.com/wippyai/go-lua/analysis/schema/program/catalog"
	programpublication "github.com/wippyai/go-lua/analysis/schema/program/publication"
)

func entryBodyLawID(t *testing.T, label string) identity.ContentID {
	t.Helper()
	id, ok := identity.DeriveContentID("schema/program/entry-body-law/"+label, nil)
	if !ok {
		t.Fatalf("derive %s", label)
	}
	return id
}

func entryBodyLawRow(t *testing.T, label string, callable bool) programschema.Body {
	t.Helper()
	id := entryBodyLawID(t, label)
	function, formal := identity.ContentID{}, identity.ContentID{}
	if callable {
		function, formal = entryBodyLawID(t, label+"/function"), entryBodyLawID(t, label+"/formal")
	}
	row, ok := programschema.NewBody(
		id,
		entryBodyLawID(t, label+"/context"),
		entryBodyLawID(t, label+"/entry"),
		function,
		formal,
		0, 1,
		0, 0,
		0, 1,
		callable,
	)
	if !ok {
		t.Fatalf("build %s", label)
	}
	return row
}

func entryBodyLawProgram(t *testing.T, entryID identity.ContentID, bodies ...programschema.Body) programschema.Program {
	t.Helper()
	schemaID := entryBodyLawID(t, "schema")
	catalog, ok := programcatalog.CatalogID(schemaID)
	if !ok {
		t.Fatal("derive catalog")
	}
	store, ok := identity.IssueStore()
	if !ok {
		t.Fatal("issue store")
	}
	frozen, ok := (programpublication.Publication{Bodies: bodies}).Seal(catalog, store)
	if !ok {
		t.Fatal("seal body publication")
	}
	return programschema.Program{
		Frozen:      frozen,
		ArtifactID:  entryBodyLawID(t, "artifact"),
		ProgramID:   entryBodyLawID(t, "program"),
		SchemaID:    schemaID,
		EntryBodyID: entryID,
	}
}

func TestProgramEntryBodyUsesOwnerIssuedRootRelation(t *testing.T) {
	nested := entryBodyLawRow(t, "nested", false)
	function := entryBodyLawRow(t, "function", true)
	entry := entryBodyLawRow(t, "entry", false)
	program := entryBodyLawProgram(t, entry.ID(), nested, function, entry)

	got, ok := program.EntryBody()
	if !ok || got.ID() != entry.ID() || got.Callable() {
		t.Fatalf("EntryBody = %v/%v, want owner-issued root body %v", got.ID(), ok, entry.ID())
	}
}

func TestProgramEntryBodyFailsClosedWithoutOwnerRelation(t *testing.T) {
	first := entryBodyLawRow(t, "first", false)
	second := entryBodyLawRow(t, "second", false)
	callable := entryBodyLawRow(t, "callable", true)
	tests := []struct {
		name    string
		entryID identity.ContentID
		bodies  []programschema.Body
	}{
		{name: "missing", bodies: []programschema.Body{first, second}},
		{name: "unknown", entryID: entryBodyLawID(t, "unknown"), bodies: []programschema.Body{first, second}},
		{name: "callable", entryID: callable.ID(), bodies: []programschema.Body{first, callable, second}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got, ok := entryBodyLawProgram(t, test.entryID, test.bodies...).EntryBody(); ok || got.Available() {
				t.Fatalf("EntryBody = %v/%v, want fail-closed absence", got.ID(), ok)
			}
		})
	}
}
