package engine

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
)

func TestFactorRefKeepsOnlyCanonicalRowAndRaw(t *testing.T) {
	typeOfRef := reflect.TypeOf(Ref[uint64]{})
	want := []string{"row", "raw", "_"}
	if typeOfRef.NumField() != len(want) {
		t.Fatalf("factor Ref fields = %d, want %d", typeOfRef.NumField(), len(want))
	}
	for index, name := range want {
		if typeOfRef.Field(index).Name != name {
			t.Fatalf("factor Ref field[%d] = %q, want %q", index, typeOfRef.Field(index).Name, name)
		}
	}
	if typeOfRef.Field(0).Type != reflect.TypeOf((*schemaFactorBinding)(nil)).Elem() {
		t.Fatalf("factor Ref row type = %v, want canonical schemaFactorBinding", typeOfRef.Field(0).Type)
	}
	typeOfImplementation := reflect.TypeOf(FactorImplementation[uint64, uint64]{})
	wantImplementation := []string{"row", "ordinal", "algebra"}
	if typeOfImplementation.NumField() != len(wantImplementation) {
		t.Fatalf("FactorImplementation fields = %d, want %d", typeOfImplementation.NumField(), len(wantImplementation))
	}
	for index, name := range wantImplementation {
		if typeOfImplementation.Field(index).Name != name {
			t.Fatalf("FactorImplementation field[%d] = %q, want %q", index, typeOfImplementation.Field(index).Name, name)
		}
	}
	if typeOfImplementation.Field(0).Type != reflect.TypeOf((*schemaFactorBinding)(nil)).Elem() {
		t.Fatalf("FactorImplementation row type = %v, want canonical schemaFactorBinding", typeOfImplementation.Field(0).Type)
	}
}

// schemaRuleMemberGeometryLaw is the small cold-row view consumed by the
// current member binder.  It deliberately carries no declaration or binding
// handle: geometry is checked against the sealed Rule cell only.
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
		Output: factor.Ref(),
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
		OperandResolver: func(OperandCoords) (struct{}, bool) { return struct{}{}, true },
		Fold: func(frame Frame[uint64, struct{}]) RuleResult[uint64] {
			return Staged(frame, uint64(1))
		},
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
	if !ok || implementation == nil {
		t.Fatal("sealed Rule implementation")
	}
	if _, valid := implementation.sealedRuleCell(); !valid {
		t.Fatal("sealed Rule implementation address")
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
	if !ok || implementation == nil || implementation.cell == nil || implementation.cell.impl == nil {
		t.Fatal("Rule write implementation")
	}
	local, projected := implementation.cell.impl.projectWrite(struct{}{})
	if !projected || local != 7 {
		t.Fatalf("write projector=%d/%t", local, projected)
	}
}

