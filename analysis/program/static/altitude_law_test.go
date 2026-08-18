package static

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

var altitudeNames = [...]string{"core", "read", "codec", "identity", "build"}

// fileAltitude derives a file's altitude from its name. The mapping is the one
// documented in doc.go: a new file declares its altitude by what it is called,
// and an unrecognized name is build, the only altitude that may reference
// everything.
func fileAltitude(name string) int {
	switch {
	case strings.HasPrefix(name, "artifact_section_"):
		return altitudeCodec
	case name == "content.go" || name == "identity.go":
		return altitudeIdentity
	case strings.HasPrefix(name, "query") || name == "counts.go" || name == "lifecycle_view.go":
		return altitudeRead
	case name == "doc.go" || name == "model.go" || name == "families.go" || name == "api.go" ||
		strings.HasSuffix(name, "_model.go"):
		return altitudeCore
	default:
		return altitudeBuild
	}
}

// packageSymbolOwners maps every package-level declared name to the file that
// declares it. Methods are excluded: a method is reached through its receiver
// type, which the receiver's own declaration already places at an altitude.
func packageSymbolOwners(t *testing.T, files map[string]*ast.File) map[string]string {
	t.Helper()
	owners := make(map[string]string)
	for name, file := range files {
		for _, decl := range file.Decls {
			switch typed := decl.(type) {
			case *ast.FuncDecl:
				if typed.Recv == nil {
					owners[typed.Name.Name] = name
				}
			case *ast.GenDecl:
				for _, spec := range typed.Specs {
					switch value := spec.(type) {
					case *ast.TypeSpec:
						owners[value.Name.Name] = name
					case *ast.ValueSpec:
						for _, ident := range value.Names {
							owners[ident.Name] = name
						}
					}
				}
			}
		}
	}
	return owners
}

// nonReferenceIdents collects the identifiers that name something other than a
// package-level symbol use: selector fields, composite-literal keys, struct
// field names, and a function's own name at its declaration.
func nonReferenceIdents(file *ast.File) map[*ast.Ident]bool {
	skip := make(map[*ast.Ident]bool)
	ast.Inspect(file, func(node ast.Node) bool {
		switch typed := node.(type) {
		case *ast.SelectorExpr:
			skip[typed.Sel] = true
		case *ast.KeyValueExpr:
			if ident, ok := typed.Key.(*ast.Ident); ok {
				skip[ident] = true
			}
		case *ast.StructType:
			for _, field := range typed.Fields.List {
				for _, ident := range field.Names {
					skip[ident] = true
				}
			}
		case *ast.FuncDecl:
			skip[typed.Name] = true
		}
		return true
	})
	return skip
}

func parsePackageFiles(t *testing.T) map[string]*ast.File {
	t.Helper()
	fset := token.NewFileSet()
	packages, err := parser.ParseDir(fset, ".", func(info os.FileInfo) bool {
		return !strings.HasSuffix(info.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatalf("parse package: %v", err)
	}
	files := make(map[string]*ast.File)
	for _, pkg := range packages {
		for path, file := range pkg.Files {
			files[filepath.Base(path)] = file
		}
	}
	if len(files) == 0 {
		t.Fatal("parsed no package files")
	}
	return files
}

// TestAltitudeLawIsOneWay proves no file names a symbol declared at a higher
// altitude than its own. It is the mechanical form of the law in doc.go.
func TestAltitudeLawIsOneWay(t *testing.T) {
	files := parsePackageFiles(t)
	owners := packageSymbolOwners(t, files)

	var violations []string
	for name, file := range files {
		altitude := fileAltitude(name)
		skip := nonReferenceIdents(file)
		reported := make(map[string]bool)
		ast.Inspect(file, func(node ast.Node) bool {
			ident, ok := node.(*ast.Ident)
			if !ok || skip[ident] {
				return true
			}
			owner, declared := owners[ident.Name]
			if !declared || owner == name {
				return true
			}
			ownerAltitude := fileAltitude(owner)
			if ownerAltitude <= altitude || reported[ident.Name] {
				return true
			}
			reported[ident.Name] = true
			violations = append(violations, name+" ("+altitudeNames[altitude]+") references "+
				ident.Name+" declared in "+owner+" ("+altitudeNames[ownerAltitude]+")")
			return true
		})
	}
	sort.Strings(violations)
	for _, violation := range violations {
		t.Errorf("altitude law: %s", violation)
	}
}

// TestAltitudeLawCoversEveryFile proves the altitude mapping is total and that
// every altitude the law orders is actually populated, so the one-way test
// cannot pass by classifying files into an empty lattice.
func TestAltitudeLawCoversEveryFile(t *testing.T) {
	files := parsePackageFiles(t)
	populated := make(map[int]int)
	for name := range files {
		altitude := fileAltitude(name)
		if altitude < altitudeCore || altitude > altitudeBuild {
			t.Fatalf("file %s has no altitude", name)
		}
		populated[altitude]++
	}
	for altitude := altitudeCore; altitude <= altitudeBuild; altitude++ {
		if populated[altitude] == 0 {
			t.Errorf("altitude %s has no files", altitudeNames[altitude])
		}
	}
}
