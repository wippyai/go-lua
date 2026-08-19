package executable

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/program/flow/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/binding"
	"github.com/wippyai/go-lua/analysis/program/flow/body"
	"github.com/wippyai/go-lua/analysis/program/flow/containment"
	"github.com/wippyai/go-lua/analysis/program/flow/control"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/flow/outcome"
	"github.com/wippyai/go-lua/analysis/program/flow/position"
	"github.com/wippyai/go-lua/analysis/program/flow/semanticpath"
	"github.com/wippyai/go-lua/analysis/program/flow/sourcecontrol"
	"github.com/wippyai/go-lua/analysis/program/imports"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/analysis/program/static"
	staticcontracts "github.com/wippyai/go-lua/analysis/program/static/contracts"
	staticoperands "github.com/wippyai/go-lua/analysis/program/static/operands"
	statictypes "github.com/wippyai/go-lua/analysis/program/static/types"
)

// sealFixture is intentionally assembled through the final Source commit and
// Outcome derivation. The executable projection must therefore prove its
// pre-Outcome denominator against a Source identity whose Outcome family is
// already nonzero.
type sealFixture struct {
	sourceView source.View
	flow       authored.View
	bodies     *body.Result
	forest     *containment.Result
	control    *sourcecontrol.Result
	paths      *semanticpath.Certificate

	staticFinalize static.Finalizer
	flowFinalize   authored.Finalizer
	moduleFinalize imports.Finalizer
}

type sealSourceExtras struct {
	keys          []source.KeyInput
	exactAtoms    []keyspace.LiteralValue
	boolOwners    []keyspace.Term
	integerOwners []keyspace.Term
	floatOwners   []keyspace.Term
	stringOwners  []keyspace.Term
}

func openSealFixture(
	t *testing.T,
	name string,
	counts [keyspace.FamilyCount]uint32,
	rows [][]keyspace.Term,
	flowInput authored.Input,
	binds []source.BindCells,
	forms []source.FunctionFormals,
	nilOwners []keyspace.Term,
) *sealFixture {
	return openSealFixtureWithSource(t, name, counts, rows, flowInput, binds, forms, nilOwners, sealSourceExtras{})
}

