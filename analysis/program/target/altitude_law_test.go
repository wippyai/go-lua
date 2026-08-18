package target

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

// The package is layered into four altitudes, documented in doc.go. This file is
// the standing statement of the direction: a reference crosses altitudes downward
// or not at all. Core is closed - an invariant that has to ask a reader is not an
// invariant of the model - and the seal is terminal, so nothing below it can name
// the freeze that produced it.
//
// The law is stated over files rather than over symbols because the altitude is a
// property of where a declaration lives. A declaration that has to move to keep the
// direction is the finding; moving the file assignment to silence this test is not.

// altitudeRank orders the four altitudes. A reference may target its own rank or a
// lower one, never a higher one.
var altitudeRank = map[string]int{
	"core":     0,
	"read":     1,
	"identity": 2,
	"seal":     3,
}

// altitudeOf assigns every production file in the package to its altitude. The map
// is exhaustive by test: a new file that is not listed here fails.
var altitudeOf = map[string]string{
	"doc.go": "core",

	"model_rows.go":        "core",
	"model_invariants.go":  "core",
	"model_publication.go": "core",
	"spec.go":              "core",
	"checked.go":           "core",

	"operation_query.go":    "read",
	"invocation_query.go":   "read",
	"continuation_query.go": "read",
	"boot_query.go":         "read",
	"protocol_query.go":     "read",
	"subedge_relation.go":   "read",
	"counts.go":             "read",

	"contentid.go":           "identity",
	"contentid_boot.go":      "identity",
	"contentid_operation.go": "identity",
	"contentid_protocol.go":  "identity",
	"contentid_subedge.go":   "identity",
	"contentid_value.go":     "identity",
	"semantic_identity.go":   "identity",
	"effect_identity.go":     "identity",
	"relation_identity.go":   "identity",
	"identity_encoding.go":   "identity",

	"seal.go":                  "seal",
	"seal_append.go":           "seal",
	"seal_drafts.go":           "seal",
	"seal_operation.go":        "seal",
	"seal_relations.go":        "seal",
	"seal_resolution.go":       "seal",
	"seal_validation.go":       "seal",
	"subedge_freeze.go":        "seal",
	"subedge_relation_seal.go": "seal",
	"subedge_rows.go":          "seal",
	"subedge_validation.go":    "seal",
	"boot.go":                  "seal",
	"protocol.go":              "seal",
	"exact_key.go":             "seal",
	"exact_key_freeze.go":      "seal",
	"values_projection.go":     "seal",

	"seal_operation_outcome.go":      "seal",
	"seal_operation_continuation.go": "seal",
	"seal_append_invocation.go":      "seal",
	"seal_append_continuation.go":    "seal",
	"seal_append_outcome.go":         "seal",
	"seal_resolution_order.go":       "seal",
	"seal_resolution_validity.go":    "seal",
	"subedge_freeze_argument.go":     "seal",
	"subedge_freeze_route.go":        "seal",
}

// TestAltitudeReferencesRunOneWay states the direction over every cross-file
// reference in the package.
func TestAltitudeReferencesRunOneWay(t *testing.T) {
	pkg := parseAltitudePackage(t)
	for _, name := range pkg.fileNames {
		from, known := altitudeOf[name]
		if !known {
			t.Errorf("file %s has no altitude; assign it in altitudeOf and in doc.go", name)
			continue
		}
		for _, ref := range pkg.references(name) {
			to := altitudeOf[ref.file]
			if altitudeRank[to] <= altitudeRank[from] {
				continue
			}
			t.Errorf("%s (%s) names %s declared in %s (%s): references run downward only",
				name, from, ref.name, ref.file, to)
		}
	}
}

// TestAltitudeMapCoversPackage states that the altitude map names exactly the
// production files that exist.
func TestAltitudeMapCoversPackage(t *testing.T) {
	pkg := parseAltitudePackage(t)
	present := map[string]bool{}
	for _, name := range pkg.fileNames {
		present[name] = true
		if _, known := altitudeOf[name]; !known {
			t.Errorf("production file %s is not assigned an altitude", name)
		}
	}
	for name := range altitudeOf {
		if !present[name] {
			t.Errorf("altitude map names %s, which the package does not contain", name)
		}
	}
}

