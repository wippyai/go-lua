package position

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/program/flow/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/binding"
	"github.com/wippyai/go-lua/analysis/program/flow/body"
	"github.com/wippyai/go-lua/analysis/program/flow/containment"
	"github.com/wippyai/go-lua/analysis/program/flow/control"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/flowtest"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/flow/outcome"
	"github.com/wippyai/go-lua/analysis/program/imports"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/analysis/program/static"
	staticcontracts "github.com/wippyai/go-lua/analysis/program/static/contracts"
	staticquery "github.com/wippyai/go-lua/analysis/program/static/query"
)

type positionFixture struct {
	preimage source.Preimage
	flow     authored.View
	bodies   *body.Result
	forest   *containment.Result
	outcomes *outcome.Result
	entry    keyspace.Term

	sourceFinalize source.Finalizer
	flowFinalize   authored.Finalizer
	staticView     staticquery.View
	moduleFinalize imports.Finalizer
}

type positionSpec struct {
	counts     [keyspace.FamilyCount]uint32
	rows       [][]keyspace.Term
	flow       authored.Input
	static     static.Input
	nilOwners  []keyspace.Term
	ints       []source.IntegerLiteral
	faults     []source.ControlFault
	exactAtoms []keyspace.LiteralValue
	module     imports.Input
}

func openPositionFixture(t *testing.T, spec positionSpec) *positionFixture {
	t.Helper()
	counts := spec.counts
	if counts[keyspace.FamilyBody] == 0 {
		t.Fatal("position fixture requires an Entry Body")
	}
	entry := keyspace.MakeTerm(keyspace.FamilyBody, 1)

	sourceInput := sourceInputForPosition(counts, spec.rows, spec.ints, spec.faults, spec.exactAtoms, spec.nilOwners)
	sourceDraft, err := source.Build(sourceInput)
	if err != nil {
		t.Fatalf("source.Build: %v", err)
	}
	sourceFinalize, err := sourceDraft.Finalizer()
	if err != nil {
		t.Fatalf("source.Finalizer: %v", err)
	}
	preimage := sourceFinalize.Preimage()

	staticInput := spec.static
	staticInput.Counts = counts
	_, staticView, err := static.Build(staticInput)
	if err != nil {
		_ = sourceFinalize.Abort()
		t.Fatalf("static.Build: %v", err)
	}

	flowInput := spec.flow
	flowInput.Counts = counts
	flowDraft, err := authored.Build(flowInput)
	if err != nil {
		_ = sourceFinalize.Abort()
		t.Fatalf("authored.Build: %v", err)
	}
	flowFinalize, err := flowDraft.Finalizer()
	if err != nil {
		_ = sourceFinalize.Abort()
		t.Fatalf("authored.Finalizer: %v", err)
	}
	flowView := flowFinalize.View()

	bodies, err := body.Seal(preimage, flowView, staticView, entry)
	if err != nil {
		flowtest.CloseFinalizers(sourceFinalize, flowFinalize, imports.Finalizer{})
		t.Fatalf("body.Seal: %v", err)
	}
	bindingResult, err := binding.Seal(preimage, flowView, bodies, entry)
	if err != nil {
		flowtest.CloseFinalizers(sourceFinalize, flowFinalize, imports.Finalizer{})
		t.Fatalf("binding.Seal: %v", err)
	}

	moduleDraft, err := imports.Build(spec.module)
	if err != nil {
		flowtest.CloseFinalizers(sourceFinalize, flowFinalize, imports.Finalizer{})
		t.Fatalf("imports.Build: %v", err)
	}
	moduleFinalize, err := moduleDraft.Finalizer()
	if err != nil {
		flowtest.CloseFinalizers(sourceFinalize, flowFinalize, imports.Finalizer{})
		t.Fatalf("imports.Finalizer: %v", err)
	}
	moduleView := moduleFinalize.View()

	forest, _, err := containment.Prove(preimage, staticView, flowView, bodies, bindingResult, moduleView, entry)
	if err != nil {
		flowtest.CloseFinalizers(sourceFinalize, flowFinalize, moduleFinalize)
		t.Fatalf("containment.Prove: %v", err)
	}
	shape, err := control.Seal(preimage, flowView, bodies, bindingResult, forest,
		staticView.ContentID(), moduleFinalize.View().ContentID())
	if err != nil {
		flowtest.CloseFinalizers(sourceFinalize, flowFinalize, moduleFinalize)
		t.Fatalf("control.Seal: %v", err)
	}
	outcomes, err := outcome.Seal(preimage.Identity(), flowView, bodies, shape,
		staticView.ContentID(), moduleFinalize.View().ContentID())
	if err != nil {
		flowtest.CloseFinalizers(sourceFinalize, flowFinalize, moduleFinalize)
		t.Fatalf("outcome.Seal: %v", err)
	}

	fixture := &positionFixture{
		preimage: preimage, flow: flowView, bodies: bodies, forest: forest, outcomes: outcomes, entry: entry,
		sourceFinalize: sourceFinalize, staticView: staticView,
		flowFinalize: flowFinalize, moduleFinalize: moduleFinalize,
	}
	t.Cleanup(func() { flowtest.CloseFinalizers(sourceFinalize, flowFinalize, moduleFinalize) })
	return fixture
}

