package engine

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
)

// schemaRuleMemberGeometryLaw is the small cold-row view consumed by the
// current member binder.  It deliberately carries no declaration or binding
// handle: geometry is checked against the sealed Rule proof only.
type schemaRuleMemberGeometryLaw struct {
	rule, family  composition.Key
	reads, writes int
	dynamic       bool
	surface       equation.Surface
	route         uint64
}

func (row schemaRuleMemberGeometryLaw) Rule() composition.Key          { return row.rule }
func (row schemaRuleMemberGeometryLaw) OperandFamily() composition.Key { return row.family }
func (row schemaRuleMemberGeometryLaw) ReadCount() int                 { return row.reads }
func (row schemaRuleMemberGeometryLaw) WriteCount() int                { return row.writes }
func (row schemaRuleMemberGeometryLaw) ActivationMember() (equation.Member, bool) {
	return equation.Member{}, row.dynamic
}
func (row schemaRuleMemberGeometryLaw) WriteAt(index int) (equation.Surface, bool) {
	return row.surface, index == 0
}
func (row schemaRuleMemberGeometryLaw) WriteRouteRead(index int) (uint64, bool) {
	return row.route, index == 0
}

func zeroWriteRuleLawSchema(t testing.TB, inputs uint64) (*Schema, *FactorSlot[uint64], *RuleSlot[uint64, struct{}], SchemaWriteSlot[uint64]) {
	t.Helper()
	builder := NewSchema()
	factor, factorOK := DeclareFactorSlot[uint64](builder, coldKey(997_001))
	form, formOK := factor.ExactWrite()
	rule, ruleOK := DeclareRuleSlot[uint64, struct{}](builder, SchemaRuleSpec[uint64]{
		Semantic: coldKey(997_002), OperandFamily: coldKey(997_003), Inputs: inputs,
		Admission: SchemaAdmission{Basis: RuleAdmissionBasisTrustedTheorem, Identity: coldKey(997_004)},
		Output:    factor.Ref(),
	})
	write, writeOK := SchemaWrite(rule, form)
	schema, sealOK := builder.Seal()
	if !factorOK || !formOK || !ruleOK || !writeOK || !sealOK || schema == nil {
		t.Fatal("Rule schema")
	}
	return schema, factor, rule, write
}

func lawHotRuleSpec() HotRuleSpec[uint64, struct{}] {
	return HotRuleSpec[uint64, struct{}]{
		OperandContent: func(value struct{}) (struct{}, [32]byte, bool) {
			return value, [32]byte{0x5a}, true
		},
		Admission: AdmitRuleByTrustedTheorem[uint64, struct{}](coldKey(997_004)),
		Transfer:  func(Access[uint64, struct{}]) bool { return true },
	}
}

func sealedLawRule(t testing.TB, inputs uint64) (*SchemaBinding, *RuleImplementation[uint64, uint64, struct{}], *Schema, *FactorSlot[uint64], *RuleSlot[uint64, struct{}], SchemaWriteSlot[uint64]) {
	t.Helper()
	schema, factor, rule, write := zeroWriteRuleLawSchema(t, inputs)
	binding := NewSchemaBinding(schema)
	if !BindFactor(binding, factor, hotUintFactorSpec()) || !BindRule[uint64, uint64, struct{}](binding, rule, write, factor, lawHotRuleSpec(), testRuleProjector[struct{}]) || !binding.Seal() {
		t.Fatal("Rule binding")
	}
	implementation, ok := RuleImplementationAt[uint64, uint64, struct{}](binding, rule)
	if !ok || implementation == nil || !implementation.binding.valid() {
		t.Fatal("sealed Rule implementation")
	}
	return binding, implementation, schema, factor, rule, write
}

func TestSchemaRuleBindingStoresPerSlotWriteProjector(t *testing.T) {
	schema, factor, rule, write := zeroWriteRuleLawSchema(t, 0)
	binding := NewSchemaBinding(schema)
	if !BindFactor(binding, factor, hotUintFactorSpec()) || !BindRule[uint64, uint64, struct{}](binding, rule, write, factor, lawHotRuleSpec(), func(struct{}) (uint64, bool) { return 7, true }) || !binding.Seal() {
		t.Fatal("Rule write binding")
	}
	implementation, ok := RuleImplementationAt[uint64, uint64, struct{}](binding, rule)
	if !ok || implementation == nil || implementation.binding.cell == nil || implementation.binding.cell.impl == nil {
		t.Fatal("Rule write implementation")
	}
	local, projected := implementation.binding.cell.impl.projectWrite(struct{}{})
	if !projected || local != 7 {
		t.Fatalf("write projector=%d/%t", local, projected)
	}
}

func TestSchemaRuleBindingPublishesZeroInputExactWrite(t *testing.T) {
	_, implementation, _, _, _, _ := sealedLawRule(t, 0)
	proof := implementation.binding.proof
	if proof == nil || !proof.valid() || proof.outputKind != composition.FactorOutput || proof.reads != 0 || proof.writes != 1 || proof.carries != 0 {
		t.Fatal("zero-input exact Rule proof")
	}
}

