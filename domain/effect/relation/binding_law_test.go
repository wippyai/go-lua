package relation_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/operand"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
	"github.com/wippyai/go-lua/analysis/schema/rule/relbindgen/harness"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	calldomain "github.com/wippyai/go-lua/domain/call"
	"github.com/wippyai/go-lua/domain/effect/callsite"
	effectfactor "github.com/wippyai/go-lua/domain/effect/factor"
	"github.com/wippyai/go-lua/domain/effect/relation"
	"github.com/wippyai/go-lua/domain/relationfixture"
)

const reserve = 64

// TestBothCallSiteReadingsAnswerUnderOneContract states what the ABI means by
// a reading being a declaration and not a form: two derivations of one owner
// judgment answer the same sealed contract through the same generated shape,
// and nothing about the binding distinguishes them.
func TestBothCallSiteReadingsAnswerUnderOneContract(t *testing.T) {
	fixture := relationfixture.New(t)
	place := harness.New(t, "row/callsite")
	effectType := place.TypeID(t, "type/effect")
	callType := place.TypeID(t, "type/call")
	mountedType := place.TypeID(t, "type/effect-mounted-call")
	effectColumn := harness.NewColumn[effectfactor.Value](t, effectType, "store/effect", reserve)
	callColumn := harness.NewColumn[calldomain.Value](t, callType, "store/call", reserve)
	mountedColumn := harness.NewColumn[effectfactor.MountedCall](t, mountedType, "store/effect-mounted-call", reserve)
	mountedAddress := place.Column(t, "column/mounted")
	dispatchedAddress := place.Column(t, "column/dispatched")
	factAddress := place.Column(t, "column/fact")
	cardinality, ok := model.NewCardinality(model.ExactlyOne, 0)
	if !ok {
		t.Fatal("cardinality")
	}
	inputs := []signature.Input{
		harness.ScalarInput(t, place.Relation, mountedAddress, mountedType, place.Denominator),
		harness.ScalarInput(t, place.Relation, dispatchedAddress, callType, place.Denominator),
	}
	outputs := []signature.Output{{Relation: place.Relation, Column: factAddress, Type: effectType, Presence: signature.ProducePresent}}

	opaqueColumns, ok := relation.NewEffectOpaqueCallSiteColumns(callColumn, effectColumn, mountedColumn)
	if !ok {
		t.Fatal("opaque columns")
	}
	selectedColumns, ok := relation.NewEffectSelectedCallSiteColumns(callColumn, effectColumn, mountedColumn)
	if !ok {
		t.Fatal("selected columns")
	}
	opaqueOperation := place.Seal(t, "operation/effect-opaque", inputs, outputs, cardinality,
		outcome.Produced, outcome.NoCandidate, outcome.NoSelection, outcome.Opaque, outcome.Refused)
	selectedOperation := place.Seal(t, "operation/effect-selected", inputs, outputs, cardinality,
		outcome.Produced, outcome.NoCandidate, outcome.NoSelection, outcome.Opaque, outcome.Refused)

	opaqueJudgment, ok := relation.NewEffectOpaqueCallSiteOperation(fixture.Effects, fixture.Calls)
	if !ok {
		t.Fatal("opaque judgment")
	}
	selectedJudgment, ok := relation.NewEffectSelectedCallSiteOperation(fixture.Effects, fixture.Calls)
	if !ok {
		t.Fatal("selected judgment")
	}
	opaqueFactory, ok := relation.BindEffectOpaqueCallSite(opaqueOperation, opaqueJudgment, opaqueColumns, place.Refusal)
	if !ok {
		t.Fatal("bind opaque call site")
	}
	selectedFactory, ok := relation.BindEffectSelectedCallSite(selectedOperation, selectedJudgment, selectedColumns, place.Refusal)
	if !ok {
		t.Fatal("bind selected call site")
	}

	mountedToken, ok := mountedColumn.Encode(place.Issuer, effectfactor.MountedCall{})
	if !ok {
		t.Fatal("encode mounted call")
	}
	dispatchedToken, ok := callColumn.Encode(place.Issuer, fixture.Calls.Top())
	if !ok {
		t.Fatal("encode dispatched call")
	}
	frame := place.Frame(t,
		harness.ScalarSlot(t, place.Cell(t, mountedAddress, place.Rows[0], mountedType, mountedToken)),
		harness.ScalarSlot(t, place.Cell(t, dispatchedAddress, place.Rows[0], callType, dispatchedToken)),
	)

	for _, reading := range []struct {
		label     string
		operation signature.Signature
		factory   binding.Factory
	}{
		{"opaque", opaqueOperation, opaqueFactory},
		{"selected", selectedOperation, selectedFactory},
	} {
		worker := place.Worker(t, reading.factory, reading.operation)
		buffer := place.Buffer(t, reading.operation)
		result := worker.Evaluate(frame, buffer)
		batch, sealed := buffer.Seal(result)
		if !sealed || !reading.operation.Allows(result.Code) {
			t.Fatalf("the %s reading settled outside its own vocabulary: %v", reading.label, result.Code)
		}
		if result.Code == outcome.Produced {
			proposal, proposalOK := batch.At(0)
			if batch.Len() != 1 || !proposalOK || proposal.Destination().Row() != place.Rows[0] {
				t.Fatalf("the %s reading did not publish at the candidate's own row", reading.label)
			}
			continue
		}
		if batch.Len() != 0 {
			t.Fatalf("the %s reading produced nothing and published %d rows", reading.label, batch.Len())
		}
	}

	// Neither reading answers the other's contract, because a factory carries
	// the exact sealed signature it was constructed with.
	if _, admitted := binding.Admit(opaqueFactory, selectedOperation); admitted {
		t.Fatal("the opaque reading admitted the selected contract")
	}
	if _, admitted := binding.Admit(selectedFactory, opaqueOperation); admitted {
		t.Fatal("the selected reading admitted the opaque contract")
	}
}