func openSealFixtureWithSource(
	t *testing.T,
	name string,
	counts [keyspace.FamilyCount]uint32,
	rows [][]keyspace.Term,
	flowInput authored.Input,
	binds []source.BindCells,
	forms []source.FunctionFormals,
	nilOwners []keyspace.Term,
	extras sealSourceExtras,
) *sealFixture {
	t.Helper()
	if counts[keyspace.FamilyBody] == 0 || len(rows) != int(counts[keyspace.FamilyBody]) {
		t.Fatal("seal fixture requires one Source Body row per Body")
	}
	flowInput.Counts = counts
	if name == "" {
		name = "executable-seal.lua"
	}

	sourceDraft, err := source.Build(sealSourceInput(name, counts, rows, binds, forms, nilOwners, extras))
	if err != nil {
		t.Fatalf("source.Build: %v", err)
	}
	sourceFinalize, err := sourceDraft.Finalizer()
	if err != nil {
		t.Fatalf("source.Finalizer: %v", err)
	}
	preimage := sourceFinalize.Preimage()

	staticInput := static.Input{}
	staticInput.Counts[keyspace.FamilyBody] = counts[keyspace.FamilyBody]
	if counts[keyspace.FamilyTypePrimitive] != 0 {
		staticInput.Types.Primitive = make([]statictypes.Primitive, counts[keyspace.FamilyTypePrimitive])
		for index := range staticInput.Types.Primitive {
			staticInput.Types.Primitive[index] = statictypes.Primitive{Kind: statictypes.PrimitiveNumber}
		}
	}
	if counts[keyspace.FamilyFunction] != 0 {
		staticInput.Contracts.Function = make([]staticcontracts.FunctionContract, counts[keyspace.FamilyFunction])
	}
	if counts[keyspace.FamilyCall] != 0 {
		staticInput.Contracts.Call = make([]staticcontracts.CallContract, counts[keyspace.FamilyCall])
	}
	staticInput.Counts[keyspace.FamilyFunction] = uint32(len(staticInput.Contracts.Function))
	staticInput.Counts[keyspace.FamilyCall] = uint32(len(staticInput.Contracts.Call))
	staticInput.Counts[keyspace.FamilyTypePrimitive] = uint32(len(staticInput.Types.Primitive))
	if counts[keyspace.FamilyTypeValue] != 0 {
		staticInput.Counts[keyspace.FamilyTypeValue] = counts[keyspace.FamilyTypeValue]
		staticInput.Operands.TypeValue = make([]staticoperands.TypeValueTarget, counts[keyspace.FamilyTypeValue])
		for index := range staticInput.Operands.TypeValue {
			staticInput.Operands.TypeValue[index] = staticoperands.TypeValueTarget{Target: keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1)}
		}
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
		closeSealFinalizers(sourceFinalize, staticFinalize, authored.Finalizer{}, imports.Finalizer{})
		t.Fatalf("authored.Build: %v", err)
	}
	flowFinalize, err := flowDraft.Finalizer()
	if err != nil {
		closeSealFinalizers(sourceFinalize, staticFinalize, authored.Finalizer{}, imports.Finalizer{})
		t.Fatalf("authored.Finalizer: %v", err)
	}
	flowView := flowFinalize.View()
	entry := keyspace.MakeTerm(keyspace.FamilyBody, 1)

	bodies, err := body.Seal(preimage, flowView, staticView, entry)
	if err != nil {
		closeSealFinalizers(sourceFinalize, staticFinalize, flowFinalize, imports.Finalizer{})
		t.Fatalf("body.Seal: %v", err)
	}
	bindingResult, err := binding.Seal(preimage, flowView, bodies, entry)
	if err != nil {
		closeSealFinalizers(sourceFinalize, staticFinalize, flowFinalize, imports.Finalizer{})
		t.Fatalf("binding.Seal: %v", err)
	}

	moduleDraft, err := imports.Build(imports.Input{})
	if err != nil {
		closeSealFinalizers(sourceFinalize, staticFinalize, flowFinalize, imports.Finalizer{})
		t.Fatalf("imports.Build: %v", err)
	}
	moduleFinalize, err := moduleDraft.Finalizer()
	if err != nil {
		closeSealFinalizers(sourceFinalize, staticFinalize, flowFinalize, imports.Finalizer{})
		t.Fatalf("imports.Finalizer: %v", err)
	}
	moduleView := moduleFinalize.View()

	forest, _, err := containment.Prove(preimage, staticView, flowView, bodies, bindingResult, moduleView, entry)
	if err != nil {
		closeSealFinalizers(sourceFinalize, staticFinalize, flowFinalize, moduleFinalize)
		t.Fatalf("containment.Prove: %v", err)
	}
	shape, err := control.Seal(preimage, flowView, bodies, bindingResult, forest,
		staticView.ContentID(), moduleView.ContentID())
	if err != nil {
		closeSealFinalizers(sourceFinalize, staticFinalize, flowFinalize, moduleFinalize)
		t.Fatalf("control.Seal: %v", err)
	}
	outcomes, err := outcome.Seal(preimage.Identity(), flowView, bodies, shape,
		staticView.ContentID(), moduleView.ContentID())
	if err != nil {
		closeSealFinalizers(sourceFinalize, staticFinalize, flowFinalize, moduleFinalize)
		t.Fatalf("outcome.Seal: %v", err)
	}
	indexInput, err := position.Seal(preimage, flowView, bodies, forest, outcomes, entry,
		staticView.ContentID(), moduleView.ContentID())
	if err != nil {
		closeSealFinalizers(sourceFinalize, staticFinalize, flowFinalize, moduleFinalize)
		t.Fatalf("position.Seal: %v", err)
	}
	sourceComponent, issuance, err := sourceFinalize.CommitWithSemanticPathIssuance(indexInput)
	if err != nil {
		closeSealFinalizers(source.Finalizer{}, staticFinalize, flowFinalize, moduleFinalize)
		t.Fatalf("source.Commit: %v", err)
	}
	sourceView := sourceComponent.View()
	controlResult, err := sourcecontrol.Seal(sourceView, flowView, bodies, forest, shape, entry,
		staticView.ContentID(), moduleView.ContentID())
	if err != nil {
		closeSealFinalizers(source.Finalizer{}, staticFinalize, flowFinalize, moduleFinalize)
		t.Fatalf("sourcecontrol.Seal: %v", err)
	}
	paths, err := semanticpath.Seal(issuance, sourceView.CellRoles(), sourceView, flowView, bodies, bindingResult, forest, outcomes,
		flowView.Cold().ContentID(), staticView.ContentID(), moduleView.ContentID())
	if err != nil {
		closeSealFinalizers(source.Finalizer{}, staticFinalize, flowFinalize, moduleFinalize)
		t.Fatalf("semanticpath.Seal: %v", err)
	}

	fixture := &sealFixture{
		sourceView:     sourceView,
		flow:           flowView,
		bodies:         bodies,
		forest:         forest,
		control:        controlResult,
		paths:          paths,
		staticFinalize: staticFinalize,
		flowFinalize:   flowFinalize,
		moduleFinalize: moduleFinalize,
	}
	t.Cleanup(func() {
		closeSealFinalizers(source.Finalizer{}, fixture.staticFinalize, fixture.flowFinalize, fixture.moduleFinalize)
	})
	return fixture
}

func closeSealFinalizers(
	sourceFinalize source.Finalizer,
	staticFinalize static.Finalizer,
	flowFinalize authored.Finalizer,
	moduleFinalize imports.Finalizer,
) {
	_ = moduleFinalize.Abort()
	_ = flowFinalize.Abort()
	_ = staticFinalize.Abort()
	_ = sourceFinalize.Abort()
}

