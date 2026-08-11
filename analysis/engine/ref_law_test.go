package engine

import (
	"reflect"
	"testing"
)

func TestFactorRefIssuesOnlySealedInRangeKeys(t *testing.T) {
	composition := NewComposition()
	spec := coldFactorSpec(coldKey(97_001))
	spec.KeyEnd = 2
	issuedDuringDeclaration := false
	factor, declared := DeclareFactor(composition, spec, func(factor *Factor[uint64, uint64]) bool {
		_, issuedDuringDeclaration = factor.Ref(0)
		return !issuedDuringDeclaration
	})
	if !declared || factor == nil {
		t.Fatal("Factor declaration")
	}
	if issuedDuringDeclaration {
		t.Fatal("Factor issued Ref while its declaration callback was open")
	}

	for _, key := range []uint64{0, 1, 2} {
		if _, issued := factor.Ref(key); issued {
			t.Fatalf("unsealed Factor issued key %d", key)
		}
	}

	read, readOK := ExactReadForm(factor)
	write, writeOK := ExactWriteForm(factor)
	rule, ruleOK := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent,
		Semantic:  coldKey(97_002),
		Output:    factor.Output(),
		Inputs:    0,
		Admission: testTrustedTheorem[uint64](97_102),
		Transfer:  func(Access[uint64, ruleUnit]) bool { return true },
	}, func(rule *Rule[uint64, ruleUnit]) bool {
		_, ok := WriteTo(rule, write)
		return ok
	})
	query, queryOK := declareColdQuery(composition, coldKey(97_003), coldKey(97_004), read)
	if !readOK || !writeOK || !ruleOK || rule == nil || !queryOK || query == nil || !composition.Seal() {
		t.Fatal("sealed Ref fixture declaration")
	}

	var first Ref[uint64]
	for _, key := range []uint64{0, 1} {
		ref, issued := factor.Ref(key)
		if !issued {
			t.Fatalf("sealed Factor rejected in-range key %d", key)
		}
		if key == 0 {
			first = ref
		}
		if reflect.TypeOf(ref).Comparable() {
			t.Fatal("Ref is comparable")
		}
	}
	if !validateRefForWaveE(factor, first) {
		t.Fatal("sealed Factor rejected its issued Ref")
	}
	if !validateRefForSchema(factor.schema, first) {
		t.Fatal("sealed Factor schema rejected its issued Ref")
	}
	if allocations := testing.AllocsPerRun(1_000, func() {
		ref, issued := factor.Ref(0)
		if !issued || !validateRefForWaveE(factor, ref) || !validateRefForSchema(factor.schema, ref) {
			t.Fatal("repeated sealed Ref validation")
		}
	}); allocations != 0 {
		t.Fatalf("sealed Ref issuance/validation allocations = %v, want 0", allocations)
	}
	copied := first
	if !validateRefForWaveE(factor, copied) {
		t.Fatal("sealed Factor rejected copied Ref")
	}
	if validateRefForWaveE(factor, Ref[uint64]{}) {
		t.Fatal("sealed Factor accepted zero Ref")
	}
	outOfRange := copied
	outOfRange.raw = 2
	if validateRefForWaveE(factor, outOfRange) {
		t.Fatal("sealed Factor accepted out-of-range Ref")
	}
	wrongBinding := copied
	wrongBinding.factorIndex++
	if validateRefForWaveE(factor, wrongBinding) {
		t.Fatal("sealed Factor accepted Ref with a foreign binding identity")
	}
	if _, issued := factor.Ref(2); issued {
		t.Fatal("sealed Factor issued key at KeyEnd")
	}
}

