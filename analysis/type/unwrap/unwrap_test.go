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

func TestRecordAliasOnly(t *testing.T) {
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
			if got := unwrap.RecordAliasOnly(tt.t); got != tt.want {
				t.Fatalf("RecordAliasOnly() = %p, want %p", got, tt.want)
			}
		})
	}
}

func TestRecordUnaliased(t *testing.T) {
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
			if got := unwrap.RecordUnaliased(tt.t); got != tt.want {
				t.Fatalf("RecordUnaliased() = %p, want %p", got, tt.want)
			}
		})
	}
}

func TestRecordAliasPoliciesDifferAfterTargetMutation(t *testing.T) {
	original := typetable.NewRecord().Field("original", typ.String).Build()
	updated := typetable.NewRecord().Field("updated", typ.String).Build()
	alias := typ.NewAlias("Alias", typ.NewAlias("Inner", original))
	alias.Target = updated

	if got := unwrap.RecordAliasOnly(alias); got != updated {
		t.Fatalf("RecordAliasOnly() = %p, want updated %p", got, updated)
	}
	if got := unwrap.RecordUnaliased(alias); got != original {
		t.Fatalf("RecordUnaliased() = %p, want original %p", got, original)
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
