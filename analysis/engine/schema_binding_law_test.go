// schema_binding_law_test.go proves the schema slot, Factor binding and summary fold laws.

package engine

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	coldcomposition "github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/identity"
)

func schemaSlotFixture(t testing.TB, routed bool, reverse bool) (*SchemaBuilder, *FactorSlot[uint64], SchemaReadForm[uint64], SchemaWriteForm[uint64], *RuleSlot[uint64, struct{}], *QuerySlot[uint64]) {
	t.Helper()
	builder := NewSchema()
	first, second := coldKey(940_001), coldKey(940_002)
	var input, output *FactorSlot[uint64]
	if reverse {
		output, _ = DeclareFactorSlot[uint64](builder, second)
		input, _ = DeclareFactorSlot[uint64](builder, first)
	} else {
		input, _ = DeclareFactorSlot[uint64](builder, first)
		output, _ = DeclareFactorSlot[uint64](builder, second)
	}
	inputRead, inputOK := input.ExactRead()
	outputWrite, outputOK := output.ExactWrite()
	if !inputOK || !outputOK {
		t.Fatal("factor forms")
	}
	rule, ok := DeclareRuleSlot[uint64, struct{}](builder, SchemaRuleSpec[uint64]{Semantic: coldKey(940_003), OperandFamily: coldKey(940_004), Inputs: 1, Admission: SchemaAdmission{Basis: RuleAdmissionBasisTrustedTheorem, Identity: coldKey(940_005)}, Output: output.Ref()})
	if !ok {
		t.Fatal("rule")
	}
	in, ok := rule.Input(0)
	if !ok {
		t.Fatal("input")
	}
	if _, ok = SchemaRead[uint64](rule, inputRead, in); !ok {
		t.Fatal("read")
	}
	if routed {
		// Route writes require a selected read of the output and one prior read
		// dependency; this fixture is intentionally minimal but valid.
		_ = outputWrite
	}
	if _, ok = SchemaWrite[uint64](rule, outputWrite); !ok {
		t.Fatal("write")
	}
	query, ok := DeclareQuerySlot[uint64](builder, SchemaQuerySpec{Semantic: coldKey(940_006), Freezer: coldKey(940_007)})
	if !ok || !SchemaQueryRead[uint64](query, inputRead) {
		t.Fatal("query")
	}
	return builder, output, inputRead, outputWrite, rule, query
}

func TestSchemaSlotsAreCallbackAndKeyDomainFree(t *testing.T) {
	builder := NewSchema()
	factor, ok := DeclareFactorSlot[struct{}](builder, coldKey(941_001))
	if !ok || factor == nil {
		t.Fatal("factor")
	}
	if _, ok := factor.ExactRead(); !ok {
		t.Fatal("exact form")
	}
	if builder.candidate.Factors[0].Forms != nil {
		t.Fatal("exact form entered cold schema")
	}
}

func TestSchemaSlotsCanonicalPermutationAndCopiedOwner(t *testing.T) {
	left, leftFactor, _, _, _, _ := schemaSlotFixture(t, false, false)
	right, _, _, _, _, _ := schemaSlotFixture(t, false, true)
	copyOfFactor := *leftFactor
	leftSchema, leftOK := left.Seal()
	rightSchema, rightOK := right.Seal()
	if !leftOK || !rightOK || leftSchema.ID() != rightSchema.ID() || copyOfFactor.Schema() != leftSchema {
		t.Fatal("permutation or copied token owner")
	}
	if ordinal, ok := copyOfFactor.Ordinal(); !ok || ordinal != 1 {
		t.Fatal("canonical factor ordinal")
	}
}

func TestSchemaSlotsRejectForeignEqualCandidateAndPoison(t *testing.T) {
	foreign := NewSchema()
	foreignFactor, ok := DeclareFactorSlot[uint64](foreign, coldKey(942_001))
	if !ok {
		t.Fatal("foreign factor")
	}
	owner := NewSchema()
	_, declared := DeclareRuleSlot[uint64, struct{}](owner, SchemaRuleSpec[uint64]{Semantic: coldKey(942_002), OperandFamily: coldKey(942_003), Admission: SchemaAdmission{Basis: RuleAdmissionBasisTrustedTheorem, Identity: coldKey(942_004)}, Output: foreignFactor.Ref()})
	sealed, _ := owner.Seal()
	if declared || sealed != nil {
		t.Fatal("foreign factor accepted")
	}
	if _, ok := DeclareFactorSlot[uint64](owner, coldKey(942_005)); ok {
		t.Fatal("poisoned candidate accepted declaration")
	}
}

