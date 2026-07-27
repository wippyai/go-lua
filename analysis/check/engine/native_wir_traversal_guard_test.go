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
		"advice.go":   true,
		"branches.go": true,
		"front.go":    true,
	},
}

const nativeTopologyDraftEmitterFile = "native_wir_topology_drafts.go"

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
			Defs:       make(map[*ast.Ident]types.Object),
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
			if item.ImportPath == modulePath+"/analysis/check/fixpoint/front" &&
				name == nativeTopologyDraftEmitterFile {
				violations = append(violations,
					findTypedTopologyDraftTraversalViolations(fset, name, file, info)...)
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

// findTypedTopologyDraftTraversalViolations admits only traversals reachable
// from a function whose compiled result contains NativeTopologyDraft. This is
// narrower than a whole-file sanction: orphan traversal and use of the former
// semantic carriers are violations. The closed draft union then makes a
// semantic conclusion unrepresentable at the only cross-file exit.
func findTypedTopologyDraftTraversalViolations(
	fset *token.FileSet,
	fileName string,
	file *ast.File,
	info *types.Info,
) []wirTraversalViolation {
	functions := make(map[*types.Func]*ast.FuncDecl)
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Recv != nil || function.Name == nil {
			continue
		}
		object, ok := info.Defs[function.Name].(*types.Func)
		if ok {
			functions[object] = function
		}
	}

	edges := make(map[*types.Func][]*types.Func)
	roots := make(map[*types.Func]bool)
	for object, declaration := range functions {
		if signatureContainsNativeTopologyDraft(object.Type()) {
			roots[object] = true
		}
		if declaration.Body == nil {
			continue
		}
		ast.Inspect(declaration.Body, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			identifier, ok := call.Fun.(*ast.Ident)
			if !ok {
				return true
			}
			callee, ok := info.Uses[identifier].(*types.Func)
			if ok && functions[callee] != nil {
				edges[object] = append(edges[object], callee)
			}
			return true
		})
	}

	reachable := make(map[*types.Func]bool)
	var visit func(*types.Func)
	visit = func(function *types.Func) {
		if reachable[function] {
			return
		}
		reachable[function] = true
		for _, callee := range edges[function] {
			visit(callee)
		}
	}
	for root := range roots {
		visit(root)
	}

	var violations []wirTraversalViolation
	for function, declaration := range functions {
		traversals := findWIRTraversalViolations(fset, fileName, declaration, info)
		if len(traversals) != 0 && !reachable[function] {
			violations = append(violations, traversals...)
		}
	}
	ast.Inspect(file, func(node ast.Node) bool {
		identifier, ok := node.(*ast.Ident)
		if !ok {
			return true
		}
		object := info.Uses[identifier]
		if object == nil {
			object = info.Defs[identifier]
		}
		if object == nil {
			return true
		}
		switch object.Name() {
		case "NativeProjection", "NativeContract":
			violations = append(violations, wirTraversalViolation{
				file: fileName, line: fset.Position(identifier.Pos()).Line,
				kind: "semantic native carrier in typed topology emitter",
			})
		}
		return true
	})
	return violations
}

func signatureContainsNativeTopologyDraft(value types.Type) bool {
	signature, ok := value.(*types.Signature)
	if !ok {
		return false
	}
	results := signature.Results()
	for index := 0; index < results.Len(); index++ {
		if typeContainsNamed(results.At(index).Type(), modulePath+"/analysis/check/fixpoint/front", "NativeTopologyDraft") {
			return true
		}
	}
	return false
}

func typeContainsNamed(value types.Type, packagePath, name string) bool {
	value = types.Unalias(value)
	switch item := value.(type) {
	case *types.Named:
		return item.Obj() != nil && item.Obj().Pkg() != nil &&
			item.Obj().Pkg().Path() == packagePath && item.Obj().Name() == name
	case *types.Pointer:
		return typeContainsNamed(item.Elem(), packagePath, name)
	case *types.Slice:
		return typeContainsNamed(item.Elem(), packagePath, name)
	case *types.Array:
		return typeContainsNamed(item.Elem(), packagePath, name)
	default:
		return false
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
	node ast.Node,
	info *types.Info,
) []wirTraversalViolation {
	var violations []wirTraversalViolation
	report := func(node ast.Node, kind string) {
		violations = append(violations, wirTraversalViolation{
			file: fileName, line: fset.Position(node.Pos()).Line, kind: kind,
		})
	}
	ast.Inspect(node, func(node ast.Node) bool {
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
	if len(sanctionedWIRTraversalFiles[modulePath+"/analysis/check/fixpoint/front"]) != 3 {
		t.Fatal("whole-file front traversal sanction must remain shrunk to three lowering owners")
	}
}

func TestTypedTopologyTraversalGuardRejectsSemanticAndOrphanMutations(t *testing.T) {
	loader := newFencePackageLoader(t, "./analysis/check/fixpoint/front")
	tests := map[string]string{
		"orphan traversal": `package front
import "` + modulePath + `/analysis/ir/wir"
type NativeTopologyDraft struct{}
func drafts() []NativeTopologyDraft { return nil }
func scan(body *wir.Body) {
	for index := 0; index < body.Len(); index++ { _ = body.Instr(index).Op }
}`,
		"semantic carrier": `package front
import "` + modulePath + `/analysis/ir/wir"
type NativeTopologyDraft struct{}
type NativeProjection struct{ Value string }
func drafts(body *wir.Body) []NativeTopologyDraft {
	for index := 0; index < body.Len(); index++ { _ = body.Instr(index).Op }
	_ = NativeProjection{Value: "completeness=complete"}
	return nil
}`,
	}
	for name, source := range tests {
		t.Run(name, func(t *testing.T) {
			typed := loader.source(modulePath+"/analysis/check/fixpoint/front", source)
			violations := findTypedTopologyDraftTraversalViolations(
				typed.fset, nativeTopologyDraftEmitterFile, typed.files[0], typed.info,
			)
			if len(violations) == 0 {
				t.Fatal("typed topology traversal fence accepted mutation")
			}
		})
	}
}

func TestSanctionedNativeWIRTraversalReferenceSetIsPinned(t *testing.T) {
	loader := newFencePackageLoader(t, "./analysis/check/fixpoint/front")
	meta := loader.metas[modulePath+"/analysis/check/fixpoint/front"]
	references := fenceWIRTraversalReferences(loader.load(meta))
	count := 0
	for _, reference := range references {
		if filepath.Base(strings.SplitN(reference, ":", 2)[0]) == nativeTopologyDraftEmitterFile {
			count++
		}
	}
	// The sanctioned topology emitter owns eleven bounded Body.Len/Body.Instr
	// pairs. Any additional traversal, including the rescan5 loop mutation,
	// changes this closed census and requires an ownership review.
	if count != 22 {
		t.Fatalf("%s traversal references = %d, want pinned 22", nativeTopologyDraftEmitterFile, count)
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
