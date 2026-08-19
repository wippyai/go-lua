package source

import (
	"math"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestKeyFaultCanonicalSourceAuthority(t *testing.T) {
	input, index := keyFaultFixture()
	draft, err := Build(input)
	if err != nil {
		t.Fatal(err)
	}
	component, err := commitSource(draft, index)
	if err != nil {
		t.Fatal(err)
	}
	view := component.View()
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	key1 := keyspace.MakeTerm(keyspace.FamilyKey, 1)
	key2 := keyspace.MakeTerm(keyspace.FamilyKey, 2)
	key3 := keyspace.MakeTerm(keyspace.FamilyKey, 3)
	if owner, text, exact, ok := view.Keys().Name(key1); !ok || owner != body || text != "z" || exact != 3 {
		t.Fatalf("Name(%v) = %v/%q/%v/%v, want owner z/key3", key1, owner, text, exact, ok)
	}
	if owner, ordinal, exact, ok := view.Keys().List(key2); !ok || owner != body || ordinal != 1 || exact != 1 {
		t.Fatalf("List(%v) = %v/%d/%v/%v, want owner 1/key1", key2, owner, ordinal, exact, ok)
	}
	if _, _, _, ok := view.Keys().List(key3); ok {
		t.Fatal("Name key was accepted as a List key")
	}
	fault, ok := view.Faults().At(keyspace.MakeTerm(keyspace.FamilyControlFault, 1))
	if !ok || fault.Kind != ControlFaultDuplicateLabel || fault.Label != keyspace.MakeTerm(keyspace.FamilyLabel, 1) {
		t.Fatalf("fault = %#v/%v, want duplicate-label evidence", fault, ok)
	}
	atoms := view.Keys()
	if atoms.ExactCount() != 3 {
		t.Fatalf("atom count = %d, want 3", atoms.ExactCount())
	}
	if key, ok := atoms.Find(keyspace.LiteralValue{Kind: keyspace.LiteralFloat, FloatBits: math.Float64bits(1)}); !ok || key != 1 {
		t.Fatalf("Find(float 1) = %v/%v, want normalized dense key 1", key, ok)
	}
	for index, want := range []keyspace.LiteralValue{
		{Kind: keyspace.LiteralInteger, Integer: 1},
		{Kind: keyspace.LiteralString, String: "a"},
		{Kind: keyspace.LiteralString, String: "z"},
	} {
		_, got, ok := atoms.ExactAt(index)
		if !ok || got != want {
			t.Fatalf("canonical atom %d = %#v/%v, want %#v", index, got, ok, want)
		}
	}
}

func TestKeyAtomsHaveCanonicalDenseHandlesAcrossInputPermutations(t *testing.T) {
	firstInput, firstIndex := keyFaultFixture()
	secondInput, secondIndex := keyFaultFixture()
	secondInput.ExactAtoms = []keyspace.LiteralValue{
		{Kind: keyspace.LiteralString, String: "z"},
		{Kind: keyspace.LiteralInteger, Integer: 1},
		{Kind: keyspace.LiteralString, String: "a"},
		{Kind: keyspace.LiteralFloat, FloatBits: math.Float64bits(1)},
	}
	first := finalizeSource(t, firstInput, firstIndex)
	second := finalizeSource(t, secondInput, secondIndex)
	firstView, secondView := first.View(), second.View()
	if got, want := secondView.Identity().ContentID(), firstView.Identity().ContentID(); got != want {
		t.Fatalf("permuted exact denominator changed ContentID: %x != %x", got, want)
	}
	for _, raw := range []keyspace.LiteralValue{
		{Kind: keyspace.LiteralInteger, Integer: 1},
		{Kind: keyspace.LiteralString, String: "a"},
		{Kind: keyspace.LiteralString, String: "z"},
	} {
		left, leftOK := firstView.Keys().Find(raw)
		right, rightOK := secondView.Keys().Find(raw)
		if !leftOK || !rightOK || left != right {
			t.Fatalf("Find(%#v) = %v/%v, %v/%v; want identical canonical handle", raw, left, leftOK, right, rightOK)
		}
	}
	for index := 0; index < firstView.Keys().ExactCount(); index++ {
		leftKey, leftValue, leftOK := firstView.Keys().ExactAt(index)
		rightKey, rightValue, rightOK := secondView.Keys().ExactAt(index)
		if !leftOK || !rightOK || leftKey != rightKey || leftValue != rightValue {
			t.Fatalf("ExactAt(%d) differs: %v/%#v/%v vs %v/%#v/%v", index, leftKey, leftValue, leftOK, rightKey, rightValue, rightOK)
		}
	}
	for ordinal := uint32(1); ordinal <= 3; ordinal++ {
		term := keyspace.MakeTerm(keyspace.FamilyKey, ordinal)
		leftOwner, leftName, leftKey, leftOK := firstView.Keys().Name(term)
		rightOwner, rightName, rightKey, rightOK := secondView.Keys().Name(term)
		if leftOK != rightOK || leftOwner != rightOwner || leftName != rightName || leftKey != rightKey {
			t.Fatalf("Name(%v) differs across atom permutation", term)
		}
		leftOwner, leftList, leftKey, leftOK := firstView.Keys().List(term)
		rightOwner, rightList, rightKey, rightOK := secondView.Keys().List(term)
		if leftOK != rightOK || leftOwner != rightOwner || leftList != rightList || leftKey != rightKey {
			t.Fatalf("List(%v) differs across atom permutation", term)
		}
	}
}

func TestKeyFaultRejectsIncompleteExactDenominatorAndClosedRows(t *testing.T) {
	for _, mutate := range []func(*Input){
		func(input *Input) { input.ExactAtoms = input.ExactAtoms[:2] },
		func(input *Input) { input.Keys[1] = ListKey(keyspace.MakeTerm(keyspace.FamilyBody, 1), 0) },
		func(input *Input) { input.Keys[0] = NameKey(keyspace.MakeTerm(keyspace.FamilyKey, 1), "z") },
		func(input *Input) {
			input.Faults[0] = ControlFault{Owner: keyspace.MakeTerm(keyspace.FamilyBody, 1), Kind: ControlFaultDuplicateLabel}
		},
		func(input *Input) {
			input.Faults[0] = ControlFault{Owner: keyspace.MakeTerm(keyspace.FamilyBody, 1), Kind: ControlFaultGotoEntersLocal, Label: keyspace.MakeTerm(keyspace.FamilyLabel, 1)}
		},
		func(input *Input) {
			input.Faults[0] = ControlFault{Owner: keyspace.MakeTerm(keyspace.FamilyBody, 1), Kind: ControlFaultKind(99), Label: keyspace.MakeTerm(keyspace.FamilyLabel, 1), Blocker: keyspace.MakeTerm(keyspace.FamilyCell, 1)}
		},
	} {
		input, _ := keyFaultFixture()
		mutate(&input)
		if _, err := Build(input); err == nil {
			t.Fatal("Build unexpectedly accepted an incomplete or invalid Source row")
		}
	}
}

func TestKeyFaultRejectsFaultWithoutOwnerBodyOccurrence(t *testing.T) {
	input, _ := keyFaultFixture()
	fault := keyspace.MakeTerm(keyspace.FamilyControlFault, 1)
	input.Bodies[0].Terms = withoutDirectTerm(input.Bodies[0].Terms, fault)
	if _, err := Build(input); err == nil {
		t.Fatal("Build accepted a control fault outside its owner Body sequence")
	}
}

func TestKeyFaultRequiresDirectFaultPosition(t *testing.T) {
	input, index := keyFaultFixture()
	draft, err := Build(input)
	if err != nil {
		t.Fatal(err)
	}
	fault := keyspace.MakeTerm(keyspace.FamilyControlFault, 1)
	index.Positions = withoutPosition(index.Positions, fault)
	if _, err := commitSource(draft, index); err == nil {
		t.Fatal("Finalize accepted a direct control fault without Position")
	}

	completeInput, completeIndex := keyFaultFixture()
	draft, err = Build(completeInput)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := commitSource(draft, completeIndex); err != nil {
		t.Fatal(err)
	}
}

func TestCopiedDraftSharesOneShotFinalization(t *testing.T) {
	input, index := keyFaultFixture()
	draft, err := Build(input)
	if err != nil {
		t.Fatal(err)
	}
	copy := *draft
	if _, err := commitSource(&copy, index); err != nil {
		t.Fatalf("copied Draft Finalize: %v", err)
	}
	if _, err := commitSource(draft, index); err == nil {
		t.Fatal("original Draft finalized after copied Draft consumed shared state")
	}
}

func TestFailedFinalizeConsumesEveryDraftCopy(t *testing.T) {
	input, bad := keyFaultFixture()
	complete := bad
	complete.Positions = append([]Position(nil), bad.Positions...)
	fault := keyspace.MakeTerm(keyspace.FamilyControlFault, 1)
	bad.Positions = withoutPosition(bad.Positions, fault)

	draft, err := Build(input)
	if err != nil {
		t.Fatal(err)
	}
	copy := *draft
	if _, err := commitSource(&copy, bad); err == nil {
		t.Fatal("copied Draft accepted incomplete Position batch")
	}
	if _, err := commitSource(draft, complete); err == nil {
		t.Fatal("original Draft finalized after copied Draft's failed attempt")
	}
}

func TestSourceContentChangesForEveryAuthoredFamily(t *testing.T) {
	baseline, _ := contentFixture()
	want := draftContentID(t, baseline)
	for name, mutate := range map[string]func(*Input){
		"name": func(input *Input) {
			input.Name = "renamed.lua"
			for family := range input.Families {
				for span := range input.Families[family].Spans {
					input.Families[family].Spans[span].File = input.Name
				}
			}
		},
		"span": func(input *Input) {
			input.Families[0].Spans[0].StartCol, input.Families[0].Spans[0].EndCol = 2, 2
		},
		"nil-owner": func(input *Input) {
			input.Nil[0].Owner = keyspace.MakeTerm(keyspace.FamilyBody, 2)
		},
		"bool-owner": func(input *Input) {
			input.Bool[0].Owner = keyspace.MakeTerm(keyspace.FamilyBody, 2)
		},
		"bool-payload": func(input *Input) {
			input.Bool[0].Value = false
		},
		"integer-owner": func(input *Input) {
			input.Integer[0].Owner = keyspace.MakeTerm(keyspace.FamilyBody, 1)
		},
		"integer-payload": func(input *Input) {
			input.Integer[0].Value = 43
		},
		"float-owner": func(input *Input) {
			input.Float[0].Owner = keyspace.MakeTerm(keyspace.FamilyBody, 2)
		},
		"float-bits": func(input *Input) {
			input.Float[0].Bits = math.Float64bits(3.25)
		},
		"string-owner": func(input *Input) {
			input.String[0].Owner = keyspace.MakeTerm(keyspace.FamilyBody, 1)
		},
		"string-payload": func(input *Input) {
			input.String[0].Value = "changed"
		},
		"body-order": func(input *Input) {
			input.Bodies[0].Terms[0], input.Bodies[0].Terms[1] = input.Bodies[0].Terms[1], input.Bodies[0].Terms[0]
		},
		"bind-order": func(input *Input) {
			input.Binds[0].Cells[0], input.Binds[0].Cells[1] = input.Binds[0].Cells[1], input.Binds[0].Cells[0]
		},
		"formal-order": func(input *Input) {
			input.Functions[0].Formals[0], input.Functions[0].Formals[1] = input.Functions[0].Formals[1], input.Functions[0].Formals[0]
		},
		"atom-denominator": func(input *Input) {
			input.ExactAtoms = append(input.ExactAtoms, keyspace.LiteralValue{Kind: keyspace.LiteralBool, Bool: false})
		},
		"key-spelling": func(input *Input) {
			input.ExactAtoms[2] = keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "q"}
			input.Keys[0] = NameKey(keyspace.MakeTerm(keyspace.FamilyBody, 1), "q")
		},
		"control-fault": func(input *Input) {
			input.Faults[0] = ControlFault{Owner: keyspace.MakeTerm(keyspace.FamilyBody, 1), Kind: ControlFaultUndefinedGoto, Label: 0, Blocker: 0}
		},
	} {
		t.Run(name, func(t *testing.T) {
			input, _ := contentFixture()
			mutate(&input)
			if got := draftContentID(t, input); got == want {
				t.Fatalf("authored %s mutation left Source ContentID unchanged", name)
			}
		})
	}
}

