package relation_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
	"github.com/wippyai/go-lua/analysis/schema/rule/relbindgen/harness"
	calldomain "github.com/wippyai/go-lua/domain/call"
	"github.com/wippyai/go-lua/domain/call/relation"
	"github.com/wippyai/go-lua/domain/relationfixture"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

const reserve = 64

// TestDispatchAnswersUnderItsOwnVocabulary drives the real dispatch fold. What
// the law states is not which arm the fixture reaches but that whatever the
// owner answers is inside the sealed outcome set, and that an answer carrying
// no fact publishes no row.
func TestDispatchAnswersUnderItsOwnVocabulary(t *testing.T) {
	fixture := relationfixture.New(t)
	place := harness.New(t, "row/call")
	callType := place.TypeID(t, "type/call")
	valueType := place.TypeID(t, "type/value")
	candidateType := place.TypeID(t, "type/call-coordinate")
	callColumn := harness.NewColumn[calldomain.Value](t, callType, "store/call", reserve)
	valueColumn := harness.NewColumn[valuedomain.Value](t, valueType, "store/value", reserve)
	candidateColumn := harness.NewColumn[calldomain.CallCoordinate](t, candidateType, "store/call-coordinate", reserve)
	columns, ok := relation.NewCallDispatchColumns(valueColumn, callColumn, candidateColumn)
	if !ok {
		t.Fatal("dispatch columns")
	}
	candidateAddress := place.Column(t, "column/candidate")
	calleeAddress := place.Column(t, "column/callee")
	factAddress := place.Column(t, "column/fact")
	cardinality, ok := model.NewCardinality(model.ExactlyOne, 0)
	if !ok {
		t.Fatal("cardinality")
	}
	operation := place.Seal(t, "operation/call-dispatch",
		[]signature.Input{
			harness.ScalarInput(t, place.Relation, candidateAddress, candidateType, place.Denominator),
			harness.ScalarInput(t, place.Relation, calleeAddress, valueType, place.Denominator),
		},
		[]signature.Output{{Relation: place.Relation, Column: factAddress, Type: callType, Presence: signature.ProducePresent, Denominator: place.Denominator}},
		cardinality, outcome.Produced, outcome.NoCandidate, outcome.NoSelection, outcome.Opaque, outcome.Refused)
	judgment, ok := relation.NewCallDispatchOperation(fixture.Calls, fixture.Values, fixture.Heap)
	if !ok {
		t.Fatal("dispatch judgment")
	}
	factory, ok := relation.BindCallDispatch(operation, judgment, columns, place.Refusal)
	if !ok {
		t.Fatal("bind call dispatch")
	}
	worker := place.Worker(t, factory, operation)

	candidateToken, ok := candidateColumn.Encode(place.Issuer, calldomain.CallCoordinate{})
	if !ok {
		t.Fatal("encode candidate")
	}
	calleeToken, ok := valueColumn.Encode(place.Issuer, fixture.Values.Top())
	if !ok {
		t.Fatal("encode callee")
	}
	frame := place.Frame(t,
		harness.ScalarSlot(t, place.Cell(t, candidateAddress, place.Rows[0], candidateType, candidateToken)),
		harness.ScalarSlot(t, place.Cell(t, calleeAddress, place.Rows[0], valueType, calleeToken)),
	)
	buffer := place.Buffer(t, operation)
	result := worker.Evaluate(frame, buffer)
	batch, sealed := buffer.Seal(result)
	if !sealed || !operation.Allows(result.Code) {
		t.Fatalf("dispatch settled outside its own vocabulary: %v", result.Code)
	}
	if result.Code == outcome.Produced {
		proposal, proposalOK := batch.At(0)
		if batch.Len() != 1 || !proposalOK || proposal.Destination().Row() != place.Rows[0] || proposal.Destination().Column() != factAddress {
			t.Fatal("a produced dispatch did not publish at the candidate's own declared destination")
		}
		return
	}
	if batch.Len() != 0 {
		t.Fatalf("a dispatch that produced nothing published %d rows", batch.Len())
	}
}