func TestSchemaSlotsRejectDuplicateSemanticClaimsAtIssuance(t *testing.T) {
	builder := NewSchema()
	semantic := coldKey(942_100)
	factor, ok := DeclareFactorSlot[uint64](builder, semantic)
	if !ok {
		t.Fatal("factor")
	}
	if _, ok := factor.SummaryRead(semantic); ok || builder.phase != schemaBuilderPoisoned || factor.Available() {
		t.Fatal("factor semantic was reclaimed by a summary form")
	}

	builder = NewSchema()
	if _, ok := DeclareFactorSlot[uint64](builder, semantic); !ok {
		t.Fatal("first factor")
	}
	if _, ok := DeclareFactorSlot[uint64](builder, semantic); ok || builder.phase != schemaBuilderPoisoned {
		t.Fatal("duplicate factor semantic was provisionally admitted")
	}
}

func TestSchemaSlotsExtensionPresenceAcceptsCanonicalOrdinalZero(t *testing.T) {
	builder := NewSchema()
	factor, ok := DeclareFactorSlot[uint64](builder, coldKey(942_300))
	if !ok {
		t.Fatal("factor")
	}
	extension, ok := factor.SummaryRead(coldKey(942_301))
	if !ok {
		t.Fatal("summary extension")
	}
	write, ok := factor.ExactWrite()
	if !ok {
		t.Fatal("write")
	}
	rule, ok := DeclareRuleSlot[uint64, struct{}](builder, SchemaRuleSpec[uint64]{
		Semantic: coldKey(942_302), OperandFamily: coldKey(942_303),
		Inputs: 1, Admission: SchemaAdmission{Basis: RuleAdmissionBasisTrustedTheorem, Identity: coldKey(942_304)}, Output: factor.Ref(),
	})
	if !ok {
		t.Fatal("rule")
	}
	in, ok := rule.Input(0)
	if !ok {
		t.Fatal("input")
	}
	if _, ok := SchemaRead(rule, extension, in); !ok {
		t.Fatal("summary read")
	}
	if _, ok := SchemaWrite(rule, write); !ok {
		t.Fatal("write")
	}
	query, ok := DeclareQuerySlot[uint64](builder, SchemaQuerySpec{Semantic: coldKey(942_305), Freezer: coldKey(942_306)})
	if !ok || !SchemaQueryRead(query, extension) {
		t.Fatal("query")
	}
	schema, ok := builder.Seal()
	if !ok || extension.Schema() != schema {
		t.Fatal("extension ordinal zero was treated as absent")
	}
	// The implementation uses an explicit found bit for extension lookup;
	// the zero-index case is the smallest hostile witness for that contract.
}

func TestSchemaSlotsFailedSealIsAtomicAndPostSealIsClosed(t *testing.T) {
	builder, factor, _, _, _, _ := schemaSlotFixture(t, false, false)
	factorCopy := *factor
	builder.candidate.Rules[0].Writes = nil
	if schema, ok := builder.Seal(); ok || schema != nil || factor.Schema() != nil || factorCopy.Schema() != nil {
		t.Fatal("failed seal exposed partial schema binding")
	}

	builder, factor, _, _, _, _ = schemaSlotFixture(t, false, false)
	if schema, ok := builder.Seal(); !ok || schema == nil {
		t.Fatal("seal")
	}
	if _, ok := DeclareFactorSlot[uint64](builder, coldKey(943_001)); ok {
		t.Fatal("post-seal factor declaration")
	}
	if _, ok := factor.ExactRead(); ok {
		t.Fatal("post-seal form declaration")
	}
}

