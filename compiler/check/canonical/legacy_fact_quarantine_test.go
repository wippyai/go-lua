package canonical

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCanonicalProductionDoesNotUseLegacyFactProducts(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Dir(file)
	banned := []string{
		"domain/interproc",
		"LegacyFacts(",
		"LegacyFactProduct",
		"LegacyFactProductReader",
		"LegacyFactProductSink",
		"LegacyInterprocPrev",
		"LegacyInterprocNext",
		"MergeLegacyFactsNext",
		"LegacyFixpointSwap",
		"InterprocFacts(",
		"InterprocFactReader",
		"InterprocFactProduct",
		"InterprocPrev",
		"InterprocNext",
		"MergeInterprocFactsNext",
		"FixpointSwap",
	}
	functionFactsAllowed := map[string]bool{
		filepath.Join(root, "function_fact_projection.go"): true,
	}

	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := string(data)
		for _, token := range banned {
			if strings.Contains(text, token) {
				t.Errorf("%s uses legacy fact-product token %q", path, token)
			}
		}
		if strings.Contains(text, "FunctionFacts") && !functionFactsAllowed[path] {
			t.Errorf("%s uses FunctionFacts outside final projection boundary", path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk canonical sources: %v", err)
	}
}

func TestCanonicalProductionTreatsFuncResultAsPublicProjection(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Dir(file)
	allowed := map[string]bool{
		filepath.Join(root, "diagnostic_emitter.go"):       true,
		filepath.Join(root, "driver.go"):                   true,
		filepath.Join(root, "public_result_projection.go"): true,
	}
	tokens := []string{
		"FuncResult",
		"FromFuncResult",
		"ViewFromResult",
	}

	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := string(data)
		for _, token := range tokens {
			if strings.Contains(text, token) && !allowed[path] {
				t.Errorf("%s uses public FuncResult projection token %q outside projection boundary", path, token)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk canonical sources: %v", err)
	}
}

func TestCanonicalObservationDoesNotUseLiveSummaryAuthority(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Dir(file)
	path := filepath.Join(root, "observe.go")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read observe.go: %v", err)
	}
	text := string(data)
	banned := []string{
		"activeReader(",
		"activeSummaryReader",
		"activeQueries",
		"summary.NewReaderWithStats",
		"summary.NewReader(",
		"queries.Summarize",
		"SummarizeWithKey(",
		"ObserveIntraWithKey(",
		"CanonicalSummarySnapshot(",
	}
	for _, token := range banned {
		if strings.Contains(text, token) {
			t.Errorf("observe.go uses live/producer summary token %q", token)
		}
	}
}

func TestCanonicalDiagnosticsDoNotUseSemanticAuthority(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Join(filepath.Dir(file), "diagnostic")
	banned := []string{
		"summary.Summary",
		"summary.Reader",
		"Summarize(",
		"SummarizeWithKey(",
		"ObserveIntra",
		"BoundaryFacts",
		"ApplyBoundaryFacts",
		"PointState",
	}

	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := string(data)
		for _, token := range banned {
			if strings.Contains(text, token) {
				t.Errorf("%s uses semantic-authority token %q", path, token)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk diagnostic sources: %v", err)
	}
}
