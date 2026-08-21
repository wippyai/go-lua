package programschema_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	programcatalog "github.com/wippyai/go-lua/analysis/schema/program/catalog"
	programpublication "github.com/wippyai/go-lua/analysis/schema/program/publication"
)

func moduleID(t *testing.T, label string) identity.ContentID {
	t.Helper()
	id, ok := identity.DeriveContentID("schema/program/module-test/"+label, nil)
	if !ok {
		t.Fatalf("derive %s", label)
	}
	return id
}

func TestModuleFamiliesPublishOneCanonicalJoin(t *testing.T) {
	importID, callID, requestID := moduleID(t, "import"), moduleID(t, "call"), moduleID(t, "request")
	valueID, entryID, returnID := moduleID(t, "value"), moduleID(t, "entry"), moduleID(t, "return")
	cellID, functionID := moduleID(t, "cell"), moduleID(t, "function")
	fieldID, parentID, tableID := moduleID(t, "field"), moduleID(t, "parent"), moduleID(t, "table")

	imported, ok := programschema.NewModuleImport(importID, callID, identity.ContentID{}, 0, 1, false)
	if !ok {
		t.Fatal("module import")
	}
	request, ok := programschema.NewModuleRequest(requestID, importID, valueID, keyspace.Key(7))
	if !ok {
		t.Fatal("module request")
	}
	entry, ok := programschema.NewModuleEntry(entryID, returnID, 3, 2, 0, 1, 0, 1, 0, 1)
	if !ok {
		t.Fatal("module entry")
	}
	cell, ok := programschema.NewModuleEntryRootCell(moduleID(t, "root-cell"), entryID, cellID, 1)
	if !ok {
		t.Fatal("module root cell")
	}
	function, ok := programschema.NewModuleEntryRootFunction(moduleID(t, "root-function"), entryID, functionID, 1)
	if !ok {
		t.Fatal("module root function")
	}
	member, ok := programschema.NewModuleEntryMember(moduleID(t, "member"), fieldID, parentID, functionID, entryID, tableID, keyspace.Key(9), 1, true)
	if !ok {
		t.Fatal("module member")
	}

	schemaID := moduleID(t, "schema")
	catalog, derived := programcatalog.CatalogID(schemaID)
	store, issued := identity.IssueStore()
	if !derived || !issued {
		t.Fatal("catalog/store")
	}
	frozen, sealed := (programpublication.Publication{
		ModuleImports:            []programschema.ModuleImport{imported},
		ModuleRequests:           []programschema.ModuleRequest{request},
		ModuleEntries:            []programschema.ModuleEntry{entry},
		ModuleEntryRootCells:     []programschema.ModuleEntryRootCell{cell},
		ModuleEntryRootFunctions: []programschema.ModuleEntryRootFunction{function},
		ModuleEntryMembers:       []programschema.ModuleEntryMember{member},
	}).Seal(catalog, store)
	if !sealed {
		t.Fatal("seal module publication")
	}
	program := programschema.Program{Frozen: frozen, ArtifactID: moduleID(t, "artifact"), ProgramID: moduleID(t, "program"), SchemaID: schemaID}
	if got, held := program.ModuleRequestFor(0); !held || got.ID() != requestID || got.ImportID() != importID {
		t.Fatal("module request join")
	}
	if got, held := program.ModuleEntryRootCellFor(0, 1); !held || got.CellID() != cellID || got.EntryID() != entryID {
		t.Fatal("module root-cell join")
	}
	if got, held := program.ModuleEntryRootFunctionFor(0, 1); !held || got.FunctionID() != functionID || got.EntryID() != entryID {
		t.Fatal("module root-function join")
	}
	if got, held := program.ModuleEntryMemberFor(0, 0); !held || got.FieldID() != fieldID || got.EntryID() != entryID {
		t.Fatal("module member join")
	}
	if got, held := program.ModuleEntryForReturnOrdinal(3); !held || got.ID() != entryID {
		t.Fatal("module return ordinal join")
	}
	if width, held := entry.RootWidth(); !held || width != 2 {
		t.Fatal("module fixed root width")
	}
}

func TestModuleSparseRootLookupUsesOriginalPosition(t *testing.T) {
	entryID := moduleID(t, "sparse-entry")
	entry, ok := programschema.NewModuleEntry(entryID, moduleID(t, "sparse-return"), 2, 4, 0, 1, 0, 0, 0, 0)
	if !ok {
		t.Fatal("module sparse entry")
	}
	cell, ok := programschema.NewModuleEntryRootCell(moduleID(t, "sparse-cell-row"), entryID, moduleID(t, "sparse-cell"), 3)
	if !ok {
		t.Fatal("module sparse cell")
	}
	schemaID := moduleID(t, "sparse-schema")
	catalog, _ := programcatalog.CatalogID(schemaID)
	store, _ := identity.IssueStore()
	frozen, sealed := (programpublication.Publication{ModuleEntries: []programschema.ModuleEntry{entry}, ModuleEntryRootCells: []programschema.ModuleEntryRootCell{cell}}).Seal(catalog, store)
	if !sealed {
		t.Fatal("seal sparse module")
	}
	program := programschema.Program{Frozen: frozen, ArtifactID: moduleID(t, "sparse-artifact"), ProgramID: moduleID(t, "sparse-program"), SchemaID: schemaID}
	if _, held := program.ModuleEntryRootCellFor(0, 0); held {
		t.Fatal("sparse child ordinal was laundered as original position")
	}
	if got, held := program.ModuleEntryRootCellFor(0, 3); !held || got.ID() != cell.ID() {
		t.Fatal("original sparse position unavailable")
	}
}

func TestModuleRowsRejectUnavailableOptionalIdentityLaundering(t *testing.T) {
	id := moduleID(t, "required")
	if _, ok := programschema.NewModuleImport(id, id, identity.ContentID{}, 0, 1, true); ok {
		t.Fatal("present alias accepted unavailable identity")
	}
	if _, ok := programschema.NewModuleEntryMember(id, id, id, identity.ContentID{}, id, id, 1, 0, true); ok {
		t.Fatal("present member value accepted unavailable identity")
	}
	if _, ok := programschema.NewModuleEntry(id, id, 1, 1, 0, 2, 0, 0, 0, 0); ok {
		t.Fatal("sparse root-cell count exceeded fixed root width")
	}
}