func TestSchemaSlotsExactKindsAndParentFence(t *testing.T) {
	builder := NewSchema()
	factor, _ := DeclareFactorSlot[uint64](builder, coldKey(944_001))
	read, _ := factor.ExactRead()
	write, _ := factor.ExactWrite()
	if read.Kind() == write.Kind() {
		t.Fatal("exact read/write kinds aliased")
	}
	ruleA, _ := DeclareRuleSlot[uint64, struct{}](builder, SchemaRuleSpec[uint64]{Semantic: coldKey(944_002), OperandFamily: coldKey(944_003), Inputs: 1, Admission: SchemaAdmission{Basis: RuleAdmissionBasisTrustedTheorem, Identity: coldKey(944_004)}, Output: factor.Ref()})
	ruleB, _ := DeclareRuleSlot[uint64, struct{}](builder, SchemaRuleSpec[uint64]{Semantic: coldKey(944_005), OperandFamily: coldKey(944_006), Inputs: 1, Admission: SchemaAdmission{Basis: RuleAdmissionBasisTrustedTheorem, Identity: coldKey(944_007)}, Output: factor.Ref()})
	inA, _ := ruleA.Input(0)
	inB, _ := ruleB.Input(0)
	readA, _ := SchemaRead[uint64](ruleA, read, inA)
	if _, ok := SchemaRead[uint64](ruleB, read, inB); !ok {
		t.Fatal("same form should work for second rule")
	}
	if _, ok := SchemaSelectedRead[uint64](ruleB, read, inB, readA.Ref()); ok {
		t.Fatal("foreign parent read accepted")
	}
}

func TestSchemaSlotsRouteAndSelectorPredecessorsAreExactAndCanonical(t *testing.T) {
	builder := NewSchema()
	factor, _ := DeclareFactorSlot[uint64](builder, coldKey(944_100))
	read, _ := factor.ExactRead()
	write, _ := factor.ExactWrite()
	rule, _ := DeclareRuleSlot[uint64, struct{}](builder, SchemaRuleSpec[uint64]{
		Semantic: coldKey(944_101), OperandFamily: coldKey(944_102), Inputs: 1,
		Admission: SchemaAdmission{Basis: RuleAdmissionBasisTrustedTheorem, Identity: coldKey(944_103)}, Output: factor.Ref(),
	})
	if _, ok := factor.Ordinal(); ok {
		t.Fatal("factor exposed provisional ordinal")
	}
	if _, ok := rule.Ordinal(); ok {
		t.Fatal("rule exposed provisional ordinal")
	}
	in, _ := rule.Input(0)
	base, ok := SchemaRead[uint64](rule, read, in)
	if !ok {
		t.Fatal("base read")
	}
	if _, ok := SchemaSelectedRead[uint64](rule, read, in, base.Ref(), base.Ref()); ok {
		t.Fatal("duplicate selected predecessor mutated rule")
	}
	selected, ok := SchemaSelectedRead[uint64](rule, read, in, base.Ref())
	if !ok {
		t.Fatal("selected read")
	}
	routed, ok := SchemaRouteWrite(rule, write, selected)
	if !ok || routed.cell == nil {
		t.Fatal("routed write")
	}
	row := builder.candidate.Rules[0].Writes[0]
	if row.Kind != coldcomposition.WriteRoute || row.Route != 2 {
		t.Fatal("route did not retain selected-read identity delta")
	}
	if _, ok := SchemaRouteWrite(rule, write, selected); ok {
		t.Fatal("second route accepted")
	}
	if _, ok := SchemaWrite(rule, write); ok {
		t.Fatal("static write after routed disposition accepted")
	}
	if _, ok := SchemaRouteWrite(rule, write, base); ok {
		t.Fatal("non-selected route accepted")
	}

	support, ok := DeclareSchemaSupportQuery[bool](builder, SchemaQuerySpec{Semantic: coldKey(944_104), Freezer: coldKey(944_105)})
	if !ok || SchemaQueryRead(support, read) {
		t.Fatal("support query accepted factor read")
	}
}

func TestSchemaSlotsRejectRouteAfterStaticWrite(t *testing.T) {
	builder := NewSchema()
	factor, _ := DeclareFactorSlot[uint64](builder, coldKey(944_200))
	read, _ := factor.ExactRead()
	write, _ := factor.ExactWrite()
	rule, _ := DeclareRuleSlot[uint64, struct{}](builder, SchemaRuleSpec[uint64]{
		Semantic: coldKey(944_202), OperandFamily: coldKey(944_203), Inputs: 1,
		Admission: SchemaAdmission{Basis: RuleAdmissionBasisTrustedTheorem, Identity: coldKey(944_204)}, Output: factor.Ref(),
	})
	input, _ := rule.Input(0)
	base, _ := SchemaRead(rule, read, input)
	selected, _ := SchemaSelectedRead[uint64](rule, read, input, base.Ref())
	if _, ok := SchemaWrite(rule, write); !ok {
		t.Fatal("static write")
	}
	if _, ok := SchemaRouteWrite(rule, write, selected); ok {
		t.Fatal("routed write accepted a competing static write")
	}
}

