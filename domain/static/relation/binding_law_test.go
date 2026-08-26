package relation_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
	"github.com/wippyai/go-lua/analysis/schema/rule/relbindgen/harness"
	"github.com/wippyai/go-lua/domain/relationfixture"
	staticdomain "github.com/wippyai/go-lua/domain/static"
	"github.com/wippyai/go-lua/domain/static/relation"
)

const reserve = 64

// TestStaticTransferCarriesTheTypeFactItRead drives the real static transfer
// and states where the carried fact lands: the row the frame delivered.
func TestStaticTransferCarriesTheTypeFactItRead(t *testing.T) {
	fixture := relationfixture.New(t)
	place := harness.New(t, "row/transfer")
	staticType := place.TypeID(t, "type/static")
	column := harness.NewColumn[staticdomain.TypeFact](t, staticType, "store/static", reserve)
	columns, ok := relation.NewStaticTransferColumns(column)
	if !ok {
		t.Fatal("static transfer columns")
	}
	sourceAddress := place.Column(t, "column/source")
	storedAddress := place.Column(t, "column/stored")
	cardinality, ok := model.NewCardinality(model.ExactlyOne, 0)
	if !ok {
		t.Fatal("cardinality")
	}
	operation := place.Seal(t, "operation/static-transfer",
		[]signature.Input{harness.ScalarInput(t, place.Relation, sourceAddress, staticType, place.Denominator)},
		[]signature.Output{{Relation: place.Relation, Column: storedAddress, Type: staticType, Presence: signature.ProducePresent}},
		cardinality, outcome.Produced, outcome.Refused)
	factory, ok := relation.BindStaticTransfer(operation, relation.StaticTransferOperation{}, columns, place.Refusal)
	if !ok {
		t.Fatal("bind static transfer")
	}
	worker := place.Worker(t, factory, operation)

	top := fixture.Classes.TypeTop()
	token, ok := column.Encode(place.Issuer, top)
	if !ok {
		t.Fatal("encode type fact")
	}
	frame := place.Frame(t, harness.ScalarSlot(t, place.Cell(t, sourceAddress, place.Rows[0], staticType, token)))
	buffer := place.Buffer(t, operation)
	result := worker.Evaluate(frame, buffer)
	batch, sealed := buffer.Seal(result)
	if result.Code != outcome.Produced || !sealed || batch.Len() != 1 {
		t.Fatalf("static transfer outcome=%v sealed=%t rows=%d", result.Code, sealed, batch.Len())
	}
	proposal, ok := batch.At(0)
	if !ok || proposal.Destination().Row() != place.Rows[0] || proposal.Destination().Column() != storedAddress {
		t.Fatal("the transfer did not publish at the declared destination of the row it read")
	}
	carried, ok := column.Decode(proposal.Value())
	if !ok || !fixture.Classes.EqualTypeFact(top, carried) {
		t.Fatal("the transfer did not carry the type fact it read")
	}
}

// TestTheStaticAlgebraResolvesByTypeAlone states this axis's ascent authority
// is keyed by TypeID.
func TestTheStaticAlgebraResolvesByTypeAlone(t *testing.T) {
	fixture := relationfixture.New(t)
	place := harness.New(t, "row/static")
	var types relation.PayloadTypes
	var tags relation.PayloadTags
	place.InstallTypes(t, &types)
	place.InstallTags(t, &tags)
	staticType := types.Static
	payloads, ok := relation.NewPayloads(types, tags, reserve)
	if !ok {
		t.Fatal("install the static columns")
	}
	witness, ok := relation.NewStaticLattice(fixture.Classes)
	if !ok {
		t.Fatal("static lattice witness")
	}
	algebras, ok := payloads.Algebras(place.Issuer, relation.Lattices{Static: witness})
	if !ok || len(algebras) != 1 || algebras[0].Type() != staticType {
		t.Fatal("the static axis did not state one ascent authority for its own TypeID")
	}
	bottom, ok := payloads.Static.Encode(place.Issuer, fixture.Classes.TypeBottom())
	if !ok {
		t.Fatal("encode type bottom")
	}
	top, ok := payloads.Static.Encode(place.Issuer, fixture.Classes.TypeTop())
	if !ok {
		t.Fatal("encode type top")
	}
	joined, ok := algebras[0].Join(bottom, top)
	if !ok || !algebras[0].LessOrEqual(bottom, joined) || !algebras[0].LessOrEqual(top, joined) {
		t.Fatal("the static join was not an upper bound of both operands")
	}
}

// TestTheStaticBoundaryDoesNotAllocate holds the generic boundary to zero
// allocations for this axis's own payload.
func TestTheStaticBoundaryDoesNotAllocate(t *testing.T) {
	fixture := relationfixture.New(t)
	place := harness.New(t, "row/static")
	column := harness.NewColumn[staticdomain.TypeFact](t, place.TypeID(t, "type/static"), "store/static", 1<<20)
	top := fixture.Classes.TypeTop()
	if allocations := testing.AllocsPerRun(200, func() {
		token, ok := column.Encode(place.Issuer, top)
		if !ok {
			t.Fatal("encode type fact")
		}
		if _, ok := column.Decode(token); !ok {
			t.Fatal("decode type fact")
		}
	}); allocations != 0 {
		t.Fatalf("the static boundary allocated %.0f times", allocations)
	}
}
