package reduction_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/relation/state/contribution"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/contribution/reduction"
	fixture "github.com/wippyai/go-lua/analysis/engine/testdata/relationfixture/arithmetic"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/schema/semantic/output"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
	"github.com/wippyai/go-lua/analysis/relation/semantic/invocation"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
)

type reductionFixture struct {
	world       fixture.Fixture
	port        output.OutputPort
	addressPort output.OutputPort
	spec        output.ContributionSpec
	cell        binding.CellToken
	target      contribution.Target
	rows        []contribution.Row
}

func newReductionFixture(t *testing.T) reductionFixture {
	t.Helper()
	world := fixture.New(t, 0xD1)
	declaration := world.Declaration()
	ids := world.IDs()
	capability, ok := model.NewAscendingCapability(ids.Type)
	if !ok {
		t.Fatal("ascending capability")
	}
	port := output.OutputPort{Operation: declaration.Arithmetic.Identity(), Column: ids.OutputWrite}
	spec, ok := output.Seal(output.Spec{Signature: declaration.Arithmetic, Port: port, ValueType: ids.Type, Algebra: capability, Reducer: output.Contributions})
	if !ok {
		t.Fatal("contribution spec")
	}
	addressPort := output.OutputPort{Operation: declaration.Arithmetic.Identity(), Column: ids.OutputAddress}
	witnessValue, ok := world.Mounted().Denominator(declaration.Outputs)
	if !ok {
		t.Fatal("output denominator")
	}
	scope, ok := world.Mounted().Scope(ids.Scope)
	if !ok {
		t.Fatal("scope")
	}
	cell, ok := world.Mounted().IssueCell(witnessValue, scope, ids.OutputWrite, ids.OutputA)
	if !ok {
		t.Fatal("output cell")
	}
	target, ok := contribution.NewTarget(port, ids.OutputA)
	if !ok {
		t.Fatal("target")
	}
	directory, ok := contribution.NewDirectory(world.Mounted().RuntimeFence())
	if !ok {
		t.Fatal("contribution directory")
	}
	lineage, ok := world.Mounted().RowLineage(ids.OutputA)
	if !ok {
		t.Fatal("lineage")
	}
	rows := make([]contribution.Row, 0, 2)
	for index, source := range []model.RowID{ids.CandidateA, ids.CandidateB} {
		tuple, tupleOK := invocation.NewTupleSources([]model.RowID{source})
		if !tupleOK {
			t.Fatalf("tuple %d", index)
		}
		vector, vectorOK := invocation.NewSourceVector([]invocation.TupleSources{tuple})
		if !vectorOK {
			t.Fatalf("vector %d", index)
		}
		token, tokenOK := world.Mounted().ScopeToken(scope)
		if !tokenOK {
			t.Fatalf("scope token %d", index)
		}
		address, addressOK := invocation.New(token, []invocation.SourceVector{vector})
		if !addressOK {
			t.Fatalf("address %d", index)
		}
		var handle contribution.Handle
		var internOK bool
		directory, handle, internOK = directory.Intern(address)
		if !internOK {
			t.Fatalf("handle %d", index)
		}
		key, keyOK := contribution.NewKey(handle, port, ids.OutputA)
		if !keyOK {
			t.Fatalf("key %d", index)
		}
		opaque := identity.ContentID{byte(40 + index)}
		value, valueOK := world.Mounted().IssueValue(ids.Type, opaque)
		if !valueOK {
			t.Fatalf("value %d", index)
		}
		presence, presenceOK := model.NewPresence(model.Present)
		if !presenceOK {
			t.Fatalf("presence %d", index)
		}
		row, rowOK := contribution.NewRow(key, cell, value, presence, lineage)
		if !rowOK {
			t.Fatalf("row %d", index)
		}
		rows = append(rows, row)
	}
	return reductionFixture{world: world, port: port, addressPort: addressPort, spec: spec, cell: cell, target: target, rows: rows}
}

func outputSpec(t *testing.T, value reductionFixture, contract signature.PresenceContract) output.ContributionSpec {
	t.Helper()
	declaration := value.world.Declaration()
	outputs := declaration.Arithmetic.Outputs()
	if len(outputs) < 2 {
		t.Fatal("arithmetic outputs")
	}
	outputs[1].Presence = contract
	signed, ok := signature.Seal(signature.Spec{Identity: declaration.Arithmetic.Identity(), Fence: declaration.Arithmetic.Fence(), Outputs: outputs, Cardinality: declaration.Arithmetic.Cardinality(), Outcomes: declaration.Arithmetic.Outcomes()})
	if !ok {
		t.Fatal("presence signature")
	}
	capability, ok := model.NewAscendingCapability(value.world.IDs().Type)
	if !ok {
		t.Fatal("capability")
	}
	spec, ok := output.Seal(output.Spec{Signature: signed, Port: value.port, ValueType: value.world.IDs().Type, Algebra: capability, Reducer: output.Contributions})
	if !ok {
		t.Fatal("output spec")
	}
	return spec
}