func TestSchemaSlotsRouteRequiresSelectedOutputFactor(t *testing.T) {
	builder := NewSchema()
	output, _ := DeclareFactorSlot[uint64](builder, coldKey(944_300))
	other, _ := DeclareFactorSlot[uint64](builder, coldKey(944_301))
	outputWrite, _ := output.ExactWrite()
	otherRead, _ := other.ExactRead()
	rule, _ := DeclareRuleSlot[uint64, struct{}](builder, SchemaRuleSpec[uint64]{
		Semantic: coldKey(944_302), OperandFamily: coldKey(944_303), Inputs: 1,
		Admission: SchemaAdmission{Basis: RuleAdmissionBasisTrustedTheorem, Identity: coldKey(944_304)}, Output: output.Ref(),
	})
	input, _ := rule.Input(0)
	base, _ := SchemaRead(rule, otherRead, input)
	selected, _ := SchemaSelectedRead[uint64](rule, otherRead, input, base.Ref())
	if _, ok := SchemaRouteWrite(rule, outputWrite, selected); ok {
		t.Fatal("routed write accepted a selected read from a different factor")
	}
}

func TestSchemaSlotsOrdinaryCarryRetainsZeroTransform(t *testing.T) {
	builder, output, _, _, rule, _ := schemaSlotFixture(t, false, false)
	input, inputOK := rule.Input(0)
	carry, carryOK := SchemaCarryFrom(rule, input, output.Ref())
	if !inputOK || !carryOK || carry.cell == nil {
		t.Fatal("ordinary carry")
	}
	rows := builder.candidate.Rules[0].Carries
	if len(rows) != 1 || rows[0].Transform.Available() {
		t.Fatal("ordinary carry acquired a transformed semantic identity")
	}
	if _, ok := SchemaCarryFrom(rule, input, output.Ref()); ok {
		t.Fatal("second ordinary carry accepted")
	}
	schema, ok := builder.Seal()
	if !ok || schema == nil || carry.Schema() != schema {
		t.Fatal("ordinary carry did not bind through the sealed schema")
	}
}

func TestSchemaSlotsStructuralCapabilitiesBind(t *testing.T) {
	builder := NewSchema()
	completion, ok := DeclareSchemaCompletion(builder, coldKey(945_001))
	if !ok {
		t.Fatal("completion")
	}
	prune, ok := completion.Prune(coldKey(945_002))
	if !ok {
		t.Fatal("prune")
	}
	rule, ok := DeclareSchemaSupportRule(builder, SchemaStructuralRuleSpec{Semantic: coldKey(945_003), Admission: SchemaAdmission{Basis: RuleAdmissionBasisTrustedTheorem, Identity: coldKey(945_004)}, Completion: completion, Prune: prune})
	if !ok || rule == nil {
		t.Fatal("support rule")
	}
	query, ok := DeclareSchemaSupportQuery[bool](builder, SchemaQuerySpec{Semantic: coldKey(945_005), Freezer: coldKey(945_006)})
	if !ok || query == nil {
		t.Fatal("support query")
	}
	familyBuilder := NewSchema()
	activationFactor, ok := DeclareFactorSlot[uint64](familyBuilder, coldKey(945_012))
	if !ok {
		t.Fatal("activation factor")
	}
	activationRead, ok := activationFactor.ExactRead()
	if !ok {
		t.Fatal("activation read")
	}
	family, ok := DeclareSchemaActivationFamily(familyBuilder, coldKey(945_007))
	if !ok {
		t.Fatal("family")
	}
	if _, ok := DeclareSchemaActivationRule(familyBuilder, SchemaStructuralRuleSpec{Semantic: coldKey(945_008), Admission: SchemaAdmission{Basis: RuleAdmissionBasisTrustedTheorem, Identity: coldKey(945_009)}, Activation: family}); !ok {
		t.Fatal("activation rule")
	}
	if rows := familyBuilder.candidate.Rules; len(rows) != 1 || rows[0].OperandFamily != compositionKeyOf(unitOperandFamily) {
		t.Fatal("activation rule did not retain the engine unit operand family")
	}
	activationQuery, ok := DeclareQuerySlot[bool](familyBuilder, SchemaQuerySpec{Semantic: coldKey(945_010), Freezer: coldKey(945_011)})
	if !ok || !SchemaQueryRead(activationQuery, activationRead) {
		t.Fatal("activation query")
	}
	schema, ok := builder.Seal()
	if !ok || completion.Schema() != schema || prune.Schema() != schema {
		t.Fatal("structural owner binding")
	}
	if familySchema, ok := familyBuilder.Seal(); !ok || family.Schema() != familySchema {
		t.Fatal("activation owner binding")
	}
}