func sealPositionFixture(fixture *positionFixture) (source.IndexInput, error) {
	return Seal(fixture.preimage, fixture.flow, fixture.bodies, fixture.forest, fixture.outcomes, fixture.entry,
		fixture.staticView.ContentID(), fixture.moduleFinalize.View().ContentID())
}

func sourceInputForPosition(counts [keyspace.FamilyCount]uint32, rows [][]keyspace.Term, ints []source.IntegerLiteral, faults []source.ControlFault, exactAtoms []keyspace.LiteralValue, nilOwners []keyspace.Term) source.Input {
	input := source.Input{Name: "position-law.lua"}
	input.ExactAtoms = append([]keyspace.LiteralValue(nil), exactAtoms...)
	input.Families = flowtest.FamilySpans(input.Name, counts)
	input.Bodies = make([]source.BodySource, counts[keyspace.FamilyBody])
	for index := range input.Bodies {
		if index < len(rows) {
			input.Bodies[index].Terms = append([]keyspace.Term(nil), rows[index]...)
		}
		input.Bodies[index].Body = keyspace.MakeTerm(keyspace.FamilyBody, uint32(index+1))
	}
	input.Binds = make([]source.BindCells, counts[keyspace.FamilyBind])
	for index := range input.Binds {
		input.Binds[index].Bind = keyspace.MakeTerm(keyspace.FamilyBind, uint32(index+1))
	}
	input.Functions = make([]source.FunctionFormals, counts[keyspace.FamilyFunction])
	for index := range input.Functions {
		input.Functions[index].Function = keyspace.MakeTerm(keyspace.FamilyFunction, uint32(index+1))
	}
	input.Integer = append([]source.IntegerLiteral(nil), ints...)
	input.Faults = append([]source.ControlFault(nil), faults...)
	input.Nil = flowtest.LiteralRows(counts[keyspace.FamilyNil], nilOwners, keyspace.MakeTerm(keyspace.FamilyBody, 1), func(owner keyspace.Term, _ uint32) source.NilLiteral {
		return source.NilLiteral{Owner: owner}
	})
	return input
}