func sealSourceInput(
	name string,
	counts [keyspace.FamilyCount]uint32,
	rows [][]keyspace.Term,
	binds []source.BindCells,
	forms []source.FunctionFormals,
	nilOwners []keyspace.Term,
	extras sealSourceExtras,
) source.Input {
	input := source.Input{Name: name}
	input.Keys = append([]source.KeyInput(nil), extras.keys...)
	input.ExactAtoms = append([]keyspace.LiteralValue(nil), extras.exactAtoms...)
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		spans := make([]source.Span, counts[family])
		for ordinal := range spans {
			line := uint32(ordinal + 1)
			spans[ordinal] = source.Span{File: name, StartLine: line, StartCol: 1, EndLine: line, EndCol: 1}
		}
		input.Families = append(input.Families, source.FamilySpans{Family: family, Spans: spans})
	}
	input.Bodies = make([]source.BodySource, len(rows))
	for index, terms := range rows {
		input.Bodies[index] = source.BodySource{
			Body:  keyspace.MakeTerm(keyspace.FamilyBody, uint32(index+1)),
			Terms: append([]keyspace.Term(nil), terms...),
		}
	}
	input.Binds = make([]source.BindCells, counts[keyspace.FamilyBind])
	for index := range input.Binds {
		input.Binds[index].Bind = keyspace.MakeTerm(keyspace.FamilyBind, uint32(index+1))
		if index < len(binds) {
			input.Binds[index].Cells = append([]keyspace.Term(nil), binds[index].Cells...)
		}
	}
	input.Functions = make([]source.FunctionFormals, counts[keyspace.FamilyFunction])
	for index := range input.Functions {
		input.Functions[index].Function = keyspace.MakeTerm(keyspace.FamilyFunction, uint32(index+1))
		if index < len(forms) {
			input.Functions[index].Formals = append([]keyspace.Term(nil), forms[index].Formals...)
		}
	}
	for ordinal := uint32(1); ordinal <= counts[keyspace.FamilyNil]; ordinal++ {
		owner := keyspace.MakeTerm(keyspace.FamilyBody, 1)
		if int(ordinal) <= len(nilOwners) {
			owner = nilOwners[ordinal-1]
		}
		input.Nil = append(input.Nil, source.NilLiteral{Owner: owner})
	}
	for ordinal := uint32(1); ordinal <= counts[keyspace.FamilyBool]; ordinal++ {
		owner := keyspace.MakeTerm(keyspace.FamilyBody, 1)
		if int(ordinal) <= len(extras.boolOwners) {
			owner = extras.boolOwners[ordinal-1]
		}
		input.Bool = append(input.Bool, source.BoolLiteral{Owner: owner, Value: ordinal&1 == 1})
	}
	for ordinal := uint32(1); ordinal <= counts[keyspace.FamilyInteger]; ordinal++ {
		owner := keyspace.MakeTerm(keyspace.FamilyBody, 1)
		if int(ordinal) <= len(extras.integerOwners) {
			owner = extras.integerOwners[ordinal-1]
		}
		input.Integer = append(input.Integer, source.IntegerLiteral{Owner: owner, Value: int64(ordinal)})
	}
	for ordinal := uint32(1); ordinal <= counts[keyspace.FamilyFloat]; ordinal++ {
		owner := keyspace.MakeTerm(keyspace.FamilyBody, 1)
		if int(ordinal) <= len(extras.floatOwners) {
			owner = extras.floatOwners[ordinal-1]
		}
		input.Float = append(input.Float, source.FloatLiteral{Owner: owner, Bits: uint64(ordinal)})
	}
	for ordinal := uint32(1); ordinal <= counts[keyspace.FamilyString]; ordinal++ {
		owner := keyspace.MakeTerm(keyspace.FamilyBody, 1)
		if int(ordinal) <= len(extras.stringOwners) {
			owner = extras.stringOwners[ordinal-1]
		}
		input.String = append(input.String, source.StringLiteral{Owner: owner, Value: "string"})
	}
	return input
}

func matrixCounts() (counts [keyspace.FamilyCount]uint32) {
	counts[keyspace.FamilyBody] = 1
	counts[keyspace.FamilyNil] = 2
	counts[keyspace.FamilyValues] = 2
	counts[keyspace.FamilyCell] = 1
	counts[keyspace.FamilyBind] = 1
	counts[keyspace.FamilyReturn] = 1
	return counts
}

