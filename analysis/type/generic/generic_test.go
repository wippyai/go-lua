package generic

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestArgExtraction(t *testing.T) {
	param := typ.NewTypeParam("T", nil)
	box := typ.NewGeneric("Box", []*typ.TypeParam{param}, param)
	inst := typ.Instantiate(box, typ.String)

	got, ok := Arg(typ.NewAlias("StringBox", inst), 0)
	if !ok {
		t.Fatal("Arg(alias Box<string>, 0) failed")
	}
	assertType(t, got, typ.String)

	if _, ok := Arg(inst, 1); ok {
		t.Fatal("Arg(Box<string>, 1) succeeded")
	}
}

func TestInstantiateOne(t *testing.T) {
	param := typ.NewTypeParam("T", nil)
	channel := typ.NewGeneric("Channel", []*typ.TypeParam{param}, typ.NewInterface("Channel", nil))

	got, ok := InstantiateOne(channel, typ.NewMeta(typ.String))
	if !ok {
		t.Fatal("InstantiateOne(Channel<T>, typeof(string)) failed")
	}
	assertType(t, got, typ.Instantiate(channel, typ.String))
}

func assertType(t *testing.T, got typ.Type, want typ.Type) {
	t.Helper()
	if !typ.TypeEquals(got, want) {
		t.Fatalf("type = %v, want %v", got, want)
	}
}
