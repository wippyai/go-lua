package render

import (
	"fmt"
	"go/ast"
	"go/types"
	"sort"

	"github.com/wippyai/go-lua/cmd/flashrefactor/internal/cutplan"
	"github.com/wippyai/go-lua/cmd/flashrefactor/internal/gorewrite"
)

func (state *renderState) applyRoutes(requirements requirementIndex, operation cutplan.Operation) error {
	byConsumer := map[string]routeSet{}
	for _, binding := range operation.Bindings {
		entry := byConsumer[binding.Consumer]
		entry.bindings = append(entry.bindings, binding)
		byConsumer[binding.Consumer] = entry
	}
	for _, route := range operation.Imports {
		entry := byConsumer[route.Consumer]
		entry.imports = append(entry.imports, route)
		byConsumer[route.Consumer] = entry
	}
	consumers := make([]string, 0, len(byConsumer))
	for consumer := range byConsumer {
		consumers = append(consumers, consumer)
	}
	sort.Strings(consumers)
	for _, consumer := range consumers {
		if err := state.writeAllowed(consumer); err != nil {
			return err
		}
		if err := state.applyConsumerRoutes(requirements, consumer, byConsumer[consumer]); err != nil {
			return fmt.Errorf("route consumer %s: %w", consumer, err)
		}
	}
	return nil
}

type routeSet struct {
	bindings []cutplan.Binding
	imports  []cutplan.Import
}

func (state *renderState) applyConsumerRoutes(requirements requirementIndex, consumer string, routes routeSet) error {
	file := state.files[consumer]
	if file == nil {
		var err error
		file, _, _, err = state.existingFile(consumer)
		if err != nil {
			return fmt.Errorf("route consumer %s: %w", consumer, err)
		}
	}
	if err := file.ensureGo(); err != nil {
		return err
	}
	plan, err := state.routePlanForFile(requirements, file, routes)
	if err != nil {
		return err
	}
	if len(plan.Imports) != 0 || len(plan.Members) != 0 {
		if err := gorewrite.ApplyRoutePlan(file.file, file.fset, file.info, plan); err != nil {
			return fmt.Errorf("route consumer %s: %w", consumer, err)
		}
	}
	for _, binding := range routes.bindings {
		if binding.Form != cutplan.BindingDirect {
			continue
		}
		if err := state.applyDirectBinding(requirements, file, binding); err != nil {
			return err
		}
	}
	return nil
}

func (state *renderState) routePlanForFile(requirements requirementIndex, file *fileState, routes routeSet) (gorewrite.RoutePlan, error) {
	plan := gorewrite.RoutePlan{Consumer: file.file}
	for _, route := range routes.imports {
		binding, err := state.importBinding(file, route)
		if err != nil {
			return gorewrite.RoutePlan{}, err
		}
		plan.Imports = append(plan.Imports, binding)
	}
	for _, binding := range routes.bindings {
		if binding.Form == cutplan.BindingDirect {
			continue
		}
		member, err := state.memberBinding(requirements, file, binding)
		if err != nil {
			return gorewrite.RoutePlan{}, err
		}
		plan.Members = append(plan.Members, member)
	}
	return plan, nil
}

func (state *renderState) importBinding(file *fileState, route cutplan.Import) (gorewrite.ImportBinding, error) {
	if route.From == nil && route.To == nil {
		return gorewrite.ImportBinding{}, fmt.Errorf("route %s has no import endpoint", route.Consumer)
	}
	if route.From == nil {
		target, err := futurePackage(*route.To)
		if err != nil {
			return gorewrite.ImportBinding{}, err
		}
		return gorewrite.ImportBinding{Form: gorewrite.ImportAdd, Target: target, Alias: route.To.Alias}, nil
	}
	from, err := importFromInfo(file, *route.From)
	if err != nil {
		return gorewrite.ImportBinding{}, err
	}
	if route.To == nil {
		return gorewrite.ImportBinding{Form: gorewrite.ImportRemove, From: from}, nil
	}
	target, err := futurePackage(*route.To)
	if err != nil {
		return gorewrite.ImportBinding{}, err
	}
	return gorewrite.ImportBinding{Form: gorewrite.ImportReplace, From: from, Target: target, Alias: route.To.Alias}, nil
}

func futurePackage(value cutplan.ImportRef) (gorewrite.FuturePackage, error) {
	if value.Path == "" || value.Name == "" {
		return gorewrite.FuturePackage{}, fmt.Errorf("future import requires exact path and package name")
	}
	return gorewrite.FuturePackage{Path: value.Path, Name: value.Name}, nil
}

func importFromInfo(file *fileState, want cutplan.ImportRef) (*types.PkgName, error) {
	if file == nil || file.info == nil {
		return nil, fmt.Errorf("import removal/replacement needs typed source")
	}
	for _, spec := range file.file.Imports {
		name, _ := file.info.Implicits[spec].(*types.PkgName)
		if name == nil && spec.Name != nil {
			name, _ = file.info.Defs[spec.Name].(*types.PkgName)
		}
		// Alias is the exact import spelling committed in structural evidence:
		// an omitted source alias is "", even though go/types presents the
		// imported package's declared name as PkgName.Name(). Treating that
		// derived name as spelling would make every implicit import unroutable.
		alias := ""
		if spec.Name != nil {
			alias = spec.Name.Name
		}
		if name != nil && name.Imported() != nil && name.Imported().Path() == want.Path && name.Imported().Name() == want.Name && alias == want.Alias {
			return name, nil
		}
	}
	return nil, fmt.Errorf("exact source import %s as %s is not present", want.Path, want.Alias)
}

