package render

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"path/filepath"
	"strings"

	"github.com/wippyai/go-lua/cmd/flashrefactor/internal/cutplan"
	"github.com/wippyai/go-lua/cmd/flashrefactor/internal/gorewrite"
)

type requirementIndex map[string]cutplan.ResolutionRequirement

func indexRequirements(intent cutplan.Intent) (requirementIndex, error) {
	values, err := cutplan.ResolutionRequirements(intent)
	if err != nil {
		return nil, err
	}
	result := make(requirementIndex, len(values))
	for _, value := range values {
		result[value.Object.Object] = value
	}
	return result, nil
}

func (state *renderState) sourceFor(requirements requirementIndex, ref cutplan.SymbolRef, declaredPath string) (types.Object, error) {
	requirement, exists := requirements[ref.Object]
	if !exists || requirement.Role != cutplan.ObjectSource {
		return nil, fmt.Errorf("%s is not a reviewed pre-cut source object", ref.Object)
	}
	if requirement.Path != "" && requirement.Path != declaredPath {
		return nil, fmt.Errorf("source object %s belongs to %s, not declared source %s", ref.Object, requirement.Path, declaredPath)
	}
	object, err := state.sourceObject(ref)
	if err != nil {
		return nil, err
	}
	path, err := sourcePathForObject(state.workspace, object)
	if err != nil {
		return nil, err
	}
	if path != declaredPath {
		return nil, fmt.Errorf("source object %s resolves in %s, not declared source %s", ref.Object, path, declaredPath)
	}
	shape, err := parseSymbol(ref)
	if err != nil {
		return nil, err
	}
	if !sameObjectKind(object, shape) {
		return nil, fmt.Errorf("source object %s has wrong Go object form", ref.Object)
	}
	return object, nil
}

func (state *renderState) relocate(requirements requirementIndex, operation cutplan.Operation, edit cutplan.Relocate) error {
	if err := state.writeAllowed(edit.Source); err != nil {
		return err
	}
	if err := state.writeAllowed(edit.Destination.Path); err != nil {
		return err
	}
	if edit.Containment != nil {
		return state.relocateContainment(requirements, operation, edit)
	}
	return state.relocateDeclarations(requirements, edit)
}

func (state *renderState) relocateDeclarations(requirements requirementIndex, edit cutplan.Relocate) error {
	source, _, _, err := state.existingFile(edit.Source)
	if err != nil {
		return err
	}
	destination, err := state.file(edit.Destination.Path, edit.Destination.Package)
	if err != nil {
		return err
	}
	if err := source.ensureGo(); err != nil {
		return err
	}
	if err := destination.ensureGo(); err != nil {
		return err
	}
	selected := make(map[ast.Decl]types.Object, len(edit.Subjects))
	selector := gorewrite.DeclarationSelector{Names: map[string]struct{}{}, Methods: map[string]struct{}{}, Tests: map[string]struct{}{}}
	for _, subject := range edit.Subjects {
		object, objectErr := state.sourceFor(requirements, subject.From, edit.Source)
		if objectErr != nil {
			return objectErr
		}
		from, parseErr := parseSymbol(subject.From)
		if parseErr != nil {
			return parseErr
		}
		to, parseErr := parseSymbol(subject.To)
		if parseErr != nil {
			return parseErr
		}
		if from.kind == symbolField || to.kind != symbolPackage {
			return fmt.Errorf("relocate %s -> %s is not a supported whole declaration move", subject.From.Object, subject.To.Object)
		}
		decl, kind, declErr := declarationForObject(source, object)
		if declErr != nil {
			return declErr
		}
		if _, duplicate := selected[decl]; duplicate {
			return fmt.Errorf("two relocated objects select one declaration in %s", edit.Source)
		}
		selected[decl] = object
		switch kind {
		case declarationName:
			if _, exists := selector.Names[object.Name()]; exists {
				return fmt.Errorf("two exact declarations share selector name %s", object.Name())
			}
			selector.Names[object.Name()] = struct{}{}
		case declarationTest:
			if _, exists := selector.Tests[object.Name()]; exists {
				return fmt.Errorf("two exact test declarations share selector name %s", object.Name())
			}
			selector.Tests[object.Name()] = struct{}{}
		case declarationMethod:
			// A method declaration's receiver is part of object identity, but
			// gorewrite's narrow extraction primitive is name-only. Refuse it
			// rather than accidentally selecting an equal-named second receiver.
			return fmt.Errorf("method relocation is unsupported; extract its owning type component first: %s", subject.From.Object)
		default:
			return fmt.Errorf("object %s is not a relocatable declaration", subject.From.Object)
		}
	}
	if err := ensureSelectorExact(source, selected, selector); err != nil {
		return err
	}
	if err := extractAcrossPackages(source, destination, selector); err != nil {
		return err
	}
	for _, subject := range edit.Subjects {
		object, err := state.sourceFor(requirements, subject.From, edit.Source)
		if err != nil {
			return err
		}
		name, err := targetName(subject.To)
		if err != nil {
			return err
		}
		if err := renameMovedObject(destination, source.info, object, name); err != nil {
			return err
		}
	}
	return nil
}

