package architecture_test

import (
	"go/ast"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// Roots are subsystem boundaries, not a package inventory. WalkDir keeps
// child packages under an owned prefix in scope without naming them here.
var w0GenericRoots = []string{
	"analysis/relation",
	"analysis/engine/relation",
	"analysis/program/relationadmission",
	"analysis/schema/rule/relcompile",
	"analysis/schema/rule/relbindgen",
	"internal/relationoracle",
}

const w0ModulePath = "github.com/wippyai/go-lua"

type w0Source struct {
	path string
	file *ast.File
}

func TestW0GenericSourceBoundaries(t *testing.T) {
	sources := w0ProductionSources(t)
	aliases := make(map[string]map[string]bool)
	for _, source := range sources {
		packageDir := filepath.ToSlash(filepath.Dir(source.path))
		if aliases[packageDir] == nil {
			aliases[packageDir] = make(map[string]bool)
		}
		for _, declaration := range source.file.Decls {
			group, ok := declaration.(*ast.GenDecl)
			if !ok || group.Tok != token.TYPE {
				continue
			}
			for _, specification := range group.Specs {
				typeSpec, ok := specification.(*ast.TypeSpec)
				if ok && w0ContainsFuncType(typeSpec.Type, nil) {
					aliases[packageDir][typeSpec.Name.Name] = true
				}
			}
		}
	}

	for _, source := range sources {
		packageDir := filepath.ToSlash(filepath.Dir(source.path))
		for _, imported := range w0Imports(t, source) {
			if reason := w0ForbiddenImportReason(imported); reason != "" {
				t.Errorf("%s imports %s: %s", source.path, imported, reason)
			}
			if imported == "reflect" || strings.HasPrefix(imported, "reflect/") {
				t.Errorf("%s imports reflection package %s; payloads cross only the typed semantic ABI", source.path, imported)
			}
		}
		ast.Inspect(source.file, func(node ast.Node) bool {
			switch typed := node.(type) {
			case *ast.StructType:
				for _, field := range typed.Fields.List {
					if w0ContainsUntypedPayloadType(field.Type) {
						name := "<anonymous>"
						if len(field.Names) != 0 {
							name = field.Names[0].Name
						}
						t.Errorf("%s stores untyped payload field %s at %s", source.path, name, w0Position(source.file, field.Pos()))
					}
					if !w0ContainsFuncType(field.Type, aliases[packageDir]) {
						continue
					}
					name := "<anonymous>"
					if len(field.Names) != 0 {
						name = field.Names[0].Name
					}
					t.Errorf("%s stores generic function field %s at %s", source.path, name, w0Position(source.file, field.Pos()))
				}
			}
			return true
		})
	}
}

// Logical schema names may describe access requirements and semantic ordering,
// but cannot declare concrete physical operators or storage handles.
func TestW0LogicalSchemaRejectsPhysicalOperatorNames(t *testing.T) {
	for _, source := range w0SourcesUnder(t, "analysis/relation/schema") {
		if strings.HasSuffix(source.path, "_test.go") {
			continue
		}
		ast.Inspect(source.file, func(node ast.Node) bool {
			switch typed := node.(type) {
			case *ast.Ident:
				if vocabulary := w0PhysicalIdentifier(typed.Name); vocabulary != "" {
					t.Errorf("%s names physical operator/type %q at %s", source.path, vocabulary, w0Position(source.file, typed.Pos()))
				}
			case *ast.TypeSpec:
				if vocabulary := w0PhysicalFieldName(typed.Name.Name); vocabulary != "" {
					t.Errorf("%s names physical schema type %q at %s", source.path, vocabulary, w0Position(source.file, typed.Name.Pos()))
				}
			case *ast.StructType:
				for _, field := range typed.Fields.List {
					for _, name := range field.Names {
						if vocabulary := w0PhysicalFieldName(name.Name); vocabulary != "" {
							t.Errorf("%s stores physical schema field %q at %s", source.path, vocabulary, w0Position(source.file, name.Pos()))
						}
					}
				}
			}
			return true
		})
	}
}

func w0ForbiddenImportReason(imported string) string {
	if !strings.HasPrefix(imported, w0ModulePath+"/") {
		return ""
	}
	relative := strings.TrimPrefix(imported, w0ModulePath+"/")
	if relative == "domain" || strings.HasPrefix(relative, "domain/") {
		return "generic W0 packages cannot import domain implementations"
	}
	for _, old := range []string{
		"analysis/engine/execution",
		"analysis/engine/internal/carrier",
		"analysis/engine/internal/factbinding",
		"analysis/engine/internal/demand",
		"analysis/engine/internal/equation",
		"analysis/engine/internal/linkexecutionplan",
		"analysis/engine/internal/executioncatalog",
	} {
		if relative == old || strings.HasPrefix(relative, old+"/") {
			return "old form/execution protocol is not a W0 dependency"
		}
	}
	return ""
}

// This check is deliberately applied only to struct fields. A type parameter
// constraint such as [K any] and a local generic helper are not payload boxes;
// a field whose storage type is any or interface{} is.
func w0ContainsUntypedPayloadType(expression ast.Expr) bool {
	found := false
	ast.Inspect(expression, func(node ast.Node) bool {
		switch typed := node.(type) {
		case *ast.Ident:
			found = typed.Name == "any"
		case *ast.InterfaceType:
			found = typed.Methods == nil || len(typed.Methods.List) == 0
		}
		return !found
	})
	return found
}

func w0ContainsFuncType(expression ast.Expr, aliases map[string]bool) bool {
	switch typed := expression.(type) {
	case *ast.FuncType:
		return true
	case *ast.Ident:
		return aliases != nil && aliases[typed.Name]
	case *ast.ParenExpr:
		return w0ContainsFuncType(typed.X, aliases)
	case *ast.StarExpr:
		return w0ContainsFuncType(typed.X, aliases)
	case *ast.ArrayType:
		return w0ContainsFuncType(typed.Elt, aliases)
	case *ast.MapType:
		return w0ContainsFuncType(typed.Key, aliases) || w0ContainsFuncType(typed.Value, aliases)
	case *ast.ChanType:
		return w0ContainsFuncType(typed.Value, aliases)
	case *ast.Ellipsis:
		return w0ContainsFuncType(typed.Elt, aliases)
	case *ast.IndexExpr:
		return w0ContainsFuncType(typed.X, aliases) || w0ContainsFuncType(typed.Index, aliases)
	case *ast.IndexListExpr:
		if w0ContainsFuncType(typed.X, aliases) {
			return true
		}
		for _, index := range typed.Indices {
			if w0ContainsFuncType(index, aliases) {
				return true
			}
		}
	}
	return false
}

func w0PhysicalIdentifier(name string) string {
	switch strings.ToLower(name) {
	case "hashjoin", "mergejoin", "mtbdd", "physicalordinal", "physicalslot", "physicalhandle", "storageordinal", "storageslot", "storagehandle", "runtimeordinal", "runtimeslot", "runtimehandle", "localordinal", "localslot", "rowordinal":
		return name
	}
	return ""
}

func w0PhysicalFieldName(name string) string {
	switch strings.ToLower(name) {
	case "ordinal", "slot", "handle":
		return name
	}
	return ""
}

func w0ProductionSources(t *testing.T) []w0Source {
	t.Helper()
	all := make([]w0Source, 0, 32)
	for _, root := range w0GenericRoots {
		for _, source := range w0SourcesUnder(t, root) {
			if !strings.HasSuffix(source.path, "_test.go") {
				all = append(all, source)
			}
		}
	}
	return all
}

func w0SourcesUnder(t *testing.T, root string) []w0Source {
	t.Helper()
	repository := repositoryRoot(t)
	directory := filepath.Join(repository, filepath.FromSlash(root))
	if _, err := os.Stat(directory); err != nil {
		t.Fatalf("W0 source root %s is unavailable: %v", root, err)
	}
	sources, err := architectureSourcesUnderSkippingHidden(repository, root)
	if err != nil {
		t.Fatalf("walk W0 source root %s: %v", root, err)
	}
	return sources
}

func w0SourcePathHasHiddenDirectory(path string) bool {
	parts := strings.Split(path, "/")
	if len(parts) < 2 {
		return false
	}
	for _, component := range parts[:len(parts)-1] {
		if strings.HasPrefix(component, ".") {
			return true
		}
	}
	return false
}

func w0Imports(t *testing.T, source w0Source) []string {
	t.Helper()
	imports := make([]string, 0, len(source.file.Imports))
	for _, specification := range source.file.Imports {
		value, err := strconv.Unquote(specification.Path.Value)
		if err != nil {
			t.Fatalf("%s: unquote import %s: %v", source.path, specification.Path.Value, err)
		}
		imports = append(imports, value)
	}
	return imports
}

func w0Position(file *ast.File, position token.Pos) string {
	return "offset " + strconv.Itoa(int(position-file.Pos()))
}
