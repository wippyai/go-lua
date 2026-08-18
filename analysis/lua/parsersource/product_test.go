package parsersource

import (
	"fmt"
	"testing"
)

// TestEveryConstructedFormStatesACompleteFieldVector is the shape law of the
// product grain: a construction is not a set of the coordinates it happened to
// name, it is the whole constructor. A vector that stated only the named
// coordinates could not distinguish a form whose carrier is always filled from
// one whose carrier is merely filled here, which is the whole judgement the
// grain exists to support.
func TestEveryConstructedFormStatesACompleteFieldVector(t *testing.T) {
	root := moduleRoot(t)
	analysis, err := DiscoverProducts(root)
	if err != nil {
		t.Fatal(err)
	}
	schema, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	declared := make(map[string][]Field, len(schema.Constructors))
	for _, constructor := range schema.Constructors {
		declared[constructor.Name] = constructor.Fields
	}
	if len(analysis.Products) == 0 {
		t.Fatal("the parser constructs no semantic AST values")
	}
	built := make(map[string]bool, len(declared))
	for _, product := range analysis.Products {
		fields, known := declared[product.Constructor]
		if !known {
			t.Fatalf("product %s#%d builds %s, which no AST declaration states", product.Owner, product.Ordinal, product.Constructor)
		}
		built[product.Constructor] = true
		if len(product.Fields) != len(fields) {
			t.Fatalf("product %s#%d states %d coordinates of %s, which declares %d", product.Owner, product.Ordinal, len(product.Fields), product.Constructor, len(fields))
		}
		for index, coordinate := range product.Fields {
			if coordinate.Field != fields[index].Name || coordinate.Ordinal != fields[index].Ordinal {
				t.Fatalf("product %s#%d states coordinate %d as %s, %s declares %s", product.Owner, product.Ordinal, index, coordinate.Field, product.Constructor, fields[index].Name)
			}
		}
	}
	for _, constructor := range schema.Constructors {
		if constructor.Semantic && !built[constructor.Name] {
			t.Fatalf("semantic form %s is constructed by no product row", constructor.Name)
		}
	}
}

// TestUnassignedCoordinatesHoldExactlyTheDeclaredZeroState states why an
// omitted coordinate is evidence. A construction that does not name a carrier
// leaves it in the zero state its declared form has, and nothing else: reading
// such a coordinate as unknown would make every construction admit every state
// and the grain would decide nothing.
func TestUnassignedCoordinatesHoldExactlyTheDeclaredZeroState(t *testing.T) {
	root := moduleRoot(t)
	analysis, err := DiscoverProducts(root)
	if err != nil {
		t.Fatal(err)
	}
	schema, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	forms := make(map[string]map[string]FieldForm, len(schema.Constructors))
	for _, constructor := range schema.Constructors {
		forms[constructor.Name] = make(map[string]FieldForm, len(constructor.Fields))
		for _, field := range constructor.Fields {
			forms[constructor.Name][field.Name] = field.Form
		}
	}
	omitted := 0
	for _, product := range analysis.Products {
		for _, coordinate := range product.Fields {
			if coordinate.Assigned {
				continue
			}
			states := forms[product.Constructor][coordinate.Field].States()
			if len(states) == 0 {
				if len(coordinate.States) != 0 {
					t.Fatalf("%s.%s has no modelled state space but the product states %v", product.Constructor, coordinate.Field, coordinate.States)
				}
				continue
			}
			omitted++
			if len(coordinate.States) != 1 || coordinate.States[0] != states[0] {
				t.Fatalf("omitted coordinate %s.%s of %s#%d holds %v, want only %s", product.Constructor, coordinate.Field, product.Owner, product.Ordinal, coordinate.States, states[0])
			}
		}
	}
	if omitted == 0 {
		t.Fatal("no construction omits a coordinate, so the law is vacuous")
	}
}

// TestLexemeContractDecidesStringCarrierEmptiness states the fact the parser
// alone cannot decide. Two carriers are filled from token text the same way,
// and they differ only in which terminal the text came from: a string literal's
// text is whatever stood between its quotes and can be nothing, while an
// identifier's text is anchored on the character that started it. A derivation
// that read the action and not the scanner would have to give both carriers the
// same answer.
func TestLexemeContractDecidesStringCarrierEmptiness(t *testing.T) {
	root := moduleRoot(t)
	analysis, err := DiscoverProducts(root)
	if err != nil {
		t.Fatal(err)
	}
	reach := make(map[string]map[FieldState]bool)
	for _, product := range analysis.Products {
		for _, coordinate := range product.Fields {
			key := product.Constructor + "." + coordinate.Field
			if reach[key] == nil {
				reach[key] = make(map[FieldState]bool, 2)
			}
			for _, state := range coordinate.States {
				reach[key][state] = true
			}
		}
	}
	if !reach["StringExpr.Value"][FieldStateEmpty] {
		t.Fatal("a string literal's value cannot be empty, but an empty literal is source the parser accepts")
	}
	if reach["IdentExpr.Value"][FieldStateEmpty] {
		t.Fatal("an identifier's value can be empty, but the scanner anchors an identifier on its first character")
	}
	if !reach["IdentExpr.Value"][FieldStateNonEmpty] {
		t.Fatal("an identifier's value cannot be non-empty")
	}
}

