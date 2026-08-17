package unwrap_test

import (
	"testing"

	"github.com/wippyai/go-lua/domain/type/annotation"
	typetable "github.com/wippyai/go-lua/domain/type/table"
	"github.com/wippyai/go-lua/domain/type/typ"
	"github.com/wippyai/go-lua/domain/type/typeexpr"
	"github.com/wippyai/go-lua/domain/type/unwrap"
)

func TestAlias(t *testing.T) {
	t.Run("preserves optional", func(t *testing.T) {
		opt := typeexpr.Optional(typ.String)
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
	inner := typ.NewAnnotated(typ.Number, []annotation.Annotation{{Name: "max", Arg: annotation.Float64Arg(100)}})
	outer := typ.NewAnnotated(inner, []annotation.Annotation{{Name: "min", Arg: annotation.Float64Arg(0)}})

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

func TestAnnotationsStopsOnAnnotationCycle(t *testing.T) {
	a, b := annotationCycle()

	got := unwrap.Annotations(a)
	if got != a && got != b {
		t.Fatalf("Annotations(annotation cycle) = %T, want one cycle member", got)
	}
}

func TestOptional(t *testing.T) {
	optional := typeexpr.Optional(typ.String)
	aliasToOptional := typ.NewAlias("OptString", optional)
	aliasToNil := typ.NewAlias("NilAlias", typ.Nil)
	aliasAroundOptional := typ.NewAlias("AliasOpt", optional)

	t.Run("nested optionals", func(t *testing.T) {
		outer := typeexpr.Optional(optional)
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
		wrapped := typeexpr.Optional(aliasAroundOptional)
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

func TestAliasStopsOnAnnotationCycle(t *testing.T) {
	a, b := annotationCycle()

	got := unwrap.Alias(a)
	if got != a && got != b {
		t.Fatalf("Alias(annotation cycle) = %T, want one cycle member", got)
	}
}

func TestOptionalStopsOnAnnotationCycle(t *testing.T) {
	a, b := annotationCycle()

	got := unwrap.Optional(a)
	if got != a && got != b {
		t.Fatalf("Optional(annotation cycle) = %T, want one cycle member", got)
	}
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

func TestRecordWithAliasPolicyRejectsTargetCycle(t *testing.T) {
	original := typetable.NewRecord().Field("original", typ.String).Build()
	alias := typ.NewAlias("Alias", original)
	alias.Target = alias

	if got := unwrap.RecordWithAliasPolicy(alias, unwrap.RecordAliasTarget); got != nil {
		t.Fatalf("RecordWithAliasPolicy(RecordAliasTarget cycle) = %p, want nil", got)
	}
	if got := unwrap.RecordWithAliasPolicy(alias, unwrap.RecordAliasUnaliasedTarget); got != original {
		t.Fatalf("RecordWithAliasPolicy(RecordAliasUnaliasedTarget cycle) = %p, want original %p", got, original)
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
		{"Optional", typeexpr.Optional(typ.String), true},
		{"Union with nil", typeexpr.Union(typ.String, typ.Nil), true},
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

func TestIsOptionalLikeHandlesUnionCycle(t *testing.T) {
	u := &typ.Union{}
	u.Members = []typ.Type{u}

	if unwrap.IsOptionalLike(u) {
		t.Fatal("self-recursive union without nil should not be optional-like")
	}
}

func TestIsOptionalLikeFindsNilAfterUnionCycle(t *testing.T) {
	u := &typ.Union{}
	u.Members = []typ.Type{u, typ.Nil}

	if !unwrap.IsOptionalLike(u) {
		t.Fatal("self-recursive union with nil should be optional-like")
	}
}

func TestIsOptionalLikeRejectsAnnotationCycle(t *testing.T) {
	a, _ := annotationCycle()

	if unwrap.IsOptionalLike(a) {
		t.Fatal("annotation cycle without nil should not be optional-like")
	}
}

func TestShallowWrapperTraversalDoesNotAllocate(t *testing.T) {
	alias := typ.NewAlias("Text", typ.String)
	annotated := typ.NewAnnotated(typ.String, []annotation.Annotation{{Name: "tag"}})
	if got := testing.AllocsPerRun(1000, func() {
		_ = unwrap.Alias(alias)
	}); got != 0 {
		t.Fatalf("Alias shallow allocations = %v, want 0", got)
	}
	if got := testing.AllocsPerRun(1000, func() {
		_ = unwrap.Annotations(annotated)
	}); got != 0 {
		t.Fatalf("Annotations shallow allocations = %v, want 0", got)
	}
}

func annotationCycle() (*typ.Annotated, *typ.Annotated) {
	a := &typ.Annotated{}
	b := &typ.Annotated{}
	a.Inner = b
	b.Inner = a
	return a, b
}