type altitudeReference struct {
	name string
	file string
}

type altitudePackage struct {
	fileNames []string
	files     map[string]*ast.File
	// declFile maps a package-scope declaration to the file declaring it.
	declFile map[string]string
	// methodFile maps a method name to the file declaring it, for method names
	// that are declared exactly once and shadow no field or package-scope name.
	methodFile map[string]string
}

// references reports the declarations one file names that another file declares.
func (p *altitudePackage) references(name string) []altitudeReference {
	file := p.files[name]
	imports := map[string]bool{}
	for _, spec := range file.Imports {
		local := ""
		if spec.Name != nil {
			local = spec.Name.Name
		} else {
			path := strings.Trim(spec.Path.Value, `"`)
			local = path[strings.LastIndex(path, "/")+1:]
		}
		imports[local] = true
	}

	seen := map[altitudeReference]bool{}
	var out []altitudeReference
	record := func(ident string, declared map[string]string) {
		target, ok := declared[ident]
		if !ok || target == name {
			return
		}
		ref := altitudeReference{name: ident, file: target}
		if seen[ref] {
			return
		}
		seen[ref] = true
		out = append(out, ref)
	}

	// Package-scope references. The parser resolves file-local identifiers, so an
	// unresolved identifier is either declared by a sibling file, imported, or
	// predeclared; only the first matches declFile.
	for _, ident := range file.Unresolved {
		record(ident.Name, p.declFile)
	}

	// Method references. A selector on an imported package name is a qualified
	// identifier, not a method call, so those are skipped.
	ast.Inspect(file, func(node ast.Node) bool {
		selector, ok := node.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if base, ok := selector.X.(*ast.Ident); ok && imports[base.Name] {
			return true
		}
		record(selector.Sel.Name, p.methodFile)
		return true
	})

	sort.Slice(out, func(i, j int) bool {
		if out[i].file != out[j].file {
			return out[i].file < out[j].file
		}
		return out[i].name < out[j].name
	})
	return out
}

func parseAltitudePackage(t *testing.T) *altitudePackage {
	t.Helper()
	_, self, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("no caller information")
	}
	dir := filepath.Dir(self)
	paths, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatalf("glob %s: %v", dir, err)
	}

	pkg := &altitudePackage{
		files:      map[string]*ast.File{},
		declFile:   map[string]string{},
		methodFile: map[string]string{},
	}
	fileset := token.NewFileSet()
	methodCount := map[string]int{}
	fieldNames := map[string]bool{}

	sort.Strings(paths)
	for _, path := range paths {
		name := filepath.Base(path)
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		parsed, err := parser.ParseFile(fileset, path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		pkg.fileNames = append(pkg.fileNames, name)
		pkg.files[name] = parsed

		for _, decl := range parsed.Decls {
			switch typed := decl.(type) {
			case *ast.FuncDecl:
				if typed.Recv != nil {
					methodCount[typed.Name.Name]++
					pkg.methodFile[typed.Name.Name] = name
					continue
				}
				pkg.declFile[typed.Name.Name] = name
			case *ast.GenDecl:
				if typed.Tok == token.IMPORT {
					continue
				}
				for _, spec := range typed.Specs {
					switch spec := spec.(type) {
					case *ast.TypeSpec:
						pkg.declFile[spec.Name.Name] = name
					case *ast.ValueSpec:
						for _, ident := range spec.Names {
							pkg.declFile[ident.Name] = name
						}
					}
				}
			}
		}
		ast.Inspect(parsed, func(node ast.Node) bool {
			structure, ok := node.(*ast.StructType)
			if !ok {
				return true
			}
			for _, field := range structure.Fields.List {
				for _, ident := range field.Names {
					fieldNames[ident.Name] = true
				}
			}
			return true
		})
	}

	// A method name that is declared twice, or that collides with a field or a
	// package-scope declaration, cannot be attributed to one file by name alone.
	for method, count := range methodCount {
		_, isDecl := pkg.declFile[method]
		if count > 1 || isDecl || fieldNames[method] {
			delete(pkg.methodFile, method)
		}
	}
	return pkg
}