// TestTheCallAlgebraResolvesByTypeAlone states this axis's ascent authority is
// keyed by TypeID, so the state layer proves an ascent without asking a
// binding which operation produced the value.
func TestTheCallAlgebraResolvesByTypeAlone(t *testing.T) {
	fixture := relationfixture.New(t)
	place := harness.New(t, "row/call")
	types := relation.PayloadTypes{Call: place.TypeID(t, "type/call"), CallCandidate: place.TypeID(t, "type/call-candidate")}
	tags := relation.PayloadTags{Call: harness.Content(t, "store/call"), CallCandidate: harness.Content(t, "store/call-candidate")}
	callType := types.Call
	payloads, ok := relation.NewPayloads(types, tags, reserve)
	if !ok {
		t.Fatal("install the call columns")
	}
	witness, ok := relation.NewCallLattice(fixture.Calls)
	if !ok {
		t.Fatal("call lattice witness")
	}
	algebras, ok := payloads.Algebras(place.Issuer, relation.Lattices{Call: witness})
	if !ok || len(algebras) != 1 || algebras[0].Type() != callType {
		t.Fatal("the call axis did not state one ascent authority for its own TypeID")
	}
	bottom, ok := payloads.Call.Encode(place.Issuer, fixture.Calls.Bottom())
	if !ok {
		t.Fatal("encode call bottom")
	}
	top, ok := payloads.Call.Encode(place.Issuer, fixture.Calls.Top())
	if !ok {
		t.Fatal("encode call top")
	}
	joined, ok := algebras[0].Join(bottom, top)
	if !ok || !algebras[0].LessOrEqual(bottom, joined) || !algebras[0].LessOrEqual(top, joined) {
		t.Fatal("the call join was not an upper bound of both operands")
	}
}

// TestTheCallBoundaryDoesNotAllocate holds the generic boundary to zero
// allocations for this axis's own payload.
func TestTheCallBoundaryDoesNotAllocate(t *testing.T) {
	fixture := relationfixture.New(t)
	place := harness.New(t, "row/call")
	column := harness.NewColumn[calldomain.Value](t, place.TypeID(t, "type/call"), "store/call", 1<<20)
	top := fixture.Calls.Top()
	if allocations := testing.AllocsPerRun(200, func() {
		token, ok := column.Encode(place.Issuer, top)
		if !ok {
			t.Fatal("encode call value")
		}
		if _, ok := column.Decode(token); !ok {
			t.Fatal("decode call value")
		}
	}); allocations != 0 {
		t.Fatalf("the call boundary allocated %.0f times", allocations)
	}
}

