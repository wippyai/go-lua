package evaluation

import (
	"errors"
	"testing"

	"github.com/wippyai/go-lua/analysis/program/flow/internal/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

var errUnexpectedLeaf = errors.New("unexpected leaf event")

func mustView(t *testing.T, input authored.Input) authored.View {
	t.Helper()
	draft, err := authored.Build(input)
	if err != nil {
		t.Fatal(err)
	}
	finalizer, err := draft.Finalizer()
	if err != nil {
		t.Fatal(err)
	}
	view, err := finalizer.Commit()
	if err != nil {
		t.Fatal(err)
	}
	return view
}

func term(family keyspace.Family, ordinal uint32) keyspace.Term {
	return keyspace.MakeTerm(family, ordinal)
}

func events(t *testing.T, session *Session, roots ...keyspace.Term) []keyspace.Term {
	t.Helper()
	var result []keyspace.Term
	for _, root := range roots {
		if err := session.Start(root); err != nil {
			t.Fatal(err)
		}
		for {
			event, ok, err := session.Next()
			if err != nil {
				t.Fatal(err)
			}
			if !ok {
				break
			}
			result = append(result, event.Select)
		}
	}
	return result
}

func assertTerms(t *testing.T, got, want []keyspace.Term) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("event count = %d, want %d (%v)", len(got), len(want), want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("event[%d] = %v, want %v (all=%v)", index, got[index], want, got)
		}
	}
}

func TestRightNestedSelectUsesSemanticPostfix(t *testing.T) {
	body := term(keyspace.FamilyBody, 1)
	inner, outer := term(keyspace.FamilySelect, 1), term(keyspace.FamilySelect, 2)
	nilValue := term(keyspace.FamilyNil, 1)
	view := mustView(t, authored.Input{
		Counts: [keyspace.FamilyCount]uint32{
			keyspace.FamilyBody: 1, keyspace.FamilyNil: 1,
			keyspace.FamilyValues: 1, keyspace.FamilySelect: 2,
		},
		Values: authored.ValuesInput{
			Rows:  []authored.Value{{Owner: body, Fixed: authored.Range{End: 1}}},
			Terms: []keyspace.Term{outer},
		},
		Operators: authored.OperatorsInput{Selects: []authored.Select{
			{Owner: body, Op: kind.SelectAnd, Left: nilValue, Right: nilValue},
			{Owner: body, Op: kind.SelectOr, Left: nilValue, Right: inner},
		}},
	})
	session, err := New(view)
	if err != nil {
		t.Fatal(err)
	}
	assertTerms(t, events(t, session, outer), []keyspace.Term{outer, inner})
}

func TestLeftNestedSelectVisitsInnerBeforeOuter(t *testing.T) {
	body := term(keyspace.FamilyBody, 1)
	inner, outer := term(keyspace.FamilySelect, 1), term(keyspace.FamilySelect, 2)
	nilValue := term(keyspace.FamilyNil, 1)
	view := mustView(t, authored.Input{
		Counts: [keyspace.FamilyCount]uint32{
			keyspace.FamilyBody: 1, keyspace.FamilyNil: 1,
			keyspace.FamilyValues: 1, keyspace.FamilySelect: 2,
		},
		Values: authored.ValuesInput{
			Rows:  []authored.Value{{Owner: body, Fixed: authored.Range{End: 1}}},
			Terms: []keyspace.Term{outer},
		},
		Operators: authored.OperatorsInput{Selects: []authored.Select{
			{Owner: body, Op: kind.SelectAnd, Left: nilValue, Right: nilValue},
			{Owner: body, Op: kind.SelectOr, Left: inner, Right: nilValue},
		}},
	})
	session, err := New(view)
	if err != nil {
		t.Fatal(err)
	}
	assertTerms(t, events(t, session, outer), []keyspace.Term{inner, outer})
}

