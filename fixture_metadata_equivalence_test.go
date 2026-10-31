package lua

import (
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/diag"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/typ"
)

// Every fixture checks its modules with both fresh manifests and manifests
// passed through the binary cache format. Compare diagnostics at each import.
func TestFixtureManifestDiagnosticEquivalence(t *testing.T) {
	suites, err := discoverFixtures("testdata/fixtures")
	if err != nil {
		t.Fatal(err)
	}
	for _, suite := range suites {
		if suite.Suite.Skip != "" || (suite.Suite.Check != nil && suite.Suite.Check.Skip != "") {
			continue
		}
		for _, mode := range checkModes(suite) {
			t.Run(suite.Name+"/"+mode, func(t *testing.T) {
				t.Parallel()
				files := resolveFiles(suite)
				base := []testutil.Option{testutil.WithCheckOptions(checkOptionsFor(t, mode))}
				if resolveStdlib(suite) {
					base = append(base, testutil.WithStdlib())
				}
				for _, pkg := range suite.Suite.Packages {
					m := resolvePackageManifest(pkg)
					if m == nil {
						t.Fatalf("unknown package %s", pkg)
					}
					base = append(base, testutil.WithManifest(pkg, m))
				}
				fresh, cached := map[string]*io.Manifest{}, map[string]*io.Manifest{}
				for i, file := range files {
					source := readFixtureFile(suite.Dir, file)
					options := func(mods map[string]*io.Manifest) []testutil.Option {
						opts := append([]testutil.Option(nil), base...)
						for name, m := range mods {
							opts = append(opts, testutil.WithManifest(name, m))
						}
						return opts
					}
					var a, b []diag.Diagnostic
					if i < len(files)-1 {
						name := strings.TrimSuffix(file, ".lua")
						fm := testutil.CheckAndExport(source, name, options(fresh)...)
						cm := testutil.CheckAndExport(source, name, options(cached)...)
						a, b = fm.Session.Diagnostics, cm.Session.Diagnostics
						fresh[name] = fm.Manifest
						data, err := cm.Manifest.Encode()
						if err != nil {
							t.Fatalf("%s encode: %v", file, err)
						}
						cached[name], err = io.DecodeManifest(data)
						if err != nil {
							t.Fatalf("%s decode: %v", file, err)
						}
					} else {
						a = testutil.Check(source, options(fresh)...).Diagnostics
						b = testutil.Check(source, options(cached)...).Diagnostics
					}
					if !reflect.DeepEqual(diagnosticKeys(a), diagnosticKeys(b)) {
						t.Errorf("%s fresh diagnostics %v; decoded diagnostics %v", file, diagnosticKeys(a), diagnosticKeys(b))
					}
				}
			})
		}
	}
}

var recursiveDiagnosticID = regexp.MustCompile(`rec#[0-9]+`)

func diagnosticKeys(ds []diag.Diagnostic) []string {
	out := make([]string, len(ds))
	// Recursive IDs come from a process-wide allocator. Alpha-rename them
	// while retaining repeated/distinct identity across the diagnostics.
	ids := make(map[string]string)
	for i, d := range ds {
		message := recursiveDiagnosticID.ReplaceAllStringFunc(d.Message, func(id string) string {
			if name, ok := ids[id]; ok {
				return name
			}
			name := fmt.Sprintf("rec#%d", len(ids))
			ids[id] = name
			return name
		})
		out[i] = fmt.Sprintf("%s:%d:%d:%v:%v:%s", d.Position.File, d.Position.Line, d.Position.Column, d.Severity, d.Code, message)
	}
	return out
}

func TestDiagnosticKeysRecursiveIdentity(t *testing.T) {
	keys := func(message string) []string {
		return diagnosticKeys([]diag.Diagnostic{{Message: message}})
	}
	if !reflect.DeepEqual(keys("rec#10 | rec#20 | rec#10"), keys("rec#30 | rec#40 | rec#30")) {
		t.Fatal("equivalent recursive identities differ")
	}
	if reflect.DeepEqual(keys("rec#10 | rec#10"), keys("rec#30 | rec#40")) {
		t.Fatal("distinct recursive identities collapsed")
	}
}

func TestImportedMetadataDiagnosticEquivalence(t *testing.T) {
	const consumer = "local m = require(\"mod\")\nlocal v: number = m.lookup[\"a\"]"
	tests := []struct {
		name   string
		lookup typ.Type
	}{
		{"map inferred presence", typ.NewInferredMap(typ.String, typ.Number)},
		{"record map inferred presence", typ.NewRecord().MapComponentWithFlags(typ.String, typ.Number, true, false).Build()},
		{"field inferred presence", typ.NewRecord().AddField(typ.Field{Name: "a", Type: typ.Number, Optional: true, InferredPresence: true}).Build()},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := io.NewManifest("mod")
			m.Export = typ.NewRecord().Field("lookup", tc.lookup).Build()
			fresh := testutil.Check(consumer, testutil.WithManifest("mod", m)).Diagnostics
			data, err := m.Encode()
			if err != nil {
				t.Fatal(err)
			}
			decoded, err := io.DecodeManifest(data)
			if err != nil {
				t.Fatal(err)
			}
			cached := testutil.Check(consumer, testutil.WithManifest("mod", decoded)).Diagnostics
			if !reflect.DeepEqual(diagnosticKeys(fresh), diagnosticKeys(cached)) {
				t.Fatalf("fresh diagnostics %v; decoded diagnostics %v", diagnosticKeys(fresh), diagnosticKeys(cached))
			}
		})
	}
}