func matrixFlow() authored.Input {
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	values := keyspace.MakeTerm(keyspace.FamilyValues, 1)
	returnValues := keyspace.MakeTerm(keyspace.FamilyValues, 2)
	nilTerm := keyspace.MakeTerm(keyspace.FamilyNil, 1)
	returnNil := keyspace.MakeTerm(keyspace.FamilyNil, 2)
	return authored.Input{
		Values: authored.ValuesInput{
			Rows: []authored.Value{
				{Owner: body, Fixed: authored.Range{End: 1}},
				{Owner: body, Fixed: authored.Range{Start: 1, End: 2}},
			},
			Terms: []keyspace.Term{nilTerm, returnNil},
		},
		Storage: authored.StorageInput{
			Cells: []authored.Cell{{Kind: authored.CellLocal, Body: body}},
			Binds: []authored.Bind{{Owner: body, Values: values}},
		},
		Control: authored.ControlInput{
			Returns: []authored.Return{{Owner: body, Values: returnValues}},
		},
	}
}

func TestSealFinalSourceExcludesOutcomeAndClosesRuntimeOperands(t *testing.T) {
	counts := matrixCounts()
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	bind := keyspace.MakeTerm(keyspace.FamilyBind, 1)
	returned := keyspace.MakeTerm(keyspace.FamilyReturn, 1)
	fixture := openSealFixture(t, "executable-outcome.lua", counts,
		[][]keyspace.Term{{bind, returned}}, matrixFlow(),
		[]source.BindCells{{Bind: bind, Cells: []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyCell, 1)}}}, nil, nil)

	result, err := Seal(fixture.sourceView, fixture.flow, fixture.bodies, fixture.forest, fixture.control,
		fixture.staticFinalize.View().ContentID(), fixture.moduleFinalize.View().ContentID(), fixture.paths)
	if err != nil {
		t.Fatalf("executable.Seal: %v", err)
	}
	if fixture.sourceView.Identity().FamilyCount(keyspace.FamilyOutcome) == 0 {
		t.Fatal("fixture did not commit a nonzero final Outcome family")
	}
	if result.FamilyCount(keyspace.FamilyOutcome) != 0 || result.Contains(keyspace.MakeTerm(keyspace.FamilyOutcome, 1)) {
		t.Fatal("Outcome entered pre-Outcome executable projection")
	}
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		if family == keyspace.FamilyOutcome {
			continue
		}
		if got, want := result.FamilyCount(family), fixture.sourceView.Identity().FamilyCount(family); got != want {
			t.Fatalf("FamilyCount(%d) = %d, want final Source pre-Outcome denominator %d", family, got, want)
		}
	}
	want := []keyspace.Term{
		body,
		keyspace.MakeTerm(keyspace.FamilyCell, 1),
		keyspace.MakeTerm(keyspace.FamilyNil, 1),
		keyspace.MakeTerm(keyspace.FamilyNil, 2),
		keyspace.MakeTerm(keyspace.FamilyValues, 1),
		keyspace.MakeTerm(keyspace.FamilyValues, 2),
		bind,
		returned,
	}
	for _, term := range want {
		if !result.Contains(term) {
			t.Fatalf("Executable(%08x) = false; runtime operand was not closed", uint32(term))
		}
	}
	if result.Count() != len(want) {
		t.Fatalf("executable Count = %d, want %d", result.Count(), len(want))
	}
}

func TestSealRejectsForeignEqualCardinalityProvenance(t *testing.T) {
	counts := matrixCounts()
	bind := keyspace.MakeTerm(keyspace.FamilyBind, 1)
	returned := keyspace.MakeTerm(keyspace.FamilyReturn, 1)
	rows := [][]keyspace.Term{{bind, returned}}
	binds := []source.BindCells{{Bind: bind, Cells: []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyCell, 1)}}}
	first := openSealFixture(t, "provenance-first.lua", counts, rows, matrixFlow(), binds, nil, nil)
	foreignSource := openSealFixture(t, "provenance-foreign.lua", counts, rows, matrixFlow(), binds, nil, nil)
	if first.forest.Count() != foreignSource.forest.Count() || first.control.NodeCount() != foreignSource.control.NodeCount() {
		t.Fatal("provenance fixtures are not equal-cardinality")
	}
	if _, err := Seal(first.sourceView, first.flow, first.bodies, foreignSource.forest, first.control,
		first.staticFinalize.View().ContentID(), first.moduleFinalize.View().ContentID(), first.paths); err == nil || !strings.Contains(err.Error(), "containment provenance") {
		t.Fatalf("foreign equal-cardinality containment splice was accepted or failed outside provenance fence: %v", err)
	}
	if _, err := Seal(first.sourceView, first.flow, first.bodies, first.forest, foreignSource.control,
		first.staticFinalize.View().ContentID(), first.moduleFinalize.View().ContentID(), first.paths); err == nil || !strings.Contains(err.Error(), "source-control provenance") {
		t.Fatalf("foreign equal-cardinality source-control splice was accepted or failed outside provenance fence: %v", err)
	}
	flowVariant := matrixFlow()
	flowVariant.Values.Rows[0].Fixed.End = 0
	flowVariant.Values.Rows[1].Fixed = authored.Range{End: 2}
	foreignFlow := openSealFixture(t, "provenance-first.lua", counts, rows, flowVariant, binds, nil, nil)
	if first.sourceView.Identity().ContentID() != foreignFlow.sourceView.Identity().ContentID() {
		t.Fatal("same-source Flow provenance fixture changed Source identity")
	}
	if first.flow.Cold().ContentID() == foreignFlow.flow.Cold().ContentID() {
		t.Fatal("same-source Flow provenance fixture did not change authored Flow identity")
	}
	if first.forest.Count() != foreignFlow.forest.Count() || first.control.NodeCount() != foreignFlow.control.NodeCount() {
		t.Fatal("same-source Flow provenance fixtures are not equal-cardinality")
	}
	if _, err := Seal(first.sourceView, first.flow, first.bodies, foreignFlow.forest, first.control,
		first.staticFinalize.View().ContentID(), first.moduleFinalize.View().ContentID(), first.paths); err == nil || !strings.Contains(err.Error(), "containment provenance") {
		t.Fatalf("foreign equal-cardinality Flow forest splice was accepted or failed outside provenance fence: %v", err)
	}
	if _, err := Seal(first.sourceView, first.flow, first.bodies, first.forest, foreignFlow.control,
		first.staticFinalize.View().ContentID(), first.moduleFinalize.View().ContentID(), first.paths); err == nil || !strings.Contains(err.Error(), "source-control provenance") {
		t.Fatalf("foreign equal-cardinality Flow source-control splice was accepted or failed outside provenance fence: %v", err)
	}
}