func (state *renderState) preflightDeclarations(requirements requirementIndex, edit cutplan.Relocate) error {
	if err := state.writeAllowed(edit.Source); err != nil {
		return err
	}
	if err := state.writeAllowed(edit.Destination.Path); err != nil {
		return err
	}
	source, _, _, err := state.existingFile(edit.Source)
	if err != nil {
		return err
	}
	if _, err := state.file(edit.Destination.Path, edit.Destination.Package); err != nil {
		return err
	}
	selected := make(map[ast.Decl]types.Object, len(edit.Subjects))
	selector := gorewrite.DeclarationSelector{Names: map[string]struct{}{}, Methods: map[string]struct{}{}, Tests: map[string]struct{}{}}
	for _, subject := range edit.Subjects {
		object, err := state.sourceFor(requirements, subject.From, edit.Source)
		if err != nil {
			return err
		}
		from, err := parseSymbol(subject.From)
		if err != nil {
			return err
		}
		to, err := parseSymbol(subject.To)
		if err != nil {
			return err
		}
		if from.kind == symbolField || to.kind != symbolPackage {
			return fmt.Errorf("relocate %s -> %s is not a supported whole declaration move", subject.From.Object, subject.To.Object)
		}
		decl, kind, err := declarationForObject(source, object)
		if err != nil {
			return err
		}
		if _, duplicate := selected[decl]; duplicate {
			return fmt.Errorf("two relocated objects select one declaration in %s", edit.Source)
		}
		selected[decl] = object
		switch kind {
		case declarationName:
			selector.Names[object.Name()] = struct{}{}
		case declarationTest:
			selector.Tests[object.Name()] = struct{}{}
		default:
			return fmt.Errorf("object %s is not a supported top-level relocation form", subject.From.Object)
		}
	}
	return ensureSelectorExact(source, selected, selector)
}

