package gorewrite

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"strconv"
)

// ClassifySelector returns the type-checker classification for sel. parent is
// required to distinguish a direct method call from a method value. A package
// selector has no Selection entry in types.Info.
func ClassifySelector(info *types.Info, sel *ast.SelectorExpr, parent ast.Node) SelectorClass {
	if info == nil || sel == nil {
		return SelectorUnknown
	}
	selection := info.Selections[sel]
	if selection == nil {
		if _, ok := info.Uses[sel.Sel]; ok {
			return PackageSelection
		}
		return SelectorUnknown
	}
	switch selection.Kind() {
	case types.FieldVal:
		return FieldSelection
	case types.MethodExpr:
		return MethodExpression
	case types.MethodVal:
		if isInterface(selection.Recv()) {
			return InterfaceMethod
		}
		if call, ok := parent.(*ast.CallExpr); ok && call.Fun == sel {
			return MethodInvocation
		}
		return MethodValue
	default:
		return SelectorUnknown
	}
}

func isInterface(t types.Type) bool {
	if t == nil {
		return false
	}
	if named, ok := t.(*types.Named); ok {
		t = named.Underlying()
	}
	_, ok := t.Underlying().(*types.Interface)
	return ok
}

// ApplyRoutePlan performs one exact, type-object keyed rewrite. It validates
// all imports, aliases, selector forms, authority hazards, and receiver paths
// before changing the AST. A later typecheck remains a transaction gate, but
// this function never makes a spelling-based or partial-route guess itself.
func ApplyRoutePlan(file *ast.File, fset *token.FileSet, info *types.Info, plan RoutePlan) error {
	if file == nil || fset == nil {
		return fmt.Errorf("route plan requires file and file set")
	}
	if info == nil && planRequiresTypeInfo(plan) {
		return fmt.Errorf("route plan with source bindings requires type information")
	}
	if plan.Consumer != file {
		return fmt.Errorf("route plan consumer does not match target file")
	}
	if err := rejectAuthorityHazards(file, fset, info); err != nil {
		return err
	}
	imports, err := preflightImportBindings(file, info, plan.Imports)
	if err != nil {
		return err
	}
	members, err := preflightMemberBindings(file, fset, info, plan.Members, imports.aliases)
	if err != nil {
		return err
	}
	applyImportEdits(file, info, imports)
	applyMemberEdits(members)
	return nil
}

func planRequiresTypeInfo(plan RoutePlan) bool {
	if len(plan.Members) != 0 {
		return true
	}
	for _, binding := range plan.Imports {
		if binding.Form != ImportAdd {
			return true
		}
	}
	return false
}

// RewriteMemberBindings is the standalone finite member primitive. It admits
// only receiver-based field/direct-method routes. Package-selector bindings
// require ApplyRoutePlan so their import authority is explicit in the same
// exact consumer-file transaction.
func RewriteMemberBindings(file *ast.File, fset *token.FileSet, info *types.Info, bindings []MemberBinding) error {
	for _, binding := range bindings {
		if binding.Form == MemberPackageSelector {
			return fmt.Errorf("package selector binding requires a route plan with import binding")
		}
	}
	return ApplyRoutePlan(file, fset, info, RoutePlan{Consumer: file, Members: bindings})
}

type importEdits struct {
	aliases map[*types.PkgName]string
	change  []importChange
	add     []importAdd
	remove  map[*ast.ImportSpec]bool
}

type importChange struct {
	spec     *ast.ImportSpec
	path     string
	rawAlias string
}

type importAdd struct {
	target   FuturePackage
	rawAlias string
}

