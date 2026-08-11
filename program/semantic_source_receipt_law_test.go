package program

import (
	"testing"

	programflow "github.com/wippyai/go-lua/program/flow"
	programmodule "github.com/wippyai/go-lua/program/module"
	"github.com/wippyai/go-lua/program/semanticsource"
	programsource "github.com/wippyai/go-lua/program/source"
	programstatic "github.com/wippyai/go-lua/program/static"
)

func TestProgramSemanticSourceReceiptHasExactQuartetAndZeroRows(t *testing.T) {
	published, err := Publish(rootAssembly(t, "program-semantic-source-receipt.lua"))
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	receipt, ok := published.SemanticSourceReceipt()
	if !ok || !receipt.Valid() {
		t.Fatal("sealed Program has no valid semantic-source receipt")
	}
	if receipt.OwnerID() != published.ContentID() || receipt.Count() != programSemanticSourcePublicationCount {
		t.Fatalf("receipt owner/count = %x/%d, want %x/%d", receipt.OwnerID(), receipt.Count(), published.ContentID(), programSemanticSourcePublicationCount)
	}
	views, ok := receipt.Views()
	if !ok || !views.Valid() || views.OwnerID() != published.ContentID() {
		t.Fatal("receipt views lost exact Program provenance")
	}
	wantSource := published.Source().Identity().ContentID()
	wantFlow := published.Flow().ContentID()
	wantStatic := published.Static().ContentID()
	wantModule := published.Module().ContentID()
	if views.SourceID() != wantSource || views.FlowID() != wantFlow ||
		views.StaticID() != wantStatic || views.ModuleID() != wantModule {
		t.Fatalf("child IDs = %x/%x/%x/%x, want %x/%x/%x/%x", views.SourceID(), views.FlowID(), views.StaticID(), views.ModuleID(), wantSource, wantFlow, wantStatic, wantModule)
	}
	if views.Source().OwnerID() != published.ContentID() || views.Flow().OwnerID() != published.ContentID() ||
		views.Static().OwnerID() != published.ContentID() || views.Module().OwnerID() != published.ContentID() ||
		views.Source().ChildID() != wantSource || views.Flow().ChildID() != wantFlow ||
		views.Static().ChildID() != wantStatic || views.Module().ChildID() != wantModule {
		t.Fatal("named child fragments lost Program or child provenance")
	}

	rows := receipt.Publications()
	if len(rows) != programSemanticSourcePublicationCount {
		t.Fatalf("receipt publications = %d, want %d", len(rows), programSemanticSourcePublicationCount)
	}
	seen := make(map[semanticsource.Token]bool, len(rows))
	zero := 0
	var previous semanticsource.Token
	for index, row := range rows {
		token := row.Definition().Token()
		if seen[token] {
			t.Fatalf("duplicate receipt token: %#v", token)
		}
		if index > 0 && semanticTokenAfter(previous, token) {
			t.Fatalf("receipt tokens are not canonical at row %d", index)
		}
		previous = token
		seen[token] = true
		if row.Count() == 0 {
			zero++
		}
	}
	if zero == 0 {
		t.Fatal("receipt did not retain any required zero-cardinality definition")
	}

	// Replay and detached-slice mutation must not alter the sealed receipt.
	replayed := receipt.Publications()
	if len(replayed) != len(rows) {
		t.Fatal("replayed Program publication unavailable")
	}
	replayed[0] = semanticsource.Publication{}
	again := receipt.Publications()
	if len(again) != len(rows) {
		t.Fatal("receipt changed after caller slice mutation")
	}
	for index, row := range rows {
		if again[index] != row {
			t.Fatalf("replayed receipt changed at row %d", index)
		}
	}

	cursor := receipt.Cursor()
	for index := 0; index < programSemanticSourcePublicationCount; index++ {
		if _, ok := cursor.Next(); !ok {
			t.Fatalf("receipt cursor ended at row %d", index)
		}
	}
	if _, ok := cursor.Next(); ok {
		t.Fatal("receipt cursor yielded beyond 57 rows")
	}
}

func semanticTokenAfter(left, right semanticsource.Token) bool {
	if left.Origin() != right.Origin() {
		return left.Origin() > right.Origin()
	}
	if left.Facet() != right.Facet() {
		return left.Facet() > right.Facet()
	}
	return left.Revision() > right.Revision()
}

