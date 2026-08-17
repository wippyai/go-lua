package stdlib

import (
	"testing"

	"github.com/wippyai/go-lua/domain/effect/control"
	"github.com/wippyai/go-lua/domain/effect/mutation"
	"github.com/wippyai/go-lua/domain/effect/ownership"
	"github.com/wippyai/go-lua/domain/type/typ"
	declarations "github.com/wippyai/go-lua/manifest"
	modulemanifest "github.com/wippyai/go-lua/manifest/wire"
)

func TestProvidersHaveExactCatalogueCoverageAndFreshResults(t *testing.T) {
	providers := Providers()
	if len(providers) != len(catalogue) {
		t.Fatalf("manifest providers = %d, catalogue = %d", len(providers), len(catalogue))
	}
	for index, provider := range providers {
		library := catalogue[index]
		if provider.Identity != string(library.ID()) || provider.Declaration == nil {
			t.Fatalf("provider %d does not match catalogue entry %q", index, library.ID())
		}
		first := provider.Declaration()
		second := provider.Declaration()
		if first == second {
			t.Fatalf("%q provider returned shared mutable manifest", library.ID())
		}
		if first.Path != library.Name() || first.Version != ManifestVersion {
			t.Fatalf("%q manifest identity = %q@%q", library.ID(), first.Path, first.Version)
		}
	}
	sealed, err := declarations.Seal(Providers()...)
	if err != nil {
		t.Fatal(err)
	}
	if len(sealed.ProviderIdentities()) != len(catalogue) {
		t.Fatalf("sealed providers = %d, catalogue = %d", len(sealed.ProviderIdentities()), len(catalogue))
	}
}

func manifestForTest(t *testing.T, id ID) *modulemanifest.Manifest {
	t.Helper()
	library, ok := Lookup(id)
	if !ok {
		t.Fatalf("missing library %q", id)
	}
	return buildManifest(library, library.declaration())
}

func TestHighRiskSignaturesTrackNativeABIAndEffects(t *testing.T) {
	catalogue, err := declarations.Seal(Providers()...)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name       string
		params     int
		returns    int
		variadic   bool
		openEffect bool
	}{
		{name: "rawset", params: 3, returns: 1},
		{name: "xpcall", params: 2, returns: 2, openEffect: true},
		{name: "package.loadlib", params: 2, returns: 1},
		{name: "table.create", params: 2, returns: 1, openEffect: true},
		{name: "string.gmatch", params: 2, returns: 2, openEffect: true},
		{name: "string.rep", params: 2, returns: 1},
		{name: "math.atan", params: 1, returns: 1},
		{name: "math.log", params: 1, returns: 1},
		{name: "math.random", params: 2, returns: 1, openEffect: true},
		{name: "math.randomseed", variadic: true},
		{name: "coroutine.running", returns: 1, openEffect: true},
		{name: "coroutine.spawn", params: 1, openEffect: true},
		{name: "debug.setlocal", params: 3, returns: 1, openEffect: true},
		{name: "errors.call_stack", params: 1, returns: 1, openEffect: true},
	}
	for _, test := range tests {
		function, ok := catalogue.Function(test.name)
		if !ok {
			t.Errorf("missing signature %q", test.name)
			continue
		}
		sig := function.Signature()
		if len(sig.Type.Params) != test.params || len(sig.Type.Returns) != test.returns ||
			(sig.Type.Variadic != nil) != test.variadic || sig.Effect.IsOpen() != test.openEffect {
			t.Errorf("%s ABI/effect = params:%d returns:%d variadic:%t open:%t",
				test.name, len(sig.Type.Params), len(sig.Type.Returns),
				sig.Type.Variadic != nil, sig.Effect.IsOpen())
		}
	}

	insertFunction, _ := catalogue.Function("table.insert")
	insert := insertFunction.Signature()
	wantLabels := map[string]bool{"mutator": false, "length": false, "store": false}
	for _, label := range insert.Effect.Labels {
		switch label.(type) {
		case mutation.TableMutator:
			wantLabels["mutator"] = true
		case mutation.LengthChange:
			wantLabels["length"] = true
		case ownership.Store:
			wantLabels["store"] = true
		}
	}
	for name, present := range wantLabels {
		if !present {
			t.Errorf("table.insert omitted %s effect", name)
		}
	}
}

