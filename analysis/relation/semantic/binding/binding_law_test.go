package binding_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
)

func content(t *testing.T, label string) identity.ContentID {
	t.Helper()
	value, ok := identity.DeriveContentID("semantic-binding-law", []byte(label))
	if !ok {
		t.Fatalf("derive %q", label)
	}
	return value
}

type fixture struct {
	operationOwner model.OwnerID
	dataOwner      model.OwnerID
	valueOwner     model.OwnerID
	relation       model.RelationID
	input          model.ColumnID
	output         model.ColumnID
	denominator    model.DenominatorRef
	schema         model.SchemaID
	signature      signature.Signature
	logical        signature.Fence
	runtime        binding.Fence
	issuer         binding.Issuer
	inputScope     binding.ScopeToken
	outputScope    binding.ScopeToken
	inputWitness   binding.DenominatorWitness
	outputWitness  binding.DenominatorWitness
	inputToken     binding.CellToken
	outputToken    binding.CellToken
	inputValue     binding.ValueToken
	outputValue    binding.ValueToken
	inputType      model.TypeID
	outputType     model.TypeID
	inputRow       model.RowID
	outputRow      model.RowID
}

type testFactory struct{ operation signature.Signature }

func (factory testFactory) Bind(signature.Signature) (binding.Binding, bool) {
	return testBinding{operation: factory.operation}, true
}

type testBinding struct{ operation signature.Signature }

func (value testBinding) Signature() signature.Signature { return value.operation }
func (value testBinding) NewWorker(binding.Fence) (binding.Worker, bool) {
	return testWorker{}, true
}

type testWorker struct{}

func (testWorker) Evaluate(binding.Frame, *binding.ProposalBuffer) outcome.Result {
	return outcome.Result{Code: outcome.Produced}
}

type testAlgebra struct{ typeID model.TypeID }

func (algebra testAlgebra) Type() model.TypeID { return algebra.typeID }
func (algebra testAlgebra) Join(left, right binding.ValueToken) (binding.ValueToken, bool) {
	if left.Type() != algebra.typeID || right.Type() != algebra.typeID {
		return binding.ValueToken{}, false
	}
	return right, true
}
func (algebra testAlgebra) Widen(left, right binding.ValueToken) (binding.ValueToken, bool) {
	return algebra.Join(left, right)
}
func (algebra testAlgebra) LessOrEqual(left, right binding.ValueToken) bool {
	return left.Type() == algebra.typeID && right.Type() == algebra.typeID
}

type testAlgebraFactory struct{ algebra testAlgebra }

func (factory testAlgebraFactory) Bind(model.TypeID) (binding.ValueAlgebra, bool) {
	return factory.algebra, true
}

type testAlgebraRegistry struct{ algebra testAlgebra }

func (registry testAlgebraRegistry) Resolve(model.TypeID) (binding.ValueAlgebra, bool) {
	return registry.algebra, true
}

