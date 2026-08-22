package parsersource

import (
	goast "go/ast"
	"path/filepath"
	"testing"
)

func TestSequenceInputDistinguishesExactOperandsFromProjections(t *testing.T) {
	scope := &actionScope{kind: ProductScopeProduction}
	if got := directSequenceInput(scope, goast.NewIdent("Arg2"), nil); got != 2 {
		t.Fatalf("exact input = %d, want 2", got)
	}
	projected := &goast.SelectorExpr{X: goast.NewIdent("Arg2"), Sel: goast.NewIdent("Field")}
	if got := directSequenceInput(scope, projected, nil); got != 0 {
		t.Fatalf("projected input = %d, want no exact input", got)
	}
	indexed := &goast.IndexExpr{X: goast.NewIdent("Arg2"), Index: &goast.BasicLit{}}
	if got := directSequenceInput(scope, indexed, nil); got != 0 {
		t.Fatalf("indexed input = %d, want no exact input", got)
	}
	frame := &helperFrame{actuals: map[string]goast.Expr{"value": goast.NewIdent("Arg3")}, caller: scope}
	helper := &actionScope{kind: ProductScopeHelper}
	if got := directSequenceInput(helper, goast.NewIdent("value"), frame); got != 3 {
		t.Fatalf("forwarded exact input = %d, want 3", got)
	}
}

// TestSequenceCarriersAreDerivedFromDeclaredArms states where the list
// denominator comes from. A result tag carries a list because its %union arm is
// declared as one, or because the parser-private wrapper that arm names
// declares a slice member. Reading it any other way would let a wrapper become
// a list owner by being named like one.
func TestSequenceCarriersAreDerivedFromDeclaredArms(t *testing.T) {
	root := moduleRoot(t)
	carriers, err := SequenceCarriers(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(carriers) == 0 {
		t.Fatal("the parser declares no list-valued result")
	}
	roots, members := 0, 0
	for _, carrier := range carriers {
		if carrier.Tag == "" {
			t.Fatal("a list carrier names no result tag")
		}
		if carrier.Field == "" {
			roots++
			continue
		}
		members++
	}
	if roots == 0 || members == 0 {
		t.Fatalf("the denominator states %d whole-result lists and %d wrapper members, so one of the two readings proves nothing", roots, members)
	}
	vocabulary, err := DiscoverVocabulary(filepath.Join(root, "compiler", "parse", "parser.go.y"))
	if err != nil {
		t.Fatal(err)
	}
	declared := make(map[string]bool, len(vocabulary.Symbols))
	for _, symbol := range vocabulary.Symbols {
		if symbol.Tag != "" {
			declared[symbol.Tag] = true
		}
	}
	for _, carrier := range carriers {
		if !declared[carrier.Tag] {
			t.Fatalf("list carrier %s names a result tag no symbol declares", carrier.Tag)
		}
	}
}

// TestListDispositionsCoverTheFourConstructions states that the construction
// vocabulary is exercised rather than merely declared. The four are the whole
// law: a reduction leaves its list alone, states it outright, hands one
// through, or extends one. If the parser only ever reached some of them, a
// reader could not tell a closed vocabulary from an unreached one.
func TestListDispositionsCoverTheFourConstructions(t *testing.T) {
	root := moduleRoot(t)
	analysis, err := DiscoverProducts(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(analysis.Sequences) == 0 {
		t.Fatal("the parser states no list disposition")
	}
	reached := make(map[SequenceConstruction]int, 4)
	elements, spreads := 0, 0
	for _, sequence := range analysis.Sequences {
		reached[sequence.Construction]++
		for _, segment := range sequence.Segments {
			switch segment.Kind {
			case SequenceElement:
				elements++
			case SequenceSpread:
				spreads++
			default:
				t.Fatalf("%s operand %d states no kind", sequence.Production, segment.Ordinal)
			}
		}
	}
	for _, construction := range []SequenceConstruction{SequenceConstructionNil, SequenceConstructionLiteral, SequenceConstructionForward, SequenceConstructionAppend} {
		if reached[construction] == 0 {
			t.Fatalf("no reduction states its list as %s", construction)
		}
	}
	if elements == 0 || spreads == 0 {
		t.Fatalf("the operand vocabulary reaches %d members and %d spliced lists", elements, spreads)
	}
}

// TestForwardedListDispositionsReadTheCallOperands states the law about a
// reduction that delegates its list to a parser helper. The helper is reached
// from several alternatives, so the disposition belongs to the call: an operand
// the helper states about its own parameter must be reported at the reduction
// coordinate the caller supplied it from, or two alternatives sharing a helper
// would state the same law about different lists.
func TestForwardedListDispositionsReadTheCallOperands(t *testing.T) {
	root := moduleRoot(t)
	analysis, err := DiscoverProducts(root)
	if err != nil {
		t.Fatal(err)
	}
	forwarded := 0
	for _, sequence := range analysis.Sequences {
		if sequence.Production != "funcparamlist#2" {
			continue
		}
		forwarded++
		if sequence.Construction != SequenceConstructionAppend {
			t.Fatalf("funcparamlist#2 states its list as %s, want append", sequence.Construction)
		}
		if len(sequence.Segments) != 2 {
			t.Fatalf("funcparamlist#2 names %d operands, want 2", len(sequence.Segments))
		}
		if sequence.Segments[0].Kind != SequenceSpread || sequence.Segments[1].Kind != SequenceElement {
			t.Fatalf("funcparamlist#2 extends its list as %s then %s", sequence.Segments[0].Kind, sequence.Segments[1].Kind)
		}
		for _, segment := range sequence.Segments {
			if len(segment.Symbols) == 0 {
				t.Fatalf("funcparamlist#2 operand %d is reported as an anonymous helper parameter", segment.Ordinal)
			}
			for _, origin := range segment.Origins {
				if origin == UseOriginParameter {
					t.Fatalf("funcparamlist#2 operand %d is reported at the helper's coordinate", segment.Ordinal)
				}
			}
		}
	}
	if forwarded == 0 {
		t.Fatal("no reduction delegates a list disposition to a parser helper")
	}
}
