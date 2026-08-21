package engine

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
)

func TestSchemaRuleSealFailureDoesNotFinalizeDraftCell(t *testing.T) {
	binding, _, _, _, _, _ := bindDirectSelectedRuleLaw(t)
	cell, ok := binding.state.rules[0].(*schemaRuleBindingCellImpl[uint64, uint64, struct{}])
	if !ok || cell == nil || cell.impl == nil || cell.impl.rule == nil || cell.impl.write.cell == nil || cell.impl.carry == nil {
		t.Fatal("direct Rule draft fixture")
	}

	if binding.Seal() || !binding.Poisoned() {
		t.Fatal("incomplete Rule Seal did not fail atomically")
	}
	if cell.impl.rule == nil || cell.impl.write.cell == nil || cell.impl.carry == nil {
		t.Fatal("failed Seal partially finalized the Rule draft")
	}
	if cell.impl.writeMode != 0 || cell.impl.routeRead != 0 || cell.impl.carryPresent || cell.impl.ruleSemantic.Available() || cell.impl.operandFamily.Available() {
		t.Fatal("failed Seal published direct Rule geometry")
	}
}

func TestSchemaRuleSealPublishesDirectGeometryAndDropsDraftHandles(t *testing.T) {
	_, implementation, _, _, _, _ := sealedLawRule(t, 0)
	hot := implementation.cell.impl
	if hot == nil || hot.rule != nil || hot.write.cell != nil || hot.carry != nil {
		t.Fatal("sealed exact Rule retained construction handles")
	}
	if hot.writeMode != directRuleWriteExact || hot.routeRead != 0 || hot.carryPresent || !hot.ruleSemantic.Available() || !hot.operandFamily.Available() || hot.operandContent == nil || hot.fold == nil || hot.projectWrite == nil {
		t.Fatal("sealed exact Rule direct geometry")
	}
	if _, ok := implementation.sealedRuleCell(); !ok || hot.writeMode != directRuleWriteExact || hot.routeRead != 0 || implementation.cell.schemaRuleReadAt(0) != nil {
		t.Fatal("sealed exact Rule direct fields")
	}
}

func TestSchemaRuleSealPublishesRouteAndCarryGeometry(t *testing.T) {
	schema, factor, rule, exact, static, operand, carry, write := directRouteRuleLawSchema(t)
	binding := NewSchemaBinding(schema)
	if !BindFactor(binding, factor, hotUintFactorSpec()) ||
		!BindSelectedRouteRuleDirect[uint64](binding, rule, carry, write, factor.Ref(), directRouteRuleHotLaw(), HotCarrySpec[uint64, struct{}]{}, func(struct{}) (uint64, bool) { return 0, true }) {
		t.Fatal("route Rule draft")
	}
	if _, ok := BindSelectedRuleDirectExactRead[uint64, uint64, struct{}, uint64](binding, rule, exact, factor.Ref(), func(struct{}) (uint64, bool) { return 0, true }); !ok {
		t.Fatal("route exact read")
	}
	if _, ok := BindSelectedRuleDirectSelectedRead[uint64, uint64, struct{}, uint64, uint64](binding, rule, static, factor.Ref(), func(SelectorContext) bool { return true }); !ok {
		t.Fatal("route static read")
	}
	if _, ok := BindSelectedRuleDirectOperandRead[uint64, uint64, struct{}, uint64, uint64](binding, rule, operand, factor.Ref(), func(SelectorContext, struct{}) bool { return true }); !ok {
		t.Fatal("route operand read")
	}
	if !binding.Seal() {
		t.Fatal("route Rule Seal")
	}
	implementation, ok := RuleImplementationAt[uint64, uint64, struct{}](binding, rule)
	if !ok || implementation == nil || implementation.cell == nil || implementation.cell.impl == nil {
		t.Fatal("route Rule implementation")
	}
	hot := implementation.cell.impl
	if hot.rule != nil || hot.write.cell != nil || hot.carry != nil || hot.writeMode != directRuleWriteRoute || hot.routeRead != 2 || !hot.carryPresent || hot.carryInput != 0 || hot.carryTransform.Available() || hot.carryApply != nil {
		t.Fatal("sealed route/carry geometry")
	}
	row := implementation.cell.schemaRuleReadAt(hot.routeRead - 1)
	if hot.writeMode != directRuleWriteRoute || hot.routeRead != 2 || row == nil || row.kind != composition.ReadSelect || row.owner != implementation.cell || row.ownerOrdinal != implementation.ordinal || row.readOrdinal != 1 || len(row.dependencies) == 0 {
		t.Fatal("sealed route direct fields")
	}
}

