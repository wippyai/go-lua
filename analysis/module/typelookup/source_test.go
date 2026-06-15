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