func TestSealPositionsUseExactAnchorClosureAndOmitRootless(t *testing.T) {
	counts := positionCounts(1, 1, 1, 1, 0, 0, 0, 0, 0, 0)
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	returned := keyspace.MakeTerm(keyspace.FamilyReturn, 1)
	values := keyspace.MakeTerm(keyspace.FamilyValues, 1)
	integer := keyspace.MakeTerm(keyspace.FamilyInteger, 1)
	fixture := openPositionFixture(t, positionSpec{
		counts: counts,
		rows:   [][]keyspace.Term{{returned}},
		ints:   []source.IntegerLiteral{{Owner: body, Value: 7}},
		flow: authored.Input{
			Values:  authored.ValuesInput{Rows: []authored.Value{{Owner: body, Fixed: authored.Range{End: 1}}}, Terms: []keyspace.Term{integer}},
			Control: authored.ControlInput{Returns: []authored.Return{{Owner: body, Values: values}}},
		},
	})
	index, err := sealPositionFixture(fixture)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	if entry, ok := fixture.bodies.Entry(); !ok || entry != body {
		t.Fatalf("Body Entry = %v/%v, want %v/true", entry, ok, body)
	}
	if rootCount, ok := fixture.bodies.RootCount(body); !ok || rootCount != 1 {
		t.Fatalf("Body RootCount = %d/%v, want 1/true", rootCount, ok)
	}
	if root, ok := fixture.bodies.RootAt(body, 0); !ok || root != returned {
		t.Fatalf("Body RootAt = %v/%v, want %v/true", root, ok, returned)
	}
	if len(index.Positions) != 3 {
		t.Fatalf("Positions = %d, want Return/Values/Integer closure", len(index.Positions))
	}
	for _, term := range []keyspace.Term{returned, values, integer} {
		row, ok := positionFor(index.Positions, term)
		if !ok || row.Root != returned || row.Body != body || row.Offset != 0 || row.Cursor != 0 || row.FrontierBody != body || row.FrontierCursor != 0 || row.Repeat {
			t.Fatalf("Position(%v) = %#v/%v", term, row, ok)
		}
	}
	if _, ok := positionFor(index.Positions, body); ok {
		t.Fatal("rootless Entry Body unexpectedly acquired a position")
	}
	if len(index.OutcomeOrigins) != fixture.outcomes.Count() {
		t.Fatalf("OutcomeOrigins = %d, want %d", len(index.OutcomeOrigins), fixture.outcomes.Count())
	}
	assertOutcomeOriginRows(t, fixture, index)
	assertNoOutcomePositions(t, index)
	component, err := fixture.sourceFinalize.Commit(index)
	if err != nil {
		t.Fatalf("Source Commit: %v", err)
	}
	if root, ok := component.View().Index().Root(integer); !ok || root != returned {
		t.Fatalf("committed Integer root = %v/%v, want Return %v", root, ok, returned)
	}
	if frontierBody, frontierCursor, ok := component.View().Index().Frontier(integer); !ok || frontierBody != body || frontierCursor != 0 {
		t.Fatalf("committed Integer frontier = %v/%d/%v", frontierBody, frontierCursor, ok)
	}
}

func TestSealGlobalAndChunkVarargCellsRemainRootless(t *testing.T) {
	counts := positionCounts(1, 1, 1, 0, 0, 0, 0, 0, 0, 0)
	counts[keyspace.FamilyCell] = 2
	counts[keyspace.FamilyVararg] = 1
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	returned := keyspace.MakeTerm(keyspace.FamilyReturn, 1)
	values := keyspace.MakeTerm(keyspace.FamilyValues, 1)
	global := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	chunk := keyspace.MakeTerm(keyspace.FamilyCell, 2)
	vararg := keyspace.MakeTerm(keyspace.FamilyVararg, 1)
	fixture := openPositionFixture(t, positionSpec{
		counts:     counts,
		rows:       [][]keyspace.Term{{returned}},
		exactAtoms: []keyspace.LiteralValue{{Kind: keyspace.LiteralString, String: "global"}},
		flow: authored.Input{
			Values: authored.ValuesInput{Rows: []authored.Value{{Owner: body, Tail: vararg}}},
			Storage: authored.StorageInput{
				Cells:   []authored.Cell{{Kind: authored.CellGlobal, Key: 1}, {Kind: authored.CellLocal, Body: body}},
				Varargs: []authored.Vararg{{Owner: body, Cell: chunk}},
			},
			Control: authored.ControlInput{Returns: []authored.Return{{Owner: body, Values: values}}},
		},
	})
	index, err := sealPositionFixture(fixture)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	if _, ok := positionFor(index.Positions, global); ok {
		t.Fatal("global Cell unexpectedly acquired a source position")
	}
	if _, ok := positionFor(index.Positions, chunk); ok {
		t.Fatal("entry-hosted chunk-vararg Cell unexpectedly acquired a source position")
	}
	component, err := fixture.sourceFinalize.Commit(index)
	if err != nil {
		t.Fatalf("Source Commit with rootless Cells: %v", err)
	}
	for _, term := range []keyspace.Term{global, chunk} {
		if _, _, _, ok := component.View().Index().Position(term); ok {
			t.Fatalf("committed rootless Cell %v unexpectedly acquired a position", term)
		}
	}
}

