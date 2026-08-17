package source

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestContentIdentityChangesOnlyForAuthoredSourceRows(t *testing.T) {
	baseInput, baseIndex := sourceFixture(1)
	base := finalizeSource(t, baseInput, baseIndex)
	derivedInput, derivedIndex := sourceFixture(1)
	derivedIndex.OutcomeOrigins = []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyBody, 1)}
	derived := finalizeSource(t, derivedInput, derivedIndex)
	if got, want := derived.Cold().ContentID(), base.Cold().ContentID(); got != want {
		t.Fatalf("derived outcome changed authored ContentID: %x != %x", got, want)
	}
	changedInput, changedIndex := sourceFixture(1)
	changedInput.Name = "other.lua"
	for index := range changedInput.Families {
		for span := range changedInput.Families[index].Spans {
			changedInput.Families[index].Spans[span].File = changedInput.Name
		}
	}
	changed := finalizeSource(t, changedInput, changedIndex)
	if changed.Cold().ContentID() == base.Cold().ContentID() {
		t.Fatal("authored source name did not change ContentID")
	}
}