func newFixture(t *testing.T, cardinality model.Cardinality) fixture {
	t.Helper()
	operationOwner := issueOwner(t, "owner/operation")
	dataOwner := issueOwner(t, "owner/data")
	valueOwner := issueOwner(t, "owner/value")
	relation := issueRelation(t, dataOwner, "relation")
	input := issueColumn(t, relation, "column/input")
	output := issueColumn(t, relation, "column/output")
	key := issueKey(t, relation, "key/denominator")
	denominator, ok := model.NewDenominatorRef(relation, key)
	if !ok {
		t.Fatalf("issue denominator")
	}
	schema := issueSchema(t, operationOwner, "schema")
	operationID := issueOperation(t, operationOwner, "operation")
	inputType := issueType(t, valueOwner, "type/input")
	outputType := issueType(t, valueOwner, "type/output")
	delivery, ok := signature.NewScalarDelivery()
	if !ok {
		t.Fatalf("construct scalar delivery")
	}
	logical := signature.Fence{Owner: operationOwner, Schema: schema}
	outcomes, ok := outcome.NewSet(outcome.Produced, outcome.NoCandidate, outcome.NoSelection, outcome.Opaque, outcome.Refused)
	if !ok {
		t.Fatalf("construct outcomes")
	}
	spec := signature.Spec{
		Identity: signature.Identity{Operation: operationID, Version: 1}, Fence: logical,
		Inputs:      []signature.Input{{Relation: relation, Column: input, Type: inputType, Presence: signature.RequirePresent, Delivery: delivery, Denominator: denominator}},
		Outputs:     []signature.Output{{Relation: relation, Column: output, Type: outputType, Presence: signature.ProducePresent}},
		Authority:   signature.OutputAuthority{Denominator: denominator},
		Cardinality: cardinality, Outcomes: outcomes,
	}
	sealed, ok := signature.Seal(spec)
	if !ok {
		t.Fatalf("seal operation")
	}
	runtime, ok := binding.NewFence(schema, identity.MountID{1}, identity.Generation(1))
	if !ok {
		t.Fatalf("construct runtime fence")
	}
	issuer, ok := binding.NewIssuer(runtime)
	if !ok {
		t.Fatalf("issue token authority")
	}
	inputScope, ok := issuer.IssueScope(content(t, "scope-formula/input"))
	if !ok {
		t.Fatalf("issue input scope")
	}
	outputScope, ok := issuer.IssueScope(content(t, "scope-formula/output"))
	if !ok {
		t.Fatalf("issue output scope")
	}
	inputRow := mustRow(t, relation, "row/input")
	outputRow := mustRow(t, relation, "row/output")
	inputMembership, ok := binding.NewMembershipView(relation, []model.RowID{inputRow})
	if !ok {
		t.Fatalf("issue input membership")
	}
	outputMembership, ok := binding.NewMembershipView(relation, []model.RowID{outputRow})
	if !ok {
		t.Fatalf("issue output membership")
	}
	inputWitness, ok := issuer.IssueDenominator(denominator, inputMembership, content(t, "denominator-witness/input"))
	if !ok {
		t.Fatalf("issue input denominator witness")
	}
	outputWitness, ok := issuer.IssueDenominator(denominator, outputMembership, content(t, "denominator-witness/output"))
	if !ok {
		t.Fatalf("issue output denominator witness")
	}
	inputToken, ok := issuer.IssueCell(inputWitness, inputScope, input, inputRow)
	if !ok {
		t.Fatalf("issue input address")
	}
	outputToken, ok := issuer.IssueCell(outputWitness, outputScope, output, outputRow)
	if !ok {
		t.Fatalf("issue output address")
	}
	inputValue, ok := issuer.IssueValue(inputType, content(t, "value/input"))
	if !ok {
		t.Fatalf("issue input value")
	}
	outputValue, ok := issuer.IssueValue(outputType, content(t, "value/output"))
	if !ok {
		t.Fatalf("issue output value")
	}
	return fixture{
		operationOwner: operationOwner, dataOwner: dataOwner, valueOwner: valueOwner,
		relation: relation, input: input, output: output, denominator: denominator,
		schema: schema, signature: sealed, logical: logical, runtime: runtime, issuer: issuer,
		inputScope: inputScope, outputScope: outputScope, inputWitness: inputWitness,
		outputWitness: outputWitness, inputToken: inputToken, outputToken: outputToken,
		inputValue: inputValue, outputValue: outputValue, inputType: inputType,
		outputType: outputType, inputRow: inputRow, outputRow: outputRow,
	}
}

func TestTokensRefuseStaleAndForeignRuntimeFences(t *testing.T) {
	exact, _ := model.NewCardinality(model.ExactlyOne, 0)
	value := newFixture(t, exact)
	if !value.issuer.AuthenticateCell(value.inputToken) || !value.inputToken.ValidFor(value.runtime) || !value.inputValue.ValidFor(value.runtime) {
		t.Fatalf("issued tokens did not authenticate")
	}
	stale, ok := binding.NewFence(value.schema, value.runtime.Mount(), identity.Generation(2))
	if !ok {
		t.Fatalf("construct stale fence")
	}
	if value.inputScope.ValidFor(stale) || value.inputToken.ValidFor(stale) || value.inputValue.ValidFor(stale) {
		t.Fatalf("stale generation token accepted")
	}
	foreign, ok := binding.NewFence(value.schema, identity.MountID{2}, value.runtime.Generation())
	if !ok {
		t.Fatalf("construct foreign mount fence")
	}
	if value.inputScope.ValidFor(foreign) || value.inputToken.ValidFor(foreign) {
		t.Fatalf("foreign mount token accepted")
	}
	foreignSchema := issueSchema(t, value.operationOwner, "schema/foreign")
	foreign, ok = binding.NewFence(foreignSchema, value.runtime.Mount(), value.runtime.Generation())
	if !ok {
		t.Fatalf("construct foreign schema fence")
	}
	if value.inputScope.ValidFor(foreign) || value.inputToken.ValidFor(foreign) {
		t.Fatalf("foreign schema token accepted")
	}
}