// TestSchemaSlotsPortIdentityIsItsIndex proves both Rule kinds issue ordered
// predecessor ports through one implementation: a port redeclared at the same
// index is the same port and never a second token, and two Rules never share
// one port.
func TestSchemaSlotsPortIdentityIsItsIndex(t *testing.T) {
	builder := NewSchema()
	factor, factorOK := DeclareFactorSlot[uint64](builder, coldKey(946_001))
	if !factorOK {
		t.Fatal("factor")
	}
	rule, ruleOK := DeclareRuleSlot[uint64, struct{}](builder, SchemaRuleSpec[uint64]{
		Semantic: coldKey(946_002), OperandFamily: coldKey(946_003), Inputs: 2,
		Admission: SchemaAdmission{Basis: RuleAdmissionBasisTrustedTheorem, Identity: coldKey(946_004)}, Output: factor.Ref(),
	})
	if !ruleOK {
		t.Fatal("rule")
	}
	first, firstOK := rule.Input(1)
	again, againOK := rule.Input(1)
	if !firstOK || !againOK || first.cell == nil || first.cell != again.cell {
		t.Fatal("a redeclared factor-output port issued a second token")
	}
	if _, ok := rule.Input(2); ok {
		t.Fatal("port beyond the declared arity")
	}
	family, familyOK := DeclareSchemaActivationFamily(builder, coldKey(946_005))
	if !familyOK {
		t.Fatal("activation family")
	}
	structural, structuralOK := DeclareSchemaActivationRule(builder, SchemaStructuralRuleSpec{
		Semantic: coldKey(946_006), Inputs: 2,
		Admission: SchemaAdmission{Basis: RuleAdmissionBasisTrustedTheorem, Identity: coldKey(946_007)}, Activation: family,
	})
	if !structuralOK {
		t.Fatal("activation rule")
	}
	structuralFirst, structuralFirstOK := structural.Input(1)
	structuralAgain, structuralAgainOK := structural.Input(1)
	if !structuralFirstOK || !structuralAgainOK || structuralFirst.cell == nil || structuralFirst.cell != structuralAgain.cell {
		t.Fatal("a redeclared structural port issued a second token")
	}
	if structuralFirst.cell == first.cell {
		t.Fatal("two Rules shared one predecessor port token")
	}
	if _, ok := structural.Input(2); ok {
		t.Fatal("structural port beyond the declared arity")
	}
}

// TestSchemaSlotsRefuseCrossRoleTokens proves the declaration role is the
// authority for what a token addresses: a token issued for one role never
// resolves as another role's declaration row, even where the two rows carry
// the same declaration state.
func TestSchemaSlotsRefuseCrossRoleTokens(t *testing.T) {
	builder := NewSchema()
	factor, factorOK := DeclareFactorSlot[uint64](builder, coldKey(946_101))
	if !factorOK {
		t.Fatal("factor")
	}
	read, readOK := factor.ExactRead()
	write, writeOK := factor.ExactWrite()
	if !readOK || !writeOK {
		t.Fatal("forms")
	}
	family, familyOK := DeclareSchemaActivationFamily(builder, coldKey(946_102))
	if !familyOK {
		t.Fatal("activation family")
	}
	rule, ruleOK := DeclareRuleSlot[uint64, struct{}](builder, SchemaRuleSpec[uint64]{
		Semantic: coldKey(946_103), OperandFamily: coldKey(946_104), Inputs: 1,
		Admission: SchemaAdmission{Basis: RuleAdmissionBasisTrustedTheorem, Identity: coldKey(946_105)}, Output: factor.Ref(),
	})
	if !ruleOK {
		t.Fatal("rule")
	}
	input, inputOK := rule.Input(0)
	if !inputOK {
		t.Fatal("input")
	}
	if _, ok := SchemaRead(rule, read, input); !ok {
		t.Fatal("base read")
	}
	written, writtenOK := SchemaWrite(rule, write)
	if !writtenOK {
		t.Fatal("write")
	}
	writeAsRead := SchemaReadRef{slotHandle[rowDraft[readRole]]{cell: written.cell}}
	if _, ok := SchemaSelectedRead[uint64](rule, read, input, writeAsRead); ok {
		t.Fatal("a write row was accepted as a read predecessor")
	}
	familyAsFactor := FactorRef[uint64]{slotHandle[keyDraft[factorRole]]{cell: family.cell}}
	if _, ok := DeclareRuleSlot[uint64, struct{}](builder, SchemaRuleSpec[uint64]{
		Semantic: coldKey(946_106), OperandFamily: coldKey(946_107), Inputs: 1,
		Admission: SchemaAdmission{Basis: RuleAdmissionBasisTrustedTheorem, Identity: coldKey(946_108)}, Output: familyAsFactor,
	}); ok {
		t.Fatal("an activation family was accepted as a Factor output")
	}
}

