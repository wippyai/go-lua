package evaluation

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/binding"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/body"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/candidates"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/containment"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/control"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/executable"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/outcome"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/position"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/sourcecontrol"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/imports"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/analysis/program/static"
)

// pendingFixture is deliberately the complete owner chain. Pending tests must
// exercise the same committed Source/Flow boundary that production assembly
// will use; a fabricated Result or an ordinary Session is not evidence for
// SealPending.
type pendingFixture struct {
	sourceView source.View
	flowView   authored.View
	pending    *Pending
	executable *executable.Result
	candidates *candidates.Result
	staticID   identity.ContentID
	moduleID   identity.ContentID

	staticFinalize static.Finalizer
	flowFinalize   authored.Finalizer
	moduleFinalize imports.Finalizer
}

type pendingSourceExtras struct {
	keys          []source.KeyInput
	exactAtoms    []keyspace.LiteralValue
	boolOwners    []keyspace.Term
	integerOwners []keyspace.Term
	floatOwners   []keyspace.Term
	stringOwners  []keyspace.Term
}

func openPendingFixture(
	t *testing.T,
	name string,
	counts [keyspace.FamilyCount]uint32,
	rows [][]keyspace.Term,
	flowInput authored.Input,
	binds []source.BindCells,
	forms []source.FunctionFormals,
	nilOwners []keyspace.Term,
	extras pendingSourceExtras,
) *pendingFixture {
	t.Helper()
	if counts[keyspace.FamilyBody] == 0 || len(rows) != int(counts[keyspace.FamilyBody]) {
		t.Fatal("pending fixture requires one Source Body row per Body")
	}
	flowInput.Counts = counts
	if name == "" {
		name = "pending-fixture.lua"
	}

	sourceInput := pendingSourceInput(name, counts, rows, binds, forms, nilOwners, extras)
	sourceDraft, err := source.Build(sourceInput)
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
		staticInput.Types.Primitive = make([]static.Primitive, counts[keyspace.FamilyTypePrimitive])
		for index := range staticInput.Types.Primitive {
			staticInput.Types.Primitive[index] = static.Primitive{Kind: static.PrimitiveNumber}
		}
	}
	if counts[keyspace.FamilyFunction] != 0 {
		staticInput.Contracts.Function = make([]static.FunctionContract, counts[keyspace.FamilyFunction])
	}
	if counts[keyspace.FamilyCall] != 0 {
		staticInput.Contracts.Call = make([]static.CallContract, counts[keyspace.FamilyCall])
	}
	staticInput.Counts[keyspace.FamilyFunction] = uint32(len(staticInput.Contracts.Function))
	staticInput.Counts[keyspace.FamilyCall] = uint32(len(staticInput.Contracts.Call))
	staticInput.Counts[keyspace.FamilyTypePrimitive] = uint32(len(staticInput.Types.Primitive))
	if counts[keyspace.FamilyTypeValue] != 0 && counts[keyspace.FamilyTypePrimitive] != 0 {
		staticInput.Counts[keyspace.FamilyTypeValue] = counts[keyspace.FamilyTypeValue]
		staticInput.Operands.TypeValue = make([]static.TypeValueTarget, counts[keyspace.FamilyTypeValue])
		for index := range staticInput.Operands.TypeValue {
			staticInput.Operands.TypeValue[index] = static.TypeValueTarget{
				Target: keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1),
			}
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

	flowDraft, err := authored.Build(flowInput)
	if err != nil {
		_ = sourceFinalize.Abort()
		_ = staticFinalize.Abort()
		t.Fatalf("authored.Build: %v", err)
	}
	flowFinalize, err := flowDraft.Finalizer()
	if err != nil {
		_ = sourceFinalize.Abort()
		_ = staticFinalize.Abort()
		t.Fatalf("authored.Finalizer: %v", err)
	}
	flowView := flowFinalize.View()
	entry := keyspace.MakeTerm(keyspace.FamilyBody, 1)

	bodies, err := body.Seal(preimage, flowView, staticFinalize.View(), entry)
	if err != nil {
		closePendingFinalizers(sourceFinalize, staticFinalize, flowFinalize, imports.Finalizer{})
		t.Fatalf("body.Seal: %v", err)
	}
	bindingResult, err := binding.Seal(preimage, flowView, bodies, entry)
	if err != nil {
		closePendingFinalizers(sourceFinalize, staticFinalize, flowFinalize, imports.Finalizer{})
		t.Fatalf("binding.Seal: %v", err)
	}

	moduleDraft, err := imports.Build(imports.Input{})
	if err != nil {
		closePendingFinalizers(sourceFinalize, staticFinalize, flowFinalize, imports.Finalizer{})
		t.Fatalf("imports.Build: %v", err)
	}
	moduleFinalize, err := moduleDraft.Finalizer()
	if err != nil {
		closePendingFinalizers(sourceFinalize, staticFinalize, flowFinalize, imports.Finalizer{})
		t.Fatalf("imports.Finalizer: %v", err)
	}
	moduleView := moduleFinalize.View()

	forest, _, err := containment.Prove(preimage, staticFinalize.View(), flowView, bodies, bindingResult, moduleView, entry)
	if err != nil {
		closePendingFinalizers(sourceFinalize, staticFinalize, flowFinalize, moduleFinalize)
		t.Fatalf("containment.Prove: %v", err)
	}
	shape, err := control.Seal(preimage, flowView, bodies, bindingResult, forest,
		staticFinalize.View().ContentID(), moduleView.ContentID())
	if err != nil {
		closePendingFinalizers(sourceFinalize, staticFinalize, flowFinalize, moduleFinalize)
		t.Fatalf("control.Seal: %v", err)
	}
	outcomes, err := outcome.Seal(preimage.Identity(), flowView, bodies, shape,
		staticFinalize.View().ContentID(), moduleView.ContentID())
	if err != nil {
		closePendingFinalizers(sourceFinalize, staticFinalize, flowFinalize, moduleFinalize)
		t.Fatalf("outcome.Seal: %v", err)
	}
	index, err := position.Seal(preimage, flowView, bodies, forest, outcomes, entry,
		staticFinalize.View().ContentID(), moduleView.ContentID())
	if err != nil {
		closePendingFinalizers(sourceFinalize, staticFinalize, flowFinalize, moduleFinalize)
		t.Fatalf("position.Seal: %v", err)
	}
	sourceComponent, err := sourceFinalize.Commit(index)
	if err != nil {
		closePendingFinalizers(source.Finalizer{}, staticFinalize, flowFinalize, moduleFinalize)
		t.Fatalf("source.Commit: %v", err)
	}
	sourceView := sourceComponent.View()
	controlResult, err := sourcecontrol.Seal(sourceView, flowView, bodies, forest, shape, entry,
		staticFinalize.View().ContentID(), moduleView.ContentID())
	if err != nil {
		closePendingFinalizers(source.Finalizer{}, staticFinalize, flowFinalize, moduleFinalize)
		t.Fatalf("sourcecontrol.Seal: %v", err)
	}
	executableResult, err := executable.Seal(sourceView, flowView, forest, controlResult,
		staticFinalize.View().ContentID(), moduleView.ContentID())
	if err != nil {
		closePendingFinalizers(source.Finalizer{}, staticFinalize, flowFinalize, moduleFinalize)
		t.Fatalf("executable.Seal: %v", err)
	}
	candidateResult, err := candidates.Seal(sourceView.Identity(), flowView, executableResult,
		staticFinalize.View().ContentID(), moduleView.ContentID())
	if err != nil {
		closePendingFinalizers(source.Finalizer{}, staticFinalize, flowFinalize, moduleFinalize)
		t.Fatalf("candidates.Seal: %v", err)
	}
	pending, err := SealPending(sourceView, flowView, executableResult, candidateResult,
		staticFinalize.View().ContentID(), moduleView.ContentID())
	if err != nil {
		closePendingFinalizers(source.Finalizer{}, staticFinalize, flowFinalize, moduleFinalize)
		t.Fatalf("SealPending: %v", err)
	}
	fixture := &pendingFixture{
		sourceView: sourceView, flowView: flowView, pending: pending,
		executable: executableResult, candidates: candidateResult,
		staticID: staticFinalize.View().ContentID(), moduleID: moduleView.ContentID(),
		staticFinalize: staticFinalize, flowFinalize: flowFinalize, moduleFinalize: moduleFinalize,
	}
	t.Cleanup(func() {
		closePendingFinalizers(source.Finalizer{}, fixture.staticFinalize, fixture.flowFinalize, fixture.moduleFinalize)
	})
	return fixture
}

