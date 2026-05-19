package value

import (
	"testing"

	"github.com/wippyai/go-lua/types/typ"
)

func TestExtendsRecord_NilTypes(t *testing.T) {
	if ExtendsRecord(nil, typ.String) {
		t.Error("nil a should not extend")
	}
	if ExtendsRecord(typ.String, nil) {
		t.Error("nil b should not extend")
	}
}

func TestExtendsRecord_NotRecord(t *testing.T) {
	if ExtendsRecord(typ.String, typ.String) {
		t.Error("non-record should not extend")
	}
}

func TestExtendsRecord_MapComponentConsistency(t *testing.T) {
	oldRec := typ.NewRecord().MapComponent(typ.String, typ.Number).Build()
	newRec := typ.NewRecord().Field("x", typ.Number).Build()
	if ExtendsRecord(newRec, oldRec) {
		t.Error("record without map component should not extend record with map component")
	}
}

func TestRefinesFalsyMapKey(t *testing.T) {
	candidate := typ.NewMap(typ.String, typ.Number)
	baseline := typ.NewMap(typ.NewUnion(typ.String, typ.False), typ.Number)

	ok, changed := RefinesFalsyMapKey(candidate, baseline)
	if !ok || !changed {
		t.Fatalf("expected truthy key refinement, got ok=%v changed=%v", ok, changed)
	}
}

func TestNestedNilOnlyRegression(t *testing.T) {
	candidate := typ.NewRecord().Field("value", typ.Nil).Build()
	baseline := typ.NewRecord().OptField("value", typ.String).Build()

	if !NestedNilOnlyRegression(candidate, baseline) {
		t.Fatalf("expected nested nil-only regression")
	}
}

func TestContainsNestedStructuralShape(t *testing.T) {
	shape := typ.NewMap(typ.String, typ.Any)
	growing := typ.NewMap(typ.String, typ.NewMap(typ.String, typ.Nil))

	if !ContainsNestedStructuralShape(growing, shape) {
		t.Fatalf("expected nested structural shape")
	}
}
