package signaturelookup

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/effect"
	"github.com/wippyai/go-lua/analysis/domain/effect/control"
	"github.com/wippyai/go-lua/analysis/domain/effect/dispatch"
	"github.com/wippyai/go-lua/analysis/domain/effect/signature"
	"github.com/wippyai/go-lua/analysis/module/manifest"
	"github.com/wippyai/go-lua/analysis/module/stdlib"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestLookupStdlib(t *testing.T) {
	src := Source{IncludeStdlib: true}

	got, ok := src.Lookup(stdlib.Require)
	if !ok {
		t.Fatalf("Lookup(%q) missing", stdlib.Require)
	}
	if !hasLabel(got.Effect, dispatch.ModuleLoad{}) {
		t.Fatalf("Lookup(%q) effect = %v, want module load", stdlib.Require, got.Effect)
	}
}

func TestLookupManifest(t *testing.T) {
	want := testSignature("custom", control.IO{})
	m := manifest.New("example/module")
	m.DefineFunctionSignature("custom", want)
	src := Source{Manifests: []*manifest.Manifest{m}}

	got, ok := src.Lookup("custom")
	if !ok {
		t.Fatal("Lookup(custom) missing")
	}
	if !want.Equals(got) {
		t.Fatalf("Lookup(custom) = %v, want %v", got, want)
	}
}

func TestLookupManifestOverridesStdlib(t *testing.T) {
	want := testSignature("override", control.IO{})
	m := manifest.New("example/module")
	m.DefineFunctionSignature(stdlib.Require, want)
	src := Source{Manifests: []*manifest.Manifest{m}, IncludeStdlib: true}

	got, ok := src.Lookup(stdlib.Require)
	if !ok {
		t.Fatalf("Lookup(%q) missing", stdlib.Require)
	}
	if !want.Equals(got) {
		t.Fatalf("Lookup(%q) = %v, want manifest override %v", stdlib.Require, got, want)
	}
}

func TestLookupMissing(t *testing.T) {
	src := Source{IncludeStdlib: true}

	if got, ok := src.Lookup("not.a.function"); ok {
		t.Fatalf("Lookup(not.a.function) = %v, want missing", got)
	}
}

func TestLookupReturnsClones(t *testing.T) {
	want := testSignature("custom", control.IO{})
	m := manifest.New("example/module")
	m.DefineFunctionSignature("custom", want)
	src := Source{Manifests: []*manifest.Manifest{m}, IncludeStdlib: true}

	first, ok := src.Lookup("custom")
	if !ok {
		t.Fatal("Lookup(custom) missing")
	}
	first.Type.Params[0].Name = "changed"
	first.Effect.Labels = nil

	second, ok := src.Lookup("custom")
	if !ok {
		t.Fatal("Lookup(custom) missing after local mutation")
	}
	if second.Type.Params[0].Name != "custom_arg" {
		t.Fatalf("Lookup returned aliased function params: %q", second.Type.Params[0].Name)
	}
	if len(second.Effect.Labels) == 0 {
		t.Fatal("Lookup returned aliased effect labels")
	}

	std, ok := src.Lookup(stdlib.Type)
	if !ok {
		t.Fatalf("Lookup(%q) missing", stdlib.Type)
	}
	std.Type.Params[0].Name = "changed"
	std.Effect.Labels = nil

	stdAgain, ok := src.Lookup(stdlib.Type)
	if !ok {
		t.Fatalf("Lookup(%q) missing after local mutation", stdlib.Type)
	}
	if stdAgain.Type.Params[0].Name != "v" {
		t.Fatalf("stdlib lookup returned aliased function params: %q", stdAgain.Type.Params[0].Name)
	}
	if len(stdAgain.Effect.Labels) == 0 {
		t.Fatal("stdlib lookup returned aliased effect labels")
	}
}

func TestSignaturesReturnsClonesAndRespectsPrecedence(t *testing.T) {
	first := manifest.New("example/first")
	first.DefineFunctionSignature("shared", testSignature("first", control.IO{}))
	second := manifest.New("example/second")
	wantShared := testSignature("second", dispatch.TypePredicate{})
	wantRequire := testSignature("require_override", control.IO{})
	second.DefineFunctionSignature("shared", wantShared)
	second.DefineFunctionSignature(stdlib.Require, wantRequire)
	src := Source{Manifests: []*manifest.Manifest{first, second}, IncludeStdlib: true}

	all := src.Signatures()
	if got := all["shared"]; !wantShared.Equals(got) {
		t.Fatalf("Signatures()[shared] = %v, want later manifest %v", got, wantShared)
	}
	if got := all[stdlib.Require]; !wantRequire.Equals(got) {
		t.Fatalf("Signatures()[%s] = %v, want manifest override %v", stdlib.Require, got, wantRequire)
	}
	if _, ok := all[stdlib.Type]; !ok {
		t.Fatalf("Signatures() missing stdlib %q", stdlib.Type)
	}

	shared := all["shared"]
	shared.Type.Params[0].Name = "changed"
	shared.Effect.Labels = nil

	again := src.Signatures()
	if again["shared"].Type.Params[0].Name != "second_arg" {
		t.Fatalf("Signatures returned aliased function params: %q", again["shared"].Type.Params[0].Name)
	}
	if len(again["shared"].Effect.Labels) == 0 {
		t.Fatal("Signatures returned aliased effect labels")
	}
}

func TestTypePackageDoesNotImportEffect(t *testing.T) {
	root := filepath.Clean("../../type/typ")
	fset := token.NewFileSet()
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || filepath.Ext(path) != ".go" {
			return nil
		}
		file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, imp := range file.Imports {
			path := strings.Trim(imp.Path.Value, `"`)
			if path == "github.com/wippyai/go-lua/analysis/domain/effect" ||
				strings.HasPrefix(path, "github.com/wippyai/go-lua/analysis/domain/effect/") {
				t.Fatalf("%s imports analysis/domain/effect", path)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk typ package: %v", err)
	}
}

func testSignature(name string, labels ...effect.Label) signature.Function {
	return signature.Function{
		Type: typ.Func().
			Param(name+"_arg", typ.String).
			Returns(typ.String).
			Build(),
		Effect: effect.Row{Labels: labels},
	}
}

func hasLabel(row effect.Row, want effect.Label) bool {
	return row.Has(func(got effect.Label) bool {
		return want.Equals(got)
	})
}
