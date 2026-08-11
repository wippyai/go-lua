package architecture

import (
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/cmd/flashrefactor/internal/cutplan"
)

// Compile produces the one complete mechanical Intent implied by declaration
// and its resolver-backed Survey. It never guesses a missing boundary member
// or falls back to an untyped/textual route.
func Compile(declaration Declaration, survey Survey) (cutplan.Intent, error) {
	if err := surveyMatchesDeclaration(survey, declaration); err != nil {
		return cutplan.Intent{}, err
	}
	model, err := sourceContainment(declaration, survey.snapshot)
	if err != nil {
		return cutplan.Intent{}, err
	}
	destinationExists, err := validateDestination(declaration.Destination, survey.snapshot)
	if err != nil {
		return cutplan.Intent{}, err
	}
	if err := validateTargetAbsence(model, declaration, survey.snapshot); err != nil {
		return cutplan.Intent{}, err
	}
	bindings, err := deriveFieldBindings(model, declaration, survey.snapshot)
	if err != nil {
		return cutplan.Intent{}, err
	}
	if err := validateCrossPackageVisibility(model, declaration, bindings, survey.snapshot); err != nil {
		return cutplan.Intent{}, err
	}
	imports, err := deriveImports(model, declaration, survey.snapshot)
	if err != nil {
		return cutplan.Intent{}, err
	}

	subjects := make([]cutplan.Relocation, 0, len(model.fields))
	for _, field := range model.fields {
		subjects = append(subjects, cutplan.Relocation{From: field.ref, To: targetField(declaration, field.name)})
	}
	sort.Slice(subjects, func(left, right int) bool {
		return subjects[left].From.Object < subjects[right].From.Object
	})

	read, write := deriveFootprint(model.path, declaration.Destination.Path, destinationExists, bindings, imports)
	intent := cutplan.Intent{
		Schema: cutplan.Version,
		Name:   declaration.Name,
		Operations: []cutplan.Operation{{
			ID:        declaration.Boundary.ID,
			Authority: cutplan.Authority{From: declaration.Boundary.From, To: declaration.Boundary.To},
			Edits: []cutplan.Edit{{Kind: cutplan.EditRelocate, Relocate: &cutplan.Relocate{
				Source: model.path,
				Destination: cutplan.Destination{
					Path:    declaration.Destination.Path,
					Package: declaration.Destination.Package,
				},
				Subjects: subjects,
				Containment: &cutplan.Containment{
					Parent:  declaration.Parent,
					Child:   targetChild(declaration),
					Through: targetThrough(model, declaration),
				},
			}}},
			Bindings:  bindings,
			Imports:   imports,
			Footprint: cutplan.Footprint{Read: read, Write: write},
			Verify: cutplan.Verification{
				Laws: append([]cutplan.Law(nil), declaration.Laws...),
				Gates: []cutplan.Gate{
					cutplan.GateDiagnostics,
					cutplan.GateImportDAG,
					cutplan.GateResidue,
				},
			},
		}},
	}
	canonical, err := cutplan.CanonicalIntent(intent)
	if err != nil {
		return cutplan.Intent{}, fmt.Errorf("architecture compilation produced invalid intent: %w", err)
	}
	return canonical, nil
}

func surveyMatchesDeclaration(survey Survey, declaration Declaration) error {
	if !sameDeclaration(survey.declaration, declaration) {
		return fmt.Errorf("architecture survey does not belong to declaration")
	}
	return validateSurvey(survey.snapshot, declaration.Parent, declaration.Fields)
}

func targetChild(declaration Declaration) cutplan.SymbolRef {
	return cutplan.SymbolRef{Object: declaration.Destination.ImportPath + "#package:" + declaration.Destination.Child}
}

func targetField(declaration Declaration, name string) cutplan.SymbolRef {
	return cutplan.SymbolRef{Object: declaration.Destination.ImportPath + "#type:" + declaration.Destination.Child + "/field:" + name}
}

func targetThrough(model containmentSource, declaration Declaration) cutplan.SymbolRef {
	return cutplan.SymbolRef{Object: model.parentImportPath + "#type:" + model.parentName + "/field:" + declaration.Destination.Through}
}

func deriveFootprint(source, destination string, destinationExists bool, bindings []cutplan.Binding, imports []cutplan.Import) ([]string, []string) {
	readSet := map[string]bool{source: true}
	if destinationExists {
		readSet[destination] = true
	}
	writeSet := map[string]bool{source: true, destination: true}
	for _, binding := range bindings {
		readSet[binding.Consumer] = true
		writeSet[binding.Consumer] = true
	}
	for _, route := range imports {
		readSet[route.Consumer] = true
		writeSet[route.Consumer] = true
	}
	return sortedPaths(readSet), sortedPaths(writeSet)
}

func sortedPaths(values map[string]bool) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}