func TestSchemaRuleSealLeavesActivationLaneUntouched(t *testing.T) {
	builder := NewSchema()
	factor, factorOK := DeclareFactorSlot[uint64](builder, coldKey(998_101))
	family, familyOK := DeclareSchemaActivationFamily(builder, coldKey(998_102))
	rule, ruleOK := DeclareSchemaActivationRule(builder, SchemaStructuralRuleSpec{Semantic: coldKey(998_103), Activation: family})
	schema, schemaOK := builder.Seal()
	if !factorOK || !familyOK || !ruleOK || !schemaOK || schema == nil {
		t.Fatal("activation schema")
	}
	binding := NewSchemaBinding(schema)
	if !BindFactor(binding, factor, hotUintFactorSpec()) || !BindActivationRule(binding, rule, HotActivationSpec{Fold: func(frame ActivationFrame) ActivationResult { return Activated(frame) }}) || !binding.Seal() {
		t.Fatal("activation Seal")
	}
	cell, ok := binding.state.rules[0].(*schemaActivationRuleBindingCell)
	if !ok || cell == nil || cell.impl == nil || cell.impl.rule == nil || cell.impl.fold == nil || !cell.schemaRuleComplete() {
		t.Fatal("activation lane changed by ordinary finalization")
	}
}

func TestSchemaRuleInputIntBoundRejectsOverflow(t *testing.T) {
	maxInt := uint64(^uint(0) >> 1)
	if !schemaRuleInputFitsInt(0) || !schemaRuleInputFitsInt(maxInt) || schemaRuleInputFitsInt(maxInt+1) {
		t.Fatal("Rule input int bound")
	}
}

// oversizedRuleInputLawSchema preserves the issued handles from the direct
// Rule fixture while replacing its cold composition with a valid persisted
// shape whose read and carry name an input past the host int limit. Public
// declarations cap their input count at schemaSlotMax, so this is the narrow
// way to exercise receipt admission against an actually oversized uint64.
func oversizedRuleInputLawSchema(t testing.TB, withRead bool) (*Schema, *FactorSlot[uint64], *RuleSlot[uint64, struct{}], SchemaReadSlot[uint64], SchemaCarrySlot[uint64], SchemaWriteSlot[uint64]) {
	t.Helper()
	schema, factor, _, rule, read, _, carry, write := directSelectedRuleLawSchema(t)
	overflow := uint64(^uint(0)>>1) + 1
	shape, shapeOK := schema.ruleShapeAt(0)
	factorKey := schema.factorSemanticAt(0)
	ruleKey := schema.ruleSemanticAt(0)
	if !shapeOK || !factorKey.Available() || !ruleKey.Available() || !shape.OperandFamily.Available() {
		t.Fatal("oversized Rule fixture keys")
	}
	row := composition.Rule{
		Key: ruleKey, OperandFamily: shape.OperandFamily,
		OutputKind: composition.FactorOutput, Output: factorKey, Inputs: overflow + 1,
		Carries: []composition.Carry{{Input: overflow, Factor: factorKey}},
		Writes:  []composition.Write{{Kind: composition.WriteExact, Factor: factorKey}},
	}
	if withRead {
		row.Reads = []composition.Read{{Kind: composition.ReadExact, Input: overflow, Factor: factorKey}}
	}
	cold, coldOK := composition.Seal(composition.Candidate{
		Factors: []composition.Factor{{Key: factorKey}},
		Rules:   []composition.Rule{row},
	})
	if !coldOK || cold == nil {
		t.Fatal("oversized Rule cold shape")
	}
	var digest [32]byte
	coldID := cold.ID()
	copy(digest[:], coldID[:])
	schema.cold, schema.id = cold, CompositionID{digest: digest}
	return schema, factor, rule, read, carry, write
}

