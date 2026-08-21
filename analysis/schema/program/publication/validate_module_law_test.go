package publication

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	programcatalog "github.com/wippyai/go-lua/analysis/schema/program/catalog"
	"github.com/wippyai/go-lua/analysis/schema/program/lifecycle"
	programstate "github.com/wippyai/go-lua/analysis/schema/program/state"
)

func moduleSealLawID(t *testing.T, name string) identity.ContentID {
	t.Helper()
	id, ok := identity.DeriveContentID("program-publication/module-seal-law/"+name, nil)
	if !ok {
		t.Fatalf("derive %s", name)
	}
	return id
}

func moduleSealState(returnIDs ...identity.ContentID) validationState {
	outcomes := make(map[identity.ContentID]int, len(returnIDs))
	for index, id := range returnIDs {
		outcomes[id] = index
	}
	return validationState{
		callRows:    map[identity.ContentID]struct{}{},
		outcomeRows: outcomes,
	}
}

func moduleSealValidator(t *testing.T, entries []programschema.ModuleEntry, cells []programschema.ModuleEntryRootCell, lifetimes []lifecycle.StorageCellLifetime) *validator {
	t.Helper()
	seed := moduleSealLawID(t, "catalog")
	catalog, ok := programcatalog.CatalogID(seed)
	if !ok {
		t.Fatal("seal catalog")
	}
	bodyID := moduleSealLawID(t, "entry-body")
	outcomes := make([]programschema.Outcome, 0, len(entries))
	for _, entry := range entries {
		outcome, outcomeOK := programschema.NewOutcome(entry.ReturnID(), bodyID, identity.ContentID{}, identity.ContentID{}, programschema.OutcomeReturn, 0, 0, 0, 0, false, false)
		if !outcomeOK {
			t.Fatal("return outcome")
		}
		outcomes = append(outcomes, outcome)
	}
	frozen, ok := (Publication{
		Outcomes:             outcomes,
		ModuleEntries:        entries,
		ModuleEntryRootCells: cells,
		Lifecycle:            lifecycle.Publication{StorageCellLifetimes: lifetimes},
	}).Seal(catalog, identity.StoreID(2))
	if !ok {
		t.Fatal("seal module publication")
	}
	state, ok := programstate.New(frozen, catalog)
	if !ok {
		t.Fatal("sealed module state")
	}
	lifecycleView, ok := lifecycle.NewView(state)
	if !ok {
		t.Fatal("sealed lifecycle view")
	}
	return &validator{frozen: frozen, catalog: catalog, lifecycle: lifecycleView}
}

func TestModuleSealRejectsMalformedSpans(t *testing.T) {
	entryID := moduleSealLawID(t, "bad-span-entry")
	returnID := moduleSealLawID(t, "bad-span-return")
	entry, ok := programschema.NewModuleEntry(entryID, returnID, 1, 1, 0, 1, 0, 0, 0, 0)
	if !ok {
		t.Fatal("entry")
	}
	validator := moduleSealValidator(t, []programschema.ModuleEntry{entry}, nil, nil)
	state := moduleSealState(returnID)
	if validator.validateSealModule(&state) {
		t.Fatal("Module seal accepted a span wider than its published child plane")
	}
}

func TestModuleSealRejectsMissingReturnOutcome(t *testing.T) {
	returnID := moduleSealLawID(t, "missing-return")
	entry, ok := programschema.NewModuleEntry(moduleSealLawID(t, "missing-return-entry"), returnID, 1, 0, 0, 0, 0, 0, 0, 0)
	if !ok {
		t.Fatal("entry")
	}
	validator := moduleSealValidator(t, []programschema.ModuleEntry{entry}, nil, nil)
	state := moduleSealState()
	if validator.validateSealModule(&state) {
		t.Fatal("Module seal accepted an entry without its exact Return outcome")
	}
}

