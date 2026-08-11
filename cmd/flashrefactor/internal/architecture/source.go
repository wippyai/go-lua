package architecture

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"sort"

	"github.com/wippyai/go-lua/cmd/flashrefactor/internal/cutplan"
	"github.com/wippyai/go-lua/cmd/flashrefactor/internal/gorewrite"
	"github.com/wippyai/go-lua/cmd/flashrefactor/internal/semantic"
)

type containmentSource struct {
	path             string
	parentImportPath string
	parentName       string
	fields           []sourceField
}

type sourceField struct {
	ref      cutplan.SymbolRef
	name     string
	evidence cutplan.ObjectEvidence
}

func sourceContainment(declaration Declaration, snapshot semantic.Snapshot) (containmentSource, error) {
	evidence := sourceEvidence(snapshot)
	parentEvidence, exists := evidence[declaration.Parent.Object]
	if !exists {
		return containmentSource{}, fmt.Errorf("architecture survey misses parent %s", declaration.Parent.Object)
	}
	workspace := snapshot.Workspace
	parentObject, err := workspace.Object(declaration.Parent)
	if err != nil {
		return containmentSource{}, fmt.Errorf("resolve containment parent: %w", err)
	}
	parent, ok := parentObject.(*types.TypeName)
	if !ok || parent.IsAlias() {
		return containmentSource{}, fmt.Errorf("containment parent must be one non-alias named type")
	}
	structure, ok := parent.Type().Underlying().(*types.Struct)
	if !ok {
		return containmentSource{}, fmt.Errorf("containment parent %s is not a struct", declaration.Parent.Object)
	}
	if parent.Pkg() == nil || parent.Pkg().Path() == "" {
		return containmentSource{}, fmt.Errorf("containment parent has no package identity")
	}

	direct := make(map[types.Object]bool, structure.NumFields())
	for index := 0; index < structure.NumFields(); index++ {
		direct[structure.Field(index)] = true
	}
	result := containmentSource{
		path:             parentEvidence.Definition.Path,
		parentImportPath: parent.Pkg().Path(),
		parentName:       parent.Name(),
		fields:           make([]sourceField, 0, len(declaration.Fields)),
	}
	for _, ref := range canonicalRefs(declaration.Fields) {
		fieldEvidence, exists := evidence[ref.Object]
		if !exists {
			return containmentSource{}, fmt.Errorf("architecture survey misses field %s", ref.Object)
		}
		if fieldEvidence.Definition.Path != result.path {
			return containmentSource{}, fmt.Errorf("containment field %s is defined in %s, not parent source %s", ref.Object, fieldEvidence.Definition.Path, result.path)
		}
		object, objectErr := workspace.Object(ref)
		field, isField := object.(*types.Var)
		if objectErr != nil || !isField || !field.IsField() || !direct[field] {
			return containmentSource{}, fmt.Errorf("containment field %s is not a direct field of %s", ref.Object, declaration.Parent.Object)
		}
		result.fields = append(result.fields, sourceField{ref: ref, name: field.Name(), evidence: fieldEvidence})
	}
	if declaration.Destination.Child == parent.Name() {
		return containmentSource{}, fmt.Errorf("containment child must differ from parent")
	}
	if declaration.Destination.Through == "" {
		return containmentSource{}, fmt.Errorf("containment destination requires a through field")
	}
	for index := 0; index < structure.NumFields(); index++ {
		if structure.Field(index).Name() == declaration.Destination.Through {
			return containmentSource{}, fmt.Errorf("containment through field already exists on %s", declaration.Parent.Object)
		}
	}
	source, err := workspace.File(result.path)
	if err != nil {
		return containmentSource{}, fmt.Errorf("containment source syntax %s: %w", result.path, err)
	}
	fieldNames := make(map[string]struct{}, len(result.fields))
	for _, field := range result.fields {
		fieldNames[field.name] = struct{}{}
	}
	if err := gorewrite.CheckNamedFieldRelocationLiterals(source.AST, workspace.FileSet(), gorewrite.FieldRelocation{
		Owner: parent.Name(), Child: declaration.Destination.Child, ChildField: declaration.Destination.Through, Fields: fieldNames,
	}); err != nil {
		return containmentSource{}, fmt.Errorf("containment source literals: %w", err)
	}
	if err := validateForeignParentLiterals(result, snapshot); err != nil {
		return containmentSource{}, err
	}
	return result, nil
}

func sourceEvidence(snapshot semantic.Snapshot) map[string]cutplan.ObjectEvidence {
	result := make(map[string]cutplan.ObjectEvidence, len(snapshot.Objects))
	for _, object := range snapshot.Objects {
		result[object.Object.Object] = object
	}
	return result
}

