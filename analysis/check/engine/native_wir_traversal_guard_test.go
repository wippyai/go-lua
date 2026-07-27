package engine

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

const wirPackagePath = "github.com/wippyai/go-lua/analysis/ir/wir"

var sanctionedWIRTraversalFiles = map[string]map[string]bool{
	"github.com/wippyai/go-lua/analysis/check/fixpoint/front": {
		"advice.go":               true,
		"branches.go":             true,
		"front.go":                true,
		"native_wir_contracts.go": true,
	},
}

type listedGoPackage struct {
	ImportPath string
	Dir        string
	GoFiles    []string
	CgoFiles   []string
	Export     string
}

type wirTraversalViolation struct {
	file string
	line int
	kind string
}

func TestProductionWIRTraversalIsConfinedToLoweringFront(t *testing.T) {
	moduleRoot := moduleRootForGuard(t)
	packages := listedPackagesForGuard(t, moduleRoot)
	exports := make(map[string]string, len(packages))
	for _, item := range packages {
		if item.Export != "" {
			exports[item.ImportPath] = item.Export
		}
	}
	fset := token.NewFileSet()
	imports := importer.ForCompiler(fset, "gc", func(path string) (io.ReadCloser, error) {
		export := exports[path]
		if export == "" {
			return nil, fmt.Errorf("no export data for %s", path)
		}
		return os.Open(export)
	})
	var violations []wirTraversalViolation
	for _, item := range packages {
		if !strings.HasPrefix(item.ImportPath, "github.com/wippyai/go-lua/analysis/check") {
			continue
		}
		files := append(append([]string(nil), item.GoFiles...), item.CgoFiles...)
		syntax := make([]*ast.File, 0, len(files))
		fileNames := make(map[*ast.File]string, len(files))
		for _, name := range files {
			parsed, err := parser.ParseFile(fset, filepath.Join(item.Dir, name), nil, 0)
			if err != nil {
				t.Fatalf("parse %s/%s: %v", item.ImportPath, name, err)
			}
			syntax = append(syntax, parsed)
			fileNames[parsed] = name
		}
		info := &types.Info{
			Types:      make(map[ast.Expr]types.TypeAndValue),
			Uses:       make(map[*ast.Ident]types.Object),
			Selections: make(map[*ast.SelectorExpr]*types.Selection),
		}
		config := types.Config{Importer: imports}
		if _, err := config.Check(item.ImportPath, fset, syntax, info); err != nil {
			t.Fatalf("type-check %s: %v", item.ImportPath, err)
		}
		for _, file := range syntax {
			name := fileNames[file]
			if wirTraversalSanctioned(item.ImportPath, name) {
				continue
			}
			violations = append(violations, findWIRTraversalViolations(fset, name, file, info)...)
		}
	}
	sort.Slice(violations, func(i, j int) bool {
		if violations[i].file != violations[j].file {
			return violations[i].file < violations[j].file
		}
		return violations[i].line < violations[j].line
	})
	if len(violations) != 0 {
		var lines []string
		for _, violation := range violations {
			lines = append(lines, fmt.Sprintf("%s:%d: %s", violation.file, violation.line, violation.kind))
		}
		t.Fatalf("production WIR traversal escaped the explicit lowering-front sanction:\n%s", strings.Join(lines, "\n"))
	}
}

func wirTraversalSanctioned(importPath, file string) bool {
	if strings.HasPrefix(importPath, "github.com/wippyai/go-lua/analysis/lua/wirlower") {
		return true
	}
	return sanctionedWIRTraversalFiles[importPath][file]
}

