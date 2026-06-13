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

func TestAnnotated(t *testing.T) {
	t.Run("unwraps one annotation layer", func(t *testing.T) {
		ann := typ.NewAnnotated(typ.Number, []annotation.Annotation{{Name: "min", Arg: float64(0)}})
		if got := unwrap.Annotated(ann); got != typ.Number {
			t.Fatalf("Annotated() = %T, want number", got)
		}
	})

	t.Run("preserves non-annotated values", func(t *testing.T) {
		if got := unwrap.Annotated(typ.Number); got != typ.Number {
			t.Fatalf("Annotated() = %T, want number", got)
		}
	})
}

func TestAnnotations(t *testing.T) {
	inner := typ.NewAnnotated(typ.Number, []annotation.Annotation{{Name: "max", Arg: float64(100)}})
	outer := typ.NewAnnotated(inner, []annotation.Annotation{{Name: "min", Arg: float64(0)}})

	if got := unwrap.Annotations(outer); got != typ.Number {
		t.Fatalf("Annotations() should strip nested wrappers, got %T", got)
	}
	if got := unwrap.Annotations(typ.Number); got != typ.Number {
		t.Fatalf("Annotations() on non-annotated should return same type, got %T", got)
	}
}

func TestOptional(t *testing.T) {
	t.Run("nested", func(t *testing.T) {
		inner := typ.NewOptional(typ.String)
		outer := typ.NewOptional(inner)
		result := unwrap.Optional(outer)
		if result != typ.String {
			t.Error("Optional should fully unwrap nested optionals")
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
		{"Any", typ.Any, true},
		{"Unknown", typ.Unknown, true},
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
