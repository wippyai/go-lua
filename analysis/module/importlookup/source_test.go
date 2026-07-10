package importlookup

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/module/manifest"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestSourceLookupExportExactPathOnly(t *testing.T) {
	provider := manifest.New("provider")
	provider.SetExport(typ.String)
	fooProvider := manifest.New("foo.provider")
	fooProvider.SetExport(typ.Number)

	source := Source{Manifests: []*manifest.Manifest{provider, fooProvider}}
	if got, ok := source.LookupExport("provider"); !ok || got != typ.String {
		t.Fatalf("LookupExport(provider) = %v/%v, want string/true", got, ok)
	}
	if got, ok := source.LookupExport("foo.provider"); !ok || got != typ.Number {
		t.Fatalf("LookupExport(foo.provider) = %v/%v, want number/true", got, ok)
	}
	if _, ok := source.LookupExport("bar.provider"); ok {
		t.Fatal("LookupExport(bar.provider) succeeded, want exact-path miss")
	}
}

func TestSourceLookupExportLaterManifestOverridesEarlier(t *testing.T) {
	first := manifest.New("provider")
	first.SetExport(typ.String)
	second := manifest.New("provider")
	second.SetExport(typ.Number)

	source := Source{Manifests: []*manifest.Manifest{first, second}}
	if got, ok := source.LookupExport("provider"); !ok || got != typ.Number {
		t.Fatalf("LookupExport(provider) = %v/%v, want later number/true", got, ok)
	}
}

func TestSourceLookupExportResolvesBareReferenceInOwningManifest(t *testing.T) {
	entry := typetable.NewRecord().Field("id", typ.String).Build()
	registry := manifest.New("registry")
	registry.DefineType("Entry", entry)
	registry.SetExport(typetable.NewRecord().Field("get", typ.Func().Returns(typ.NewRef("", "Entry")).Build()).Build())
	fs := manifest.New("fs")
	fs.DefineType("Entry", typetable.NewRecord().Field("name", typ.String).Build())

	got, ok := (Source{Manifests: []*manifest.Manifest{registry, fs}}).LookupExport("registry")
	if !ok {
		t.Fatal("LookupExport(registry) missing")
	}
	export, ok := got.(*typ.Record)
	if !ok || export.GetField("get") == nil {
		t.Fatalf("LookupExport(registry) = %T %[1]v, want record with get", got)
	}
	fn, ok := export.GetField("get").Type.(*typ.Function)
	if !ok || len(fn.Returns) != 1 || !typ.TypeEquals(fn.Returns[0], entry) {
		t.Fatalf("registry get = %v, want return registry Entry %v", export.GetField("get").Type, entry)
	}
}
