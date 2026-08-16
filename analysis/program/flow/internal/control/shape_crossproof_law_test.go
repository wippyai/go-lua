package control

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/program/flow/internal/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/binding"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/body"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/containment"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/imports"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/analysis/program/static"
)

// TestSealRejectsForeignBodyAtFunctionBoundary keeps the authored Function
// relation fixed while splicing a separately sealed Body result whose
// Function owner is a different lexical Body. Equal denominators are not
// enough: Control must cross-check the current Function→Body edge against the
// Body-owned parent and activation projections.
func TestSealRejectsForeignBodyAtFunctionBoundary(t *testing.T) {
	counts := controlCounts(3, 2, 2, 0, 0, 1, 0, 0, 0, 1)
	counts[keyspace.FamilyReturn] = 2
	body, loopBody, functionBody := terms(counts, keyspace.FamilyBody, 1), terms(counts, keyspace.FamilyBody, 2), terms(counts, keyspace.FamilyBody, 3)
	loop, function, returned1, returned2 := terms(counts, keyspace.FamilyLoop, 1), terms(counts, keyspace.FamilyFunction, 1), terms(counts, keyspace.FamilyReturn, 1), terms(counts, keyspace.FamilyReturn, 2)
	values1, values2 := terms(counts, keyspace.FamilyValues, 1), terms(counts, keyspace.FamilyValues, 2)
	nil1, nil2 := terms(counts, keyspace.FamilyNil, 1), terms(counts, keyspace.FamilyNil, 2)

	currentInput := authored.Input{
		Values: authored.ValuesInput{
			Rows:  []authored.Value{{Owner: loopBody, Fixed: authored.Range{End: 1}}, {Owner: loopBody, Fixed: authored.Range{Start: 1, End: 2}}},
			Terms: []keyspace.Term{function, nil2},
		},
		Functions: authored.FunctionsInput{Rows: []authored.Function{{Owner: loopBody, Body: functionBody}}},
		Control: authored.ControlInput{
			Returns: []authored.Return{{Owner: loopBody, Values: values1}, {Owner: loopBody, Values: values2}},
			Loops:   []authored.Loop{{Owner: body, Body: loopBody, Kind: kind.LoopWhile, Control: nil1}},
		},
		Counts: counts,
	}
	current := openShapeFixture(t, shapeSpec{
		counts:    counts,
		rows:      bodyRows([]keyspace.Term{loop}, []keyspace.Term{returned1, returned2}, nil),
		flow:      currentInput,
		forms:     []source.FunctionFormals{{Function: function}},
		nilOwners: []keyspace.Term{body, loopBody},
	})
	current.seal(t)

	foreignInput := authored.Input{
		Values: authored.ValuesInput{
			Rows:  []authored.Value{{Owner: body, Fixed: authored.Range{End: 1}}, {Owner: body, Fixed: authored.Range{Start: 1, End: 2}}},
			Terms: []keyspace.Term{function, nil2},
		},
		Functions: authored.FunctionsInput{Rows: []authored.Function{{Owner: body, Body: functionBody}}},
		Control: authored.ControlInput{
			Returns: []authored.Return{{Owner: body, Values: values1}, {Owner: body, Values: values2}},
			Loops:   []authored.Loop{{Owner: body, Body: loopBody, Kind: kind.LoopWhile, Control: nil1}},
		},
		Counts: counts,
	}
	foreign := openShapeFixture(t, shapeSpec{
		counts:    counts,
		rows:      bodyRows([]keyspace.Term{loop, returned1, returned2}, nil, nil),
		flow:      foreignInput,
		forms:     []source.FunctionFormals{{Function: function}},
		nilOwners: []keyspace.Term{body, body},
	})
	if _, err := Seal(current.preimage, current.flow, foreign.bodies, current.binding, current.forest,
		current.staticFinalize.View().ContentID(), current.moduleFinalize.View().ContentID()); err == nil {
		t.Fatal("foreign Body Function boundary was accepted")
	}
}