func preflightImportBindings(file *ast.File, info *types.Info, bindings []ImportBinding) (importEdits, error) {
	edits := importEdits{aliases: make(map[*types.PkgName]string), remove: make(map[*ast.ImportSpec]bool)}
	if len(bindings) == 0 {
		return edits, nil
	}
	if info == nil {
		return preflightImportAddsWithoutTypeInfo(file, bindings)
	}
	if err := uniqueFutureImportTargets(bindings); err != nil {
		return importEdits{}, err
	}
	specs := importSpecs(file)
	byName := make(map[*types.PkgName]*ast.ImportSpec, len(specs))
	byPath := make(map[string][]*types.PkgName, len(specs))
	for spec := range specs {
		name := importPackageName(info, spec)
		if name == nil || name.Imported() == nil {
			return importEdits{}, fmt.Errorf("%s: import lacks resolved package binding", spec.Path.Value)
		}
		byName[name] = spec
		byPath[name.Imported().Path()] = append(byPath[name.Imported().Path()], name)
	}
	seen := make(map[*types.PkgName]bool, len(bindings))
	added := make(map[string]bool, len(bindings))
	reserved := make(map[string]*types.PkgName, len(byName))
	for name := range byName {
		reserved[name.Name()] = name
	}
	for _, binding := range bindings {
		if err := binding.validate(); err != nil {
			return importEdits{}, err
		}
		switch binding.Form {
		case ImportAdd:
			if added[binding.Target.Path] || len(byPath[binding.Target.Path]) != 0 {
				return importEdits{}, fmt.Errorf("future package %s is already imported or added", binding.Target.Path)
			}
			alias := binding.effectiveAlias()
			if occupied, exists := reserved[alias]; exists {
				return importEdits{}, importAliasCollision(alias, occupied)
			}
			reserved[alias] = nil
			added[binding.Target.Path] = true
			edits.add = append(edits.add, importAdd{target: binding.Target, rawAlias: binding.Alias})
		case ImportRemove:
			spec, err := sourceImportSpec(file, info, byName, seen, binding.From, false)
			if err != nil {
				return importEdits{}, err
			}
			if len(packageSelectorsFor(file, info, binding.From)) != 0 {
				return importEdits{}, fmt.Errorf("source import %s still has resolved selector uses", binding.From.Name())
			}
			edits.remove[spec] = true
		case ImportReplace:
			spec, err := sourceImportSpec(file, info, byName, seen, binding.From, true)
			if err != nil {
				return importEdits{}, err
			}
			alias := binding.effectiveAlias()
			allowed := types.Object(binding.From)
			targetNames := distinctTargetNames(byPath[binding.Target.Path], binding.From)
			if len(targetNames) != 0 {
				if len(targetNames) != 1 {
					return importEdits{}, fmt.Errorf("target package %s is imported more than once", binding.Target.Path)
				}
				target := targetNames[0]
				if target.Imported().Name() != binding.Target.Name {
					return importEdits{}, fmt.Errorf("target import %s declares %q, not locked %q", binding.Target.Path, target.Imported().Name(), binding.Target.Name)
				}
				if target.Name() != alias {
					return importEdits{}, fmt.Errorf("target package %s already has exact alias %q, not %q", binding.Target.Path, target.Name(), alias)
				}
				allowed = target
				edits.remove[spec] = true
			} else {
				if occupied, exists := reserved[alias]; exists && occupied != binding.From {
					return importEdits{}, importAliasCollision(alias, occupied)
				}
				reserved[alias] = binding.From
				edits.change = append(edits.change, importChange{spec: spec, path: binding.Target.Path, rawAlias: binding.Alias})
			}
			if alias != binding.From.Name() {
				if err := rejectAliasShadowing(file, info, binding.From, alias, allowed); err != nil {
					return importEdits{}, err
				}
			}
			edits.aliases[binding.From] = alias
		}
	}
	return edits, nil
}

func uniqueFutureImportTargets(bindings []ImportBinding) error {
	seen := make(map[string]bool, len(bindings))
	for _, binding := range bindings {
		if binding.Form != ImportAdd && binding.Form != ImportReplace {
			continue
		}
		if seen[binding.Target.Path] {
			return fmt.Errorf("multiple import bindings target future package %s; split the cut explicitly", binding.Target.Path)
		}
		seen[binding.Target.Path] = true
	}
	return nil
}

