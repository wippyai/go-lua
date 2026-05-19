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