func TestManifestsRoundTripAndDoNotDeclareRuntimeScope(t *testing.T) {
	for _, library := range catalogue {
		m := manifestForTest(t, library.ID())
		if err := m.Validate(); err != nil {
			t.Fatalf("%q manifest validation: %v", library.ID(), err)
		}
		data, err := modulemanifest.Encode(m)
		if err != nil {
			t.Fatalf("%q manifest encode: %v", library.ID(), err)
		}
		decoded, err := modulemanifest.Decode(data)
		if err != nil {
			t.Fatalf("%q manifest decode: %v", library.ID(), err)
		}
		if decoded.Path != m.Path || len(decoded.FunctionSignatures) != len(m.FunctionSignatures) {
			t.Fatalf("%q manifest changed across wire round trip", library.ID())
		}
		if library.Mount() == MountModule && len(m.Globals) != 0 {
			t.Fatalf("named library %q tried to establish analysis scope: %v", library.ID(), m.Globals)
		}
	}
}

func TestEveryDirectCallableIsExportedWithAnAuditedEffectRow(t *testing.T) {
	for _, library := range catalogue {
		m := manifestForTest(t, library.ID())
		export, ok := m.Export.(*typ.Record)
		if !ok {
			t.Fatalf("%q export is %T, want record", library.ID(), m.Export)
		}
		decl := library.declaration()
		for name, sig := range decl.signatures {
			if sig.Type == nil {
				t.Fatalf("%q.%s has nil function type", library.ID(), name)
			}
			field := export.GetField(name)
			if field == nil || !typ.TypeEquals(field.Type, sig.Type) {
				t.Fatalf("%q.%s export/signature type drift", library.ID(), name)
			}
			if sig.Effect.Tail != nil && (sig.Effect.Tail.Name == "" || sig.Effect.Tail.Name == "?") {
				t.Fatalf("%q.%s has anonymous open effect row", library.ID(), name)
			}
			for _, label := range sig.Effect.Labels {
				switch label.(type) {
				case control.IO, control.Throw:
					t.Fatalf("%q.%s uses reserved inactive effect %T", library.ID(), name, label)
				}
			}
		}
	}
}

func TestNestedErrorMethodsAreTypedButNotDirectModuleExports(t *testing.T) {
	m := manifestForTest(t, Errors)
	export := m.Export.(*typ.Record)
	if export.GetField("Error.kind") != nil || export.GetField("Error") != nil {
		t.Fatal("errors manifest fabricated a direct Error export")
	}
	if _, ok := m.FunctionSignatures["Error.kind"]; !ok {
		t.Fatal("errors manifest omitted error-object method signature")
	}
	if m.ErrorType == nil || m.Types["Error"] == nil {
		t.Fatal("errors manifest omitted canonical Error type")
	}
}

func TestInitialGlobalsAreProjectedFromMountShape(t *testing.T) {
	sealed, err := declarations.Seal(Providers()...)
	if err != nil {
		t.Fatal(err)
	}
	names := sealed.InitialGlobals()
	want := make(map[string]bool)
	for _, library := range catalogue {
		decl := library.declaration()
		if library.Mount() == MountGlobals {
			for name := range decl.signatures {
				want[name] = true
			}
			for name := range decl.values {
				want[name] = true
			}
		} else {
			want[library.Name()] = true
		}
	}
	if len(names) != len(want) {
		t.Fatalf("initial globals = %v, want %d catalogue-derived names", names, len(want))
	}
	for _, name := range names {
		if !want[name] {
			t.Fatalf("initial globals contain non-catalogue name %q", name)
		}
	}
	for _, foreign := range []string{"require", "collectgarbage", "io", "os", "rawlen"} {
		if want[foreign] {
			t.Errorf("catalogue fabricated host/runtime global %q", foreign)
		}
	}
}