func TestModuleSealRejectsOutOfOrderRootPositions(t *testing.T) {
	entryID := moduleSealLawID(t, "position-entry")
	returnID := moduleSealLawID(t, "position-return")
	cell0 := moduleSealLawID(t, "position-cell-0")
	cell1 := moduleSealLawID(t, "position-cell-1")
	entry, ok := programschema.NewModuleEntry(entryID, returnID, 1, 2, 0, 2, 0, 0, 0, 0)
	if !ok {
		t.Fatal("entry")
	}
	first, ok := programschema.NewModuleEntryRootCell(moduleSealLawID(t, "position-child-0"), entryID, cell0, 1)
	if !ok {
		t.Fatal("first child")
	}
	second, ok := programschema.NewModuleEntryRootCell(moduleSealLawID(t, "position-child-1"), entryID, cell1, 0)
	if !ok {
		t.Fatal("second child")
	}
	lifetime0, ok := lifecycle.NewStorageCellLifetime(cell0, lifecycle.StorageLifetimeModule)
	if !ok {
		t.Fatal("cell lifetime 0")
	}
	lifetime1, ok := lifecycle.NewStorageCellLifetime(cell1, lifecycle.StorageLifetimeModule)
	if !ok {
		t.Fatal("cell lifetime 1")
	}
	validator := moduleSealValidator(t, []programschema.ModuleEntry{entry}, []programschema.ModuleEntryRootCell{first, second}, []lifecycle.StorageCellLifetime{lifetime0, lifetime1})
	state := moduleSealState(returnID)
	if validator.validateSealModule(&state) {
		t.Fatal("Module seal accepted out-of-order root positions")
	}
}

func TestModuleSealRejectsDuplicateRootPositions(t *testing.T) {
	entryID := moduleSealLawID(t, "duplicate-position-entry")
	returnID := moduleSealLawID(t, "duplicate-position-return")
	cell0 := moduleSealLawID(t, "duplicate-position-cell-0")
	cell1 := moduleSealLawID(t, "duplicate-position-cell-1")
	entry, ok := programschema.NewModuleEntry(entryID, returnID, 1, 2, 0, 2, 0, 0, 0, 0)
	if !ok {
		t.Fatal("entry")
	}
	first, ok := programschema.NewModuleEntryRootCell(moduleSealLawID(t, "duplicate-position-child-0"), entryID, cell0, 0)
	if !ok {
		t.Fatal("first child")
	}
	second, ok := programschema.NewModuleEntryRootCell(moduleSealLawID(t, "duplicate-position-child-1"), entryID, cell1, 0)
	if !ok {
		t.Fatal("second child")
	}
	lifetime0, ok := lifecycle.NewStorageCellLifetime(cell0, lifecycle.StorageLifetimeModule)
	if !ok {
		t.Fatal("cell lifetime 0")
	}
	lifetime1, ok := lifecycle.NewStorageCellLifetime(cell1, lifecycle.StorageLifetimeModule)
	if !ok {
		t.Fatal("cell lifetime 1")
	}
	validator := moduleSealValidator(t, []programschema.ModuleEntry{entry}, []programschema.ModuleEntryRootCell{first, second}, []lifecycle.StorageCellLifetime{lifetime0, lifetime1})
	state := moduleSealState(returnID)
	if validator.validateSealModule(&state) {
		t.Fatal("Module seal accepted duplicate root positions")
	}
}

func TestModuleSealRejectsOutOfOrderReturnOrdinals(t *testing.T) {
	return0 := moduleSealLawID(t, "ordinal-return-0")
	return1 := moduleSealLawID(t, "ordinal-return-1")
	entry0, ok := programschema.NewModuleEntry(moduleSealLawID(t, "ordinal-entry-0"), return0, 2, 0, 0, 0, 0, 0, 0, 0)
	if !ok {
		t.Fatal("entry 0")
	}
	entry1, ok := programschema.NewModuleEntry(moduleSealLawID(t, "ordinal-entry-1"), return1, 1, 0, 0, 0, 0, 0, 0, 0)
	if !ok {
		t.Fatal("entry 1")
	}
	validator := moduleSealValidator(t, []programschema.ModuleEntry{entry0, entry1}, nil, nil)
	state := moduleSealState(return0, return1)
	if validator.validateSealModule(&state) {
		t.Fatal("Module seal accepted out-of-order Return ordinals")
	}
}

func TestModuleSealRejectsDuplicateReturnOrdinals(t *testing.T) {
	return0 := moduleSealLawID(t, "duplicate-ordinal-return-0")
	return1 := moduleSealLawID(t, "duplicate-ordinal-return-1")
	entry0, ok := programschema.NewModuleEntry(moduleSealLawID(t, "duplicate-ordinal-entry-0"), return0, 1, 0, 0, 0, 0, 0, 0, 0)
	if !ok {
		t.Fatal("entry 0")
	}
	entry1, ok := programschema.NewModuleEntry(moduleSealLawID(t, "duplicate-ordinal-entry-1"), return1, 1, 0, 0, 0, 0, 0, 0, 0)
	if !ok {
		t.Fatal("entry 1")
	}
	validator := moduleSealValidator(t, []programschema.ModuleEntry{entry0, entry1}, nil, nil)
	state := moduleSealState(return0, return1)
	if validator.validateSealModule(&state) {
		t.Fatal("Module seal accepted duplicate Return ordinals")
	}
}
