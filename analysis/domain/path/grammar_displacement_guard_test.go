package path

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"runtime"
	"testing"
)

func TestDisplacedPathGrammarsStayDeleted(t *testing.T) {
	_, here, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller did not locate grammar guard")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(here), "../../.."))
	forbidden := map[string]bool{
		"parseStableSymbolRootSuffix": true,
		"parseResolverRootSuffix":     true,
		"parseEncodedNamedRootSuffix": true,
		"parsePlainNamedRootSuffix":   true,
		"namedRootNeedsEncoding":      true,
		"PrefixedDecimalKey":          true,
		"LooksEncodedNamedRootKey":    true,
		"LooksStableSymbolRootSuffix": true,
		"LooksResolverRootSuffix":     true,
	}
	wrappers := map[string]bool{
		"StableKey":          true,
		"LocalKey":           true,
		"StateKey":           true,
		"SuffixKey":          true,
		"PlaceholderKey":     true,
		"RootPlaceholderKey": true,
		"StructuralKey":      true,
	}
	foundWrappers := make(map[string]bool, len(wrappers))
	err := filepath.WalkDir(root, func(filename string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(root, filename)
		if err != nil {
			return err
		}
		if entry.IsDir() {
			switch filepath.ToSlash(rel) {
			case ".git", "__legacy", "testdata":
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(filename) != ".go" || filepath.Base(filename) == "grammar_displacement_guard_test.go" {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), filename, nil, 0)
		if err != nil {
			return err
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if ok && forbidden[fn.Name.Name] {
				t.Errorf("%s: displaced path grammar function %s returned", rel, fn.Name.Name)
			}
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.TYPE {
				continue
			}
			for _, spec := range gen.Specs {
				typeSpec, ok := spec.(*ast.TypeSpec)
				if !ok || !wrappers[typeSpec.Name.Name] {
					continue
				}
				foundWrappers[typeSpec.Name.Name] = true
				if _, ok := typeSpec.Type.(*ast.StructType); !ok {
					t.Errorf("%s: semantic wrapper %s is string-constructible; want opaque struct", rel, typeSpec.Name.Name)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for name := range wrappers {
		if !foundWrappers[name] {
			t.Errorf("semantic path wrapper %s is absent", name)
		}
	}
}
