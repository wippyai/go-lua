package authored

import (
	"testing"

	"github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
)

func TestOperatorsAreDenseImmutableAuthoredRows(t *testing.T) {
	input, terms := operatorFixture()
	component := buildFlowForTest(t, input)
	operators := component.Operators()
	if operators.Unaries().Count() != 4 || operators.Binaries().Count() != 19 || operators.Selects().Count() != 2 {
		t.Fatalf("operator counts = %d %d %d", operators.Unaries().Count(), operators.Binaries().Count(), operators.Selects().Count())
	}
	if term, ok := operators.Unaries().At(0); !ok || term != terms.unary[0] {
		t.Fatalf("Unary At = %08x, %v", uint32(term), ok)
	}
	if owner, op, operand, ok := operators.Unaries().Get(terms.unary[0]); !ok || owner != terms.body || op != kind.UnaryNeg || operand != terms.integer {
		t.Fatalf("Unary Get = %08x %v %08x %v", uint32(owner), op, uint32(operand), ok)
	}
	if owner, op, left, right, ok := operators.Binaries().Get(terms.binary[18]); !ok || owner != terms.body || op != kind.BinaryGreaterEqual || left != terms.integer || right != terms.float {
		t.Fatalf("Binary Get = %08x %v %08x %08x %v", uint32(owner), op, uint32(left), uint32(right), ok)
	}
	if owner, op, left, right, ok := operators.Selects().Get(terms.selects[1]); !ok || owner != terms.body || op != kind.SelectOr || left != terms.boolean || right != terms.integer {
		t.Fatalf("Select Get = %08x %v %08x %08x %v", uint32(owner), op, uint32(left), uint32(right), ok)
	}
	input.Operators.Unaries[0].Operand = terms.boolean
	if _, _, operand, ok := operators.Unaries().Get(terms.unary[0]); !ok || operand != terms.integer {
		t.Fatal("operator input mutation leaked through caller copy")
	}
	if _, _, _, _, ok := operators.Binaries().Get(terms.unary[0]); ok {
		t.Fatal("Binary Get accepted a Unary term")
	}
}

func TestOperatorsRejectHostileRowsAndChangeIdentity(t *testing.T) {
	input, terms := operatorFixture()
	first := buildFlowForTest(t, input)
	for _, test := range []struct {
		name  string
		apply func(*Input)
	}{
		{"Unary owner", func(changed *Input) { changed.Operators.Unaries[0].Owner = terms.otherBody }},
		{"Unary op", func(changed *Input) { changed.Operators.Unaries[0].Op = kind.UnaryLen }},
		{"Unary operand", func(changed *Input) { changed.Operators.Unaries[0].Operand = terms.boolean }},
		{"Binary owner", func(changed *Input) { changed.Operators.Binaries[0].Owner = terms.otherBody }},
		{"Binary op", func(changed *Input) { changed.Operators.Binaries[0].Op = kind.BinarySub }},
		{"Binary left", func(changed *Input) { changed.Operators.Binaries[0].Left = terms.boolean }},
		{"Binary right", func(changed *Input) { changed.Operators.Binaries[0].Right = terms.integer }},
		{"Select owner", func(changed *Input) { changed.Operators.Selects[0].Owner = terms.otherBody }},
		{"Select op", func(changed *Input) { changed.Operators.Selects[0].Op = kind.SelectOr }},
		{"Select left", func(changed *Input) { changed.Operators.Selects[0].Left = terms.integer }},
		{"Select right", func(changed *Input) { changed.Operators.Selects[0].Right = terms.boolean }},
	} {
		t.Run(test.name, func(t *testing.T) {
			changed := copyOperatorsInput(input)
			test.apply(&changed)
			if first.Cold().ContentID() == buildFlowForTest(t, changed).Cold().ContentID() {
				t.Fatal("authored operator field did not change ContentID")
			}
		})
	}

	for _, test := range []struct {
		name  string
		apply func(*Input, operatorTerms)
	}{
		{"Unary count", func(input *Input, _ operatorTerms) { input.Counts[keyspace.FamilyUnary]-- }},
		{"Binary count", func(input *Input, _ operatorTerms) { input.Counts[keyspace.FamilyBinary]-- }},
		{"Select count", func(input *Input, _ operatorTerms) { input.Counts[keyspace.FamilySelect]-- }},
		{"Unary enum", func(input *Input, _ operatorTerms) { input.Operators.Unaries[0].Op = 0 }},
		{"Binary enum", func(input *Input, _ operatorTerms) { input.Operators.Binaries[0].Op = 0 }},
		{"Select enum", func(input *Input, _ operatorTerms) { input.Operators.Selects[0].Op = 0 }},
		{"Unary owner", func(input *Input, terms operatorTerms) { input.Operators.Unaries[0].Owner = terms.integer }},
		{"Binary owner", func(input *Input, terms operatorTerms) { input.Operators.Binaries[0].Owner = terms.integer }},
		{"Select owner", func(input *Input, terms operatorTerms) { input.Operators.Selects[0].Owner = terms.integer }},
		{"Unary operand", func(input *Input, terms operatorTerms) { input.Operators.Unaries[0].Operand = terms.body }},
		{"Binary operand", func(input *Input, terms operatorTerms) { input.Operators.Binaries[0].Left = terms.body }},
		{"Select operand", func(input *Input, terms operatorTerms) { input.Operators.Selects[0].Right = terms.body }},
	} {
		t.Run(test.name, func(t *testing.T) {
			input, terms := operatorFixture()
			test.apply(&input, terms)
			if _, err := Build(input); err == nil {
				t.Fatal("hostile operator row accepted")
			}
		})
	}
}