// relocateContainment converts a set of owned fields into one new child
// struct. gorewrite owns the delicate literal rewrite; this wrapper supplies
// exact object selections and, for a package cut, changes only the inserted
// field's type to the explicit import qualifier declared by the cut.
func (state *renderState) relocateContainment(requirements requirementIndex, operation cutplan.Operation, edit cutplan.Relocate) error {
	source, _, _, err := state.existingFile(edit.Source)
	if err != nil {
		return err
	}
	destination, err := state.file(edit.Destination.Path, edit.Destination.Package)
	if err != nil {
		return err
	}
	parent, err := state.sourceFor(requirements, edit.Containment.Parent, edit.Source)
	if err != nil {
		return err
	}
	parentShape, err := parseSymbol(edit.Containment.Parent)
	if err != nil || parentShape.kind != symbolPackage {
		return fmt.Errorf("containment parent must be an exact source type: %w", err)
	}
	if _, ok := parent.(*types.TypeName); !ok {
		return fmt.Errorf("containment parent %s is not a named type", edit.Containment.Parent.Object)
	}
	childShape, err := parseSymbol(edit.Containment.Child)
	if err != nil || childShape.kind != symbolPackage {
		return fmt.Errorf("containment child must be an exact target type: %w", err)
	}
	throughShape, err := parseSymbol(edit.Containment.Through)
	if err != nil || throughShape.kind != symbolField || throughShape.owner != parentShape.member {
		return fmt.Errorf("containment through must be the new field on %s", parentShape.member)
	}
	if childShape.member == parentShape.member {
		return fmt.Errorf("containment child must be distinct from parent")
	}
	if existing, findErr := state.workspace.Object(edit.Containment.Child); findErr == nil && existing != nil {
		return fmt.Errorf("containment child target already exists pre-cut: %s", edit.Containment.Child.Object)
	}
	fields := make(map[string]struct{}, len(edit.Subjects))
	for _, subject := range edit.Subjects {
		object, objectErr := state.sourceFor(requirements, subject.From, edit.Source)
		if objectErr != nil {
			return objectErr
		}
		from, parseErr := parseSymbol(subject.From)
		if parseErr != nil {
			return parseErr
		}
		to, parseErr := parseSymbol(subject.To)
		if parseErr != nil {
			return parseErr
		}
		field, ok := object.(*types.Var)
		if !ok || !field.IsField() || from.kind != symbolField || from.owner != parentShape.member || to.kind != symbolField || to.owner != childShape.member || to.member != from.member {
			return fmt.Errorf("containment relocation must map each exact %s field to the same-named %s field", parentShape.member, childShape.member)
		}
		if _, duplicate := fields[from.member]; duplicate {
			return fmt.Errorf("duplicate containment field %s", from.member)
		}
		fields[from.member] = struct{}{}
	}
	if err := appendEmptyChild(source, childShape.member); err != nil {
		return err
	}
	if err := gorewrite.RelocateNamedFields(source.file, source.fset, gorewrite.FieldRelocation{
		Owner: parentShape.member, Child: childShape.member, ChildField: throughShape.member, Fields: fields,
	}); err != nil {
		return err
	}
	selector := gorewrite.DeclarationSelector{Names: map[string]struct{}{childShape.member: {}}, Methods: map[string]struct{}{}, Tests: map[string]struct{}{}}
	if err := extractAcrossPackages(source, destination, selector); err != nil {
		return err
	}
	if source.packageName != destination.packageName {
		qualifier, qualifierErr := containmentQualifier(operation, edit.Containment.Child, edit.Source)
		if qualifierErr != nil {
			return qualifierErr
		}
		if err := qualifyInsertedField(source.file, parentShape.member, throughShape.member, qualifier, childShape.member); err != nil {
			return err
		}
	}
	return nil
}

func (state *renderState) preflightContainment(requirements requirementIndex, operation cutplan.Operation, edit cutplan.Relocate) error {
	if err := state.writeAllowed(edit.Source); err != nil {
		return err
	}
	if err := state.writeAllowed(edit.Destination.Path); err != nil {
		return err
	}
	source, _, _, err := state.existingFile(edit.Source)
	if err != nil {
		return err
	}
	if _, err := state.file(edit.Destination.Path, edit.Destination.Package); err != nil {
		return err
	}
	parent, err := state.sourceFor(requirements, edit.Containment.Parent, edit.Source)
	if err != nil {
		return err
	}
	parentShape, err := parseSymbol(edit.Containment.Parent)
	if err != nil || parentShape.kind != symbolPackage {
		return fmt.Errorf("containment parent must be an exact source type: %w", err)
	}
	if _, ok := parent.(*types.TypeName); !ok {
		return fmt.Errorf("containment parent %s is not a named type", edit.Containment.Parent.Object)
	}
	childShape, err := parseSymbol(edit.Containment.Child)
	if err != nil || childShape.kind != symbolPackage {
		return fmt.Errorf("containment child must be an exact target type: %w", err)
	}
	throughShape, err := parseSymbol(edit.Containment.Through)
	if err != nil || throughShape.kind != symbolField || throughShape.owner != parentShape.member {
		return fmt.Errorf("containment through must be the new field on %s", parentShape.member)
	}
	if existing, findErr := state.workspace.Object(edit.Containment.Child); findErr == nil && existing != nil {
		return fmt.Errorf("containment child target already exists pre-cut: %s", edit.Containment.Child.Object)
	}
	for _, subject := range edit.Subjects {
		object, err := state.sourceFor(requirements, subject.From, edit.Source)
		if err != nil {
			return err
		}
		from, err := parseSymbol(subject.From)
		if err != nil {
			return err
		}
		to, err := parseSymbol(subject.To)
		if err != nil {
			return err
		}
		field, ok := object.(*types.Var)
		if !ok || !field.IsField() || from.kind != symbolField || from.owner != parentShape.member || to.kind != symbolField || to.owner != childShape.member || to.member != from.member {
			return fmt.Errorf("containment relocation must map each exact %s field to the same-named %s field", parentShape.member, childShape.member)
		}
	}
	if source.file == nil {
		return fmt.Errorf("containment source %s has no syntax", edit.Source)
	}
	if source.packageName != edit.Destination.Package {
		if _, err := containmentQualifier(operation, edit.Containment.Child, edit.Source); err != nil {
			return err
		}
	}
	return nil
}

