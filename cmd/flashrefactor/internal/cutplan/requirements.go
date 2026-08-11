package cutplan

import (
	"fmt"
	"sort"
)

// ResolutionRequirements derives the one resolver denominator from reviewed
// intent. Consumers must use this view rather than re-implementing object
// roles or relocation path/package classification.
func ResolutionRequirements(intent Intent) ([]ResolutionRequirement, error) {
	if err := ValidateIntent(intent); err != nil {
		return nil, err
	}
	return resolutionRequirements(intent)
}

// ImpactObjects derives the exact declarations whose package surface can
// change during a cut. Containment is structural evidence, not an independent
// changed declaration, so it is deliberately absent here.
func ImpactObjects(intent Intent) ([]SymbolRef, error) {
	if err := ValidateIntent(intent); err != nil {
		return nil, err
	}
	seen := map[string]SymbolRef{}
	for _, operation := range intent.Operations {
		for _, edit := range operation.Edits {
			switch edit.Kind {
			case EditRelocate:
				for _, subject := range edit.Relocate.Subjects {
					seen[subject.From.Object] = subject.From
					seen[subject.To.Object] = subject.To
				}
			case EditRetire:
				for _, object := range edit.Retire.Symbols {
					seen[object.Object] = object
				}
			}
		}
	}
	result := make([]SymbolRef, 0, len(seen))
	for _, object := range seen {
		result = append(result, object)
	}
	sort.Slice(result, func(left, right int) bool { return result[left].Object < result[right].Object })
	return result, nil
}

// ReferenceRouteRequirements derives the complete route denominator from the
// reviewed relocation subjects. It deliberately has no independent authored
// route list: resolver classification remains the one authority for both
// endpoints.
func ReferenceRouteRequirements(intent Intent) ([]ReferenceRoute, error) {
	requirements, err := ResolutionRequirements(intent)
	if err != nil {
		return nil, err
	}
	roles := make(map[string]ObjectRole, len(requirements))
	for _, requirement := range requirements {
		roles[requirement.Object.Object] = requirement.Role
	}
	seen := map[string]bool{}
	result := make([]ReferenceRoute, 0)
	for _, operation := range intent.Operations {
		for _, edit := range operation.Edits {
			if edit.Kind != EditRelocate {
				continue
			}
			for _, subject := range edit.Relocate.Subjects {
				if roles[subject.From.Object] != ObjectSource || roles[subject.To.Object] != ObjectTarget {
					return nil, fmt.Errorf("relocation route has invalid resolver classification: %s -> %s", subject.From.Object, subject.To.Object)
				}
				key := routeKey(subject.From, subject.To)
				if seen[key] {
					return nil, fmt.Errorf("duplicate relocation route requirement: %s", key)
				}
				seen[key] = true
				result = append(result, ReferenceRoute{From: subject.From, To: subject.To})
			}
		}
	}
	sort.Slice(result, func(left, right int) bool {
		return routeKey(result[left].From, result[left].To) < routeKey(result[right].From, result[right].To)
	})
	return result, nil
}

// GateRequirements returns the exact set-union of requested gate kinds.
// Gate evidence is generated once per kind, even when several operations
// require it, so a lock cannot hide a missing operation behind duplicate rows.
func GateRequirements(intent Intent) ([]Gate, error) {
	if err := ValidateIntent(intent); err != nil {
		return nil, err
	}
	seen := map[Gate]bool{}
	for _, operation := range intent.Operations {
		for _, gate := range operation.Verify.Gates {
			seen[gate] = true
		}
	}
	result := make([]Gate, 0, len(seen))
	for gate := range seen {
		result = append(result, gate)
	}
	sort.Slice(result, func(left, right int) bool { return result[left] < result[right] })
	return result, nil
}

func resolutionRequirements(intent Intent) ([]ResolutionRequirement, error) {
	roles, err := intentObjectRoles(intent)
	if err != nil {
		return nil, err
	}
	targets, err := relocationTargets(intent)
	if err != nil {
		return nil, err
	}
	sources, err := sourceObjectPaths(intent)
	if err != nil {
		return nil, err
	}
	locations := map[string]forcedLocation{}
	for object, source := range sources {
		if err := addLocation(locations, object, forcedLocation{Path: source}); err != nil {
			return nil, err
		}
	}
	for object, target := range targets {
		if err := addLocation(locations, object, forcedLocation{Path: target.Path, Package: target.Package}); err != nil {
			return nil, err
		}
	}
	for _, operation := range intent.Operations {
		for _, edit := range operation.Edits {
			if edit.Kind != EditRelocate || edit.Relocate.Containment == nil {
				continue
			}
			containment := edit.Relocate.Containment
			if err := addLocation(locations, containment.Parent.Object, forcedLocation{Path: edit.Relocate.Source}); err != nil {
				return nil, err
			}
			if err := addLocation(locations, containment.Child.Object, forcedLocation{Path: edit.Relocate.Destination.Path, Package: edit.Relocate.Destination.Package}); err != nil {
				return nil, err
			}
			if err := addLocation(locations, containment.Through.Object, forcedLocation{Path: edit.Relocate.Source}); err != nil {
				return nil, err
			}
		}
	}
	result := make([]ResolutionRequirement, 0, len(roles))
	for object, role := range roles {
		requirement := ResolutionRequirement{Object: SymbolRef{Object: object}, Role: role}
		if location, exists := locations[object]; exists {
			requirement.Path = location.Path
			requirement.Package = location.Package
		}
		result = append(result, requirement)
	}
	sort.Slice(result, func(left, right int) bool {
		return result[left].Object.Object < result[right].Object.Object
	})
	return result, nil
}