func TestOperatorUnaryNegIsTheOnlyAdditionalExactStaticKey(t *testing.T) {
	for _, test := range []struct {
		name   string
		family keyspace.Family
	}{
		{"Integer", keyspace.FamilyInteger},
		{"Float", keyspace.FamilyFloat},
	} {
		t.Run(test.name, func(t *testing.T) {
			input, terms := flowFixture()
			operand := keyspace.MakeTerm(test.family, 1)
			unary := keyspace.MakeTerm(keyspace.FamilyUnary, 1)
			input.Counts[test.family] = 1
			input.Counts[keyspace.FamilyUnary] = 1
			input.Counts[keyspace.FamilyLensExact] = 1
			input.Operators.Unaries = []Unary{{Owner: terms.body, Op: kind.UnaryNeg, Operand: operand}}
			input.Tables.Fields[0].Kind, input.Tables.Fields[0].Key = kind.FieldExact, unary
			input.Access.Exact = []ExactLens{{Owner: terms.body, Base: terms.nil, Source: unary, Kind: kind.FieldExact}}
			if component := buildFlowForTest(t, input); component.Operators().Unaries().Count() == 0 {
				t.Fatal("negative numeric static Field/Lens key rejected")
			}
		})
	}

	input, terms := flowFixture()
	integer := keyspace.MakeTerm(keyspace.FamilyInteger, 1)
	unary := keyspace.MakeTerm(keyspace.FamilyUnary, 1)
	input.Counts[keyspace.FamilyInteger], input.Counts[keyspace.FamilyUnary], input.Counts[keyspace.FamilyLensExact] = 1, 1, 1
	input.Operators.Unaries = []Unary{{Owner: terms.body, Op: kind.UnaryNeg, Operand: integer}}
	input.Tables.Fields[0].Kind, input.Tables.Fields[0].Key = kind.FieldExact, unary
	input.Access.Exact = []ExactLens{{Owner: terms.body, Base: terms.nil, Source: unary, Kind: kind.FieldExact}}
	input.Operators.Unaries[0].Op = kind.UnaryNot
	if _, err := Build(input); err == nil {
		t.Fatal("logical Unary accepted as static key")
	}
	input.Operators.Unaries[0] = Unary{Owner: terms.body, Op: kind.UnaryNeg, Operand: terms.boolean}
	if _, err := Build(input); err == nil {
		t.Fatal("negative nonnumeric Unary accepted as static key")
	}
}

