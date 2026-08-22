// sealed_availability_law_test.go states the sealing law for the engine's
// construction-time verdicts: a value that a constructor authenticated carries
// that verdict as a field, and its accessors read it. Re-deriving the verdict
// on every read would make every accessor a second authority over the same
// question, and the two authorities could disagree.

package engine

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
)

// sealedAvailabilityBinding seals one minimal binding: a single Factor, the
// mounted Rule that writes it, and the registered slot capability.
func sealedAvailabilityBinding(t testing.TB) (*SchemaBinding, *Schema) {
	t.Helper()
	builder := NewSchema()
	factor, factorOK := DeclareFactorSlot[uint64](builder, coldKey(998_000))
	write, writeOK := factor.ExactWrite()
	if !factorOK || !writeOK {
		t.Fatal("factor declaration")
	}
	rule, ruleOK := DeclareRuleSlot[uint64, ruleUnit](builder, SchemaRuleSpec[uint64]{
		Semantic: coldKey(998_100), OperandFamily: unitOperandFamily, Inputs: 0, Output: factor.Ref(),
	})
	slot, slotOK := SchemaWrite(rule, write)
	schema, schemaOK := builder.Seal()
	if !ruleOK || !slotOK || !schemaOK || schema == nil {
		t.Fatal("schema seal")
	}
	binding := NewSchemaBinding(schema)
	spec := HotRuleSpec[uint64, ruleUnit]{
		OperandContent:  ruleUnitContent,
		OperandResolver: func(OperandCoords) (ruleUnit, bool) { return ruleUnitForSemantic(coldKey(998_100)), true },
		Fold:            func(frame Frame[uint64, ruleUnit]) RuleResult[uint64] { return Staged(frame, uint64(1)) },
	}
	capability, capabilityOK := IssueMountedRuleCapability(binding, rule)
	if !BindFactor(binding, factor, hotUintFactorSpec()) ||
		!BindRule[uint64, uint64, ruleUnit](binding, rule, slot, factor, spec, testRuleProjector[ruleUnit]) ||
		!capabilityOK || !RegisterRuleSlot(binding, rule, capability) || !binding.Seal() {
		t.Fatal("binding seal")
	}
	return binding, schema
}

// TestSchemaAvailabilityIsSealedAtTheBuilderSeal pins that a sealed Schema
// carries the builder's verdict. The cold composition and the composition
// identity are proved once, where the composition is canonicalized; an
// accessor that re-proved them would answer from the schema's own fields
// rather than from the seal that issued them.
func TestSchemaAvailabilityIsSealedAtTheBuilderSeal(t *testing.T) {
	_, schema := sealedAvailabilityBinding(t)
	if !schema.Available() {
		t.Fatal("sealed schema unavailable")
	}
	detached := *schema
	detached.cold = nil
	detached.id = CompositionID{}
	if !(&detached).Available() {
		t.Fatal("Available re-derives the cold composition instead of reading the sealed verdict")
	}
	if allocs := testing.AllocsPerRun(100, func() { _ = schema.Available() }); allocs != 0 {
		t.Fatalf("Schema.Available allocates %v per call", allocs)
	}
	var absent *Schema
	if absent.Available() || (&Schema{}).Available() {
		t.Fatal("unsealed schema available")
	}
	if (&Schema{}).ID().Available() {
		t.Fatal("unsealed schema published a composition identity")
	}
}

// TestSealedBindingRetainsItsAuthority pins the invariant the sealed-phase
// readers depend on: a binding reaches the sealed phase only through Seal,
// which refuses without an authority, and poisoning cannot follow a seal.
// Sealed therefore names one fact, and no reader restates it as a second one.
func TestSealedBindingRetainsItsAuthority(t *testing.T) {
	binding, schema := sealedAvailabilityBinding(t)
	if !binding.Sealed() || binding.Poisoned() {
		t.Fatal("binding did not seal")
	}
	state := bindingState(binding)
	if state == nil || state.authority == nil || state.schema != schema {
		t.Fatal("a sealed binding lost the authority Seal proved")
	}
	state.poisonLocked()
	if !binding.Sealed() || binding.Poisoned() || bindingState(binding).authority == nil {
		t.Fatal("a sealed binding was poisoned after its seal")
	}
	if binding.Schema() != schema {
		t.Fatal("a sealed binding stopped publishing its schema")
	}
	open := NewSchemaBinding(schema)
	if open.Sealed() {
		t.Fatal("an open binding reports sealed")
	}
}