// TestTheEffectAlgebraResolvesByTypeAlone states this axis's ascent authority
// is keyed by TypeID.
func TestTheEffectAlgebraResolvesByTypeAlone(t *testing.T) {
	fixture := relationfixture.New(t)
	place := harness.New(t, "row/effect")
	var types relation.PayloadTypes
	var tags relation.PayloadTags
	place.InstallTypes(t, &types)
	place.InstallTags(t, &tags)
	effectType := types.Effect
	payloads, ok := relation.NewPayloads(types, tags, reserve)
	if !ok {
		t.Fatal("install the effect columns")
	}
	witness, ok := relation.NewEffectLattice(fixture.Effects)
	if !ok {
		t.Fatal("effect lattice witness")
	}
	algebras, ok := payloads.Algebras(place.Issuer, relation.Lattices{Effect: witness})
	if !ok || len(algebras) != 1 || algebras[0].Type() != effectType {
		t.Fatal("the effect axis did not state one ascent authority for its own TypeID")
	}
	bottom, ok := payloads.Effect.Encode(place.Issuer, fixture.Effects.Bottom())
	if !ok {
		t.Fatal("encode effect bottom")
	}
	top, ok := payloads.Effect.Encode(place.Issuer, fixture.Effects.Top())
	if !ok {
		t.Fatal("encode effect top")
	}
	joined, ok := algebras[0].Join(bottom, top)
	if !ok || !algebras[0].LessOrEqual(bottom, joined) || !algebras[0].LessOrEqual(top, joined) {
		t.Fatal("the effect join was not an upper bound of both operands")
	}
}