func preflightImportAddsWithoutTypeInfo(file *ast.File, bindings []ImportBinding) (importEdits, error) {
	edits := importEdits{aliases: make(map[*types.PkgName]string), remove: make(map[*ast.ImportSpec]bool)}
	if err := uniqueFutureImportTargets(bindings); err != nil {
		return importEdits{}, err
	}
	paths := make(map[string]bool, len(file.Imports))
	aliases := make(map[string]bool, len(file.Imports))
	for _, spec := range file.Imports {
		path, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			return importEdits{}, fmt.Errorf("invalid import path %s", spec.Path.Value)
		}
		paths[path] = true
		if spec.Name == nil {
			return importEdits{}, fmt.Errorf("type information is required to add beside implicit import %s", path)
		}
		if spec.Name.Name == "." || spec.Name.Name == "_" {
			return importEdits{}, fmt.Errorf("type information is required to add beside non-qualified import %s", path)
		}
		aliases[spec.Name.Name] = true
	}
	for _, binding := range bindings {
		if err := binding.validate(); err != nil {
			return importEdits{}, err
		}
		if binding.Form != ImportAdd {
			return importEdits{}, fmt.Errorf("type information is required for %s import binding", binding.Form)
		}
		if paths[binding.Target.Path] {
			return importEdits{}, fmt.Errorf("future package %s is already imported", binding.Target.Path)
		}
		alias := binding.effectiveAlias()
		if aliases[alias] || declaredAliasWithoutTypes(file, alias) {
			return importEdits{}, fmt.Errorf("new import alias %q collides with an existing declaration", alias)
		}
		paths[binding.Target.Path] = true
		aliases[alias] = true
		edits.add = append(edits.add, importAdd{target: binding.Target, rawAlias: binding.Alias})
	}
	return edits, nil
}

func declaredAliasWithoutTypes(file *ast.File, alias string) bool {
	var found bool
	ast.Inspect(file, func(node ast.Node) bool {
		ident, ok := node.(*ast.Ident)
		if !ok || ident.Name != alias || ident.Obj == nil {
			return true
		}
		if ident.Obj.Pos() == ident.Pos() {
			found = true
			return false
		}
		return true
	})
	return found
}

func sourceImportSpec(file *ast.File, info *types.Info, byName map[*types.PkgName]*ast.ImportSpec, seen map[*types.PkgName]bool, from *types.PkgName, requireUse bool) (*ast.ImportSpec, error) {
	if seen[from] {
		return nil, fmt.Errorf("duplicate import binding for %s", from.Name())
	}
	seen[from] = true
	spec := byName[from]
	if spec == nil {
		return nil, fmt.Errorf("source import %s is not bound in target file", from.Name())
	}
	if requireUse && len(packageSelectorsFor(file, info, from)) == 0 {
		return nil, fmt.Errorf("source import %s has no resolved selector use", from.Name())
	}
	return spec, nil
}

func importAliasCollision(alias string, occupied *types.PkgName) error {
	if occupied == nil {
		return fmt.Errorf("new import alias %q collides with another planned addition", alias)
	}
	return fmt.Errorf("new import alias %q collides with %s", alias, occupied.Imported().Path())
}

func distinctTargetNames(names []*types.PkgName, source *types.PkgName) []*types.PkgName {
	targets := make([]*types.PkgName, 0, len(names))
	for _, name := range names {
		if name != source {
			targets = append(targets, name)
		}
	}
	return targets
}

func importPackageName(info *types.Info, spec *ast.ImportSpec) *types.PkgName {
	if spec.Name != nil {
		name, _ := info.Defs[spec.Name].(*types.PkgName)
		return name
	}
	name, _ := info.Implicits[spec].(*types.PkgName)
	return name
}

func importSpecs(file *ast.File) map[*ast.ImportSpec]*ast.GenDecl {
	specs := make(map[*ast.ImportSpec]*ast.GenDecl, len(file.Imports))
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.IMPORT {
			continue
		}
		for _, raw := range gen.Specs {
			if spec, ok := raw.(*ast.ImportSpec); ok {
				specs[spec] = gen
			}
		}
	}
	return specs
}

func rejectAliasShadowing(file *ast.File, info *types.Info, from *types.PkgName, alias string, allowed types.Object) error {
	for _, selector := range packageSelectorsFor(file, info, from) {
		if object := objectAt(info, selector.Pos(), alias); object != nil && object != from && object != allowed {
			return fmt.Errorf("position %d: import alias %q is shadowed by %s", selector.Pos(), alias, object.Name())
		}
	}
	return nil
}