func closePendingFinalizers(sourceFinalize source.Finalizer, staticFinalize static.Finalizer, flowFinalize authored.Finalizer, moduleFinalize imports.Finalizer) {
	_ = moduleFinalize.Abort()
	_ = flowFinalize.Abort()
	_ = staticFinalize.Abort()
	_ = sourceFinalize.Abort()
}

func pendingSourceInput(
	name string,
	counts [keyspace.FamilyCount]uint32,
	rows [][]keyspace.Term,
	binds []source.BindCells,
	forms []source.FunctionFormals,
	nilOwners []keyspace.Term,
	extras pendingSourceExtras,
) source.Input {
	input := source.Input{Name: name, Keys: append([]source.KeyInput(nil), extras.keys...), ExactAtoms: append([]keyspace.LiteralValue(nil), extras.exactAtoms...)}
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		spans := make([]source.Span, counts[family])
		for index := range spans {
			line := uint32(index + 1)
			spans[index] = source.Span{File: name, StartLine: line, StartCol: 1, EndLine: line, EndCol: 1}
		}
		input.Families = append(input.Families, source.FamilySpans{Family: family, Spans: spans})
	}
	input.Bodies = make([]source.BodySource, len(rows))
	for index, terms := range rows {
		input.Bodies[index] = source.BodySource{Body: keyspace.MakeTerm(keyspace.FamilyBody, uint32(index+1)), Terms: append([]keyspace.Term(nil), terms...)}
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

func pendingTerm(family keyspace.Family, ordinal uint32) keyspace.Term {
	return keyspace.MakeTerm(family, ordinal)
}

// pendingRuntimeMatrixCounts deliberately gives each candidate family room
// for its own authored row. Later tests can change operations without changing
// the owner-chain builder or its denominator discipline.
func pendingRuntimeMatrixCounts() (counts [keyspace.FamilyCount]uint32) {
	counts[keyspace.FamilyBody] = 1
	counts[keyspace.FamilyNil] = 19
	counts[keyspace.FamilyBool] = 2
	counts[keyspace.FamilyInteger] = 4
	counts[keyspace.FamilyFloat] = 1
	counts[keyspace.FamilyString] = 7
	counts[keyspace.FamilyValues] = 12
	counts[keyspace.FamilyLensExact] = 3
	counts[keyspace.FamilyLensKey] = 2
	counts[keyspace.FamilyReturn] = 1
	counts[keyspace.FamilyCell] = 5
	counts[keyspace.FamilyRead] = 6
	counts[keyspace.FamilyVararg] = 1
	counts[keyspace.FamilyUnary] = 3
	counts[keyspace.FamilyBinary] = 5
	counts[keyspace.FamilySelect] = 1
	counts[keyspace.FamilyBind] = 3
	counts[keyspace.FamilyAssign] = 1
	counts[keyspace.FamilyWrite] = 4
	counts[keyspace.FamilyCall] = 3
	counts[keyspace.FamilyTable] = 1
	counts[keyspace.FamilyTableField] = 4
	counts[keyspace.FamilyValueClaim] = 1
	counts[keyspace.FamilyTypeValue] = 1
	counts[keyspace.FamilyKey] = 3
	counts[keyspace.FamilyTypePrimitive] = 1
	return counts
}

// pendingRuntimeMatrixFlow places every semantic composite under exactly one
// owner edge while retaining the source-statement roots needed by the real
// control and executable seals. The matrix intentionally includes method
// calls, a Values tail, all table field modes, assignment lenses, and every
// non-loop candidate plane except GenericLoop (covered by the loop fixture).
func pendingRuntimeMatrixFlow() authored.Input {
	term := pendingTerm
	body := term(keyspace.FamilyBody, 1)
	values := func(index uint32) keyspace.Term { return term(keyspace.FamilyValues, index) }
	nilTerm := func(index uint32) keyspace.Term { return term(keyspace.FamilyNil, index) }
	return authored.Input{
		Values: authored.ValuesInput{
			Rows: []authored.Value{
				{Owner: body, Fixed: authored.Range{End: 1}},
				{Owner: body, Fixed: authored.Range{Start: 1, End: 2}},
				{Owner: body, Fixed: authored.Range{Start: 2, End: 3}},
				{Owner: body, Fixed: authored.Range{Start: 3, End: 4}},
				{Owner: body, Fixed: authored.Range{Start: 4, End: 5}, Tail: term(keyspace.FamilyVararg, 1)},
				{Owner: body, Fixed: authored.Range{Start: 5, End: 13}},
				{Owner: body, Fixed: authored.Range{Start: 13, End: 14}},
				{Owner: body, Fixed: authored.Range{Start: 14, End: 15}},
				{Owner: body, Fixed: authored.Range{Start: 15, End: 16}},
				{Owner: body, Fixed: authored.Range{Start: 16, End: 17}},
				{Owner: body, Fixed: authored.Range{Start: 17, End: 18}},
				{Owner: body, Fixed: authored.Range{Start: 18, End: 23}},
			},
			Terms: []keyspace.Term{
				nilTerm(1), nilTerm(2), nilTerm(3), nilTerm(4),
				term(keyspace.FamilyRead, 2), term(keyspace.FamilyTable, 1), term(keyspace.FamilyUnary, 1),
				term(keyspace.FamilyBinary, 1), term(keyspace.FamilyFloat, 1),
				term(keyspace.FamilyValueClaim, 1), term(keyspace.FamilyTypeValue, 1),
				term(keyspace.FamilyRead, 3), term(keyspace.FamilyCall, 2),
				nilTerm(10), nilTerm(11), nilTerm(12), nilTerm(13), term(keyspace.FamilyString, 6), nilTerm(15), nilTerm(16), nilTerm(19), nilTerm(14), nilTerm(5),
			},
		},
		Access: authored.AccessInput{
			Exact: []authored.ExactLens{
				{Owner: body, Base: term(keyspace.FamilyRead, 6), Source: term(keyspace.FamilyKey, 1), Kind: kind.FieldName},
				{Owner: body, Base: nilTerm(6), Source: term(keyspace.FamilyString, 4), Kind: kind.FieldExact},
				{Owner: body, Base: nilTerm(7), Source: term(keyspace.FamilyUnary, 3), Kind: kind.FieldExact},
			},
			Dynamic: []authored.DynamicLens{
				{Owner: body, Base: nilTerm(8), Key: term(keyspace.FamilyString, 1)},
				{Owner: body, Base: nilTerm(9), Key: term(keyspace.FamilyString, 2)},
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
				{Owner: body, Source: term(keyspace.FamilyCell, 5)},
				{Owner: body, Source: term(keyspace.FamilyCell, 5)},
				{Owner: body, Source: term(keyspace.FamilyCell, 5)},
				{Owner: body, Source: term(keyspace.FamilyCell, 5)},
			},
			Varargs: []authored.Vararg{{Owner: body, Cell: term(keyspace.FamilyCell, 1)}},
			Binds: []authored.Bind{
				{Owner: body, Values: values(1)},
				{Owner: body, Values: values(2)},
				{Owner: body, Values: values(3)},
			},
			Assigns: []authored.Assign{{Owner: body, Values: values(4)}},
			Writes: []authored.Write{
				{Assign: term(keyspace.FamilyAssign, 1), Target: term(keyspace.FamilyCell, 2)},
				{Assign: term(keyspace.FamilyAssign, 1), Target: term(keyspace.FamilyLensExact, 2)},
				{Assign: term(keyspace.FamilyAssign, 1), Target: term(keyspace.FamilyLensKey, 2)},
				{Assign: term(keyspace.FamilyAssign, 1), Target: term(keyspace.FamilyLensExact, 3)},
			},
		},
		Tables: authored.TablesInput{
			Rows: []authored.Table{{Owner: body, Fields: authored.Range{End: 4}}},
			Fields: []authored.Field{
				{Table: term(keyspace.FamilyTable, 1), Key: term(keyspace.FamilyKey, 2), Values: values(7), Kind: kind.FieldList},
				{Table: term(keyspace.FamilyTable, 1), Key: term(keyspace.FamilyKey, 3), Values: values(8), Kind: kind.FieldName},
				{Table: term(keyspace.FamilyTable, 1), Key: term(keyspace.FamilyInteger, 3), Values: values(9), Kind: kind.FieldExact},
				{Table: term(keyspace.FamilyTable, 1), Key: term(keyspace.FamilyString, 3), Values: values(10), Kind: kind.FieldKey},
			},
			Order: []keyspace.Term{term(keyspace.FamilyTableField, 1), term(keyspace.FamilyTableField, 2), term(keyspace.FamilyTableField, 3), term(keyspace.FamilyTableField, 4)},
		},
		Calls: []authored.Call{
			{Owner: body, Callee: term(keyspace.FamilyRead, 1), Receiver: term(keyspace.FamilyRead, 6), Actuals: values(5)},
			{Owner: body, Callee: term(keyspace.FamilyRead, 4), Actuals: values(11)},
			{Owner: body, Callee: term(keyspace.FamilyRead, 5), Actuals: values(12)},
		},
		Control: authored.ControlInput{Returns: []authored.Return{{Owner: body, Values: values(6)}}},
		Operators: authored.OperatorsInput{
			Unaries: []authored.Unary{
				{Owner: body, Op: kind.UnaryNeg, Operand: term(keyspace.FamilyBool, 1)},
				{Owner: body, Op: kind.UnaryLen, Operand: term(keyspace.FamilyString, 7)},
				{Owner: body, Op: kind.UnaryNeg, Operand: term(keyspace.FamilyInteger, 4)},
			},
			Binaries: []authored.Binary{
				{Owner: body, Op: kind.BinaryAdd, Left: term(keyspace.FamilyBinary, 2), Right: term(keyspace.FamilyBinary, 3)},
				{Owner: body, Op: kind.BinaryBitAnd, Left: term(keyspace.FamilyUnary, 2), Right: term(keyspace.FamilyBinary, 4)},
				{Owner: body, Op: kind.BinaryConcat, Left: term(keyspace.FamilyInteger, 1), Right: term(keyspace.FamilyBinary, 5)},
				{Owner: body, Op: kind.BinaryEqual, Left: term(keyspace.FamilyBool, 2), Right: term(keyspace.FamilyInteger, 2)},
				{Owner: body, Op: kind.BinaryLess, Left: term(keyspace.FamilyCall, 3), Right: term(keyspace.FamilySelect, 1)},
			},
			Selects: []authored.Select{{Owner: body, Op: kind.SelectAnd, Left: nilTerm(17), Right: nilTerm(18)}},
		},
		Claims:     []authored.ValueClaim{{Owner: body, Operand: term(keyspace.FamilyString, 5), Kind: kind.ValueClaimNonNil}},
		TypeValues: []authored.TypeValue{{Owner: body}},
	}
}

func pendingRuntimeMatrixRows() [][]keyspace.Term {
	term := pendingTerm
	return [][]keyspace.Term{{
		term(keyspace.FamilyBind, 1), term(keyspace.FamilyBind, 2), term(keyspace.FamilyBind, 3),
		term(keyspace.FamilyAssign, 1), term(keyspace.FamilyCall, 1),
		term(keyspace.FamilyReturn, 1),
	}}
}

func TestSealPendingProductionRuntimeMatrixBuilds(t *testing.T) {
	counts := pendingRuntimeMatrixCounts()
	body := pendingTerm(keyspace.FamilyBody, 1)
	binds := []source.BindCells{
		{Bind: pendingTerm(keyspace.FamilyBind, 1), Cells: []keyspace.Term{pendingTerm(keyspace.FamilyCell, 2)}},
		{Bind: pendingTerm(keyspace.FamilyBind, 2), Cells: []keyspace.Term{pendingTerm(keyspace.FamilyCell, 3)}},
		{Bind: pendingTerm(keyspace.FamilyBind, 3), Cells: []keyspace.Term{pendingTerm(keyspace.FamilyCell, 4)}},
	}
	fixture := openPendingFixture(t, "pending-runtime-matrix.lua", counts,
		pendingRuntimeMatrixRows(), pendingRuntimeMatrixFlow(), binds, nil, nil,
		pendingSourceExtras{
			keys: []source.KeyInput{
				source.NameKey(body, "field-list"), source.NameKey(body, "field-name"), source.NameKey(body, "method"),
			},
			exactAtoms: []keyspace.LiteralValue{
				{Kind: keyspace.LiteralString, String: "field-list"},
				{Kind: keyspace.LiteralString, String: "field-name"},
				{Kind: keyspace.LiteralString, String: "method"},
			},
		})
	if fixture.pending == nil || !MatchesPending(fixture.pending, fixture.sourceView.Identity().ContentID(), fixture.flowView.Cold().ContentID(), fixture.staticFinalize.View().ContentID(), fixture.moduleFinalize.View().ContentID()) {
		t.Fatal("production runtime matrix did not produce matching Pending")
	}
}

func TestSealPendingProductionMatrixCoversSixSubjectPlanesAndTenBuckets(t *testing.T) {
	counts := pendingRuntimeMatrixCounts()
	body := pendingTerm(keyspace.FamilyBody, 1)
	fixture := openPendingFixture(t, "pending-runtime-buckets.lua", counts,
		pendingRuntimeMatrixRows(), pendingRuntimeMatrixFlow(), []source.BindCells{
			{Bind: pendingTerm(keyspace.FamilyBind, 1), Cells: []keyspace.Term{pendingTerm(keyspace.FamilyCell, 2)}},
			{Bind: pendingTerm(keyspace.FamilyBind, 2), Cells: []keyspace.Term{pendingTerm(keyspace.FamilyCell, 3)}},
			{Bind: pendingTerm(keyspace.FamilyBind, 3), Cells: []keyspace.Term{pendingTerm(keyspace.FamilyCell, 4)}},
		}, nil, nil, pendingSourceExtras{
			keys: []source.KeyInput{source.NameKey(body, "field-list"), source.NameKey(body, "field-name"), source.NameKey(body, "method")},
			exactAtoms: []keyspace.LiteralValue{
				{Kind: keyspace.LiteralString, String: "field-list"},
				{Kind: keyspace.LiteralString, String: "field-name"},
				{Kind: keyspace.LiteralString, String: "method"},
			},
		})
	for _, bucket := range []struct {
		name     string
		contains func(keyspace.Term) bool
		subject  keyspace.Term
	}{
		{"UnaryNumeric", fixture.candidates.UnaryNumeric().Contains, pendingTerm(keyspace.FamilyUnary, 1)},
		{"Length", fixture.candidates.Length().Contains, pendingTerm(keyspace.FamilyUnary, 2)},
		{"Arithmetic", fixture.candidates.Arithmetic().Contains, pendingTerm(keyspace.FamilyBinary, 1)},
		{"Bitwise", fixture.candidates.Bitwise().Contains, pendingTerm(keyspace.FamilyBinary, 2)},
		{"Concat", fixture.candidates.Concat().Contains, pendingTerm(keyspace.FamilyBinary, 3)},
		{"Equality", fixture.candidates.Equality().Contains, pendingTerm(keyspace.FamilyBinary, 4)},
		{"Order", fixture.candidates.Order().Contains, pendingTerm(keyspace.FamilyBinary, 5)},
		{"IndexGet", fixture.candidates.IndexGet().Contains, pendingTerm(keyspace.FamilyRead, 1)},
		{"IndexSet", fixture.candidates.IndexSet().Contains, pendingTerm(keyspace.FamilyWrite, 2)},
	} {
		if !bucket.contains(bucket.subject) {
			t.Fatalf("candidate bucket %s did not contain %v", bucket.name, bucket.subject)
		}
	}
	loopFixture := openPendingLoopFixture(t, "pending-control-bucket.lua")
	if !loopFixture.candidates.GenericLoop().Contains(pendingTerm(keyspace.FamilyLoop, 4)) {
		t.Fatal("candidate bucket GenericLoop did not contain the fixed-header GenericFor")
	}
	if !fixture.candidates.IndexGet().Contains(pendingTerm(keyspace.FamilyRead, 2)) ||
		!fixture.candidates.IndexSet().Contains(pendingTerm(keyspace.FamilyWrite, 3)) ||
		!fixture.candidates.IndexSet().Contains(pendingTerm(keyspace.FamilyWrite, 4)) {
		t.Fatal("dynamic IndexGet/IndexSet rows were not retained")
	}
	if !fixture.candidates.UnaryNumeric().Contains(pendingTerm(keyspace.FamilyUnary, 3)) {
		t.Fatal("runtime FieldExact UnaryNeg was not classified as a live numeric candidate")
	}
	binaryOne := pendingTerm(keyspace.FamilyBinary, 1)
	binaryOneCount, binaryOneOK := fixture.pending.Count(binaryOne)
	if !binaryOneOK || binaryOneCount == 0 {
		t.Fatal("table allocation did not precede the first Binary pending boundary")
	}
	var sawTableAllocation bool
	for index := 0; index < binaryOneCount; index++ {
		value, valueOK := fixture.pending.At(binaryOne, index)
		if !valueOK {
			t.Fatalf("Binary1 pending At(%d) was unavailable", index)
		}
		sawTableAllocation = sawTableAllocation || value == pendingTerm(keyspace.FamilyTable, 1)
	}
	if !sawTableAllocation {
		t.Fatal("table allocation was not retained before the first Binary")
	}
	if fixture.candidates.IndexGet().Contains(pendingTerm(keyspace.FamilyRead, 3)) ||
		fixture.candidates.IndexGet().Contains(pendingTerm(keyspace.FamilyRead, 4)) ||
		fixture.candidates.IndexGet().Contains(pendingTerm(keyspace.FamilyRead, 5)) ||
		fixture.candidates.IndexSet().Contains(pendingTerm(keyspace.FamilyWrite, 1)) {
		t.Fatal("static/cell read-write rows entered candidate buckets")
	}
	for _, subject := range []keyspace.Term{
		pendingTerm(keyspace.FamilyUnary, 1), pendingTerm(keyspace.FamilyUnary, 2), pendingTerm(keyspace.FamilyUnary, 3),
		pendingTerm(keyspace.FamilyBinary, 1), pendingTerm(keyspace.FamilyBinary, 2), pendingTerm(keyspace.FamilyBinary, 3),
		pendingTerm(keyspace.FamilyBinary, 4), pendingTerm(keyspace.FamilyBinary, 5),
		pendingTerm(keyspace.FamilyRead, 1), pendingTerm(keyspace.FamilyRead, 2),
		pendingTerm(keyspace.FamilyWrite, 2), pendingTerm(keyspace.FamilyWrite, 3), pendingTerm(keyspace.FamilyWrite, 4),
		pendingTerm(keyspace.FamilyCall, 1), pendingTerm(keyspace.FamilyCall, 2), pendingTerm(keyspace.FamilyCall, 3),
	} {
		_, ok := fixture.pending.Count(subject)
		if !ok {
			t.Fatalf("production candidate subject %v was not admitted", subject)
		}
	}
	methodRead := pendingTerm(keyspace.FamilyRead, 2)
	methodReadCount, methodReadOK := fixture.pending.Count(methodRead)
	if !methodReadOK || methodReadCount < 2 {
		t.Fatalf("method actual Read pending = %d/%v, want callee and receiver prefix", methodReadCount, methodReadOK)
	}
	var sawMethodCallee, sawMethodReceiver bool
	for index := 0; index < methodReadCount; index++ {
		value, valueOK := fixture.pending.At(methodRead, index)
		if !valueOK {
			t.Fatalf("method actual Read At(%d) was unavailable", index)
		}
		sawMethodCallee = sawMethodCallee || value == pendingTerm(keyspace.FamilyRead, 1)
		sawMethodReceiver = sawMethodReceiver || value == pendingTerm(keyspace.FamilyRead, 6)
	}
	if !sawMethodCallee || !sawMethodReceiver {
		t.Fatalf("method prefix omitted callee/receiver: callee=%v receiver=%v", sawMethodCallee, sawMethodReceiver)
	}
	if _, tail, ok := fixture.flowView.Values().Get(pendingTerm(keyspace.FamilyValues, 5)); !ok || tail != pendingTerm(keyspace.FamilyVararg, 1) || !fixture.executable.Executable(tail) {
		t.Fatal("method actual Values tail was not retained as the authored open tail")
	}
	if fieldCount, ok := fixture.flowView.Tables().FieldCount(pendingTerm(keyspace.FamilyTable, 1)); !ok || fieldCount != 4 {
		t.Fatalf("table field count = %d/%v, want four ordered fields", fieldCount, ok)
	}
	for index := 0; index < 4; index++ {
		field, ok := fixture.flowView.Tables().FieldAt(pendingTerm(keyspace.FamilyTable, 1), index)
		if !ok || field != pendingTerm(keyspace.FamilyTableField, uint32(index+1)) {
			t.Fatalf("table FieldAt(%d) = %v/%v, want Field%d/true", index, field, ok, index+1)
		}
	}
	assign := pendingTerm(keyspace.FamilyAssign, 1)
	writeCount, writeCountOK := fixture.flowView.Storage().Assigns().WriteCount(assign)
	if !writeCountOK || writeCount != 4 {
		t.Fatalf("assignment write count = %d/%v, want four target-ordered writes", writeCount, writeCountOK)
	}
	wantTargets := []keyspace.Term{
		pendingTerm(keyspace.FamilyCell, 2), pendingTerm(keyspace.FamilyLensExact, 2),
		pendingTerm(keyspace.FamilyLensKey, 2), pendingTerm(keyspace.FamilyLensExact, 3),
	}
	for index, wantTarget := range wantTargets {
		write, ok := fixture.flowView.Storage().Assigns().WriteAt(assign, index)
		if !ok {
			t.Fatalf("assignment WriteAt(%d) unavailable", index)
		}
		_, target, ok := fixture.flowView.Storage().Writes().Get(write)
		if !ok || target != wantTarget {
			t.Fatalf("assignment target %d = %v/%v, want %v/true", index, target, ok, wantTarget)
		}
	}
	call2 := pendingTerm(keyspace.FamilyCall, 2)
	call2Count, call2OK := fixture.pending.Count(call2)
	if !call2OK || call2Count == 0 {
		t.Fatal("nested Call after the guarded Select did not receive a nonempty prefix")
	}
	for index := 0; index < call2Count; index++ {
		value, valueOK := fixture.pending.At(call2, index)
		if !valueOK || value == pendingTerm(keyspace.FamilyNil, 17) || value == pendingTerm(keyspace.FamilyNil, 18) {
			t.Fatalf("guarded Select operand leaked into later Call prefix at %d: %v/%v", index, value, valueOK)
		}
	}
	for _, absent := range []keyspace.Term{
		pendingTerm(keyspace.FamilyRead, 3), pendingTerm(keyspace.FamilyRead, 4), pendingTerm(keyspace.FamilyRead, 5), pendingTerm(keyspace.FamilyRead, 6), pendingTerm(keyspace.FamilyWrite, 1),
		pendingTerm(keyspace.FamilyLoop, 1),
		pendingTerm(keyspace.FamilyKey, 1),
	} {
		if _, ok := fixture.pending.Count(absent); ok {
			t.Fatalf("noncandidate subject %v was admitted", absent)
		}
	}
}

func pendingLoopCounts() (counts [keyspace.FamilyCount]uint32) {
	counts[keyspace.FamilyBody] = 7
	counts[keyspace.FamilyNil] = 6
	counts[keyspace.FamilyValues] = 2
	counts[keyspace.FamilyCell] = 2
	counts[keyspace.FamilyBranch] = 1
	counts[keyspace.FamilyLoop] = 4
	return counts
}

func pendingLoopRows() [][]keyspace.Term {
	term := pendingTerm
	return [][]keyspace.Term{
		{term(keyspace.FamilyBranch, 1)},
		{term(keyspace.FamilyLoop, 1), term(keyspace.FamilyLoop, 2)},
		{term(keyspace.FamilyLoop, 3), term(keyspace.FamilyLoop, 4)},
		{}, {}, {}, {},
	}
}

func pendingLoopFlow() authored.Input {
	term := pendingTerm
	body1 := term(keyspace.FamilyBody, 1)
	body2 := term(keyspace.FamilyBody, 2)
	body3 := term(keyspace.FamilyBody, 3)
	return authored.Input{
		Values: authored.ValuesInput{
			Rows:  []authored.Value{{Owner: body3, Fixed: authored.Range{End: 2}}, {Owner: body3, Fixed: authored.Range{Start: 2, End: 3}}},
			Terms: []keyspace.Term{term(keyspace.FamilyNil, 2), term(keyspace.FamilyNil, 3), term(keyspace.FamilyNil, 4)},
		},
		Control: authored.ControlInput{
			Branches: []authored.Branch{{Owner: body1, Condition: term(keyspace.FamilyNil, 1), WhenTrue: body2, WhenFalse: body3}},
			Loops: []authored.Loop{
				{Owner: body2, Body: term(keyspace.FamilyBody, 4), Kind: kind.LoopWhile, Control: term(keyspace.FamilyNil, 5)},
				{Owner: body2, Body: term(keyspace.FamilyBody, 5), Kind: kind.LoopRepeat, Control: term(keyspace.FamilyNil, 6)},
				{Owner: body3, Body: term(keyspace.FamilyBody, 6), Kind: kind.LoopNumericFor, Control: term(keyspace.FamilyValues, 1), Cells: authored.Range{End: 1}},
				{Owner: body3, Body: term(keyspace.FamilyBody, 7), Kind: kind.LoopGenericFor, Control: term(keyspace.FamilyValues, 2), Cells: authored.Range{Start: 1, End: 2}},
			},
			Cells: []keyspace.Term{term(keyspace.FamilyCell, 1), term(keyspace.FamilyCell, 2)},
		},
		Storage: authored.StorageInput{Cells: []authored.Cell{{Kind: authored.CellLocal, Body: term(keyspace.FamilyBody, 6)}, {Kind: authored.CellLocal, Body: term(keyspace.FamilyBody, 7)}}},
	}
}

func openPendingLoopFixture(t *testing.T, name string) *pendingFixture {
	t.Helper()
	return openPendingFixture(t, name, pendingLoopCounts(), pendingLoopRows(), pendingLoopFlow(), nil, nil,
		[]keyspace.Term{pendingTerm(keyspace.FamilyBody, 1), pendingTerm(keyspace.FamilyBody, 3), pendingTerm(keyspace.FamilyBody, 3), pendingTerm(keyspace.FamilyBody, 3), pendingTerm(keyspace.FamilyBody, 2), pendingTerm(keyspace.FamilyBody, 5)}, pendingSourceExtras{})
}

func TestSealPendingProductionBranchAndAllLoopPhases(t *testing.T) {
	fixture := openPendingLoopFixture(t, "pending-control-phases.lua")
	if !fixture.candidates.GenericLoop().Contains(pendingTerm(keyspace.FamilyLoop, 4)) {
		t.Fatal("GenericLoop candidate bucket did not retain the fixed-header GenericFor")
	}
	if _, ok := fixture.pending.Count(pendingTerm(keyspace.FamilyLoop, 4)); !ok {
		t.Fatal("GenericFor loop was not admitted as a candidate subject")
	}
	for _, term := range []keyspace.Term{
		pendingTerm(keyspace.FamilyBranch, 1), pendingTerm(keyspace.FamilyLoop, 1),
		pendingTerm(keyspace.FamilyLoop, 2), pendingTerm(keyspace.FamilyLoop, 3),
	} {
		if _, ok := fixture.pending.Count(term); ok {
			t.Fatalf("noncandidate control subject %v was admitted", term)
		}
	}
}