func appendEmptyChild(source *fileState, name string) error {
	if source == nil || source.file == nil {
		return fmt.Errorf("missing containment source")
	}
	for _, decl := range source.file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.TYPE {
			continue
		}
		for _, spec := range gen.Specs {
			if typeSpec, ok := spec.(*ast.TypeSpec); ok && typeSpec.Name.Name == name {
				return fmt.Errorf("generated containment child %s already exists", name)
			}
		}
	}
	source.file.Decls = append(source.file.Decls, &ast.GenDecl{Tok: token.TYPE, Specs: []ast.Spec{
		&ast.TypeSpec{Name: ast.NewIdent(name), Type: &ast.StructType{Fields: &ast.FieldList{}}},
	}})
	return nil
}

func containmentQualifier(operation cutplan.Operation, child cutplan.SymbolRef, consumer string) (string, error) {
	childShape, err := parseSymbol(child)
	if err != nil {
		return "", err
	}
	for _, route := range operation.Imports {
		if route.Consumer != consumer || route.To == nil || route.To.Path != childShape.packagePath {
			continue
		}
		if route.To.Alias != "" {
			return route.To.Alias, nil
		}
		return route.To.Name, nil
	}
	return "", fmt.Errorf("cross-package containment needs an exact destination import for %s in %s", childShape.packagePath, consumer)
}

func qualifyInsertedField(file *ast.File, parent, field, qualifier, child string) error {
	found := false
	for _, declaration := range file.Decls {
		gen, ok := declaration.(*ast.GenDecl)
		if !ok || gen.Tok != token.TYPE {
			continue
		}
		for _, raw := range gen.Specs {
			spec, ok := raw.(*ast.TypeSpec)
			if !ok || spec.Name.Name != parent {
				continue
			}
			structure, ok := spec.Type.(*ast.StructType)
			if !ok {
				return fmt.Errorf("containment parent %s is not a struct", parent)
			}
			for _, value := range structure.Fields.List {
				if len(value.Names) == 1 && value.Names[0].Name == field {
					value.Type = &ast.SelectorExpr{X: ast.NewIdent(qualifier), Sel: ast.NewIdent(child)}
					found = true
					break
				}
			}
		}
	}
	if !found {
		return fmt.Errorf("inserted containment field %s.%s was not found", parent, field)
	}
	// Flow did not exist in the source package before this cut (the preflight
	// proves that), so every remaining unqualified Flow composite literal here
	// was introduced by gorewrite's keyed-literal preservation rewrite.
	ast.Inspect(file, func(node ast.Node) bool {
		literal, ok := node.(*ast.CompositeLit)
		if !ok {
			return true
		}
		name, ok := literal.Type.(*ast.Ident)
		if !ok || name.Name != child {
			return true
		}
		literal.Type = &ast.SelectorExpr{X: ast.NewIdent(qualifier), Sel: ast.NewIdent(child)}
		return true
	})
	return nil
}

type declarationKind uint8

const (
	declarationUnknown declarationKind = iota
	declarationName
	declarationTest
	declarationMethod
)

func declarationForObject(state *fileState, object types.Object) (ast.Decl, declarationKind, error) {
	if err := state.ensureGo(); err != nil {
		return nil, declarationUnknown, err
	}
	var found ast.Decl
	var kind declarationKind
	for _, declaration := range state.file.Decls {
		candidate, candidateKind := objectDeclaration(state.info, declaration, object)
		if !candidate {
			continue
		}
		if found != nil {
			return nil, declarationUnknown, fmt.Errorf("object %s has multiple declaration sites in %s", object.Name(), state.path)
		}
		found, kind = declaration, candidateKind
	}
	if found == nil {
		mapped := 0
		for _, candidate := range state.info.Defs {
			if candidate == object {
				mapped++
			}
		}
		return nil, declarationUnknown, fmt.Errorf("object %s has no direct declaration in %s (detached definitions=%d)", object.Name(), state.path, mapped)
	}
	return found, kind, nil
}

