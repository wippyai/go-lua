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

// TestSequenceRowsAreTotalOverListCarriers states the denominator law of the
// list-building grain. A list carrier belongs to a nonterminal, so every
// alternative of that nonterminal disposes of it: one that never mentions the
// carrier leaves it empty, which is a law about the alternative and not an
// absence of evidence. A grain that only stated the alternatives which do
// mention a list could not tell a seeded list from an unstated one.
func TestSequenceRowsAreTotalOverListCarriers(t *testing.T) {
	root := moduleRoot(t)
	value, err := Current(root)
	if err != nil {
		t.Fatal(err)
	}
	carriers, err := parsersource.SequenceCarriers(root)
	if err != nil {
		t.Fatal(err)
	}
	byTag := make(map[string][]parsersource.SequenceCarrier, len(carriers))
	for _, carrier := range carriers {
		byTag[carrier.Tag] = append(byTag[carrier.Tag], carrier)
	}
	stated := make(map[string]bool, len(value.Sequences))
	for _, sequence := range value.Sequences {
		key := SequenceRow(sequence.Production, sequence.Tag, sequence.Field)
		if stated[key] {
			t.Fatalf("the census states %s twice", key)
		}
		stated[key] = true
		if sequence.Construction == parsersource.SequenceConstructionInvalid {
			t.Fatalf("%s states no construction", key)
		}
		if sequence.Construction == parsersource.SequenceConstructionNil && len(sequence.Segments) != 0 {
			t.Fatalf("%s states no list and names %d operands", key, len(sequence.Segments))
		}
		for ordinal, segment := range sequence.Segments {
			if segment.Ordinal != ordinal+1 {
				t.Fatalf("%s operand %d is numbered %d", key, ordinal+1, segment.Ordinal)
			}
			if segment.Kind == parsersource.SequenceSegmentInvalid {
				t.Fatalf("%s operand %d states no kind", key, segment.Ordinal)
			}
			if len(segment.Origins) == 0 {
				t.Fatalf("%s operand %d names no origin", key, segment.Ordinal)
			}
			if segment.Kind == parsersource.SequenceSpread && sequence.Construction == parsersource.SequenceConstructionLiteral {
				t.Fatalf("%s states a whole list and splices another into it", key)
			}
		}
	}
	var missing []string
	for _, production := range value.Productions {
		for _, carrier := range byTag[production.ResultTag] {
			key := SequenceRow(production.Key, carrier.Tag, carrier.Field)
			if !stated[key] {
				missing = append(missing, key)
			}
		}
	}
	sort.Strings(missing)
	if len(missing) != 0 {
		t.Fatalf("alternatives whose result declares a list carrier state no disposition for it: %v", missing)
	}
	if len(stated) != len(value.Sequences) {
		t.Fatalf("the census states %d sequence rows under %d keys", len(value.Sequences), len(stated))
	}
}

// TestSequenceOperandsNameTheirProvenance states that a list operand is an edge
// and not a fact about a length. Two alternatives that extend the same carrier
// from different reduction operands build different lists, and a row that
// stated only how many members it added could not tell them apart.
func TestSequenceOperandsNameTheirProvenance(t *testing.T) {
	root := moduleRoot(t)
	value, err := Current(root)
	if err != nil {
		t.Fatal(err)
	}
	productions := make(map[string]parsersource.ActionTemplate, len(value.Productions))
	for _, production := range value.Productions {
		productions[production.Key] = production
	}
	named := 0
	for _, sequence := range value.Sequences {
		production, known := productions[sequence.Production]
		if !known {
			t.Fatalf("sequence row %s names no census production", SequenceRow(sequence.Production, sequence.Tag, sequence.Field))
		}
		symbols, symbolErr := parsersource.ProductionSymbols(production.RHS)
		if symbolErr != nil {
			t.Fatal(symbolErr)
		}
		for _, segment := range sequence.Segments {
			for _, symbol := range segment.Symbols {
				if symbol <= 0 || symbol > len(symbols) {
					t.Fatalf("%s operand %d names reduction operand %d of %d", SequenceRow(sequence.Production, sequence.Tag, sequence.Field), segment.Ordinal, symbol, len(symbols))
				}
				named++
			}
			for _, origin := range segment.Origins {
				if origin == parsersource.UseOriginInvalid {
					t.Fatalf("%s operand %d states an invalid origin", SequenceRow(sequence.Production, sequence.Tag, sequence.Field), segment.Ordinal)
				}
			}
		}
	}
	if named == 0 {
		t.Fatal("no list operand names a reduction operand, so the provenance column proves nothing")
	}
}

// TestDriftGuardRejectsARelistedResult is the drift guard of the list-building
// grain. Rewriting a one-member list literal as an append over an empty list
// leaves every other grain identical - the same alternative, the same forms,
// the same carriers, the same whole-constructor field vectors, the same
// consumption edges - and changes only how the reduction assembles its result
// list. A census that did not close over that would keep describing a parser
// that seeds a list where it now extends one.
func TestDriftGuardRejectsARelistedResult(t *testing.T) {
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
	const original = "$$ = []ast.AnnotationExpr{$1}"
	const edited = "$$ = append([]ast.AnnotationExpr(nil), $1)"
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
	// below is attributable to the list-building grain alone.
	if !sameProducts(current, edit) {
		t.Fatal("relisting a result changed the product grain, so this edit does not isolate the list grain")
	}
	if !reflect.DeepEqual(current.Slots, edit.Slots) || !reflect.DeepEqual(current.Uses, edit.Uses) || !reflect.DeepEqual(current.Mutations, edit.Mutations) {
		t.Fatal("relisting a result changed the slot, consumption or mutation grain, so this edit does not isolate the list grain")
	}
	before := sequenceOf(t, current, "annotations#1")
	after := sequenceOf(t, edit, "annotations#1")
	if before.Construction != parsersource.SequenceConstructionLiteral {
		t.Fatalf("annotations#1 states its list as %s, want literal", before.Construction)
	}
	if after.Construction != parsersource.SequenceConstructionAppend {
		t.Fatalf("the relisted annotations#1 states its list as %s, want append", after.Construction)
	}
	if err := Generated.Validate(copied); err == nil {
		t.Fatal("census accepted a list construction it was not generated from")
	}
}

func sequenceOf(t *testing.T, value Census, production string) parsersource.ActionSequence {
	t.Helper()
	for _, sequence := range value.Sequences {
		if sequence.Production == production {
			return sequence
		}
	}
	t.Fatalf("the census states no list disposition for %s", production)
	return parsersource.ActionSequence{}
}
