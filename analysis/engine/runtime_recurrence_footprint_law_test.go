package engine

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
)

// carryClosureOnlyMember publishes its carry closure as the only carry surface
// and therefore exercises the binder contract rather than the nesting one
// concrete draft happens to have. runtimeMember permits it: carryTargets is
// the closure a carry reaches, not a superset of the member's own writes.
type carryClosureOnlyMember struct {
	runtimeMember
	closure []carrier.Target
}

func (member carryClosureOnlyMember) carryTargets() []carrier.Target { return member.closure }

// carryFootprintFixture is one bound carrying member: a source Rule writing
// the Factor's first coordinate and a carry Rule that carries that predecessor
// and writes the Factor's second coordinate. It is the smallest program in
// which a member's own write surface and its carry closure are distinct.
type carryFootprintFixture struct {
	carrying runtimeMember
	member   equation.RuleMember
}

func newCarryFootprintFixture(t *testing.T) carryFootprintFixture {
	t.Helper()
	builder := NewSchema()
	factor, factorOK := DeclareFactorSlot[uint64](builder, coldKey(947_600))
	writeForm, writeFormOK := factor.ExactWrite()
	source, sourceOK := DeclareRuleSlot[uint64, ruleUnit](builder, SchemaRuleSpec[uint64]{
		Semantic: coldKey(947_601), OperandFamily: unitOperandFamily, Inputs: 0,
		Admission: SchemaAdmission{Basis: RuleAdmissionBasisTrustedTheorem, Identity: coldKey(947_602)}, Output: factor.Ref(),
	})
	sourceWrite, sourceWriteOK := SchemaWrite(source, writeForm)
	carryRule, carryRuleOK := DeclareRuleSlot[uint64, ruleUnit](builder, SchemaRuleSpec[uint64]{
		Semantic: coldKey(947_603), OperandFamily: unitOperandFamily, Inputs: 1,
		Admission: SchemaAdmission{Basis: RuleAdmissionBasisTrustedTheorem, Identity: coldKey(947_604)}, Output: factor.Ref(),
	})
	carryInput, carryInputOK := carryRule.Input(0)
	carrySlot, carrySlotOK := SchemaCarryFrom(carryRule, carryInput, factor.Ref())
	carryWrite, carryWriteOK := SchemaWrite(carryRule, writeForm)
	schema, schemaOK := builder.Seal()
	if !factorOK || !writeFormOK || !sourceOK || !sourceWriteOK || !carryRuleOK || !carryInputOK || !carrySlotOK || !carryWriteOK || !schemaOK {
		t.Fatal("carry footprint schema")
	}
	binding := NewSchemaBinding(schema)
	sourceHot := HotRuleSpec[uint64, ruleUnit]{
		OperandContent: ruleUnitContent,
		Admission:      AdmitRuleByTrustedTheorem[uint64, ruleUnit](coldKey(947_602)),
		Transfer: func(access Access[uint64, ruleUnit]) bool {
			return Product(access, func(row Row) bool { return StageValue(access, row, uint64(3)) })
		},
	}
	carryHot := HotRuleSpec[uint64, ruleUnit]{
		OperandContent: ruleUnitContent,
		Admission:      AdmitRuleByTrustedTheorem[uint64, ruleUnit](coldKey(947_604)),
		Transfer: func(access Access[uint64, ruleUnit]) bool {
			return Product(access, func(row Row) bool { return StageValue(access, row, uint64(9)) })
		},
	}
	if !BindFactor(binding, factor, hotUintFactorSpec()) ||
		!BindRule[uint64, uint64, ruleUnit](binding, source, sourceWrite, factor, sourceHot, func(ruleUnit) (uint64, bool) { return 1, true }) ||
		!BindRuleWithCarry[uint64, uint64, ruleUnit](binding, carryRule, carrySlot, carryWrite, factor, carryHot, HotCarrySpec[uint64, ruleUnit]{}, func(ruleUnit) (uint64, bool) { return 2, true }) ||
		!binding.Seal() {
		t.Fatal("carry footprint binding")
	}
	sourceImplementation, sourceImplementationOK := RuleImplementationAt[uint64, uint64, ruleUnit](binding, source)
	carryImplementation, carryImplementationOK := RuleImplementationAt[uint64, uint64, ruleUnit](binding, carryRule)
	if !sourceImplementationOK || !carryImplementationOK || carryImplementation.receipt.proof.carries != 1 {
		t.Fatal("carry footprint implementations")
	}
	sourceOperand := ruleUnitForSemantic(coldKey(947_605))
	carryOperand := ruleUnitForSemantic(coldKey(947_606))
	batch := equation.NewBatch()
	scope := equation.EmptyScope()
	sourceSite, sourceSiteOK := batch.AdmitSite(compositionKeyOf(coldKey(947_607)), scope, equation.TrueExpr(), equation.InitPresent)
	carrySite, carrySiteOK := batch.AdmitSite(compositionKeyOf(coldKey(947_608)), scope, equation.TrueExpr(), equation.InitPresent)
	sourceOccurrence, sourceOccurrenceOK := batch.At(sourceSite)
	carryOccurrence, carryOccurrenceOK := batch.At(carrySite)
	sourceEntity, sourceEntityOK := operandEntityForContent(sourceOperand.content)
	carryEntity, carryEntityOK := operandEntityForContent(carryOperand.content)
	sourceOperandRow, sourceOperandOK := batch.AdmitOperand(sourceOccurrence, sourceEntity)
	carryOperandRow, carryOperandOK := batch.AdmitOperand(carryOccurrence, carryEntity)
	if !sourceSiteOK || !carrySiteOK || !sourceOccurrenceOK || !carryOccurrenceOK || !sourceEntityOK || !carryEntityOK || !sourceOperandOK || !carryOperandOK || !batch.Seal() {
		t.Fatal("carry footprint batch")
	}
	boundary := equation.BoundaryInput(sourceSite, carrySite, compositionKeyOf(coldKey(947_609)), equation.TrueExpr(), equation.IdentityReindex(scope), equation.TrueExpr())
	topology, topologyOK := equation.SealTopology(schema.cold, equation.TopologySpec{Batch: batch, Rules: []equation.RuleInstance{
		{Schema: sourceImplementation.receipt.proof.semantic, OperandFamily: compositionKeyOf(unitOperandFamily), Occurrence: sourceOccurrence, Operand: sourceOperandRow, Writes: []equation.ResolvedWrite{{Index: 0, Surface: equation.Surface{Factor: sourceImplementation.receipt.output.semantic, Form: equation.SurfaceWriteExact, Local: 1, Mode: equation.TargetModeStrong}}}},
		{Schema: carryImplementation.receipt.proof.semantic, OperandFamily: compositionKeyOf(unitOperandFamily), Occurrence: carryOccurrence, Operand: carryOperandRow, Carries: []equation.ResolvedCarry{{Index: 0}}, Writes: []equation.ResolvedWrite{{Index: 0, Surface: equation.Surface{Factor: carryImplementation.receipt.output.semantic, Form: equation.SurfaceWriteExact, Local: 2, Mode: equation.TargetModeStrong}}}},
	}, Points: []equation.PointSpec{{Site: sourceSite}, {Site: carrySite}}, Groups: []equation.Group{
		{Members: []equation.RuleRef{equation.RuleAt(0)}, Output: equation.PointAt(0)},
		{Members: []equation.RuleRef{equation.RuleAt(1)}, Output: equation.PointAt(1), Inputs: []equation.Input{boundary}},
	}})
	if !topologyOK || topology == nil {
		t.Fatal("carry footprint topology")
	}
	graph, graphOK := initialEquationGraph(topology)
	if !graphOK || graph == nil {
		t.Fatal("carry footprint graph")
	}
	var sourceMember, carryMember equation.RuleMember
	for groupIndex := 0; groupIndex < graph.GroupCount(); groupIndex++ {
		group, groupOK := graph.HyperedgeAt(groupIndex)
		if !groupOK {
			t.Fatal("carry footprint group")
		}
		for memberIndex := 0; memberIndex < group.MemberCount(); memberIndex++ {
			member, memberOK := group.MemberAt(memberIndex)
			if !memberOK {
				t.Fatal("carry footprint member")
			}
			switch member.Rule() {
			case sourceImplementation.receipt.proof.semantic:
				sourceMember = member
			case carryImplementation.receipt.proof.semantic:
				carryMember = member
			}
		}
	}
	compilation, compiled := beginProgramConstruction(binding, graph)
	_, sourceRowOK := attachProgramRuleMember(compilation, sourceImplementation, sourceMember, sourceOperand)
	carryRow, carryRowOK := attachProgramRuleMember(compilation, carryImplementation, carryMember, carryOperand)
	if !compiled || !sourceRowOK || !carryRowOK {
		t.Fatal("carry footprint members")
	}
	if len(carryRow.carries()) == 0 || len(carryRow.targets()) == 0 {
		t.Fatalf("carry footprint member carries %d inputs and writes %d targets", len(carryRow.carries()), len(carryRow.targets()))
	}
	return carryFootprintFixture{carrying: carryRow, member: carryMember}
}