// TestFactorCapabilityAvailabilityIsSealedAtIssuance pins that the Factor row
// geometry is proved once, by the issuer, against a binding that is already
// sealed. A sealed binding is terminal, so nothing the issuer proved can move
// afterwards and the capability reads its own verdict.
func TestFactorCapabilityAvailabilityIsSealedAtIssuance(t *testing.T) {
	binding, _ := sealedAvailabilityBinding(t)
	capability, capabilityOK := FactorCapabilityForSemantic(binding, coldKey(998_000))
	if !capabilityOK || !capability.Available() {
		t.Fatal("issued factor capability unavailable")
	}
	detached := capability
	detached.state = nil
	detached.authority = nil
	if !detached.Available() {
		t.Fatal("Available re-walks the binding rows instead of reading the sealed verdict")
	}
	if allocs := testing.AllocsPerRun(100, func() { _ = capability.Available() }); allocs != 0 {
		t.Fatalf("FactorSlotCapability.Available allocates %v per call", allocs)
	}
	if (FactorSlotCapability{}).Available() {
		t.Fatal("unissued capability available")
	}
	if _, ok := FactorCapabilityForSemantic(binding, coldKey(998_999)); ok {
		t.Fatal("a capability was issued for a semantic the schema never declared")
	}
	if refused, ok := FactorCapabilityForSemantic(binding, coldKey(998_999)); ok || refused.Available() {
		t.Fatal("a refused issuance still published an available capability")
	}
	if _, ok := FactorCapabilityForSemantic(NewSchemaBinding(nil), coldKey(998_000)); ok {
		t.Fatal("a capability was issued from an unsealed binding")
	}
}

// TestLinkBootstrapWitnessAvailabilityIsSealedAtConstruction pins that both
// witness constructors decide the seam once over their own arguments.
func TestLinkBootstrapWitnessAvailabilityIsSealedAtConstruction(t *testing.T) {
	owner := stageTotalityID(4_001)
	pointID := stageTotalityID(4_002)
	point := LinkBootstrapPoint{PointID: pointID, Known: true}
	witness, witnessOK := NewLinkBootstrapWitness(owner, point, nil)
	if !witnessOK || !witness.Available() {
		t.Fatal("issued bootstrap witness unavailable")
	}
	detached := witness
	detached.owner = identity.ContentID{}
	detached.point = LinkBootstrapPoint{}
	if !detached.Available() {
		t.Fatal("Available re-derives the seam instead of reading the sealed verdict")
	}
	if allocs := testing.AllocsPerRun(100, func() { _ = witness.Available() }); allocs != 0 {
		t.Fatalf("LinkBootstrapWitness.Available allocates %v per call", allocs)
	}
	if (LinkBootstrapWitness{}).Available() {
		t.Fatal("unissued witness available")
	}
	if refused, ok := NewLinkBootstrapWitness(identity.ContentID{}, point, nil); ok || refused.Available() {
		t.Fatal("a witness was issued without an owner")
	}
	if refused, ok := NewLinkBootstrapWitness(owner, LinkBootstrapPoint{PointID: pointID}, nil); ok || refused.Available() {
		t.Fatal("a witness was issued for an unknown bootstrap point")
	}
	if refused, ok := NewLinkBootstrapWitness(owner, LinkBootstrapPoint{Known: true}, nil); ok || refused.Available() {
		t.Fatal("a witness was issued without a bootstrap point identity")
	}
}

// TestNativeCallStageAvailabilityIsSealedAtLookup pins that the committed
// native-stage inverse is consulted once, by the lookup that issues the
// handle. A committed program never exchanges that map, so the accessors read
// the issued row rather than resolving the key again on every call.
func TestNativeCallStageAvailabilityIsSealedAtLookup(t *testing.T) {
	fixture := newSelectedOverlayLawFixtureWithOptions(t, selectedOverlayLawOptions{candidateCount: 0, nativeStage: true})
	if fixture.graph == nil {
		t.Fatal("native-staged program refused")
	}
	stage, stageOK := fixture.graph.MountedNativeCallStage(fixture.activationRole, fixture.activationMount, fixture.activationOccurrence)
	if !stageOK || !stage.Available() {
		t.Fatal("issued native call stage unavailable")
	}
	detached := stage
	detached.handle.program = nil
	if !detached.Available() || !detached.PointID().Available() || detached.Kind() != stage.Kind() {
		t.Fatal("the stage accessors resolve the committed map instead of reading the issued row")
	}
	if allocs := testing.AllocsPerRun(100, func() { _ = stage.Available() }); allocs != 0 {
		t.Fatalf("ProgramCallStage.Available allocates %v per call", allocs)
	}
	if (ProgramCallStage{}).Available() {
		t.Fatal("unissued stage available")
	}
	if refused, ok := fixture.graph.MountedNativeCallStage(fixture.activationRole, fixture.activationMount, stageTotalityID(4_100)); ok || refused.Available() {
		t.Fatal("a stage was issued for an occurrence the program never staged")
	}
	if refused, ok := fixture.graph.MountedNativeCallStage(RuleSlotCapability{}, fixture.activationMount, fixture.activationOccurrence); ok || refused.Available() {
		t.Fatal("a stage was issued without a mounted rule capability")
	}
}
