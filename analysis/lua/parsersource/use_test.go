package parsersource

import (
	"fmt"
	"strings"
	"testing"
)

// TestEverySlotRefinesADeclaredCarrier is the shape law of the slot grain. A
// slot is not a new coordinate: it is a carrier the declarations already state,
// read at the state it holds a child in. A slot naming a field no form declares,
// or a cardinality the field's own form does not admit, would be a coordinate
// invented here rather than one derived from the AST.
func TestEverySlotRefinesADeclaredCarrier(t *testing.T) {
	root := moduleRoot(t)
	schema, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	slots, err := UseSlots(schema)
	if err != nil {
		t.Fatal(err)
	}
	if len(slots) == 0 {
		t.Fatal("the parser-constructed forms declare no child carrier")
	}
	fields := make(map[string]Field, schema.FieldCount())
	for _, constructor := range schema.Constructors {
		for _, field := range constructor.Fields {
			fields[constructor.Name+"."+field.Name] = field
		}
	}
	for _, slot := range slots {
		field, declared := fields[slot.Form+"."+slot.Field]
		if !declared {
			t.Fatalf("slot %s.%s refines no declared carrier", slot.Form, slot.Field)
		}
		states := field.Form.States()
		if len(states) != 2 || slot.Cardinality != states[1] {
			t.Fatalf("slot %s.%s holds its child at %s, the carrier admits %v", slot.Form, slot.Field, slot.Cardinality, states)
		}
		if !strings.Contains(field.Type, slot.ChildType) {
			t.Fatalf("slot %s.%s accepts %s, the carrier is declared %s", slot.Form, slot.Field, slot.ChildType, field.Type)
		}
	}
}

// TestEveryConstructionIsYieldedOrConsumedAtASlot is the law the use grain
// exists for, and the exact dual of the product grain: a construction the
// parser performs is either handed back by the action that performs it or put
// into one typed slot of another construction that action performs. A
// construction that was neither would be a value the parser builds and nothing
// receives, which no reduction can do.
func TestEveryConstructionIsYieldedOrConsumedAtASlot(t *testing.T) {
	root := moduleRoot(t)
	analysis, err := DiscoverProducts(root)
	if err != nil {
		t.Fatal(err)
	}
	consumers := make(map[string][]string, len(analysis.Uses))
	for _, use := range analysis.Uses {
		for _, source := range use.Sources {
			key := fmt.Sprintf("%s#%d", use.Owner, source)
			consumers[key] = append(consumers[key], fmt.Sprintf("%s.%s", use.Form, use.Field))
		}
	}
	nested := 0
	for _, product := range analysis.Products {
		key := fmt.Sprintf("%s#%d", product.Owner, product.Ordinal)
		if product.Root {
			continue
		}
		nested++
		switch reached := consumers[key]; len(reached) {
		case 1:
		case 0:
			t.Fatalf("construction %s of %s is neither yielded nor consumed", key, product.Constructor)
		default:
			t.Fatalf("construction %s of %s is consumed at %v, an exact slot is one", key, product.Constructor, reached)
		}
	}
	if nested == 0 {
		t.Fatal("no action feeds one construction into another, so the law is vacuous")
	}
}

