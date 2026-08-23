package schema

import (
	"fmt"
	"go/ast"
	"go/types"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"
)

// CX-07 is the substrate boundary. Domain packages hand the engine a cold
// Rule declaration and a fold; they do not retain the engine's runtime
// binding vocabulary or reintroduce a per-domain hot wrapper. This law is
// deliberately a production-source gate: test helpers may still use legacy
// names while a migration is in flight, but a shipped domain cannot add more
// residue.
//
// The scan is typed rather than a filename or text census. Package scope
// objects establish declarations, and go/types resolves aliases, import
// renames, pointers, and generic instantiations for retained engine handles.
// The engine package itself is outside the scanned domain prefix, so its
// canonical RuleImplementation and Read definitions remain legal.
func TestCX07DomainHotSubstrateHasZeroResidue(t *testing.T) {
	violations := cx07DomainResidues(t)
	if len(violations) == 0 {
		return
	}
	sort.Slice(violations, func(i, j int) bool { return violations[i].String() < violations[j].String() })
	var report strings.Builder
	for _, violation := range violations {
		fmt.Fprintln(&report, violation.String())
	}
	t.Fatalf("CX-07 domain substrate residue: %d violation(s)\n%s", len(violations), report.String())
}

const (
	cx07Module       = "github.com/wippyai/go-lua"
	cx07DomainPrefix = cx07Module + "/domain/"
	cx07Engine       = cx07Module + "/analysis/engine"
)

type cx07Residue struct {
	packagePath string
	file        string
	line        int
	column      int
	detail      string
}

func (residue cx07Residue) String() string {
	return fmt.Sprintf("%s:%d:%d: %s (%s)", residue.file, residue.line, residue.column, residue.detail, residue.packagePath)
}

// cx07DomainResidues loads production domain packages with syntax and type
// information. NeedDeps is required: TypeOf(field.Type) must resolve the
// imported engine package, including when a domain gives that import another
// local name or aliases a handle through a declaration of its own.
func cx07DomainResidues(t *testing.T) []cx07Residue {
	t.Helper()
	repository := cx07RepositoryRoot(t)
	cache, err := os.MkdirTemp("", "go-lua-cx07-gocache-")
	if err != nil {
		t.Fatalf("create CX-07 package cache: %v", err)
	}
	defer os.RemoveAll(cache)

	config := &packages.Config{
		Dir:  repository,
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles | packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo | packages.NeedImports | packages.NeedDeps,
		// The gate describes shipped domain code. A test package is not a
		// production declaration and must not hide or create a residue.
		Tests: false,
		// The default build cache is read-only in some verification runners.
		// Package loading is still deterministic with a private ephemeral cache.
		Env: append(os.Environ(), "GOCACHE="+cache),
	}
	loaded, err := packages.Load(config, cx07DomainPrefix+"...")
	if err != nil {
		t.Fatalf("load CX-07 domain packages: %v", err)
	}
	if len(loaded) == 0 {
		t.Fatal("CX-07 loaded no production domain packages")
	}

	var residues []cx07Residue
	for _, pkg := range loaded {
		if pkg.PkgPath == "" || !strings.HasPrefix(pkg.PkgPath, cx07DomainPrefix) || pkg.PkgPath == cx07Engine {
			continue
		}
		if len(pkg.Errors) != 0 {
			for _, packageErr := range pkg.Errors {
				t.Errorf("load CX-07 package %s: %v", pkg.PkgPath, packageErr)
			}
			continue
		}
		if pkg.Types == nil || pkg.TypesInfo == nil || pkg.Fset == nil {
			t.Errorf("load CX-07 package %s without syntax/type information", pkg.PkgPath)
			continue
		}
		residues = append(residues, cx07DeclaredResidues(pkg)...)
	}
	return residues
}

func cx07DeclaredResidues(pkg *packages.Package) []cx07Residue {
	var residues []cx07Residue
	// Scope lookup is a typed declaration check. PkgName objects are imports,
	// not declarations made by the package, and are therefore excluded.
	for _, name := range []string{"HotRule", "HotOwner", "BindHot"} {
		object := pkg.Types.Scope().Lookup(name)
		if object == nil {
			continue
		}
		if _, imported := object.(*types.PkgName); imported {
			continue
		}
		position := pkg.Fset.PositionFor(object.Pos(), false)
		residues = append(residues, cx07Residue{
			packagePath: pkg.PkgPath,
			file:        position.Filename,
			line:        position.Line,
			column:      position.Column,
			detail:      "forbidden production declaration " + name,
		})
	}

	for _, file := range pkg.Syntax {
		ast.Inspect(file, func(node ast.Node) bool {
			structType, isStruct := node.(*ast.StructType)
			if !isStruct {
				return true
			}
			for _, field := range structType.Fields.List {
				typ := pkg.TypesInfo.TypeOf(field.Type)
				if typ == nil {
					// A package type error is reported by the loader. Keeping the
					// structural walk total makes this law useful while a migration
					// is temporarily uncompilable as well.
					continue
				}
				forbidden := cx07EngineHandleIn(typ, make(map[types.Type]bool))
				if forbidden == "" {
					continue
				}
				position := pkg.Fset.PositionFor(field.Pos(), false)
				fieldName := "<embedded>"
				if len(field.Names) != 0 {
					fieldName = field.Names[0].Name
				}
				residues = append(residues, cx07Residue{
					packagePath: pkg.PkgPath,
					file:        position.Filename,
					line:        position.Line,
					column:      position.Column,
					detail:      fmt.Sprintf("struct field %q retains engine.%s", fieldName, forbidden),
				})
			}
			return true
		})
	}
	return residues
}

// cx07EngineHandleIn resolves a field's type through value-bearing wrappers.
// It intentionally does not inspect function signatures or the implementation
// fields of an unrelated named struct: those are not a retained Read or
// RuleImplementation field. A generic argument is inspected because a domain
// container that stores an engine handle still retains that handle.
func cx07EngineHandleIn(typ types.Type, seen map[types.Type]bool) string {
	if typ == nil {
		return ""
	}
	typ = types.Unalias(typ)
	if seen[typ] {
		return ""
	}
	seen[typ] = true

	switch current := typ.(type) {
	case *types.Pointer:
		return cx07EngineHandleIn(current.Elem(), seen)
	case *types.Slice:
		return cx07EngineHandleIn(current.Elem(), seen)
	case *types.Array:
		return cx07EngineHandleIn(current.Elem(), seen)
	case *types.Map:
		if forbidden := cx07EngineHandleIn(current.Key(), seen); forbidden != "" {
			return forbidden
		}
		return cx07EngineHandleIn(current.Elem(), seen)
	case *types.Chan:
		return cx07EngineHandleIn(current.Elem(), seen)
	case *types.Named:
		object := current.Obj()
		if object != nil && object.Pkg() != nil && object.Pkg().Path() == cx07Engine && (object.Name() == "Read" || object.Name() == "RuleImplementation") {
			return object.Name()
		}
		arguments := current.TypeArgs()
		for index := 0; index < arguments.Len(); index++ {
			if forbidden := cx07EngineHandleIn(arguments.At(index), seen); forbidden != "" {
				return forbidden
			}
		}
	}
	return ""
}

func cx07RepositoryRoot(t *testing.T) string {
	t.Helper()
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("CX-07 source location unavailable")
	}
	directory := filepath.Dir(current)
	for {
		if _, err := os.Stat(filepath.Join(directory, "go.mod")); err == nil {
			return directory
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			t.Fatal("CX-07 repository root unavailable")
		}
		directory = parent
	}
}