func keyFaultFixture() (Input, IndexInput) {
	input, index := sourceFixture(1)
	for at := range input.Families {
		switch input.Families[at].Family {
		case keyspace.FamilyKey:
			input.Families[at].Spans = keyFaultSpans(3)
		case keyspace.FamilyLabel:
			input.Families[at].Spans = keyFaultSpans(1)
		case keyspace.FamilyControlFault:
			input.Families[at].Spans = keyFaultSpans(1)
		}
	}
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	fault := keyspace.MakeTerm(keyspace.FamilyControlFault, 1)
	input.ExactAtoms = []keyspace.LiteralValue{
		{Kind: keyspace.LiteralInteger, Integer: 1},
		{Kind: keyspace.LiteralString, String: "a"},
		{Kind: keyspace.LiteralString, String: "z"},
	}
	input.Keys = []KeyInput{NameKey(body, "z"), ListKey(body, 1), NameKey(body, "a")}
	input.Faults = []ControlFault{{Owner: body, Kind: ControlFaultDuplicateLabel, Label: keyspace.MakeTerm(keyspace.FamilyLabel, 1)}}
	for ordinal := uint32(1); ordinal <= 3; ordinal++ {
		if !appendFixturePositionTerm(&index, keyspace.MakeTerm(keyspace.FamilyKey, ordinal), keyspace.MakeTerm(keyspace.FamilyBind, 1)) {
			panic("key fixture lost Bind position root")
		}
	}
	if !appendFixturePositionTerm(&index, keyspace.MakeTerm(keyspace.FamilyLabel, 1), keyspace.MakeTerm(keyspace.FamilyBind, 1)) {
		panic("key fixture lost Label position root")
	}
	input.Bodies[0].Terms = append(input.Bodies[0].Terms, fault)
	offset := uint32(len(input.Bodies[0].Terms) - 1)
	appendCanonicalFixturePosition(&index, Position{
		Term: fault, Root: fault, Body: body, Offset: offset, Cursor: offset,
		FrontierBody: body, FrontierCursor: offset,
	})
	return input, index
}