// footprintTargetsFor returns the sealed occurrence targets the binder minted
// for one Factor.
func footprintTargetsFor(fold memberFold, factor equation.RuleMember, draft runtimeMember) ([]carrier.Target, bool) {
	key, keyed := draft.factorKey()
	if !keyed {
		return nil, false
	}
	for _, occurrence := range fold.footprint {
		if occurrence.key == key {
			return occurrence.targets, true
		}
	}
	return nil, false
}

// TestOccurrenceFootprintCoversEveryExactWriteTarget is the binder's recurrence
// footprint law. The footprint is the only input the active Region seals its
// widening scope from, so every exact write target a member publishes has to
// be inside it. A carrying member is the case that decides the law: its carry
// closure and its own write surface are two distinct member answers, and the
// footprint owes the union of both.
func TestOccurrenceFootprintCoversEveryExactWriteTarget(t *testing.T) {
	fixture := newCarryFootprintFixture(t)
	fold, folded := foldMemberDrafts(1, []runtimeMember{fixture.carrying})
	if !folded {
		t.Fatal("binder rejected the carrying member group")
	}
	sealed, present := footprintTargetsFor(fold, fixture.member, fixture.carrying)
	if !present {
		t.Fatal("binder minted no occurrence footprint for the carrying member's Factor")
	}
	for _, target := range fixture.carrying.targets() {
		if !runtimeContainsTarget(sealed, target) {
			t.Fatalf("occurrence footprint omits an exact write target the member publishes: footprint=%d targets, member writes %d", len(sealed), len(fixture.carrying.targets()))
		}
	}
	for _, target := range fixture.carrying.carryTargets() {
		if !runtimeContainsTarget(sealed, target) {
			t.Fatal("occurrence footprint omits a target the member's carry closure reaches")
		}
	}
}

