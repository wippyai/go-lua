package inspect

import (
	"testing"

	"github.com/wippyai/go-lua/domain/type/annotation"
	"github.com/wippyai/go-lua/domain/type/typ"
)

func TestVisit_AnnotatedDispatchesToInner(t *testing.T) {
	ann := typ.NewAnnotated(typ.NewMap(typ.String, typ.Any), []annotation.Annotation{{Name: "x"}})
	got := Visit(ann, Visitor[string]{
		Map: func(*typ.Map) string { return "map" },
		Default: func(typ.Type) string {
			return "default"
		},
	})
	if got != "map" {
		t.Fatalf("expected map visitor, got %q", got)
	}
}

func TestVisit_AnnotatedAliasDispatchesToAlias(t *testing.T) {
	alias := typ.NewAlias("T", typ.String)
	ann := typ.NewAnnotated(alias, []annotation.Annotation{{Name: "x"}})
	got := Visit(ann, Visitor[string]{
		Alias: func(*typ.Alias) string { return "alias" },
		Default: func(typ.Type) string {
			return "default"
		},
	})
	if got != "alias" {
		t.Fatalf("expected alias visitor, got %q", got)
	}
}

func TestVisit_AnnotatedNilInnerFallsBackToDefault(t *testing.T) {
	ann := &typ.Annotated{Inner: nil, Annotations: []annotation.Annotation{{Name: "x"}}}
	got := Visit(ann, Visitor[string]{
		Default: func(typ.Type) string {
			return "default"
		},
	})
	if got != "default" {
		t.Fatalf("expected default visitor, got %q", got)
	}
}
