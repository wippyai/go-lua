package engine

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/front"
)

func TestFrontDraftWireOwnershipStaysDisplaced(t *testing.T) {
	root := wireFenceRepositoryRoot(t)
	owned := filepath.Join(root, "analysis", "check", "fixpoint", "front", "wire_codec.go")
	prefixes := []string{
		"front/branch-predicate/v1/",
		"front/branch-evidence/v1/",
		"front/branch-diff/v1/",
		"provider/module/v1/",
	}
	for _, directory := range []string{
		filepath.Join(root, "analysis", "check", "fixpoint", "front"),
		filepath.Join(root, "analysis", "check", "engine"),
	} {
		entries, err := os.ReadDir(directory)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			path := filepath.Join(directory, entry.Name())
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") || path == owned {
				continue
			}
			file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
			if err != nil {
				t.Fatalf("parse %s: %v", path, err)
			}
			ast.Inspect(file, func(node ast.Node) bool {
				switch item := node.(type) {
				case *ast.BasicLit:
					if item.Kind != token.STRING {
						return true
					}
					value, err := strconv.Unquote(item.Value)
					if err != nil {
						return true
					}
					for _, prefix := range prefixes {
						if strings.Contains(value, prefix) {
							t.Errorf("%s:%d constructs or parses front draft wire prefix %q outside its codec", path, item.Pos(), prefix)
						}
					}
				case *ast.TypeSpec:
					if _, structType := item.Type.(*ast.StructType); structType {
						switch item.Name.Name {
						case "branchPredicateWire", "branchDiffWire", "moduleProviderWire",
							"BranchPredicateWire", "BranchDiffWire", "ModuleProviderWire":
							t.Errorf("%s:%d redeclares front-owned wire struct %s", path, item.Pos(), item.Name.Name)
						}
					}
				}
				return true
			})
		}
	}
}

func wireFenceRepositoryRoot(t *testing.T) string {
	t.Helper()
	directory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if info, statErr := os.Stat(filepath.Join(directory, "go.mod")); statErr == nil && !info.IsDir() {
			return directory
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			t.Fatal("could not find repository go.mod")
		}
		directory = parent
	}
}

func TestBranchKernelSurfacesMalformedPredicateWire(t *testing.T) {
	operation := equation.BoundEquation{
		Target: equation.Coordinate{Name: "branch"},
		Operands: []equation.BoundOperand{{
			Role:  "predicate",
			Value: []byte("front/branch-predicate/v1/{"),
		}},
	}
	if _, err := branchSelectionKernel(operation, equation.Partition{}); err == nil {
		t.Fatal("malformed predicate wire was silently treated as absent semantics")
	}
}

func TestBranchNumericConsumerSurfacesMalformedDifferenceWire(t *testing.T) {
	operation := equation.BoundEquation{Operands: []equation.BoundOperand{{
		Role:  "difference-00000000",
		Value: []byte("front/branch-diff/v1/{"),
	}}}
	if _, _, err := branchNumericTruth(operation, equation.Partition{}); err == nil {
		t.Fatal("malformed difference wire was silently treated as no relation")
	}
}

func TestEngineAdmissionSurfacesMalformedModuleProviderWire(t *testing.T) {
	compilation := front.Compilation{Artifact: equation.Artifact{Equations: []equation.Equation{{
		Target: equation.Coordinate{Name: "call"},
		Operands: []equation.Operand{{
			Role: "provider",
			Term: equation.ClosedTerm([]byte("provider/module/v1/not-base64")),
		}},
	}}}}
	if _, _, err := evaluateCheck(compilation, equation.EntryBinding{}, nil, nil); err == nil {
		t.Fatal("malformed module-provider wire was silently treated as an unresolved provider")
	}
}