// TestSealRejectsBodyResultThatOmitsOrReordersBindRoot verifies that a Body
// proof cannot be substituted for the Body proof belonging to Source order.
// The current Source places a Goto before a Bind and its target Label after
// that Bind, so accepting a Body image which omits or moves the Bind would
// erase the local-scope barrier used by Goto validation.
func TestSealRejectsBodyResultThatOmitsOrReordersBindRoot(t *testing.T) {
	counts := controlCounts(2, 1, 1, 1, 1, 0, 1, 1, 0, 0)
	body, child := terms(counts, keyspace.FamilyBody, 1), terms(counts, keyspace.FamilyBody, 2)
	bind, cell := terms(counts, keyspace.FamilyBind, 1), terms(counts, keyspace.FamilyCell, 1)
	values, label, jump := terms(counts, keyspace.FamilyValues, 1), terms(counts, keyspace.FamilyLabel, 1), terms(counts, keyspace.FamilyGoto, 1)

	currentInput := authored.Input{
		Values: authored.ValuesInput{
			Rows:  []authored.Value{{Owner: body, Fixed: authored.Range{End: 1}}},
			Terms: []keyspace.Term{terms(counts, keyspace.FamilyNil, 1)},
		},
		Storage: authored.StorageInput{
			Cells: []authored.Cell{{Kind: authored.CellLocal, Body: body}},
			Binds: []authored.Bind{{Owner: body, Values: values}},
		},
		Control: authored.ControlInput{
			Labels: []authored.Label{{Owner: body}},
			Gotos:  []authored.Goto{{Owner: body, Target: label}},
		},
		Counts: counts,
	}
	current := openShapeFixture(t, shapeSpec{
		counts: counts,
		// Bind is between the Goto and its target Label. The child Body is a
		// second statement root so the foreign fixtures can omit/reorder Bind
		// while retaining every family denominator.
		rows:  bodyRows([]keyspace.Term{jump, bind, label, child}, nil),
		flow:  currentInput,
		binds: []source.BindCells{{Bind: bind, Cells: []keyspace.Term{cell}}},
	})
	if err := current.sealError(); err == nil || !strings.Contains(err.Error(), "Goto enters") {
		t.Fatalf("baseline Goto-over-Bind fixture error = %v, want entered-local rejection", err)
	}

	cases := []struct {
		name      string
		rows      bodyOrder
		flow      authored.Input
		binds     []source.BindCells
		nilOwners []keyspace.Term
	}{
		{
			name: "bind omitted from current Body roots",
			// This honest Body proof places Bind in the child Body. Relative to
			// currentInput, the current Body's Bind root is therefore absent.
			rows: bodyRows(
				[]keyspace.Term{jump, label, child},
				[]keyspace.Term{bind},
			),
			flow: func() authored.Input {
				input := currentInput
				input.Values.Rows = []authored.Value{{Owner: child, Fixed: authored.Range{End: 1}}}
				input.Storage.Cells = []authored.Cell{{Kind: authored.CellLocal, Body: child}}
				input.Storage.Binds = []authored.Bind{{Owner: child, Values: values}}
				return input
			}(),
			binds:     []source.BindCells{{Bind: bind, Cells: []keyspace.Term{cell}}},
			nilOwners: []keyspace.Term{child},
		},
		{
			name: "bind reordered after child Body root",
			// Both fixtures own Bind by the entry Body, but the foreign Body
			// image orders the child Body root before Bind.
			rows: bodyRows(
				[]keyspace.Term{jump, child, bind, label},
				nil,
			),
			flow:  currentInput,
			binds: []source.BindCells{{Bind: bind, Cells: []keyspace.Term{cell}}},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			foreign := openShapeFixture(t, shapeSpec{
				counts:    counts,
				rows:      tc.rows,
				flow:      tc.flow,
				binds:     tc.binds,
				nilOwners: tc.nilOwners,
			})
			if _, err := Seal(current.preimage, current.flow, foreign.bodies, current.binding, current.forest,
				current.staticFinalize.View().ContentID(), current.moduleFinalize.View().ContentID()); err == nil {
				t.Fatal("foreign Body proof was accepted despite Source root mismatch")
			} else if !strings.Contains(err.Error(), "Body root") && !strings.Contains(err.Error(), "source position") && !strings.Contains(err.Error(), "Body provenance") {
				t.Fatalf("foreign Body proof rejection = %v, want semantic root/source mismatch", err)
			}
		})
	}
}