func TestBinaryAndValuesAreLeftToRight(t *testing.T) {
	body := term(keyspace.FamilyBody, 1)
	left, right := term(keyspace.FamilySelect, 1), term(keyspace.FamilySelect, 2)
	binary, values := term(keyspace.FamilyBinary, 1), term(keyspace.FamilyValues, 1)
	nilValue := term(keyspace.FamilyNil, 1)
	view := mustView(t, authored.Input{
		Counts: [keyspace.FamilyCount]uint32{
			keyspace.FamilyBody: 1, keyspace.FamilyNil: 1,
			keyspace.FamilyValues: 1, keyspace.FamilyBinary: 1,
			keyspace.FamilySelect: 2,
		},
		Values: authored.ValuesInput{
			Rows:  []authored.Value{{Owner: body, Fixed: authored.Range{End: 1}}},
			Terms: []keyspace.Term{binary},
		},
		Operators: authored.OperatorsInput{
			Binaries: []authored.Binary{{Owner: body, Op: kind.BinaryAdd, Left: left, Right: right}},
			Selects: []authored.Select{
				{Owner: body, Op: kind.SelectAnd, Left: nilValue, Right: nilValue},
				{Owner: body, Op: kind.SelectOr, Left: nilValue, Right: nilValue},
			},
		},
	})
	session, err := New(view)
	if err != nil {
		t.Fatal(err)
	}
	assertTerms(t, events(t, session, values), []keyspace.Term{left, right})

	// A second root cannot silently re-walk a composite occurrence. This is
	// the one-session assembly law, not a per-root scratch walk.
	if err := session.Start(binary); err == nil {
		t.Fatal("cross-root composite alias was accepted")
	}
}

func TestCallEvaluatesCalleeThenActuals(t *testing.T) {
	body := term(keyspace.FamilyBody, 1)
	callee, argument := term(keyspace.FamilySelect, 1), term(keyspace.FamilySelect, 2)
	values, call := term(keyspace.FamilyValues, 1), term(keyspace.FamilyCall, 1)
	nilValue := term(keyspace.FamilyNil, 1)
	view := mustView(t, authored.Input{
		Counts: [keyspace.FamilyCount]uint32{
			keyspace.FamilyBody: 1, keyspace.FamilyNil: 1,
			keyspace.FamilyValues: 1, keyspace.FamilySelect: 2,
			keyspace.FamilyCall: 1,
		},
		Values: authored.ValuesInput{
			Rows:  []authored.Value{{Owner: body, Fixed: authored.Range{End: 1}}},
			Terms: []keyspace.Term{argument},
		},
		Calls: []authored.Call{{Owner: body, Callee: callee, Actuals: values}},
		Operators: authored.OperatorsInput{Selects: []authored.Select{
			{Owner: body, Op: kind.SelectAnd, Left: nilValue, Right: nilValue},
			{Owner: body, Op: kind.SelectOr, Left: nilValue, Right: nilValue},
		}},
	})
	session, err := New(view)
	if err != nil {
		t.Fatal(err)
	}
	assertTerms(t, events(t, session, call), []keyspace.Term{callee, argument})
}

func TestDynamicLensAndReadEvaluateBaseThenKey(t *testing.T) {
	body := term(keyspace.FamilyBody, 1)
	base, key := term(keyspace.FamilySelect, 1), term(keyspace.FamilySelect, 2)
	lens, read := term(keyspace.FamilyLensKey, 1), term(keyspace.FamilyRead, 1)
	nilValue := term(keyspace.FamilyNil, 1)
	view := mustView(t, authored.Input{
		Counts: [keyspace.FamilyCount]uint32{
			keyspace.FamilyBody: 1, keyspace.FamilyNil: 1,
			keyspace.FamilySelect: 2, keyspace.FamilyLensKey: 1,
			keyspace.FamilyRead: 1,
		},
		Access:  authored.AccessInput{Dynamic: []authored.DynamicLens{{Owner: body, Base: base, Key: key}}},
		Storage: authored.StorageInput{Reads: []authored.Read{{Owner: body, Source: lens}}},
		Operators: authored.OperatorsInput{Selects: []authored.Select{
			{Owner: body, Op: kind.SelectAnd, Left: nilValue, Right: nilValue},
			{Owner: body, Op: kind.SelectOr, Left: nilValue, Right: nilValue},
		}},
	})
	session, err := New(view)
	if err != nil {
		t.Fatal(err)
	}
	assertTerms(t, events(t, session, read), []keyspace.Term{base, key})
}