func validateDestination(destination ContainmentDestination, snapshot semantic.Snapshot) (bool, error) {
	if destination.Path == "" || destination.ImportPath == "" || destination.Package == "" || destination.Child == "" || destination.Through == "" {
		return false, fmt.Errorf("containment destination requires path, import path, package, child, and through identities")
	}
	packageNames := map[string]string{}
	for _, pkg := range snapshot.Structure.Packages {
		packageNames[pkg.ID] = pkg.Name
	}
	exists := false
	for _, file := range snapshot.Structure.Files {
		if file.Path != destination.Path {
			continue
		}
		exists = true
		name, known := packageNames[file.PackageID]
		if !known || file.PackagePath != destination.ImportPath || name != destination.Package {
			return false, fmt.Errorf("destination %s is ambiguous or has different package identity", destination.Path)
		}
	}
	return exists, nil
}

func validateTargetAbsence(model containmentSource, declaration Declaration, snapshot semantic.Snapshot) error {
	workspace := snapshot.Workspace
	if object, found, err := workspace.LookupObject(targetChild(declaration)); err != nil {
		return fmt.Errorf("containment child target is ambiguous: %w", err)
	} else if found && object != nil {
		return fmt.Errorf("containment child target already exists pre-cut: %s", targetChild(declaration).Object)
	}
	if object, found, err := workspace.LookupObject(targetThrough(model, declaration)); err != nil {
		return fmt.Errorf("containment through target is ambiguous: %w", err)
	} else if found && object != nil {
		return fmt.Errorf("containment through target already exists pre-cut: %s", targetThrough(model, declaration).Object)
	}
	return nil
}

type physicalSite struct {
	path   string
	offset int
	role   cutplan.SiteRole
}

type fieldUses struct {
	field         sourceField
	expected      map[physicalSite]bool
	seen          map[physicalSite]bool
	selectorPaths map[string]bool
}

func deriveFieldBindings(model containmentSource, declaration Declaration, snapshot semantic.Snapshot) ([]cutplan.Binding, error) {
	uses := make(map[string]*fieldUses, len(model.fields))
	for _, field := range model.fields {
		entry := &fieldUses{field: field, expected: map[physicalSite]bool{}, seen: map[physicalSite]bool{}, selectorPaths: map[string]bool{}}
		for _, reference := range field.evidence.References {
			entry.expected[physicalSite{path: reference.Path, offset: reference.Offset, role: reference.Role}] = true
		}
		uses[field.ref.Object] = entry
	}
	workspace := snapshot.Workspace
	for _, file := range workspace.Files() {
		active, err := visibleFieldsForFile(workspace, file, uses)
		if err != nil {
			return nil, err
		}
		if len(active) == 0 {
			continue
		}
		pkg, err := workspace.PackageForFile(file)
		if err != nil {
			return nil, fmt.Errorf("source route package %s: %w", file.Path, err)
		}
		parentObject, err := workspace.ObjectForFile(declaration.Parent, file)
		if err != nil {
			return nil, fmt.Errorf("source route parent %s in %s: %w", declaration.Parent.Object, file.Path, err)
		}
		parent, ok := parentObject.(*types.TypeName)
		if !ok || parent.IsAlias() {
			return nil, fmt.Errorf("source route parent is not one named type in %s", file.Path)
		}
		if pkg.Info == nil || file.AST == nil {
			return nil, fmt.Errorf("source route file lacks typed syntax: %s", file.Path)
		}
		parents := parentIndex(file.AST)
		for _, identifier := range sortedUses(pkg.Info) {
			entry := active[pkg.Info.Uses[identifier]]
			if entry == nil {
				continue
			}
			position, err := sourcePosition(workspace.FileSet(), file.Path, identifier)
			if err != nil {
				return nil, err
			}
			kind, err := classifyFieldUse(file.Path, model.path, pkg.Info, identifier, parents, pkg.Info.Uses[identifier], entry.field, parent)
			if err != nil {
				return nil, fmt.Errorf("field route %s at %s:%d: %w", entry.field.ref.Object, file.Path, position.offset, err)
			}
			if kind == fieldUseSelector {
				position.role = cutplan.SiteSelector
			}
			if !entry.expected[position] {
				return nil, fmt.Errorf("field route %s has a use outside resolver denominator at %s:%d", entry.field.ref.Object, file.Path, position.offset)
			}
			entry.seen[position] = true
			if kind == fieldUseSelector {
				entry.selectorPaths[file.Path] = true
			}
		}
	}

	bindings := make([]cutplan.Binding, 0)
	through := targetThrough(model, declaration)
	for _, field := range model.fields {
		entry := uses[field.ref.Object]
		for expected := range entry.expected {
			if !entry.seen[expected] {
				return nil, fmt.Errorf("resolver field reference lacks an exact supported route: %s at %s:%d", field.ref.Object, expected.path, expected.offset)
			}
		}
		for _, consumer := range sortedPaths(entry.selectorPaths) {
			bindings = append(bindings, cutplan.Binding{
				Consumer: consumer,
				From:     field.ref,
				To:       targetField(declaration, field.name),
				Form:     cutplan.BindingField,
				Receiver: []cutplan.ReceiverPathStep{{Kind: cutplan.ReceiverField, Object: through}},
			})
		}
	}
	sort.Slice(bindings, func(left, right int) bool {
		if bindings[left].Consumer != bindings[right].Consumer {
			return bindings[left].Consumer < bindings[right].Consumer
		}
		return bindings[left].From.Object < bindings[right].From.Object
	})
	return bindings, nil
}

