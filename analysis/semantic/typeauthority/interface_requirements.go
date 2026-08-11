package typeauthority

import (
	"github.com/wippyai/go-lua/analysis/type/subst"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

type interfaceRequirement struct {
	field    bool
	typ      typ.Type
	optional bool
	readonly bool
}

// addInterfaceRequirements proves that inherited materialized requirements
// have one unambiguous member contract per name. Intersections are otherwise a
// useful representation of conjunction, but using their may-access behavior to
// hide conflicting inherited fields or methods would change source semantics.
func addInterfaceRequirements(value typ.Type, requirements map[string]interfaceRequirement, active map[typ.Type]bool) bool {
	if value == nil || requirements == nil {
		return false
	}
	// Every work item is a finite sealed graph node. `active` deliberately
	// tracks the current structural path rather than all visited nodes: shared
	// inherited requirements must be checked at each occurrence, while a
	// recursive inheritance path stays an unsupported source form.
	type item struct {
		value typ.Type
		leave bool
	}
	work := []item{{value: value}}
	for len(work) != 0 {
		last := len(work) - 1
		current := work[last]
		work = work[:last]
		if current.leave {
			delete(active, current.value)
			continue
		}
		if current.value == nil || active[current.value] {
			return false
		}
		active[current.value] = true
		work = append(work, item{value: current.value, leave: true})
		switch value := current.value.(type) {
		case *typ.Alias:
			// Do not use UnaliasedTarget here: that helper owns a recursive alias
			// walk in typ. Projection descent remains under this package's finite
			// work stack.
			work = append(work, item{value: value.Target})
		case *typ.Instantiated:
			expanded, ok := subst.ExpandInstantiatedChanged(value)
			if !ok {
				return false
			}
			work = append(work, item{value: expanded})
		case *typ.Intersection:
			for index := len(value.Members) - 1; index >= 0; index-- {
				work = append(work, item{value: value.Members[index]})
			}
		case *typ.Record:
			// Interface syntax has neither map nor bracket-static requirement
			// forms. Accepting either through an extends alias would be a silent
			// vocabulary expansion, so leave it to the interface Rule.
			if value.HasMapComponent() || len(value.StaticMembers) != 0 {
				return false
			}
			for _, field := range value.Fields {
				if !addInterfaceRequirement(requirements, field.Name, interfaceRequirement{
					field: true, typ: field.Type, optional: field.Optional, readonly: field.Readonly,
				}) {
					return false
				}
			}
		case *typ.Interface:
			for _, method := range value.Methods {
				if method.Type == nil || !addInterfaceRequirement(requirements, method.Name, interfaceRequirement{typ: method.Type}) {
					return false
				}
			}
		default:
			// Recursive inheritance needs a dedicated coinductive interface law;
			// it is not safe to infer by terminating this finite conflict walk.
			return false
		}
	}
	return true
}

func addInterfaceRequirement(requirements map[string]interfaceRequirement, name string, requirement interfaceRequirement) bool {
	if name == "" || requirement.typ == nil || requirements == nil {
		return false
	}
	if prior, exists := requirements[name]; exists {
		return prior.field == requirement.field &&
			prior.optional == requirement.optional &&
			prior.readonly == requirement.readonly &&
			typ.TypeEquals(prior.typ, requirement.typ)
	}
	requirements[name] = requirement
	return true
}
