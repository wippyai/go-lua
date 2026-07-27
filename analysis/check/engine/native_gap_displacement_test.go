package engine

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

var n7DisplacedScannerPatterns = []string{
	"publishedNestedConstantValues(",
	"nativeFoldedConstants(",
	"nativeFoldUnary(",
	"nativeFoldBinary(",
	"nativeFoldIntegerArithmetic(",
	"nativeConstantWriteCounts(",
	"nativeCapturedBindings(",
	"typedProducerNativeFacts(",
	"tableConstructionBoundFacts(",
	"constructorOccurrences(",
	`"typed_producer"`,
	`"table_construction_bound"`,
}

func n7DisplacedScanner(source []byte) string {
	for _, pattern := range n7DisplacedScannerPatterns {
		if strings.Contains(string(source), pattern) {
			return pattern
		}
	}
	return ""
}

// TestN7SoundPrefixScannersStayDisplaced is filename-independent: every engine
// production file is checked, so moving or renaming a deleted scanner cannot
// bypass the fence. Named incumbents and the generic full-instruction walk
// shapes are both forbidden: engine projection consumes closed publications.
func TestN7SoundPrefixScannersStayDisplaced(t *testing.T) {
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
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		source, readErr := os.ReadFile(filepath.Join(engineDir, name))
		if readErr != nil {
			t.Fatal(readErr)
		}
		if pattern := n7DisplacedScanner(source); pattern != "" {
			t.Errorf("%s resurrects an N7 sound-prefix scanner via %q", name, pattern)
		}
	}
	assertEngineHasNoWIRTraversal(t)
}

func TestN7SoundPrefixFenceRejectsRenamedScanner(t *testing.T) {
	mutated := []byte("package engine\nfunc typedProducerNativeFacts() {}\n")
	if pattern := n7DisplacedScanner(mutated); pattern == "" {
		t.Fatal("pattern-over-all-files fence accepted a renamed scanner file")
	}
}

func TestNativeTraversalFenceRejectsEquivalentForms(t *testing.T) {
	loader := newFencePackageLoader(t, "./analysis/check/engine")
	tests := map[string]string{
		"canonical indexed loop": `package engine
import "` + modulePath + `/analysis/ir/wir"
func scan(body *wir.Body) {
	for index := 0; index < body.Len(); index++ {
		_ = body.Instr(index)
	}
}`,
		"rescan4 open ended loop": `package engine
import "` + modulePath + `/analysis/ir/wir"
func scan(body *wir.Body) {
	for index := 0; ; index++ {
		if index >= body.Len() { break }
		_ = body.Instr(index)
	}
}`,
		"body type alias": `package engine
import "` + modulePath + `/analysis/ir/wir"
type scanBody = wir.Body
func scan(body *scanBody) {
	for index := 0; index != body.Len(); index++ {
		_ = body.Instr(index)
	}
}`,
		"bound traversal methods": `package engine
import "` + modulePath + `/analysis/ir/wir"
func scan(body *wir.Body) {
	length := body.Len
	instruction := body.Instr
	for index := 0; index < length(); index++ {
		_ = instruction(index)
	}
}`,
		"point instruction range": `package engine
import (
	"` + modulePath + `/analysis/ir/cfg"
	"` + modulePath + `/analysis/ir/wir"
)
func scan(body *wir.Body, point cfg.Point) {
	for range body.PointInstructions(point) {}
}`,
	}
	for name, source := range tests {
		t.Run(name, func(t *testing.T) {
			typed := loader.source(modulePath+"/analysis/check/engine", source)
			if references := fenceWIRTraversalReferences(typed); len(references) == 0 {
				t.Fatal("type-based WIR traversal fence accepted traversal reference")
			}
		})
	}
}

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
	frontDir := filepath.Join(engineDir, "..", "fixpoint", "front")
	entries, err := os.ReadDir(frontDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		path := filepath.Join(frontDir, entry.Name())
		source, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{
			"captureNativeFacts",
			"captureTransportNativeFacts",
			"captureTransportFacts",
			"nativeCaptureTransportCount",
			"nativeFunctionReferences",
			"nativeDirectFreeReferenceCountNamed",
		} {
			if strings.Contains(string(source), name) {
				t.Errorf("%s resurrects displaced native capture scan %q", path, name)
			}
		}
	}
}

func TestNativeFrontCannotPublishSemanticVerdicts(t *testing.T) {
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate engine source")
	}
	frontDir := filepath.Join(filepath.Dir(current), "..", "fixpoint", "front")
	entries, err := os.ReadDir(frontDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		source, err := os.ReadFile(filepath.Join(frontDir, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{
			"native-publications",
			"completions={'known'",
			"identity=stable_cross_module",
			"stable=true",
			"fresh=true",
			"ownership=move",
			"exhaustive=true",
			"fixpoint=reached",
			"exactness=exact",
			"new_identity=minted",
		} {
			if strings.Contains(string(source), forbidden) {
				t.Errorf("%s states front semantic verdict %q", entry.Name(), forbidden)
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
	assertEngineHasNoWIRTraversal(t)

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

	frontDir := filepath.Join(engineDir, "..", "fixpoint", "front")
	for _, deleted := range []string{
		"native_contracts.go",
		"native_frozen_projection.go",
		"native_shape_epoch_projection.go",
		"native_summary_projection.go",
		"native_wir_contracts.go",
	} {
		if _, err := os.Stat(filepath.Join(frontDir, deleted)); !os.IsNotExist(err) {
			t.Errorf("displaced native semantic plane returned as %s", deleted)
		}
	}
}

func assertEngineHasNoWIRTraversal(t *testing.T) {
	t.Helper()
	loader := newFencePackageLoader(t, "./analysis/check/engine")
	typed := loader.load(loader.metas[modulePath+"/analysis/check/engine"])
	if references := fenceWIRTraversalReferences(typed); len(references) != 0 {
		t.Errorf("engine references WIR traversal APIs outside W7A lowering ownership: %v", references)
	}
}