func TestSealRepeatCopiesFrontierOnlyThroughRepeatAnchor(t *testing.T) {
	counts := positionCounts(3, 3, 3, 3, 2, 0, 0, 0, 2, 0)
	body1 := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	body2 := keyspace.MakeTerm(keyspace.FamilyBody, 2)
	body3 := keyspace.MakeTerm(keyspace.FamilyBody, 3)
	returns := []keyspace.Term{
		keyspace.MakeTerm(keyspace.FamilyReturn, 1),
		keyspace.MakeTerm(keyspace.FamilyReturn, 2),
		keyspace.MakeTerm(keyspace.FamilyReturn, 3),
	}
	loops := []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyLoop, 1), keyspace.MakeTerm(keyspace.FamilyLoop, 2)}
	nils := []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyNil, 1), keyspace.MakeTerm(keyspace.FamilyNil, 2)}
	values := []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyValues, 1), keyspace.MakeTerm(keyspace.FamilyValues, 2), keyspace.MakeTerm(keyspace.FamilyValues, 3)}
	fixture := openPositionFixture(t, positionSpec{
		counts: counts,
		rows:   [][]keyspace.Term{{returns[0], loops[0], loops[1]}, {returns[1]}, {returns[2]}},
		ints: []source.IntegerLiteral{
			{Owner: body1, Value: 1}, {Owner: body2, Value: 2}, {Owner: body3, Value: 3},
		},
		flow: authored.Input{
			Values: authored.ValuesInput{Rows: []authored.Value{{Owner: body1, Fixed: authored.Range{End: 1}}, {Owner: body2, Fixed: authored.Range{Start: 1, End: 2}}, {Owner: body3, Fixed: authored.Range{Start: 2, End: 3}}}, Terms: []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyInteger, 1), keyspace.MakeTerm(keyspace.FamilyInteger, 2), keyspace.MakeTerm(keyspace.FamilyInteger, 3)}},
			Control: authored.ControlInput{
				Returns: []authored.Return{{Owner: body1, Values: values[0]}, {Owner: body2, Values: values[1]}, {Owner: body3, Values: values[2]}},
				Loops:   []authored.Loop{{Owner: body1, Body: body2, Kind: kind.LoopRepeat, Control: nils[0]}, {Owner: body1, Body: body3, Kind: kind.LoopWhile, Control: nils[1]}},
			},
		},
		nilOwners: []keyspace.Term{body2, body1},
	})
	index, err := sealPositionFixture(fixture)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	repeat, ok := positionFor(index.Positions, loops[0])
	if !ok || !repeat.Repeat || repeat.Root != loops[0] || repeat.FrontierBody != body2 || repeat.FrontierCursor != 1 {
		t.Fatalf("Repeat position = %#v/%v", repeat, ok)
	}
	control, ok := positionFor(index.Positions, nils[0])
	if !ok || !control.Repeat || control.Root != loops[0] || control.FrontierBody != body2 || control.FrontierCursor != 1 {
		t.Fatalf("Repeat descendant position = %#v/%v", control, ok)
	}
	ordinary, ok := positionFor(index.Positions, loops[1])
	if !ok || ordinary.Repeat || ordinary.Root != loops[1] || ordinary.FrontierBody != body1 || ordinary.FrontierCursor != 2 {
		t.Fatalf("ordinary Loop position = %#v/%v", ordinary, ok)
	}
	ordinaryControl, ok := positionFor(index.Positions, nils[1])
	if !ok || ordinaryControl.Repeat || ordinaryControl.Root != loops[1] || ordinaryControl.FrontierBody != body1 || ordinaryControl.FrontierCursor != 2 {
		t.Fatalf("ordinary descendant position = %#v/%v", ordinaryControl, ok)
	}
	for _, typedBody := range []keyspace.Term{body2, body3} {
		if row, ok := positionFor(index.Positions, typedBody); ok {
			t.Fatalf("typed loop child Body %v acquired position: %#v", typedBody, row)
		}
	}
	for _, term := range []keyspace.Term{returns[1], returns[2]} {
		row, ok := positionFor(index.Positions, term)
		if !ok || row.Root != term {
			t.Fatalf("direct Body closure term %v = %#v/%v", term, row, ok)
		}
	}
	for _, want := range []struct {
		term   keyspace.Term
		body   keyspace.Term
		parent keyspace.Term
		roots  []keyspace.Term
	}{
		{body1, body1, 0, []keyspace.Term{returns[0], loops[0], loops[1]}},
		{body2, body2, body1, []keyspace.Term{returns[1]}},
		{body3, body3, body1, []keyspace.Term{returns[2]}},
	} {
		parent, hasParent := fixture.bodies.Parent(want.term)
		if (want.parent == 0 && hasParent) || (want.parent != 0 && (!hasParent || parent != want.parent)) {
			t.Fatalf("Body Parent(%v) = %v/%v, want %v", want.term, parent, hasParent, want.parent)
		}
		rootCount, ok := fixture.bodies.RootCount(want.term)
		if !ok || rootCount != len(want.roots) {
			t.Fatalf("Body RootCount(%v) = %d/%v, want %d", want.term, rootCount, ok, len(want.roots))
		}
		for rootIndex, root := range want.roots {
			got, ok := fixture.bodies.RootAt(want.term, rootIndex)
			if !ok || got != root {
				t.Fatalf("Body RootAt(%v,%d) = %v/%v, want %v", want.term, rootIndex, got, ok, root)
			}
		}
	}
	loopRow, _ := positionFor(index.Positions, loops[0])
	ordinaryRow, _ := positionFor(index.Positions, loops[1])
	if loopRow.Offset != 1 || loopRow.Cursor != 1 || ordinaryRow.Offset != 2 || ordinaryRow.Cursor != 2 {
		t.Fatalf("Loop source coordinates = repeat %d/%d ordinary %d/%d, want 1/1 and 2/2", loopRow.Offset, loopRow.Cursor, ordinaryRow.Offset, ordinaryRow.Cursor)
	}
	assertOutcomeOriginRows(t, fixture, index)
	assertNoOutcomePositions(t, index)
	_, err = fixture.sourceFinalize.Commit(index)
	if err != nil {
		t.Fatalf("Source Commit nested Bodies: %v", err)
	}
}