// TestSealRejectsForeignBindingAgainstBindCell uses two honest proofs with
// equal denominators. The fixtures swap the global and Bind-local Cell
// ordinals, so the current authored Source asks for its local Cell while the
// foreign Binding Result classifies that same ordinal as global.
func TestSealRejectsForeignBindingAgainstBindCell(t *testing.T) {
	counts := controlCounts(1, 1, 1, 2, 1, 0, 0, 0, 0, 0)
	body := terms(counts, keyspace.FamilyBody, 1)
	bind, values := terms(counts, keyspace.FamilyBind, 1), terms(counts, keyspace.FamilyValues, 1)
	nilTerm := terms(counts, keyspace.FamilyNil, 1)
	cell1, cell2 := terms(counts, keyspace.FamilyCell, 1), terms(counts, keyspace.FamilyCell, 2)
	exacts := []keyspace.LiteralValue{{Kind: keyspace.LiteralString, String: "global"}}

	currentInput := authored.Input{
		Values: authored.ValuesInput{
			Rows:  []authored.Value{{Owner: body, Fixed: authored.Range{End: 1}}},
			Terms: []keyspace.Term{nilTerm},
		},
		Storage: authored.StorageInput{
			Cells: []authored.Cell{
				{Kind: authored.CellGlobal, Key: 1},
				{Kind: authored.CellLocal, Body: body},
			},
			Binds: []authored.Bind{{Owner: body, Values: values}},
		},
		Counts: counts,
	}
	current := openShapeFixtureWithExactAtoms(t, shapeSpec{
		counts: counts,
		rows:   bodyRows([]keyspace.Term{bind}),
		flow:   currentInput,
		binds:  []source.BindCells{{Bind: bind, Cells: []keyspace.Term{cell2}}},
	}, exacts)

	foreignInput := currentInput
	foreignInput.Storage.Cells = []authored.Cell{
		{Kind: authored.CellLocal, Body: body},
		{Kind: authored.CellGlobal, Key: 1},
	}
	foreign := openShapeFixtureWithExactAtoms(t, shapeSpec{
		counts: counts,
		rows:   bodyRows([]keyspace.Term{bind}),
		flow:   foreignInput,
		binds:  []source.BindCells{{Bind: bind, Cells: []keyspace.Term{cell1}}},
	}, exacts)

	if _, err := Seal(current.preimage, current.flow, current.bodies, foreign.binding, current.forest,
		current.staticFinalize.View().ContentID(), current.moduleFinalize.View().ContentID()); err == nil {
		t.Fatal("foreign Binding Result with global Bind Cell was accepted")
	} else if !strings.Contains(err.Error(), "Bind Cell scope") && !strings.Contains(err.Error(), "Binding provenance") {
		t.Fatalf("foreign Bind Cell rejection = %v, want semantic Bind Cell scope rejection", err)
	}
}

// TestSealRejectsForeignBindingAgainstLoopCell uses the same honest-result
// swap for a Loop host. The current authored Loop points at Cell 2; the
// foreign Binding Result classifies Cell 2 as global rather than CellLoop.
func TestSealRejectsForeignBindingAgainstLoopCell(t *testing.T) {
	counts := controlCounts(2, 1, 2, 2, 0, 1, 0, 0, 0, 0)
	body, loopBody := terms(counts, keyspace.FamilyBody, 1), terms(counts, keyspace.FamilyBody, 2)
	loop := terms(counts, keyspace.FamilyLoop, 1)
	cell1, cell2 := terms(counts, keyspace.FamilyCell, 1), terms(counts, keyspace.FamilyCell, 2)
	exacts := []keyspace.LiteralValue{{Kind: keyspace.LiteralString, String: "global"}}

	currentInput := authored.Input{
		Values: authored.ValuesInput{
			Rows:  []authored.Value{{Owner: body, Fixed: authored.Range{End: 2}}},
			Terms: []keyspace.Term{terms(counts, keyspace.FamilyNil, 1), terms(counts, keyspace.FamilyNil, 2)},
		},
		Storage: authored.StorageInput{Cells: []authored.Cell{
			{Kind: authored.CellGlobal, Key: 1},
			{Kind: authored.CellLocal, Body: loopBody},
		}},
		Control: authored.ControlInput{
			Loops: []authored.Loop{{
				Owner: body, Body: loopBody, Kind: kind.LoopNumericFor,
				Control: terms(counts, keyspace.FamilyValues, 1), Cells: authored.Range{End: 1},
			}},
			Cells: []keyspace.Term{cell2},
		},
		Counts: counts,
	}
	current := openShapeFixtureWithExactAtoms(t, shapeSpec{
		counts: counts,
		rows:   bodyRows([]keyspace.Term{loop}, nil),
		flow:   currentInput,
	}, exacts)

	foreignInput := currentInput
	foreignInput.Storage.Cells = []authored.Cell{
		{Kind: authored.CellLocal, Body: loopBody},
		{Kind: authored.CellGlobal, Key: 1},
	}
	foreignInput.Control.Cells = []keyspace.Term{cell1}
	foreign := openShapeFixtureWithExactAtoms(t, shapeSpec{
		counts: counts,
		rows:   bodyRows([]keyspace.Term{loop}, nil),
		flow:   foreignInput,
	}, exacts)

	if _, err := Seal(current.preimage, current.flow, current.bodies, foreign.binding, current.forest,
		current.staticFinalize.View().ContentID(), current.moduleFinalize.View().ContentID()); err == nil {
		t.Fatal("foreign Binding Result with global Loop Cell was accepted")
	} else if !strings.Contains(err.Error(), "Loop Cell binding") && !strings.Contains(err.Error(), "Binding provenance") {
		t.Fatalf("foreign Loop Cell rejection = %v, want semantic Loop Cell binding rejection", err)
	}
}