func TestScopeTokenCarriesConjoinedFormulaWithoutNominalScopeID(t *testing.T) {
	exact, _ := model.NewCardinality(model.ExactlyOne, 0)
	value := newFixture(t, exact)
	leftScope := issueScope(t, value.operationOwner, "scope/source-left")
	foreignOwner := issueOwner(t, "owner/other-scope")
	rightScope := issueScope(t, foreignOwner, "scope/source-right")
	formula := content(t, "formula/source-left-and-source-right")
	if formula == leftScope.Content() || formula == rightScope.Content() {
		t.Fatalf("test formula accidentally reused a source scope identity")
	}
	conjoined, ok := value.issuer.IssueScope(formula)
	if !ok || !conjoined.Available() || !value.issuer.AuthenticateScope(conjoined) {
		t.Fatalf("canonical conjoined formula was not authenticated")
	}
	reissued, ok := value.issuer.IssueScope(formula)
	if !ok || !conjoined.Same(reissued) {
		t.Fatalf("canonical formula identity was not stable within its fence")
	}
	if conjoined.Same(value.inputScope) {
		t.Fatalf("distinct canonical formula identities collapsed")
	}
	stale, ok := binding.NewFence(value.schema, value.runtime.Mount(), identity.Generation(2))
	if !ok {
		t.Fatalf("construct stale fence")
	}
	if conjoined.ValidFor(stale) {
		t.Fatalf("conjoined formula survived stale generation fence")
	}
	foreign, ok := binding.NewFence(value.schema, identity.MountID{2}, value.runtime.Generation())
	if !ok {
		t.Fatalf("construct foreign mount fence")
	}
	if conjoined.ValidFor(foreign) {
		t.Fatalf("conjoined formula survived foreign mount fence")
	}
}

func TestFrameChecksEveryTypedCellInOrderedSlots(t *testing.T) {
	exact, _ := model.NewCardinality(model.ExactlyOne, 0)
	value := newFixture(t, exact)
	present, _ := model.NewPresence(model.Present)
	cell, ok := binding.NewCell(value.inputToken, value.inputType, value.inputValue, present)
	if !ok {
		t.Fatalf("construct cell")
	}
	slot, ok := binding.NewScalarSlot(cell)
	if !ok {
		t.Fatalf("construct scalar slot")
	}
	frame, ok := binding.NewFrame(value.inputScope, slot)
	if !ok || !frame.Validate(value.signature, value.runtime) {
		t.Fatalf("valid frame rejected")
	}
	foreignToken, ok := value.issuer.IssueCell(value.inputWitness, value.inputScope, value.output, value.inputRow)
	if !ok {
		t.Fatalf("issue cross-column address")
	}
	foreignCell, ok := binding.NewCell(foreignToken, value.outputType, value.outputValue, present)
	if !ok {
		t.Fatalf("construct foreign type cell")
	}
	foreignSlot, _ := binding.NewScalarSlot(foreignCell)
	foreignFrame, _ := binding.NewFrame(value.inputScope, foreignSlot)
	if foreignFrame.Validate(value.signature, value.runtime) {
		t.Fatalf("foreign type frame accepted")
	}
	wrongKey := issueKey(t, value.relation, "key/wrong-input")
	wrongDenominator, _ := model.NewDenominatorRef(value.relation, wrongKey)
	wrongMembership, _ := binding.NewMembershipView(value.relation, []model.RowID{value.inputRow})
	wrongWitness, _ := value.issuer.IssueDenominator(wrongDenominator, wrongMembership, content(t, "witness/wrong-input"))
	wrongAddress, _ := value.issuer.IssueCell(wrongWitness, value.inputScope, value.input, value.inputRow)
	wrongDenominatorCell, _ := binding.NewCell(wrongAddress, value.inputType, value.inputValue, present)
	wrongDenominatorSlot, _ := binding.NewScalarSlot(wrongDenominatorCell)
	wrongDenominatorFrame, _ := binding.NewFrame(value.inputScope, wrongDenominatorSlot)
	if wrongDenominatorFrame.Validate(value.signature, value.runtime) {
		t.Fatalf("scalar wrong denominator accepted")
	}
}