func objectDeclaration(info *types.Info, declaration ast.Decl, object types.Object) (bool, declarationKind) {
	if info == nil {
		return false, declarationUnknown
	}
	switch value := declaration.(type) {
	case *ast.FuncDecl:
		if info.Defs[value.Name] != object {
			return false, declarationUnknown
		}
		if value.Recv != nil {
			return true, declarationMethod
		}
		if isTestDeclaration(value) {
			return true, declarationTest
		}
		return true, declarationName
	case *ast.GenDecl:
		for _, raw := range value.Specs {
			switch spec := raw.(type) {
			case *ast.TypeSpec:
				if info.Defs[spec.Name] == object {
					return true, declarationName
				}
			case *ast.ValueSpec:
				for _, name := range spec.Names {
					if info.Defs[name] == object {
						if len(value.Specs) != 1 || len(spec.Names) != 1 {
							return false, declarationUnknown
						}
						return true, declarationName
					}
				}
			}
		}
	}
	return false, declarationUnknown
}

func isTestDeclaration(function *ast.FuncDecl) bool {
	name := function.Name.Name
	return strings.HasPrefix(name, "Test") || strings.HasPrefix(name, "Benchmark") || strings.HasPrefix(name, "Example")
}

func ensureSelectorExact(state *fileState, selected map[ast.Decl]types.Object, selector gorewrite.DeclarationSelector) error {
	seen := map[string]int{}
	for _, declaration := range state.file.Decls {
		match, kind := objectDeclaration(state.info, declaration, nil)
		_ = match
		_ = kind
		// gorewrite's selector is deliberately string-shaped. Check every
		// potential collision before handing it those strings.
		for _, name := range declarationNames(declaration) {
			if _, wanted := selector.Names[name]; wanted {
				seen["name:"+name]++
			}
			if _, wanted := selector.Tests[name]; wanted {
				seen["test:"+name]++
			}
		}
	}
	for name := range selector.Names {
		if seen["name:"+name] != 1 {
			return fmt.Errorf("exact declaration selector %s is ambiguous", name)
		}
	}
	for name := range selector.Tests {
		if seen["test:"+name] != 1 {
			return fmt.Errorf("exact test selector %s is ambiguous", name)
		}
	}
	if len(selected) != len(selector.Names)+len(selector.Tests) {
		return fmt.Errorf("declaration selector does not equal exact selected objects")
	}
	return nil
}

func declarationNames(declaration ast.Decl) []string {
	switch value := declaration.(type) {
	case *ast.FuncDecl:
		if value.Recv == nil {
			return []string{value.Name.Name}
		}
	case *ast.GenDecl:
		var names []string
		for _, raw := range value.Specs {
			switch spec := raw.(type) {
			case *ast.TypeSpec:
				names = append(names, spec.Name.Name)
			case *ast.ValueSpec:
				for _, name := range spec.Names {
					names = append(names, name.Name)
				}
			}
		}
		return names
	}
	return nil
}

func extractAcrossPackages(source, destination *fileState, selector gorewrite.DeclarationSelector) error {
	if err := source.ensureGo(); err != nil {
		return err
	}
	if err := destination.ensureGo(); err != nil {
		return err
	}
	// gorewrite has the comment/build-constraint preservation law. Its narrow
	// cross-package guard exists to protect callers that have not supplied an
	// explicit package cut. Here Destination.Package is reviewed, so temporarily
	// equalize the clause solely while invoking that structural primitive.
	actual := destination.file.Name
	if source.file.Name.Name != destination.file.Name.Name {
		destination.file.Name = ast.NewIdent(source.file.Name.Name)
	}
	_, err := gorewrite.ExtractDeclarations(source.file, destination.file, selector)
	if err == nil && source.packageName != destination.packageName {
		destination.file.Name = ast.NewIdent(destination.packageName)
	} else {
		destination.file.Name = actual
	}
	return err
}

func renameMovedObject(destination *fileState, sourceInfo *types.Info, object types.Object, name string) error {
	if object.Name() == name {
		return nil
	}
	found := false
	ast.Inspect(destination.file, func(node ast.Node) bool {
		identifier, ok := node.(*ast.Ident)
		if !ok {
			return true
		}
		if sourceInfo.Defs[identifier] == object || sourceInfo.Uses[identifier] == object {
			identifier.Name = name
			found = true
		}
		return true
	})
	if !found {
		return fmt.Errorf("moved object %s was not present in destination %s", object.Name(), destination.path)
	}
	return nil
}