func TestRefWaveEValidationRejectsForeignFactorAndComposition(t *testing.T) {
	composition := NewComposition()
	left := coldFactor(composition, coldKey(97_101))
	right := coldFactor(composition, coldKey(97_201))
	if left == nil || right == nil {
		t.Fatal("Factor declarations")
	}
	declareRefFixtureMember(t, composition, left, 97_102)
	declareRefFixtureMember(t, composition, right, 97_202)
	if !composition.Seal() {
		t.Fatal("two-Factor Composition seal")
	}
	leftRef, issued := left.Ref(0)
	if !issued || !validateRefForWaveE(left, leftRef) {
		t.Fatal("left Factor Ref")
	}
	if validateRefForWaveE(right, leftRef) {
		t.Fatal("foreign Factor accepted same-key Ref")
	}
	if validateRefForSchema(right.schema, leftRef) {
		t.Fatal("foreign Factor schema accepted same-key Ref")
	}

	foreignComposition := NewComposition()
	// The same Factor semantic identity and raw key leave the foreign sealed
	// CompositionID as the only differing Ref identity component.
	foreign := coldFactor(foreignComposition, coldKey(97_101))
	if foreign == nil {
		t.Fatal("foreign Factor declaration")
	}
	declareRefFixtureMember(t, foreignComposition, foreign, 97_302)
	if !foreignComposition.Seal() {
		t.Fatal("foreign Composition seal")
	}
	foreignRef, issued := foreign.Ref(0)
	if !issued {
		t.Fatal("foreign Ref issuance")
	}
	if validateRefForWaveE(left, foreignRef) {
		t.Fatal("foreign Composition Ref was accepted")
	}
}

func TestRefWaveEValidationRejectsEqualButForeignComposition(t *testing.T) {
	declare := func(t testing.TB) (*Composition, *Factor[uint64, uint64]) {
		t.Helper()
		composition := NewComposition()
		factor := coldFactor(composition, coldKey(97_601))
		if factor == nil {
			t.Fatal("Factor declaration")
		}
		declareRefFixtureMember(t, composition, factor, 97_602)
		if !composition.Seal() {
			t.Fatal("Composition seal")
		}
		return composition, factor
	}
	leftComposition, left := declare(t)
	rightComposition, right := declare(t)
	if leftComposition.ID() != rightComposition.ID() {
		t.Fatal("equal declaration did not derive equal semantic identity")
	}
	leftRef, issued := left.Ref(0)
	if !issued || !validateRefForWaveE(left, leftRef) {
		t.Fatal("own Ref")
	}
	if validateRefForWaveE(right, leftRef) {
		t.Fatal("equal but foreign Composition accepted Ref")
	}
}

func TestRefWaveEValidationFailsClosedForAbsentUnsealedAndPoisonedOwners(t *testing.T) {
	var absent *Factor[uint64, uint64]
	if _, issued := absent.Ref(0); issued || validateRefForWaveE(absent, Ref[uint64]{}) {
		t.Fatal("absent Factor admitted a Ref")
	}

	unsealed := NewComposition()
	unsealedFactor := coldFactor(unsealed, coldKey(97_401))
	if unsealedFactor == nil {
		t.Fatal("unsealed Factor declaration")
	}
	if _, issued := unsealedFactor.Ref(0); issued || validateRefForWaveE(unsealedFactor, Ref[uint64]{}) {
		t.Fatal("unsealed Factor admitted a Ref")
	}

	poisoned := NewComposition()
	poisonedFactor := coldFactor(poisoned, coldKey(97_501))
	if poisonedFactor == nil || poisoned.Seal() {
		t.Fatal("poisoned Factor fixture")
	}
	if _, issued := poisonedFactor.Ref(0); issued || validateRefForWaveE(poisonedFactor, Ref[uint64]{}) {
		t.Fatal("poisoned Factor admitted a Ref")
	}
}

func declareRefFixtureMember(t testing.TB, composition *Composition, factor *Factor[uint64, uint64], semantic uint64) {
	t.Helper()
	read, readOK := ExactReadForm(factor)
	write, writeOK := ExactWriteForm(factor)
	rule, ruleOK := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent,
		Semantic:  coldKey(semantic),
		Output:    factor.Output(),
		Inputs:    0,
		Admission: testTrustedTheorem[uint64](semantic + 100),
		Transfer:  func(Access[uint64, ruleUnit]) bool { return true },
	}, func(rule *Rule[uint64, ruleUnit]) bool {
		_, ok := WriteTo(rule, write)
		return ok
	})
	query, queryOK := declareColdQuery(composition, coldKey(semantic+1), coldKey(semantic+2), read)
	if !readOK || !writeOK || !ruleOK || rule == nil || !queryOK || query == nil {
		t.Fatal("Ref fixture member declaration")
	}
}
