package frozen

import (
	"fmt"
	"sort"
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/census"
	"github.com/wippyai/go-lua/analysis/lua/internal/grammarproof/requirements/parserproducts"
	"github.com/wippyai/go-lua/analysis/lua/internal/grammarproof/requirements/parseruses"
	"github.com/wippyai/go-lua/analysis/lua/parsersource"
)

// The frozen parser-use evidence proves that every value the parser builds is
// consumed at an exact typed parent slot. These receipts state that the census
// reaches the same slots and the same consumption edges from the parser and AST
// source alone, so the frozen vectors are a second reading of one derivation.
//
// Every disagreement is named below and excluded from both sides by coordinate,
// never by filtering the comparison until it closes.

// slotDifferences are the parent coordinates where the two denominators
// genuinely differ. Each is a coordinate, so naming one excludes it from both
// sides and the rest of the receipt stays exact.
var slotDifferences = map[string]string{
	// The frozen denominator lifts a bool coordinate into the carrier universe
	// by name and calls it an adjustment carrier. It carries no child, so the
	// derived denominator, which admits a slot exactly when the declared field
	// holds another AST value, does not state one.
	"Comma3Expr.AdjustRet":   "frozen-only: a bool coordinate lifted into the carrier universe",
	"FuncCallExpr.AdjustRet": "frozen-only: a bool coordinate lifted into the carrier universe",

	// Write eligibility is not a parser fact. The grammar restricts which forms
	// may stand left of an assignment, but nothing in the declarations or the
	// actions says the value is written rather than read, so the derivation
	// states the slot and leaves that judgment to the owner that can make it.
	"AssignStmt.Lhs": "role differs: the frozen evidence calls this slot an lvalue, the derivation states the slot only",

	// The frozen evidence decides staticness from a list of three child type
	// names and omits TypeParamExpr, so it calls a type-parameter list static
	// under a type expression and an ordinary child everywhere else. The
	// derivation decides it structurally - a carrier is static when every child
	// it can hold is type-level material - so a type-parameter list is static
	// under every parent.
	"FunctionExpr.TypeParams": "role differs: the frozen static child-type list omits TypeParamExpr",
	"TypeDefStmt.TypeParams":  "role differs: the frozen static child-type list omits TypeParamExpr",

	// A child declared at its concrete form is a child edge by structure. The
	// frozen carrier admission enumerates abstract child type names, so these
	// six real parent-to-child edges have no slot and nothing proves that what
	// the parser builds for them is consumed anywhere.
	"FuncDefStmt.Func":         "derived-only: a concretely typed child the frozen carrier admission does not enumerate",
	"FuncDefStmt.Name":         "derived-only: a concretely typed child the frozen carrier admission does not enumerate",
	"FunctionExpr.ParList":     "derived-only: a concretely typed child the frozen carrier admission does not enumerate",
	"GenericTypeExpr.Base":     "derived-only: a concretely typed child the frozen carrier admission does not enumerate",
	"InterfaceDefStmt.Extends": "derived-only: a concretely typed child the frozen carrier admission does not enumerate",
	"TableExpr.Fields":         "derived-only: a concretely typed child the frozen carrier admission does not enumerate",
}

// TestUseSlotsReproduceFrozenUseSlots is the slot receipt: once the named
// coordinates are set aside, the derived slot denominator and the frozen one
// are the same rows - same parent, same field, same role, same child type, same
// cardinality - with nothing missing and nothing extra.
func TestUseSlotsReproduceFrozenUseSlots(t *testing.T) {
	root := moduleRoot(t)
	value, err := census.Current(root)
	if err != nil {
		t.Fatal(err)
	}
	derived := make(map[string]string, len(value.Slots))
	derivedCoordinates := make(map[string]bool, len(value.Slots))
	for _, slot := range value.Slots {
		coordinate := slot.Form + "." + slot.Field
		if derivedCoordinates[coordinate] {
			t.Fatalf("derived slot %s is stated twice", coordinate)
		}
		derivedCoordinates[coordinate] = true
		if _, named := slotDifferences[coordinate]; named {
			continue
		}
		derived[coordinate] = slotKey(coordinate, slot.Role.String(), slot.ChildType, slot.Cardinality)
	}
	frozen := make(map[string]string, len(parseruses.Generated.UseSlots))
	frozenCoordinates := make(map[string]bool, len(parseruses.Generated.UseSlots))
	for _, slot := range parseruses.Generated.UseSlots {
		coordinate := slot.ParentForm + "." + slot.ParentField
		if frozenCoordinates[coordinate] {
			t.Fatalf("frozen slot %s is stated twice", coordinate)
		}
		frozenCoordinates[coordinate] = true
		if _, named := slotDifferences[coordinate]; named {
			continue
		}
		frozen[coordinate] = slotKey(coordinate, frozenUseRole(slot.Role), slot.ChildType, parsersource.FieldState(slot.Cardinality))
	}
	if len(frozen) == 0 {
		t.Fatal("the frozen evidence states no use slots")
	}
	for coordinate, reason := range slotDifferences {
		if !derivedCoordinates[coordinate] && !frozenCoordinates[coordinate] {
			t.Fatalf("the receipt names %s (%s) and neither denominator states it", coordinate, reason)
		}
	}
	var missing, extra, differing []string
	for coordinate, key := range frozen {
		switch derivedKey, present := derived[coordinate]; {
		case !present:
			missing = append(missing, coordinate)
		case derivedKey != key:
			differing = append(differing, fmt.Sprintf("%s: derived %s, frozen %s", coordinate, derivedKey, key))
		}
	}
	for coordinate := range derived {
		if _, present := frozen[coordinate]; !present {
			extra = append(extra, coordinate)
		}
	}
	sort.Strings(missing)
	sort.Strings(extra)
	sort.Strings(differing)
	if len(missing) != 0 || len(extra) != 0 || len(differing) != 0 {
		t.Fatalf("use slots differ from the frozen evidence:\nmissing %v\nextra %v\ndiffering %v", missing, extra, differing)
	}
	if len(derived) != len(frozen) {
		t.Fatalf("derived %d use slots, the frozen evidence states %d", len(derived), len(frozen))
	}
}

