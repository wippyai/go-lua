package engine

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func w6aSourceRoots(t *testing.T) (engineDir, frontDir, exporterDir string) {
	t.Helper()
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate engine source")
	}
	engineDir = filepath.Dir(current)
	checkDir := filepath.Dir(engineDir)
	return engineDir, filepath.Join(checkDir, "fixpoint", "front"), filepath.Join(checkDir, "exporter")
}

func w6aProductionFiles(t *testing.T, directories ...string) []string {
	t.Helper()
	var files []string
	for _, directory := range directories {
		entries, err := os.ReadDir(directory)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			name := entry.Name()
			if !entry.IsDir() && strings.HasSuffix(name, ".go") && !strings.HasSuffix(name, "_test.go") {
				files = append(files, filepath.Join(directory, name))
			}
		}
	}
	return files
}

func w6aNodeMentionsRole(node ast.Node) bool {
	mentions := false
	ast.Inspect(node, func(candidate ast.Node) bool {
		switch value := candidate.(type) {
		case *ast.SelectorExpr:
			mentions = mentions || value.Sel.Name == "Role"
		case *ast.Ident:
			mentions = mentions || value.Name == "role"
		}
		return !mentions
	})
	return mentions
}

func w6aRawRoleParser(source []byte) string {
	file, err := parser.ParseFile(token.NewFileSet(), "candidate.go", source, 0)
	if err != nil {
		return "unparseable source"
	}
	found := ""
	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		pkg, ok := selector.X.(*ast.Ident)
		if !ok {
			return true
		}
		if pkg.Name != "strings" ||
			(selector.Sel.Name != "HasPrefix" && selector.Sel.Name != "TrimPrefix" &&
				selector.Sel.Name != "HasSuffix" && selector.Sel.Name != "TrimSuffix" &&
				selector.Sel.Name != "Cut" && selector.Sel.Name != "CutPrefix") {
			return true
		}
		for _, argument := range call.Args {
			if w6aNodeMentionsRole(argument) {
				found = pkg.Name + "." + selector.Sel.Name
				return false
			}
		}
		return true
	})
	return found
}

func TestTypedOperandRolesStayDisplaced(t *testing.T) {
	engineDir, frontDir, exporterDir := w6aSourceRoots(t)
	for _, path := range w6aProductionFiles(t, engineDir, frontDir, exporterDir) {
		source, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if parser := w6aRawRoleParser(source); parser != "" {
			t.Errorf("%s reconstructs operand roles with %s", path, parser)
		}
	}
}

func TestTypedOperandRoleFenceRejectsPrefixReconstruction(t *testing.T) {
	mutated := []byte(`package engine
import "strings"
func resurrect(operand struct{ Role string }) bool {
	return strings.HasPrefix(operand.Role, "result-")
}`)
	if parser := w6aRawRoleParser(mutated); parser == "" {
		t.Fatal("role fence accepted a reconstructed result family")
	}
}

func w6aForbiddenChannelConsumer(source []byte) string {
	text := string(source)
	for _, forbidden := range []string{"compilation.WIR", "ForEachIfChainDescriptor", "BranchChecks("} {
		if strings.Contains(text, forbidden) {
			return forbidden
		}
	}
	if strings.Contains(text, `".channel"`) &&
		(strings.Contains(text, "HasSuffix(") || strings.Contains(text, "TrimSuffix(")) {
		return "channel suffix parser"
	}
	return ""
}

func TestChannelSelectConsumersStayOnFrontOperands(t *testing.T) {
	engineDir, _, _ := w6aSourceRoots(t)
	source, err := os.ReadFile(filepath.Join(engineDir, "channel_select_consumers.go"))
	if err != nil {
		t.Fatal(err)
	}
	if forbidden := w6aForbiddenChannelConsumer(source); forbidden != "" {
		t.Fatalf("channel-select consumer resurrected %q", forbidden)
	}
}

func TestChannelSelectFenceRejectsTopologyAndSuffixReconstruction(t *testing.T) {
	for _, mutated := range [][]byte{
		[]byte(`package engine
func resurrect() { compilation.WIR.ForEachIfChainDescriptor(nil) }`),
		[]byte(`package engine
import "strings"
func resurrect(path string) { _ = strings.TrimSuffix(path, ".channel") }`),
	} {
		if forbidden := w6aForbiddenChannelConsumer(mutated); forbidden == "" {
			t.Fatal("channel-select fence accepted reconstructed topology")
		}
	}
}

func w6aForbiddenNativeRecognizer(source []byte) string {
	text := string(source)
	for _, forbidden := range []string{
		"nativeDirectCall(",
		"nativeMemberCall(",
		`Value == "pcall"`,
		`Value == "os"`,
		`Value == "clock"`,
	} {
		if strings.Contains(text, forbidden) {
			return forbidden
		}
	}
	return ""
}

func TestNativeRecognitionStaysBinderAndRegistryOwned(t *testing.T) {
	_, frontDir, _ := w6aSourceRoots(t)
	source, err := os.ReadFile(filepath.Join(frontDir, "native_operations.go"))
	if err != nil {
		t.Fatal(err)
	}
	if forbidden := w6aForbiddenNativeRecognizer(source); forbidden != "" {
		t.Fatalf("native recognition resurrected %q", forbidden)
	}
	for _, required := range []string{"ResolvesToGlobal(", "LookupView("} {
		if !strings.Contains(string(source), required) {
			t.Fatalf("native recognition omitted %q", required)
		}
	}
}

func TestNativeRecognitionFenceRejectsRawNameMatching(t *testing.T) {
	mutated := []byte(`package front
func resurrect(id *Ident) bool { return id.Value == "pcall" }`)
	if forbidden := w6aForbiddenNativeRecognizer(mutated); forbidden == "" {
		t.Fatal("native-recognition fence accepted raw AST name matching")
	}
}