func TestCrossOwnerFrameCarriesDistinctDenominatorsAndScopes(t *testing.T) {
	exact, _ := model.NewCardinality(model.ExactlyOne, 0)
	value := newFixture(t, exact)
	otherOwner := issueOwner(t, "owner/other-relation")
	otherRelation := issueRelation(t, otherOwner, "relation/other")
	otherColumn := issueColumn(t, otherRelation, "column/other-input")
	otherKey := issueKey(t, otherRelation, "key/other")
	otherDenominator, ok := model.NewDenominatorRef(otherRelation, otherKey)
	if !ok {
		t.Fatalf("construct other denominator")
	}
	otherType := issueType(t, otherOwner, "type/other-input")
	otherRow := mustRow(t, otherRelation, "row/other-input")
	otherMembership, ok := binding.NewMembershipView(otherRelation, []model.RowID{otherRow})
	if !ok {
		t.Fatalf("construct other membership")
	}
	otherWitness, ok := value.issuer.IssueDenominator(otherDenominator, otherMembership, content(t, "witness/other"))
	if !ok {
		t.Fatalf("construct other witness")
	}
	otherToken, ok := value.issuer.IssueCell(otherWitness, value.inputScope, otherColumn, otherRow)
	if !ok {
		t.Fatalf("construct other address")
	}
	otherValue, ok := value.issuer.IssueValue(otherType, content(t, "value/other"))
	if !ok {
		t.Fatalf("construct other value")
	}
	secondInput := signature.Input{Relation: otherRelation, Column: otherColumn, Type: otherType, Presence: signature.RequirePresent, Delivery: scalarDelivery(t), Denominator: otherDenominator}
	spec := signature.Spec{
		Identity: value.signature.Identity(), Fence: value.logical,
		Inputs: []signature.Input{
			{Relation: value.relation, Column: value.input, Type: value.inputType, Presence: signature.RequirePresent, Delivery: scalarDelivery(t), Denominator: value.denominator},
			secondInput,
		},
		Outputs: value.signature.Outputs(), Authority: value.signature.Authority(),
		Cardinality: value.signature.Cardinality(), Outcomes: outcomesFor(),
	}
	twoInput, ok := signature.Seal(spec)
	if !ok {
		t.Fatalf("seal cross-owner signature")
	}
	present, _ := model.NewPresence(model.Present)
	firstCell, _ := binding.NewCell(value.inputToken, value.inputType, value.inputValue, present)
	secondCell, _ := binding.NewCell(otherToken, otherType, otherValue, present)
	firstSlot, _ := binding.NewScalarSlot(firstCell)
	secondSlot, _ := binding.NewScalarSlot(secondCell)
	frame, _ := binding.NewFrame(value.inputScope, firstSlot, secondSlot)
	if !frame.Validate(twoInput, value.runtime) {
		t.Fatalf("cross-owner frame rejected")
	}
	if value.inputToken.Witness().Same(otherToken.Witness()) {
		t.Fatalf("cross-owner frame collapsed denominator authority")
	}
	if !value.inputToken.Scope().Same(otherToken.Scope()) {
		t.Fatalf("cross-owner frame did not use one invocation scope")
	}
}

func TestDenominatorWitnessRejectsRowsOutsideMembership(t *testing.T) {
	exact, _ := model.NewCardinality(model.ExactlyOne, 0)
	value := newFixture(t, exact)
	outside := mustRow(t, value.relation, "row/outside-denominator")
	if _, ok := value.issuer.IssueCell(value.inputWitness, value.inputScope, value.input, outside); ok {
		t.Fatalf("row outside denominator membership was issued")
	}
}

func TestMembershipViewRejectsForeignDuplicateAndMutableSources(t *testing.T) {
	owner := issueOwner(t, "owner/membership")
	relation := issueRelation(t, owner, "relation/membership")
	foreignRelation := issueRelation(t, owner, "relation/membership-foreign")
	row := mustRow(t, relation, "row/membership")
	foreignRow := mustRow(t, foreignRelation, "row/membership-foreign")
	rows := []model.RowID{row}
	view, ok := binding.NewMembershipView(relation, rows)
	if !ok || !view.Contains(row) {
		t.Fatalf("valid membership view rejected")
	}
	rows[0] = foreignRow
	if !view.Contains(row) || view.Contains(foreignRow) {
		t.Fatalf("membership view retained mutable source storage")
	}
	if _, ok := binding.NewMembershipView(relation, []model.RowID{row, row}); ok {
		t.Fatalf("duplicate denominator rows accepted")
	}
	if _, ok := binding.NewMembershipView(relation, []model.RowID{foreignRow}); ok {
		t.Fatalf("same-owner foreign row accepted")
	}
}