// openShapeFixtureWithExactAtoms is the existing Shape fixture with the
// Source-owned exact atom batch exposed for cross-proof global-Cell laws. It
// intentionally still seals every owner proof; only the final Control call
// receives a foreign immutable result.
func openShapeFixtureWithExactAtoms(t *testing.T, spec shapeSpec, exactAtoms []keyspace.LiteralValue) *shapeFixture {
	t.Helper()
	if spec.counts[keyspace.FamilyBody] == 0 {
		t.Fatal("shape fixture requires an Entry Body")
	}
	flowInput := spec.flow
	flowInput.Counts = spec.counts

	sourceInput := shapeSourceInput(spec.counts, spec.rows, spec.binds, spec.forms, spec.nilOwners, spec.faults)
	sourceInput.ExactAtoms = append([]keyspace.LiteralValue(nil), exactAtoms...)
	sourceDraft, err := source.Build(sourceInput)
	if err != nil {
		t.Fatalf("source.Build: %v", err)
	}
	sourceFinalize, err := sourceDraft.Finalizer()
	if err != nil {
		t.Fatalf("source.Finalizer: %v", err)
	}
	preimage := sourceFinalize.Preimage()

	staticInput := static.Input{Counts: spec.counts}
	if spec.counts[keyspace.FamilyFunction] != 0 {
		staticInput.Contracts.Function = make([]static.FunctionContract, spec.counts[keyspace.FamilyFunction])
	}
	staticDraft, err := static.Build(staticInput)
	if err != nil {
		_ = sourceFinalize.Abort()
		t.Fatalf("static.Build: %v", err)
	}
	staticFinalize, err := staticDraft.Finalizer()
	if err != nil {
		_ = sourceFinalize.Abort()
		t.Fatalf("static.Finalizer: %v", err)
	}
	staticView := staticFinalize.View()

	flowDraft, err := authored.Build(flowInput)
	if err != nil {
		_ = staticFinalize.Abort()
		_ = sourceFinalize.Abort()
		t.Fatalf("authored.Build: %v", err)
	}
	flowFinalize, err := flowDraft.Finalizer()
	if err != nil {
		_ = staticFinalize.Abort()
		_ = sourceFinalize.Abort()
		t.Fatalf("authored.Finalizer: %v", err)
	}
	flowView := flowFinalize.View()

	entry := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	bodies, err := body.Seal(preimage, flowView, staticView, entry)
	if err != nil {
		_ = flowFinalize.Abort()
		_ = staticFinalize.Abort()
		_ = sourceFinalize.Abort()
		t.Fatalf("body.Seal: %v", err)
	}
	bindingResult, err := binding.Seal(preimage, flowView, bodies, entry)
	if err != nil {
		_ = flowFinalize.Abort()
		_ = staticFinalize.Abort()
		_ = sourceFinalize.Abort()
		t.Fatalf("binding.Seal: %v", err)
	}

	moduleDraft, err := imports.Build(imports.Input{})
	if err != nil {
		_ = flowFinalize.Abort()
		_ = staticFinalize.Abort()
		_ = sourceFinalize.Abort()
		t.Fatalf("imports.Build: %v", err)
	}
	moduleFinalize, err := moduleDraft.Finalizer()
	if err != nil {
		_ = flowFinalize.Abort()
		_ = staticFinalize.Abort()
		_ = sourceFinalize.Abort()
		t.Fatalf("imports.Finalizer: %v", err)
	}
	moduleView := moduleFinalize.View()

	forest, _, err := containment.Prove(preimage, staticView, flowView, bodies, bindingResult, moduleView, entry)
	if err != nil {
		_ = moduleFinalize.Abort()
		_ = flowFinalize.Abort()
		_ = staticFinalize.Abort()
		_ = sourceFinalize.Abort()
		t.Fatalf("containment.Prove: %v", err)
	}
	fixture := &shapeFixture{
		preimage: preimage, flow: flowView, bodies: bodies, binding: bindingResult, forest: forest,
		sourceFinalize: sourceFinalize, staticFinalize: staticFinalize,
		flowFinalize: flowFinalize, moduleFinalize: moduleFinalize,
	}
	t.Cleanup(func() {
		_ = fixture.moduleFinalize.Abort()
		_ = fixture.flowFinalize.Abort()
		_ = fixture.staticFinalize.Abort()
		_ = fixture.sourceFinalize.Abort()
	})
	return fixture
}