// TestTheEffectBoundaryDoesNotAllocate holds the generic boundary to zero
// allocations for this axis's own payload.
func TestTheEffectBoundaryDoesNotAllocate(t *testing.T) {
	fixture := relationfixture.New(t)
	place := harness.New(t, "row/effect")
	column := harness.NewColumn[effectfactor.Value](t, place.TypeID(t, "type/effect"), "store/effect", 1<<20)
	top := fixture.Effects.Top()
	if allocations := testing.AllocsPerRun(200, func() {
		token, ok := column.Encode(place.Issuer, top)
		if !ok {
			t.Fatal("encode effect value")
		}
		if _, ok := column.Decode(token); !ok {
			t.Fatal("decode effect value")
		}
	}); allocations != 0 {
		t.Fatalf("the effect boundary allocated %.0f times", allocations)
	}
}

// TestTheBodyFoldsAnswerDoesNotDependOnTheTagsItsCellsCarry is the
// independence law.
//
// The binding materializes the delivered span into the operand vocabulary the
// fold reads, and a materialized cell carries a tag. For a fold that keys its
// cells by an owner tag, that tag has to be the owner's own resolution. This
// one reads presence and value and never a tag, and the way to state that is
// to ask the same question twice under two numberings and require one answer,
// rather than to pick a numbering and assert it does not matter.
//
// It is asked where the fold actually folds: over the call site the calling
// fixture's own program seals, across cell groups that reach its concrete arm
// as well as ones that do not. A group that answers nothing would compare two
// values neither algebra owns, which is what an earlier form of this law got
// wrong before it was asked at a real call.
func TestTheBodyFoldsAnswerDoesNotDependOnTheTagsItsCellsCarry(t *testing.T) {
	fixture := relationfixture.NewCalling(t)
	judgment, ok := callsite.DeriveBody(fixture.Effects, fixture.Calls)
	if !ok {
		t.Fatal("body call-site judgment")
	}
	if fixture.Effects.MountedCallCount() == 0 {
		t.Fatal("the calling fixture sealed no mounted call")
	}

	groups := [][]effectfactor.Value{
		{fixture.Effects.Bottom()},
		{fixture.Effects.Top()},
		{fixture.Effects.Bottom(), fixture.Effects.Top(), fixture.Effects.Bottom()},
		{fixture.Effects.Top(), fixture.Effects.Bottom()},
	}

	concrete := 0
	for groupIndex, values := range groups {
		ascending := make([]operand.SelectedCell[effectfactor.Value], 0, len(values))
		descending := make([]operand.SelectedCell[effectfactor.Value], 0, len(values))
		for index, value := range values {
			ascending = append(ascending, operand.SelectedCell[effectfactor.Value]{Value: value, Present: true, Tag: uint64(index) + 1})
			descending = append(descending, operand.SelectedCell[effectfactor.Value]{Value: value, Present: true, Tag: uint64(len(values) - index)})
		}
		for ordinal := 0; ordinal < fixture.Effects.MountedCallCount(); ordinal++ {
			mounted, mountedOK := fixture.Effects.MountedCallAt(ordinal)
			if !mountedOK {
				t.Fatalf("mounted call %d is not issued", ordinal)
			}
			first, firstOutcome := judgment.BodyEffect(mounted, ascending)
			second, secondOutcome := judgment.BodyEffect(mounted, descending)
			if firstOutcome != secondOutcome {
				t.Fatalf("group %d at call %d settled as %v under one numbering and %v under another", groupIndex, ordinal, firstOutcome, secondOutcome)
			}
			if firstOutcome != structure.Concrete {
				continue
			}
			concrete++
			if !fixture.Effects.Equal(first, second) {
				t.Fatalf("group %d at call %d answered differently under two numberings of the same cells, so its cells are keyed by their tags after all", groupIndex, ordinal)
			}
		}
	}
	if concrete == 0 {
		t.Fatal("no cell group reached the fold's concrete arm, so the independence this law states was never exercised")
	}
	t.Logf("concrete answers compared under two numberings: %d", concrete)
}
