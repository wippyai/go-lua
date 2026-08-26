package binding_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
)

func TestJoinedCompleteFrameSeparatesCellAndRangeAuthorities(t *testing.T) {
	exact, _ := model.NewCardinality(model.ExactlyOne, 0)
	value := newFixture(t, exact)

	globalRelation := issueRelation(t, value.dataOwner, "relation/global-owner")
	globalColumn := issueColumn(t, globalRelation, "column/global-owner")
	globalKey := issueKey(t, globalRelation, "key/global-owner")
	globalDenominator, ok := model.NewDenominatorRef(globalRelation, globalKey)
	if !ok {
		t.Fatal("global denominator")
	}
	globalRow := mustRow(t, globalRelation, "row/global-owner")
	globalMembership, ok := binding.NewMembershipView(globalRelation, []model.RowID{globalRow})
	if !ok {
		t.Fatal("global membership")
	}
	globalWitness, ok := value.issuer.IssueDenominator(globalDenominator, globalMembership, content(t, "witness/global-owner"))
	if !ok {
		t.Fatal("global witness")
	}

	carrierRow := mustRow(t, value.relation, "row/carrier-second")
	carrierMembership, ok := binding.NewMembershipView(value.relation, []model.RowID{value.inputRow, carrierRow})
	if !ok {
		t.Fatal("carrier membership")
	}
	carrierWitness, ok := value.issuer.IssueDenominator(value.denominator, carrierMembership, content(t, "witness/carrier"))
	if !ok {
		t.Fatal("carrier witness")
	}
	complete, ok := signature.NewCompleteSpanDelivery(value.denominator.Key())
	if !ok {
		t.Fatal("complete delivery")
	}
	joinedInput, ok := signature.NewJoinedInput(globalRelation, globalColumn, value.inputType, signature.RequirePresent, complete, globalDenominator, value.denominator)
	if !ok {
		t.Fatal("joined input")
	}
	operation, ok := signature.Seal(signature.Spec{
		Identity: value.signature.Identity(), Fence: value.logical,
		Inputs:      []signature.Input{joinedInput},
		Outputs:     value.signature.Outputs(),
		Cardinality: value.signature.Cardinality(), Outcomes: outcomesFor(),
	})
	if !ok {
		t.Fatal("joined signature")
	}

	address, ok := value.issuer.IssueCell(globalWitness, value.inputScope, globalColumn, globalRow)
	if !ok {
		t.Fatal("global source address")
	}
	encoded, ok := value.issuer.IssueValue(value.inputType, content(t, "value/global-owner"))
	if !ok {
		t.Fatal("global source value")
	}
	cell, ok := binding.NewCell(address, value.inputType, encoded, mustPresence(t, model.Present))
	if !ok {
		t.Fatal("global source cell")
	}

	// The same global owner cell is delivered across two QAllocation carrier
	// rows.  It is valid only because the slot preserves those range anchors
	// independently of Cell.Address().Row().
	slot, ok := binding.NewJoinedSpanSlot([]binding.Cell{cell, cell}, []model.RowID{value.inputRow, carrierRow}, carrierWitness)
	if !ok || slot.RangeWitness().Same(globalWitness) {
		t.Fatalf("joined slot=%#v/%t", slot, ok)
	}
	if first, firstOK := slot.RangeRowAt(0); !firstOK || first != value.inputRow {
		t.Fatalf("first carrier row=%#v/%t", first, firstOK)
	}
	frame, ok := binding.NewFrame(value.inputScope, slot)
	if !ok || !frame.Validate(operation, value.runtime) {
		t.Fatal("dual-authority complete frame rejected")
	}

	// Order/completeness belongs to the carrier witness, not to the repeated
	// global source row.  Reversing only carrier anchors refuses the frame.
	reversed, ok := binding.NewJoinedSpanSlot([]binding.Cell{cell, cell}, []model.RowID{carrierRow, value.inputRow}, carrierWitness)
	if !ok {
		t.Fatal("reversed joined slot construction")
	}
	reversedFrame, _ := binding.NewFrame(value.inputScope, reversed)
	if reversedFrame.Validate(operation, value.runtime) {
		t.Fatal("carrier order was inferred from source cells")
	}

	// The historical homogeneous constructor carries each source row as the
	// range row and therefore cannot silently encode this joined delivery.
	homogeneousSlot, ok := binding.NewSpanSlot([]binding.Cell{cell, cell})
	if !ok {
		t.Fatal("homogeneous source slot")
	}
	homogeneousFrame, _ := binding.NewFrame(value.inputScope, homogeneousSlot)
	if homogeneousFrame.Validate(operation, value.runtime) {
		t.Fatal("homogeneous slot silently crossed carrier authority")
	}

	wrongKey := issueKey(t, globalRelation, "key/global-owner-wrong")
	wrongDenominator, _ := model.NewDenominatorRef(globalRelation, wrongKey)
	wrongWitness, _ := value.issuer.IssueDenominator(wrongDenominator, globalMembership, content(t, "witness/global-owner-wrong"))
	wrongAddress, _ := value.issuer.IssueCell(wrongWitness, value.inputScope, globalColumn, globalRow)
	wrongCell, _ := binding.NewCell(wrongAddress, value.inputType, encoded, mustPresence(t, model.Present))
	wrongSlot, _ := binding.NewJoinedSpanSlot([]binding.Cell{wrongCell, wrongCell}, []model.RowID{value.inputRow, carrierRow}, carrierWitness)
	wrongFrame, _ := binding.NewFrame(value.inputScope, wrongSlot)
	if wrongFrame.Validate(operation, value.runtime) {
		t.Fatal("source cell authenticated by an undeclared source witness")
	}
}
