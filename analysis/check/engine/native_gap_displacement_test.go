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
		filepath.Join(engineDir, "native_structural.go"): {
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
