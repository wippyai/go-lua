package static

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestPublicationsQueryRootReturnsCanonicalTermAndPair(t *testing.T) {
	component := staticContentComponent(t, publicationFixture(t))
	view := component.View().Publications()
	term, ok := view.At(0)
	if !ok || term != keyspace.MakeTerm(keyspace.FamilyTypePublication, 1) {
		t.Fatalf("Publications.At(0) = %v/%v", term, ok)
	}
	assign, pair, target, ok := view.Get(term)
	if !ok || assign != keyspace.MakeTerm(keyspace.FamilyAssign, 1) || pair != 0 ||
		target != keyspace.MakeTerm(keyspace.FamilyTypeRef, 1) {
		t.Fatalf("Publications.Get() = %v/%d/%v/%v", assign, pair, target, ok)
	}
	if _, ok := view.At(1); ok {
		t.Fatal("Publications.At accepted out-of-range index")
	}
}
