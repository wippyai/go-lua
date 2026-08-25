package relation_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
	"github.com/wippyai/go-lua/analysis/schema/rule/relbindgen/harness"
	packdomain "github.com/wippyai/go-lua/domain/pack"
	"github.com/wippyai/go-lua/domain/pack/relation"
	"github.com/wippyai/go-lua/domain/relationfixture"
)

const reserve = 64

// TestPackSourceSeedsTheSourceItWasGiven drives the real pack source seed.
func TestPackSourceSeedsTheSourceItWasGiven(t *testing.T) {
	fixture := relationfixture.New(t)
	place := harness.New(t, "row/source")
	packType := place.TypeID(t, "type/pack")
	sourceType := place.TypeID(t, "type/pack-source")
	packColumn := harness.NewColumn[packdomain.Value](t, packType, "store/pack", reserve)
	sourceColumn := harness.NewColumn[packdomain.Source](t, sourceType, "store/pack-source", reserve)
	columns, ok := relation.NewPackSourceColumns(packColumn, sourceColumn)
	if !ok {
		t.Fatal("pack source columns")
	}
	sourceAddress := place.Column(t, "column/source")
	factAddress := place.Column(t, "column/fact")
	cardinality, ok := model.NewCardinality(model.ExactlyOne, 0)
	if !ok {
		t.Fatal("cardinality")
	}
	operation := place.Seal(t, "operation/pack-source",
		[]signature.Input{harness.ScalarInput(t, place.Relation, sourceAddress, sourceType, place.Denominator)},
		[]signature.Output{{Relation: place.Relation, Column: factAddress, Type: packType, Presence: signature.ProducePresent}},
		cardinality, outcome.Produced, outcome.NoCandidate, outcome.NoSelection, outcome.Opaque, outcome.Refused)
	factory, ok := relation.BindPackSource(operation, relation.PackSourceOperation{}, columns, place.Refusal)
	if !ok {
		t.Fatal("bind pack source")
	}
	worker := place.Worker(t, factory, operation)

	source, declared := fixture.Packs.SourceAt(0)
	if !declared {
		source = packdomain.Source{}
	}
	token, ok := sourceColumn.Encode(place.Issuer, source)
	if !ok {
		t.Fatal("encode pack source")
	}
	frame := place.Frame(t, harness.ScalarSlot(t, place.Cell(t, sourceAddress, place.Rows[0], sourceType, token)))
	buffer := place.Buffer(t, operation)
	result := worker.Evaluate(frame, buffer)
	batch, sealed := buffer.Seal(result)
	if !sealed || !operation.Allows(result.Code) {
		t.Fatalf("the pack seed settled outside its own vocabulary: %v", result.Code)
	}
	if result.Code == outcome.Produced {
		proposal, proposalOK := batch.At(0)
		if batch.Len() != 1 || !proposalOK || proposal.Destination().Row() != place.Rows[0] || proposal.Destination().Column() != factAddress {
			t.Fatal("a produced seed did not publish at the candidate's own declared destination")
		}
		return
	}
	if batch.Len() != 0 {
		t.Fatalf("a seed that produced nothing published %d rows", batch.Len())
	}
}

// TestThePackAlgebraResolvesByTypeAlone states this axis's ascent authority is
// keyed by TypeID. The owner spells its lattice as a struct of total
// operators, and it reaches the same generic surface as an owner that spells
// its lattice as methods.
func TestThePackAlgebraResolvesByTypeAlone(t *testing.T) {
	fixture := relationfixture.New(t)
	place := harness.New(t, "row/pack")
	packType := place.TypeID(t, "type/pack")
	types := relation.PayloadTypes{Pack: packType, PackSourceCandidate: place.TypeID(t, "type/pack-source")}
	tags := relation.PayloadTags{Pack: harness.Content(t, "store/pack"), PackSourceCandidate: harness.Content(t, "store/pack-source")}
	payloads, ok := relation.NewPayloads(types, tags, reserve)
	if !ok {
		t.Fatal("install the pack columns")
	}
	witness, ok := relation.NewPackLattice(fixture.Packs)
	if !ok {
		t.Fatal("pack lattice witness")
	}
	algebras, ok := payloads.Algebras(place.Issuer, relation.Lattices{Pack: witness})
	if !ok || len(algebras) != 1 || algebras[0].Type() != packType {
		t.Fatal("the pack axis did not state one ascent authority for its own TypeID")
	}
	bottom, ok := payloads.Pack.Encode(place.Issuer, fixture.Packs.Bottom())
	if !ok {
		t.Fatal("encode pack bottom")
	}
	top, ok := payloads.Pack.Encode(place.Issuer, fixture.Packs.Top())
	if !ok {
		t.Fatal("encode pack top")
	}
	joined, ok := algebras[0].Join(bottom, top)
	if !ok || !algebras[0].LessOrEqual(bottom, joined) || !algebras[0].LessOrEqual(top, joined) {
		t.Fatal("the pack join was not an upper bound of both operands")
	}
}

// TestThePackBoundaryDoesNotAllocate holds the generic boundary to zero
// allocations for this axis's own payload.
func TestThePackBoundaryDoesNotAllocate(t *testing.T) {
	fixture := relationfixture.New(t)
	place := harness.New(t, "row/pack")
	column := harness.NewColumn[packdomain.Value](t, place.TypeID(t, "type/pack"), "store/pack", 1<<20)
	top := fixture.Packs.Top()
	if allocations := testing.AllocsPerRun(200, func() {
		token, ok := column.Encode(place.Issuer, top)
		if !ok {
			t.Fatal("encode pack value")
		}
		if _, ok := column.Decode(token); !ok {
			t.Fatal("decode pack value")
		}
	}); allocations != 0 {
		t.Fatalf("the pack boundary allocated %.0f times", allocations)
	}
}