func TestFrameEnforcesBoundedAndCompleteSpanDelivery(t *testing.T) {
	exact, _ := model.NewCardinality(model.ExactlyOne, 0)
	value := newFixture(t, exact)
	rowTwo := mustRow(t, value.relation, "row/span-2")
	rowThree := mustRow(t, value.relation, "row/span-3")
	membership, ok := binding.NewMembershipView(value.relation, []model.RowID{value.inputRow, rowTwo, rowThree})
	if !ok {
		t.Fatalf("construct span membership")
	}
	witness, ok := value.issuer.IssueDenominator(value.denominator, membership, content(t, "witness/span"))
	if !ok {
		t.Fatalf("construct span witness")
	}
	rows := []model.RowID{value.inputRow, rowTwo, rowThree}
	cells := make([]binding.Cell, 0, len(rows))
	for index, row := range rows {
		address, issueOK := value.issuer.IssueCell(witness, value.inputScope, value.input, row)
		if !issueOK {
			t.Fatalf("issue span address %d", index)
		}
		encoded, issueOK := value.issuer.IssueValue(value.inputType, content(t, "value/span-"+string(rune('0'+index))))
		if !issueOK {
			t.Fatalf("issue span value %d", index)
		}
		cell, cellOK := binding.NewCell(address, value.inputType, encoded, mustPresence(t, model.Present))
		if !cellOK {
			t.Fatalf("construct span cell %d", index)
		}
		cells = append(cells, cell)
	}
	bounded, ok := signature.NewBoundedSpanDelivery(2, value.denominator.Key())
	if !ok {
		t.Fatalf("construct bounded span delivery")
	}
	spanSpec := signature.Spec{
		Identity: value.signature.Identity(), Fence: value.logical,
		Inputs:  []signature.Input{{Relation: value.relation, Column: value.input, Type: value.inputType, Presence: signature.RequirePresent, Delivery: bounded, Denominator: value.denominator}},
		Outputs: value.signature.Outputs(), Authority: value.signature.Authority(),
		Cardinality: value.signature.Cardinality(), Outcomes: outcomesFor(),
	}
	boundedSignature, ok := signature.Seal(spanSpec)
	if !ok {
		t.Fatalf("seal bounded span signature")
	}
	shortSlot, _ := binding.NewSpanSlot(cells[:2])
	shortFrame, _ := binding.NewFrame(value.inputScope, shortSlot)
	if !shortFrame.Validate(boundedSignature, value.runtime) {
		t.Fatalf("bounded span rejected")
	}
	longSlot, _ := binding.NewSpanSlot(cells)
	longFrame, _ := binding.NewFrame(value.inputScope, longSlot)
	if longFrame.Validate(boundedSignature, value.runtime) {
		t.Fatalf("bounded span exceeded its declared limit")
	}
	reversedCells := []binding.Cell{cells[1], cells[0]}
	reversedSlot, _ := binding.NewSpanSlot(reversedCells)
	reversedFrame, _ := binding.NewFrame(value.inputScope, reversedSlot)
	if reversedFrame.Validate(boundedSignature, value.runtime) {
		t.Fatalf("ordered span accepted descending rows")
	}
	complete, ok := signature.NewCompleteSpanDelivery(value.denominator.Key())
	if !ok {
		t.Fatalf("construct complete span delivery")
	}
	spanSpec.Inputs[0].Delivery = complete
	completeSignature, ok := signature.Seal(spanSpec)
	if !ok {
		t.Fatalf("seal complete span signature")
	}
	if !longFrame.Validate(completeSignature, value.runtime) {
		t.Fatalf("complete span rejected")
	}
	if shortFrame.Validate(completeSignature, value.runtime) {
		t.Fatalf("partial complete span accepted")
	}
}

func TestFrameUsesOneInvocationScopeAndAuthenticatesEmptyCompleteRange(t *testing.T) {
	exact, _ := model.NewCardinality(model.ExactlyOne, 0)
	value := newFixture(t, exact)
	emptyRows := []model.RowID{}
	emptyMembership, ok := binding.NewMembershipView(value.relation, emptyRows)
	if !ok {
		t.Fatalf("construct empty membership")
	}
	emptyWitness, ok := value.issuer.IssueDenominator(value.denominator, emptyMembership, content(t, "witness/empty"))
	if !ok {
		t.Fatalf("construct empty witness")
	}
	emptySlot, ok := binding.NewEmptySpanSlot(emptyWitness)
	if !ok {
		t.Fatalf("construct authenticated empty span")
	}
	complete, ok := signature.NewCompleteSpanDelivery(value.denominator.Key())
	if !ok {
		t.Fatalf("construct complete delivery")
	}
	spec := signature.Spec{
		Identity: value.signature.Identity(), Fence: value.logical,
		Inputs:  []signature.Input{{Relation: value.relation, Column: value.input, Type: value.inputType, Presence: signature.RequirePresent, Delivery: complete, Denominator: value.denominator}},
		Outputs: value.signature.Outputs(), Authority: value.signature.Authority(),
		Cardinality: value.signature.Cardinality(), Outcomes: outcomesFor(),
	}
	completeSignature, ok := signature.Seal(spec)
	if !ok {
		t.Fatalf("seal empty complete signature")
	}
	emptyFrame, ok := binding.NewFrame(value.inputScope, emptySlot)
	if !ok || !emptyFrame.Validate(completeSignature, value.runtime) {
		t.Fatalf("empty authenticated complete range rejected")
	}
	foreignScope, _ := value.issuer.IssueScope(content(t, "scope-formula/mixed"))
	foreignAddress, ok := value.issuer.IssueCell(value.inputWitness, foreignScope, value.input, value.inputRow)
	if !ok {
		t.Fatalf("issue mixed-scope address")
	}
	foreignCell, _ := binding.NewCell(foreignAddress, value.inputType, value.inputValue, mustPresence(t, model.Present))
	foreignSlot, _ := binding.NewScalarSlot(foreignCell)
	mixedFrame, _ := binding.NewFrame(value.inputScope, foreignSlot)
	scalarSpec := spec
	scalarSpec.Inputs[0].Delivery = scalarDelivery(t)
	scalarSignature, _ := signature.Seal(scalarSpec)
	if mixedFrame.Validate(scalarSignature, value.runtime) {
		t.Fatalf("mixed invocation scopes accepted")
	}
}

