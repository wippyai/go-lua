package gorewrite

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"sort"
	"strings"
)

// ExtractDeclarations moves exact complete declarations from source to
// destination. Both files must already declare a package; cross-package
// extraction is allowed structurally, but callers must provide explicit import
// routes and type-check the result before committing it.
func ExtractDeclarations(source, destination *ast.File, selector DeclarationSelector) (ExtractionResult, error) {
	if source == nil || destination == nil || source.Name == nil || destination.Name == nil {
		return ExtractionResult{}, fmt.Errorf("source and destination must declare packages")
	}
	if len(selector.Names)+len(selector.Methods)+len(selector.Tests) == 0 {
		return ExtractionResult{}, fmt.Errorf("empty declaration selector")
	}
	if err := rejectAuthorityHazards(source, nil, nil); err != nil {
		return ExtractionResult{}, err
	}
	if err := rejectAuthorityHazards(destination, nil, nil); err != nil {
		return ExtractionResult{}, err
	}
	if err := compatibleFileConstraints(source, destination); err != nil {
		return ExtractionResult{}, err
	}
	if source.Name.Name != destination.Name.Name && !testOnlyRehome(source.Name.Name, destination.Name.Name, selector) {
		return ExtractionResult{}, fmt.Errorf("cross-package extraction may move only tests between peer internal/external test packages")
	}
	selected := make(map[ast.Decl]bool)
	var moved []ast.Decl
	seen := make(map[string]bool)
	for _, decl := range source.Decls {
		match, label, err := declarationMatch(decl, selector)
		if err != nil {
			return ExtractionResult{}, err
		}
		if !match {
			continue
		}
		if seen[label] {
			return ExtractionResult{}, fmt.Errorf("ambiguous declaration %s", label)
		}
		seen[label] = true
		selected[decl] = true
		moved = append(moved, decl)
	}
	for name := range selector.Names {
		if !seen["name:"+name] {
			return ExtractionResult{}, fmt.Errorf("missing declaration %s", name)
		}
	}
	for name := range selector.Methods {
		if !seen["method:"+name] {
			return ExtractionResult{}, fmt.Errorf("missing method %s", name)
		}
	}
	for name := range selector.Tests {
		if !seen["test:"+name] {
			return ExtractionResult{}, fmt.Errorf("missing test %s", name)
		}
	}
	commentMove, err := preflightCommentMove(source, selected)
	if err != nil {
		return ExtractionResult{}, err
	}
	remaining := make([]ast.Decl, 0, len(source.Decls)-len(moved))
	for _, decl := range source.Decls {
		if !selected[decl] {
			remaining = append(remaining, decl)
		}
	}
	source.Decls = remaining
	destination.Decls = append(destination.Decls, moved...)
	moveComments(source, destination, commentMove)
	return ExtractionResult{Moved: moved}, nil
}

func testOnlyRehome(source, destination string, selector DeclarationSelector) bool {
	if len(selector.Names) != 0 || len(selector.Methods) != 0 || len(selector.Tests) == 0 {
		return false
	}
	return strings.TrimSuffix(source, "_test") == strings.TrimSuffix(destination, "_test") &&
		(strings.HasSuffix(source, "_test") || strings.HasSuffix(destination, "_test"))
}

func compatibleFileConstraints(source, destination *ast.File) error {
	left := fileConstraints(source)
	right := fileConstraints(destination)
	if len(left) != len(right) {
		return fmt.Errorf("source and destination build constraints differ")
	}
	for index := range left {
		if left[index] != right[index] {
			return fmt.Errorf("source and destination build constraints differ")
		}
	}
	return nil
}

func fileConstraints(file *ast.File) []string {
	var constraints []string
	for _, group := range file.Comments {
		if group.Pos() >= file.Name.Pos() {
			continue
		}
		for _, comment := range group.List {
			text := strings.TrimSpace(comment.Text)
			if strings.HasPrefix(text, "//go:build") || strings.HasPrefix(text, "// +build") {
				constraints = append(constraints, text)
			}
		}
	}
	sort.Strings(constraints)
	return constraints
}

