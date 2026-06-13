package unwrap_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/type/annotation"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

func TestAlias(t *testing.T) {
	t.Run("preserves optional", func(t *testing.T) {
		opt := typ.NewOptional(typ.String)
		alias := typ.NewAlias("OptString", opt)
		result := unwrap.Alias(alias)
		if _, ok := result.(*typ.Optional); !ok {
			t.Error("Alias should preserve Optional")
		}
	})
}

func TestNormalizeNil(t *testing.T) {
	t.Run("nil interface", func(t *testing.T) {
		if got := unwrap.NormalizeNil(nil); got != nil {
			t.Fatalf("NormalizeNil(nil) = %T, want nil", got)
		}
	})

	t.Run("typed nil", func(t *testing.T) {
		var arr *typ.Array
		var input typ.Type = arr
		if got := unwrap.NormalizeNil(input); got != nil {
			t.Fatalf("NormalizeNil(typed nil) = %T, want nil", got)
		}
	})

	t.Run("non-nil", func(t *testing.T) {
		if got := unwrap.NormalizeNil(typ.String); got != typ.String {
			t.Fatalf("NormalizeNil(string) = %T, want string", got)
		}
	})
}

func TestAnnotatedAndAnnotations(t *testing.T) {
	inner := typ.NewAnnotated(typ.Number, []annotation.Annotation{{Name: "max", Arg: float64(100)}})
	outer := typ.NewAnnotated(inner, []annotation.Annotation{{Name: "min", Arg: float64(0)}})

	t.Run("Annotated removes one layer", func(t *testing.T) {
		if got := unwrap.Annotated(outer); got != inner {
			t.Fatalf("Annotated(outer) = %T, want %T", got, inner)
		}
	})

	t.Run("Annotations removes all layers", func(t *testing.T) {
		if got := unwrap.Annotations(outer); got != typ.Number {
			t.Fatalf("Annotations(outer) = %T, want number", got)
		}
	})

	t.Run("preserves non-annotated values", func(t *testing.T) {
		if got := unwrap.Annotated(typ.Number); got != typ.Number {
			t.Fatalf("Annotated(number) = %T, want number", got)
		}
		if got := unwrap.Annotations(typ.Number); got != typ.Number {
			t.Fatalf("Annotations(number) = %T, want number", got)
		}
	})
}

func TestOptional(t *testing.T) {
	optional := typ.NewOptional(typ.String)
	aliasToOptional := typ.NewAlias("OptString", optional)
	aliasToNil := typ.NewAlias("NilAlias", typ.Nil)
	aliasAroundOptional := typ.NewAlias("AliasOpt", optional)

	t.Run("nested optionals", func(t *testing.T) {
		outer := typ.NewOptional(optional)
		if got := unwrap.Optional(outer); got != typ.String {
			t.Fatalf("Optional(nested optional) = %T, want string", got)
		}
	})

	t.Run("unwraps aliases to optionals", func(t *testing.T) {
		if got := unwrap.Optional(aliasToOptional); got != typ.String {
			t.Fatalf("Optional(alias to optional) = %T, want string", got)
		}
	})

	t.Run("unwraps optional around alias to optional", func(t *testing.T) {
		wrapped := typ.NewOptional(aliasAroundOptional)
		if got := unwrap.Optional(wrapped); got != typ.String {
			t.Fatalf("Optional(optional around alias) = %T, want string", got)
		}
	})

	t.Run("returns nil for nil-like inputs", func(t *testing.T) {
		if got := unwrap.Optional(nil); got != nil {
			t.Fatalf("Optional(nil) = %T, want nil", got)
		}
		if got := unwrap.Optional(typ.Nil); got != typ.Nil {
			t.Fatalf("Optional(Nil) = %T, want Nil", got)
		}
		if got := unwrap.Optional(aliasToNil); got != typ.Nil {
			t.Fatalf("Optional(alias to nil) = %T, want Nil", got)
		}
	})
}

func TestRecordWithAliasPolicyTarget(t *testing.T) {
	rec := typetable.NewRecord().Field("id", typ.String).Build()
	nested := typ.NewAlias("Outer", typ.NewAlias("Inner", rec))
	annotated := typ.NewAnnotated(rec, []annotation.Annotation{{Name: "brand"}})

	tests := []struct {
		name string
		t    typ.Type
		want *typ.Record
	}{
		{"record", rec, rec},
		{"alias to record", typ.NewAlias("User", rec), rec},
		{"nested alias to record", nested, rec},
		{"nil", nil, nil},
		{"non-record", typ.String, nil},
		{"alias to non-record", typ.NewAlias("Name", typ.String), nil},
		{"annotated record is not unwrapped", annotated, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := unwrap.RecordWithAliasPolicy(tt.t, unwrap.RecordAliasTarget); got != tt.want {
				t.Fatalf("RecordWithAliasPolicy(RecordAliasTarget) = %p, want %p", got, tt.want)
			}
		})
	}
}

func TestRecordWithAliasPolicyUnaliasedTarget(t *testing.T) {
	rec := typetable.NewRecord().Field("id", typ.String).Build()
	nested := typ.NewAlias("Outer", typ.NewAlias("Inner", rec))
	annotated := typ.NewAnnotated(rec, []annotation.Annotation{{Name: "brand"}})

	tests := []struct {
		name string
		t    typ.Type
		want *typ.Record
	}{
		{"record", rec, rec},
		{"alias to record", typ.NewAlias("User", rec), rec},
		{"nested alias to record", nested, rec},
		{"nil", nil, nil},
		{"non-record", typ.String, nil},
		{"alias to non-record", typ.NewAlias("Name", typ.String), nil},
		{"annotated record is not unwrapped", annotated, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := unwrap.RecordWithAliasPolicy(tt.t, unwrap.RecordAliasUnaliasedTarget); got != tt.want {
				t.Fatalf("RecordWithAliasPolicy(RecordAliasUnaliasedTarget) = %p, want %p", got, tt.want)
			}
		})
	}
}

func TestRecordAliasPoliciesDifferAfterTargetMutation(t *testing.T) {
	original := typetable.NewRecord().Field("original", typ.String).Build()
	updated := typetable.NewRecord().Field("updated", typ.String).Build()
	alias := typ.NewAlias("Alias", typ.NewAlias("Inner", original))
	alias.Target = updated

	if got := unwrap.RecordWithAliasPolicy(alias, unwrap.RecordAliasTarget); got != updated {
		t.Fatalf("RecordWithAliasPolicy(RecordAliasTarget) = %p, want updated %p", got, updated)
	}
	if got := unwrap.RecordWithAliasPolicy(alias, unwrap.RecordAliasUnaliasedTarget); got != original {
		t.Fatalf("RecordWithAliasPolicy(RecordAliasUnaliasedTarget) = %p, want original %p", got, original)
	}
}

func TestIsOptionalLike(t *testing.T) {
	tests := []struct {
		name string
		t    typ.Type
		want bool
	}{
		{"nil type", nil, true},
		{"Nil", typ.Nil, true},
		{"Optional", typ.NewOptional(typ.String), true},
		{"Union with nil", typ.NewUnion(typ.String, typ.Nil), true},
		{"String", typ.String, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := unwrap.IsOptionalLike(tt.t); got != tt.want {
				t.Errorf("IsOptionalLike() = %v, want %v", got, tt.want)
			}
		})
	}
}