func TestSchemaRuleBindingPublishesZeroInputExactWrite(t *testing.T) {
	_, implementation, _, _, _, _ := sealedLawRule(t, 0)
	shape, shapeOK := implementation.cell.schema.ruleShapeAt(implementation.ordinal)
	if !shapeOK || shape.OutputKind != composition.FactorOutput || shape.ReadCount != 0 || shape.WriteCount != 1 || shape.CarryCount != 0 {
		t.Fatal("zero-input exact Rule shape")
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

func TestSchemaRuleBindingCheckerActivationRetainsOnlyCellAndOrdinal(t *testing.T) {
	builder := NewSchema()
	factor, factorOK := DeclareFactorSlot[uint64](builder, coldKey(997_101))
	family, familyOK := DeclareSchemaActivationFamily(builder, coldKey(997_102))
	rule, ruleOK := DeclareSchemaActivationRule(builder, SchemaStructuralRuleSpec{
		Semantic:   coldKey(997_103),
		Activation: family,
	})
	schema, schemaOK := builder.Seal()
	binding := NewSchemaBinding(schema)
	if !factorOK || !familyOK || !ruleOK || !schemaOK || schema == nil ||
		!BindFactor(binding, factor, hotUintFactorSpec()) ||
		!BindActivationRule(binding, rule, HotActivationSpec{Fold: func(frame ActivationFrame) ActivationResult { return Activated(frame) }}) ||
		!binding.Seal() {
		t.Fatal("activation Rule binding")
	}
	implementation, ok := ActivationRuleImplementationAt(binding, rule)
	if !ok || implementation == nil || implementation.cell == nil || implementation.ordinal != implementation.cell.ordinal {
		t.Fatal("activation Rule cell")
	}
	typeOfImplementation := reflect.TypeOf(*implementation)
	if typeOfImplementation.NumField() != 2 || typeOfImplementation.Field(0).Name != "cell" || typeOfImplementation.Field(1).Name != "ordinal" {
		t.Fatal("activation implementation retained non-cell state")
	}
	shape, shapeOK := schema.ruleShapeAt(implementation.ordinal)
	if !shapeOK || shape.OutputKind != composition.StructuralOutput || shape.Output != (composition.Key{}) || shape.ActivationCount != 1 || !shape.ActivationFamily.Available() {
		t.Fatal("activation checker lost exact structural shape")
	}
	foreign := NewSchemaBinding(schema)
	if _, foreignOK := ActivationRuleImplementationAt(foreign, rule); foreignOK {
		t.Fatal("foreign open Binding issued activation proof")
	}
}

func TestSchemaRuleBindingRejectsUnsupportedShape(t *testing.T) {
	schema, factor, rule, write := zeroWriteRuleLawSchema(t, 1)
	binding := NewSchemaBinding(schema)
	if !BindFactor(binding, factor, hotUintFactorSpec()) || BindRule[uint64, uint64, struct{}](binding, rule, write, factor, lawHotRuleSpec(), testRuleProjector[struct{}]) || !binding.Poisoned() {
		t.Fatal("unsupported Rule shape admitted")
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
	shape, shapeOK := implementation.cell.schema.ruleShapeAt(implementation.ordinal)
	if !shapeOK {
		t.Fatal("Rule implementation shape")
	}
	rule := implementation.cell.schema.ruleSemanticAt(implementation.ordinal)
	valid := schemaRuleMemberGeometryLaw{
		rule: rule, family: shape.OperandFamily, writes: 1,
		surface: equation.Surface{Factor: shape.Output, Form: equation.SurfaceWriteExact, Local: 1, Mode: equation.TargetModeStrong},
	}
	if _, ok := exactSchemaRuleMemberGeometry(implementation.cell, implementation.ordinal, valid); !ok {
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
			if _, ok := exactSchemaRuleMemberGeometry(implementation.cell, implementation.ordinal, row); ok {
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
		Output: factor.Ref(),
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
		OperandContent:  func(value struct{}) (struct{}, [32]byte, bool) { return value, [32]byte{0x6b}, true },
		OperandResolver: func(OperandCoords) (struct{}, bool) { return struct{}{}, true },
		Fold: func(frame Frame[uint64, struct{}]) RuleResult[uint64] {
			return Staged(frame, uint64(1))
		},
	}
	bound := BindSelectedRouteRuleDirect[uint64](binding, rule, carry, write, factor.Ref(), hot, HotCarrySpec[uint64, struct{}]{}, func(struct{}) (uint64, bool) { return 1, true })
	_, exactBound := BindSelectedRuleDirectExactRead[uint64, uint64, struct{}, uint64](binding, rule, base, factor.Ref(), func(struct{}) (uint64, bool) { return 1, true })
	_, selectedBound := BindSelectedRuleDirectSelectedRead[uint64, uint64, struct{}, uint64, uint64](binding, rule, selected, factor.Ref(), func(SelectorContext) bool { return true })
	if !bound || !binding.Seal() {
		t.Fatalf("selected Rule binding bound=%t exact=%t selected=%t poisoned=%t", bound, exactBound, selectedBound, binding.Poisoned())
	}
	implementation, implementationOK := RuleImplementationAt[uint64, uint64, struct{}](binding, rule)
	if !implementationOK || implementation == nil {
		t.Fatal("selected Rule implementation")
	}
	shape, shapeOK := implementation.cell.schema.ruleShapeAt(implementation.ordinal)
	sealedHot := implementation.cell.impl
	selectedRow := implementation.cell.schemaRuleReadAt(1)
	routeRow := implementation.cell.schemaRuleReadAt(sealedHot.routeRead - 1)
	if !shapeOK || shape.ReadCount != 2 || shape.CarryCount != 1 || shape.WriteCount != 1 || sealedHot.writeMode != directRuleWriteRoute || sealedHot.routeRead != 2 || selectedRow == nil || selectedRow.kind != composition.ReadSelect || selectedRow.owner != implementation.cell || selectedRow.ownerOrdinal != implementation.ordinal || selectedRow.readOrdinal != 1 || len(selectedRow.dependencies) == 0 || routeRow != selectedRow {
		t.Fatal("read/carry/selected product shape was not sealed")
	}
}

func directSelectedRuleLawSchema(t testing.TB) (*Schema, *FactorSlot[uint64], *FactorSlot[uint64], *RuleSlot[uint64, struct{}], SchemaReadSlot[uint64], SchemaReadSlot[uint64], SchemaCarrySlot[uint64], SchemaWriteSlot[uint64]) {
	t.Helper()
	builder := NewSchema()
	factor, factorOK := DeclareFactorSlot[uint64](builder, coldKey(997_701))
	foreign, foreignOK := DeclareFactorSlot[uint64](builder, coldKey(997_702))
	readForm, readOK := factor.ExactRead()
	writeForm, writeOK := factor.ExactWrite()
	rule, ruleOK := DeclareRuleSlot[uint64, struct{}](builder, SchemaRuleSpec[uint64]{
		Semantic: coldKey(997_703), OperandFamily: coldKey(997_704), Inputs: 1,
		Output: factor.Ref(),
	})
	input, inputOK := rule.Input(0)
	left, leftOK := SchemaRead(rule, readForm, input)
	right, rightOK := SchemaRead(rule, readForm, input)
	carry, carryOK := SchemaCarryFrom(rule, input, factor.Ref())
	writeSlot, writeSlotOK := SchemaWrite(rule, writeForm)
	schema, schemaOK := builder.Seal()
	if !factorOK || !foreignOK || !readOK || !writeOK || !ruleOK || !inputOK || !leftOK || !rightOK || !carryOK || !writeSlotOK || !schemaOK || schema == nil {
		t.Fatal("direct selected Rule schema")
	}
	return schema, factor, foreign, rule, left, right, carry, writeSlot
}

func directSelectedRuleHotLaw() HotRuleSpec[uint64, struct{}] {
	return HotRuleSpec[uint64, struct{}]{
		OperandContent:  func(value struct{}) (struct{}, [32]byte, bool) { return value, [32]byte{0x7a}, true },
		OperandResolver: func(OperandCoords) (struct{}, bool) { return struct{}{}, true },
		Fold: func(frame Frame[uint64, struct{}]) RuleResult[uint64] {
			return Staged(frame, uint64(1))
		},
	}
}

func bindDirectSelectedRuleLaw(t testing.TB) (*SchemaBinding, *FactorSlot[uint64], *FactorSlot[uint64], *RuleSlot[uint64, struct{}], SchemaReadSlot[uint64], SchemaReadSlot[uint64]) {
	t.Helper()
	schema, factor, foreign, rule, left, right, carry, write := directSelectedRuleLawSchema(t)
	binding := NewSchemaBinding(schema)
	if !BindFactor(binding, factor, hotUintFactorSpec()) || !BindFactor(binding, foreign, hotUintFactorSpec()) ||
		!BindSelectedRuleDirect[uint64](binding, rule, carry, write, factor.Ref(), directSelectedRuleHotLaw(), HotCarrySpec[uint64, struct{}]{}, func(struct{}) (uint64, bool) { return 0, true }) {
		t.Fatal("direct selected Rule binding")
	}
	return binding, factor, foreign, rule, left, right
}

func TestSchemaRuleBindingDirectReadOrdinalsAreOrderIndependent(t *testing.T) {
	binding, factor, _, rule, leftSlot, rightSlot := bindDirectSelectedRuleLaw(t)
	right, rightOK := BindSelectedRuleDirectExactRead[uint64, uint64, struct{}, uint64](binding, rule, rightSlot, factor.Ref(), func(struct{}) (uint64, bool) { return 1, true })
	left, leftOK := BindSelectedRuleDirectExactRead[uint64, uint64, struct{}, uint64](binding, rule, leftSlot, factor.Ref(), func(struct{}) (uint64, bool) { return 0, true })
	if !rightOK || !leftOK || right.index != 1 || left.index != 0 || right.row == nil || left.row == nil || right.row.readOrdinal != 1 || left.row.readOrdinal != 0 || !binding.Seal() {
		t.Fatalf("direct ordinal binding right=%t/%d left=%t/%d sealed=%t", rightOK, right.index, leftOK, left.index, binding.Sealed())
	}
	implementation, implementationOK := RuleImplementationAt[uint64, uint64, struct{}](binding, rule)
	shape, shapeOK := implementation.cell.schema.ruleShapeAt(implementation.ordinal)
	if !implementationOK || implementation == nil || !shapeOK || shape.ReadCount != 2 || shape.CarryCount != 1 || shape.WriteCount != 1 {
		t.Fatal("direct selected Rule shape")
	}
	if implementation.cell.impl.reads[0].readRow() != left.row || implementation.cell.impl.reads[1].readRow() != right.row {
		t.Fatal("returned Reads do not share the compiled owner rows")
	}
}

func TestSchemaRuleBindingDirectReadMismatchAndDuplicatePoison(t *testing.T) {
	binding, factor, foreign, rule, leftSlot, _ := bindDirectSelectedRuleLaw(t)
	if _, ok := BindSelectedRuleDirectExactRead[uint64, uint64, struct{}, uint64](binding, rule, leftSlot, foreign.Ref(), func(struct{}) (uint64, bool) { return 0, true }); ok || !binding.Poisoned() {
		t.Fatal("foreign read Factor crossed direct Rule")
	}

	binding, factor, _, rule, leftSlot, _ = bindDirectSelectedRuleLaw(t)
	if _, ok := BindSelectedRuleDirectExactRead[uint64, uint64, struct{}, uint64](binding, rule, leftSlot, factor.Ref(), func(struct{}) (uint64, bool) { return 0, true }); !ok {
		t.Fatal("initial direct read")
	}
	if _, ok := BindSelectedRuleDirectExactRead[uint64, uint64, struct{}, uint64](binding, rule, leftSlot, factor.Ref(), func(struct{}) (uint64, bool) { return 0, true }); ok || !binding.Poisoned() {
		t.Fatal("duplicate direct read was not poisoned")
	}
}

func TestSchemaRuleBindingDirectIncompleteSealPoisons(t *testing.T) {
	binding, _, _, _, _, _ := bindDirectSelectedRuleLaw(t)
	if binding.Seal() || !binding.Poisoned() {
		t.Fatal("incomplete direct Rule sealed")
	}
}

func directNoCarryRuleLawSchema(t testing.TB) (*Schema, *FactorSlot[uint64], *RuleSlot[uint64, struct{}], SchemaWriteSlot[uint64]) {
	t.Helper()
	builder := NewSchema()
	factor, factorOK := DeclareFactorSlot[uint64](builder, coldKey(997_711))
	writeForm, writeOK := factor.ExactWrite()
	rule, ruleOK := DeclareRuleSlot[uint64, struct{}](builder, SchemaRuleSpec[uint64]{
		Semantic: coldKey(997_712), OperandFamily: coldKey(997_713), Inputs: 1,
		Output: factor.Ref(),
	})
	_, inputOK := rule.Input(0)
	write, writeSlotOK := SchemaWrite(rule, writeForm)
	schema, schemaOK := builder.Seal()
	if !factorOK || !writeOK || !ruleOK || !inputOK || !writeSlotOK || !schemaOK || schema == nil {
		t.Fatal("direct no-carry Rule schema")
	}
	return schema, factor, rule, write
}

func directNoCarryRuleHotLaw() HotRuleSpec[uint64, struct{}] {
	return HotRuleSpec[uint64, struct{}]{
		OperandContent:  func(value struct{}) (struct{}, [32]byte, bool) { return value, [32]byte{0x7b}, true },
		OperandResolver: func(OperandCoords) (struct{}, bool) { return struct{}{}, true },
		Fold: func(frame Frame[uint64, struct{}]) RuleResult[uint64] {
			return Staged(frame, uint64(1))
		},
	}
}

func TestSchemaRuleBindingDirectNoCarryExactCompletes(t *testing.T) {
	schema, factor, rule, write := directNoCarryRuleLawSchema(t)
	binding := NewSchemaBinding(schema)
	if !BindFactor(binding, factor, hotUintFactorSpec()) || !BindSelectedExactRuleDirect[uint64](binding, rule, write, factor.Ref(), directNoCarryRuleHotLaw(), func(struct{}) (uint64, bool) { return 0, true }) || !binding.Seal() {
		t.Fatal("direct no-carry exact Rule binding")
	}
	implementation, ok := RuleImplementationAt[uint64, uint64, struct{}](binding, rule)
	shape, shapeOK := implementation.cell.schema.ruleShapeAt(implementation.ordinal)
	if !ok || implementation == nil || !shapeOK || shape.Inputs != 1 || shape.ReadCount != 0 || shape.CarryCount != 0 || shape.WriteCount != 1 {
		t.Fatal("direct no-carry exact Rule shape")
	}
}

func directRouteRuleLawSchema(t testing.TB) (*Schema, *FactorSlot[uint64], *RuleSlot[uint64, struct{}], SchemaReadSlot[uint64], SchemaReadSlot[uint64], SchemaReadSlot[uint64], SchemaCarrySlot[uint64], SchemaWriteSlot[uint64]) {
	t.Helper()
	builder := NewSchema()
	factor, factorOK := DeclareFactorSlot[uint64](builder, coldKey(997_721))
	readForm, readOK := factor.ExactRead()
	writeForm, writeOK := factor.ExactWrite()
	rule, ruleOK := DeclareRuleSlot[uint64, struct{}](builder, SchemaRuleSpec[uint64]{
		Semantic: coldKey(997_722), OperandFamily: coldKey(997_723), Inputs: 1,
		Output: factor.Ref(),
	})
	input, inputOK := rule.Input(0)
	exact, exactOK := SchemaRead(rule, readForm, input)
	static, staticOK := SchemaSelectedRead[uint64](rule, readForm, input, exact.Ref())
	operand, operandOK := SchemaSelectedRead[uint64](rule, readForm, input, exact.Ref())
	carry, carryOK := SchemaCarryFrom(rule, input, factor.Ref())
	write, writeSlotOK := SchemaRouteWrite(rule, writeForm, static)
	schema, schemaOK := builder.Seal()
	if !factorOK || !readOK || !writeOK || !ruleOK || !inputOK || !exactOK || !staticOK || !operandOK || !carryOK || !writeSlotOK || !schemaOK || schema == nil {
		t.Fatal("direct routed Rule schema")
	}
	return schema, factor, rule, exact, static, operand, carry, write
}

func directRouteRuleHotLaw() HotRuleSpec[uint64, struct{}] {
	return HotRuleSpec[uint64, struct{}]{
		OperandContent:  func(value struct{}) (struct{}, [32]byte, bool) { return value, [32]byte{0x7c}, true },
		OperandResolver: func(OperandCoords) (struct{}, bool) { return struct{}{}, true },
		Fold: func(frame Frame[uint64, struct{}]) RuleResult[uint64] {
			return RuleResult[uint64]{}
		},
	}
}

func TestSchemaRuleBindingDirectRouteAndSelectedReadsComplete(t *testing.T) {
	schema, factor, rule, exact, static, operand, carry, write := directRouteRuleLawSchema(t)
	binding := NewSchemaBinding(schema)
	if !BindFactor(binding, factor, hotUintFactorSpec()) || !BindSelectedRouteRuleDirect[uint64](binding, rule, carry, write, factor.Ref(), directRouteRuleHotLaw(), HotCarrySpec[uint64, struct{}]{}, func(struct{}) (uint64, bool) { return 0, true }) {
		t.Fatal("direct routed Rule cell")
	}
	operandRead, operandOK := BindSelectedRuleDirectOperandRead[uint64, uint64, struct{}, uint64, uint64](binding, rule, operand, factor.Ref(), func(SelectorContext, struct{}) bool { return true })
	staticRead, staticOK := BindSelectedRuleDirectSelectedRead[uint64, uint64, struct{}, uint64, uint64](binding, rule, static, factor.Ref(), func(SelectorContext) bool { return true })
	exactRead, exactOK := BindSelectedRuleDirectExactRead[uint64, uint64, struct{}, uint64](binding, rule, exact, factor.Ref(), func(struct{}) (uint64, bool) { return 0, true })
	if !operandOK || !staticOK || !exactOK || operandRead.index != 2 || staticRead.index != 1 || exactRead.index != 0 || operandRead.row == nil || staticRead.row == nil || exactRead.row == nil || operandRead.row.kind != composition.ReadSelect || staticRead.row.kind != composition.ReadSelect || exactRead.row.kind != composition.ReadExact || !binding.Seal() {
		t.Fatalf("direct route reads operand=%t/%d static=%t/%d exact=%t/%d sealed=%t", operandOK, operandRead.index, staticOK, staticRead.index, exactOK, exactRead.index, binding.Sealed())
	}
	implementation, ok := RuleImplementationAt[uint64, uint64, struct{}](binding, rule)
	shape, shapeOK := implementation.cell.schema.ruleShapeAt(implementation.ordinal)
	if !ok || implementation == nil || !shapeOK || shape.ReadCount != 3 || shape.CarryCount != 1 || shape.WriteCount != 1 {
		t.Fatal("direct routed Rule shape")
	}
	hot := implementation.cell.impl
	routeRow := implementation.cell.schemaRuleReadAt(hot.routeRead - 1)
	if hot.writeMode != directRuleWriteRoute || hot.routeRead != 2 || routeRow == nil || routeRow.kind != composition.ReadSelect || routeRow.owner != implementation.cell || routeRow.ownerOrdinal != implementation.ordinal || routeRow.readOrdinal != 1 || len(routeRow.dependencies) == 0 {
		t.Fatal("direct routed write shape")
	}
}

func TestCanonicalRuleCellBoundaryRetainsRouteAndAuthorityFences(t *testing.T) {
	schema, factor, rule, exact, static, operand, carry, write := directRouteRuleLawSchema(t)
	binding := NewSchemaBinding(schema)
	if !BindFactor(binding, factor, hotUintFactorSpec()) || !BindSelectedRouteRuleDirect[uint64](binding, rule, carry, write, factor.Ref(), directRouteRuleHotLaw(), HotCarrySpec[uint64, struct{}]{}, func(struct{}) (uint64, bool) { return 0, true }) {
		t.Fatal("direct routed Rule cell")
	}
	if _, ok := BindSelectedRuleDirectExactRead[uint64, uint64, struct{}, uint64](binding, rule, exact, factor.Ref(), func(struct{}) (uint64, bool) { return 0, true }); !ok {
		t.Fatal("direct exact read")
	}
	if _, ok := BindSelectedRuleDirectSelectedRead[uint64, uint64, struct{}, uint64, uint64](binding, rule, static, factor.Ref(), func(SelectorContext) bool { return true }); !ok {
		t.Fatal("direct selected read")
	}
	if _, ok := BindSelectedRuleDirectOperandRead[uint64, uint64, struct{}, uint64, uint64](binding, rule, operand, factor.Ref(), func(SelectorContext, struct{}) bool { return true }); !ok {
		t.Fatal("direct operand read")
	}
	if !binding.Seal() {
		t.Fatal("direct routed Rule seal")
	}
	implementation, implementationOK := RuleImplementationAt[uint64, uint64, struct{}](binding, rule)
	if !implementationOK || implementation == nil {
		t.Fatal("canonical Rule cell owner")
	}
	cell, cellOK := implementation.sealedRuleCell()
	declared, declaredOK := cell.declareRuleOperand(lawCanonicalRuleCoords())
	anchor := lawCanonicalRuleAnchor(t)
	surfaces, surfacesOK := cell.declareRuleSurfaces(declared, anchor)
	semantic, family := implementation.cell.impl.ruleSemantic, implementation.cell.impl.operandFamily
	row, rowOK := resolveDeclaredRuleInstance(schema, binding.state.authority, semantic, family, anchor, surfaces)
	expectedWrite, expectedWriteOK := schema.ruleWriteShapeAt(implementation.ordinal, 0)
	if !cellOK || !declaredOK || !surfacesOK || !rowOK || !expectedWriteOK || len(row.Writes) != 1 || row.Writes[0].Route != expectedWrite.Route {
		t.Fatal("route surface did not preserve the sealed graph route index")
	}

	foreign := surfaces
	foreign.writes = append([]ruleWriteSurface(nil), surfaces.writes...)
	foreign.writes[0].authority = &schemaBindingAuthority{marker: 2}
	if _, accepted := resolveDeclaredRuleInstance(schema, binding.state.authority, semantic, family, anchor, foreign); accepted {
		t.Fatal("foreign surface crossed the retained authority fence")
	}
	malformed := surfaces
	malformed.writes = append([]ruleWriteSurface(nil), surfaces.writes...)
	malformed.writes[0].value.Form = equation.SurfaceWriteExact
	if _, accepted := resolveDeclaredRuleInstance(schema, binding.state.authority, semantic, family, anchor, malformed); accepted {
		t.Fatal("malformed route surface crossed the retained form fence")
	}
}

func TestCanonicalRuleCellRejectsMalformedSelectedDependencyOrdering(t *testing.T) {
	schema, factor, rule, exact, static, operand, carry, write := directRouteRuleLawSchema(t)
	binding := NewSchemaBinding(schema)
	if !BindFactor(binding, factor, hotUintFactorSpec()) || !BindSelectedRouteRuleDirect[uint64](binding, rule, carry, write, factor.Ref(), directRouteRuleHotLaw(), HotCarrySpec[uint64, struct{}]{}, func(struct{}) (uint64, bool) { return 0, true }) {
		t.Fatal("direct routed Rule cell")
	}
	if _, ok := BindSelectedRuleDirectExactRead[uint64, uint64, struct{}, uint64](binding, rule, exact, factor.Ref(), func(struct{}) (uint64, bool) { return 0, true }); !ok {
		t.Fatal("direct exact read")
	}
	if _, ok := BindSelectedRuleDirectSelectedRead[uint64, uint64, struct{}, uint64, uint64](binding, rule, static, factor.Ref(), func(SelectorContext) bool { return true }); !ok {
		t.Fatal("direct selected read")
	}
	if _, ok := BindSelectedRuleDirectOperandRead[uint64, uint64, struct{}, uint64, uint64](binding, rule, operand, factor.Ref(), func(SelectorContext, struct{}) bool { return true }); !ok {
		t.Fatal("direct operand read")
	}
	if !binding.Seal() {
		t.Fatal("direct routed Rule seal")
	}
	implementation, implementationOK := RuleImplementationAt[uint64, uint64, struct{}](binding, rule)
	if !implementationOK || implementation == nil {
		t.Fatal("canonical Rule cell owner")
	}
	row := implementation.cell.schemaRuleReadAt(1)
	if row == nil || len(row.dependencies) == 0 {
		t.Fatal("selected dependency row")
	}
	row.dependencies[0] = row.readOrdinal
	cell, cellOK := implementation.sealedRuleCell()
	declared, declaredOK := cell.declareRuleOperand(lawCanonicalRuleCoords())
	_, surfacesOK := cell.declareRuleSurfaces(declared, lawCanonicalRuleAnchor(t))
	if !cellOK || !declaredOK || surfacesOK {
		t.Fatal("selected surface accepted a non-prior dependency")
	}
}

func TestSchemaRuleBindingDirectSummaryReadCompletes(t *testing.T) {
	base := uint64(997_731)
	builder := NewSchema()
	factor, factorOK := DeclareFactorSlot[uint64](builder, coldKey(base))
	form, formOK := factor.SummaryRead(coldKey(base + 1))
	writeForm, writeOK := factor.ExactWrite()
	rule, ruleOK := DeclareRuleSlot[uint64, struct{}](builder, SchemaRuleSpec[uint64]{
		Semantic: coldKey(base + 2), OperandFamily: coldKey(base + 3), Inputs: 1,
		Output: factor.Ref(),
	})
	input, inputOK := rule.Input(0)
	summarySlot, summaryOK := SchemaRead(rule, form, input)
	carry, carryOK := SchemaCarryFrom(rule, input, factor.Ref())
	write, writeSlotOK := SchemaWrite(rule, writeForm)
	schema, schemaOK := builder.Seal()
	binding := NewSchemaBinding(schema)
	spec := directSelectedRuleHotLaw()
	bound := factorOK && formOK && writeOK && ruleOK && inputOK && summaryOK && carryOK && writeSlotOK && schemaOK &&
		BindFactor(binding, factor, hotUintFactorSpec()) &&
		BindSummaryReadForFactor[uint64, uint64, OrderedCells[uint64]](binding, factor, form,
			func(value OrderedCells[uint64]) OrderedCells[uint64] { return value },
			func(left, right OrderedCells[uint64]) bool {
				return equalOrderedCellRecords(left.record, right.record, func(left, right uint64) bool { return left == right })
			},
			func(value OrderedCells[uint64]) uint64 { return uint64(value.Count()) }) &&
		BindSelectedRuleDirect[uint64](binding, rule, carry, write, factor.Ref(), spec, HotCarrySpec[uint64, struct{}]{}, func(struct{}) (uint64, bool) { return 0, true })
	if !bound {
		t.Fatal("direct summary Rule cell")
	}
	summaryRuntime, summaryBound := BindSelectedRuleDirectSummaryRead[uint64, uint64, struct{}, uint64, OrderedCells[uint64]](binding, rule, summarySlot, factor.Ref(), form)
	if !summaryBound || summaryRuntime.row == nil || summaryRuntime.row.kind != composition.ReadSummary || summaryRuntime.row.readOrdinal != 0 || summaryRuntime.row.summaryOrdinal != 0 || summaryRuntime.row.semantic != compositionKeyOf(coldKey(base+1)) || !binding.Seal() {
		t.Fatalf("direct summary read bound=%t sealed=%t poisoned=%t", summaryBound, binding.Sealed(), binding.Poisoned())
	}
	implementation, implementationOK := RuleImplementationAt[uint64, uint64, struct{}](binding, rule)
	shape, shapeOK := implementation.cell.schema.ruleShapeAt(implementation.ordinal)
	if !implementationOK || implementation == nil || !shapeOK || shape.ReadCount != 1 || shape.CarryCount != 1 || shape.WriteCount != 1 {
		t.Fatal("direct summary Rule shape")
	}
}

func TestSchemaRuleBindingDirectConstructorsRejectWrongGeometry(t *testing.T) {
	schema, factor, rule, _, _, _, carry, write := directRouteRuleLawSchema(t)
	binding := NewSchemaBinding(schema)
	if !BindFactor(binding, factor, hotUintFactorSpec()) || BindSelectedExactRuleDirect[uint64](binding, rule, write, factor.Ref(), directRouteRuleHotLaw(), func(struct{}) (uint64, bool) { return 0, true }) || !binding.Poisoned() {
		t.Fatal("no-carry direct constructor admitted routed shape")
	}

	schema, factor, rule, _, _, _, carry, write = directRouteRuleLawSchema(t)
	binding = NewSchemaBinding(schema)
	if !BindFactor(binding, factor, hotUintFactorSpec()) || BindSelectedRouteRuleDirect[uint64](binding, rule, carry, write, factor.Ref(), directRouteRuleHotLaw(), HotCarrySpec[uint64, struct{}]{Apply: func(value struct{}, input uint64) (uint64, bool) { return input, true }}, func(struct{}) (uint64, bool) { return 0, true }) || !binding.Poisoned() {
		t.Fatal("mismatched direct carry transform admitted")
	}
}

func lawCanonicalRuleAnchor(t testing.TB) ruleSurfaceAnchor {
	t.Helper()
	batch := equation.NewBatch()
	site, siteOK := batch.AdmitSite(compositionKeyOf(coldKey(997_500)), equation.EmptyScope(), equation.TrueExpr(), equation.InitPresent)
	occurrence, occurrenceOK := batch.At(site)
	entity, entityOK := operandEntityForContent([32]byte{0x5a})
	operand, operandOK := batch.AdmitOperand(occurrence, entity)
	if !siteOK || !occurrenceOK || !entityOK || !operandOK || !batch.Seal() {
		t.Fatal("canonical Rule cell source anchor")
	}
	return ruleSurfaceAnchor{occurrence: occurrence, operand: operand}
}

// TestCanonicalRuleCellThreadsExactReadAndCarryThroughProductEvidenceAndPatch
// rehomes the old compiler-thread law on the current canonical cell. The
// sealed cell, compiled read row, carry token and one write surface must all
// survive into the declaration consumed by ConstructProgram.
func TestCanonicalRuleCellThreadsExactReadAndCarryThroughProductEvidenceAndPatch(t *testing.T) {
	builder := NewSchema()
	factor, factorOK := DeclareFactorSlot[uint64](builder, coldKey(997_510))
	readForm, readOK := factor.ExactRead()
	writeForm, writeOK := factor.ExactWrite()
	rule, ruleOK := DeclareRuleSlot[uint64, struct{}](builder, SchemaRuleSpec[uint64]{
		Semantic: coldKey(997_511), OperandFamily: coldKey(997_512), Inputs: 1,
		Output: factor.Ref(),
	})
	input, inputOK := rule.Input(0)
	read, readSlotOK := SchemaRead(rule, readForm, input)
	carry, carryOK := SchemaCarryFrom(rule, input, factor.Ref())
	writeSlot, writeSlotOK := SchemaWrite(rule, writeForm)
	schema, schemaOK := builder.Seal()
	binding := NewSchemaBinding(schema)
	hot := lawHotRuleSpec()
	readRuntime := Read[OrderedCells[uint64]]{}
	bound := false
	if factorOK && readOK && writeOK && ruleOK && inputOK && readSlotOK && carryOK && writeSlotOK && schemaOK && BindFactor(binding, factor, hotUintFactorSpec()) {
		readRuntime, bound = BindRuleWithExactReadAndCarry[uint64, uint64, struct{}, uint64, uint64](binding, rule, read, factor, carry, writeSlot, factor, hot, HotCarrySpec[uint64, struct{}]{}, func(struct{}) (uint64, bool) { return 0, true }, func(struct{}) (uint64, bool) { return 1, true })
	}
	if !bound || !binding.Seal() {
		t.Fatalf("exact read/carry canonical cell binding factor=%t read=%t write=%t rule=%t input=%t readSlot=%t carry=%t writeSlot=%t schema=%t bound=%t poisoned=%t", factorOK, readOK, writeOK, ruleOK, inputOK, readSlotOK, carryOK, writeSlotOK, schemaOK, bound, binding.Poisoned())
	}
	implementation, implementationOK := RuleImplementationAt[uint64, uint64, struct{}](binding, rule)
	shape, shapeOK := implementation.cell.schema.ruleShapeAt(implementation.ordinal)
	if !implementationOK || implementation == nil || readRuntime.row == nil || readRuntime.row.kind != composition.ReadExact || !shapeOK || shape.ReadCount != 1 || shape.CarryCount != 1 {
		t.Fatal("exact read/carry shape thread")
	}
	cell, cellOK := implementation.sealedRuleCell()
	declared, declaredOK := cell.declareRuleOperand(lawCanonicalRuleCoords())
	surfaces, surfacesOK := cell.declareRuleSurfaces(declared, lawCanonicalRuleAnchor(t))
	if !cellOK || !declaredOK || !surfacesOK || len(surfaces.reads) != 1 || len(surfaces.writes) != 1 || surfaces.carries != 1 || surfaces.reads[0].value.Form != equation.SurfaceReadExact {
		t.Fatal("exact read/carry canonical cell surfaces")
	}
}

// TestCanonicalRuleCellThreadsSummaryReadThroughProductAndEvidence retains the
// summary read's normalizer and ordered-cell contract at the sealed Rule
// boundary; no exact-read fallback or parallel callback may replace it.
func TestCanonicalRuleCellThreadsSummaryReadThroughProductAndEvidence(t *testing.T) {
	runCanonicalRuleSummaryThreadLaw(t, 997_520, 8)
}

// TestCanonicalRuleCellThreadsLargeSummaryReadThroughProductAndEvidence uses a
// wide Factor key plane, preserving the old large-summary no-alias law while
// exercising only the current sealed binding.
func TestCanonicalRuleCellThreadsLargeSummaryReadThroughProductAndEvidence(t *testing.T) {
	runCanonicalRuleSummaryThreadLaw(t, 997_530, 10_000)
}

type summaryLawOperand struct{}

func (summaryLawOperand) SummaryKeyCount() int { return 1 }

func (summaryLawOperand) SummaryKeyAt(index int) (uint64, bool) { return 0, index == 0 }

func runCanonicalRuleSummaryThreadLaw(t testing.TB, base uint64, keyEnd uint64) {
	t.Helper()
	builder := NewSchema()
	factor, factorOK := DeclareFactorSlot[uint64](builder, coldKey(base))
	form, formOK := factor.SummaryRead(coldKey(base + 1))
	writeForm, writeOK := factor.ExactWrite()
	rule, ruleOK := DeclareRuleSlot[uint64, summaryLawOperand](builder, SchemaRuleSpec[uint64]{
		Semantic: coldKey(base + 2), OperandFamily: coldKey(base + 3), Inputs: 1,
		Output: factor.Ref(),
	})
	input, inputOK := rule.Input(0)
	read, readOK := SchemaRead(rule, form, input)
	writeSlot, writeSlotOK := SchemaWrite(rule, writeForm)
	schema, schemaOK := builder.Seal()
	binding := NewSchemaBinding(schema)
	spec := hotUintFactorSpec()
	spec.KeyEnd = keyEnd
	if !factorOK || !formOK || !writeOK || !ruleOK || !inputOK || !readOK || !writeSlotOK || !schemaOK || !BindFactor(binding, factor, spec) || !BindSummaryReadForFactor[uint64, uint64, OrderedCells[uint64]](binding, factor, form,
		func(value OrderedCells[uint64]) OrderedCells[uint64] { return value },
		func(left, right OrderedCells[uint64]) bool {
			return equalOrderedCellRecords(left.record, right.record, func(left, right uint64) bool { return left == right })
		},
		func(value OrderedCells[uint64]) uint64 { return uint64(value.Count()) }) {
		t.Fatal("summary Factor form binding")
	}
	bound := BindSelectedExactRuleDirect[uint64](binding, rule, writeSlot, factor.Ref(), HotRuleSpec[uint64, summaryLawOperand]{
		OperandContent: func(summaryLawOperand) (summaryLawOperand, [32]byte, bool) {
			return summaryLawOperand{}, [32]byte{0x5a}, true
		},
		OperandResolver: func(OperandCoords) (summaryLawOperand, bool) { return summaryLawOperand{}, true },
		Fold: func(frame Frame[uint64, summaryLawOperand]) RuleResult[uint64] {
			return Staged(frame, uint64(1))
		},
	}, func(summaryLawOperand) (uint64, bool) { return 1, true })
	readRuntime, readBound := BindSelectedRuleDirectSummaryRead[uint64, uint64, summaryLawOperand, uint64, OrderedCells[uint64]](binding, rule, read, factor.Ref(), form)
	if !bound || !readBound || !binding.Seal() || readRuntime.row == nil || readRuntime.row.kind != composition.ReadSummary {
		t.Fatal("summary canonical cell binding")
	}
	implementation, implementationOK := RuleImplementationAt[uint64, uint64, summaryLawOperand](binding, rule)
	shape, shapeOK := implementation.cell.schema.ruleShapeAt(implementation.ordinal)
	if !implementationOK || implementation == nil || !shapeOK || shape.ReadCount != 1 || shape.CarryCount != 0 {
		t.Fatal("summary canonical cell shape thread")
	}
	cell, cellOK := implementation.sealedRuleCell()
	declared, declaredOK := cell.declareRuleOperand(lawCanonicalRuleCoords())
	surfaces, surfacesOK := cell.declareRuleSurfaces(declared, lawCanonicalRuleAnchor(t))
	mapped := declaredSummaryMappings(surfaces)
	if !cellOK || !declaredOK || !surfacesOK || len(mapped) != 1 || mapped[0].summary == nil || mapped[0].summary.state != binding.state || mapped[0].summary.authority != binding.state.authority {
		t.Fatal("summary canonical cell mapping")
	}
	summaries, appended := appendDeclaredSummary(nil, mapped[0].summary, binding.state, binding.state.authority)
	if !appended || len(summaries) != 1 || len(summaries[0].Keys) != 1 || summaries[0].Keys[0] != 0 {
		t.Fatal("summary canonical cell topology mapping")
	}
	foreign := *mapped[0].summary
	foreign.authority = &schemaBindingAuthority{marker: 1}
	if _, accepted := appendDeclaredSummary(nil, &foreign, binding.state, binding.state.authority); accepted {
		t.Fatal("summary mapping crossed the binding authority")
	}
	conflictingKeys := *mapped[0].summary
	conflictingKeys.keys = summaryKeyVector{keys: []uint64{1}, valid: true}
	if _, accepted := appendDeclaredSummary(summaries, &conflictingKeys, binding.state, binding.state.authority); accepted {
		t.Fatal("summary mapping accepted a conflicting key vector")
	}
}

// TestCanonicalRuleCellThreadsOneExactCarryThroughProductAndEvidence proves the
// no-read carry lane still publishes one carried input and one exact write
// through the same canonical cell surface.
func TestCanonicalRuleCellThreadsOneExactCarryThroughProductAndEvidence(t *testing.T) {
	// Construct the current carry lane explicitly so this test cannot pass by
	// observing the ordinary zero-input Rule shape.
	builder := NewSchema()
	factor, factorOK := DeclareFactorSlot[uint64](builder, coldKey(997_560))
	writeForm, writeOK := factor.ExactWrite()
	rule, ruleOK := DeclareRuleSlot[uint64, struct{}](builder, SchemaRuleSpec[uint64]{Semantic: coldKey(997_561), OperandFamily: coldKey(997_562), Inputs: 1, Output: factor.Ref()})
	input, inputOK := rule.Input(0)
	carry, carryOK := SchemaCarryFrom(rule, input, factor.Ref())
	writeSlot, writeSlotOK := SchemaWrite(rule, writeForm)
	schema, schemaOK := builder.Seal()
	binding := NewSchemaBinding(schema)
	hot := HotRuleSpec[uint64, struct{}]{
		OperandContent:  func(struct{}) (struct{}, [32]byte, bool) { return struct{}{}, [32]byte{0x5a}, true },
		OperandResolver: func(OperandCoords) (struct{}, bool) { return struct{}{}, true },
		Fold: func(frame Frame[uint64, struct{}]) RuleResult[uint64] {
			return Staged(frame, uint64(1))
		},
	}
	bound := BindFactor(binding, factor, hotUintFactorSpec()) && BindRuleWithCarry[uint64, uint64, struct{}](binding, rule, carry, writeSlot, factor, hot, HotCarrySpec[uint64, struct{}]{}, func(struct{}) (uint64, bool) { return 1, true })
	if !factorOK || !writeOK || !ruleOK || !inputOK || !carryOK || !writeSlotOK || !schemaOK || !bound || !binding.Seal() {
		t.Fatal("carry canonical cell binding")
	}
	implementation, implementationOK := RuleImplementationAt[uint64, uint64, struct{}](binding, rule)
	shape, shapeOK := implementation.cell.schema.ruleShapeAt(implementation.ordinal)
	if !implementationOK || implementation == nil || !shapeOK || shape.ReadCount != 0 || shape.CarryCount != 1 {
		t.Fatal("carry canonical cell shape thread")
	}
	cell, cellOK := implementation.sealedRuleCell()
	declared, declaredOK := cell.declareRuleOperand(lawCanonicalRuleCoords())
	surfaces, surfacesOK := cell.declareRuleSurfaces(declared, lawCanonicalRuleAnchor(t))
	if !cellOK || !declaredOK || !surfacesOK || len(surfaces.reads) != 0 || len(surfaces.writes) != 1 || surfaces.carries != 1 {
		t.Fatal("carry canonical cell surfaces")
	}
}

func lawCanonicalRuleCoords() OperandCoords {
	return OperandCoords{Member: programMatrixID(221), Mount: programMatrixID(222), Point: programMatrixID(223), Occurrence: programMatrixID(224)}
}
