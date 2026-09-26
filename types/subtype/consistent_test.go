package subtype

import (
	"testing"

	"github.com/wippyai/go-lua/types/typ"
)

func TestIsConsistentSubtype_AnyIsConsistentWithEveryType(t *testing.T) {
	anyMap := typ.NewMap(typ.String, typ.Any)
	intMap := typ.NewMap(typ.String, typ.Integer)
	rec := typ.NewRecord().Field("id", typ.String).Build()
	cases := []struct {
		name       string
		sub, super typ.Type
	}{
		{"any to string", typ.Any, typ.String},
		{"string to any", typ.String, typ.Any},
		{"any to record", typ.Any, rec},
		{"any-valued map to integer map", anyMap, intMap},
		{"integer map to any-valued map", intMap, anyMap},
		{"record with any field to record", typ.NewRecord().Field("id", typ.Any).Build(), rec},
		{"array of any to array of string", typ.NewArray(typ.Any), typ.NewArray(typ.String)},
		{"function over any", typ.Func().Param("x", typ.Any).Returns(typ.Any).Build(), typ.Func().Param("x", typ.String).Returns(typ.String).Build()},
		{"optional any to optional string", typ.NewOptional(typ.Any), typ.NewOptional(typ.String)},
	}
	for _, c := range cases {
		if !IsConsistentSubtype(c.sub, c.super) {
			t.Errorf("%s: %s must be consistent with %s", c.name, c.sub, c.super)
		}
	}
}

func TestIsConsistentSubtype_KeepsStructureOutsideAny(t *testing.T) {
	cases := []struct {
		name       string
		sub, super typ.Type
	}{
		{"unknown to string", typ.Unknown, typ.String},
		{"string to number", typ.String, typ.Number},
		{"optional string to string", typ.NewOptional(typ.String), typ.String},
		{"record missing a field", typ.NewRecord().Field("id", typ.Any).Build(), typ.NewRecord().Field("id", typ.String).Field("name", typ.String).Build()},
		{"string map to integer map", typ.NewMap(typ.String, typ.String), typ.NewMap(typ.String, typ.Integer)},
	}
	for _, c := range cases {
		if IsConsistentSubtype(c.sub, c.super) {
			t.Errorf("%s: %s must not be consistent with %s", c.name, c.sub, c.super)
		}
	}
}

func TestAssignability_StrictAnyTreatsAnyAsUnknown(t *testing.T) {
	if StrictAny.Assignable(typ.Any, typ.String) {
		t.Error("strict any must not be assignable to string")
	}
	if StrictAny.Assignable(typ.Unknown, typ.String) != Gradual.Assignable(typ.Unknown, typ.String) {
		t.Error("unknown must behave the same under both modes")
	}
	if !Gradual.Assignable(typ.Any, typ.String) {
		t.Error("gradual any must be assignable to string")
	}
	if !StrictAny.Assignable(typ.String, typ.Any) {
		t.Error("every type is assignable to any")
	}
}