func TestSchemaRuleBindingCapabilityKeepsOneAuthorityAcrossSeal(t *testing.T) {
	binding, _, _, _, rule, _ := sealedLawRule(t, 0)
	// Capabilities must be issued before Seal.  Use a second open owner to
	// exercise the complete handoff without weakening the sealed authority.
	openSchema, factor, openRule, write := zeroWriteRuleLawSchema(t, 0)
	open := NewSchemaBinding(openSchema)
	if !BindFactor(open, factor, hotUintFactorSpec()) || !BindRule[uint64, uint64, struct{}](open, openRule, write, factor, lawHotRuleSpec(), testRuleProjector[struct{}]) {
		t.Fatal("open Rule binding")
	}
	capability, issued := IssueMountedRuleCapability(open, openRule)
	if !issued || !RegisterRuleSlot(open, openRule, capability) || !open.Seal() {
		t.Fatal("Rule capability seal")
	}
	sealed, sealedOK := MountedCapabilityForSlot(open, openRule)
	bySemantic, semanticOK := BindingRuleSlot(open, coldKey(997_002))
	if !sealedOK || !semanticOK || sealed != capability || bySemantic != capability || binding == open {
		t.Fatal("Rule capability authority changed at seal")
	}
	if _, found := MountedCapabilityForSlot(binding, rule); found {
		// The first fixture deliberately has no registered role.  A binding's
		// implementation cannot invent a capability after its terminal seal.
		t.Fatal("unregistered Rule capability was invented")
	}
}

func TestSchemaRuleBindingCheckerActivationDerivationCarriesExactProof(t *testing.T) {
	builder := NewSchema()
	factor, factorOK := DeclareFactorSlot[uint64](builder, coldKey(997_101))
	family, familyOK := DeclareSchemaActivationFamily(builder, coldKey(997_102))
	rule, ruleOK := DeclareSchemaActivationRule(builder, SchemaStructuralRuleSpec{
		Semantic:   coldKey(997_103),
		Admission:  SchemaAdmission{Basis: RuleAdmissionBasisTrustedTheorem, Identity: coldKey(997_104)},
		Activation: family,
	})
	schema, schemaOK := builder.Seal()
	binding := NewSchemaBinding(schema)
	if !factorOK || !familyOK || !ruleOK || !schemaOK || schema == nil ||
		!BindFactor(binding, factor, hotUintFactorSpec()) ||
		!BindActivationRule(binding, rule, HotActivationSpec{Admission: AdmitActivationByTrustedTheorem(coldKey(997_104)), Run: func(Activation) bool { return true }}) ||
		!binding.Seal() {
		t.Fatal("activation Rule binding")
	}
	implementation, ok := ActivationRuleImplementationAt(binding, rule)
	if !ok || implementation == nil || !implementation.binding.valid() || implementation.binding.proof == nil {
		t.Fatal("activation Rule proof")
	}
	proof := implementation.binding.proof
	shape, shapeOK := schema.ruleShapeAt(proof.ordinal)
	if !shapeOK || !proof.valid() || proof.outputKind != composition.StructuralOutput || proof.output != (composition.Key{}) || shape.ActivationCount != 1 || !shape.ActivationFamily.Available() {
		t.Fatal("activation checker lost exact structural proof")
	}
	foreign := NewSchemaBinding(schema)
	if _, foreignOK := ActivationRuleImplementationAt(foreign, rule); foreignOK {
		t.Fatal("foreign open Binding issued activation proof")
	}
}

func TestSchemaRuleBindingRejectsShapeAndColdAdmissionMismatch(t *testing.T) {
	schema, factor, rule, write := zeroWriteRuleLawSchema(t, 1)
	binding := NewSchemaBinding(schema)
	if !BindFactor(binding, factor, hotUintFactorSpec()) || BindRule[uint64, uint64, struct{}](binding, rule, write, factor, lawHotRuleSpec(), testRuleProjector[struct{}]) || !binding.Poisoned() {
		t.Fatal("unsupported Rule shape admitted")
	}
	schema, factor, rule, write = zeroWriteRuleLawSchema(t, 0)
	binding = NewSchemaBinding(schema)
	bad := lawHotRuleSpec()
	bad.Admission = AdmitRuleByTrustedTheorem[uint64, struct{}](coldKey(997_099))
	if !BindFactor(binding, factor, hotUintFactorSpec()) || BindRule[uint64, uint64, struct{}](binding, rule, write, factor, bad, testRuleProjector[struct{}]) || !binding.Poisoned() {
		t.Fatal("mismatched cold admission accepted")
	}
}

func TestSchemaRuleBindingRejectsForeignEqualAuthority(t *testing.T) {
	first, _, _, _, _, _ := sealedLawRule(t, 0)
	second, _, _, foreignFactor, _, _ := sealedLawRule(t, 0)
	if first == second || first.state == second.state || first.state.authority == second.state.authority {
		t.Fatal("equal schemas collapsed Binding authorities")
	}
	// The local schema token cannot be admitted through a foreign binding, even
	// when the schema is content-equal.
	if BindFactor(first, foreignFactor, hotUintFactorSpec()) || !first.Sealed() {
		t.Fatal("foreign equal-schema Factor crossed authority")
	}
}

