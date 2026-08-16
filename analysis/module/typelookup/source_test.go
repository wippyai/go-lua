package typelookup

import (
	"testing"

	typetable "github.com/wippyai/go-lua/analysis/domain/type/table"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
	"github.com/wippyai/go-lua/analysis/module/manifest"
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

func TestSourceResolveTypeRefUsesSelectedRequireAlias(t *testing.T) {
	m := manifest.New("app.store")
	m.DefineType("Record", typ.String)
	source := Source{Manifests: []*manifest.Manifest{m}, Aliases: map[string]string{"store_mod": "app.store"}}
	got, ok := source.ResolveTypeRef([]string{"store_mod", "Record"})
	if !ok || !typ.TypeEquals(got, typ.String) {
		t.Fatalf("ResolveTypeRef(store_mod.Record) = %v/%v, want string", got, ok)
	}
}

func TestSourceLookupResolvesNestedBareReferenceInOwningManifest(t *testing.T) {
	entry := typetable.NewRecord().Field("id", typ.String).Build()
	registry := manifest.New("registry")
	registry.DefineType("Entry", entry)
	registry.DefineType("Page", typetable.NewRecord().Field("entry", typ.NewRef("", "Entry")).Build())
	fs := manifest.New("fs")
	fs.DefineType("Entry", typetable.NewRecord().Field("name", typ.String).Build())

	got, ok := (Source{Manifests: []*manifest.Manifest{registry, fs}}).Lookup("registry", "Page")
	if !ok {
		t.Fatal("Lookup(registry, Page) missing")
	}
	page, ok := got.(*typ.Record)
	if !ok || page.GetField("entry") == nil {
		t.Fatalf("Lookup(registry, Page) = %T %[1]v, want record with entry", got)
	}
	if !typ.TypeEquals(page.GetField("entry").Type, entry) {
		t.Fatalf("registry Page.entry = %v, want registry Entry %v", page.GetField("entry").Type, entry)
	}
}
