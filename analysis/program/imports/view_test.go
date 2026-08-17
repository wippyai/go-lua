package imports

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestViewLookupRejectsWrongFamilyAndBounds(t *testing.T) {
	component := buildCommitted(t, CommitInput{
		Resolutions: authoredResolutions(7, 8),
		Entry:       emptyEntry(),
	})
	view := component.View()
	if _, ok := view.ImportAt(-1); ok {
		t.Fatal("ImportAt accepted a negative index")
	}
	if _, ok := view.ImportAt(view.Count()); ok {
		t.Fatal("ImportAt accepted the count boundary")
	}
	if _, ok := view.Import(keyspace.MakeTerm(keyspace.FamilyCall, 1)); ok {
		t.Fatal("Import accepted a non-Import family")
	}
	if view.Entry().ReturnCount() != 0 {
		t.Fatal("committed View did not expose its empty Entry projection")
	}
}