func TestDiscriminantFidelityNamesTheMemberEachConstructionAssigns(t *testing.T) {
	root := moduleRoot(t)
	analysis, err := DiscoverProducts(root)
	if err != nil {
		t.Fatal(err)
	}
	constants, err := DiscoverConstants(root)
	if err != nil {
		t.Fatal(err)
	}
	families := make(map[string]DiscriminantEnum)
	memberFamily := make(map[string]string)
	for _, family := range DiscriminantEnums(constants) {
		families[family.Type] = family
		for _, member := range family.Members {
			memberFamily[member] = family.Type
		}
	}
	schema, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	declared := make(map[string]Field)
	for _, constructor := range schema.Constructors {
		for _, field := range constructor.Fields {
			declared[constructor.Name+"."+field.Name] = field
		}
	}
	named := 0
	for _, product := range analysis.Products {
		for _, coordinate := range product.Fields {
			field, known := declared[product.Constructor+"."+coordinate.Field]
			if !known {
				continue
			}
			family, admitted := families[field.Type]
			if coordinate.Member == "" {
				if admitted && !coordinate.Assigned && family.Zero != "" {
					t.Fatalf("%s#%d:%s.%s omits the coordinate and names no member, want %s", product.Owner, product.Ordinal, product.Constructor, coordinate.Field, family.Zero)
				}
				continue
			}
			named++
			if !admitted || memberFamily[coordinate.Member] != family.Type {
				t.Fatalf("%s#%d:%s.%s names member %s outside declared family %s", product.Owner, product.Ordinal, product.Constructor, coordinate.Field, coordinate.Member, family.Type)
			}
		}
	}
	if named == 0 {
		t.Fatal("no construction names a family member")
	}
	syntax := keySyntaxMembers(analysis)
	want := map[string]string{
		"field#1#1:Field":           "AttrKeyDot",
		"field#2#1:Field":           "AttrKeyIndex",
		"field#3#1:Field":           "AttrKeyUnknown",
		"var#2#1:AttrGetExpr":       "AttrKeyIndex",
		"var#3#1:AttrGetExpr":       "AttrKeyDot",
		"funcname1#2#2:AttrGetExpr": "AttrKeyDot",
	}
	for site, member := range want {
		if syntax[site] != member {
			t.Fatalf("%s assigns key syntax %q, want %q", site, syntax[site], member)
		}
	}
	if syntax["field#1#1:Field"] == syntax["field#2#1:Field"] {
		t.Fatal("named and bracket table-field constructions name the same member")
	}
}

func TestDiscriminantFidelityDoesNotWidenTheStateSpace(t *testing.T) {
	root := moduleRoot(t)
	analysis, err := DiscoverProducts(root)
	if err != nil {
		t.Fatal(err)
	}
	constants, err := DiscoverConstants(root)
	if err != nil {
		t.Fatal(err)
	}
	zeroMember := make(map[string]bool)
	for _, family := range DiscriminantEnums(constants) {
		if family.Zero != "" {
			zeroMember[family.Zero] = true
		}
	}
	agreed := 0
	for _, product := range analysis.Products {
		for _, coordinate := range product.Fields {
			where := fmt.Sprintf("%s#%d:%s.%s", product.Owner, product.Ordinal, product.Constructor, coordinate.Field)
			if len(coordinate.States) > 2 {
				t.Fatalf("%s holds %d states", where, len(coordinate.States))
			}
			if coordinate.Member == "" {
				continue
			}
			if len(coordinate.States) != 1 || (coordinate.States[0] == FieldStateZero) != zeroMember[coordinate.Member] {
				t.Fatalf("%s names member %s and holds states %v", where, coordinate.Member, coordinate.States)
			}
			agreed++
		}
	}
	if agreed == 0 {
		t.Fatal("no coordinate names a member")
	}
}

func keySyntaxMembers(analysis ProductAnalysis) map[string]string {
	result := make(map[string]string)
	for _, product := range analysis.Products {
		for _, coordinate := range product.Fields {
			if coordinate.Field == "KeySyntax" {
				result[fmt.Sprintf("%s#%d:%s", product.Owner, product.Ordinal, product.Constructor)] = coordinate.Member
			}
		}
	}
	return result
}
