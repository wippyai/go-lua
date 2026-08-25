package specimen_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
	"github.com/wippyai/go-lua/analysis/schema/rule/relbindgen"
)

// counter is a synthetic owner payload with a total ascent order. It stands in
// for a domain lattice wherever a law is about the binding substrate rather
// than about a particular owner's mathematics.
type counter uint64

type counterLattice struct{}

func (counterLattice) Join(left, right counter) (counter, bool) {
	if left > right {
		return left, true
	}
	return right, true
}

func (counterLattice) Widen(previous, next counter) (counter, bool) {
	return counterLattice{}.Join(previous, next)
}

func (counterLattice) LessOrEq(left, right counter) bool { return left <= right }

var _ binding.ValueAlgebra = (*relbindgen.Algebra[counter, counterLattice])(nil)

type counterArgument struct {
	value counter
}

type counterDecoder struct {
	column *relbindgen.Column[counter]
}

func (decoder counterDecoder) Decode(inputs relbindgen.Inputs) (counterArgument, bool) {
	value, ok := relbindgen.ScalarAt(inputs, 0, decoder.column)
	if !ok {
		return counterArgument{}, false
	}
	return counterArgument{value: value}, true
}

type counterEncoder struct {
	column *relbindgen.Column[counter]
}

func (encoder counterEncoder) Encode(outputs relbindgen.Outputs, value counter) bool {
	return relbindgen.PutColumn(outputs, 0, encoder.column, value)
}

// scriptedOperation replays one declared behaviour. It exists so a law can
// state what an owner operation did, never so the substrate can branch on a
// form.
type scriptedOperation struct {
	code      outcome.Code
	addressed []counter
	named     []namedEmission
	putAt     identity.ContentID
	put       bool
}

type namedEmission struct {
	key   identity.ContentID
	value counter
}

func (operation scriptedOperation) Evaluate(argument counterArgument, emitter *relbindgen.Emitter[counter]) outcome.Code {
	for _, value := range operation.addressed {
		if !emitter.Put(argument.value + value) {
			return outcome.Refused
		}
	}
	for _, emission := range operation.named {
		if !emitter.PutAt(emission.key, argument.value+emission.value) {
			return outcome.Refused
		}
	}
	if operation.putAt.Available() {
		if !emitter.PutAt(operation.putAt, argument.value) {
			return outcome.Refused
		}
	}
	if operation.put {
		if !emitter.Put(argument.value) {
			return outcome.Refused
		}
	}
	return operation.code
}

type counterFamily struct {
	place   harness
	input   model.ColumnID
	output  model.ColumnID
	typeID  model.TypeID
	column  *relbindgen.Column[counter]
	algebra *relbindgen.Algebra[counter, counterLattice]
}

func newCounterFamily(t testing.TB, place harness, reserve int) counterFamily {
	t.Helper()
	typeID := place.typeID(t, "type/counter")
	store, ok := relbindgen.NewStore[counter](content(t, "store/counter"), reserve)
	if !ok {
		t.Fatal("counter store")
	}
	column, ok := relbindgen.NewColumn(typeID, store)
	if !ok {
		t.Fatal("counter column")
	}
	algebra, ok := relbindgen.NewAlgebra[counter, counterLattice](column, place.issuer, counterLattice{})
	if !ok {
		t.Fatal("counter algebra")
	}
	return counterFamily{
		place:  place,
		input:  place.column(t, "column/counter-in"),
		output: place.column(t, "column/counter-out"),
		typeID: typeID, column: column, algebra: algebra,
	}
}

func (family counterFamily) scalarSignature(t testing.TB, label string, codes ...outcome.Code) signature.Signature {
	t.Helper()
	exact, ok := model.NewCardinality(model.ExactlyOne, 0)
	if !ok {
		t.Fatal("cardinality")
	}
	return family.place.seal(t, label,
		[]signature.Input{scalarInput(t, family.place.relation, family.input, family.typeID, family.place.denominator)},
		[]signature.Output{{Relation: family.place.relation, Column: family.output, Type: family.typeID, Presence: signature.ProducePresent}},
		exact, codes...)
}

func (family counterFamily) expansionSignature(t testing.TB, label string, bound uint32, codes ...outcome.Code) signature.Signature {
	t.Helper()
	many, ok := model.NewCardinality(model.BoundedMany, bound)
	if !ok {
		t.Fatal("cardinality")
	}
	return family.place.seal(t, label,
		[]signature.Input{scalarInput(t, family.place.relation, family.input, family.typeID, family.place.denominator)},
		[]signature.Output{{Relation: family.place.relation, Column: family.output, Type: family.typeID, Presence: signature.ProducePresent}},
		many, codes...)
}