func objectAt(info *types.Info, pos token.Pos, name string) types.Object {
	var closest *types.Scope
	for node, scope := range info.Scopes {
		if node.Pos() <= pos && pos <= node.End() {
			if closest == nil || (closest.Pos() <= scope.Pos() && scope.End() <= closest.End()) {
				closest = scope
			}
		}
	}
	if closest == nil {
		return objectAtByDefinition(info, pos, name)
	}
	_, object := closest.LookupParent(name, pos)
	if object != nil {
		return object
	}
	return objectAtByDefinition(info, pos, name)
}

func objectAtByDefinition(info *types.Info, pos token.Pos, name string) types.Object {
	for ident, object := range info.Defs {
		if ident.Name != name || object == nil || object.Parent() == nil || ident.Pos() >= pos {
			continue
		}
		if object.Parent().Contains(pos) {
			return object
		}
	}
	return nil
}

func packageSelectorsFor(file *ast.File, info *types.Info, name *types.PkgName) []*ast.SelectorExpr {
	var selectors []*ast.SelectorExpr
	ast.Inspect(file, func(node ast.Node) bool {
		selector, ok := node.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		root, ok := selector.X.(*ast.Ident)
		if ok && info.Uses[root] == name {
			selectors = append(selectors, selector)
		}
		return true
	})
	return selectors
}

func applyImportEdits(file *ast.File, info *types.Info, edits importEdits) {
	for _, change := range edits.change {
		change.spec.Path.Value = fmt.Sprintf("%q", change.path)
		change.spec.Name = importSpecName(change.rawAlias)
	}
	if len(edits.remove) != 0 {
		rebuildImports(file, edits.remove)
	}
	for _, addition := range edits.add {
		addImport(file, addition)
	}
	for from, alias := range edits.aliases {
		for _, selector := range packageSelectorsFor(file, info, from) {
			root := selector.X.(*ast.Ident)
			root.Name = alias
		}
	}
}

func addImport(file *ast.File, addition importAdd) {
	spec := &ast.ImportSpec{
		Name: importSpecName(addition.rawAlias),
		Path: &ast.BasicLit{Kind: token.STRING, Value: fmt.Sprintf("%q", addition.target.Path)},
	}
	for _, decl := range file.Decls {
		if group, ok := decl.(*ast.GenDecl); ok && group.Tok == token.IMPORT {
			group.Specs = append(group.Specs, spec)
			file.Imports = append(file.Imports, spec)
			return
		}
	}
	group := &ast.GenDecl{Tok: token.IMPORT, Specs: []ast.Spec{spec}}
	file.Decls = append([]ast.Decl{group}, file.Decls...)
	file.Imports = append([]*ast.ImportSpec{spec}, file.Imports...)
}

func importSpecName(rawAlias string) *ast.Ident {
	if rawAlias == "" {
		return nil
	}
	return ast.NewIdent(rawAlias)
}

func rebuildImports(file *ast.File, remove map[*ast.ImportSpec]bool) {
	decls := make([]ast.Decl, 0, len(file.Decls))
	imports := make([]*ast.ImportSpec, 0, len(file.Imports)-len(remove))
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.IMPORT {
			decls = append(decls, decl)
			continue
		}
		specs := make([]ast.Spec, 0, len(gen.Specs))
		for _, raw := range gen.Specs {
			spec, ok := raw.(*ast.ImportSpec)
			if ok && remove[spec] {
				continue
			}
			specs = append(specs, raw)
			if ok {
				imports = append(imports, spec)
			}
		}
		if len(specs) == 0 {
			continue
		}
		gen.Specs = specs
		decls = append(decls, gen)
	}
	file.Decls = decls
	file.Imports = imports
}

type memberEdit struct {
	selector  *ast.SelectorExpr
	terminal  string
	via       []ReceiverStep
	qualifier string
}