func fullOperandCounts() (counts [keyspace.FamilyCount]uint32) {
	counts[keyspace.FamilyBody] = 1
	counts[keyspace.FamilyNil] = 12
	counts[keyspace.FamilyBool] = 2
	counts[keyspace.FamilyInteger] = 3
	counts[keyspace.FamilyFloat] = 1
	counts[keyspace.FamilyString] = 5
	counts[keyspace.FamilyValues] = 10
	counts[keyspace.FamilyLensExact] = 2
	counts[keyspace.FamilyLensKey] = 2
	counts[keyspace.FamilyReturn] = 1
	counts[keyspace.FamilyCell] = 5
	counts[keyspace.FamilyRead] = 3
	counts[keyspace.FamilyVararg] = 1
	counts[keyspace.FamilyUnary] = 1
	counts[keyspace.FamilyBinary] = 1
	counts[keyspace.FamilySelect] = 1
	counts[keyspace.FamilyBind] = 3
	counts[keyspace.FamilyAssign] = 1
	counts[keyspace.FamilyWrite] = 3
	counts[keyspace.FamilyCall] = 1
	counts[keyspace.FamilyTable] = 1
	counts[keyspace.FamilyTableField] = 4
	counts[keyspace.FamilyValueClaim] = 1
	counts[keyspace.FamilyTypeValue] = 1
	counts[keyspace.FamilyKey] = 3
	counts[keyspace.FamilyTypePrimitive] = 1
	return counts
}