func (state *renderState) memberBinding(requirements requirementIndex, file *fileState, binding cutplan.Binding) (gorewrite.MemberBinding, error) {
	from, err := state.bindingSource(requirements, file, binding.From)
	if err != nil {
		return gorewrite.MemberBinding{}, err
	}
	terminal, err := targetName(binding.To)
	if err != nil {
		return gorewrite.MemberBinding{}, err
	}
	result := gorewrite.MemberBinding{From: from, Target: gorewrite.FutureMember{Name: terminal}}
	switch binding.Form {
	case cutplan.BindingField:
		result.Form = gorewrite.MemberField
		result.Via, err = state.receiverSteps(requirements, binding.Receiver)
	case cutplan.BindingMethodCall:
		result.Form = gorewrite.MemberDirectMethodCall
		result.Via, err = state.receiverSteps(requirements, binding.Receiver)
	case cutplan.BindingPackageSelector:
		result.Form = gorewrite.MemberPackageSelector
		if from.Pkg() == nil {
			return gorewrite.MemberBinding{}, fmt.Errorf("package selector source %s has no package", binding.From.Object)
		}
		result.Package, err = state.importForPackageSelector(file, from.Pkg())
	default:
		return gorewrite.MemberBinding{}, fmt.Errorf("unsupported non-direct binding form %q", binding.Form)
	}
	if err != nil {
		return gorewrite.MemberBinding{}, err
	}
	return result, nil
}

func (state *renderState) importForPackageSelector(file *fileState, pkg *types.Package) (*types.PkgName, error) {
	if pkg == nil {
		return nil, fmt.Errorf("package selector has no package")
	}
	for _, spec := range file.file.Imports {
		name, _ := file.info.Implicits[spec].(*types.PkgName)
		if name == nil && spec.Name != nil {
			name, _ = file.info.Defs[spec.Name].(*types.PkgName)
		}
		if name != nil && name.Imported() == pkg {
			return name, nil
		}
	}
	return nil, fmt.Errorf("package selector source import %s is not present", pkg.Path())
}

func (state *renderState) bindingSource(requirements requirementIndex, file *fileState, ref cutplan.SymbolRef) (types.Object, error) {
	requirement, exists := requirements[ref.Object]
	if !exists || requirement.Role != cutplan.ObjectSource {
		return nil, fmt.Errorf("binding source %s is not a reviewed source object", ref.Object)
	}
	if file == nil || file.origin.AST == nil {
		return nil, fmt.Errorf("binding source %s has no exact consumer package variant", ref.Object)
	}
	object, err := state.workspace.ObjectForFile(ref, file.origin)
	if err != nil {
		return nil, fmt.Errorf("binding source %s in %s: %w", ref.Object, file.path, err)
	}
	return object, nil
}

func (state *renderState) receiverSteps(requirements requirementIndex, values []cutplan.ReceiverPathStep) ([]gorewrite.ReceiverStep, error) {
	result := make([]gorewrite.ReceiverStep, 0, len(values))
	for _, value := range values {
		requirement, exists := requirements[value.Object.Object]
		if !exists || requirement.Role != cutplan.ObjectTarget {
			return nil, fmt.Errorf("receiver step %s is not a reviewed target object", value.Object.Object)
		}
		shape, err := parseSymbol(value.Object)
		if err != nil {
			return nil, err
		}
		step := gorewrite.ReceiverStep{Name: shape.member}
		switch value.Kind {
		case cutplan.ReceiverField:
			if shape.kind != symbolField {
				return nil, fmt.Errorf("field receiver step must target a field: %s", value.Object.Object)
			}
			step.Form = gorewrite.ReceiverField
		case cutplan.ReceiverDirectView:
			if shape.kind != symbolMethod {
				return nil, fmt.Errorf("direct-view receiver step must target a method: %s", value.Object.Object)
			}
			// The target is generally new, so a pre-cut types.Signature is not
			// available. Its exact signature is a post-state semantic gate; the
			// renderer's finite grammar only emits the declared zero-arg call.
			step.Form = gorewrite.ReceiverDirectView
		default:
			return nil, fmt.Errorf("unknown receiver step form %q", value.Kind)
		}
		result = append(result, step)
	}
	return result, nil
}

func (state *renderState) applyDirectBinding(requirements requirementIndex, file *fileState, binding cutplan.Binding) error {
	from, err := state.bindingSource(requirements, file, binding.From)
	if err != nil {
		return err
	}
	if from.Pkg() == nil {
		return fmt.Errorf("direct binding source %s has no package", binding.From.Object)
	}
	terminal, err := targetName(binding.To)
	if err != nil {
		return err
	}
	if from.Name() == terminal {
		return fmt.Errorf("direct binding %s is unchanged", binding.From.Object)
	}
	seen := false
	ast.Inspect(file.file, func(node ast.Node) bool {
		identifier, ok := node.(*ast.Ident)
		if !ok {
			return true
		}
		if file.info.Defs[identifier] == from || file.info.Uses[identifier] == from {
			identifier.Name = terminal
			seen = true
		}
		return true
	})
	if !seen {
		return fmt.Errorf("direct binding %s has no resolved use in %s", binding.From.Object, file.path)
	}
	return nil
}