// TestSchemaSlotsRefusedDeclarationsEmitNoRow proves each declaration emits its
// cold row complete and final: a refused declaration leaves the candidate
// exactly as it found it, and an accepted one carries its whole disposition on
// the row it appended.
func TestSchemaSlotsRefusedDeclarationsEmitNoRow(t *testing.T) {
	builder := NewSchema()
	factor, factorOK := DeclareFactorSlot[uint64](builder, coldKey(946_201))
	if !factorOK {
		t.Fatal("factor")
	}
	exactRead, exactReadOK := factor.ExactRead()
	summaryRead, summaryReadOK := factor.SummaryRead(coldKey(946_202))
	exactWrite, exactWriteOK := factor.ExactWrite()
	if !exactReadOK || !summaryReadOK || !exactWriteOK {
		t.Fatal("forms")
	}
	rule, ruleOK := DeclareRuleSlot[uint64, struct{}](builder, SchemaRuleSpec[uint64]{
		Semantic: coldKey(946_204), OperandFamily: coldKey(946_205), Inputs: 1,
		Admission: SchemaAdmission{Basis: RuleAdmissionBasisTrustedTheorem, Identity: coldKey(946_206)}, Output: factor.Ref(),
	})
	if !ruleOK {
		t.Fatal("rule")
	}
	input, inputOK := rule.Input(0)
	if !inputOK {
		t.Fatal("input")
	}
	base, baseOK := SchemaRead(rule, exactRead, input)
	if !baseOK {
		t.Fatal("base read")
	}
	reads, writes := len(builder.candidate.Rules[0].Reads), len(builder.candidate.Rules[0].Writes)
	if _, ok := SchemaSelectedRead[uint64](rule, summaryRead, input, base.Ref()); ok {
		t.Fatal("a summary form was accepted as a selected read")
	}
	if got := len(builder.candidate.Rules[0].Reads); got != reads {
		t.Fatalf("refused selected read emitted %d read rows", got-reads)
	}
	selected, selectedOK := SchemaSelectedRead[uint64](rule, exactRead, input, base.Ref())
	if !selectedOK {
		t.Fatal("selected read")
	}
	row := builder.candidate.Rules[0].Reads[reads]
	if row.Kind != coldcomposition.ReadSelect || row.Semantic != row.Factor || len(row.Dependencies) != 1 || row.Dependencies[0] != 0 {
		t.Fatal("the selected read row was not emitted complete")
	}
	if _, ok := SchemaRouteWrite(rule, exactWrite, base); ok {
		t.Fatal("a non-selected read was accepted as a route")
	}
	if got := len(builder.candidate.Rules[0].Writes); got != writes {
		t.Fatalf("refused routed write emitted %d write rows", got-writes)
	}
	routed, routedOK := SchemaRouteWrite(rule, exactWrite, selected)
	if !routedOK {
		t.Fatal("routed write")
	}
	written := builder.candidate.Rules[0].Writes[writes]
	if written.Kind != coldcomposition.WriteRoute || written.Route != uint64(reads+1) || routed.cell == nil {
		t.Fatal("the routed write row was not emitted complete")
	}
}

