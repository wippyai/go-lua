package evaluation

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/binding"
	"github.com/wippyai/go-lua/analysis/program/flow/body"
	"github.com/wippyai/go-lua/analysis/program/flow/candidates"
	"github.com/wippyai/go-lua/analysis/program/flow/containment"
	"github.com/wippyai/go-lua/analysis/program/flow/control"
	"github.com/wippyai/go-lua/analysis/program/flow/executable"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/flowtest"
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

func TestSealPendingProductionAssignTargetsBeforeRHSWithoutCommit(t *testing.T) {
	term := pendingTerm
	body := term(keyspace.FamilyBody, 1)
	counts := [keyspace.FamilyCount]uint32{}
	counts[keyspace.FamilyBody] = 1
	counts[keyspace.FamilyBool] = 4
	counts[keyspace.FamilyString] = 1
	counts[keyspace.FamilyValues] = 1
	counts[keyspace.FamilyLensExact] = 1
	counts[keyspace.FamilyLensKey] = 1
	counts[keyspace.FamilyUnary] = 4
	counts[keyspace.FamilyAssign] = 1
	counts[keyspace.FamilyWrite] = 2
	flow := authored.Input{
		Values: authored.ValuesInput{
			Rows:  []authored.Value{{Owner: body, Fixed: authored.Range{End: 1}}},
			Terms: []keyspace.Term{term(keyspace.FamilyUnary, 4)},
		},
		Access: authored.AccessInput{
			Exact:   []authored.ExactLens{{Owner: body, Base: term(keyspace.FamilyUnary, 1), Source: term(keyspace.FamilyString, 1), Kind: kind.FieldExact}},
			Dynamic: []authored.DynamicLens{{Owner: body, Base: term(keyspace.FamilyUnary, 2), Key: term(keyspace.FamilyUnary, 3)}},
		},
		Storage: authored.StorageInput{
			Assigns: []authored.Assign{{Owner: body, Values: term(keyspace.FamilyValues, 1)}},
			Writes: []authored.Write{
				{Assign: term(keyspace.FamilyAssign, 1), Target: term(keyspace.FamilyLensExact, 1)},
				{Assign: term(keyspace.FamilyAssign, 1), Target: term(keyspace.FamilyLensKey, 1)},
			},
		},
		Operators: authored.OperatorsInput{Unaries: []authored.Unary{
			{Owner: body, Op: kind.UnaryNeg, Operand: term(keyspace.FamilyBool, 1)},
			{Owner: body, Op: kind.UnaryNeg, Operand: term(keyspace.FamilyBool, 2)},
			{Owner: body, Op: kind.UnaryNeg, Operand: term(keyspace.FamilyBool, 3)},
			{Owner: body, Op: kind.UnaryNeg, Operand: term(keyspace.FamilyBool, 4)},
		}},
	}
	fixture := openPendingFixture(t, "pending-assign-order.lua", counts,
		[][]keyspace.Term{{term(keyspace.FamilyAssign, 1)}}, flow, nil, nil, nil, pendingSourceExtras{})

	// A Write's own boundary excludes its address. The second target begins
	// only after the first address has completed.
	assertPendingExact(t, fixture.pending, term(keyspace.FamilyWrite, 1))
	assertPendingExact(t, fixture.pending, term(keyspace.FamilyUnary, 1))
	assertPendingExact(t, fixture.pending, term(keyspace.FamilyWrite, 2), term(keyspace.FamilyLensExact, 1))
	assertPendingExact(t, fixture.pending, term(keyspace.FamilyUnary, 2), term(keyspace.FamilyLensExact, 1))
	assertPendingExact(t, fixture.pending, term(keyspace.FamilyUnary, 3),
		term(keyspace.FamilyLensExact, 1), term(keyspace.FamilyUnary, 2))

	// RHS evaluation begins after both address lenses, but target-internal
	// computations and Write identities do not masquerade as committed values.
	assertPendingExact(t, fixture.pending, term(keyspace.FamilyUnary, 4),
		term(keyspace.FamilyLensExact, 1), term(keyspace.FamilyLensKey, 1))
}
func pendingExactControlFixture(t *testing.T, name string) *pendingFixture {
	t.Helper()
	term := pendingTerm
	body := func(ordinal uint32) keyspace.Term { return term(keyspace.FamilyBody, ordinal) }
	counts := [keyspace.FamilyCount]uint32{}
	counts[keyspace.FamilyBody] = 7
	counts[keyspace.FamilyBool] = 6
	counts[keyspace.FamilyInteger] = 4
	counts[keyspace.FamilyValues] = 3
	counts[keyspace.FamilyReturn] = 1
	counts[keyspace.FamilyCell] = 2
	counts[keyspace.FamilyUnary] = 6
	counts[keyspace.FamilyBinary] = 2
	counts[keyspace.FamilyBranch] = 1
	counts[keyspace.FamilyLoop] = 4
	flow := authored.Input{
		Values: authored.ValuesInput{
			Rows: []authored.Value{
				{Owner: body(3), Fixed: authored.Range{End: 2}},
				{Owner: body(3), Fixed: authored.Range{Start: 2, End: 4}},
				{Owner: body(5), Fixed: authored.Range{Start: 4, End: 5}},
			},
			Terms: []keyspace.Term{
				term(keyspace.FamilyUnary, 4), term(keyspace.FamilyBinary, 1),
				term(keyspace.FamilyUnary, 5), term(keyspace.FamilyBinary, 2),
				term(keyspace.FamilyUnary, 6),
			},
		},
		Control: authored.ControlInput{
			Returns:  []authored.Return{{Owner: body(5), Values: term(keyspace.FamilyValues, 3)}},
			Branches: []authored.Branch{{Owner: body(1), Condition: term(keyspace.FamilyUnary, 1), WhenTrue: body(2), WhenFalse: body(3)}},
			Loops: []authored.Loop{
				{Owner: body(2), Body: body(4), Kind: kind.LoopWhile, Control: term(keyspace.FamilyUnary, 2)},
				{Owner: body(2), Body: body(5), Kind: kind.LoopRepeat, Control: term(keyspace.FamilyUnary, 3)},
				{Owner: body(3), Body: body(6), Kind: kind.LoopNumericFor, Control: term(keyspace.FamilyValues, 1), Cells: authored.Range{End: 1}},
				{Owner: body(3), Body: body(7), Kind: kind.LoopGenericFor, Control: term(keyspace.FamilyValues, 2), Cells: authored.Range{Start: 1, End: 2}},
			},
			Cells: []keyspace.Term{term(keyspace.FamilyCell, 1), term(keyspace.FamilyCell, 2)},
		},
		Storage: authored.StorageInput{Cells: []authored.Cell{
			{Kind: authored.CellLocal, Body: body(6)}, {Kind: authored.CellLocal, Body: body(7)},
		}},
		Operators: authored.OperatorsInput{
			Unaries: []authored.Unary{
				{Owner: body(1), Op: kind.UnaryNeg, Operand: term(keyspace.FamilyBool, 1)},
				{Owner: body(2), Op: kind.UnaryNeg, Operand: term(keyspace.FamilyBool, 2)},
				{Owner: body(5), Op: kind.UnaryNeg, Operand: term(keyspace.FamilyBool, 3)},
				{Owner: body(3), Op: kind.UnaryNeg, Operand: term(keyspace.FamilyBool, 4)},
				{Owner: body(3), Op: kind.UnaryNeg, Operand: term(keyspace.FamilyBool, 5)},
				{Owner: body(5), Op: kind.UnaryNeg, Operand: term(keyspace.FamilyBool, 6)},
			},
			Binaries: []authored.Binary{
				{Owner: body(3), Op: kind.BinaryAdd, Left: term(keyspace.FamilyInteger, 1), Right: term(keyspace.FamilyInteger, 2)},
				{Owner: body(3), Op: kind.BinaryAdd, Left: term(keyspace.FamilyInteger, 3), Right: term(keyspace.FamilyInteger, 4)},
			},
		},
	}
	rows := [][]keyspace.Term{
		{term(keyspace.FamilyBranch, 1)},
		{term(keyspace.FamilyLoop, 1), term(keyspace.FamilyLoop, 2)},
		{term(keyspace.FamilyLoop, 3), term(keyspace.FamilyLoop, 4)},
		{}, {term(keyspace.FamilyReturn, 1)}, {}, {},
	}
	return openPendingFixture(t, name, counts, rows, flow, nil, nil, nil, pendingSourceExtras{
		boolOwners:    []keyspace.Term{body(1), body(2), body(5), body(3), body(3), body(5)},
		integerOwners: []keyspace.Term{body(3), body(3), body(3), body(3)},
	})
}

