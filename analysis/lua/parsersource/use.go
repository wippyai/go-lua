package parsersource

import (
	"fmt"
	"sort"
	"strings"
)

// UseRole is the role a parent form gives every value one of its carriers
// holds. It is derived from the declarations alone: whether the parent is
// itself static material, whether the child carrier is static material, and
// whether the parent owns a statement block. A role a declaration cannot decide
// is not stated here, so the vocabulary is exactly the three the AST decides.
type UseRole uint8

const (
	UseRoleInvalid UseRole = iota
	// UseRoleStatic: the slot carries type-level material, either because the
	// parent is a type expression or because the child carrier only ever holds
	// type expressions.
	UseRoleStatic
	// UseRoleControl: the parent is a statement form that owns a statement
	// block, so every carrier of it is part of a control construct.
	UseRoleControl
	// UseRoleChild: the ordinary value or statement child of a form.
	UseRoleChild
)

// String is the stable spelling a row key uses.
func (r UseRole) String() string {
	switch r {
	case UseRoleStatic:
		return "static"
	case UseRoleControl:
		return "control"
	case UseRoleChild:
		return "child"
	default:
		return "invalid"
	}
}

// UseSlot is one typed parent slot: the exact coordinate at which a form
// carries another AST value. Cardinality is the state the carrier is in once it
// holds a child, which is the state a consumption law is stated at: an absent
// optional child and an empty sequence carry nothing, so they are the zero side
// of the same carrier rather than a second slot.
type UseSlot struct {
	Form  string
	Field string
	// Class is the parent form's own declared class.
	Class ConstructorClass
	Role  UseRole
	// ChildType is the declared type of the value the slot carries, with the
	// sequence and pointer spelling removed: the slot's cardinality already
	// states which of the two it was.
	ChildType string
	// ChildClass is the child type's own declared class. A structural child is
	// one the AST declares without a marker, so it reaches the analyzer only
	// inside the form declaring it.
	ChildClass  ConstructorClass
	Cardinality FieldState
}

// UseOrigin names where the value one consumption edge delivers enters the
// action that performs it. It is the provenance side of the edge: the slot says
// what the parent accepts, the origin says what the action put there.
type UseOrigin uint8

const (
	UseOriginInvalid UseOrigin = iota
	// UseOriginSymbol: a positional operand of the reduction.
	UseOriginSymbol
	// UseOriginParameter: a formal of the parser helper performing the edge.
	UseOriginParameter
	// UseOriginConstruction: a value the same action constructs.
	UseOriginConstruction
	// UseOriginHelper: the result of a parser helper call.
	UseOriginHelper
	// UseOriginAssembly: a sequence the action itself builds, whether by
	// literal, append, or allocation.
	UseOriginAssembly
	// UseOriginConstant: a declared constant or a literal.
	UseOriginConstant
	// UseOriginOpaque: an origin the analysis cannot name. It is stated rather
	// than dropped, because an edge whose provenance is unknown is still an
	// edge the parser performs.
	UseOriginOpaque
)

// String is the stable spelling a row key uses.
func (o UseOrigin) String() string {
	switch o {
	case UseOriginSymbol:
		return "symbol"
	case UseOriginParameter:
		return "parameter"
	case UseOriginConstruction:
		return "construction"
	case UseOriginHelper:
		return "helper"
	case UseOriginAssembly:
		return "assembly"
	case UseOriginConstant:
		return "constant"
	case UseOriginOpaque:
		return "opaque"
	default:
		return "invalid"
	}
}

// ActionUse is one consumption edge: the coordinate of one construction that
// receives another AST value, and where that value came from. It is the dual of
// ActionProduct at the same grain - a product row states what one construction
// builds, a use row states where each built value lands - so Owner, Scope and
// Ordinal address the same construction in both.
//
// Sources are the ordinals of constructions the same action performs which can
// reach this coordinate. A construction whose value leaves through a grammar
// symbol is not named here: it belongs to the action that built it, and the
// coordinate only records that a symbol delivered it.
//
// Symbols are the positional operands of the reduction that can reach the
// coordinate. Naming them is what makes the row an edge rather than a fact
// about a field: two reductions that fill the same slot from different operands
// consume different things, and a row that stated only the origin kind could
// not tell them apart.
type ActionUse struct {
	Owner   string
	Scope   ProductScope
	Ordinal int
	Form    string
	Field   string
	Origins []UseOrigin
	Sources []int
	Symbols []int
}

