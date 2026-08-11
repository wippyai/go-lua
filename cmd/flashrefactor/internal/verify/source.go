package verify

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"sort"
	"strconv"
)

func validateSnapshot(snapshot Snapshot, label string) (map[string]SourceFile, error) {
	files := make(map[string]SourceFile, len(snapshot.Sources))
	for path, file := range snapshot.Sources {
		if path == "" || file.Path != path || file.Package == "" {
			return nil, fmt.Errorf("%s source has empty path or package", label)
		}
		if _, err := parser.ParseFile(token.NewFileSet(), file.Path, file.Source, parser.AllErrors); err != nil {
			return nil, fmt.Errorf("%s source %s does not parse: %w", label, file.Path, err)
		}
		files[file.Path] = file
	}
	return files, nil
}

func importSpecs(files map[string]SourceFile, label string) ([]ImportSpec, error) {
	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	result := make([]ImportSpec, 0)
	seen := map[string]bool{}
	for _, path := range paths {
		file := files[path]
		parsed, err := parser.ParseFile(token.NewFileSet(), file.Path, file.Source, parser.ImportsOnly)
		if err != nil {
			return nil, fmt.Errorf("%s source %s import parse: %w", label, file.Path, err)
		}
		for _, declaration := range parsed.Decls {
			general, ok := declaration.(*ast.GenDecl)
			if !ok || general.Tok != token.IMPORT {
				continue
			}
			for _, raw := range general.Specs {
				specification, ok := raw.(*ast.ImportSpec)
				if !ok || specification.Path == nil {
					return nil, fmt.Errorf("%s source %s has malformed import specification", label, file.Path)
				}
				importPath, err := strconv.Unquote(specification.Path.Value)
				if err != nil || importPath == "" {
					return nil, fmt.Errorf("%s source %s has invalid import path", label, file.Path)
				}
				alias := ""
				if specification.Name != nil {
					alias = specification.Name.Name
				}
				entry := ImportSpec{Consumer: file.Path, Path: importPath, Alias: alias}
				key := importSpecKey(entry)
				if seen[key] {
					return nil, fmt.Errorf("%s source duplicates import %s", label, printableImportSpec(entry))
				}
				seen[key] = true
				result = append(result, entry)
			}
		}
	}
	return result, nil
}
