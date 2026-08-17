package static

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestOperatorsQueryRootEnumeratesAllTypedColumns(t *testing.T) {
	component := staticContentComponent(t, operatorFixture())
	view := component.View().Operators()
	if view.TypeOfs().Count() != 2 || view.KeyOfs().Count() != 1 ||
		view.IndexAccesses().Count() != 1 || view.Conditionals().Count() != 1 {
		t.Fatalf("operator column counts = typeof:%d keyof:%d index:%d conditional:%d",
			view.TypeOfs().Count(), view.KeyOfs().Count(), view.IndexAccesses().Count(), view.Conditionals().Count())
	}
	if got, ok := view.Conditionals().At(0); !ok || got != keyspace.MakeTerm(keyspace.FamilyTypeConditional, 1) {
		t.Fatalf("Conditionals.At(0) = %v/%v", got, ok)
	}
	if _, _, ok := view.TypeOfs().Get(keyspace.MakeTerm(keyspace.FamilyBody, 1)); ok {
		t.Fatal("TypeOfs.Get accepted a non-TypeOf family")
	}
}