// TestActivationPublishesItsDispositionAndNoFact drives the outcome-only
// signature form against the real branch selector.
//
// A structural rule answers whether its occurrence holds and stages nothing.
// The ABI reads that off the contract: a signature that declares no output
// column is an operation that publishes no fact, and the emitter it is handed
// is opened at a capacity of none, so publishing is not merely something this
// family does not do but something it cannot.
func TestActivationPublishesItsDispositionAndNoFact(t *testing.T) {
	fixture := relationfixture.New(t)
	place := harness.New(t, "row/activation")
	callType := place.TypeID(t, "type/call")
	candidateType := place.TypeID(t, "type/call-coordinate")
	callColumn := harness.NewColumn[calldomain.Value](t, callType, "store/call", reserve)
	candidateColumn := harness.NewColumn[calldomain.CallCoordinate](t, candidateType, "store/call-coordinate", reserve)
	columns, ok := relation.NewCallActivationColumns(callColumn, candidateColumn)
	if !ok {
		t.Fatal("activation columns")
	}
	candidateAddress := place.Column(t, "column/candidate")
	triggerAddress := place.Column(t, "column/trigger")
	optional, ok := model.NewCardinality(model.Optional, 0)
	if !ok {
		t.Fatal("cardinality")
	}
	operation := place.Seal(t, "operation/call-activation",
		[]signature.Input{
			harness.ScalarInput(t, place.Relation, candidateAddress, candidateType, place.Denominator),
			harness.ScalarInput(t, place.Relation, triggerAddress, callType, place.Denominator),
		},
		nil,
		optional, outcome.Produced, outcome.NoSelection, outcome.Refused)
	judgment, ok := relation.NewCallActivationOperation(fixture.Calls)
	if !ok {
		t.Fatal("activation selector")
	}
	factory, ok := relation.BindCallActivation(operation, judgment, columns, place.Refusal)
	if !ok {
		t.Fatal("bind call activation")
	}
	worker := place.Worker(t, factory, operation)

	candidateToken, ok := candidateColumn.Encode(place.Issuer, calldomain.CallCoordinate{})
	if !ok {
		t.Fatal("encode candidate")
	}
	triggerToken, ok := callColumn.Encode(place.Issuer, fixture.Calls.Top())
	if !ok {
		t.Fatal("encode trigger")
	}
	frame := place.Frame(t,
		harness.ScalarSlot(t, place.Cell(t, candidateAddress, place.Rows[0], candidateType, candidateToken)),
		harness.ScalarSlot(t, place.Cell(t, triggerAddress, place.Rows[0], callType, triggerToken)),
	)
	buffer := place.Buffer(t, operation)
	result := worker.Evaluate(frame, buffer)
	batch, sealed := buffer.Seal(result)
	if !sealed || !operation.Allows(result.Code) {
		t.Fatalf("the activation settled outside its own vocabulary: %v", result.Code)
	}
	if batch.Len() != 0 {
		t.Fatalf("a family that publishes no fact published %d rows", batch.Len())
	}
}

// TestAFamilyThatPublishesNoFactIsRefusedAnEncoder states the other half of the
// form. The absence of a fact is read off the signature, so a spec that
// declares no output column and still carries an encoder is contradicting
// itself and is refused rather than quietly ignored.
func TestAFamilyThatPublishesNoFactIsRefusedAnEncoder(t *testing.T) {
	fixture := relationfixture.New(t)
	place := harness.New(t, "row/activation")
	callType := place.TypeID(t, "type/call")
	candidateType := place.TypeID(t, "type/call-coordinate")
	columns, ok := relation.NewCallActivationColumns(
		harness.NewColumn[calldomain.Value](t, callType, "store/call", reserve),
		harness.NewColumn[calldomain.CallCoordinate](t, candidateType, "store/call-coordinate", reserve))
	if !ok {
		t.Fatal("activation columns")
	}
	optional, ok := model.NewCardinality(model.Optional, 0)
	if !ok {
		t.Fatal("cardinality")
	}
	// One output column and the same judgment: the family now claims a fact it
	// has no encoder for, and admission refuses.
	published := place.Seal(t, "operation/call-activation-published",
		[]signature.Input{
			harness.ScalarInput(t, place.Relation, place.Column(t, "column/candidate"), candidateType, place.Denominator),
			harness.ScalarInput(t, place.Relation, place.Column(t, "column/trigger"), callType, place.Denominator),
		},
		[]signature.Output{{Relation: place.Relation, Column: place.Column(t, "column/fact"), Type: callType, Presence: signature.ProducePresent, Denominator: place.Denominator}},
		optional, outcome.Produced, outcome.Refused)
	judgment, ok := relation.NewCallActivationOperation(fixture.Calls)
	if !ok {
		t.Fatal("activation selector")
	}
	if _, admitted := relation.BindCallActivation(published, judgment, columns, place.Refusal); admitted {
		t.Fatal("a family that publishes no fact was admitted under a contract that declares one")
	}
}
