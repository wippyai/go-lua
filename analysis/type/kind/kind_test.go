package kind

import "testing"

func TestKindString(t *testing.T) {
	tests := []struct {
		k    Kind
		want string
	}{
		{Nil, "nil"},
		{Boolean, "boolean"},
		{Number, "number"},
		{Integer, "integer"},
		{String, "string"},
		{Any, "any"},
		{Unknown, "unknown"},
		{Never, "never"},
		{Optional, "optional"},
		{Union, "union"},
		{Function, "function"},
		{Array, "array"},
		{ReadonlyMap, "readonlymap"},
		{Record, "record"},
	}

	for _, tt := range tests {
		if got := tt.k.String(); got != tt.want {
			t.Errorf("Kind(%d).String() = %q, want %q", tt.k, got, tt.want)
		}
	}
}

func TestKindStringOutOfRange(t *testing.T) {
	tests := []Kind{
		Kind(-1),
		Kind(9999),
	}
	for _, k := range tests {
		if got := k.String(); got != "unknown" {
			t.Errorf("out of range Kind(%d).String() = %q, want %q", k, got, "unknown")
		}
	}
}

func TestKindValuesUnique(t *testing.T) {
	seen := make(map[Kind]string)
	kinds := []Kind{
		Nil, Boolean, Number, Integer, String, Any, Unknown, Never,
		Optional, Union, Intersection, Tuple, Function, Array, Map, Record,
		Interface, Alias, Generic, Instantiated, Literal,
		Self, Ref, Meta, TypeParam, Refined, Recursive,
		ReadonlyMap,
	}

	for _, k := range kinds {
		name := k.String()
		if existing, ok := seen[k]; ok {
			t.Errorf("duplicate Kind value: %d used for both %q and %q", k, existing, name)
		}

		seen[k] = name
	}
}

func TestKindNamesComplete(t *testing.T) {
	kinds := []Kind{
		Nil, Boolean, Number, Integer, String, Any, Unknown, Never,
		Optional, Union, Intersection, Tuple, Function, Array, Map, Record,
		Interface, Alias, Generic, Instantiated, Literal,
		Self, Ref, Meta, TypeParam, Refined, Recursive, ReadonlyMap,
	}

	for _, k := range kinds {
		name := k.String()
		if name == "" {
			t.Errorf("Kind(%d) has no name", k)
		}
	}
}

func TestKindIsPlaceholder(t *testing.T) {
	placeholders := []Kind{Any, Unknown}
	for _, k := range placeholders {
		if !k.IsPlaceholder() {
			t.Errorf("%s should be placeholder", k)
		}
	}

	nonPlaceholders := []Kind{Nil, Boolean, Number, Integer, String, Never, Union, Function, Array, Record, Ref}
	for _, k := range nonPlaceholders {
		if k.IsPlaceholder() {
			t.Errorf("%s should not be placeholder", k)
		}
	}
}

func TestKindIsNever(t *testing.T) {
	if !Never.IsNever() {
		t.Error("Never should return true for IsNever()")
	}

	nonNever := []Kind{Nil, Boolean, Number, Any, Unknown, Union, Function}
	for _, k := range nonNever {
		if k.IsNever() {
			t.Errorf("%s should not be Never", k)
		}
	}
}
