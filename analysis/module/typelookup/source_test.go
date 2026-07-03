package typelookup

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/module/manifest"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestSourceResolveTypeRefUnqualifiedUniqueManifestType(t *testing.T) {
	m := manifest.New("result")
	m.DefineType("Result", typ.String)

	got, ok := (Source{Manifests: []*manifest.Manifest{m}}).ResolveTypeRef([]string{"Result"})
	if !ok || got != typ.String {
		t.Fatalf("ResolveTypeRef(Result) = %v/%v, want string", got, ok)
	}
}

func TestSourceResolveTypeRefUnqualifiedCollisionFailsClosed(t *testing.T) {
	left := manifest.New("left")
	left.DefineType("Result", typ.String)
	right := manifest.New("right")
	right.DefineType("Result", typ.Number)

	if got, ok := (Source{Manifests: []*manifest.Manifest{left, right}}).ResolveTypeRef([]string{"Result"}); ok {
		t.Fatalf("ResolveTypeRef(Result) = %v, want unresolved collision", got)
	}
}

func TestSourceResolveTypeRefWithModulePrefix(t *testing.T) {
	root := manifest.New("app.store")
	root.DefineType("Record", typ.String)
	nested := manifest.New("app.store.schema")
	nested.DefineType("Record", typ.Number)
	source := Source{Manifests: []*manifest.Manifest{root, nested}}

	got, ok := source.ResolveTypeRefWithModulePrefix("app.store", []string{"Record"})
	if !ok || got != typ.String {
		t.Fatalf("ResolveTypeRefWithModulePrefix(app.store, Record) = %v/%v, want string", got, ok)
	}
	got, ok = source.ResolveTypeRefWithModulePrefix("app.store", []string{"schema", "Record"})
	if !ok || got != typ.Number {
		t.Fatalf("ResolveTypeRefWithModulePrefix(app.store, schema.Record) = %v/%v, want number", got, ok)
	}
	if got, ok := source.ResolveTypeRefWithModulePrefix("", []string{"Record"}); ok || got != nil {
		t.Fatalf("ResolveTypeRefWithModulePrefix(empty prefix) = %v/%v, want unresolved", got, ok)
	}
	if got, ok := source.ResolveTypeRefWithModulePrefix("app.store", nil); ok || got != nil {
		t.Fatalf("ResolveTypeRefWithModulePrefix(empty suffix) = %v/%v, want unresolved", got, ok)
	}
}