func findWIRTraversalViolations(
	fset *token.FileSet,
	fileName string,
	file *ast.File,
	info *types.Info,
) []wirTraversalViolation {
	var violations []wirTraversalViolation
	report := func(node ast.Node, kind string) {
		violations = append(violations, wirTraversalViolation{
			file: fileName, line: fset.Position(node.Pos()).Line, kind: kind,
		})
	}
	ast.Inspect(file, func(node ast.Node) bool {
		switch item := node.(type) {
		case *ast.SelectorExpr:
			selection := info.Selections[item]
			if selection != nil && isWIRNamed(selection.Recv(), "Body") {
				// This is the union of the landed semantic traversal fence and
				// the W7A whole-production sanction: direct calls and bound
				// method references are both traversal references.
				switch selection.Obj().Name() {
				case "Instr", "Len", "PointInstructions", "ForEachCall", "ForEachConst":
					report(item, "wir.Body."+selection.Obj().Name())
				}
			}
			if item.Sel.Name == "Op" && isWIRNamed(info.TypeOf(item.X), "Instruction") {
				report(item, "wir.Instruction.Op kind inspection")
			}
		}
		return true
	})
	return violations
}

func TestWIRTraversalGuardResolvesTypeAliases(t *testing.T) {
	pkg := types.NewPackage(wirPackagePath, "wir")
	body := types.NewNamed(
		types.NewTypeName(token.NoPos, pkg, "Body", nil),
		types.NewStruct(nil, nil),
		nil,
	)
	alias := types.NewAlias(types.NewTypeName(token.NoPos, pkg, "BodyAlias", nil), body)
	if !isWIRNamed(types.NewPointer(alias), "Body") {
		t.Fatal("WIR traversal guard lost a wir.Body type alias")
	}
	if wirTraversalSanctioned("github.com/wippyai/go-lua/analysis/check/fixpoint/front", "renamed.go") {
		t.Fatal("WIR traversal guard sanctioned a renamed lowering file")
	}
}

func TestSanctionedNativeWIRTraversalReferenceSetIsPinned(t *testing.T) {
	loader := newFencePackageLoader(t, "./analysis/check/fixpoint/front")
	meta := loader.metas[modulePath+"/analysis/check/fixpoint/front"]
	references := fenceWIRTraversalReferences(loader.load(meta))
	count := 0
	for _, reference := range references {
		if filepath.Base(strings.SplitN(reference, ":", 2)[0]) == "native_wir_contracts.go" {
			count++
		}
	}
	// The sanctioned file currently owns fourteen bounded Body.Len/Body.Instr
	// pairs. Any additional traversal, including the rescan5 loop mutation,
	// changes this closed census and requires an explicit ownership review.
	if count != 28 {
		t.Fatalf("native_wir_contracts.go traversal references = %d, want pinned 28", count)
	}
}

func isWIRNamed(value types.Type, name string) bool {
	value = types.Unalias(value)
	if pointer, ok := value.(*types.Pointer); ok {
		value = types.Unalias(pointer.Elem())
	}
	named, ok := value.(*types.Named)
	if !ok || named.Obj() == nil || named.Obj().Pkg() == nil {
		return false
	}
	return named.Obj().Name() == name && named.Obj().Pkg().Path() == wirPackagePath
}

func moduleRootForGuard(t *testing.T) string {
	t.Helper()
	command := exec.Command("go", "env", "GOMOD")
	output, err := command.Output()
	if err != nil {
		t.Fatalf("go env GOMOD: %v", err)
	}
	mod := strings.TrimSpace(string(output))
	if mod == "" || mod == os.DevNull {
		t.Fatalf("go env GOMOD returned %q", mod)
	}
	return filepath.Dir(mod)
}

func listedPackagesForGuard(t *testing.T, moduleRoot string) []listedGoPackage {
	t.Helper()
	command := exec.Command("go", "list", "-json", "-export", "-deps", "./analysis/check/...")
	command.Dir = moduleRoot
	output, err := command.Output()
	if err != nil {
		t.Fatalf("go list production package graph: %v", err)
	}
	decoder := json.NewDecoder(strings.NewReader(string(output)))
	var packages []listedGoPackage
	for {
		var item listedGoPackage
		if err := decoder.Decode(&item); err != nil {
			if err == io.EOF {
				break
			}
			t.Fatalf("decode go list package graph: %v", err)
		}
		packages = append(packages, item)
	}
	return packages
}