func TestAbsentCellsAndProposalsCarryNoValueHandle(t *testing.T) {
	exact, _ := model.NewCardinality(model.ExactlyOne, 0)
	value := newFixture(t, exact)
	absent, _ := model.NewPresence(model.ProvenAbsent)
	if _, ok := binding.NewCell(value.inputToken, value.inputType, binding.ValueToken{}, absent); !ok {
		t.Fatalf("absent cell required a fabricated value handle")
	}
	if _, ok := binding.NewProposal(value.outputToken, binding.ValueToken{}, absent); !ok {
		t.Fatalf("absent proposal required a fabricated value handle")
	}
}

func TestProposalBufferIsOutputBoundAndAllOrNothing(t *testing.T) {
	exact, _ := model.NewCardinality(model.ExactlyOne, 0)
	value := newFixture(t, exact)
	buffer, ok := binding.NewProposalBuffer(value.signature, value.runtime, value.outputWitness, value.outputScope)
	if !ok {
		t.Fatalf("construct proposal buffer")
	}
	present, _ := model.NewPresence(model.Present)
	wrongBuffer, ok := binding.NewProposalBuffer(value.signature, value.runtime, value.outputWitness, value.outputScope)
	if !ok {
		t.Fatalf("construct type-check buffer")
	}
	wrongProposal, ok := binding.NewProposal(value.outputToken, value.inputValue, present)
	if !ok || wrongBuffer.Append(wrongProposal) {
		t.Fatalf("foreign value type crossed output boundary")
	}
	proposal, ok := binding.NewProposal(value.outputToken, value.outputValue, present)
	if !ok || !buffer.Append(proposal) || buffer.Len() != 1 {
		t.Fatalf("valid proposal rejected")
	}
	rowTwo := mustRow(t, value.relation, "row/exactly-one-2")
	rowMembership, _ := binding.NewMembershipView(value.relation, []model.RowID{value.outputRow, rowTwo})
	rowWitness, _ := value.issuer.IssueDenominator(value.denominator, rowMembership, content(t, "witness/exactly-one"))
	rowFirstToken, _ := value.issuer.IssueCell(rowWitness, value.outputScope, value.output, value.outputRow)
	rowFirstValue, _ := value.issuer.IssueValue(value.outputType, content(t, "value/exactly-one-1"))
	rowFirstProposal, _ := binding.NewProposal(rowFirstToken, rowFirstValue, present)
	rowToken, _ := value.issuer.IssueCell(rowWitness, value.outputScope, value.output, rowTwo)
	rowValue, _ := value.issuer.IssueValue(value.outputType, content(t, "value/exactly-one-2"))
	rowProposal, _ := binding.NewProposal(rowToken, rowValue, present)
	singleBuffer, _ := binding.NewProposalBuffer(value.signature, value.runtime, rowWitness, value.outputScope)
	if !singleBuffer.Append(rowFirstProposal) || singleBuffer.Append(rowProposal) || singleBuffer.Len() != 0 {
		t.Fatalf("ExactlyOne buffer admitted more than one destination row")
	}
	foreignScope, _ := value.issuer.IssueScope(content(t, "scope-formula/foreign-output"))
	foreignToken, _ := value.issuer.IssueCell(value.outputWitness, foreignScope, value.output, value.outputRow)
	foreignProposal, ok := binding.NewProposal(foreignToken, value.outputValue, present)
	if !ok || buffer.Append(foreignProposal) || buffer.Len() != 0 {
		t.Fatalf("foreign output scope did not poison buffer atomically")
	}
	if _, ok := buffer.Seal(outcome.Result{Code: outcome.Produced}); ok {
		t.Fatalf("poisoned buffer yielded a partial batch")
	}
}