func preflightCommentMove(file *ast.File, selected map[ast.Decl]bool) (map[*ast.CommentGroup]bool, error) {
	move := make(map[*ast.CommentGroup]bool)
	for index, decl := range file.Decls {
		if !selected[decl] {
			continue
		}
		lower := file.Name.End()
		if index > 0 {
			lower = file.Decls[index-1].End()
		}
		upper := token.Pos(int(^uint(0) >> 1))
		if index+1 < len(file.Decls) {
			upper = file.Decls[index+1].Pos()
		}
		for _, group := range file.Comments {
			if group.Pos() >= decl.Pos() && group.End() <= decl.End() {
				move[group] = true
				continue
			}
			if doc := declDoc(decl); doc == group {
				move[group] = true
				continue
			}
			if group.Pos() > lower && group.End() < upper {
				return nil, fmt.Errorf("detached comment near selected declaration at %d is ambiguous; attach or move it explicitly", group.Pos())
			}
		}
	}
	return move, nil
}

func moveComments(source, destination *ast.File, moved map[*ast.CommentGroup]bool) {
	if len(moved) == 0 {
		return
	}
	remaining := make([]*ast.CommentGroup, 0, len(source.Comments)-len(moved))
	for _, group := range source.Comments {
		if moved[group] {
			destination.Comments = append(destination.Comments, group)
			continue
		}
		remaining = append(remaining, group)
	}
	source.Comments = remaining
	sort.SliceStable(destination.Comments, func(i, j int) bool {
		return destination.Comments[i].Pos() < destination.Comments[j].Pos()
	})
}

func declarationMatch(decl ast.Decl, selector DeclarationSelector) (bool, string, error) {
	function, ok := decl.(*ast.FuncDecl)
	if ok {
		if function.Recv != nil {
			if _, wanted := selector.Methods[function.Name.Name]; wanted {
				return true, "method:" + function.Name.Name, nil
			}
			return false, "", nil
		}
		if _, wanted := selector.Tests[function.Name.Name]; wanted {
			if !isTestFunction(function) {
				return false, "", fmt.Errorf("%s selected as test but has no test-like name", function.Name.Name)
			}
			return true, "test:" + function.Name.Name, nil
		}
		if _, wanted := selector.Names[function.Name.Name]; wanted {
			return true, "name:" + function.Name.Name, nil
		}
		return false, "", nil
	}
	general, ok := decl.(*ast.GenDecl)
	if !ok {
		return false, "", nil
	}
	var names []string
	for _, spec := range general.Specs {
		switch concrete := spec.(type) {
		case *ast.TypeSpec:
			names = append(names, concrete.Name.Name)
		case *ast.ValueSpec:
			for _, name := range concrete.Names {
				names = append(names, name.Name)
			}
		}
	}
	if len(names) != 1 {
		for _, name := range names {
			if _, wanted := selector.Names[name]; wanted {
				return false, "", fmt.Errorf("%s shares a declaration with other names; split it explicitly before extraction", name)
			}
		}
		return false, "", nil
	}
	if _, wanted := selector.Names[names[0]]; wanted {
		return true, "name:" + names[0], nil
	}
	return false, "", nil
}

func isTestFunction(function *ast.FuncDecl) bool {
	name := function.Name.Name
	return len(name) >= 4 && (name[:4] == "Test" || (len(name) >= 9 && name[:9] == "Benchmark") || (len(name) >= 7 && name[:7] == "Example"))
}

// RenameObjects applies names only to identifiers bound to the supplied type
// objects. It refuses duplicate targets in one lexical declaration group only
// when the type checker has already identified the duplicate object.
func RenameObjects(file *ast.File, info *types.Info, renames []IdentifierRename) error {
	if info == nil {
		return fmt.Errorf("object rename requires type information")
	}
	if err := rejectAuthorityHazards(file, nil, info); err != nil {
		return err
	}
	byObject := make(map[types.Object]string, len(renames))
	for _, rename := range renames {
		if rename.Object == nil || rename.To == "" || !token.IsIdentifier(rename.To) {
			return fmt.Errorf("rename requires object and valid identifier")
		}
		if prior, exists := byObject[rename.Object]; exists && prior != rename.To {
			return fmt.Errorf("conflicting rename for %s", rename.Object.Name())
		}
		byObject[rename.Object] = rename.To
	}
	for ident, object := range info.Defs {
		if replacement, exists := byObject[object]; exists {
			ident.Name = replacement
		}
	}
	for ident, object := range info.Uses {
		if replacement, exists := byObject[object]; exists {
			ident.Name = replacement
		}
	}
	return nil
}