func TestAssignEvaluatesOrderedTargetsBeforeRHS(t *testing.T) {
	body := term(keyspace.FamilyBody, 1)
	keyOne, keyTwo, rhs := term(keyspace.FamilySelect, 1), term(keyspace.FamilySelect, 2), term(keyspace.FamilySelect, 3)
	lensOne, lensTwo := term(keyspace.FamilyLensKey, 1), term(keyspace.FamilyLensKey, 2)
	assign, values := term(keyspace.FamilyAssign, 1), term(keyspace.FamilyValues, 1)
	nilValue := term(keyspace.FamilyNil, 1)
	view := mustView(t, authored.Input{
		Counts: [keyspace.FamilyCount]uint32{
			keyspace.FamilyBody: 1, keyspace.FamilyNil: 1,
			keyspace.FamilyValues: 1, keyspace.FamilySelect: 3,
			keyspace.FamilyLensKey: 2, keyspace.FamilyAssign: 1,
			keyspace.FamilyWrite: 2,
		},
		Values: authored.ValuesInput{
			Rows:  []authored.Value{{Owner: body, Fixed: authored.Range{End: 1}}},
			Terms: []keyspace.Term{rhs},
		},
		Access: authored.AccessInput{Dynamic: []authored.DynamicLens{
			{Owner: body, Base: nilValue, Key: keyOne},
			{Owner: body, Base: nilValue, Key: keyTwo},
		}},
		Storage: authored.StorageInput{
			Assigns: []authored.Assign{{Owner: body, Values: values}},
			Writes:  []authored.Write{{Assign: assign, Target: lensOne}, {Assign: assign, Target: lensTwo}},
		},
		Operators: authored.OperatorsInput{Selects: []authored.Select{
			{Owner: body, Op: kind.SelectAnd, Left: nilValue, Right: nilValue},
			{Owner: body, Op: kind.SelectOr, Left: nilValue, Right: nilValue},
			{Owner: body, Op: kind.SelectAnd, Left: nilValue, Right: nilValue},
		}},
	})
	session, err := New(view)
	if err != nil {
		t.Fatal(err)
	}
	assertTerms(t, events(t, session, assign), []keyspace.Term{keyOne, keyTwo, rhs})
}

func TestExactLensUsesBaseAndStaticKeyIsNotWalked(t *testing.T) {
	body := term(keyspace.FamilyBody, 1)
	base, lens, read := term(keyspace.FamilySelect, 1), term(keyspace.FamilyLensExact, 1), term(keyspace.FamilyRead, 1)
	key := term(keyspace.FamilyKey, 1)
	nilValue := term(keyspace.FamilyNil, 1)
	view := mustView(t, authored.Input{
		Counts: [keyspace.FamilyCount]uint32{
			keyspace.FamilyBody: 1, keyspace.FamilyNil: 1, keyspace.FamilyKey: 1,
			keyspace.FamilySelect: 1, keyspace.FamilyLensExact: 1, keyspace.FamilyRead: 1,
		},
		Access:    authored.AccessInput{Exact: []authored.ExactLens{{Owner: body, Base: base, Source: key, Kind: kind.FieldName}}},
		Storage:   authored.StorageInput{Reads: []authored.Read{{Owner: body, Source: lens}}},
		Operators: authored.OperatorsInput{Selects: []authored.Select{{Owner: body, Op: kind.SelectAnd, Left: nilValue, Right: nilValue}}},
	})
	session, err := New(view)
	if err != nil {
		t.Fatal(err)
	}
	assertTerms(t, events(t, session, read), []keyspace.Term{base})
}

func TestMethodCallUsesCalleeLensBaseOnce(t *testing.T) {
	body := term(keyspace.FamilyBody, 1)
	receiver := term(keyspace.FamilySelect, 1)
	lens, read, actuals, call := term(keyspace.FamilyLensExact, 1), term(keyspace.FamilyRead, 1), term(keyspace.FamilyValues, 1), term(keyspace.FamilyCall, 1)
	key, nilValue := term(keyspace.FamilyKey, 1), term(keyspace.FamilyNil, 1)
	view := mustView(t, authored.Input{
		Counts: [keyspace.FamilyCount]uint32{
			keyspace.FamilyBody: 1, keyspace.FamilyNil: 1, keyspace.FamilyKey: 1,
			keyspace.FamilySelect: 1, keyspace.FamilyLensExact: 1, keyspace.FamilyRead: 1,
			keyspace.FamilyValues: 1, keyspace.FamilyCall: 1,
		},
		Values:    authored.ValuesInput{Rows: []authored.Value{{Owner: body, Fixed: authored.Range{End: 1}}}, Terms: []keyspace.Term{nilValue}},
		Access:    authored.AccessInput{Exact: []authored.ExactLens{{Owner: body, Base: receiver, Source: key, Kind: kind.FieldName}}},
		Storage:   authored.StorageInput{Reads: []authored.Read{{Owner: body, Source: lens}}},
		Calls:     []authored.Call{{Owner: body, Callee: read, Receiver: receiver, Actuals: actuals}},
		Operators: authored.OperatorsInput{Selects: []authored.Select{{Owner: body, Op: kind.SelectAnd, Left: nilValue, Right: nilValue}}},
	})
	session, err := New(view)
	if err != nil {
		t.Fatal(err)
	}
	assertTerms(t, events(t, session, call), []keyspace.Term{receiver})
}