func (state *renderState) retire(requirements requirementIndex, edit cutplan.Retire) error {
	if err := state.writeAllowed(edit.Source); err != nil {
		return err
	}
	source, _, _, err := state.existingFile(edit.Source)
	if err != nil {
		return err
	}
	selected := make(map[ast.Decl]types.Object, len(edit.Symbols))
	selector := gorewrite.DeclarationSelector{Names: map[string]struct{}{}, Methods: map[string]struct{}{}, Tests: map[string]struct{}{}}
	for _, ref := range edit.Symbols {
		object, objectErr := state.sourceFor(requirements, ref, edit.Source)
		if objectErr != nil {
			return objectErr
		}
		decl, kind, declErr := declarationForObject(source, object)
		if declErr != nil {
			return fmt.Errorf("retire %s: %w", ref.Object, declErr)
		}
		if _, exists := selected[decl]; exists {
			return fmt.Errorf("retire selects a declaration twice: %s", ref.Object)
		}
		selected[decl] = object
		switch kind {
		case declarationName:
			selector.Names[object.Name()] = struct{}{}
		case declarationTest:
			selector.Tests[object.Name()] = struct{}{}
		default:
			return fmt.Errorf("retire of %s is unsupported object form", ref.Object)
		}
	}
	if err := ensureSelectorExact(source, selected, selector); err != nil {
		return err
	}
	temporary, err := emptyPeerFile(source)
	if err != nil {
		return err
	}
	if _, err := gorewrite.ExtractDeclarations(source.file, temporary.file, selector); err != nil {
		return err
	}
	if len(nonImportDeclarations(source.file)) == 0 && len(source.file.Imports) == 0 {
		source.deleted = true
	}
	return nil
}

func (state *renderState) preflightRetire(requirements requirementIndex, edit cutplan.Retire) error {
	if err := state.writeAllowed(edit.Source); err != nil {
		return err
	}
	source, _, _, err := state.existingFile(edit.Source)
	if err != nil {
		return err
	}
	selected := make(map[ast.Decl]types.Object, len(edit.Symbols))
	selector := gorewrite.DeclarationSelector{Names: map[string]struct{}{}, Methods: map[string]struct{}{}, Tests: map[string]struct{}{}}
	for _, ref := range edit.Symbols {
		object, err := state.sourceFor(requirements, ref, edit.Source)
		if err != nil {
			return err
		}
		decl, kind, err := declarationForObject(source, object)
		if err != nil {
			return err
		}
		if _, duplicate := selected[decl]; duplicate {
			return fmt.Errorf("retire selects a declaration twice: %s", ref.Object)
		}
		selected[decl] = object
		switch kind {
		case declarationName:
			selector.Names[object.Name()] = struct{}{}
		case declarationTest:
			selector.Tests[object.Name()] = struct{}{}
		default:
			return fmt.Errorf("retire of %s is unsupported object form", ref.Object)
		}
	}
	return ensureSelectorExact(source, selected, selector)
}

func emptyPeerFile(source *fileState) (*fileState, error) {
	var header strings.Builder
	for _, group := range source.file.Comments {
		if group.End() >= source.file.Name.Pos() {
			continue
		}
		for _, comment := range group.List {
			text := strings.TrimSpace(comment.Text)
			if strings.HasPrefix(text, "//go:build") || strings.HasPrefix(text, "// +build") {
				header.WriteString(comment.Text)
				header.WriteByte('\n')
			}
		}
	}
	header.WriteString("package ")
	header.WriteString(source.packageName)
	header.WriteByte('\n')
	set := token.NewFileSet()
	file, err := parser.ParseFile(set, filepath.Base(source.path)+".retire", header.String(), parser.ParseComments)
	if err != nil {
		return nil, err
	}
	return &fileState{path: source.path + ".retire", packageName: source.packageName, fset: set, file: file, info: &types.Info{}}, nil
}

func nonImportDeclarations(file *ast.File) []ast.Decl {
	var result []ast.Decl
	for _, declaration := range file.Decls {
		if general, ok := declaration.(*ast.GenDecl); ok && general.Tok == token.IMPORT {
			continue
		}
		result = append(result, declaration)
	}
	return result
}
