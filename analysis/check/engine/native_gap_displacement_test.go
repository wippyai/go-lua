package engine

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestNativeCaptureScansStayDisplaced is the source fence for the capture
// families now owned by closure materialization. Native serialization may
// consume their publications, but neither the former WIR walk nor the former
// AST free-reference recognizer may return.
func TestNativeCaptureScansStayDisplaced(t *testing.T) {
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate engine source")
	}
	engineDir := filepath.Dir(current)
	for path, forbidden := range map[string][]string{
		filepath.Join(engineDir, "lowering_structural_projection.go"): {
			"captureNativeFacts",
			"captureTransportNativeFacts",
			"captureTransportFacts",
			"capture_epoch_root",
			"capture_transport",
		},
		filepath.Join(engineDir, "..", "fixpoint", "front", "native_operations.go"): {
			"nativeCaptureTransportCount",
			"nativeFunctionReferences",
			"nativeDirectFreeReferenceCountNamed",
		},
	} {
		source, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, name := range forbidden {
			if strings.Contains(string(source), name) {
				t.Errorf("%s resurrects displaced native capture scan %q", path, name)
			}
		}
	}
}

// TestNativeProjectionHasNoWIRConsumers is the N5 source fence. Native output
// files are serializers over published facts only; WIR and CFG belong to the
// lowering-side projection producers and may not return to this boundary.
func TestNativeProjectionHasNoWIRConsumers(t *testing.T) {
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate engine source")
	}
	engineDir := filepath.Dir(current)
	entries, err := os.ReadDir(engineDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasPrefix(name, "native") || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		source, readErr := os.ReadFile(filepath.Join(engineDir, name))
		if readErr != nil {
			t.Fatal(readErr)
		}
		for _, forbidden := range []string{"analysis/ir/wir", "analysis/ir/cfg"} {
			if strings.Contains(string(source), forbidden) {
				t.Errorf("%s imports forbidden native consumer dependency %q", name, forbidden)
			}
		}
	}

	engineSource, err := os.ReadFile(filepath.Join(engineDir, "engine.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		"closure.Values = append(closure.Values, publishedNestedConstantValues",
		"closure.Values = append(closure.Values, publishedNestedNativeKernelFacts",
	} {
		if strings.Contains(string(engineSource), forbidden) {
			t.Errorf("projectCheck resurrects post-solve native injection %q", forbidden)
		}
	}

	astContracts, err := os.ReadFile(filepath.Join(engineDir, "..", "fixpoint", "front", "native_contracts.go"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(astContracts), `Family: "call_scc"`) {
		t.Error("AST native contracts resurrect the displaced call_scc recognizer")
	}
}
