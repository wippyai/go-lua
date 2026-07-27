package engine

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
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

const engineWIRInstructionLoopCeiling = 36

var n7InstructionWalkPatterns = []*regexp.Regexp{
	regexp.MustCompile(`for\s+\w+\s*:=\s*0;\s*\w+\s*<\s*[\w.]+\.Len\(\);`),
	regexp.MustCompile(`for\s+[^{}]*range\s+[\w.]+\.PointInstructions\(`),
}

func n7DisplacedScanner(source []byte) string {
	for _, pattern := range n7DisplacedScannerPatterns {
		if strings.Contains(string(source), pattern) {
			return pattern
		}
	}
	for _, pattern := range n7InstructionWalkPatterns {
		if pattern.Match(source) {
			return pattern.String()
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
	assertEngineWIRInstructionLoopCeiling(t, engineDir)
}

func TestN7SoundPrefixFenceRejectsRenamedScanner(t *testing.T) {
	mutated := []byte("package engine\nfunc typedProducerNativeFacts() {}\n")
	if pattern := n7DisplacedScanner(mutated); pattern == "" {
		t.Fatal("pattern-over-all-files fence accepted a renamed scanner file")
	}
}

func TestNativeFencesRejectGenericWIRInstructionWalk(t *testing.T) {
	file, err := fenceParseSource(`package engine
func numericNativeFacts(root Compilation) {
	if root.WIR != nil {
		for index := 0; index < root.WIR.Len(); index++ {
			_ = root.WIR.Instr(index)
		}
	}
}`)
	if err != nil {
		t.Fatal(err)
	}
	if got := fenceWIRInstructionLoopCount(file); got != 1 {
		t.Fatalf("generic WIR mutation matched %d instruction loops, want 1", got)
	}
}

func TestN7SoundPrefixFenceRejectsGenericInstructionWalk(t *testing.T) {
	mutated := []byte("package engine\nfunc scan(body *wir.Body) { for index := 0; index < body.Len(); index++ { _ = body.Instr(index) } }\n")
	if pattern := n7DisplacedScanner(mutated); pattern == "" {
		t.Fatal("pattern-over-all-files fence accepted a generic WIR instruction walk")
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
	for path, forbidden := range map[string][]string{
		filepath.Join(engineDir, "..", "fixpoint", "front", "lowering_structural_projection.go"): {
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
	assertEngineWIRInstructionLoopCeiling(t, engineDir)

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

func assertEngineWIRInstructionLoopCeiling(t *testing.T, engineDir string) {
	t.Helper()
	entries, err := os.ReadDir(engineDir)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, parseErr := parser.ParseFile(token.NewFileSet(), filepath.Join(engineDir, name), nil, 0)
		if parseErr != nil {
			t.Fatalf("parse %s: %v", name, parseErr)
		}
		count += fenceWIRInstructionLoopCount(file)
	}
	if count > engineWIRInstructionLoopCeiling {
		t.Errorf("engine contains %d indexed WIR instruction walks, ceiling is %d; native projections must consume published facts", count, engineWIRInstructionLoopCeiling)
	}
}
