package static

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestOperandsQueryRootPreservesSparseAndDenseOrdinals(t *testing.T) {
	component := staticContentComponent(t, operandsFixture(t))
	view := component.View().Operands()
	if got, ok := view.Claims().At(0); !ok || got != keyspace.MakeTerm(keyspace.FamilyValueClaim, 1) {
		t.Fatalf("Claims.At(0) = %v/%v", got, ok)
	}
	if got, ok := view.TypeValues().At(0); !ok || got != keyspace.MakeTerm(keyspace.FamilyTypeValue, 1) {
		t.Fatalf("TypeValues.At(0) = %v/%v", got, ok)
	}
	if got, ok := view.Annotations().At(1); !ok || got != keyspace.MakeTerm(keyspace.FamilyAnnotation, 2) {
		t.Fatalf("Annotations.At(1) = %v/%v", got, ok)
	}
	if _, ok := view.Claims().At(-1); ok {
		t.Fatal("Claims.At accepted negative index")
	}
	if _, ok := view.Annotations().At(2); ok {
		t.Fatal("Annotations.At accepted out-of-range index")
	}
}
