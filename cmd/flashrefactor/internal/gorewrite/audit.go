package gorewrite

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"strconv"
	"strings"
)

// FindResidue reports remaining identifiers, selectors, keyed literal fields,
// and reflection authority. Plain diagnostic strings are classified separately
// and do not make a mechanical cut unsafe by themselves.
func FindResidue(file *ast.File, fset *token.FileSet, names map[string]struct{}) []Residue {
	var residue []Residue
	ast.Inspect(file, func(node ast.Node) bool {
		switch current := node.(type) {
		case *ast.Ident:
			if _, wanted := names[current.Name]; wanted {
				residue = append(residue, Residue{Pos: fset.Position(current.Pos()), Kind: "identifier", Text: current.Name})
			}
		case *ast.BasicLit:
			if current.Kind != token.STRING {
				return true
			}
			value, err := strconv.Unquote(current.Value)
			if err != nil {
				return true
			}
			if _, wanted := names[value]; wanted {
				residue = append(residue, Residue{Pos: fset.Position(current.Pos()), Kind: "string", Text: value})
			}
		}
		return true
	})
	return residue
}

// FindObjectResidue is the type-aware variant for an exact preflight object
// inventory. Unlike spelling scans, it does not confuse a shadowed local with
// the selected declaration. Call it after re-type-checking the proposed cut.
func FindObjectResidue(file *ast.File, fset *token.FileSet, info *types.Info, objects map[types.Object]struct{}) []Residue {
	if info == nil {
		return []Residue{{Kind: "missing-type-info", Text: "object residue requires type information"}}
	}
	var residue []Residue
	for ident, object := range info.Defs {
		if _, wanted := objects[object]; wanted {
			residue = append(residue, Residue{Pos: fset.Position(ident.Pos()), Kind: "definition", Text: ident.Name})
		}
	}
	for ident, object := range info.Uses {
		if _, wanted := objects[object]; wanted {
			residue = append(residue, Residue{Pos: fset.Position(ident.Pos()), Kind: "use", Text: ident.Name})
		}
	}
	return residue
}

// FindHazards identifies authority that a structural rewrite cannot safely
// account for. A string merely used for an error/log/panic message is
// explicitly non-authoritative.
//
// Source-only callers retain the hazards that source can establish exactly:
// imports, cgo, generated source, and linkname. They never infer reflection
// from a selector spelling. When type information is available, package and
// receiver identity classify reflect and unsafe calls exactly.
func FindHazards(file *ast.File, fset *token.FileSet, info *types.Info) []Hazard {
	reflectAliases, unsafeAliases := importedAliases(file)
	var hazards []Hazard
	for alias := range reflectAliases {
		hazards = append(hazards, Hazard{Pos: positionOf(fset, file.Pos()), Kind: "reflect-import", Detail: alias, Authority: true})
	}
	for alias := range unsafeAliases {
		hazards = append(hazards, Hazard{Pos: positionOf(fset, file.Pos()), Kind: "unsafe-import", Detail: alias, Authority: true})
	}
	for _, spec := range file.Imports {
		path, err := strconv.Unquote(spec.Path.Value)
		if err == nil && path == "C" {
			hazards = append(hazards, Hazard{Pos: positionOf(fset, spec.Pos()), Kind: "cgo-import", Detail: "import C", Authority: true})
		}
	}
	if generatedSource(file) {
		hazards = append(hazards, Hazard{Pos: positionOf(fset, file.Pos()), Kind: "generated-source", Detail: "generated files are not mechanically rewritten", Authority: true})
	}
	for _, group := range allCommentGroups(file) {
		for _, comment := range group.List {
			if strings.HasPrefix(strings.TrimSpace(comment.Text), "//go:linkname") {
				hazards = append(hazards, Hazard{Pos: positionOf(fset, comment.Pos()), Kind: "go-linkname", Detail: strings.TrimSpace(comment.Text), Authority: true})
			}
		}
	}
	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		reflectName := typedCallHazards(call, fset, info, &hazards)
		for _, argument := range call.Args {
			literal, ok := argument.(*ast.BasicLit)
			if !ok || literal.Kind != token.STRING {
				continue
			}
			value, err := strconv.Unquote(literal.Value)
			if err != nil {
				continue
			}
			if isDiagnosticCall(call) {
				hazards = append(hazards, Hazard{Pos: positionOf(fset, literal.Pos()), Kind: "diagnostic-string", Detail: value, Authority: false})
				continue
			}
			if reflectName {
				hazards = append(hazards, Hazard{Pos: positionOf(fset, literal.Pos()), Kind: "reflection-string", Detail: value, Authority: true})
			}
		}
		return true
	})
	return hazards
}