func TestOperatorViewsFailClosedAtEveryBoundary(t *testing.T) {
	component, terms := buildOperatorComponent(t)
	operators := component.Operators()
	var nilView View
	nilOperators := nilView.Operators()
	for _, test := range []struct {
		name        string
		count       func() int
		at          func(int) (keyspace.Term, bool)
		get         func(keyspace.Term) bool
		nilCount    func() int
		nilAt       func(int) (keyspace.Term, bool)
		nilGet      func(keyspace.Term) bool
		wrong, over keyspace.Term
	}{
		{
			name: "Unary", count: operators.Unaries().Count, at: operators.Unaries().At,
			get:      func(term keyspace.Term) bool { _, _, _, ok := operators.Unaries().Get(term); return ok },
			nilCount: nilOperators.Unaries().Count, nilAt: nilOperators.Unaries().At,
			nilGet: func(term keyspace.Term) bool { _, _, _, ok := nilOperators.Unaries().Get(term); return ok },
			wrong:  terms.binary[0], over: keyspace.MakeTerm(keyspace.FamilyUnary, uint32(len(terms.unary)+1)),
		},
		{
			name: "Binary", count: operators.Binaries().Count, at: operators.Binaries().At,
			get:      func(term keyspace.Term) bool { _, _, _, _, ok := operators.Binaries().Get(term); return ok },
			nilCount: nilOperators.Binaries().Count, nilAt: nilOperators.Binaries().At,
			nilGet: func(term keyspace.Term) bool { _, _, _, _, ok := nilOperators.Binaries().Get(term); return ok },
			wrong:  terms.selects[0], over: keyspace.MakeTerm(keyspace.FamilyBinary, uint32(len(terms.binary)+1)),
		},
		{
			name: "Select", count: operators.Selects().Count, at: operators.Selects().At,
			get:      func(term keyspace.Term) bool { _, _, _, _, ok := operators.Selects().Get(term); return ok },
			nilCount: nilOperators.Selects().Count, nilAt: nilOperators.Selects().At,
			nilGet: func(term keyspace.Term) bool { _, _, _, _, ok := nilOperators.Selects().Get(term); return ok },
			wrong:  terms.unary[0], over: keyspace.MakeTerm(keyspace.FamilySelect, uint32(len(terms.selects)+1)),
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if test.count() == 0 || test.nilCount() != 0 {
				t.Fatal("unexpected view count")
			}
			for _, index := range []int{-1, test.count()} {
				if _, ok := test.at(index); ok {
					t.Fatalf("At(%d) accepted", index)
				}
			}
			if _, ok := test.nilAt(0); ok || test.nilGet(0) || test.get(0) || test.get(test.wrong) || test.get(test.over) {
				t.Fatal("operator view accepted an invalid boundary")
			}
		})
	}
}

type operatorTerms struct {
	body, otherBody, boolean, integer, float keyspace.Term
	unary                                    [4]keyspace.Term
	binary                                   [19]keyspace.Term
	selects                                  [2]keyspace.Term
}

func operatorFixture() (Input, operatorTerms) {
	terms := operatorTerms{
		body: keyspace.MakeTerm(keyspace.FamilyBody, 1), otherBody: keyspace.MakeTerm(keyspace.FamilyBody, 2), boolean: keyspace.MakeTerm(keyspace.FamilyBool, 1),
		integer: keyspace.MakeTerm(keyspace.FamilyInteger, 1), float: keyspace.MakeTerm(keyspace.FamilyFloat, 1),
	}
	var counts [keyspace.FamilyCount]uint32
	for _, term := range []keyspace.Term{terms.body, terms.otherBody, terms.boolean, terms.integer, terms.float} {
		counts[keyspace.TermFamily(term)]++
	}
	for index := range terms.unary {
		terms.unary[index] = keyspace.MakeTerm(keyspace.FamilyUnary, uint32(index+1))
		counts[keyspace.FamilyUnary]++
	}
	for index := range terms.binary {
		terms.binary[index] = keyspace.MakeTerm(keyspace.FamilyBinary, uint32(index+1))
		counts[keyspace.FamilyBinary]++
	}
	for index := range terms.selects {
		terms.selects[index] = keyspace.MakeTerm(keyspace.FamilySelect, uint32(index+1))
		counts[keyspace.FamilySelect]++
	}
	unaries := make([]Unary, len(terms.unary))
	for index := range unaries {
		unaries[index] = Unary{Owner: terms.body, Op: kind.UnaryOp(index + 1), Operand: terms.integer}
	}
	binaries := make([]Binary, len(terms.binary))
	for index := range binaries {
		binaries[index] = Binary{Owner: terms.body, Op: kind.BinaryOp(index + 1), Left: terms.integer, Right: terms.float}
	}
	return Input{Counts: counts, Operators: OperatorsInput{
		Unaries: unaries, Binaries: binaries,
		Selects: []Select{{Owner: terms.body, Op: kind.SelectAnd, Left: terms.boolean, Right: terms.integer}, {Owner: terms.body, Op: kind.SelectOr, Left: terms.boolean, Right: terms.integer}},
	}}, terms
}

func copyOperatorsInput(input Input) Input {
	copy := input
	copy.Operators.Unaries = append([]Unary(nil), input.Operators.Unaries...)
	copy.Operators.Binaries = append([]Binary(nil), input.Operators.Binaries...)
	copy.Operators.Selects = append([]Select(nil), input.Operators.Selects...)
	return copy
}

func buildOperatorComponent(t *testing.T) (View, operatorTerms) {
	t.Helper()
	input, terms := operatorFixture()
	return buildFlowForTest(t, input), terms
}