func factorOnlySlotSchema(t testing.TB, semantic identity.SemanticKey) (*Schema, *FactorSlot[uint64]) {
	t.Helper()
	builder := NewSchema()
	factor, factorOK := DeclareFactorSlot[uint64](builder, semantic)
	schema, schemaOK := builder.Seal()
	if !factorOK || !schemaOK || schema == nil || factor == nil || factor.Schema() != schema {
		t.Fatal("factor-only schema")
	}
	return schema, factor
}

func hotUintFactorSpec() HotFactorSpec[uint64, uint64] {
	return HotFactorSpec[uint64, uint64]{
		KeyEnd: 2, Lattice: coldUintLattice(), Default: 0,
		AdmitAt:     func(_ uint64, _ uint64) bool { return true },
		Fingerprint: func(value uint64) uint64 { return value },
	}
}

func TestSchemaBindingCopiesShareOneTerminalLifecycle(t *testing.T) {
	schema, factor := factorOnlySlotSchema(t, coldKey(946_001))
	binding := NewSchemaBinding(schema)
	if binding == nil || binding.Sealed() || binding.Schema() != nil {
		t.Fatal("open factor binding")
	}
	copyOfBinding := *binding
	if !BindFactor(binding, factor, hotUintFactorSpec()) || !copyOfBinding.Seal() {
		t.Fatal("shared binding publication")
	}
	implementation, ok := FactorImplementationAt[uint64, uint64](&copyOfBinding, factor)
	if !ok || implementation == nil || implementation.algebra == nil || binding.Schema() != schema || !binding.Sealed() {
		t.Fatal("published factor implementation")
	}
	if binding.Seal() || BindFactor(binding, factor, hotUintFactorSpec()) {
		t.Fatal("terminal binding admitted a second transition")
	}
}

func TestSchemaBindingRejectsForeignSlotAndIncompleteInventory(t *testing.T) {
	schema, factor := factorOnlySlotSchema(t, coldKey(946_010))
	_, foreign := factorOnlySlotSchema(t, coldKey(946_010))
	binding := NewSchemaBinding(schema)
	copyOfBinding := *binding
	if BindFactor(binding, foreign, hotUintFactorSpec()) || !binding.Poisoned() || !copyOfBinding.Poisoned() {
		t.Fatal("foreign equal-schema slot crossed the shared owner fence")
	}
	if BindFactor(&copyOfBinding, factor, hotUintFactorSpec()) || copyOfBinding.Seal() {
		t.Fatal("poisoned binding recovered")
	}

	schema, _ = factorOnlySlotSchema(t, coldKey(946_011))
	binding = NewSchemaBinding(schema)
	if binding.Seal() || !binding.Poisoned() {
		t.Fatal("missing factor implementation published")
	}
}

func TestSchemaBindingRetainsRicherSchemaUntilFullInventory(t *testing.T) {
	builder := NewSchema()
	factor, _ := DeclareFactorSlot[uint64](builder, coldKey(946_020))
	summary, summaryOK := factor.SummaryRead(coldKey(946_021))
	if !summaryOK {
		t.Fatal("summary form")
	}
	schema, ok := builder.Seal()
	if !ok || schema == nil {
		t.Fatal("summary schema")
	}
	binding := NewSchemaBinding(schema)
	identity := func(cells OrderedCells[uint64]) OrderedCells[uint64] { return cells }
	if binding == nil || !BindFactor(binding, factor, hotUintFactorSpec()) ||
		!BindSummaryReadForFactor[uint64](binding, factor, summary, identity,
			func(left, right OrderedCells[uint64]) bool { return len(left.record.cells) == len(right.record.cells) },
			func(value OrderedCells[uint64]) uint64 { return uint64(len(value.record.cells)) }) ||
		!binding.Seal() || binding.Schema() != schema {
		t.Fatal("complete Factor extension inventory did not publish")
	}

	rich := NewSchema()
	richFactor, _ := DeclareFactorSlot[uint64](rich, coldKey(946_030))
	richWrite, richWriteOK := richFactor.ExactWrite()
	rule, ruleOK := NewRuleSlot[uint64, uint64](rich, SchemaRuleSpec[uint64]{
		Semantic: coldKey(946_031), OperandFamily: coldKey(946_032),
		Admission: SchemaAdmission{Basis: RuleAdmissionBasisTrustedTheorem, Identity: coldKey(946_033)},
		Output:    richFactor.Ref(),
	})
	_, ruleWriteOK := SchemaWrite(rule, richWrite)
	if !richWriteOK || !ruleOK || rule == nil || !ruleWriteOK {
		t.Fatal("rich schema Rule")
	}
	richSchema, richOK := rich.Seal()
	richBinding := NewSchemaBinding(richSchema)
	if !richOK || richBinding == nil || !BindFactor(richBinding, richFactor, hotUintFactorSpec()) || richBinding.Seal() || !richBinding.Poisoned() {
		t.Fatal("Rule-bearing schema published before its full hot inventory")
	}
}

