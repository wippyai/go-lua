package containment

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/flow/internal/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/analysis/program/static"
	staticcontracts "github.com/wippyai/go-lua/analysis/program/static/contracts"
	staticoperators "github.com/wippyai/go-lua/analysis/program/static/operators"
	staticquery "github.com/wippyai/go-lua/analysis/program/static/query"
	statictypes "github.com/wippyai/go-lua/analysis/program/static/types"
)

// buildStaticCallChain creates one Call-owned type argument with a long
// Optional chain beneath it. The fixture is a real Static owner proof, not a
// hand-built relation, so this law exercises the same LocalContainment
// boundary used by emitStaticMarks.
func buildStaticCallChain(t *testing.T, width uint32) (staticquery.LocalContainment, [keyspace.FamilyCount]uint32) {
	t.Helper()
	counts := [keyspace.FamilyCount]uint32{
		keyspace.FamilyTypePrimitive: 1,
		keyspace.FamilyTypeOptional:  width,
		keyspace.FamilyCall:          1,
	}
	primitive := keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1)
	optionals := make([]statictypes.Optional, width)
	for ordinal := uint32(1); ordinal <= width; ordinal++ {
		inner := primitive
		if ordinal > 1 {
			inner = keyspace.MakeTerm(keyspace.FamilyTypeOptional, ordinal-1)
		}
		optionals[ordinal-1] = statictypes.Optional{Inner: inner}
	}
	input := static.Input{
		Counts: counts,
		Types: statictypes.Input{
			Primitive: []statictypes.Primitive{{Kind: statictypes.PrimitiveAny}},
			Optional:  optionals,
		},
		Contracts: staticcontracts.Input{Call: []staticcontracts.CallContract{{
			TypeArguments: []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyTypeOptional, width)},
		}}},
	}
	draft, err := static.Build(input)
	if err != nil {
		t.Fatalf("static.Build(%d): %v", width, err)
	}
	finalizer, err := draft.Finalizer()
	if err != nil {
		t.Fatalf("static.Finalizer(%d): %v", width, err)
	}
	t.Cleanup(func() { _ = finalizer.Abort() })
	local := finalizer.View().LocalContainment()
	if got, want := local.Count(), int(width)+1; got != want {
		t.Fatalf("LocalContainment.Count(%d) = %d, want %d", width, got, want)
	}
	return local, counts
}

func runStaticCallOwnerChain(t *testing.T, local staticquery.LocalContainment, counts [keyspace.FamilyCount]uint32) int {
	t.Helper()
	marks := newStaticMarkBits(counts)
	if err := markCallOwnedStaticTypes(local, counts, &marks); err != nil {
		t.Fatalf("markCallOwnedStaticTypes: %v", err)
	}
	return len(marks.marked)
}