// TestUseEdgesReproduceFrozenUsePaths is the consumption receipt. The frozen
// evidence states one path per direct action edge and expands a helper edge
// once per call site; this grain states a helper's edges once, in the helper
// that performs them, for the same reason the product grain does: a helper is
// reached from many reductions, so attributing its edges to one of them would
// state a law about an alternative that does not hold it.
func TestUseEdgesReproduceFrozenUsePaths(t *testing.T) {
	root := moduleRoot(t)
	value, err := census.Current(root)
	if err != nil {
		t.Fatal(err)
	}
	// A construction performed once per element of an input sequence is filed by
	// the frozen evidence under its map-summary law rather than as a product, so
	// its edges are not among the frozen paths. This grain keeps the
	// construction and states which it is, exactly as the product receipt does.
	elementwise := make(map[string]bool)
	for _, product := range value.Products {
		if product.Elementwise {
			elementwise[fmt.Sprintf("%s#%d", product.Owner, product.Ordinal)] = true
		}
	}
	derived := make(map[string]bool, len(value.Uses))
	perElement := 0
	for _, use := range value.Uses {
		if _, named := slotDifferences[use.Form+"."+use.Field]; named {
			continue
		}
		if elementwise[fmt.Sprintf("%s#%d", use.Owner, use.Ordinal)] {
			perElement++
			continue
		}
		key := useKey(use.Owner, use.Ordinal, use.Form, use.Field)
		if derived[key] {
			t.Fatalf("derived use %s is stated twice", key)
		}
		derived[key] = true
	}
	if perElement == 0 {
		t.Fatal("no consumption edge belongs to a per-element construction, so a grain difference the receipt accounts for is untested")
	}
	frozen := frozenUseKeys(t)
	if len(frozen) == 0 {
		t.Fatal("the frozen evidence states no consumption edges")
	}
	var missing, extra []string
	for key := range frozen {
		if !derived[key] {
			missing = append(missing, key)
		}
	}
	for key := range derived {
		if !frozen[key] {
			extra = append(extra, key)
		}
	}
	sort.Strings(missing)
	sort.Strings(extra)
	if len(missing) != 0 || len(extra) != 0 {
		t.Fatalf("consumption edges differ from the frozen evidence:\nmissing %v\nextra %v", missing, extra)
	}
	if len(derived) != len(frozen) {
		t.Fatalf("derived %d consumption edges, the frozen evidence states %d", len(derived), len(frozen))
	}
}

// mutationFillDifferences are the slot fills the census mutation grain states
// and the frozen mutation-use family does not. Each is an assignment onto an
// already constructed value that reaches a real child slot, so the frozen
// family is missing a consumption edge rather than the census inventing one.
var mutationFillDifferences = map[string]string{
	"stat#6:IfStmt.Else": "grain differs: the frozen evidence folds this chained assignment into the construction's direct edges",
	"stat#7:IfStmt.Else": "grain differs: the frozen evidence folds this chained assignment into the construction's direct edges",
}

// chainedFill maps a frozen direct edge onto the census mutation fill that
// states the same assignment. The frozen evidence reads an ordered chain
// assignment as a coordinate of the construction it links from; this grain
// reads it as an assignment onto an already constructed value, because such an
// assignment can reach a state no construction of that form names. Neither
// loses the edge, so the receipt joins them rather than excluding either.
var chainedFill = map[string]string{
	"stat#6#1:IfStmt.Else": "stat#6:IfStmt.Else",
	"stat#7#1:IfStmt.Else": "stat#7:IfStmt.Else",
}