func TestSchemaRuleBindingMemberGeometryIsExactAndStatic(t *testing.T) {
	_, implementation, _, _, _, _ := sealedLawRule(t, 0)
	proof := implementation.binding.proof
	valid := schemaRuleMemberGeometryLaw{
		rule: proof.semantic, family: proof.operandFamily, writes: 1,
		surface: equation.Surface{Factor: proof.output, Form: equation.SurfaceWriteExact, Local: 1, Mode: equation.TargetModeStrong},
	}
	if _, ok := exactSchemaRuleMemberGeometry(proof, valid); !ok {
		t.Fatal("exact Rule member geometry rejected")
	}
	for name, edit := range map[string]func(*schemaRuleMemberGeometryLaw){
		"foreign-family": func(row *schemaRuleMemberGeometryLaw) { row.family = compositionKeyOf(coldKey(997_199)) },
		"dynamic":        func(row *schemaRuleMemberGeometryLaw) { row.dynamic = true },
		"hidden-route":   func(row *schemaRuleMemberGeometryLaw) { row.route = 1 },
		"weak-write":     func(row *schemaRuleMemberGeometryLaw) { row.surface.Mode = equation.TargetModeWeak },
	} {
		t.Run(name, func(t *testing.T) {
			row := valid
			edit(&row)
			if _, ok := exactSchemaRuleMemberGeometry(proof, row); ok {
				t.Fatal("non-exact Rule member geometry admitted")
			}
		})
	}
}

func TestSchemaRuleBindingReadCarryAndSelectedProductProof(t *testing.T) {
	builder := NewSchema()
	factor, factorOK := DeclareFactorSlot[uint64](builder, coldKey(997_301))
	readForm, readOK := factor.ExactRead()
	writeForm, writeOK := factor.ExactWrite()
	rule, ruleOK := DeclareRuleSlot[uint64, struct{}](builder, SchemaRuleSpec[uint64]{
		Semantic: coldKey(997_302), OperandFamily: coldKey(997_303), Inputs: 1,
		Admission: SchemaAdmission{Basis: RuleAdmissionBasisTrustedTheorem, Identity: coldKey(997_304)}, Output: factor.Ref(),
	})
	input, inputOK := rule.Input(0)
	base, baseOK := SchemaRead(rule, readForm, input)
	selected, selectedOK := SchemaSelectedRead[uint64](rule, readForm, input, base.Ref())
	carry, carryOK := SchemaCarryFrom(rule, input, factor.Ref())
	write, routeOK := SchemaRouteWrite(rule, writeForm, selected)
	schema, schemaOK := builder.Seal()
	if !factorOK || !readOK || !writeOK || !ruleOK || !inputOK || !baseOK || !selectedOK || !carryOK || !routeOK || !schemaOK || schema == nil {
		t.Fatal("selected Rule schema")
	}
	binding := NewSchemaBinding(schema)
	if !BindFactor(binding, factor, hotUintFactorSpec()) {
		t.Fatal("selected Factor binding")
	}
	hot := HotRuleSpec[uint64, struct{}]{
		OperandContent: func(value struct{}) (struct{}, [32]byte, bool) { return value, [32]byte{0x6b}, true },
		Admission:      AdmitRuleByTrustedTheorem[uint64, struct{}](coldKey(997_304)),
		Transfer:       func(Access[uint64, struct{}]) bool { return true },
	}
	exactBound, selectedBound := false, false
	bound := BindSelectedRouteRule[uint64, uint64, struct{}](binding, rule, carry, write, factor.Ref(), hot, HotCarrySpec[uint64, struct{}]{}, func(struct{}) (uint64, bool) { return 1, true }, func(tx *SelectedRouteRuleBindingTransaction[uint64, uint64, struct{}]) bool {
		_, exactBound = AddSelectedRouteExactRead(tx, base, factor.Ref(), func(struct{}) (uint64, bool) { return 1, true })
		_, selectedBound = AddSelectedRouteRead[uint64, uint64, struct{}, uint64, uint64](tx, selected, factor.Ref(), func(SelectorContext) bool { return true })
		return exactBound && selectedBound
	})
	if !bound || !binding.Seal() {
		t.Fatalf("selected Rule binding bound=%t exact=%t selected=%t poisoned=%t", bound, exactBound, selectedBound, binding.Poisoned())
	}
	implementation, implementationOK := RuleImplementationAt[uint64, uint64, struct{}](binding, rule)
	if !implementationOK || implementation == nil || implementation.binding.proof == nil {
		t.Fatal("selected Rule implementation")
	}
	proof := implementation.binding.proof
	selectedProof, selectedProofOK := implementation.selectedRead(1)
	routeProof, routeProofOK := implementation.routeWrite()
	if !proof.valid() || proof.reads != 2 || proof.carries != 1 || !selectedProofOK || !selectedProof.Valid() || !routeProofOK || !routeProof.Valid() {
		t.Fatal("read/carry/selected product proof was not sealed")
	}
}