func fullOperandFlow() authored.Input {
	term := keyspace.MakeTerm
	body := term(keyspace.FamilyBody, 1)
	values := func(ordinal uint32) keyspace.Term { return term(keyspace.FamilyValues, ordinal) }
	nilTerm := func(ordinal uint32) keyspace.Term { return term(keyspace.FamilyNil, ordinal) }
	cell := func(ordinal uint32) keyspace.Term { return term(keyspace.FamilyCell, ordinal) }
	return authored.Input{
		Values: authored.ValuesInput{
			Rows: []authored.Value{
				{Owner: body, Fixed: authored.Range{Start: 0, End: 1}},
				{Owner: body, Fixed: authored.Range{Start: 1, End: 2}},
				{Owner: body, Fixed: authored.Range{Start: 2, End: 3}},
				{Owner: body, Fixed: authored.Range{Start: 3, End: 4}},
				{Owner: body, Fixed: authored.Range{Start: 4, End: 4}, Tail: term(keyspace.FamilyVararg, 1)},
				{Owner: body, Fixed: authored.Range{Start: 4, End: 12}},
				{Owner: body, Fixed: authored.Range{Start: 12, End: 13}},
				{Owner: body, Fixed: authored.Range{Start: 13, End: 14}},
				{Owner: body, Fixed: authored.Range{Start: 14, End: 15}},
				{Owner: body, Fixed: authored.Range{Start: 15, End: 16}},
			},
			Terms: []keyspace.Term{
				nilTerm(1), nilTerm(2), nilTerm(3), nilTerm(4),
				term(keyspace.FamilyTable, 1), term(keyspace.FamilyUnary, 1),
				term(keyspace.FamilyBinary, 1), term(keyspace.FamilySelect, 1),
				term(keyspace.FamilyValueClaim, 1), term(keyspace.FamilyTypeValue, 1),
				term(keyspace.FamilyRead, 2), term(keyspace.FamilyRead, 3),
				nilTerm(9), nilTerm(10), nilTerm(11), nilTerm(12),
			},
		},
		Access: authored.AccessInput{
			Exact: []authored.ExactLens{
				{Owner: body, Base: nilTerm(5), Source: term(keyspace.FamilyKey, 1), Kind: kind.FieldName},
				{Owner: body, Base: nilTerm(6), Source: term(keyspace.FamilyInteger, 2), Kind: kind.FieldExact},
			},
			Dynamic: []authored.DynamicLens{
				{Owner: body, Base: nilTerm(7), Key: term(keyspace.FamilyString, 2)},
				{Owner: body, Base: nilTerm(8), Key: term(keyspace.FamilyString, 3)},
			},
		},
		Storage: authored.StorageInput{
			Cells: []authored.Cell{
				{Kind: authored.CellLocal, Body: body},
				{Kind: authored.CellLocal, Body: body},
				{Kind: authored.CellLocal, Body: body},
				{Kind: authored.CellLocal, Body: body},
				{Kind: authored.CellGlobal, Key: keyspace.Key(1)},
			},
			Reads: []authored.Read{
				{Owner: body, Source: term(keyspace.FamilyLensExact, 1)},
				{Owner: body, Source: term(keyspace.FamilyLensKey, 1)},
				{Owner: body, Source: cell(5)},
			},
			Varargs: []authored.Vararg{{Owner: body, Cell: cell(1)}},
			Binds: []authored.Bind{
				{Owner: body, Values: values(1)},
				{Owner: body, Values: values(2)},
				{Owner: body, Values: values(3)},
			},
			Assigns: []authored.Assign{{Owner: body, Values: values(4)}},
			Writes: []authored.Write{
				{Assign: term(keyspace.FamilyAssign, 1), Target: cell(2)},
				{Assign: term(keyspace.FamilyAssign, 1), Target: term(keyspace.FamilyLensExact, 2)},
				{Assign: term(keyspace.FamilyAssign, 1), Target: term(keyspace.FamilyLensKey, 2)},
			},
		},
		Tables: authored.TablesInput{
			Rows: []authored.Table{{Owner: body, Fields: authored.Range{End: 4}}},
			Fields: []authored.Field{
				{Table: term(keyspace.FamilyTable, 1), Key: term(keyspace.FamilyKey, 2), Values: values(7), Kind: kind.FieldList},
				{Table: term(keyspace.FamilyTable, 1), Key: term(keyspace.FamilyKey, 3), Values: values(8), Kind: kind.FieldName},
				{Table: term(keyspace.FamilyTable, 1), Key: term(keyspace.FamilyInteger, 3), Values: values(9), Kind: kind.FieldExact},
				{Table: term(keyspace.FamilyTable, 1), Key: term(keyspace.FamilyString, 4), Values: values(10), Kind: kind.FieldKey},
			},
			Order: []keyspace.Term{
				term(keyspace.FamilyTableField, 1), term(keyspace.FamilyTableField, 2),
				term(keyspace.FamilyTableField, 3), term(keyspace.FamilyTableField, 4),
			},
		},
		Calls: []authored.Call{{
			Owner: body, Callee: term(keyspace.FamilyRead, 1),
			Receiver: term(keyspace.FamilyNil, 5), Actuals: values(5),
		}},
		Control: authored.ControlInput{Returns: []authored.Return{{Owner: body, Values: values(6)}}},
		Operators: authored.OperatorsInput{
			Unaries:  []authored.Unary{{Owner: body, Op: kind.UnaryNeg, Operand: term(keyspace.FamilyBool, 1)}},
			Binaries: []authored.Binary{{Owner: body, Op: kind.BinaryAdd, Left: term(keyspace.FamilyBool, 2), Right: term(keyspace.FamilyInteger, 1)}},
			Selects:  []authored.Select{{Owner: body, Op: kind.SelectAnd, Left: term(keyspace.FamilyFloat, 1), Right: term(keyspace.FamilyString, 1)}},
		},
		Claims:     []authored.ValueClaim{{Owner: body, Operand: term(keyspace.FamilyString, 5), Kind: kind.ValueClaimNonNil}},
		TypeValues: []authored.TypeValue{{Owner: body}},
	}
}

