package engine

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestEngineUsesCompilationProjectionsAndCentralWireDecode(t *testing.T) {
	root := enginePackageDir(t)
	directBagRead := regexp.MustCompile(`\b(?:child|compilation|parent|callee|nested)\.(?:Artifact|Cyclic|Frozen|WIR|Graph|Body|PrototypeName|LexicalPath|Boundary|RebindsBoundary|Nested|ClaimSpans|ClaimTargetSpans|ClaimNameSpans|BranchJoinSpans|CallSpans|BranchSpans|EffectSpans|ExpressionSpans|ReturnSpans|TableMemberValueSpans|Catalog|ControlDiagnostics|PolicyDiagnostics|TypeDefinitions|TypeFieldSpans|NativeContracts)\b`)
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		source, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(source), "json.Unmarshal(") {
			t.Errorf("%s: JSON decode bypasses front wire codec", path)
		}
		if match := directBagRead.Find(source); match != nil {
			t.Errorf("%s: direct Compilation bag read %q bypasses a projection", path, match)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func enginePackageDir(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return dir
}
