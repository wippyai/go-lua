package globalenv

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/typ"
)

func TestTypeOverlayFromMapNormalizesNames(t *testing.T) {
	got := TypeOverlayFromMap(map[string]typ.Type{
		"z":   typ.Number,
		"":    typ.Boolean,
		"a":   typ.String,
		"nil": nil,
	})
	want := TypeOverlay{
		{Name: Name("a"), Type: typ.String},
		{Name: Name("z"), Type: typ.Number},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("TypeOverlayFromMap = %+v, want %+v", got, want)
	}
}

func TestTypeOverlayCloneNormalizesAndCopies(t *testing.T) {
	overlay := TypeOverlay{
		{Name: Name("z"), Type: typ.Number},
		{Name: Name("a"), Type: typ.String},
		{Name: Name("a"), Type: typ.Boolean},
		{Name: Name(""), Type: typ.Integer},
	}
	cloned := overlay.Clone()
	if len(cloned) != 2 || cloned[0].Name != Name("a") || cloned[1].Name != Name("z") {
		t.Fatalf("Clone order = %+v, want a,z", cloned)
	}
	if merged, ok := cloned.Type("a"); !ok || !typ.TypeEquals(merged, typ.JoinReturnSlot(typ.String, typ.Boolean)) {
		t.Fatalf("cloned a = %v/%v, want string|boolean", merged, ok)
	}
	cloned[0].Type = typ.Nil
	if overlay[0].Type != typ.Number || overlay[1].Type != typ.String {
		t.Fatalf("mutating clone changed source overlay: %+v", overlay)
	}
}

func TestMergeTypeOverlayJoinsByNameDeterministically(t *testing.T) {
	got := MergeTypeOverlay(
		TypeOverlayFromMap(map[string]typ.Type{"z": typ.Number, "a": typ.String}),
		TypeOverlayFromMap(map[string]typ.Type{"a": typ.Boolean, "m": typ.Integer}),
	)
	if len(got) != 3 || got[0].Name != Name("a") || got[1].Name != Name("m") || got[2].Name != Name("z") {
		t.Fatalf("MergeTypeOverlay order = %+v, want a,m,z", got)
	}
	if merged, ok := got.Type("a"); !ok || !typ.TypeEquals(merged, typ.JoinReturnSlot(typ.String, typ.Boolean)) {
		t.Fatalf("merged a = %v, want string|boolean", merged)
	}
}

func TestOverrideTypeOverlayReplacesByNameDeterministically(t *testing.T) {
	got := OverrideTypeOverlay(
		TypeOverlayFromMap(map[string]typ.Type{"z": typ.Number, "a": typ.String}),
		TypeOverlayFromMap(map[string]typ.Type{"a": typ.Boolean, "m": typ.Integer}),
	)
	if len(got) != 3 || got[0].Name != Name("a") || got[1].Name != Name("m") || got[2].Name != Name("z") {
		t.Fatalf("OverrideTypeOverlay order = %+v, want a,m,z", got)
	}
	if overridden, ok := got.Type("a"); !ok || !typ.TypeEquals(overridden, typ.Boolean) {
		t.Fatalf("overridden a = %v, want boolean", overridden)
	}
}

func TestTypeOverlayRoundTripToMap(t *testing.T) {
	overlay := TypeOverlay{
		{Name: Name("ctx"), Type: typ.String},
		{Name: "", Type: typ.Boolean},
		{Name: Name("skip"), Type: nil},
	}
	got := overlay.ToMap()
	if len(got) != 1 || !typ.TypeEquals(got["ctx"], typ.String) {
		t.Fatalf("ToMap = %+v, want ctx=string", got)
	}
}

func TestTypeOverlayNamesPreserveDeterministicOrder(t *testing.T) {
	overlay := TypeOverlayFromMap(map[string]typ.Type{
		"z": typ.Number,
		"a": typ.String,
	})
	if got, want := overlay.Names(), []string{"a", "z"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Names = %+v, want %+v", got, want)
	}
}

func TestValueOverlayNormalizesAndConvergesDuplicates(t *testing.T) {
	stringValue := product.FromType(typ.String)
	boolValue := product.FromType(typ.Boolean)
	got := NormalizeValueOverlay(ValueOverlay{
		{Name: Name("z"), Value: product.FromType(typ.Number)},
		{Name: "", Value: product.FromType(typ.Integer)},
		{Name: Name("drop")},
		{Name: Name("a"), Value: stringValue},
		{Name: Name("a"), Value: boolValue},
	})
	if len(got) != 2 || got[0].Name != Name("a") || got[1].Name != Name("z") {
		t.Fatalf("NormalizeValueOverlay order = %+v, want a,z", got)
	}
	wantA := product.CarryForward(stringValue, boolValue)
	if !product.Equal(got[0].Value, wantA) {
		t.Fatalf("NormalizeValueOverlay duplicate a = %v, want %v", got[0].Value, wantA)
	}
}

func TestValueOverlayProjectionAndLookup(t *testing.T) {
	overlay := ValueOverlayFromTypeMap(map[string]typ.Type{
		"z":   typ.Number,
		"":    typ.Boolean,
		"a":   typ.String,
		"nil": nil,
	})
	if len(overlay) != 2 || overlay[0].Name != Name("a") || overlay[1].Name != Name("z") {
		t.Fatalf("ValueOverlayFromTypeMap = %+v, want a,z", overlay)
	}
	if got, ok := overlay.Type("a"); !ok || !typ.TypeEquals(got, typ.String) {
		t.Fatalf("Type(a) = %v, %v; want string, true", got, ok)
	}
	if _, ok := overlay.Value("missing"); ok {
		t.Fatal("Value(missing) returned true")
	}
	projected := overlay.ToTypeMap()
	if len(projected) != 2 || !typ.TypeEquals(projected["a"], typ.String) || !typ.TypeEquals(projected["z"], typ.Number) {
		t.Fatalf("ToTypeMap = %+v, want a=string,z=number", projected)
	}
}

func TestMergeValueOverlayCarriesForwardByNameDeterministically(t *testing.T) {
	leftA := product.FromType(typ.String)
	rightA := product.FromType(typ.Boolean)
	got := MergeValueOverlay(
		ValueOverlayFromValueMap(map[Name]product.AbstractValue{
			Name("z"): product.FromType(typ.Number),
			Name("a"): leftA,
		}),
		ValueOverlayFromValueMap(map[Name]product.AbstractValue{
			Name("a"): rightA,
			Name("m"): product.FromType(typ.Integer),
		}),
	)
	if len(got) != 3 || got[0].Name != Name("a") || got[1].Name != Name("m") || got[2].Name != Name("z") {
		t.Fatalf("MergeValueOverlay order = %+v, want a,m,z", got)
	}
	if !product.Equal(got[0].Value, product.CarryForward(leftA, rightA)) {
		t.Fatalf("merged a = %v, want CarryForward(string, boolean)", got[0].Value)
	}
}

func TestEqualValueOverlayNormalizesBeforeCompare(t *testing.T) {
	a := ValueOverlay{
		{Name: Name("z"), Value: product.FromType(typ.Number)},
		{Name: Name("a"), Value: product.FromType(typ.String)},
	}
	b := ValueOverlay{
		{Name: Name("a"), Value: product.FromType(typ.String)},
		{Name: Name("z"), Value: product.FromType(typ.Number)},
	}
	if !EqualValueOverlay(a, b) {
		t.Fatalf("EqualValueOverlay(%+v, %+v) = false, want true", a, b)
	}
}
