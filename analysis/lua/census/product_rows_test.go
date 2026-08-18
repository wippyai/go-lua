package census

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/parsersource"
)

// TestDiscriminantRowsRefineTheProductGrain states the shape of the fidelity
// column on the projected rows. A member row is keyed on the carrier it refines,
// so a reader holding one reaches the carrier and the field-state rows beside it
// without a second index, and a construction that names a member for a carrier
// its form does not declare would be addressing nothing.
func TestDiscriminantRowsRefineTheProductGrain(t *testing.T) {
	root := moduleRoot(t)
	value, err := Current(root)
	if err != nil {
		t.Fatal(err)
	}
	carriers := make(map[string]bool)
	for _, row := range Project(value).Rows {
		if row.Kind == RowCarrier {
			carriers[row.Key] = true
		}
	}
	stated := 0
	for _, row := range Project(value).Products {
		for _, member := range row.Discriminants {
			if !strings.HasPrefix(member, "member:") {
				t.Fatalf("product row %s states discriminant %s outside the member grain", row.Key, member)
			}
			coordinate := strings.TrimPrefix(member, "member:")
			at := strings.LastIndex(coordinate, "@")
			if at <= 0 || at == len(coordinate)-1 {
				t.Fatalf("product row %s states discriminant %s with no member", row.Key, member)
			}
			if !carriers[CarrierRow(carrierForm(coordinate[:at]), carrierField(coordinate[:at]))] {
				t.Fatalf("product row %s refines %s, which no form declares", row.Key, coordinate[:at])
			}
			stated++
		}
	}
	if stated == 0 {
		t.Fatal("no product row states a discriminant, so the fidelity column is unpublished")
	}
}

// TestDiscriminantColumnIsExactlyTheClosedFamilyCarriers states the admission
// rule at the census grain: the column appears on the carriers a closed
// compiler/ast constant family discriminates and on no others. A column that
// reached further would be pricing a state space the declarations do not close,
// and one that reached less would leave a decidable choice unstated.
func TestDiscriminantColumnIsExactlyTheClosedFamilyCarriers(t *testing.T) {
	root := moduleRoot(t)
	value, err := Current(root)
	if err != nil {
		t.Fatal(err)
	}
	constants, err := parsersource.DiscoverConstants(root)
	if err != nil {
		t.Fatal(err)
	}
	families := make(map[string]parsersource.DiscriminantEnum)
	for _, family := range parsersource.DiscriminantEnums(constants) {
		families[family.Type] = family
	}
	declared := make(map[string]string)
	for _, constructor := range value.Constructors {
		for _, field := range constructor.Fields {
			declared[constructor.Name+"."+field.Name] = field.Type
		}
	}
	refined := make(map[string]bool)
	for _, product := range value.Products {
		for _, coordinate := range product.Fields {
			carrier := product.Constructor + "." + coordinate.Field
			family, closed := families[declared[carrier]]
			if coordinate.Member == "" {
				continue
			}
			if !closed {
				t.Fatalf("%s names member %s and its declared type %s is no closed family", carrier, coordinate.Member, declared[carrier])
			}
			if !containsMember(family.Members, coordinate.Member) {
				t.Fatalf("%s names member %s, family %s declares %v", carrier, coordinate.Member, family.Type, family.Members)
			}
			refined[carrier] = true
		}
	}
	var names []string
	for carrier := range refined {
		names = append(names, carrier)
	}
	sort.Strings(names)
	want := []string{"AttrGetExpr.KeySyntax", "CastExpr.Syntax", "Field.KeySyntax", "InterfaceMember.Kind"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("the discriminant column reaches %v, the closed families declare carriers %v", names, want)
	}
}

// TestDriftGuardRejectsARelabelledDiscriminant is the drift guard of the
// fidelity column, and it is the one edit the census could not previously see.
// Rewriting a named table field as a bracket one changes which member of one
// closed family a reduction assigns and nothing else the older grains state:
// both members are non-zero, so every carrier state vector in the census is
// identical before and after, and every slot, consumption edge, mutation and
// list disposition is untouched. A census that abstracted the family to
// zero-ness would keep describing a parser that builds a.b where it now builds
// a[b].
func TestDriftGuardRejectsARelabelledDiscriminant(t *testing.T) {
	root := moduleRoot(t)
	copied := copyParserSources(t, root)
	if err := Generated.Validate(copied); err != nil {
		t.Fatalf("census rejected an unmodified copy of the parser sources: %v", err)
	}
	grammarPath := filepath.Join(copied, "compiler", "parse", "parser.go.y")
	contents, err := os.ReadFile(grammarPath)
	if err != nil {
		t.Fatal(err)
	}
	const original = "&ast.Field{Key: key, KeySyntax: ast.AttrKeyDot, Value: $3}"
	const edited = "&ast.Field{Key: key, KeySyntax: ast.AttrKeyIndex, Value: $3}"
	if strings.Count(string(contents), original) != 1 {
		t.Fatalf("parser.go.y does not state %q exactly once", original)
	}
	mutated := strings.Replace(string(contents), original, edited, 1)
	if err := os.WriteFile(grammarPath, []byte(mutated), 0o644); err != nil {
		t.Fatal(err)
	}
	edit, err := Build(copied)
	if err != nil {
		t.Fatal(err)
	}
	current, err := Current(root)
	if err != nil {
		t.Fatal(err)
	}
	// The grains that do not see the edit are stated first, so the rejection
	// below is attributable to the discriminant column alone.
	if !sameProducts(current, edit) {
		t.Fatal("relabelling a discriminant changed a carrier state vector, so this edit does not isolate the fidelity column")
	}
	if !reflect.DeepEqual(current.Slots, edit.Slots) || !reflect.DeepEqual(current.Uses, edit.Uses) || !reflect.DeepEqual(current.Mutations, edit.Mutations) {
		t.Fatal("relabelling a discriminant changed the slot, consumption or mutation grain, so this edit does not isolate the fidelity column")
	}
	if !reflect.DeepEqual(current.Sequences, edit.Sequences) {
		t.Fatal("relabelling a discriminant changed the list grain, so this edit does not isolate the fidelity column")
	}
	before := memberOf(t, current, "field#1", 1, "KeySyntax")
	after := memberOf(t, edit, "field#1", 1, "KeySyntax")
	if before != "AttrKeyDot" {
		t.Fatalf("field#1 assigns key syntax %q, want AttrKeyDot", before)
	}
	if after != "AttrKeyIndex" {
		t.Fatalf("the relabelled field#1 assigns key syntax %q, want AttrKeyIndex", after)
	}
	if err := Generated.Validate(copied); err == nil {
		t.Fatal("census accepted a discriminant assignment it was not generated from")
	}
}

func memberOf(t *testing.T, value Census, owner string, ordinal int, field string) string {
	t.Helper()
	for _, product := range value.Products {
		if product.Owner != owner || product.Ordinal != ordinal {
			continue
		}
		for _, coordinate := range product.Fields {
			if coordinate.Field == field {
				return coordinate.Member
			}
		}
	}
	t.Fatalf("the census states no %s coordinate for %s#%d", field, owner, ordinal)
	return ""
}

func containsMember(members []string, name string) bool {
	for _, member := range members {
		if member == name {
			return true
		}
	}
	return false
}

func carrierForm(coordinate string) string {
	if index := strings.Index(coordinate, "."); index > 0 {
		return coordinate[:index]
	}
	return coordinate
}

func carrierField(coordinate string) string {
	if index := strings.Index(coordinate, "."); index > 0 {
		return coordinate[index+1:]
	}
	return ""
}