func TestSealClosesCompleteRuntimeOperandMatrix(t *testing.T) {
	counts := fullOperandCounts()
	term := keyspace.MakeTerm
	body := term(keyspace.FamilyBody, 1)
	bindRoots := []keyspace.Term{term(keyspace.FamilyBind, 1), term(keyspace.FamilyBind, 2), term(keyspace.FamilyBind, 3)}
	rows := [][]keyspace.Term{{bindRoots[0], bindRoots[1], bindRoots[2], term(keyspace.FamilyAssign, 1), term(keyspace.FamilyCall, 1), term(keyspace.FamilyReturn, 1)}}
	extras := sealSourceExtras{
		keys: []source.KeyInput{
			source.NameKey(body, "lens-key"), source.NameKey(body, "list-key"),
			source.NameKey(body, "name-key"),
		},
		exactAtoms: []keyspace.LiteralValue{
			{Kind: keyspace.LiteralString, String: "lens-key"},
			{Kind: keyspace.LiteralString, String: "list-key"},
			{Kind: keyspace.LiteralString, String: "name-key"},
		},
	}
	fixture := openSealFixtureWithSource(t, "complete-runtime-matrix.lua", counts, rows, fullOperandFlow(),
		[]source.BindCells{
			{Bind: bindRoots[0], Cells: []keyspace.Term{term(keyspace.FamilyCell, 2)}},
			{Bind: bindRoots[1], Cells: []keyspace.Term{term(keyspace.FamilyCell, 3)}},
			{Bind: bindRoots[2], Cells: []keyspace.Term{term(keyspace.FamilyCell, 4)}},
		}, nil, nil, extras)
	result, err := Seal(fixture.sourceView, fixture.flow, fixture.bodies, fixture.forest, fixture.control,
		fixture.staticFinalize.View().ContentID(), fixture.moduleFinalize.View().ContentID(), fixture.paths)
	if err != nil {
		t.Fatalf("complete operand executable.Seal: %v", err)
	}
	wantCount := 0
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		for ordinal := uint32(1); ordinal <= counts[family]; ordinal++ {
			term := term(family, ordinal)
			want := family != keyspace.FamilyKey && family != keyspace.FamilyTypePrimitive
			if family == keyspace.FamilyCell && ordinal == 5 {
				want = false
			}
			if want {
				wantCount++
			}
			got := result.Contains(term)
			if got != want {
				t.Fatalf("Executable(%d,%d) = %v, want %v", family, ordinal, got, want)
			}
		}
	}
	if result.Contains(term(keyspace.FamilyCell, 5)) {
		t.Fatal("global Cell became executable")
	}
	if got := result.Count(); got != wantCount {
		t.Fatalf("complete runtime executable Count = %d, want %d", got, wantCount)
	}
}

func functionClosureCounts() (counts [keyspace.FamilyCount]uint32) {
	counts[keyspace.FamilyBody] = 2
	counts[keyspace.FamilyNil] = 1
	counts[keyspace.FamilyValues] = 2
	counts[keyspace.FamilyCell] = 4
	counts[keyspace.FamilyVararg] = 1
	counts[keyspace.FamilyBind] = 1
	counts[keyspace.FamilyFunction] = 1
	counts[keyspace.FamilyReturn] = 1
	return counts
}

func functionClosureFlow() authored.Input {
	term := keyspace.MakeTerm
	body1 := term(keyspace.FamilyBody, 1)
	body2 := term(keyspace.FamilyBody, 2)
	return authored.Input{
		Values: authored.ValuesInput{
			Rows:  []authored.Value{{Owner: body1, Fixed: authored.Range{End: 1}}, {Owner: body2, Fixed: authored.Range{Start: 1, End: 3}}},
			Terms: []keyspace.Term{term(keyspace.FamilyFunction, 1), term(keyspace.FamilyNil, 1), term(keyspace.FamilyVararg, 1)},
		},
		Storage: authored.StorageInput{
			Cells: []authored.Cell{
				{Kind: authored.CellLocal, Body: body1},
				{Kind: authored.CellLocal, Body: body2},
				{Kind: authored.CellLocal, Body: body2},
				{Kind: authored.CellLocal, Body: body2},
			},
			Varargs: []authored.Vararg{{Owner: body2, Cell: term(keyspace.FamilyCell, 4)}},
			Binds:   []authored.Bind{{Owner: body1, Values: term(keyspace.FamilyValues, 1)}},
		},
		Functions: authored.FunctionsInput{
			Rows: []authored.Function{{
				Owner: body1, Body: body2, Vararg: term(keyspace.FamilyCell, 4),
				Captures: authored.Range{End: 1},
			}},
			Captures: []authored.Capture{{Inner: term(keyspace.FamilyCell, 3), Outer: term(keyspace.FamilyCell, 1)}},
		},
		Control: authored.ControlInput{
			Returns: []authored.Return{{Owner: body2, Values: term(keyspace.FamilyValues, 2)}},
		},
	}
}

func branchCounts() (counts [keyspace.FamilyCount]uint32) {
	counts[keyspace.FamilyBody] = 4
	counts[keyspace.FamilyNil] = 2
	counts[keyspace.FamilyBranch] = 1
	counts[keyspace.FamilyLoop] = 1
	return counts
}

