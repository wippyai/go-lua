package architecture_test

import (
	"go/ast"
	"go/token"
	"go/types"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"
)

const lowerStorePath = modulePath + "/analysis/engine/relation/state/store"

// TestW2OnlyDatabaseMayRedeemTheStorePublicationSurface is deliberately a
// source/type law rather than a convention check. Store is the physical
// column aggregate; database is the first owner that can attach arrangement
// successors and publish one complete W2 root. A lower store Commit function,
// or a caller that reaches Commit/Candidate through an alias or dot import,
// would create a second publication door.
func TestW2OnlyDatabaseMayRedeemTheStorePublicationSurface(t *testing.T) {
	root := repositoryRoot(t)
	checkLowerStoreDeclarations(t, root)
	production := lowerStoreProductionSources(t)
	packageDirs := make(map[string]struct{})
	for _, source := range production {
		if hasLowerStoreImport(source.file) {
			packageDirs[filepath.ToSlash(filepath.Dir(source.path))] = struct{}{}
		}
	}

	// Keep the fallback structural check independent of type loading: it still
	// catches a malformed/incomplete package while the go/types check below
	// gives the law the actual imported function object when available.
	for _, source := range production {
		if !hasLowerStoreImport(source.file) {
			continue
		}
		checkLowerStorePublicationSyntax(t, source)
	}

	config := &packages.Config{
		Dir:   root,
		Mode:  packages.NeedName | packages.NeedTypes | packages.NeedTypesInfo | packages.NeedSyntax | packages.NeedImports,
		Tests: false,
	}
	storePackages, err := packages.Load(config, "./analysis/engine/relation/state/store")
	if err != nil {
		t.Fatalf("load lower store package: %v", err)
	}
	if len(storePackages) != 1 || storePackages[0].Types == nil || packageLoadErrors(storePackages) {
		// The AST check above remains authoritative when another lane has left
		// an unrelated dependency temporarily untypeable. Once the package
		// graph is valid, this same law additionally resolves the actual
		// go/types object below.
		return
	}
	if object := storePackages[0].Types.Scope().Lookup("Commit"); object != nil {
		t.Fatalf("lower store still exports publication door %v", object)
	}

	patterns := make([]string, 0, len(packageDirs))
	for directory := range packageDirs {
		patterns = append(patterns, "./"+directory)
	}
	if len(patterns) == 0 {
		return
	}
	callerPackages, err := packages.Load(config, patterns...)
	if err != nil {
		t.Fatalf("load lower-store callers: %v", err)
	}
	if packageLoadErrors(callerPackages) {
		return
	}
	for _, caller := range callerPackages {
		if caller.TypesInfo == nil {
			t.Fatalf("package %s has no type information", caller.PkgPath)
		}
		for _, file := range caller.Syntax {
			ast.Inspect(file, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				identifier, ok := calledIdentifier(call.Fun)
				if !ok {
					return true
				}
				object := caller.TypesInfo.Uses[identifier]
				if lowerStorePublicationObject(object) {
					t.Errorf("%s calls lower store publication surface %s at %s", caller.PkgPath, object.Name(), tokenPosition(caller.Fset, identifier.Pos()))
				}
				return true
			})
		}
	}
}

func packageLoadErrors(packages []*packages.Package) bool {
	for _, value := range packages {
		if len(value.Errors) != 0 {
			return true
		}
	}
	return false
}

func checkLowerStoreDeclarations(t *testing.T, root string) {
	t.Helper()
	sources, err := architectureSourcesUnder(root, "analysis/engine/relation/state/store")
	if err != nil {
		t.Fatalf("walk lower store declarations: %v", err)
	}
	for _, source := range sources {
		if strings.HasSuffix(source.path, "_test.go") {
			continue
		}
		ast.Inspect(source.file, func(node ast.Node) bool {
			declaration, ok := node.(*ast.FuncDecl)
			if ok && (declaration.Name.Name == "Commit" || declaration.Name.Name == "Candidate") {
				t.Errorf("lower store declares forbidden publication surface %s in %s", declaration.Name.Name, filepath.ToSlash(filepath.Join(root, filepath.FromSlash(source.path))))
			}
			return true
		})
	}
}

func lowerStoreProductionSources(t *testing.T) []w0Source {
	t.Helper()
	root := repositoryRoot(t)
	sources := make([]w0Source, 0, 16)
	for _, sourceRoot := range []string{"analysis", "domain", "stdlib", "internal", "cmd"} {
		production, err := architectureSourcesUnderSkippingHidden(root, sourceRoot)
		if err != nil {
			t.Fatalf("walk lower-store source root %s: %v", sourceRoot, err)
		}
		for _, source := range production {
			if strings.HasSuffix(source.path, "_test.go") || isDatabaseProductionSource(source.path) || !hasLowerStoreImport(source.file) {
				continue
			}
			sources = append(sources, source)
		}
	}
	return sources
}

func isDatabaseProductionSource(path string) bool {
	path = filepath.ToSlash(path)
	return path == "analysis/engine/relation/state/database" ||
		strings.HasPrefix(path, "analysis/engine/relation/state/database/")
}

func hasLowerStoreImport(file *ast.File) bool {
	for _, specification := range file.Imports {
		path, err := strconv.Unquote(specification.Path.Value)
		if err == nil && path == lowerStorePath {
			return true
		}
	}
	return false
}

func checkLowerStorePublicationSyntax(t *testing.T, source w0Source) {
	t.Helper()
	aliases := make(map[string]bool)
	dotImport := false
	for _, specification := range source.file.Imports {
		path, err := strconv.Unquote(specification.Path.Value)
		if err != nil || path != lowerStorePath {
			continue
		}
		if specification.Name == nil {
			aliases["store"] = true
			continue
		}
		switch specification.Name.Name {
		case ".":
			dotImport = true
		case "_":
		default:
			aliases[specification.Name.Name] = true
		}
	}
	ast.Inspect(source.file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		switch function := call.Fun.(type) {
		case *ast.SelectorExpr:
			qualifier, qualifierOK := function.X.(*ast.Ident)
			if qualifierOK && aliases[qualifier.Name] && (function.Sel.Name == "Commit" || function.Sel.Name == "Candidate") {
				t.Errorf("%s calls lower store %s at %s", source.path, function.Sel.Name, w0Position(source.file, function.Sel.Pos()))
			}
		case *ast.Ident:
			if dotImport && (function.Name == "Commit" || function.Name == "Candidate") {
				t.Errorf("%s calls dot-imported lower store %s at %s", source.path, function.Name, w0Position(source.file, function.Pos()))
			}
		}
		return true
	})
}

func calledIdentifier(expression ast.Expr) (*ast.Ident, bool) {
	switch function := expression.(type) {
	case *ast.Ident:
		return function, true
	case *ast.SelectorExpr:
		return function.Sel, function.Sel != nil
	default:
		return nil, false
	}
}

func lowerStorePublicationObject(object types.Object) bool {
	if object == nil || object.Pkg() == nil || object.Pkg().Path() != lowerStorePath {
		return false
	}
	return object.Name() == "Commit" || object.Name() == "Candidate"
}

func tokenPosition(files *token.FileSet, position token.Pos) string {
	if files == nil {
		return "unknown position"
	}
	return files.Position(position).String()
}