func visibleFieldsForFile(workspace *semantic.Workspace, file semantic.WorkspaceFile, uses map[string]*fieldUses) (map[types.Object]*fieldUses, error) {
	result := map[types.Object]*fieldUses{}
	for _, entry := range uses {
		if !hasExpectedPath(entry.expected, file.Path) {
			continue
		}
		object, err := workspace.ObjectForFile(entry.field.ref, file)
		if err != nil {
			return nil, fmt.Errorf("resolve field %s in %s: %w", entry.field.ref.Object, file.Path, err)
		}
		if _, exists := result[object]; exists {
			return nil, fmt.Errorf("field route has ambiguous visible source object in %s", file.Path)
		}
		result[object] = entry
	}
	return result, nil
}

func hasExpectedPath(expected map[physicalSite]bool, path string) bool {
	for site := range expected {
		if site.path == path {
			return true
		}
	}
	return false
}

type fieldUseKind uint8

const (
	fieldUseUnknown fieldUseKind = iota
	fieldUseSelector
	fieldUseSourceLiteral
)

func classifyFieldUse(path, sourcePath string, info *types.Info, identifier *ast.Ident, parents map[ast.Node]ast.Node, resolved types.Object, field sourceField, parent *types.TypeName) (fieldUseKind, error) {
	if selector, ok := parents[identifier].(*ast.SelectorExpr); ok && selector.Sel == identifier {
		if gorewrite.ClassifySelector(info, selector, parents[selector]) != gorewrite.FieldSelection {
			return fieldUseUnknown, fmt.Errorf("field use is not a direct field selection")
		}
		selection := info.Selections[selector]
		if selection == nil || selection.Kind() != types.FieldVal || selection.Obj() != resolved || selection.Obj().Name() != field.name || len(selection.Index()) != 1 || !sameNamedType(selection.Recv(), parent) {
			return fieldUseUnknown, fmt.Errorf("field selection is promoted, indirect, or has a different receiver")
		}
		return fieldUseSelector, nil
	}
	key, ok := parents[identifier].(*ast.KeyValueExpr)
	if !ok || key.Key != identifier {
		return fieldUseUnknown, fmt.Errorf("field use has no finite containment route")
	}
	literal, ok := parents[key].(*ast.CompositeLit)
	if !ok || path != sourcePath || !sameNamedType(info.TypeOf(literal.Type), parent) {
		return fieldUseUnknown, fmt.Errorf("field literal is not the source parent literal")
	}
	return fieldUseSourceLiteral, nil
}

func sameNamedType(value types.Type, parent *types.TypeName) bool {
	if value == nil || parent == nil {
		return false
	}
	if parent.Pkg() == nil {
		return false
	}
	return sameNamedIdentity(value, parent.Pkg().Path(), parent.Name())
}

func sameNamedIdentity(value types.Type, packagePath, name string) bool {
	if value == nil || packagePath == "" || name == "" {
		return false
	}
	if pointer, ok := value.(*types.Pointer); ok {
		value = pointer.Elem()
	}
	named, ok := value.(*types.Named)
	if !ok || named.Obj() == nil {
		return false
	}
	return named.Obj().Name() == name && named.Obj().Pkg() != nil && named.Obj().Pkg().Path() == packagePath
}

func validateForeignParentLiterals(model containmentSource, snapshot semantic.Snapshot) error {
	for _, file := range snapshot.Workspace.Files() {
		if file.Path == model.path || file.AST == nil {
			continue
		}
		pkg, err := snapshot.Workspace.PackageForFile(file)
		if err != nil {
			return fmt.Errorf("containment literal package %s: %w", file.Path, err)
		}
		if pkg.Info == nil {
			return fmt.Errorf("containment literal file lacks type information: %s", file.Path)
		}
		var rejected error
		ast.Inspect(file.AST, func(node ast.Node) bool {
			if rejected != nil {
				return false
			}
			literal, ok := node.(*ast.CompositeLit)
			if !ok || len(literal.Elts) == 0 || !sameNamedIdentity(pkg.Info.TypeOf(literal.Type), model.parentImportPath, model.parentName) {
				return true
			}
			for _, element := range literal.Elts {
				if _, keyed := element.(*ast.KeyValueExpr); !keyed {
					rejected = fmt.Errorf("unkeyed %s literal outside containment source is not routable: %s", model.parentName, file.Path)
					return false
				}
			}
			return true
		})
		if rejected != nil {
			return rejected
		}
	}
	return nil
}

