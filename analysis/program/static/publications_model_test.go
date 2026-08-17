package static

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestPublicationModelRowsRetainPairIdentityAcrossCallerMutation(t *testing.T) {
	input := publicationFixture(t)
	draft, err := Build(input)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	input.Publications.Type[0].Assign = 0
	input.Publications.Type[0].Pair = 99
	input.Publications.Type[0].Target = 0
	component, err := commitStaticDraft(t, draft)
	if err != nil {
		t.Fatalf("Commit() error = %v", err)
	}
	assign, pair, target, ok := component.View().Publications().Get(keyspace.MakeTerm(keyspace.FamilyTypePublication, 1))
	if !ok || assign != keyspace.MakeTerm(keyspace.FamilyAssign, 1) || pair != 0 ||
		target != keyspace.MakeTerm(keyspace.FamilyTypeRef, 1) {
		t.Fatalf("Publication model row = %v/%d/%v/%v", assign, pair, target, ok)
	}
}