func preflightMemberBindings(file *ast.File, fset *token.FileSet, info *types.Info, bindings []MemberBinding, aliases map[*types.PkgName]string) ([]memberEdit, error) {
	if len(bindings) == 0 {
		return nil, nil
	}
	if info == nil {
		return nil, fmt.Errorf("member bindings require type information")
	}
	byObject := make(map[types.Object]MemberBinding, len(bindings))
	for _, binding := range bindings {
		if err := binding.validate(); err != nil {
			return nil, err
		}
		if _, exists := byObject[binding.From]; exists {
			return nil, fmt.Errorf("duplicate member binding for %s", binding.From.Name())
		}
		if binding.Form == MemberPackageSelector {
			if _, routed := aliases[binding.Package]; !routed {
				return nil, fmt.Errorf("package selector %s requires matching import binding", binding.From.Name())
			}
		}
		byObject[binding.From] = binding
	}
	parents := parentMap(file)
	matched := make(map[types.Object]bool, len(bindings))
	var edits []memberEdit
	var first error
	ast.Inspect(file, func(node ast.Node) bool {
		if first != nil {
			return false
		}
		selector, ok := node.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		object := selectorObject(info, selector)
		binding, routed := byObject[object]
		if !routed {
			return true
		}
		matched[binding.From] = true
		class := ClassifySelector(info, selector, parents[selector])
		switch binding.Form {
		case MemberField:
			selection := info.Selections[selector]
			if class != FieldSelection || selection == nil || len(selection.Index()) != 1 {
				first = routeFormError(fset, selector, binding, class)
				return false
			}
			edits = append(edits, memberEdit{selector: selector, terminal: binding.Target.Name, via: binding.Via})
		case MemberDirectMethodCall:
			selection := info.Selections[selector]
			if class != MethodInvocation || selection == nil || len(selection.Index()) != 1 {
				first = routeFormError(fset, selector, binding, class)
				return false
			}
			edits = append(edits, memberEdit{selector: selector, terminal: binding.Target.Name, via: binding.Via})
		case MemberPackageSelector:
			root, ok := selector.X.(*ast.Ident)
			if class != PackageSelection || !ok || info.Uses[root] != binding.Package {
				first = routeFormError(fset, selector, binding, class)
				return false
			}
			edits = append(edits, memberEdit{selector: selector, terminal: binding.Target.Name, qualifier: aliases[binding.Package]})
		}
		return true
	})
	if first != nil {
		return nil, first
	}
	for _, binding := range bindings {
		if !matched[binding.From] {
			return nil, fmt.Errorf("member binding source %s has no resolved use in target file", binding.From.Name())
		}
	}
	return edits, nil
}

func selectorObject(info *types.Info, selector *ast.SelectorExpr) types.Object {
	if selection := info.Selections[selector]; selection != nil {
		return selection.Obj()
	}
	return info.Uses[selector.Sel]
}

func routeFormError(fset *token.FileSet, selector *ast.SelectorExpr, binding MemberBinding, got SelectorClass) error {
	return fmt.Errorf("%s: routed %s is %s; %s binding would require a bridge", fset.Position(selector.Pos()), binding.From.Name(), got, binding.Form)
}

func applyMemberEdits(edits []memberEdit) {
	for _, edit := range edits {
		if edit.qualifier != "" {
			edit.selector.X = ast.NewIdent(edit.qualifier)
		} else {
			edit.selector.X = applyReceiverPath(edit.selector.X, edit.via)
		}
		edit.selector.Sel.Name = edit.terminal
	}
}

func applyReceiverPath(receiver ast.Expr, steps []ReceiverStep) ast.Expr {
	current := receiver
	for _, step := range steps {
		selector := &ast.SelectorExpr{X: current, Sel: ast.NewIdent(step.Name)}
		if step.Form == ReceiverDirectView {
			current = &ast.CallExpr{Fun: selector}
		} else {
			current = selector
		}
	}
	return current
}

func parentMap(file *ast.File) map[ast.Node]ast.Node {
	parents := make(map[ast.Node]ast.Node)
	var stack []ast.Node
	ast.Inspect(file, func(node ast.Node) bool {
		if node == nil {
			stack = stack[:len(stack)-1]
			return false
		}
		if len(stack) != 0 {
			parents[node] = stack[len(stack)-1]
		}
		stack = append(stack, node)
		return true
	})
	return parents
}