func TestSchemaRuleReadBindingRejectsOversizedInputBeforeIntConversion(t *testing.T) {
	schema, factor, rule, read, carry, write := oversizedRuleInputLawSchema(t, true)
	overflow := uint64(^uint(0)>>1) + 1
	readShape, readOK := schema.ruleReadShapeAt(0, 0)
	if !readOK || readShape.Input != overflow {
		t.Fatal("oversized Rule read shape")
	}
	binding := NewSchemaBinding(schema)
	if !BindFactor(binding, factor, hotUintFactorSpec()) {
		t.Fatal("oversized Rule Factor binding")
	}
	if _, ok := BindRuleWithExactReadAndCarry[uint64, uint64, struct{}, uint64, uint64](binding, rule, read, factor, carry, write, factor, lawHotRuleSpec(), HotCarrySpec[uint64, struct{}]{}, testRuleProjector[struct{}], testRuleProjector[struct{}]); ok || !binding.Poisoned() {
		t.Fatal("oversized Rule read crossed into the int-indexed binding lane")
	}
}

func TestSchemaRuleCarrySealRejectsOversizedInputBeforeIntConversion(t *testing.T) {
	schema, factor, rule, _, carry, write := oversizedRuleInputLawSchema(t, false)
	overflow := uint64(^uint(0)>>1) + 1
	carryShape, carryOK := schema.ruleCarryShapeAt(0, 0)
	if !carryOK || carryShape.Input != overflow {
		t.Fatal("oversized Rule carry shape")
	}
	binding := NewSchemaBinding(schema)
	if !BindFactor(binding, factor, hotUintFactorSpec()) ||
		!BindSelectedRuleDirect[uint64](binding, rule, carry, write, factor.Ref(), directSelectedRuleHotLaw(), HotCarrySpec[uint64, struct{}]{}, testRuleProjector[struct{}]) {
		t.Fatal("oversized Rule carry binding")
	}
	if binding.Seal() || !binding.Poisoned() {
		t.Fatal("oversized Rule carry crossed into the int-indexed Seal lane")
	}
}

type unfinalizedRuleCellLaw struct {
	state   *schemaBindingState
	schema  *Schema
	ordinal uint64
}

func (cell *unfinalizedRuleCellLaw) schemaBindingSchema() *Schema {
	if cell == nil {
		return nil
	}
	return cell.schema
}

func (cell *unfinalizedRuleCellLaw) schemaRuleOrdinal() uint64 {
	if cell == nil {
		return 0
	}
	return cell.ordinal
}

func (cell *unfinalizedRuleCellLaw) schemaRuleBindingState() *schemaBindingState {
	if cell == nil {
		return nil
	}
	return cell.state
}

func (*unfinalizedRuleCellLaw) schemaRuleReadAt(uint64) *schemaRuleReadRow { return nil }
func (*unfinalizedRuleCellLaw) schemaRuleComplete() bool                   { return true }

func TestSchemaRuleSealRejectsUnfinalizedOrdinaryLane(t *testing.T) {
	schema, factor, rule, write := zeroWriteRuleLawSchema(t, 0)
	binding := NewSchemaBinding(schema)
	if !BindFactor(binding, factor, hotUintFactorSpec()) || !BindRule[uint64, uint64, struct{}](binding, rule, write, factor, lawHotRuleSpec(), testRuleProjector[struct{}]) {
		t.Fatal("ordinary Rule draft")
	}
	binding.state.rules[0] = &unfinalizedRuleCellLaw{state: binding.state, schema: schema, ordinal: 0}
	if binding.Seal() || !binding.Poisoned() {
		t.Fatal("unfinalized ordinary Rule was published")
	}
}