func rejectAuthorityHazards(file *ast.File, fset *token.FileSet, info *types.Info) error {
	for _, hazard := range FindHazards(file, fset, info) {
		if hazard.Authority {
			return fmt.Errorf("%s: mechanical rewrite rejects %s (%s)", hazard.Pos, hazard.Kind, hazard.Detail)
		}
	}
	return nil
}

func positionOf(fset *token.FileSet, pos token.Pos) token.Position {
	if fset == nil {
		return token.Position{}
	}
	return fset.Position(pos)
}

func allCommentGroups(file *ast.File) []*ast.CommentGroup {
	groups := append([]*ast.CommentGroup(nil), file.Comments...)
	for _, decl := range file.Decls {
		if doc := declDoc(decl); doc != nil {
			groups = append(groups, doc)
		}
	}
	return groups
}

func declDoc(decl ast.Decl) *ast.CommentGroup {
	switch current := decl.(type) {
	case *ast.FuncDecl:
		return current.Doc
	case *ast.GenDecl:
		return current.Doc
	default:
		return nil
	}
}

func generatedSource(file *ast.File) bool {
	for _, group := range allCommentGroups(file) {
		for _, comment := range group.List {
			text := strings.ToLower(comment.Text)
			if strings.Contains(text, "code generated") && strings.Contains(text, "do not edit") {
				return true
			}
		}
	}
	return false
}

func importedAliases(file *ast.File) (map[string]bool, map[string]bool) {
	reflectAliases := make(map[string]bool)
	unsafeAliases := make(map[string]bool)
	for _, spec := range file.Imports {
		path, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			continue
		}
		alias := ""
		if spec.Name != nil {
			alias = spec.Name.Name
		} else if path == "reflect" {
			alias = "reflect"
		} else if path == "unsafe" {
			alias = "unsafe"
		}
		switch path {
		case "reflect":
			if alias != "" {
				reflectAliases[alias] = true
			}
		case "unsafe":
			if alias != "" {
				unsafeAliases[alias] = true
			}
		}
	}
	return reflectAliases, unsafeAliases
}

func typedCallHazards(call *ast.CallExpr, fset *token.FileSet, info *types.Info, hazards *[]Hazard) bool {
	if info == nil {
		return false
	}
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	object := selectorObject(info, selector)
	if object == nil || object.Pkg() == nil {
		return false
	}
	switch object.Pkg().Path() {
	case "reflect":
		*hazards = append(*hazards, Hazard{Pos: positionOf(fset, selector.Pos()), Kind: "reflect-call", Detail: selector.Sel.Name, Authority: true})
		if selection := info.Selections[selector]; selection != nil && nameBasedReflectMethod(selection.Obj().Name()) {
			*hazards = append(*hazards, Hazard{Pos: positionOf(fset, selector.Pos()), Kind: "reflect-name-call", Detail: selector.Sel.Name, Authority: true})
			return true
		}
	case "unsafe":
		*hazards = append(*hazards, Hazard{Pos: positionOf(fset, selector.Pos()), Kind: "unsafe-call", Detail: selector.Sel.Name, Authority: true})
	}
	return false
}

func nameBasedReflectMethod(name string) bool {
	switch name {
	case "FieldByName", "MethodByName":
		return true
	default:
		return false
	}
}

func isDiagnosticCall(call *ast.CallExpr) bool {
	switch fun := call.Fun.(type) {
	case *ast.Ident:
		return fun.Name == "panic"
	case *ast.SelectorExpr:
		name := fun.Sel.Name
		if strings.HasPrefix(name, "Error") || strings.HasPrefix(name, "Fatal") || strings.HasPrefix(name, "Log") {
			return true
		}
		if root, ok := fun.X.(*ast.Ident); ok && root.Name == "fmt" && (name == "Errorf" || name == "Sprintf") {
			return true
		}
		if root, ok := fun.X.(*ast.Ident); ok && root.Name == "errors" && name == "New" {
			return true
		}
	}
	return false
}