func TestBoundedManyAndReusableBatch(t *testing.T) {
	bounded, _ := model.NewCardinality(model.BoundedMany, 2)
	value := newFixture(t, bounded)
	rowTwo := mustRow(t, value.relation, "row/output-2")
	membership, _ := binding.NewMembershipView(value.relation, []model.RowID{value.outputRow, rowTwo})
	witness, ok := value.issuer.IssueDenominator(value.denominator, membership, content(t, "witness/bounded"))
	if !ok {
		t.Fatalf("construct bounded witness")
	}
	firstToken, _ := value.issuer.IssueCell(witness, value.outputScope, value.output, value.outputRow)
	secondToken, _ := value.issuer.IssueCell(witness, value.outputScope, value.output, rowTwo)
	firstValue, _ := value.issuer.IssueValue(value.outputType, content(t, "value/output-1"))
	secondValue, _ := value.issuer.IssueValue(value.outputType, content(t, "value/output-2"))
	present := mustPresence(t, model.Present)
	first, _ := binding.NewProposal(firstToken, firstValue, present)
	second, _ := binding.NewProposal(secondToken, secondValue, present)
	buffer, ok := binding.NewProposalBuffer(value.signature, value.runtime, witness, value.outputScope)
	if !ok || !buffer.Append(first) || !buffer.Append(second) {
		t.Fatalf("bounded proposals rejected")
	}
	batch, ok := buffer.Seal(outcome.Result{Code: outcome.Produced})
	if !ok || batch.Len() != 2 || batch.Outcome().Code != outcome.Produced {
		t.Fatalf("bounded batch mismatch")
	}
	if !buffer.Reset() || batch.Available() {
		t.Fatalf("buffer lease was not reset")
	}
	if !buffer.Append(first) {
		t.Fatalf("reused bounded buffer rejected proposal")
	}
	if _, ok := buffer.Seal(outcome.Result{Code: outcome.Produced}); !ok {
		t.Fatalf("reused bounded buffer did not seal")
	}
	optional, _ := model.NewCardinality(model.Optional, 0)
	optionalValue := newFixture(t, optional)
	empty, ok := binding.NewProposalBuffer(optionalValue.signature, optionalValue.runtime, optionalValue.outputWitness, optionalValue.outputScope)
	if !ok {
		t.Fatalf("construct optional buffer")
	}
	if batch, ok := empty.Seal(outcome.Result{Code: outcome.Produced}); !ok || !batch.Available() || batch.Len() != 0 {
		t.Fatalf("optional empty result rejected")
	}
}

func TestSealedBatchIsInvalidatedByIllegalBufferReuse(t *testing.T) {
	exact, _ := model.NewCardinality(model.ExactlyOne, 0)
	value := newFixture(t, exact)
	present := mustPresence(t, model.Present)
	proposal, ok := binding.NewProposal(value.outputToken, value.outputValue, present)
	if !ok {
		t.Fatal("proposal")
	}
	buffer, ok := binding.NewProposalBuffer(value.signature, value.runtime, value.outputWitness, value.outputScope)
	if !ok || !buffer.Append(proposal) {
		t.Fatal("proposal buffer")
	}
	batch, ok := buffer.Seal(outcome.Result{Code: outcome.Produced})
	if !ok || !batch.Available() || batch.Len() != 1 {
		t.Fatal("sealed batch")
	}
	if buffer.Append(proposal) {
		t.Fatal("closed buffer accepted a write")
	}
	if batch.Available() || batch.Len() != 0 {
		t.Fatal("stale batch survived illegal buffer reuse")
	}
}

func TestFactoryAdmitsOnlyTheExactSealedSignature(t *testing.T) {
	exact, _ := model.NewCardinality(model.ExactlyOne, 0)
	value := newFixture(t, exact)
	if admitted, ok := binding.Admit(testFactory{operation: value.signature}, value.signature); !ok || admitted == nil {
		t.Fatalf("exact binding was refused")
	}
	bounded, _ := model.NewCardinality(model.BoundedMany, 2)
	foreign := newFixture(t, bounded)
	if _, ok := binding.Admit(testFactory{operation: foreign.signature}, value.signature); ok {
		t.Fatalf("foreign signature was admitted")
	}
}