func TestProgramSemanticSourceReceiptRejectsForeignChildAndMalformedState(t *testing.T) {
	left, err := Publish(rootAssembly(t, "program-semantic-source-left.lua"))
	if err != nil {
		t.Fatalf("left Publish: %v", err)
	}
	right, err := Publish(rootAssembly(t, "program-semantic-source-right.lua"))
	if err != nil {
		t.Fatalf("right Publish: %v", err)
	}

	leftReceipt := left.semanticReceipt
	foreign := leftReceipt
	foreign.views.source = right.semanticReceipt.views.source
	left.semanticReceipt = foreign
	if _, ok := left.SemanticSourceReceipt(); ok {
		t.Fatal("Program accepted a foreign child fragment")
	}

	left.semanticReceipt = leftReceipt
	malformed := leftReceipt
	malformed.views.flow.SemanticSourceView = SemanticSourceView{}
	left.semanticReceipt = malformed
	if _, ok := left.SemanticSourceReceipt(); ok {
		t.Fatal("Program accepted a truncated child fragment")
	}
}

func TestProgramSemanticSourceReceiptRejectsFederatedRangeSubstitution(t *testing.T) {
	published, err := Publish(rootAssembly(t, "program-semantic-source-range-substitution.lua"))
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	original := published.semanticReceipt
	tests := []struct {
		name string
		rows func() ([]semanticsource.Publication, error)
		seal func([]semanticsource.Publication) (semanticsource.PublicationRange, error)
		set  func(*SemanticSourceReceipt, semanticsource.PublicationRange)
	}{
		{
			name: "Source",
			rows: func() ([]semanticsource.Publication, error) {
				return programsource.SemanticSourceFragment(published.Source())
			},
			seal: func(rows []semanticsource.Publication) (semanticsource.PublicationRange, error) {
				fragment, err := programsource.SealSemanticSourceFragment(published.Source().Identity().ContentID(), rows)
				if err != nil {
					return semanticsource.PublicationRange{}, err
				}
				return fragment.Range(), nil
			},
			set: func(receipt *SemanticSourceReceipt, rangeValue semanticsource.PublicationRange) {
				receipt.views.source.rangeValue = rangeValue
			},
		},
		{
			name: "Flow",
			rows: func() ([]semanticsource.Publication, error) {
				return programflow.SemanticSourceFragment(published.Flow())
			},
			seal: func(rows []semanticsource.Publication) (semanticsource.PublicationRange, error) {
				fragment, err := programflow.SealSemanticSourceFragment(published.Flow().ContentID(), rows)
				if err != nil {
					return semanticsource.PublicationRange{}, err
				}
				return fragment.Range(), nil
			},
			set: func(receipt *SemanticSourceReceipt, rangeValue semanticsource.PublicationRange) {
				receipt.views.flow.rangeValue = rangeValue
			},
		},
		{
			name: "Static",
			rows: func() ([]semanticsource.Publication, error) {
				return programstatic.SemanticSourceFragment(published.Static())
			},
			seal: func(rows []semanticsource.Publication) (semanticsource.PublicationRange, error) {
				fragment, err := programstatic.SealSemanticSourceFragment(published.Static().ContentID(), rows)
				if err != nil {
					return semanticsource.PublicationRange{}, err
				}
				return fragment.Range(), nil
			},
			set: func(receipt *SemanticSourceReceipt, rangeValue semanticsource.PublicationRange) {
				receipt.views.static.rangeValue = rangeValue
			},
		},
		{
			name: "Module",
			rows: func() ([]semanticsource.Publication, error) {
				return programmodule.SemanticSourceFragment(published.Module())
			},
			seal: func(rows []semanticsource.Publication) (semanticsource.PublicationRange, error) {
				fragment, err := programmodule.SealSemanticSourceFragment(published.Module().ContentID(), rows)
				if err != nil {
					return semanticsource.PublicationRange{}, err
				}
				return fragment.Range(), nil
			},
			set: func(receipt *SemanticSourceReceipt, rangeValue semanticsource.PublicationRange) {
				receipt.views.module.rangeValue = rangeValue
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rows, err := test.rows()
			if err != nil {
				t.Fatal(err)
			}
			if len(rows) == 0 {
				t.Fatal("owner returned no rows")
			}
			rows = append([]semanticsource.Publication(nil), rows...)
			row, err := semanticsource.SealPublication(rows[0].Definition(), rows[0].Count()+1)
			if err != nil {
				t.Fatal(err)
			}
			rows[0] = row
			forged, err := test.seal(rows)
			if err != nil {
				t.Fatal(err)
			}
			candidate := original
			test.set(&candidate, forged)
			published.semanticReceipt = candidate
			if _, ok := published.SemanticSourceReceipt(); ok {
				t.Fatal("Program accepted a substituted owner range")
			}
			published.semanticReceipt = original
		})
	}
}