// TestSummaryReadFoldIsSealedIntoTheDeclaredForm proves the coordinate-wise
// fold is a property of the declared cold read form. Two summary forms over
// the same Factor carry different cold rows, and the fold is recovered from
// the sealed schema alone: no Rule, Query, or observation supplies it.
func TestSummaryReadFoldIsSealedIntoTheDeclaredForm(t *testing.T) {
	builder := NewSchema()
	factor, factorOK := DeclareFactorSlot[uint64](builder, coldKey(948_001))
	correlated, correlatedOK := factor.SummaryRead(coldKey(948_002))
	distributive, distributiveOK := factor.DistributiveSummaryRead(coldKey(948_003))
	schema, schemaOK := builder.Seal()
	if !factorOK || !correlatedOK || !distributiveOK || !schemaOK || schema == nil {
		t.Fatal("two-fold summary schema")
	}
	if correlated.Kind() != SchemaFormReadSummary || distributive.Kind() != SchemaFormReadDistributiveSummary {
		t.Fatalf("declared form kinds = %d/%d", correlated.Kind(), distributive.Kind())
	}
	ordinal, ordinalOK := factor.Ordinal()
	if !ordinalOK {
		t.Fatal("factor ordinal")
	}
	count, countOK := schema.factorFormCount(ordinal)
	if !countOK || count != 2 {
		t.Fatalf("declared cold form rows = %d, want 2", count)
	}
	kinds := map[composition.Key]composition.FactorFormKind{}
	for index := 0; index < count; index++ {
		shape, shapeOK := schema.factorFormShapeAt(ordinal, uint64(index))
		if !shapeOK {
			t.Fatal("cold form shape")
		}
		kinds[shape.Semantic] = shape.Kind
	}
	if kinds[compositionKeyOf(coldKey(948_002))] != composition.FactorSummaryRead {
		t.Fatal("correlated summary lost its cold row kind")
	}
	if kinds[compositionKeyOf(coldKey(948_003))] != composition.FactorDistributiveSummaryRead {
		t.Fatal("distributive summary lost its cold row kind")
	}

	correlatedFold, correlatedFoldOK := summaryReadFormFold(schema, ordinal, compositionKeyOf(coldKey(948_002)))
	distributiveFold, distributiveFoldOK := summaryReadFormFold(schema, ordinal, compositionKeyOf(coldKey(948_003)))
	if !correlatedFoldOK || correlatedFold || !distributiveFoldOK || !distributiveFold {
		t.Fatalf("recovered folds = %t/%t (%t/%t)", correlatedFold, distributiveFold, correlatedFoldOK, distributiveFoldOK)
	}
	if _, ok := summaryReadFormFold(schema, ordinal, compositionKeyOf(coldKey(948_004))); ok {
		t.Fatal("undeclared normalizer resolved a fold")
	}
}

// TestSummaryReadFoldsAreDistinctDeclaredSemantics proves the second fold is a
// distinct declaration rather than a re-reading of the first: one semantic key
// names exactly one form and one fold.
func TestSummaryReadFoldsAreDistinctDeclaredSemantics(t *testing.T) {
	builder := NewSchema()
	factor, factorOK := DeclareFactorSlot[uint64](builder, coldKey(948_010))
	_, correlatedOK := factor.SummaryRead(coldKey(948_011))
	if !factorOK || !correlatedOK {
		t.Fatal("correlated summary form")
	}
	if _, ok := factor.DistributiveSummaryRead(coldKey(948_011)); ok {
		t.Fatal("one semantic key declared both folds")
	}
	if _, ok := builder.Seal(); ok {
		t.Fatal("poisoned builder sealed")
	}
}