func TestAlgebraResolutionIsNominallyTypeBound(t *testing.T) {
	exact, _ := model.NewCardinality(model.ExactlyOne, 0)
	value := newFixture(t, exact)
	algebra := testAlgebra{typeID: value.inputType}
	if got, ok := binding.AdmitAlgebra(testAlgebraFactory{algebra: algebra}, value.inputType); !ok || got.Type() != value.inputType {
		t.Fatalf("matching type algebra was refused")
	}
	if _, ok := binding.AdmitAlgebra(testAlgebraFactory{algebra: algebra}, value.outputType); ok {
		t.Fatalf("foreign type algebra was admitted")
	}
	if got, ok := binding.ResolveAlgebra(testAlgebraRegistry{algebra: algebra}, value.inputType); !ok || got.Type() != value.inputType {
		t.Fatalf("matching registry algebra was refused")
	}
	if _, ok := binding.ResolveAlgebra(testAlgebraRegistry{algebra: algebra}, value.outputType); ok {
		t.Fatalf("registry returned algebra for a foreign type")
	}
}

func scalarDelivery(t *testing.T) signature.Delivery {
	t.Helper()
	delivery, ok := signature.NewScalarDelivery()
	if !ok {
		t.Fatalf("construct scalar delivery")
	}
	return delivery
}

func outcomesFor() outcome.Set {
	set, _ := outcome.NewSet(outcome.Produced, outcome.NoCandidate, outcome.NoSelection, outcome.Refused)
	return set
}

func issueOwner(t *testing.T, label string) model.OwnerID {
	t.Helper()
	owner, ok := model.IssueOwnerID(content(t, label))
	if !ok {
		t.Fatalf("issue owner %q", label)
	}
	return owner
}

func issueRelation(t *testing.T, owner model.OwnerID, label string) model.RelationID {
	t.Helper()
	relation, ok := model.IssueRelationID(owner, content(t, label))
	if !ok {
		t.Fatalf("issue relation %q", label)
	}
	return relation
}

func issueColumn(t *testing.T, relation model.RelationID, label string) model.ColumnID {
	t.Helper()
	column, ok := model.IssueColumnID(relation, content(t, label))
	if !ok {
		t.Fatalf("issue column %q", label)
	}
	return column
}

func issueKey(t *testing.T, relation model.RelationID, label string) model.KeyID {
	t.Helper()
	key, ok := model.IssueKeyID(relation, content(t, label))
	if !ok {
		t.Fatalf("issue key %q", label)
	}
	return key
}

func issueSchema(t *testing.T, owner model.OwnerID, label string) model.SchemaID {
	t.Helper()
	schema, ok := model.IssueSchemaID(owner, content(t, label))
	if !ok {
		t.Fatalf("issue schema %q", label)
	}
	return schema
}

func issueScope(t *testing.T, owner model.OwnerID, label string) model.ScopeID {
	t.Helper()
	scope, ok := model.IssueScopeID(owner, content(t, label))
	if !ok {
		t.Fatalf("issue scope %q", label)
	}
	return scope
}

func issueOperation(t *testing.T, owner model.OwnerID, label string) model.OperationID {
	t.Helper()
	operation, ok := model.IssueOperationID(owner, content(t, label))
	if !ok {
		t.Fatalf("issue operation %q", label)
	}
	return operation
}

func issueType(t *testing.T, owner model.OwnerID, label string) model.TypeID {
	t.Helper()
	typeID, ok := model.IssueTypeID(owner, content(t, label))
	if !ok {
		t.Fatalf("issue type %q", label)
	}
	return typeID
}

func mustRow(t *testing.T, relation model.RelationID, label string) model.RowID {
	t.Helper()
	row, ok := model.IssueRowID(relation, content(t, label))
	if !ok {
		t.Fatalf("issue row %q", label)
	}
	return row
}

func mustPresence(t *testing.T, kind model.PresenceKind) model.Presence {
	t.Helper()
	presence, ok := model.NewPresence(kind)
	if !ok {
		t.Fatalf("issue presence %s", kind)
	}
	return presence
}

func TestOpaqueSealCarriesItsStagedRows(t *testing.T) {
	exact, _ := model.NewCardinality(model.ExactlyOne, 0)
	value := newFixture(t, exact)
	buffer, ok := binding.NewProposalBuffer(value.signature, value.runtime, value.outputWitness, value.outputScope)
	if !ok {
		t.Fatalf("construct proposal buffer")
	}
	present := mustPresence(t, model.Present)
	proposal, ok := binding.NewProposal(value.outputToken, value.outputValue, present)
	if !ok || !buffer.Append(proposal) {
		t.Fatalf("valid proposal rejected")
	}
	batch, ok := buffer.Seal(outcome.Result{Code: outcome.Opaque})
	if !ok || batch.Len() != 1 || batch.Outcome().Code != outcome.Opaque {
		t.Fatalf("an authenticated-opaque seal dropped its staged row: ok=%v len=%d", ok, batch.Len())
	}
}