func TestReducePresentJoinsTwoProducersAndIsPermutationStable(t *testing.T) {
	value := newReductionFixture(t)
	first, ok := reduction.Reduce(value.target, value.rows, value.world.Mounted(), value.spec)
	if !ok || !first.Available() || first.Removal() {
		t.Fatal("present reduction")
	}
	second, ok := reduction.Reduce(value.target, []contribution.Row{value.rows[1], value.rows[0]}, value.world.Mounted(), value.spec)
	if !ok || !second.Available() {
		t.Fatal("permuted reduction")
	}
	firstValue, firstValueOK := first.Value()
	secondValue, secondValueOK := second.Value()
	if !firstValueOK || !secondValueOK || !firstValue.Same(secondValue) {
		t.Fatal("permutation changed joined value")
	}
	firstLineage, firstLineageOK := first.Lineage()
	secondLineage, secondLineageOK := second.Lineage()
	if !firstLineageOK || !secondLineageOK || firstLineage != secondLineage {
		t.Fatal("permutation changed lineage")
	}
	firstCell, firstCellOK := first.Destination()
	if !firstCellOK || !firstCell.Same(value.cell) {
		t.Fatal("destination token was not retained")
	}
}

func TestReduceEmptyIsExplicitSparseRemovalAndRetainsCellWithReduceAt(t *testing.T) {
	value := newReductionFixture(t)
	removed, ok := reduction.ReduceAt(value.target, nil, value.cell, value.world.Mounted(), value.spec)
	if !ok || !removed.Available() || !removed.Removal() {
		t.Fatal("sparse removal")
	}
	if _, present := removed.Presence(); present {
		t.Fatal("sparse removal fabricated Presence")
	}
	if _, present := removed.Value(); present {
		t.Fatal("sparse removal fabricated value")
	}
	destination, present := removed.Destination()
	if !present || !destination.Same(value.cell) {
		t.Fatal("sparse removal lost exact destination")
	}
	if _, ok := reduction.Reduce(value.target, nil, value.world.Mounted(), value.spec); ok {
		t.Fatal("row-id target fabricated sparse destination")
	}
}

func TestReduceRejectsMixedTargetAndSiblingColumn(t *testing.T) {
	value := newReductionFixture(t)
	otherTarget, ok := contribution.NewTarget(value.addressPort, value.target.Destination)
	if !ok {
		t.Fatal("sibling target")
	}
	otherSpec := outputSpecForColumn(t, value, value.addressPort)
	if _, ok := reduction.Reduce(otherTarget, value.rows, value.world.Mounted(), otherSpec); ok {
		t.Fatal("sibling column reused contribution row")
	}
	if _, ok := reduction.Reduce(value.target, value.rows[:1], value.world.Mounted(), otherSpec); ok {
		t.Fatal("mixed output port accepted")
	}
}

func outputSpecForColumn(t *testing.T, value reductionFixture, port output.OutputPort) output.ContributionSpec {
	t.Helper()
	declaration := value.world.Declaration()
	capability, ok := model.NewAscendingCapability(value.world.IDs().Type)
	if !ok {
		t.Fatal("capability")
	}
	spec, ok := output.Seal(output.Spec{Signature: declaration.Arithmetic, Port: port, ValueType: value.world.IDs().Type, Algebra: capability, Reducer: output.Contributions})
	if !ok {
		t.Fatal("sibling spec")
	}
	return spec
}

func TestReduceOpaqueRequiresExactTokenEquality(t *testing.T) {
	value := newReductionFixture(t)
	spec := outputSpec(t, value, signature.ProduceOpaque)
	opaqueRows := make([]contribution.Row, len(value.rows))
	for index, row := range value.rows {
		presence, ok := model.NewPresence(model.AuthenticatedOpaque)
		if !ok {
			t.Fatal("opaque presence")
		}
		opaqueRows[index], ok = contribution.NewRow(row.Key, row.Destination, value.rows[0].Value, presence, row.Lineage)
		if !ok {
			t.Fatal("opaque row")
		}
	}
	if reduced, ok := reduction.Reduce(value.target, opaqueRows, value.world.Mounted(), spec); !ok || !reduced.Available() {
		t.Fatal("equal opaque rows refused")
	}
	if _, ok := reduction.Reduce(value.target, value.rowsForDifferentValues(), value.world.Mounted(), spec); ok {
		t.Fatal("different opaque tokens were accepted")
	}
}

func (value reductionFixture) rowsForDifferentValues() []contribution.Row {
	rows := append([]contribution.Row(nil), value.rows...)
	issuer, _ := binding.NewIssuer(value.world.Mounted().RuntimeFence())
	presence, _ := model.NewPresence(model.AuthenticatedOpaque)
	firstValue, _ := issuer.IssueValue(value.world.IDs().Type, identity.ContentID{98})
	secondValue, _ := issuer.IssueValue(value.world.IDs().Type, identity.ContentID{99})
	rows[0], _ = contribution.NewRow(rows[0].Key, rows[0].Destination, firstValue, presence, rows[0].Lineage)
	rows[1], _ = contribution.NewRow(rows[1].Key, rows[1].Destination, secondValue, presence, rows[1].Lineage)
	return rows
}

func TestReduceRefusesOptionalAndAbsentWithoutProducerDenominatorProof(t *testing.T) {
	value := newReductionFixture(t)
	for _, contract := range []signature.PresenceContract{signature.ProduceOptional, signature.ProduceAbsent} {
		spec := outputSpec(t, value, contract)
		if _, ok := reduction.Reduce(value.target, value.rows, value.world.Mounted(), spec); ok {
			t.Fatalf("contract %d accepted without closed-world producer proof", contract)
		}
		if _, ok := reduction.ReduceAt(value.target, nil, value.cell, value.world.Mounted(), spec); ok {
			t.Fatalf("empty contract %d fabricated absence", contract)
		}
	}
}