func branchFlowKind(loopBody, whenTrue, whenFalse keyspace.Term, loopKind kind.LoopKind) authored.Input {
	body1 := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	body3 := keyspace.MakeTerm(keyspace.FamilyBody, 3)
	return authored.Input{
		Control: authored.ControlInput{
			Branches: []authored.Branch{{
				Owner: body1, Condition: keyspace.MakeTerm(keyspace.FamilyNil, 1),
				WhenTrue: whenTrue, WhenFalse: whenFalse,
			}},
			Loops: []authored.Loop{{
				Owner: body3, Body: loopBody, Kind: loopKind,
				Control: keyspace.MakeTerm(keyspace.FamilyNil, 2),
			}},
		},
	}
}

func branchFlow(loopBody, whenTrue, whenFalse keyspace.Term) authored.Input {
	return branchFlowKind(loopBody, whenTrue, whenFalse, kind.LoopWhile)
}

func TestSealClosesBranchAndRepeatLoopControls(t *testing.T) {
	counts := branchCounts()
	branch := keyspace.MakeTerm(keyspace.FamilyBranch, 1)
	loop := keyspace.MakeTerm(keyspace.FamilyLoop, 1)
	body1 := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	body3 := keyspace.MakeTerm(keyspace.FamilyBody, 3)
	rows := [][]keyspace.Term{{branch}, nil, {loop}, nil}
	fixture := openSealFixture(t, "branch-repeat.lua", counts, rows,
		branchFlowKind(keyspace.MakeTerm(keyspace.FamilyBody, 4), keyspace.MakeTerm(keyspace.FamilyBody, 2), keyspace.MakeTerm(keyspace.FamilyBody, 3), kind.LoopRepeat),
		nil, nil, []keyspace.Term{body1, keyspace.MakeTerm(keyspace.FamilyBody, 4)})
	result, err := Seal(fixture.sourceView, fixture.flow, fixture.bodies, fixture.forest, fixture.control,
		fixture.staticFinalize.View().ContentID(), fixture.moduleFinalize.View().ContentID(), fixture.paths)
	if err != nil {
		t.Fatalf("branch/repeat executable.Seal: %v", err)
	}
	for _, term := range []keyspace.Term{
		body1, keyspace.MakeTerm(keyspace.FamilyBody, 2), body3, keyspace.MakeTerm(keyspace.FamilyBody, 4),
		keyspace.MakeTerm(keyspace.FamilyNil, 1), keyspace.MakeTerm(keyspace.FamilyNil, 2), branch, loop,
	} {
		if !result.Contains(term) {
			t.Fatalf("branch/repeat left %v nonexecutable", term)
		}
	}
	if got, want := result.Count(), 8; got != want {
		t.Fatalf("branch/repeat executable Count = %d, want %d", got, want)
	}
}

func TestValidateBodyRootsRejectsSourceBodyParentDisagreement(t *testing.T) {
	counts := branchCounts()
	branch := keyspace.MakeTerm(keyspace.FamilyBranch, 1)
	loop := keyspace.MakeTerm(keyspace.FamilyLoop, 1)
	rows := [][]keyspace.Term{{branch}, nil, {loop}, nil}
	first := openSealFixture(t, "body-parent-disagreement.lua", counts, rows,
		branchFlow(keyspace.MakeTerm(keyspace.FamilyBody, 4), keyspace.MakeTerm(keyspace.FamilyBody, 2), keyspace.MakeTerm(keyspace.FamilyBody, 3)), nil, nil,
		[]keyspace.Term{keyspace.MakeTerm(keyspace.FamilyBody, 1), keyspace.MakeTerm(keyspace.FamilyBody, 3)})
	foreign := openSealFixture(t, "body-parent-disagreement.lua", counts, rows,
		branchFlow(keyspace.MakeTerm(keyspace.FamilyBody, 2), keyspace.MakeTerm(keyspace.FamilyBody, 3), keyspace.MakeTerm(keyspace.FamilyBody, 4)), nil, nil,
		[]keyspace.Term{keyspace.MakeTerm(keyspace.FamilyBody, 1), keyspace.MakeTerm(keyspace.FamilyBody, 3)})
	if first.sourceView.Identity().ContentID() != foreign.sourceView.Identity().ContentID() {
		t.Fatal("Body-parent disagreement fixture changed Source identity")
	}
	body2 := keyspace.MakeTerm(keyspace.FamilyBody, 2)
	firstParent, firstOK := first.forest.Parent(body2)
	foreignParent, foreignOK := foreign.forest.Parent(body2)
	if !firstOK || !foreignOK || firstParent == foreignParent {
		t.Fatalf("Body-parent fixtures did not disagree: first=%v/%v foreign=%v/%v", firstParent, firstOK, foreignParent, foreignOK)
	}
	if _, err := validateBodyRoots(first.bodies, foreign.forest, first.control, counts); err == nil || !strings.Contains(err.Error(), "Body parent disagrees") {
		t.Fatalf("foreign Body parent was accepted or failed outside exact parent check: %v", err)
	}
}