func TestValuesTailIsAfterFixedMembers(t *testing.T) {
	body := term(keyspace.FamilyBody, 1)
	fixedSelect, tailSelect := term(keyspace.FamilySelect, 1), term(keyspace.FamilySelect, 2)
	call := term(keyspace.FamilyCall, 1)
	values, actuals := term(keyspace.FamilyValues, 1), term(keyspace.FamilyValues, 2)
	nilValue := term(keyspace.FamilyNil, 1)
	view := mustView(t, authored.Input{
		Counts: [keyspace.FamilyCount]uint32{
			keyspace.FamilyBody: 1, keyspace.FamilyNil: 1, keyspace.FamilySelect: 2,
			keyspace.FamilyCall: 1, keyspace.FamilyValues: 2,
		},
		Values: authored.ValuesInput{Rows: []authored.Value{
			{Owner: body, Fixed: authored.Range{End: 1}, Tail: call},
			{Owner: body, Fixed: authored.Range{Start: 1, End: 2}},
		}, Terms: []keyspace.Term{fixedSelect, nilValue}},
		Calls: []authored.Call{{Owner: body, Callee: tailSelect, Actuals: actuals}},
		Operators: authored.OperatorsInput{Selects: []authored.Select{
			{Owner: body, Op: kind.SelectAnd, Left: nilValue, Right: nilValue},
			{Owner: body, Op: kind.SelectOr, Left: nilValue, Right: nilValue},
		}},
	})
	session, err := New(view)
	if err != nil {
		t.Fatal(err)
	}
	assertTerms(t, events(t, session, values), []keyspace.Term{fixedSelect, tailSelect})
}

func TestSessionSupportsPositiveMultipleRoots(t *testing.T) {
	body := term(keyspace.FamilyBody, 1)
	first, second := term(keyspace.FamilySelect, 1), term(keyspace.FamilySelect, 2)
	valuesOne, valuesTwo := term(keyspace.FamilyValues, 1), term(keyspace.FamilyValues, 2)
	nilValue := term(keyspace.FamilyNil, 1)
	view := mustView(t, authored.Input{
		Counts: [keyspace.FamilyCount]uint32{keyspace.FamilyBody: 1, keyspace.FamilyNil: 1, keyspace.FamilySelect: 2, keyspace.FamilyValues: 2},
		Values: authored.ValuesInput{Rows: []authored.Value{
			{Owner: body, Fixed: authored.Range{End: 1}},
			{Owner: body, Fixed: authored.Range{Start: 1, End: 2}},
		}, Terms: []keyspace.Term{first, second}},
		Operators: authored.OperatorsInput{Selects: []authored.Select{
			{Owner: body, Op: kind.SelectAnd, Left: nilValue, Right: nilValue},
			{Owner: body, Op: kind.SelectOr, Left: nilValue, Right: nilValue},
		}},
	})
	session, err := New(view)
	if err != nil {
		t.Fatal(err)
	}
	assertTerms(t, events(t, session, valuesOne, valuesTwo), []keyspace.Term{first, second})
}

