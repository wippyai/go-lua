package signaturelookup

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/effect"
	"github.com/wippyai/go-lua/analysis/domain/effect/dispatch"
	"github.com/wippyai/go-lua/analysis/domain/effect/lifecycle"
	"github.com/wippyai/go-lua/analysis/domain/effect/returns"
	"github.com/wippyai/go-lua/analysis/domain/typestate"
	"github.com/wippyai/go-lua/analysis/module/manifest"
	"github.com/wippyai/go-lua/analysis/module/signature"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup/internal/stdlib"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typeexpr"
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
	want := testSignature("custom", returns.ErrorReturn{ValueIndex: 0, ErrorIndex: 1})
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

func TestLookupManifestLocalMemberByQualifiedModulePath(t *testing.T) {
	want := testSignature("get", returns.ErrorReturn{ValueIndex: 0, ErrorIndex: 1})
	other := testSignature("other", returns.Return{ReturnIndex: 0, Transform: returns.SameAs{Source: effect.ParamRef{Index: 0}}})
	sql := manifest.New("sql")
	sql.DefineFunctionSignature("get", want)
	json := manifest.New("json")
	json.DefineFunctionSignature("get", other)
	src := Source{Manifests: []*manifest.Manifest{json, sql}}

	got, ok := src.Lookup("sql.get")
	if !ok {
		t.Fatal("Lookup(sql.get) missing")
	}
	if !want.Equals(got) {
		t.Fatalf("Lookup(sql.get) = %v, want sql-local get %v", got, want)
	}
}

func TestLookupManifestLocalStaticIntMemberByQualifiedModulePath(t *testing.T) {
	want := testSignature("[1]", returns.ErrorReturn{ValueIndex: 0, ErrorIndex: 1})
	m := manifest.New("pkg")
	m.DefineFunctionSignature("[1]", want)
	src := Source{Manifests: []*manifest.Manifest{m}}

	got, ok := src.Lookup("pkg[1]")
	if !ok {
		t.Fatal("Lookup(pkg[1]) missing")
	}
	if !want.Equals(got) {
		t.Fatalf("Lookup(pkg[1]) = %v, want pkg-local [1] %v", got, want)
	}
}

func TestLookupDerivesManifestInterfaceMethodTypeWithoutErrorType(t *testing.T) {
	instanceType := typ.NewInterface("contract.Instance", nil)
	openType := typ.Func().
		Param("self", typ.Self).
		Returns(instanceType, typeexpr.Optional(typ.String)).
		Build()
	contractType := typ.NewInterface("contract.Contract", []typ.Method{{Name: "open", Type: openType}})
	m := manifest.New("contract")
	m.DefineType("Contract", contractType)
	src := Source{Manifests: []*manifest.Manifest{m}}

	got, ok := src.Lookup("contract.Contract.open")
	if !ok {
		t.Fatal("Lookup(contract.Contract.open) missing")
	}
	if got.Type == nil || !typ.TypeEquals(got.Type, openType) {
		t.Fatalf("Lookup(contract.Contract.open).Type = %v, want %v", got.Type, openType)
	}
	if len(got.Effect.Labels) != 0 {
		t.Fatalf("Lookup(contract.Contract.open).Effect = %v, want no derived labels without ErrorType", got.Effect)
	}
}