func (family counterFamily) bind(t testing.TB, operation signature.Signature, script scriptedOperation, address int) binding.Factory {
	t.Helper()
	factory, ok := relbindgen.Bind(relbindgen.Spec[counterArgument, counter]{
		Signature: operation,
		Decoder:   counterDecoder{column: family.column},
		Encoder:   counterEncoder{column: family.column},
		Operation: script,
		Address:   address,
		Refusal:   family.place.refusal,
	})
	if !ok {
		t.Fatal("bind counter family")
	}
	return factory
}

func (family counterFamily) inputFrame(t testing.TB, row model.RowID, value counter) (binding.Frame, binding.ValueToken) {
	t.Helper()
	token, ok := family.column.Encode(family.place.issuer, value)
	if !ok {
		t.Fatal("encode counter")
	}
	cell := family.place.cell(t, family.input, row, family.typeID, token)
	return family.place.frame(t, scalarSlot(t, cell)), token
}

func TestBindingAdmitsOnlyItsExactSealedSignature(t *testing.T) {
	place := newHarness(t, "row/one", "row/two")
	family := newCounterFamily(t, place, 32)
	operation := family.scalarSignature(t, "operation/scalar", outcome.Produced, outcome.NoCandidate, outcome.Refused)
	factory := family.bind(t, operation, scriptedOperation{code: outcome.Produced, addressed: []counter{1}}, 0)
	if admitted, ok := binding.Admit(factory, operation); !ok || admitted == nil {
		t.Fatal("exact sealed signature refused")
	}
	drifted := family.scalarSignature(t, "operation/scalar-drifted", outcome.Produced, outcome.Refused)
	if drifted.Digest() == operation.Digest() {
		t.Fatal("mutation did not change the sealed contract")
	}
	if _, ok := binding.Admit(factory, drifted); ok {
		t.Fatal("drifted contract admitted")
	}
}

func TestScalarJudgmentPublishesAtTheRowItRead(t *testing.T) {
	place := newHarness(t, "row/one", "row/two")
	family := newCounterFamily(t, place, 32)
	operation := family.scalarSignature(t, "operation/scalar", outcome.Produced, outcome.NoCandidate, outcome.Refused)
	factory := family.bind(t, operation, scriptedOperation{code: outcome.Produced, addressed: []counter{5}}, 0)
	worker := place.worker(t, factory, operation)
	frame, _ := family.inputFrame(t, place.rows[1], 3)
	buffer := place.buffer(t, operation)
	result := worker.Evaluate(frame, buffer)
	if result.Code != outcome.Produced {
		t.Fatalf("scalar judgment outcome %v", result.Code)
	}
	batch, ok := buffer.Seal(result)
	if !ok || batch.Len() != 1 {
		t.Fatalf("scalar batch ok=%t len=%d", ok, batch.Len())
	}
	proposal, ok := batch.At(0)
	if !ok || proposal.Destination().Row() != place.rows[1] || proposal.Destination().Column() != family.output {
		t.Fatal("scalar judgment did not publish at the row it read")
	}
	value, ok := family.column.Decode(proposal.Value())
	if !ok || value != 8 {
		t.Fatalf("published value %d ok=%t", value, ok)
	}
}

func TestOutcomeVocabularyStaysClosed(t *testing.T) {
	place := newHarness(t, "row/one", "row/two")
	family := newCounterFamily(t, place, 32)
	operation := family.scalarSignature(t, "operation/scalar", outcome.Produced, outcome.NoCandidate, outcome.Refused)

	declared := family.bind(t, operation, scriptedOperation{code: outcome.NoCandidate}, 0)
	worker := place.worker(t, declared, operation)
	frame, _ := family.inputFrame(t, place.rows[0], 1)
	buffer := place.buffer(t, operation)
	result := worker.Evaluate(frame, buffer)
	if result.Code != outcome.NoCandidate || result.RefusalID.Available() {
		t.Fatalf("declared empty outcome settled as %v", result.Code)
	}
	if batch, ok := buffer.Seal(result); !ok || batch.Len() != 0 {
		t.Fatal("no-candidate outcome carried rows")
	}

	undeclared := family.bind(t, operation, scriptedOperation{code: outcome.Opaque}, 0)
	worker = place.worker(t, undeclared, operation)
	frame, _ = family.inputFrame(t, place.rows[0], 1)
	buffer = place.buffer(t, operation)
	result = worker.Evaluate(frame, buffer)
	if result.Code != outcome.Refused || result.RefusalID != place.refusal {
		t.Fatalf("outcome outside the sealed vocabulary settled as %v", result.Code)
	}
}