func TestSealPositionsCanonicalAndDeterministic(t *testing.T) {
	counts := positionCounts(1, 1, 1, 1, 0, 0, 0, 0, 0, 0)
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	returned := keyspace.MakeTerm(keyspace.FamilyReturn, 1)
	values := keyspace.MakeTerm(keyspace.FamilyValues, 1)
	integer := keyspace.MakeTerm(keyspace.FamilyInteger, 1)
	fixture := openPositionFixture(t, positionSpec{
		counts: counts,
		rows:   [][]keyspace.Term{{returned}},
		ints:   []source.IntegerLiteral{{Owner: body, Value: 9}},
		flow: authored.Input{
			Values:  authored.ValuesInput{Rows: []authored.Value{{Owner: body, Fixed: authored.Range{End: 1}}}, Terms: []keyspace.Term{integer}},
			Control: authored.ControlInput{Returns: []authored.Return{{Owner: body, Values: values}}},
		},
	})
	first, err := sealPositionFixture(fixture)
	if err != nil {
		t.Fatalf("first Seal: %v", err)
	}
	second, err := sealPositionFixture(fixture)
	if err != nil {
		t.Fatalf("second Seal: %v", err)
	}
	if len(first.Positions) != len(second.Positions) || len(first.OutcomeOrigins) != len(second.OutcomeOrigins) {
		t.Fatalf("repeat Seal cardinalities differ: %#v vs %#v", first, second)
	}
	if !reflect.DeepEqual(first.Positions, second.Positions) || !reflect.DeepEqual(first.OutcomeOrigins, second.OutcomeOrigins) {
		t.Fatalf("repeat Seal changed a projection: first=%#v second=%#v", first, second)
	}
	wantTerms := []keyspace.Term{integer, values, returned}
	for index, row := range first.Positions {
		if index >= len(wantTerms) || row.Term != wantTerms[index] {
			t.Fatalf("position order[%d] = %v, want %v", index, row.Term, wantTerms[index])
		}
		if row != second.Positions[index] {
			t.Fatalf("repeat Seal row %d differs: %#v vs %#v", index, row, second.Positions[index])
		}
	}
}

