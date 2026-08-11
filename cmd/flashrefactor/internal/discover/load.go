package discover

import (
	"fmt"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type checkedPackage struct {
	path  string
	fset  *token.FileSet
	files []*ast.File
	info  *types.Info
	pkg   *types.Package
	byPos map[token.Pos]string
}

// AnalyzeFiles parses and type-checks an exact package source set. It is the
// deterministic entry point for tools and tests that already own the source.
// All files must declare the same package; type errors are returned unchanged.
func AnalyzeFiles(importPath string, source map[string]string) (Report, error) {
	if importPath == "" {
		return Report{}, fmt.Errorf("discover: empty import path")
	}
	if len(source) == 0 {
		return Report{}, fmt.Errorf("discover: empty source set")
	}
	fset := token.NewFileSet()
	paths := make([]string, 0, len(source))
	for path := range source {
		paths = append(paths, filepath.ToSlash(path))
	}
	sort.Strings(paths)
	files := make([]*ast.File, 0, len(paths))
	byPos := make(map[token.Pos]string)
	var pkgName string
	for _, path := range paths {
		file, err := parser.ParseFile(fset, path, source[path], parser.ParseComments)
		if err != nil {
			return Report{}, fmt.Errorf("discover: parse %s: %w", path, err)
		}
		if pkgName == "" {
			pkgName = file.Name.Name
		} else if pkgName != file.Name.Name {
			return Report{}, fmt.Errorf("discover: mixed packages %q and %q", pkgName, file.Name.Name)
		}
		files = append(files, file)
		byPos[file.Pos()] = path
	}
	info := typeInfo()
	pkg, err := (&types.Config{Importer: importer.Default()}).Check(importPath, fset, files, info)
	if err != nil {
		return Report{}, fmt.Errorf("discover: type-check %s: %w", importPath, err)
	}
	return analyze(checkedPackage{path: importPath, fset: fset, files: files, info: info, pkg: pkg, byPos: byPos}), nil
}

// AnalyzeDir reads one package directory and rejects parse/type errors. It
// includes in-package tests (package p) so test ownership evidence remains
// available, but deliberately excludes external tests (package p_test): those
// are a distinct package and require their own explicit source closure.
func AnalyzeDir(dir string) (Report, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return Report{}, err
	}
	source := map[string]string{}
	testFiles := map[string]string{}
	packageName := ""
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		body, err := os.ReadFile(path)
		if err != nil {
			return Report{}, err
		}
		key := filepath.ToSlash(path)
		if strings.HasSuffix(entry.Name(), "_test.go") {
			testFiles[key] = string(body)
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), key, body, parser.PackageClauseOnly)
		if err != nil {
			return Report{}, fmt.Errorf("discover: parse %s: %w", key, err)
		}
		if packageName == "" {
			packageName = file.Name.Name
		}
		source[key] = string(body)
	}
	for path, body := range testFiles {
		file, err := parser.ParseFile(token.NewFileSet(), path, body, parser.PackageClauseOnly)
		if err != nil {
			return Report{}, fmt.Errorf("discover: parse %s: %w", path, err)
		}
		if file.Name.Name == packageName {
			source[path] = body
		}
	}
	if len(source) == 0 {
		return Report{}, fmt.Errorf("discover: no non-test Go files in %s", dir)
	}
	return AnalyzeFiles(filepath.ToSlash(dir), source)
}

func typeInfo() *types.Info {
	return &types.Info{
		Defs:       make(map[*ast.Ident]types.Object),
		Uses:       make(map[*ast.Ident]types.Object),
		Selections: make(map[*ast.SelectorExpr]*types.Selection),
		Types:      make(map[ast.Expr]types.TypeAndValue),
		Scopes:     make(map[ast.Node]*types.Scope),
	}
}

// CallerPackages reports declared imports of targetImport under root. It does
// not claim that an import is semantically used: the evidence says exactly
// "declared import", avoiding text guesses about selector binding in foreign
// package closures. Parse errors fail closed.
func CallerPackages(root, targetImport string) ([]Candidate, error) {
	if targetImport == "" {
		return nil, fmt.Errorf("discover: empty target import")
	}
	type group struct {
		pkg       string
		positions []Position
	}
	groups := map[string]*group{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() && (entry.Name() == ".git" || entry.Name() == "vendor") {
			return filepath.SkipDir
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
			return nil
		}
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			return fmt.Errorf("discover: parse %s: %w", path, err)
		}
		for _, spec := range file.Imports {
			value := strings.Trim(spec.Path.Value, "\"")
			if value != targetImport {
				continue
			}
			pos := position(fset, spec.Pos())
			key := filepath.ToSlash(filepath.Dir(path)) + ":" + file.Name.Name
			if groups[key] == nil {
				groups[key] = &group{pkg: file.Name.Name}
			}
			groups[key].positions = append(groups[key].positions, pos)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	candidates := make([]Candidate, 0, len(groups))
	for key, group := range groups {
		sortPositions(group.positions)
		candidates = append(candidates, Candidate{
			Kind: CallerPackage, Key: key + ":" + targetImport, Package: group.pkg,
			Symbols:   []Symbol{{ID: "import:" + targetImport, Name: targetImport, Kind: "import", Position: group.positions[0]}},
			Positions: group.positions, Reasons: []string{"declared import of target package"}, Confidence: "high",
			Evidence: []Evidence{{Code: "declared-import", Detail: targetImport, Count: len(group.positions)}},
		})
	}
	sortCandidates(candidates)
	return candidates, nil
}