func TestExpansionIsFiniteUnderItsDeclaredDenominator(t *testing.T) {
	place := newHarness(t, "row/one", "row/two", "row/three")
	family := newCounterFamily(t, place, 64)
	operation := family.expansionSignature(t, "operation/expansion", 2, outcome.Produced, outcome.NoCandidate, outcome.Refused)
	within := []namedEmission{
		{key: content(t, "row/one"), value: 1},
		{key: content(t, "row/two"), value: 2},
	}
	factory := family.bind(t, operation, scriptedOperation{code: outcome.Produced, named: within}, relbindgen.KeyedDestination)
	worker := place.worker(t, factory, operation)
	frame, _ := family.inputFrame(t, place.rows[0], 10)
	buffer := place.buffer(t, operation)
	result := worker.Evaluate(frame, buffer)
	batch, ok := buffer.Seal(result)
	if result.Code != outcome.Produced || !ok || batch.Len() != 2 {
		t.Fatalf("bounded expansion outcome=%v ok=%t len=%d", result.Code, ok, batch.Len())
	}

	beyond := append(append([]namedEmission(nil), within...), namedEmission{key: content(t, "row/three"), value: 3})
	factory = family.bind(t, operation, scriptedOperation{code: outcome.Produced, named: beyond}, relbindgen.KeyedDestination)
	worker = place.worker(t, factory, operation)
	frame, _ = family.inputFrame(t, place.rows[0], 10)
	buffer = place.buffer(t, operation)
	result = worker.Evaluate(frame, buffer)
	if result.Code != outcome.Refused || result.RefusalID != place.refusal {
		t.Fatalf("expansion past its declared bound settled as %v", result.Code)
	}
	if batch, ok := buffer.Seal(result); !ok || batch.Len() != 0 {
		t.Fatal("expansion past its declared bound published rows")
	}

	foreign := []namedEmission{{key: content(t, "row/outside-denominator"), value: 1}}
	factory = family.bind(t, operation, scriptedOperation{code: outcome.Produced, named: foreign}, relbindgen.KeyedDestination)
	worker = place.worker(t, factory, operation)
	frame, _ = family.inputFrame(t, place.rows[0], 10)
	buffer = place.buffer(t, operation)
	result = worker.Evaluate(frame, buffer)
	if result.Code != outcome.Refused {
		t.Fatalf("row outside the mounted denominator settled as %v", result.Code)
	}
	if _, ok := buffer.Seal(result); ok {
		t.Fatal("a refusal that had already staged rows yielded a partial batch")
	}
}

func TestDestinationFormIsDeclaredNeverChosenByTheOperation(t *testing.T) {
	place := newHarness(t, "row/one", "row/two")
	family := newCounterFamily(t, place, 32)

	scalar := family.scalarSignature(t, "operation/scalar", outcome.Produced, outcome.Refused)
	factory := family.bind(t, scalar, scriptedOperation{code: outcome.Produced, putAt: content(t, "row/one")}, 0)
	worker := place.worker(t, factory, scalar)
	frame, _ := family.inputFrame(t, place.rows[0], 1)
	buffer := place.buffer(t, scalar)
	if result := worker.Evaluate(frame, buffer); result.Code != outcome.Refused {
		t.Fatal("an addressed binding accepted an owner-named destination")
	}

	expansion := family.expansionSignature(t, "operation/expansion", 2, outcome.Produced, outcome.Refused)
	factory = family.bind(t, expansion, scriptedOperation{code: outcome.Produced, put: true}, relbindgen.KeyedDestination)
	worker = place.worker(t, factory, expansion)
	frame, _ = family.inputFrame(t, place.rows[0], 1)
	buffer = place.buffer(t, expansion)
	if result := worker.Evaluate(frame, buffer); result.Code != outcome.Refused {
		t.Fatal("a keyed expansion published at an address it was never given")
	}
}

func TestUpdateProposalsAreAscentsOfTheCellTheyRead(t *testing.T) {
	place := newHarness(t, "row/one", "row/two")
	family := newCounterFamily(t, place, 64)
	operation := family.scalarSignature(t, "operation/update", outcome.Produced, outcome.Refused)
	factory := family.bind(t, operation, scriptedOperation{code: outcome.Produced, addressed: []counter{0}}, 0)
	worker := place.worker(t, factory, operation)
	for _, current := range []counter{0, 1, 7, 4096} {
		frame, token := family.inputFrame(t, place.rows[0], current)
		buffer := place.buffer(t, operation)
		result := worker.Evaluate(frame, buffer)
		batch, ok := buffer.Seal(result)
		if result.Code != outcome.Produced || !ok || batch.Len() != 1 {
			t.Fatalf("update outcome=%v ok=%t", result.Code, ok)
		}
		proposal, ok := batch.At(0)
		if !ok || !family.algebra.LessOrEqual(token, proposal.Value()) {
			t.Fatalf("update proposal for %d was not an ascent", current)
		}
		if !buffer.Reset() {
			t.Fatal("buffer reset")
		}
	}
}