func TestSealPendingProductionExactBranchAndLoopPhases(t *testing.T) {
	fixture := pendingExactControlFixture(t, "pending-exact-control-phases.lua")
	term := pendingTerm

	// Branch, While, and Repeat each expose only their condition/header phase.
	assertPendingExact(t, fixture.pending, term(keyspace.FamilyUnary, 1))
	assertPendingExact(t, fixture.pending, term(keyspace.FamilyUnary, 2))
	assertPendingExact(t, fixture.pending, term(keyspace.FamilyUnary, 3))

	// Numeric and Generic headers preserve the fixed Values order exactly.
	assertPendingExact(t, fixture.pending, term(keyspace.FamilyUnary, 4))
	assertPendingExact(t, fixture.pending, term(keyspace.FamilyBinary, 1), term(keyspace.FamilyUnary, 4))
	assertPendingExact(t, fixture.pending, term(keyspace.FamilyLoop, 4))
	assertPendingExact(t, fixture.pending, term(keyspace.FamilyUnary, 5))
	assertPendingExact(t, fixture.pending, term(keyspace.FamilyBinary, 2), term(keyspace.FamilyUnary, 5))

	// Repeat's body is executable before its condition in control semantics,
	// but its committed body work is not an uncommitted expression prefix for
	// the condition. The separate body subject therefore remains exactly empty
	// and is absent from Unary3's exact empty sequence.
	repeatBodySubject := term(keyspace.FamilyUnary, 6)
	if !fixture.executable.Contains(repeatBodySubject) {
		t.Fatal("Repeat body subject was not executable in the production control proof")
	}
	assertPendingExact(t, fixture.pending, repeatBodySubject)
	_, repeatBody, repeatKind, repeatCondition, ok := fixture.flowView.Control().Loops().Get(term(keyspace.FamilyLoop, 2))
	if !ok || repeatBody != term(keyspace.FamilyBody, 5) || repeatKind != kind.LoopRepeat || repeatCondition != term(keyspace.FamilyUnary, 3) {
		t.Fatal("production Repeat owner did not retain its body-before-condition topology")
	}
}

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
	if counts[keyspace.FamilyTypeValue] != 0 && counts[keyspace.FamilyTypePrimitive] != 0 {
		staticInput.Counts[keyspace.FamilyTypeValue] = counts[keyspace.FamilyTypeValue]
		staticInput.Operands.TypeValue = make([]staticoperands.TypeValueTarget, counts[keyspace.FamilyTypeValue])
		for index := range staticInput.Operands.TypeValue {
			staticInput.Operands.TypeValue[index] = staticoperands.TypeValueTarget{
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
	sourceComponent, issuance, err := sourceFinalize.CommitWithSemanticPathIssuance(index)
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
	paths, err := semanticpath.Seal(issuance, sourceView.CellRoles(), sourceView, flowView, bodies, bindingResult, forest, outcomes,
		flowView.Cold().ContentID(), staticFinalize.View().ContentID(), moduleView.ContentID())
	if err != nil {
		closePendingFinalizers(source.Finalizer{}, staticFinalize, flowFinalize, moduleFinalize)
		t.Fatalf("semanticpath.Seal: %v", err)
	}
	executableResult, err := executable.Seal(sourceView, flowView, bodies, forest, controlResult,
		staticFinalize.View().ContentID(), moduleView.ContentID(), paths)
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
	return flowtest.Term(family, ordinal)
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
func pendingSourceSpans(name string, counts [keyspace.FamilyCount]uint32) []source.FamilySpans {
	rows := make([]source.FamilySpans, 0, keyspace.FamilyCount-1)
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		spans := make([]source.Span, counts[family])
		for index := range spans {
			line := uint32(index + 1)
			spans[index] = source.Span{File: name, StartLine: line, StartCol: 1, EndLine: line, EndCol: 1}
		}
		rows = append(rows, source.FamilySpans{Family: family, Spans: spans})
	}
	return rows
}

func TestSealPendingProductionRejectsDuplicateDirectSourceBeforeSeal(t *testing.T) {
	name := "pending-duplicate-direct.lua"
	counts := [keyspace.FamilyCount]uint32{keyspace.FamilyBody: 1, keyspace.FamilyReturn: 1}
	returnTerm := pendingTerm(keyspace.FamilyReturn, 1)
	_, err := source.Build(source.Input{
		Name:     name,
		Families: pendingSourceSpans(name, counts),
		Bodies:   []source.BodySource{{Body: pendingTerm(keyspace.FamilyBody, 1), Terms: []keyspace.Term{returnTerm, returnTerm}}},
	})
	if err == nil {
		t.Fatal("Source accepted a duplicate direct root that would make SealPending's root order ambiguous")
	}
}

func TestSealPendingProductionRejectsCyclicAuthoredParentBeforeSeal(t *testing.T) {
	counts := [keyspace.FamilyCount]uint32{
		keyspace.FamilyBody: 1, keyspace.FamilyValues: 1,
		keyspace.FamilyUnary: 1, keyspace.FamilyReturn: 1,
	}
	body := pendingTerm(keyspace.FamilyBody, 1)
	unary := pendingTerm(keyspace.FamilyUnary, 1)
	values := pendingTerm(keyspace.FamilyValues, 1)
	draft, err := authored.Build(authored.Input{
		Counts: counts,
		Values: authored.ValuesInput{
			Rows:  []authored.Value{{Owner: body, Fixed: authored.Range{End: 1}}},
			Terms: []keyspace.Term{unary},
		},
		Operators: authored.OperatorsInput{Unaries: []authored.Unary{{Owner: body, Op: kind.UnaryNeg, Operand: unary}}},
		Control:   authored.ControlInput{Returns: []authored.Return{{Owner: body, Values: values}}},
	})
	if err != nil {
		t.Fatalf("authored.Build rejected the cycle before the discovery gate: %v", err)
	}
	finalize, err := draft.Finalizer()
	if err != nil {
		t.Fatalf("authored.Finalizer: %v", err)
	}
	t.Cleanup(func() { _ = finalize.Abort() })
	walker, err := New(finalize.View())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	builder := &pendingBuilder{view: finalize.View(), discover: true}
	for index, family := range pendingAncestorFamilyKeys {
		builder.parents[index] = make([]keyspace.Term, counts[family]+1)
	}
	for index, family := range pendingClaimFamilyKeys {
		builder.claimed[index] = make([]bool, counts[family]+1)
	}
	if err := discoverPendingParents(walker, builder, countsToInt(counts)); err == nil {
		t.Fatal("SealPending discovery accepted a cyclic authored parent")
	}
}

// TestProductionOwnerChainRejectsDuplicatePendingOccurrences proves the
// malformed cases at the earliest genuine owner boundary. A real SealPending
// call requires a containment/executable/candidate proof quartet; fabricating
// those proofs after this rejection would make the test dishonest.
func TestProductionOwnerChainRejectsDuplicatePendingOccurrences(t *testing.T) {
	for _, row := range []pendingDuplicateSpec{pendingDuplicateLeafSpec(), pendingDuplicateCompositeSpec()} {
		t.Run(row.name, func(t *testing.T) {
			assertProductionContainmentRejectsPendingDuplicate(t, row.name+".lua", row.counts, row.flow)
		})
	}
}

type pendingDuplicateSpec struct {
	name   string
	counts [keyspace.FamilyCount]uint32
	flow   authored.Input
}

func pendingDuplicateLeafSpec() pendingDuplicateSpec {
	term := pendingTerm
	bodyTerm := term(keyspace.FamilyBody, 1)
	counts := [keyspace.FamilyCount]uint32{}
	counts[keyspace.FamilyBody] = 1
	counts[keyspace.FamilyBool] = 1
	counts[keyspace.FamilyValues] = 1
	counts[keyspace.FamilyReturn] = 1
	counts[keyspace.FamilyBinary] = 1
	return pendingDuplicateSpec{
		name: "duplicate-leaf", counts: counts,
		flow: authored.Input{
			Values:    authored.ValuesInput{Rows: []authored.Value{{Owner: bodyTerm, Fixed: authored.Range{End: 1}}}, Terms: []keyspace.Term{term(keyspace.FamilyBinary, 1)}},
			Operators: authored.OperatorsInput{Binaries: []authored.Binary{{Owner: bodyTerm, Op: kind.BinaryAdd, Left: term(keyspace.FamilyBool, 1), Right: term(keyspace.FamilyBool, 1)}}},
			Control:   authored.ControlInput{Returns: []authored.Return{{Owner: bodyTerm, Values: term(keyspace.FamilyValues, 1)}}},
		},
	}
}

func pendingDuplicateCompositeSpec() pendingDuplicateSpec {
	term := pendingTerm
	bodyTerm := term(keyspace.FamilyBody, 1)
	counts := [keyspace.FamilyCount]uint32{}
	counts[keyspace.FamilyBody] = 1
	counts[keyspace.FamilyBool] = 1
	counts[keyspace.FamilyValues] = 1
	counts[keyspace.FamilyReturn] = 1
	counts[keyspace.FamilyUnary] = 1
	return pendingDuplicateSpec{
		name: "duplicate-composite", counts: counts,
		flow: authored.Input{
			Values:    authored.ValuesInput{Rows: []authored.Value{{Owner: bodyTerm, Fixed: authored.Range{End: 2}}}, Terms: []keyspace.Term{term(keyspace.FamilyUnary, 1), term(keyspace.FamilyUnary, 1)}},
			Operators: authored.OperatorsInput{Unaries: []authored.Unary{{Owner: bodyTerm, Op: kind.UnaryNeg, Operand: term(keyspace.FamilyBool, 1)}}},
			Control:   authored.ControlInput{Returns: []authored.Return{{Owner: bodyTerm, Values: term(keyspace.FamilyValues, 1)}}},
		},
	}
}

func assertProductionContainmentRejectsPendingDuplicate(t *testing.T, name string, counts [keyspace.FamilyCount]uint32, flowInput authored.Input) {
	t.Helper()
	bodyTerm := pendingTerm(keyspace.FamilyBody, 1)
	sourceDraft, err := source.Build(pendingSourceInput(name, counts,
		[][]keyspace.Term{{pendingTerm(keyspace.FamilyReturn, 1)}}, nil, nil, nil,
		pendingSourceExtras{boolOwners: []keyspace.Term{bodyTerm}}))
	if err != nil {
		t.Fatalf("source.Build: %v", err)
	}
	sourceFinalize, err := sourceDraft.Finalizer()
	if err != nil {
		t.Fatalf("source.Finalizer: %v", err)
	}
	t.Cleanup(func() { _ = sourceFinalize.Abort() })

	staticCounts := [keyspace.FamilyCount]uint32{}
	staticCounts[keyspace.FamilyBody] = 1
	staticDraft, err := static.Build(static.Input{Counts: staticCounts})
	if err != nil {
		t.Fatalf("static.Build: %v", err)
	}
	staticFinalize, err := staticDraft.Finalizer()
	if err != nil {
		t.Fatalf("static.Finalizer: %v", err)
	}
	t.Cleanup(func() { _ = staticFinalize.Abort() })

	flowInput.Counts = counts
	flowDraft, err := authored.Build(flowInput)
	if err != nil {
		t.Fatalf("authored.Build rejected before the canonical containment owner: %v", err)
	}
	flowFinalize, err := flowDraft.Finalizer()
	if err != nil {
		t.Fatalf("authored.Finalizer: %v", err)
	}
	t.Cleanup(func() { _ = flowFinalize.Abort() })

	bodies, err := body.Seal(sourceFinalize.Preimage(), flowFinalize.View(), staticFinalize.View(), bodyTerm)
	if err != nil {
		t.Fatalf("body.Seal: %v", err)
	}
	bindings, err := binding.Seal(sourceFinalize.Preimage(), flowFinalize.View(), bodies, bodyTerm)
	if err != nil {
		t.Fatalf("binding.Seal: %v", err)
	}
	moduleDraft, err := imports.Build(imports.Input{})
	if err != nil {
		t.Fatalf("imports.Build: %v", err)
	}
	moduleFinalize, err := moduleDraft.Finalizer()
	if err != nil {
		t.Fatalf("imports.Finalizer: %v", err)
	}
	t.Cleanup(func() { _ = moduleFinalize.Abort() })
	if _, _, err := containment.Prove(sourceFinalize.Preimage(), staticFinalize.View(), flowFinalize.View(), bodies, bindings, moduleFinalize.View(), bodyTerm); err == nil {
		t.Fatal("canonical containment owner accepted a duplicate Pending occurrence")
	}
}

func countsToInt(counts [keyspace.FamilyCount]uint32) [keyspace.FamilyCount]int {
	var result [keyspace.FamilyCount]int
	for family, count := range counts {
		result[family] = int(count)
	}
	return result
}
func openPendingDeepFixture(t *testing.T, depth int) *pendingFixture {
	t.Helper()
	if depth < 2 {
		t.Fatal("deep fixture requires at least two Unary rows")
	}
	body := pendingTerm(keyspace.FamilyBody, 1)
	returnCounts := [keyspace.FamilyCount]uint32{
		keyspace.FamilyBody: 1, keyspace.FamilyNil: 2, keyspace.FamilyValues: 1,
		keyspace.FamilyUnary: uint32(depth + 1), keyspace.FamilyReturn: 1,
	}
	unaries := make([]authored.Unary, depth+1)
	for index := range unaries {
		ordinal := uint32(index + 1)
		operand := pendingTerm(keyspace.FamilyNil, 1)
		if ordinal > 1 && ordinal <= uint32(depth) {
			operand = pendingTerm(keyspace.FamilyUnary, ordinal-1)
		} else if ordinal == uint32(depth+1) {
			operand = pendingTerm(keyspace.FamilyNil, 2)
		}
		unaries[index] = authored.Unary{Owner: body, Op: kind.UnaryNeg, Operand: operand}
	}
	return openPendingFixture(t, "pending-deep.lua", returnCounts,
		[][]keyspace.Term{{pendingTerm(keyspace.FamilyReturn, 1)}}, authored.Input{
			Values: authored.ValuesInput{
				Rows:  []authored.Value{{Owner: body, Fixed: authored.Range{End: 2}}},
				Terms: []keyspace.Term{pendingTerm(keyspace.FamilyUnary, uint32(depth)), pendingTerm(keyspace.FamilyUnary, uint32(depth+1))},
			},
			Operators: authored.OperatorsInput{Unaries: unaries},
			Control:   authored.ControlInput{Returns: []authored.Return{{Owner: body, Values: pendingTerm(keyspace.FamilyValues, 1)}}},
		}, nil, nil, nil, pendingSourceExtras{})
}

func openPendingWideFixture(t *testing.T, width int) *pendingFixture {
	t.Helper()
	if width < 2 {
		t.Fatal("wide fixture requires at least two payload terms")
	}
	body := pendingTerm(keyspace.FamilyBody, 1)
	counts := [keyspace.FamilyCount]uint32{
		keyspace.FamilyBody: 1, keyspace.FamilyNil: uint32(width + 2),
		keyspace.FamilyValues: 1, keyspace.FamilyBinary: 1, keyspace.FamilyReturn: 1,
	}
	terms := make([]keyspace.Term, width+1)
	for index := 0; index < width; index++ {
		terms[index] = pendingTerm(keyspace.FamilyNil, uint32(index+1))
	}
	terms[width] = pendingTerm(keyspace.FamilyBinary, 1)
	return openPendingFixture(t, "pending-wide.lua", counts,
		[][]keyspace.Term{{pendingTerm(keyspace.FamilyReturn, 1)}}, authored.Input{
			Values: authored.ValuesInput{
				Rows:  []authored.Value{{Owner: body, Fixed: authored.Range{End: uint32(len(terms))}}},
				Terms: terms,
			},
			Operators: authored.OperatorsInput{Binaries: []authored.Binary{{
				Owner: body, Op: kind.BinaryAdd,
				Left: pendingTerm(keyspace.FamilyNil, uint32(width+1)), Right: pendingTerm(keyspace.FamilyNil, uint32(width+2)),
			}}},
			Control: authored.ControlInput{Returns: []authored.Return{{Owner: body, Values: pendingTerm(keyspace.FamilyValues, 1)}}},
		}, nil, nil, nil, pendingSourceExtras{})
}

func TestSealPendingProductionDeepAndWideFixturesRemainQueryable(t *testing.T) {
	const deepDepth = 4096
	deep := openPendingDeepFixture(t, deepDepth)
	deepSubject := pendingTerm(keyspace.FamilyUnary, deepDepth+1)
	deepPrefixCount, deepOK := deep.pending.Count(deepSubject)
	if !deepOK || deepPrefixCount != 1 {
		t.Fatalf("deep final Unary pending = %d/%v, want one retained deep predecessor", deepPrefixCount, deepOK)
	}
	if got, ok := deep.pending.At(deepSubject, 0); !ok || got != pendingTerm(keyspace.FamilyUnary, deepDepth) {
		t.Fatalf("deep final Unary At(0) = %v/%v, want Unary%d/true", got, ok, deepDepth)
	}
	if !deep.executable.Contains(pendingTerm(keyspace.FamilyUnary, deepDepth)) {
		t.Fatal("deep executable closure did not reach the innermost authored Unary")
	}
	if allocations := testing.AllocsPerRun(1000, func() {
		if count, ok := deep.pending.Count(deepSubject); !ok || count != 1 {
			t.Fatal("deep Count changed during allocation probe")
		}
		if _, ok := deep.pending.At(deepSubject, 0); !ok {
			t.Fatal("deep At changed during allocation probe")
		}
	}); allocations != 0 {
		t.Fatalf("deep Pending queries allocated %v objects per run", allocations)
	}

	const width = 2048
	wide := openPendingWideFixture(t, width)
	wideSubject := pendingTerm(keyspace.FamilyBinary, 1)
	wideCount, wideOK := wide.pending.Count(wideSubject)
	if !wideOK || wideCount != width {
		t.Fatalf("wide Binary pending = %d/%v, want %d retained payloads", wideCount, wideOK, width)
	}
	for _, index := range []int{0, width / 2, width - 1} {
		want := pendingTerm(keyspace.FamilyNil, uint32(index+1))
		if got, ok := wide.pending.At(wideSubject, index); !ok || got != want {
			t.Fatalf("wide Binary At(%d) = %v/%v, want Nil%d/true", index, got, ok, index+1)
		}
	}
	if allocations := testing.AllocsPerRun(1000, func() {
		if count, ok := wide.pending.Count(wideSubject); !ok || count != width {
			t.Fatal("wide Count changed during allocation probe")
		}
		if _, ok := wide.pending.At(wideSubject, width/2); !ok {
			t.Fatal("wide At changed during allocation probe")
		}
	}); allocations != 0 {
		t.Fatalf("wide Pending queries allocated %v objects per run", allocations)
	}
}