func contentFixture() (Input, IndexInput) {
	input, index := keyFaultFixture()
	for at := range input.Families {
		switch input.Families[at].Family {
		case keyspace.FamilyCell:
			input.Families[at].Spans = keyFaultSpans(4)
		case keyspace.FamilyFloat, keyspace.FamilyString:
			input.Families[at].Spans = keyFaultSpans(1)
		}
	}
	input.Float = []FloatLiteral{{Owner: keyspace.MakeTerm(keyspace.FamilyBody, 1), Bits: math.Float64bits(3.5)}}
	input.String = []StringLiteral{{Owner: keyspace.MakeTerm(keyspace.FamilyBody, 2), Value: "original"}}
	input.Binds[0].Cells = []keyspace.Term{
		keyspace.MakeTerm(keyspace.FamilyCell, 1), keyspace.MakeTerm(keyspace.FamilyCell, 2),
	}
	input.Functions[0].Formals = []keyspace.Term{
		keyspace.MakeTerm(keyspace.FamilyCell, 3), keyspace.MakeTerm(keyspace.FamilyCell, 4),
	}
	for ordinal := uint32(3); ordinal <= 4; ordinal++ {
		if !appendFixturePositionTerm(&index, keyspace.MakeTerm(keyspace.FamilyCell, ordinal), keyspace.MakeTerm(keyspace.FamilyFunction, 1)) {
			panic("content fixture lost Function position root")
		}
	}
	if !appendFixturePositionTerm(&index, keyspace.MakeTerm(keyspace.FamilyFloat, 1), keyspace.MakeTerm(keyspace.FamilyInteger, 1)) {
		panic("content fixture lost Integer position root")
	}
	if !appendFixturePositionTerm(&index, keyspace.MakeTerm(keyspace.FamilyString, 1), keyspace.MakeTerm(keyspace.FamilyBool, 1)) {
		panic("content fixture lost Bool position root")
	}
	return input, index
}

func draftContentID(t *testing.T, input Input) identity.ContentID {
	t.Helper()
	draft, err := Build(input)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	return draft.state.authority.content
}

func finalizeSource(t *testing.T, input Input, index IndexInput) *Component {
	t.Helper()
	draft, err := Build(input)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	component, err := commitSource(draft, index)
	if err != nil {
		t.Fatalf("Finalize: %v", err)
	}
	return component
}

func withoutPosition(rows []Position, omit keyspace.Term) []Position {
	result := make([]Position, 0, len(rows))
	for _, row := range rows {
		if row.Term != omit {
			result = append(result, row)
		}
	}
	return result
}

func withoutDirectTerm(terms []keyspace.Term, omit keyspace.Term) []keyspace.Term {
	result := make([]keyspace.Term, 0, len(terms))
	for _, term := range terms {
		if term != omit {
			result = append(result, term)
		}
	}
	return result
}

func keyFaultSpans(count int) []Span {
	result := make([]Span, count)
	for index := range result {
		result[index] = Span{File: "fixture.lua", StartLine: uint32(index + 1), StartCol: 1, EndLine: uint32(index + 1), EndCol: 1}
	}
	return result
}