func TestSealPositionsIncludeControlFaultAfterOutcomeFamily(t *testing.T) {
	counts := positionCounts(1, 0, 0, 0, 0, 0, 0, 0, 0, 0)
	counts[keyspace.FamilyControlFault] = 1
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	fault := keyspace.MakeTerm(keyspace.FamilyControlFault, 1)
	fixture := openPositionFixture(t, positionSpec{
		counts: counts,
		rows:   [][]keyspace.Term{{fault}},
		faults: []source.ControlFault{{Owner: body, Kind: source.ControlFaultUndefinedGoto}},
	})
	index, err := sealPositionFixture(fixture)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	row, ok := positionFor(index.Positions, fault)
	if !ok || row.Root != fault || row.Body != body || row.FrontierBody != body || row.FrontierCursor != 0 {
		t.Fatalf("ControlFault position = %#v/%v", row, ok)
	}
	if len(index.Positions) != 1 {
		t.Fatalf("Positions = %d, want one ControlFault", len(index.Positions))
	}
	component, err := fixture.sourceFinalize.Commit(index)
	if err != nil {
		t.Fatalf("Source Commit with ControlFault: %v", err)
	}
	if root, ok := component.View().Index().Root(fault); !ok || root != fault {
		t.Fatalf("committed ControlFault root = %v/%v", root, ok)
	}
}

func TestSealPositionsContainModuleImportThroughItsCall(t *testing.T) {
	counts := positionCounts(1, 0, 1, 2, 0, 0, 1, 0, 0, 0)
	counts[keyspace.FamilyImport] = 1
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	call := keyspace.MakeTerm(keyspace.FamilyCall, 1)
	importTerm := keyspace.MakeTerm(keyspace.FamilyImport, 1)
	values := keyspace.MakeTerm(keyspace.FamilyValues, 1)
	callee := keyspace.MakeTerm(keyspace.FamilyInteger, 1)
	actual := keyspace.MakeTerm(keyspace.FamilyInteger, 2)
	fixture := openPositionFixture(t, positionSpec{
		counts: counts,
		rows:   [][]keyspace.Term{{call}},
		ints:   []source.IntegerLiteral{{Owner: body, Value: 1}, {Owner: body, Value: 2}},
		flow: authored.Input{
			Values: authored.ValuesInput{Rows: []authored.Value{{Owner: body, Fixed: authored.Range{End: 1}}}, Terms: []keyspace.Term{actual}},
			Calls:  []authored.Call{{Owner: body, Callee: callee, Actuals: values}},
		},
		static: static.Input{Contracts: staticcontracts.Input{Call: []staticcontracts.CallContract{{}}}},
		module: imports.Input{Imports: []imports.Import{{Term: importTerm, Call: call, Request: keyspace.MakeTerm(keyspace.FamilyString, 1)}}},
	})
	index, err := sealPositionFixture(fixture)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	for _, term := range []keyspace.Term{call, importTerm, values, callee, actual} {
		row, ok := positionFor(index.Positions, term)
		if !ok || row.Root != call || row.Body != body || row.FrontierBody != body {
			t.Fatalf("Import closure position(%v) = %#v/%v", term, row, ok)
		}
	}
	if _, err := fixture.sourceFinalize.Commit(index); err != nil {
		t.Fatalf("Source Commit with Import: %v", err)
	}
}