func TestControlTopologyIsNotAnEvaluationRoot(t *testing.T) {
	body, whenTrue, whenFalse := term(keyspace.FamilyBody, 1), term(keyspace.FamilyBody, 2), term(keyspace.FamilyBody, 3)
	branch, nilValue := term(keyspace.FamilyBranch, 1), term(keyspace.FamilyNil, 1)
	view := mustView(t, authored.Input{
		Counts:  [keyspace.FamilyCount]uint32{keyspace.FamilyBody: 3, keyspace.FamilyNil: 1, keyspace.FamilyBranch: 1},
		Control: authored.ControlInput{Branches: []authored.Branch{{Owner: body, Condition: nilValue, WhenTrue: whenTrue, WhenFalse: whenFalse}}},
	})
	session, err := New(view)
	if err != nil {
		t.Fatal(err)
	}
	if err := session.Start(branch); err == nil {
		t.Fatal("Branch was accepted as an evaluation root")
	}

	loopBody, loop := term(keyspace.FamilyBody, 2), term(keyspace.FamilyLoop, 1)
	loopView := mustView(t, authored.Input{
		Counts: [keyspace.FamilyCount]uint32{keyspace.FamilyBody: 2, keyspace.FamilyNil: 1, keyspace.FamilyLoop: 1},
		Control: authored.ControlInput{Loops: []authored.Loop{{
			Owner: body, Body: loopBody, Kind: kind.LoopWhile, Control: nilValue,
		}}},
	})
	loopSession, err := New(loopView)
	if err != nil {
		t.Fatal(err)
	}
	if err := loopSession.Start(loop); err == nil {
		t.Fatal("Loop was accepted as an evaluation root")
	}
}

func TestCrossBodyExpressionEdgeFailsClosed(t *testing.T) {
	bodyOne, bodyTwo := term(keyspace.FamilyBody, 1), term(keyspace.FamilyBody, 2)
	left, root := term(keyspace.FamilySelect, 1), term(keyspace.FamilySelect, 2)
	nilValue := term(keyspace.FamilyNil, 1)
	view := mustView(t, authored.Input{
		Counts: [keyspace.FamilyCount]uint32{
			keyspace.FamilyBody: 2, keyspace.FamilyNil: 1, keyspace.FamilySelect: 2,
		},
		Operators: authored.OperatorsInput{Selects: []authored.Select{
			{Owner: bodyTwo, Op: kind.SelectAnd, Left: nilValue, Right: nilValue},
			{Owner: bodyOne, Op: kind.SelectOr, Left: left, Right: nilValue},
		}},
	})
	session, err := New(view)
	if err != nil {
		t.Fatal(err)
	}
	if err := session.Start(root); err != nil {
		t.Fatal(err)
	}
	if _, _, err := session.Next(); err == nil {
		t.Fatal("cross-Body edge was accepted")
	}
}

func TestTableEvaluatesDynamicKeyThenFieldValues(t *testing.T) {
	body := term(keyspace.FamilyBody, 1)
	keyOne, valueOne, keyTwo := term(keyspace.FamilySelect, 1), term(keyspace.FamilySelect, 2), term(keyspace.FamilySelect, 3)
	table, fieldOne, fieldTwo := term(keyspace.FamilyTable, 1), term(keyspace.FamilyTableField, 1), term(keyspace.FamilyTableField, 2)
	valuesOne, valuesTwo := term(keyspace.FamilyValues, 1), term(keyspace.FamilyValues, 2)
	nilValue := term(keyspace.FamilyNil, 1)
	view := mustView(t, authored.Input{
		Counts: [keyspace.FamilyCount]uint32{
			keyspace.FamilyBody: 1, keyspace.FamilyNil: 1,
			keyspace.FamilyValues: 2, keyspace.FamilySelect: 3,
			keyspace.FamilyTable: 1, keyspace.FamilyTableField: 2,
		},
		Values: authored.ValuesInput{
			Rows: []authored.Value{
				{Owner: body, Fixed: authored.Range{End: 1}},
				{Owner: body, Fixed: authored.Range{Start: 1, End: 2}},
			},
			Terms: []keyspace.Term{valueOne, nilValue},
		},
		Tables: authored.TablesInput{
			Rows:   []authored.Table{{Owner: body, Fields: authored.Range{End: 2}}},
			Fields: []authored.Field{{Table: table, Key: keyOne, Values: valuesOne, Kind: kind.FieldKey}, {Table: table, Key: keyTwo, Values: valuesTwo, Kind: kind.FieldKey}},
			Order:  []keyspace.Term{fieldOne, fieldTwo},
		},
		Operators: authored.OperatorsInput{Selects: []authored.Select{
			{Owner: body, Op: kind.SelectAnd, Left: nilValue, Right: nilValue},
			{Owner: body, Op: kind.SelectOr, Left: nilValue, Right: nilValue},
			{Owner: body, Op: kind.SelectAnd, Left: nilValue, Right: nilValue},
		}},
	})
	session, err := New(view)
	if err != nil {
		t.Fatal(err)
	}
	assertTerms(t, events(t, session, table), []keyspace.Term{keyOne, valueOne, keyTwo})
}