// TestStaticCallOwnerDeepChainScales protects the recurrence law of the
// owner walk. A long acyclic chain must memoize each local term once; losing
// the shared state header turns the same chain into a fresh O(n)-byte state
// allocation for every ancestor and therefore quadratic allocation growth.
// The assertion is deliberately a relative scaling law, not a machine-specific
// time or absolute-memory budget.
func TestProveStaticMarksStorageIdentitiesAndReferenceExclusions(t *testing.T) {
	counts := countsFor(
		c(keyspace.FamilyBody, 3),
		c(keyspace.FamilyKey, 2),
		c(keyspace.FamilyCell, 9),
		c(keyspace.FamilyRead, 4),
		c(keyspace.FamilyLensExact, 1),
		c(keyspace.FamilyValues, 6),
		c(keyspace.FamilyBind, 2),
		c(keyspace.FamilyAssign, 1),
		c(keyspace.FamilyWrite, 1),
		c(keyspace.FamilyFunction, 1),
		c(keyspace.FamilyCall, 1),
		c(keyspace.FamilyLoop, 1),
		c(keyspace.FamilyTable, 1),
		c(keyspace.FamilyTableField, 1),
		c(keyspace.FamilyTypeOf, 1),
	)
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	functionBody := keyspace.MakeTerm(keyspace.FamilyBody, 2)
	loopBody := keyspace.MakeTerm(keyspace.FamilyBody, 3)
	scopeCell := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	bindCell := keyspace.MakeTerm(keyspace.FamilyCell, 2)
	formalCell := keyspace.MakeTerm(keyspace.FamilyCell, 3)
	varargCell := keyspace.MakeTerm(keyspace.FamilyCell, 4)
	captureInner := keyspace.MakeTerm(keyspace.FamilyCell, 5)
	captureOuter := keyspace.MakeTerm(keyspace.FamilyCell, 6)
	writeCell := keyspace.MakeTerm(keyspace.FamilyCell, 7)
	receiverCell := keyspace.MakeTerm(keyspace.FamilyCell, 8)
	loopCell := keyspace.MakeTerm(keyspace.FamilyCell, 9)
	receiverRead := keyspace.MakeTerm(keyspace.FamilyRead, 1)
	baseRead := keyspace.MakeTerm(keyspace.FamilyRead, 2)
	loopRead := keyspace.MakeTerm(keyspace.FamilyRead, 3)
	fieldRead := keyspace.MakeTerm(keyspace.FamilyRead, 4)
	lens := keyspace.MakeTerm(keyspace.FamilyLensExact, 1)
	bindValues := keyspace.MakeTerm(keyspace.FamilyValues, 1)
	loopValues := keyspace.MakeTerm(keyspace.FamilyValues, 2)
	callValues := keyspace.MakeTerm(keyspace.FamilyValues, 3)
	assignValues := keyspace.MakeTerm(keyspace.FamilyValues, 4)
	fieldValues := keyspace.MakeTerm(keyspace.FamilyValues, 5)
	bind2Values := keyspace.MakeTerm(keyspace.FamilyValues, 6)
	function := keyspace.MakeTerm(keyspace.FamilyFunction, 1)
	bind := keyspace.MakeTerm(keyspace.FamilyBind, 1)
	bind2 := keyspace.MakeTerm(keyspace.FamilyBind, 2)
	loop := keyspace.MakeTerm(keyspace.FamilyLoop, 1)
	assign := keyspace.MakeTerm(keyspace.FamilyAssign, 1)
	write := keyspace.MakeTerm(keyspace.FamilyWrite, 1)
	call := keyspace.MakeTerm(keyspace.FamilyCall, 1)
	table := keyspace.MakeTerm(keyspace.FamilyTable, 1)
	field := keyspace.MakeTerm(keyspace.FamilyTableField, 1)
	lensKey := keyspace.MakeTerm(keyspace.FamilyKey, 1)
	fieldKey := keyspace.MakeTerm(keyspace.FamilyKey, 2)
	typeOf := keyspace.MakeTerm(keyspace.FamilyTypeOf, 1)

	flow := authored.Input{
		Values: authored.ValuesInput{
			Rows: []authored.Value{
				{Owner: functionBody, Fixed: authored.Range{}},
				{Owner: functionBody, Fixed: authored.Range{End: 1}},
				{Owner: functionBody, Fixed: authored.Range{Start: 1, End: 2}},
				{Owner: functionBody, Fixed: authored.Range{Start: 2, End: 2}},
				{Owner: functionBody, Fixed: authored.Range{Start: 2, End: 3}},
				{Owner: body, Fixed: authored.Range{Start: 3, End: 3}},
			},
			Terms: []keyspace.Term{loopRead, table, fieldRead},
		},
		Storage: authored.StorageInput{
			Cells: []authored.Cell{
				{Kind: authored.CellLocal, Body: body},
				{Kind: authored.CellLocal, Body: functionBody},
				{Kind: authored.CellLocal, Body: functionBody},
				{Kind: authored.CellLocal, Body: functionBody},
				{Kind: authored.CellLocal, Body: functionBody},
				{Kind: authored.CellLocal, Body: body},
				{Kind: authored.CellLocal, Body: body},
				{Kind: authored.CellLocal, Body: body},
				{Kind: authored.CellLocal, Body: loopBody},
			},
			Reads: []authored.Read{
				{Owner: functionBody, Source: lens},
				{Owner: functionBody, Source: receiverCell},
				{Owner: functionBody, Source: scopeCell},
				{Owner: functionBody, Source: writeCell},
			},
			Binds: []authored.Bind{
				{Owner: functionBody, Values: bindValues},
				{Owner: body, Values: bind2Values},
			},
			Assigns: []authored.Assign{{Owner: functionBody, Values: assignValues}},
			Writes:  []authored.Write{{Assign: assign, Target: writeCell}},
		},
		Access: authored.AccessInput{Exact: []authored.ExactLens{{
			Owner: functionBody, Base: baseRead, Source: lensKey, Kind: kind.FieldName,
		}}},
		Functions: authored.FunctionsInput{
			Rows:     []authored.Function{{Owner: body, Body: functionBody, Vararg: varargCell, Captures: authored.Range{End: 1}}},
			Captures: []authored.Capture{{Inner: captureInner, Outer: captureOuter}},
		},
		Calls: []authored.Call{{Owner: functionBody, Callee: receiverRead, Receiver: baseRead, Actuals: callValues}},
		Tables: authored.TablesInput{
			Rows:   []authored.Table{{Owner: functionBody, Fields: authored.Range{End: 1}}},
			Fields: []authored.Field{{Table: table, Key: fieldKey, Values: fieldValues, Kind: kind.FieldName}},
			Order:  []keyspace.Term{field},
		},
		Control: authored.ControlInput{
			Loops: []authored.Loop{{Owner: functionBody, Body: loopBody, Kind: kind.LoopGenericFor, Control: loopValues, Cells: authored.Range{End: 1}}},
			Cells: []keyspace.Term{loopCell},
		},
	}
	staticInput := static.Input{
		Operators: staticoperators.Input{TypeOf: []staticoperators.TypeOf{{Scope: scopeCell, Operand: function}}},
		Contracts: staticcontracts.Input{Function: []staticcontracts.FunctionContract{{}}, Call: []staticcontracts.CallContract{{}}},
	}
	fixture := newProofFixture(t, proofSpec{
		counts: counts,
		rows:   [][]keyspace.Term{{bind2}, {bind, loop, assign, call}, nil},
		flow:   flow,
		static: staticInput,
		exacts: []keyspace.LiteralValue{{Kind: keyspace.LiteralString, String: "field"}},
		keys: []source.KeyInput{
			source.NameKey(functionBody, "field"),
			source.NameKey(functionBody, "field"),
		},
		binds: []source.BindCells{
			{Bind: bind, Cells: []keyspace.Term{bindCell}},
			{Bind: bind2, Cells: []keyspace.Term{scopeCell, captureOuter, writeCell, receiverCell}},
		},
		formals: []source.FunctionFormals{{Function: function, Formals: []keyspace.Term{formalCell}}},
		module:  emptyModule(t),
	})
	result, err := fixture.prove()
	if err != nil {
		t.Fatalf("Prove: %v", err)
	}

	for _, term := range []keyspace.Term{
		typeOf, function, functionBody, bind, bindValues, loop, loopBody,
		loopValues, loopRead, assign, assignValues, call, callValues, table, lensKey, fieldKey, fieldValues,
		receiverRead, lens, baseRead, bindCell, formalCell, varargCell, captureInner, loopCell,
	} {
		if !result.Static(term) {
			t.Fatalf("expected static mark for %v", term)
		}
	}
	for _, term := range []keyspace.Term{captureOuter, write, writeCell, field} {
		if result.Static(term) {
			t.Fatalf("unexpected static mark for reusable reference %v", term)
		}
	}
}