// validateCrossPackageVisibility proves that the renderer's one direct route
// remains a legal Go expression after the child moves to another package. It
// intentionally derives this from typed use sites rather than guessing from
// spelling or accepting a later post-render diagnostic as the first signal.
func validateCrossPackageVisibility(model containmentSource, declaration Declaration, bindings []cutplan.Binding, snapshot semantic.Snapshot) error {
	if model.parentImportPath == declaration.Destination.ImportPath {
		return nil
	}
	if !ast.IsExported(declaration.Destination.Child) {
		return fmt.Errorf("cross-package containment child %s must be exported", declaration.Destination.Child)
	}

	for _, field := range model.fields {
		for _, reference := range field.evidence.References {
			file, err := snapshot.Workspace.File(reference.Path)
			if err != nil {
				return fmt.Errorf("containment visibility source %s: %w", reference.Path, err)
			}
			if file.PackagePath != declaration.Destination.ImportPath && !ast.IsExported(field.name) {
				return fmt.Errorf("cross-package containment field %s must be exported for %s", field.name, reference.Path)
			}
		}
	}

	for _, binding := range bindings {
		file, err := snapshot.Workspace.File(binding.Consumer)
		if err != nil {
			return fmt.Errorf("containment visibility consumer %s: %w", binding.Consumer, err)
		}
		if file.PackagePath != model.parentImportPath && !ast.IsExported(declaration.Destination.Through) {
			return fmt.Errorf("cross-package containment through field %s must be exported for %s", declaration.Destination.Through, binding.Consumer)
		}
	}
	return nil
}

func parentIndex(root ast.Node) map[ast.Node]ast.Node {
	result := map[ast.Node]ast.Node{}
	stack := make([]ast.Node, 0)
	ast.Inspect(root, func(node ast.Node) bool {
		if node == nil {
			stack = stack[:len(stack)-1]
			return false
		}
		if len(stack) != 0 {
			result[node] = stack[len(stack)-1]
		}
		stack = append(stack, node)
		return true
	})
	return result
}

func sortedUses(info *types.Info) []*ast.Ident {
	result := make([]*ast.Ident, 0, len(info.Uses))
	for identifier := range info.Uses {
		result = append(result, identifier)
	}
	sort.Slice(result, func(left, right int) bool {
		if result[left].Pos() != result[right].Pos() {
			return result[left].Pos() < result[right].Pos()
		}
		return result[left].Name < result[right].Name
	})
	return result
}

func sourcePosition(files *token.FileSet, path string, identifier *ast.Ident) (physicalSite, error) {
	if files == nil || identifier == nil {
		return physicalSite{}, fmt.Errorf("missing typed identifier")
	}
	file := files.File(identifier.Pos())
	if file == nil {
		return physicalSite{}, fmt.Errorf("identifier has no source file")
	}
	return physicalSite{path: path, offset: file.Offset(identifier.Pos()), role: cutplan.SiteUse}, nil
}

func deriveImports(model containmentSource, declaration Declaration, snapshot semantic.Snapshot) ([]cutplan.Import, error) {
	if model.parentImportPath == declaration.Destination.ImportPath {
		return nil, nil
	}
	for _, file := range snapshot.Structure.Files {
		if file.Path != model.path {
			continue
		}
		for _, imported := range file.Imports {
			if imported.Path == declaration.Destination.ImportPath {
				return nil, fmt.Errorf("source already imports containment destination %s; no-op import routes are intentionally unsupported", imported.Path)
			}
			name := imported.Name
			if imported.Alias != "" {
				name = imported.Alias
			}
			if name == declaration.Destination.Package {
				return nil, fmt.Errorf("new containment import %q collides with existing import in %s", name, model.path)
			}
		}
	}
	for _, file := range snapshot.Workspace.Files() {
		if file.Path != model.path {
			continue
		}
		pkg, err := snapshot.Workspace.PackageForFile(file)
		if err != nil {
			return nil, fmt.Errorf("source import package %s: %w", model.path, err)
		}
		if pkg.Types != nil && pkg.Types.Scope().Lookup(declaration.Destination.Package) != nil {
			return nil, fmt.Errorf("new containment import %q collides with package declaration in %s", declaration.Destination.Package, model.path)
		}
	}
	return []cutplan.Import{{
		Consumer: model.path,
		To: &cutplan.ImportRef{
			Path: declaration.Destination.ImportPath,
			Name: declaration.Destination.Package,
		},
		Symbols: []cutplan.SymbolRef{targetChild(declaration)},
	}}, nil
}