// TestOccurrenceFootprintCoversWritesOfACarryClosureOnlyMember drives the same
// law through a member whose carry closure does not happen to nest its own
// write surface. The binder folds runtimeMember, so the law it owes is stated
// over that contract; a draft that answers the two questions independently
// must still have every write it publishes inside the sealed footprint.
func TestOccurrenceFootprintCoversWritesOfACarryClosureOnlyMember(t *testing.T) {
	fixture := newCarryFootprintFixture(t)
	closure := make([]carrier.Target, 0, len(fixture.carrying.carryTargets()))
	for _, target := range fixture.carrying.carryTargets() {
		if !runtimeContainsTarget(fixture.carrying.targets(), target) {
			closure = append(closure, target)
		}
	}
	if len(closure) == 0 {
		t.Fatal("carry fixture reaches no target outside the member's own write surface")
	}
	draft := carryClosureOnlyMember{runtimeMember: fixture.carrying, closure: closure}
	fold, folded := foldMemberDrafts(1, []runtimeMember{draft})
	if !folded {
		t.Fatal("binder rejected the carry-closure-only member group")
	}
	sealed, present := footprintTargetsFor(fold, fixture.member, draft)
	if !present {
		t.Fatal("binder minted no occurrence footprint for the carry-closure-only member")
	}
	for _, target := range draft.targets() {
		if !runtimeContainsTarget(sealed, target) {
			t.Fatalf("occurrence footprint omits an exact write target: the footprint carries the carry closure (%d targets) in place of the union with the member's %d writes", len(sealed), len(draft.targets()))
		}
	}
	for _, target := range closure {
		if !runtimeContainsTarget(sealed, target) {
			t.Fatal("occurrence footprint omits a target the carry closure reaches")
		}
	}
}
