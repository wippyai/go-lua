package programdiagnostic

import (
	"fmt"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	programcatalog "github.com/wippyai/go-lua/analysis/schema/program/catalog"
	programstate "github.com/wippyai/go-lua/analysis/schema/program/state"
	"github.com/wippyai/go-lua/analysis/snapshot"
)

func programDiagnosticLawID(t *testing.T, name string) identity.ContentID {
	t.Helper()
	id, ok := identity.DeriveContentID("program-diagnostic-law/"+name, nil)
	if !ok {
		t.Fatalf("derive %s", name)
	}
	return id
}

func programDiagnosticLawView(t *testing.T, publication Publication, catalog identity.ContentID) View {
	t.Helper()
	builder := snapshot.NewFrozen(catalog, identity.StoreID(1))
	for slot := uint32(0); slot < 58; slot++ {
		if slot >= 39 && slot <= 41 {
			continue
		}
		axis := snapshot.Axis[uint32, uint32]{SchemaID: catalog, Slot: slot}
		content := snapshot.Content[uint32, uint32]{
			Sequence:    []uint32{},
			Denominator: programDiagnosticLawID(t, fmt.Sprintf("filler-%d", slot)),
		}
		if err := snapshot.PutFrozenColumn(&builder, axis, content); err != nil {
			t.Fatalf("put filler slot %d: %v", slot, err)
		}
	}
	if !publication.Append(&builder, catalog) {
		t.Fatal("diagnostic publication append")
	}
	frozen, err := builder.Seal()
	if err != nil {
		t.Fatalf("seal diagnostic publication: %v", err)
	}
	state, ok := programstate.New(frozen, catalog)
	if !ok {
		t.Fatal("open program state")
	}
	view, ok := NewView(state)
	if !ok {
		t.Fatal("open diagnostic view")
	}
	return view
}

func TestDiagnosticFamiliesBindCanonicalSlots(t *testing.T) {
	if got, want := DiagnosticObservationFamily().Definition(), programcatalog.DiagnosticObservation(); got != want {
		t.Fatalf("diagnostic observation definition = %d/%s, want %d/%s", got.Slot(), got.Name(), want.Slot(), want.Name())
	}
	if got, want := DiagnosticEvidenceFamily().Definition(), programcatalog.DiagnosticEvidence(); got != want {
		t.Fatalf("diagnostic evidence definition = %d/%s, want %d/%s", got.Slot(), got.Name(), want.Slot(), want.Name())
	}
	if got, want := DiagnosticPathFamily().Definition(), programcatalog.DiagnosticPath(); got != want {
		t.Fatalf("diagnostic path definition = %d/%s, want %d/%s", got.Slot(), got.Name(), want.Slot(), want.Name())
	}
}

func TestDiagnosticPublicationAppendsAllEmptyColumns(t *testing.T) {
	view := programDiagnosticLawView(t, Publication{}, programDiagnosticLawID(t, "empty-catalog"))
	checks := []func() (int, bool){
		view.DiagnosticObservationCount,
		view.DiagnosticEvidenceCount,
		view.DiagnosticPathCount,
	}
	for index, check := range checks {
		if count, published := check(); !published || count != 0 {
			t.Errorf("diagnostic family %d count/published = %d/%v", index, count, published)
		}
	}
	if _, held := view.DiagnosticObservationForID(programDiagnosticLawID(t, "missing-observation")); held {
		t.Fatal("missing diagnostic observation resolved")
	}
	if _, held := view.DiagnosticObservationOrdinalForID(programDiagnosticLawID(t, "missing-ordinal")); held {
		t.Fatal("missing diagnostic ordinal resolved")
	}
}
