package artifact

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	programcatalog "github.com/wippyai/go-lua/analysis/schema/program/catalog"
	programpublication "github.com/wippyai/go-lua/analysis/schema/program/publication"
)

// moduleIdentityLawID gives each row a stable owner-issued identity without
// coupling this law to a compiler fixture. The test is about the canonical
// Module publication, not about how a particular parser happens to reach it.
func moduleIdentityLawID(t *testing.T, name string) identity.ContentID {
	t.Helper()
	id, ok := identity.DeriveContentID("artifact/module-identity-law/"+name, nil)
	if !ok {
		t.Fatalf("derive %s", name)
	}
	return id
}

type moduleIdentityVariant struct {
	alias        identity.ContentID
	requestKey   keyspace.Key
	returnOrd    uint32
	rootWidth    uint32
	cellPos      uint32
	secondParent identity.ContentID
}

// moduleIdentityPublication is deliberately small: every non-Module family
// is still published as an empty plane by Publication.Seal, while the six
// Module planes below are the only semantic payload under test.
func moduleIdentityPublication(t *testing.T, variant moduleIdentityVariant) (identity.ContentID, programpublication.Publication) {
	t.Helper()
	importID := moduleIdentityLawID(t, "import")
	callID := moduleIdentityLawID(t, "call")
	requestValueID := moduleIdentityLawID(t, "request-value")
	entryID := moduleIdentityLawID(t, "entry")
	returnID := moduleIdentityLawID(t, "return")
	cellID := moduleIdentityLawID(t, "cell")
	functionID := moduleIdentityLawID(t, "function")
	tableID := moduleIdentityLawID(t, "table")
	field0ID := moduleIdentityLawID(t, "field-0")
	field1ID := moduleIdentityLawID(t, "field-1")

	request, ok := programschema.NewModuleRequest(
		moduleIdentityLawID(t, "request"), importID, requestValueID, variant.requestKey,
	)
	if !ok {
		t.Fatal("module request")
	}
	importRow, ok := programschema.NewModuleImport(
		importID, callID, variant.alias, 0, 1, variant.alias.Available(),
	)
	if !ok {
		t.Fatal("module import")
	}
	entry, ok := programschema.NewModuleEntry(
		entryID, returnID, variant.returnOrd, variant.rootWidth,
		0, 1, 0, 1, 0, 2,
	)
	if !ok {
		t.Fatal("module entry")
	}
	cell, ok := programschema.NewModuleEntryRootCell(
		moduleIdentityLawID(t, "root-cell"), entryID, cellID, variant.cellPos,
	)
	if !ok {
		t.Fatal("module root cell")
	}
	function, ok := programschema.NewModuleEntryRootFunction(
		moduleIdentityLawID(t, "root-function"), entryID, functionID, 1,
	)
	if !ok {
		t.Fatal("module root function")
	}
	firstMember, ok := programschema.NewModuleEntryMember(
		field0ID, field0ID, tableID, identity.ContentID{}, entryID, tableID,
		keyspace.Key(11), 0, false,
	)
	if !ok {
		t.Fatal("first module member")
	}
	secondParent := tableID
	if variant.secondParent.Available() {
		secondParent = variant.secondParent
	}
	secondMember, ok := programschema.NewModuleEntryMember(
		field1ID, field1ID, secondParent, functionID, entryID, tableID,
		keyspace.Key(12), 1, true,
	)
	if !ok {
		t.Fatal("second module member")
	}

	catalogSeed := moduleIdentityLawID(t, "catalog")
	catalog, ok := programcatalog.CatalogID(catalogSeed)
	if !ok {
		t.Fatal("module catalog")
	}
	return catalog, programpublication.Publication{
		ModuleImports:            []programschema.ModuleImport{importRow},
		ModuleRequests:           []programschema.ModuleRequest{request},
		ModuleEntries:            []programschema.ModuleEntry{entry},
		ModuleEntryRootCells:     []programschema.ModuleEntryRootCell{cell},
		ModuleEntryRootFunctions: []programschema.ModuleEntryRootFunction{function},
		ModuleEntryMembers:       []programschema.ModuleEntryMember{firstMember, secondMember},
	}
}

func moduleIdentityArtifact(t *testing.T, variant moduleIdentityVariant) *Artifact {
	t.Helper()
	catalog, publication := moduleIdentityPublication(t, variant)
	frozen, ok := publication.Seal(catalog, identity.StoreID(1))
	if !ok {
		t.Fatal("module publication seal")
	}
	return &Artifact{frozen: frozen, coldCatalog: catalog}
}

func moduleIdentityBase() moduleIdentityVariant {
	return moduleIdentityVariant{
		requestKey: keyspace.Key(7), returnOrd: 1, rootWidth: 3, cellPos: 0,
	}
}

// Every Module semantic field must participate in the Artifact identity even
// when all six family counts stay unchanged. These are fields that previously
// disappeared at a projection boundary and therefore need direct hostile
// coverage at the canonical publication.
func TestArtifactIDCommitsModuleSemanticFields(t *testing.T) {
	base := moduleIdentityArtifact(t, moduleIdentityBase())
	baseID := artifactID(base)
	if !baseID.Available() {
		t.Fatal("base Module publication did not receive an Artifact identity")
	}

	variants := []struct {
		name string
		edit func(moduleIdentityVariant) moduleIdentityVariant
	}{
		{name: "alias", edit: func(v moduleIdentityVariant) moduleIdentityVariant {
			v.alias = moduleIdentityLawID(t, "different-alias")
			return v
		}},
		{name: "request-key", edit: func(v moduleIdentityVariant) moduleIdentityVariant {
			v.requestKey = keyspace.Key(8)
			return v
		}},
		{name: "return-ordinal", edit: func(v moduleIdentityVariant) moduleIdentityVariant {
			v.returnOrd = 2
			return v
		}},
		{name: "root-width", edit: func(v moduleIdentityVariant) moduleIdentityVariant {
			v.rootWidth = 4
			return v
		}},
		{name: "root-position", edit: func(v moduleIdentityVariant) moduleIdentityVariant {
			v.cellPos = 2
			return v
		}},
		{name: "member-parent", edit: func(v moduleIdentityVariant) moduleIdentityVariant {
			v.secondParent = moduleIdentityLawID(t, "field-0")
			return v
		}},
	}
	for _, variant := range variants {
		t.Run(variant.name, func(t *testing.T) {
			changed := artifactID(moduleIdentityArtifact(t, variant.edit(moduleIdentityBase())))
			if !changed.Available() {
				t.Fatal("changed Module publication was refused before identity comparison")
			}
			if changed == baseID {
				t.Fatalf("Module %s change did not change Artifact identity", variant.name)
			}
		})
	}
}