func TestStoreHandlesAreFencedToOneStoreAndOneEpoch(t *testing.T) {
	place := newHarness(t, "row/one")
	first, ok := relbindgen.NewStore[counter](content(t, "store/first"), 4)
	if !ok {
		t.Fatal("first store")
	}
	second, ok := relbindgen.NewStore[counter](content(t, "store/second"), 4)
	if !ok {
		t.Fatal("second store")
	}
	handle, ok := first.Intern(11)
	if !ok {
		t.Fatal("intern")
	}
	if value, ok := first.Load(handle); !ok || value != 11 {
		t.Fatal("issuing store refused its own handle")
	}
	if _, ok := second.Load(handle); ok {
		t.Fatal("foreign store resolved a handle it never issued")
	}
	if !first.Reset() {
		t.Fatal("reset")
	}
	if _, ok := first.Intern(22); !ok {
		t.Fatal("intern after reset")
	}
	if _, ok := first.Load(handle); ok {
		t.Fatal("stale epoch handle resolved after reset")
	}
	typeID := place.typeID(t, "type/counter")
	other := place.typeID(t, "type/other")
	column, ok := relbindgen.NewColumn(typeID, first)
	if !ok {
		t.Fatal("column")
	}
	token, ok := column.Encode(place.issuer, 33)
	if !ok {
		t.Fatal("encode")
	}
	foreign, ok := place.issuer.IssueValue(other, token.Opaque())
	if !ok {
		t.Fatal("foreign token")
	}
	if _, ok := column.Decode(foreign); ok {
		t.Fatal("column decoded a token of another type")
	}
}

func TestAlgebraIsResolvedByTypeIdentityAlone(t *testing.T) {
	place := newHarness(t, "row/one")
	family := newCounterFamily(t, place, 16)
	if got, ok := binding.ResolveAlgebra(algebraRegistry{algebra: family.algebra}, family.typeID); !ok || got.Type() != family.typeID {
		t.Fatal("algebra refused its own type")
	}
	if _, ok := binding.ResolveAlgebra(algebraRegistry{algebra: family.algebra}, place.typeID(t, "type/other")); ok {
		t.Fatal("algebra answered for a foreign type")
	}
	left, ok := family.column.Encode(place.issuer, 3)
	if !ok {
		t.Fatal("encode left")
	}
	right, ok := family.column.Encode(place.issuer, 9)
	if !ok {
		t.Fatal("encode right")
	}
	joined, ok := family.algebra.Join(left, right)
	if !ok || !family.algebra.LessOrEqual(left, joined) || !family.algebra.LessOrEqual(right, joined) {
		t.Fatal("join was not an upper bound of both operands")
	}
	widened, ok := family.algebra.Widen(left, right)
	if !ok || !family.algebra.LessOrEqual(joined, widened) {
		t.Fatal("widen did not dominate join")
	}
}

type algebraRegistry struct {
	algebra binding.ValueAlgebra
}

func (registry algebraRegistry) Resolve(typeID model.TypeID) (binding.ValueAlgebra, bool) {
	if registry.algebra.Type() != typeID {
		return nil, false
	}
	return registry.algebra, true
}

func TestOwnerColumnBoundaryDoesNotAllocate(t *testing.T) {
	place := newHarness(t, "row/one")
	family := newCounterFamily(t, place, 1<<20)
	left, ok := family.column.Encode(place.issuer, 3)
	if !ok {
		t.Fatal("encode")
	}
	right, ok := family.column.Encode(place.issuer, 9)
	if !ok {
		t.Fatal("encode")
	}
	if allocations := testing.AllocsPerRun(200, func() {
		joined, joinOK := family.algebra.Join(left, right)
		if !joinOK || !family.algebra.LessOrEqual(left, joined) {
			t.Fatal("join round trip")
		}
		if _, decodeOK := family.column.Decode(joined); !decodeOK {
			t.Fatal("decode round trip")
		}
	}); allocations != 0 {
		t.Fatalf("owner column boundary allocated %.0f times per round trip", allocations)
	}
}