func TestSealMixedPostOutcomeFamiliesRemainCanonical(t *testing.T) {
	counts := positionCounts(1, 0, 1, 2, 0, 0, 1, 0, 0, 0)
	counts[keyspace.FamilyControlFault] = 1
	counts[keyspace.FamilyImport] = 1
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	call := keyspace.MakeTerm(keyspace.FamilyCall, 1)
	fault := keyspace.MakeTerm(keyspace.FamilyControlFault, 1)
	importTerm := keyspace.MakeTerm(keyspace.FamilyImport, 1)
	values := keyspace.MakeTerm(keyspace.FamilyValues, 1)
	callee := keyspace.MakeTerm(keyspace.FamilyInteger, 1)
	actual := keyspace.MakeTerm(keyspace.FamilyInteger, 2)
	fixture := openPositionFixture(t, positionSpec{
		counts: counts,
		rows:   [][]keyspace.Term{{call, fault}},
		ints:   []source.IntegerLiteral{{Owner: body, Value: 1}, {Owner: body, Value: 2}},
		faults: []source.ControlFault{{Owner: body, Kind: source.ControlFaultUndefinedGoto}},
		flow: authored.Input{
			Values:  authored.ValuesInput{Rows: []authored.Value{{Owner: body, Fixed: authored.Range{End: 1}}}, Terms: []keyspace.Term{actual}},
			Calls:   []authored.Call{{Owner: body, Callee: callee, Actuals: values}},
			Control: authored.ControlInput{},
		},
		static: static.Input{Contracts: staticcontracts.Input{Call: []staticcontracts.CallContract{{}}}},
		module: imports.Input{Imports: []imports.Import{{Term: importTerm, Call: call, Request: keyspace.MakeTerm(keyspace.FamilyString, 1)}}},
	})
	index, err := sealPositionFixture(fixture)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	want := []keyspace.Term{callee, actual, values, call, fault, importTerm}
	if len(index.Positions) != len(want) {
		t.Fatalf("Positions = %d, want %d", len(index.Positions), len(want))
	}
	for index, row := range index.Positions {
		if row.Term != want[index] {
			t.Fatalf("Positions[%d].Term = %v, want %v", index, row.Term, want[index])
		}
		if keyspace.TermFamily(row.Term) == keyspace.FamilyOutcome {
			t.Fatalf("Outcome leaked at position %d", index)
		}
	}
	callRow, _ := positionFor(index.Positions, call)
	faultRow, _ := positionFor(index.Positions, fault)
	if callRow.Offset != 0 || callRow.Cursor != 0 || faultRow.Offset != 1 || faultRow.Cursor != 1 {
		t.Fatalf("mixed direct coordinates = Call %d/%d Fault %d/%d, want 0/0 and 1/1", callRow.Offset, callRow.Cursor, faultRow.Offset, faultRow.Cursor)
	}
	assertOutcomeOriginRows(t, fixture, index)
	assertNoOutcomePositions(t, index)
}

