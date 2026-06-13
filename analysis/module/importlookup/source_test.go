package importlookup

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/module/manifest"
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