func TestCycleAndDuplicateCompositeFailClosed(t *testing.T) {
	body := term(keyspace.FamilyBody, 1)
	nilValue := term(keyspace.FamilyNil, 1)
	selectTerm := term(keyspace.FamilySelect, 1)
	cycle := mustView(t, authored.Input{
		Counts:    [keyspace.FamilyCount]uint32{keyspace.FamilyBody: 1, keyspace.FamilyNil: 1, keyspace.FamilySelect: 1},
		Operators: authored.OperatorsInput{Selects: []authored.Select{{Owner: body, Op: kind.SelectAnd, Left: selectTerm, Right: nilValue}}},
	})
	session, err := New(cycle)
	if err != nil {
		t.Fatal(err)
	}
	if err := session.Start(selectTerm); err != nil {
		t.Fatal(err)
	}
	if _, _, err := session.Next(); err == nil {
		t.Fatal("cycle was accepted")
	}

	left, right := term(keyspace.FamilySelect, 1), term(keyspace.FamilySelect, 2)
	duplicate := mustView(t, authored.Input{
		Counts: [keyspace.FamilyCount]uint32{keyspace.FamilyBody: 1, keyspace.FamilyNil: 1, keyspace.FamilySelect: 2},
		Operators: authored.OperatorsInput{Selects: []authored.Select{
			{Owner: body, Op: kind.SelectAnd, Left: nilValue, Right: nilValue},
			{Owner: body, Op: kind.SelectOr, Left: left, Right: left},
		}},
	})
	session, err = New(duplicate)
	if err != nil {
		t.Fatal(err)
	}
	if err := session.Start(right); err != nil {
		t.Fatal(err)
	}
	for {
		_, ok, nextErr := session.Next()
		if nextErr != nil {
			return
		}
		if !ok {
			t.Fatal("duplicate composite was accepted")
		}
	}
}

func TestDeepSelectChainIsIterative(t *testing.T) {
	const depth = 4096
	body, nilValue := term(keyspace.FamilyBody, 1), term(keyspace.FamilyNil, 1)
	selects := make([]authored.Select, depth)
	for index := range selects {
		selects[index] = authored.Select{Owner: body, Op: kind.SelectAnd, Left: nilValue, Right: nilValue}
		if index != 0 {
			selects[index].Right = term(keyspace.FamilySelect, uint32(index))
		}
	}
	view := mustView(t, authored.Input{
		Counts:    [keyspace.FamilyCount]uint32{keyspace.FamilyBody: 1, keyspace.FamilyNil: 1, keyspace.FamilySelect: depth},
		Operators: authored.OperatorsInput{Selects: selects},
	})
	session, err := New(view)
	if err != nil {
		t.Fatal(err)
	}
	if err := session.Start(term(keyspace.FamilySelect, depth)); err != nil {
		t.Fatal(err)
	}
	count := 0
	for {
		_, ok, nextErr := session.Next()
		if nextErr != nil {
			t.Fatal(nextErr)
		}
		if !ok {
			break
		}
		count++
	}
	if count != depth {
		t.Fatalf("deep event count = %d, want %d", count, depth)
	}
}

func TestSessionDoesNotAllocateDenominatorsPerRoot(t *testing.T) {
	nilValue := term(keyspace.FamilyNil, 1)
	view := mustView(t, authored.Input{Counts: [keyspace.FamilyCount]uint32{
		keyspace.FamilyBody: 1, keyspace.FamilyNil: 1,
	}})
	session, err := New(view)
	if err != nil {
		t.Fatal(err)
	}
	if err := session.Start(nilValue); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := session.Next(); err != nil || ok {
		t.Fatalf("warm leaf walk = ok %v, err %v", ok, err)
	}
	var failed error
	allocs := testing.AllocsPerRun(100, func() {
		if failed != nil {
			return
		}
		if err := session.Start(nilValue); err != nil {
			failed = err
			return
		}
		if _, ok, err := session.Next(); err != nil || ok {
			failed = err
			if failed == nil {
				failed = errUnexpectedLeaf
			}
		}
	})
	if failed != nil {
		t.Fatal(failed)
	}
	if allocs != 0 {
		t.Fatalf("per-root allocations = %v, want 0 after session construction", allocs)
	}
}