func TestSealDeepClosureUsesIterativePathCompression(t *testing.T) {
	const depth = 4096
	counts := positionCounts(1, 1, 1, 1, 0, 0, 0, 0, 0, 0)
	counts[keyspace.FamilyUnary] = depth
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	returned := keyspace.MakeTerm(keyspace.FamilyReturn, 1)
	values := keyspace.MakeTerm(keyspace.FamilyValues, 1)
	integer := keyspace.MakeTerm(keyspace.FamilyInteger, 1)
	unaries := make([]authored.Unary, depth)
	for index := range unaries {
		operand := integer
		if index+1 < len(unaries) {
			operand = keyspace.MakeTerm(keyspace.FamilyUnary, uint32(index+2))
		}
		unaries[index] = authored.Unary{Owner: body, Op: kind.UnaryNot, Operand: operand}
	}
	fixture := openPositionFixture(t, positionSpec{
		counts: counts,
		rows:   [][]keyspace.Term{{returned}},
		ints:   []source.IntegerLiteral{{Owner: body, Value: 1}},
		flow: authored.Input{
			Values:    authored.ValuesInput{Rows: []authored.Value{{Owner: body, Fixed: authored.Range{End: 1}}}, Terms: []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyUnary, 1)}},
			Operators: authored.OperatorsInput{Unaries: unaries},
			Control:   authored.ControlInput{Returns: []authored.Return{{Owner: body, Values: values}}},
		},
	})
	index, err := sealPositionFixture(fixture)
	if err != nil {
		t.Fatalf("Seal depth %d: %v", depth, err)
	}
	if got, want := len(index.Positions), depth+3; got != want {
		t.Fatalf("Positions = %d, want %d", got, want)
	}
	for _, term := range []keyspace.Term{integer, keyspace.MakeTerm(keyspace.FamilyUnary, 1), keyspace.MakeTerm(keyspace.FamilyUnary, depth)} {
		row, ok := positionFor(index.Positions, term)
		if !ok || row.Root != returned || row.Body != body || row.FrontierBody != body {
			t.Fatalf("deep closure Position(%v) = %#v/%v", term, row, ok)
		}
	}
}

func positionFor(rows []source.Position, term keyspace.Term) (source.Position, bool) {
	for _, row := range rows {
		if row.Term == term {
			return row, true
		}
	}
	return source.Position{}, false
}

func assertOutcomeOriginRows(t *testing.T, fixture *positionFixture, index source.IndexInput) {
	t.Helper()
	if len(index.OutcomeOrigins) != fixture.outcomes.Count() {
		t.Fatalf("OutcomeOrigins length = %d, want %d", len(index.OutcomeOrigins), fixture.outcomes.Count())
	}
	for ordinal, origin := range index.OutcomeOrigins {
		outcomeTerm, ok := fixture.outcomes.At(ordinal)
		if !ok {
			t.Fatalf("Outcome.At(%d) unavailable", ordinal)
		}
		want, _, _, ok := fixture.outcomes.Get(outcomeTerm)
		if !ok || origin != want {
			t.Fatalf("OutcomeOrigins[%d] = %v, want Get(%v)=%v/%v", ordinal, origin, outcomeTerm, want, ok)
		}
	}
}

func assertNoOutcomePositions(t *testing.T, index source.IndexInput) {
	t.Helper()
	for _, row := range index.Positions {
		if keyspace.TermFamily(row.Term) == keyspace.FamilyOutcome || keyspace.TermFamily(row.Root) == keyspace.FamilyOutcome {
			t.Fatalf("Outcome leaked into position projection: %#v", row)
		}
	}
}

func positionCounts(body, ret, values, integers, nils, binds, calls, branches, loops, functions uint32) (counts [keyspace.FamilyCount]uint32) {
	counts[keyspace.FamilyBody] = body
	counts[keyspace.FamilyReturn] = ret
	counts[keyspace.FamilyValues] = values
	counts[keyspace.FamilyInteger] = integers
	counts[keyspace.FamilyNil] = nils
	counts[keyspace.FamilyBind] = binds
	counts[keyspace.FamilyCall] = calls
	counts[keyspace.FamilyBranch] = branches
	counts[keyspace.FamilyLoop] = loops
	counts[keyspace.FamilyFunction] = functions
	return counts
}