type forcedLocation struct {
	Path    string
	Package string
}

func addLocation(locations map[string]forcedLocation, object string, location forcedLocation) error {
	if prior, exists := locations[object]; exists && prior != location {
		return fmt.Errorf("resolution location collision for %s", object)
	}
	locations[object] = location
	return nil
}

func intentObjectRoles(intent Intent) (map[string]ObjectRole, error) {
	result := map[string]ObjectRole{}
	add := func(value SymbolRef, role ObjectRole) error {
		if existing, found := result[value.Object]; found && existing != role {
			return fmt.Errorf("object cannot be both %s and %s in one cut: %s", existing, role, value.Object)
		}
		result[value.Object] = role
		return nil
	}
	for _, operation := range intent.Operations {
		for _, edit := range operation.Edits {
			switch edit.Kind {
			case EditRelocate:
				for _, subject := range edit.Relocate.Subjects {
					if err := add(subject.From, ObjectSource); err != nil {
						return nil, err
					}
					if err := add(subject.To, ObjectTarget); err != nil {
						return nil, err
					}
				}
				if edit.Relocate.Containment != nil {
					if err := add(edit.Relocate.Containment.Parent, ObjectSource); err != nil {
						return nil, err
					}
					if err := add(edit.Relocate.Containment.Child, ObjectTarget); err != nil {
						return nil, err
					}
					if err := add(edit.Relocate.Containment.Through, ObjectTarget); err != nil {
						return nil, err
					}
				}
			case EditRetire:
				for _, object := range edit.Retire.Symbols {
					if err := add(object, ObjectSource); err != nil {
						return nil, err
					}
				}
			}
		}
		for _, binding := range operation.Bindings {
			if err := add(binding.From, ObjectSource); err != nil {
				return nil, err
			}
			if err := add(binding.To, ObjectTarget); err != nil {
				return nil, err
			}
			for _, step := range binding.Receiver {
				if err := add(step.Object, ObjectTarget); err != nil {
					return nil, err
				}
			}
		}
		for _, route := range operation.Imports {
			for _, object := range route.Symbols {
				if err := add(object, ObjectTarget); err != nil {
					return nil, err
				}
			}
		}
	}
	return result, nil
}

func relocationTargets(intent Intent) (map[string]Destination, error) {
	sources := map[string]bool{}
	targets := map[string]Destination{}
	for _, operation := range intent.Operations {
		for _, edit := range operation.Edits {
			if edit.Kind != EditRelocate {
				continue
			}
			for _, subject := range edit.Relocate.Subjects {
				if sources[subject.From.Object] {
					return nil, fmt.Errorf("relocation source is mapped more than once: %s", subject.From.Object)
				}
				if _, exists := targets[subject.To.Object]; exists {
					return nil, fmt.Errorf("relocation target is mapped more than once: %s", subject.To.Object)
				}
				sources[subject.From.Object] = true
				targets[subject.To.Object] = edit.Relocate.Destination
			}
		}
	}
	return targets, nil
}

func sourceObjectPaths(intent Intent) (map[string]string, error) {
	result := map[string]string{}
	add := func(object SymbolRef, path string) error {
		if _, exists := result[object.Object]; exists {
			return fmt.Errorf("source declaration is selected more than once: %s", object.Object)
		}
		result[object.Object] = path
		return nil
	}
	for _, operation := range intent.Operations {
		for _, edit := range operation.Edits {
			switch edit.Kind {
			case EditRelocate:
				for _, subject := range edit.Relocate.Subjects {
					if err := add(subject.From, edit.Relocate.Source); err != nil {
						return nil, err
					}
				}
			case EditRetire:
				for _, object := range edit.Retire.Symbols {
					if err := add(object, edit.Retire.Source); err != nil {
						return nil, err
					}
				}
			}
		}
	}
	return result, nil
}