// TestMutationFillsReproduceFrozenMutationUsePaths is the other half of the
// consumption relation. A slot can also be filled by an assignment onto an
// already constructed value, and the census states those in its mutation grain
// rather than folding them into the construction grain, because a mutation can
// reach a slot no construction of that form ever fills.
//
// The frozen family addresses such an edge by the action that performs it, so
// the two are joined on that action: every frozen mutation edge reaches exactly
// one census mutation row, and that row names the coordinate the edge fills.
func TestMutationFillsReproduceFrozenMutationUsePaths(t *testing.T) {
	root := moduleRoot(t)
	value, err := census.Current(root)
	if err != nil {
		t.Fatal(err)
	}
	slots := make(map[string]bool, len(value.Slots))
	for _, slot := range value.Slots {
		slots[slot.Form+"."+slot.Field] = true
	}
	derived := make(map[string][]string, len(value.Mutations))
	fills := make(map[string]bool, len(value.Mutations))
	for _, mutation := range value.Mutations {
		coordinate := mutation.Constructor + "." + mutation.Field
		if !slots[coordinate] && slotDifferences[coordinate] == "" {
			continue
		}
		key := mutation.Owner + ":" + coordinate
		if fills[key] {
			continue
		}
		fills[key] = true
		derived[mutation.Owner] = append(derived[mutation.Owner], coordinate)
	}
	frozen := make(map[string]bool, len(parseruses.Generated.MutationUsePaths))
	for _, path := range parseruses.Generated.MutationUsePaths {
		frozen[path.Production] = true
	}
	if len(frozen) == 0 {
		t.Fatal("the frozen evidence states no mutation consumption edges")
	}
	var unreached []string
	for production := range frozen {
		if len(derived[production]) != 1 {
			unreached = append(unreached, fmt.Sprintf("%s reaches %v", production, derived[production]))
		}
	}
	sort.Strings(unreached)
	if len(unreached) != 0 {
		t.Fatalf("the census mutation grain does not reach one exact fill for the frozen mutation edges: %v", unreached)
	}
	var residue []string
	for owner, coordinates := range derived {
		if frozen[owner] {
			continue
		}
		for _, coordinate := range coordinates {
			key := owner + ":" + coordinate
			if _, named := mutationFillDifferences[key]; !named {
				residue = append(residue, key)
			}
		}
	}
	sort.Strings(residue)
	if len(residue) != 0 {
		t.Fatalf("the census states mutation fills the receipt does not account for: %v", residue)
	}
}

func slotKey(coordinate, role, child string, cardinality parsersource.FieldState) string {
	return fmt.Sprintf("%s|%s|%s|%s", coordinate, role, child, cardinality)
}

func useKey(owner string, ordinal int, form, field string) string {
	return fmt.Sprintf("%s#%d:%s.%s", owner, ordinal, form, field)
}

// frozenUseRole spells one frozen role in the vocabulary the derivation uses.
// The two extra frozen roles keep their own spellings, so a row that carries
// one cannot be silently compared against a derived row.
func frozenUseRole(role parseruses.UseRole) string {
	switch role {
	case parseruses.UseRoleChild:
		return parsersource.UseRoleChild.String()
	case parseruses.UseRoleControl:
		return parsersource.UseRoleControl.String()
	case parseruses.UseRoleStatic:
		return parsersource.UseRoleStatic.String()
	case parseruses.UseRoleLValue:
		return "lvalue"
	case parseruses.UseRoleAdjustment:
		return "adjustment"
	default:
		return "invalid"
	}
}

func frozenUseKeys(t *testing.T) map[string]bool {
	t.Helper()
	evidence := parseruses.Generated
	result := make(map[string]bool)
	chained := 0
	for _, path := range evidence.UsePaths {
		if _, named := slotDifferences[path.ParentForm+"."+path.ParentField]; named {
			continue
		}
		key := useKey(path.ParentProduction, path.ParentOrdinal, path.ParentForm, path.ParentField)
		if _, named := mutationFillDifferences[chainedFill[key]]; named {
			chained++
			continue
		}
		result[key] = true
	}
	if chained != len(chainedFill) {
		t.Fatalf("the receipt names %d chained fills, the frozen evidence states %d", len(chainedFill), chained)
	}
	for _, path := range evidence.HelperUsePaths {
		if _, named := slotDifferences[path.ParentForm+"."+path.ParentField]; named {
			continue
		}
		symbol, ok := parserproducts.Generated.ActionTerms.Symbol(path.Helper)
		if !ok {
			t.Fatal("frozen helper use path names no callable")
		}
		result[useKey(symbol.Text, path.Ordinal, path.ParentForm, path.ParentField)] = true
	}
	return result
}