// UseSlots derives every typed parent slot the parser-constructed forms
// declare. A carrier is a slot exactly when its declared type is a semantic AST
// type and its form admits a two-state space, so a source coordinate, a
// discriminant, and a lexeme are excluded by what they are rather than by name.
func UseSlots(schema Schema) ([]UseSlot, error) {
	semantic := make(map[string]bool, len(schema.Types))
	classes := make(map[string]ConstructorClass, len(schema.Types))
	declared := make(map[string]bool, len(schema.Types))
	for _, declaration := range schema.Types {
		semantic[declaration.Name] = declaration.Semantic
		classes[declaration.Name] = declaration.Class
		declared[declaration.Name] = true
	}
	if len(semantic) == 0 {
		return nil, fmt.Errorf("parser uses: the AST type graph states no named type")
	}
	// A child interface carries no embedding of its own, so its class is read
	// through the base struct that implements it. The AST spells that base with
	// the interface's own name and the Base suffix, and which class a base gives
	// is decided in one place, so this reaches the same authority a struct does.
	for _, declaration := range schema.Types {
		if declaration.Kind != NamedTypeInterface || !declared[declaration.Name+"Base"] {
			continue
		}
		if class := EmbeddedBaseClass(declaration.Name + "Base"); class != ConstructorStructural {
			classes[declaration.Name] = class
		}
	}
	children := make(map[string][]UseSlot, len(schema.Constructors))
	for _, constructor := range schema.Constructors {
		for _, field := range constructor.Fields {
			child, cardinality, ok := childCarrier(field, semantic)
			if !ok {
				continue
			}
			children[constructor.Name] = append(children[constructor.Name], UseSlot{
				Form:        constructor.Name,
				Field:       field.Name,
				Class:       constructor.Class,
				ChildType:   child,
				ChildClass:  classes[child],
				Cardinality: cardinality,
			})
		}
	}
	static := staticCarriers(children, classes)
	blocks := blockOwners(children, classes)
	result := make([]UseSlot, 0, len(children))
	for form, slots := range children {
		for _, slot := range slots {
			slot.Role = slotRole(slot, static, blocks[form])
			result = append(result, slot)
		}
	}
	sort.Slice(result, func(left, right int) bool {
		if result[left].Form != result[right].Form {
			return result[left].Form < result[right].Form
		}
		return result[left].Field < result[right].Field
	})
	for _, slot := range result {
		if slot.Role == UseRoleInvalid || slot.ChildType == "" || slot.Cardinality == FieldStateInvalid {
			return nil, fmt.Errorf("parser uses: incomplete slot %s.%s", slot.Form, slot.Field)
		}
	}
	return result, nil
}

// childCarrier decides whether one declared field carries another AST value.
// The declared type decides it: a field whose base type is a semantic AST type
// carries one, and the field's own form decides whether the carrier holds one
// child or a sequence of them.
func childCarrier(field Field, semantic map[string]bool) (string, FieldState, bool) {
	base := strings.TrimPrefix(field.Type, "[]")
	base = strings.TrimPrefix(base, "*")
	base = strings.TrimPrefix(base, "ast.")
	if !semantic[base] {
		return "", FieldStateInvalid, false
	}
	states := field.Form.States()
	if len(states) != 2 {
		return "", FieldStateInvalid, false
	}
	return base, states[1], true
}

// staticCarriers is the least set of child types that carry type-level material
// only. A type expression is one by its own declared class; a structural form
// is one when every child it declares is one, which is how a parameter, an
// interface member, a record field, and a type parameter reach the same answer
// without any of them being named.
func staticCarriers(children map[string][]UseSlot, classes map[string]ConstructorClass) map[string]bool {
	result := make(map[string]bool, len(classes))
	for name, class := range classes {
		if class == ConstructorTypeExpression {
			result[name] = true
		}
	}
	for changed := true; changed; {
		changed = false
		for form, slots := range children {
			if result[form] || classes[form] != ConstructorStructural || len(slots) == 0 {
				continue
			}
			static := true
			for _, slot := range slots {
				if !result[slot.ChildType] {
					static = false
					break
				}
			}
			if static {
				result[form], changed = true, true
			}
		}
	}
	return result
}

// blockOwners are the statement forms that declare a statement carrier. Such a
// form owns a block, so the values its other carriers hold are the operands of
// a control construct rather than ordinary children.
func blockOwners(children map[string][]UseSlot, classes map[string]ConstructorClass) map[string]bool {
	result := make(map[string]bool, len(children))
	for form, slots := range children {
		if classes[form] != ConstructorStatement {
			continue
		}
		for _, slot := range slots {
			if classes[slot.ChildType] == ConstructorStatement {
				result[form] = true
				break
			}
		}
	}
	return result
}

func slotRole(slot UseSlot, static map[string]bool, block bool) UseRole {
	if slot.Class == ConstructorTypeExpression || static[slot.ChildType] {
		return UseRoleStatic
	}
	if block {
		return UseRoleControl
	}
	return UseRoleChild
}