func TestLookupManifestOverridesStdlib(t *testing.T) {
	want := testSignature("override", returns.ErrorReturn{ValueIndex: 0, ErrorIndex: 1})
	m := manifest.New("")
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

func TestLookupModuleLocalBareStdlibNameDoesNotOverrideStdlibGlobal(t *testing.T) {
	local := testSignature("channel_select", returns.Return{ReturnIndex: 0, Transform: returns.SameAs{Source: effect.ParamRef{Index: 0}}})
	m := manifest.New("channel")
	m.DefineFunctionSignature("select", local)
	src := Source{Manifests: []*manifest.Manifest{m}, IncludeStdlib: true}

	global, ok := src.Lookup("select")
	if !ok {
		t.Fatal("Lookup(select) missing")
	}
	if local.Equals(global) {
		t.Fatalf("Lookup(select) used module-local channel.select signature for bare stdlib global")
	}

	qualified, ok := src.Lookup("channel.select")
	if !ok {
		t.Fatal("Lookup(channel.select) missing")
	}
	if !local.Equals(qualified) {
		t.Fatalf("Lookup(channel.select) = %v, want module-local %v", qualified, local)
	}
}

func TestLookupMissing(t *testing.T) {
	src := Source{IncludeStdlib: true}

	if got, ok := src.Lookup("not.a.function"); ok {
		t.Fatalf("Lookup(not.a.function) = %v, want missing", got)
	}
}

func TestSourceValidateRejectsLifecycleManifestWithoutFSM(t *testing.T) {
	m := manifest.New("example/lifecycle")
	m.DefineFunctionSignature("begin", signature.Function{
		Type: typ.Func().Param("tx", typ.Any).Build(),
		Effect: effect.Empty.With(lifecycle.Acquire{
			Target:   effect.ParamRef{Index: 0},
			Protocol: "transaction",
			State:    "active",
		}),
	})
	err := (Source{Manifests: []*manifest.Manifest{m}}).Validate()
	if err == nil || !strings.Contains(err.Error(), `signature manifest "example/lifecycle"`) ||
		!strings.Contains(err.Error(), `lifecycle protocol "transaction" is not declared as a typestate FSM`) {
		t.Fatalf("Validate error = %v, want undeclared FSM", err)
	}
}

func TestSourceValidateAcceptsDeclaredLifecycleFSM(t *testing.T) {
	m := manifest.New("example/lifecycle")
	if err := m.DefineTypestateProtocol(typestate.Definition{
		Protocol:    "transaction",
		States:      []typestate.State{"active", "finished"},
		FinalStates: []typestate.State{"finished"},
		Transitions: []typestate.TransitionDecl{{From: "active", To: "finished"}},
	}); err != nil {
		t.Fatalf("DefineTypestateProtocol: %v", err)
	}
	m.DefineFunctionSignature("finish", signature.Function{
		Type: typ.Func().Param("tx", typ.Any).Build(),
		Effect: effect.Empty.With(lifecycle.Transition{
			Target:   effect.ParamRef{Index: 0},
			Protocol: "transaction",
			From:     "active",
			To:       "finished",
		}),
	})
	if err := (Source{Manifests: []*manifest.Manifest{m}}).Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestLookupReturnsClones(t *testing.T) {
	want := testSignature("custom", returns.ErrorReturn{ValueIndex: 0, ErrorIndex: 1})
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
	first.DefineFunctionSignature("shared", testSignature("first", returns.ErrorReturn{ValueIndex: 0, ErrorIndex: 1}))
	second := manifest.New("example/second")
	wantShared := testSignature("second", returns.Return{ReturnIndex: 0, Transform: returns.SameAs{Source: effect.ParamRef{Index: 0}}})
	wantRequire := testSignature("require_override", returns.ErrorReturn{ValueIndex: 0, ErrorIndex: 1})
	second.DefineFunctionSignature("shared", wantShared)
	global := manifest.New("")
	global.DefineFunctionSignature(stdlib.Require, wantRequire)
	src := Source{Manifests: []*manifest.Manifest{first, second, global}, IncludeStdlib: true}

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

func TestSignaturesDoesNotPublishModuleLocalBareStdlibNameAsGlobal(t *testing.T) {
	local := testSignature("channel_select", returns.Return{ReturnIndex: 0, Transform: returns.SameAs{Source: effect.ParamRef{Index: 0}}})
	m := manifest.New("channel")
	m.DefineFunctionSignature("select", local)
	src := Source{Manifests: []*manifest.Manifest{m}, IncludeStdlib: true}

	all := src.Signatures()
	global, ok := all["select"]
	if !ok {
		t.Fatal("Signatures() missing stdlib select")
	}
	if local.Equals(global) {
		t.Fatalf("Signatures()[select] used module-local channel.select signature for bare stdlib global")
	}
	if qualified, ok := all["channel.select"]; ok {
		t.Fatalf("Signatures() published synthesized qualified local %v; bulk map should expose stored global names only", qualified)
	}
}

func TestStdlibSignatureNamesExposeNamesWithoutSignatureMaterialization(t *testing.T) {
	names := StdlibSignatureNames()
	if len(names) == 0 {
		t.Fatal("StdlibSignatureNames returned no names")
	}
	seen := map[string]bool{}
	for _, name := range names {
		seen[name] = true
	}
	for _, name := range []string{stdlib.Require, stdlib.Type, stdlib.TableInsert} {
		if !seen[name] {
			t.Fatalf("StdlibSignatureNames missing %q", name)
		}
	}

	names[0] = "mutated"
	again := StdlibSignatureNames()
	for _, name := range again {
		if name == "mutated" {
			t.Fatal("StdlibSignatureNames returned aliased storage")
		}
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