// TestEveryConsumptionEdgeNamesASlotAndAnOrigin states the two halves a use row
// must carry. The slot half makes the edge typed: an edge into a coordinate no
// slot states would be a consumption of something the declarations do not admit
// there. The origin half makes it an edge at all: a coordinate whose value the
// analysis cannot place has an opaque origin stated, never an absent one.
func TestEveryConsumptionEdgeNamesASlotAndAnOrigin(t *testing.T) {
	root := moduleRoot(t)
	schema, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	slots, err := UseSlots(schema)
	if err != nil {
		t.Fatal(err)
	}
	declared := make(map[string]UseSlot, len(slots))
	for _, slot := range slots {
		declared[slot.Form+"."+slot.Field] = slot
	}
	analysis, err := DiscoverProducts(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(analysis.Uses) == 0 {
		t.Fatal("the parser fills no slot")
	}
	products := make(map[string]bool, len(analysis.Products))
	for _, product := range analysis.Products {
		products[fmt.Sprintf("%s#%d:%s", product.Owner, product.Ordinal, product.Constructor)] = true
	}
	symbols := 0
	for _, use := range analysis.Uses {
		if _, stated := declared[use.Form+"."+use.Field]; !stated {
			t.Fatalf("use %s#%d fills %s.%s, which no slot states", use.Owner, use.Ordinal, use.Form, use.Field)
		}
		if !products[fmt.Sprintf("%s#%d:%s", use.Owner, use.Ordinal, use.Form)] {
			t.Fatalf("use %s#%d fills a coordinate of %s, which that action does not construct", use.Owner, use.Ordinal, use.Form)
		}
		if len(use.Origins) == 0 {
			t.Fatalf("use %s#%d:%s.%s states no origin", use.Owner, use.Ordinal, use.Form, use.Field)
		}
		for _, origin := range use.Origins {
			if origin == UseOriginInvalid {
				t.Fatalf("use %s#%d:%s.%s states an invalid origin", use.Owner, use.Ordinal, use.Form, use.Field)
			}
		}
		if len(use.Symbols) != 0 {
			symbols++
			if use.Scope != ProductScopeProduction {
				t.Fatalf("use %s#%d:%s.%s names a reduction operand outside a reduction", use.Owner, use.Ordinal, use.Form, use.Field)
			}
		}
	}
	if symbols == 0 {
		t.Fatal("no slot is filled from a reduction operand, so the operand half of an edge is untested")
	}
}

// TestStaticRoleFollowsTheCarriedMaterial states why the static role is decided
// structurally. A type-parameter list, a function parameter, an interface
// member, and a record field are all structural declarations the AST states
// without a marker, and each carries type expressions only, so each is static
// material wherever it appears. Deciding it from the parent alone would make
// the same list static under a type expression and an ordinary child under a
// statement, which is a property of the parent's spelling rather than of what
// the slot carries.
func TestStaticRoleFollowsTheCarriedMaterial(t *testing.T) {
	root := moduleRoot(t)
	schema, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	slots, err := UseSlots(schema)
	if err != nil {
		t.Fatal(err)
	}
	// A slot under a type expression is static because of its parent, so it
	// says nothing about the material the child carries. The claim is about the
	// remaining slots: there, staticness is decided by the child alone.
	byChild := make(map[string]map[UseRole][]string)
	for _, slot := range slots {
		if slot.Class == ConstructorTypeExpression {
			continue
		}
		if byChild[slot.ChildType] == nil {
			byChild[slot.ChildType] = make(map[UseRole][]string, 2)
		}
		byChild[slot.ChildType][slot.Role] = append(byChild[slot.ChildType][slot.Role], slot.Form+"."+slot.Field)
	}
	structural := 0
	for child, roles := range byChild {
		static, isStatic := roles[UseRoleStatic]
		if !isStatic {
			continue
		}
		if len(roles) != 1 {
			t.Fatalf("child %s is static in %v and not in %v", child, static, roles)
		}
		structural++
	}
	if structural == 0 {
		t.Fatal("no child type is static, so the law is vacuous")
	}
	if len(byChild["TypeParamExpr"][UseRoleStatic]) < 2 {
		t.Fatalf("a type-parameter list is static under %v, which does not exercise more than one parent", byChild["TypeParamExpr"][UseRoleStatic])
	}
}

// TestControlRoleFollowsBlockOwnership states the other structural role. A
// statement form that declares a statement carrier owns a block, so the values
// its other carriers hold are the operands of a control construct. Reading it
// from the form's name would state the same fact as a list that a new control
// form would not appear in.
func TestControlRoleFollowsBlockOwnership(t *testing.T) {
	root := moduleRoot(t)
	schema, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	slots, err := UseSlots(schema)
	if err != nil {
		t.Fatal(err)
	}
	blocks := make(map[string]bool)
	control := make(map[string]bool)
	for _, slot := range slots {
		if slot.Class == ConstructorStatement && slot.ChildClass == ConstructorStatement {
			blocks[slot.Form] = true
		}
		if slot.Role == UseRoleControl {
			control[slot.Form] = true
		}
	}
	if len(blocks) == 0 {
		t.Fatal("no statement form declares a statement carrier")
	}
	for form := range blocks {
		if !control[form] {
			t.Fatalf("statement form %s owns a block and no carrier of it is a control operand", form)
		}
	}
	for form := range control {
		if !blocks[form] {
			t.Fatalf("form %s carries a control operand and owns no block", form)
		}
	}
}
