package engine

import "testing"

// TestSchemaDeclarationsSealCanonically proves that equivalent declarations
// have one observable sealed identity, independent of declaration order.
func TestSchemaDeclarationsSealCanonically(t *testing.T) {
	left := schemaBinderFixture(t, false)
	right := schemaBinderFixture(t, true)
	if !left.Seal() || !right.Seal() {
		t.Fatal("equivalent declarations did not seal")
	}
	if !left.ID().Available() || left.ID() != right.ID() {
		t.Fatal("equivalent declaration permutations have different identities")
	}
	leftInventory, leftOK := left.RuleAdmissionInventory()
	rightInventory, rightOK := right.RuleAdmissionInventory()
	if !leftOK || !rightOK || leftInventory.ID != rightInventory.ID || len(leftInventory.Rules) != 1 || len(rightInventory.Rules) != 1 || leftInventory.Rules[0] != rightInventory.Rules[0] {
		t.Fatal("canonical declaration order changed the public admission inventory")
	}
}

// TestActivationPermissionSealsWithItsTrigger proves that a cold activation
// family is a reusable semantic permission and that its trigger has no
// candidate or topology authority before equation sealing.
func TestActivationPermissionSealsWithItsTrigger(t *testing.T) {
	composition := NewComposition()
	if coldFactor(composition, coldKey(710)) == nil {
		t.Fatal("factor declaration")
	}
	completion, completionOK := DeclareSupportCompletion(composition, coldKey(711))
	prune, pruneOK := DeclarePrune(completion, coldKey(712))
	if !completionOK || !pruneOK {
		t.Fatal("support declaration")
	}
	if rule, ok := DeclareSupportRule(composition, SupportRuleSpec{
		Semantic: coldKey(713), Completion: completion, Prune: prune, Inputs: 0,
		Admission: testTrustedTheorem[Support](1713), Run: func(value Support) (Support, bool) { return value, true },
	}); !ok || rule == nil {
		t.Fatal("support rule declaration")
	}
	family, familyOK := DeclareActivationFamily(composition, coldKey(714))
	if rule, ok := DeclareActivationRule(composition, ActivationRuleSpec{
		Semantic: coldKey(717), Family: family, Inputs: 0,
		Admission: testTrustedTheorem[ActivationResult](1717), Run: func(Activation) bool { return true },
	}); !familyOK || !ok || rule == nil {
		t.Fatal("activation declaration")
	}
	if query, ok := DeclareSupportQuery(composition, coldKey(718), func(SupportObservation) uint64 { return 0 }, frozenColdResult(coldKey(719))); !ok || query == nil {
		t.Fatal("support query declaration")
	}
	if !composition.Seal() || !composition.ID().Available() {
		t.Fatal("activation permission composition did not seal")
	}
}

// TestActivationPermissionRejectsForeignTrigger proves that a trigger may use
// only a semantic permission owned by its own cold Composition. Concrete
// ranges, Members, ports, and bindings remain equation-owned and are not part
// of this declaration law.
func TestActivationPermissionRejectsForeignTrigger(t *testing.T) {
	owner := NewComposition()
	family, familyOK := DeclareActivationFamily(owner, coldKey(720))
	if !familyOK {
		t.Fatal("activation permission declaration")
	}
	foreign := NewComposition()
	rule, accepted := DeclareActivationRule(foreign, ActivationRuleSpec{
		Semantic:  coldKey(721),
		Family:    family,
		Inputs:    0,
		Admission: testTrustedTheorem[ActivationResult](1721),
		Run:       func(Activation) bool { return true },
	})
	if accepted || rule != nil {
		t.Fatal("foreign activation permission was accepted")
	}
}

func TestActivationFamilyMustBindAnActivationRuleBeforeSeal(t *testing.T) {
	declareSealBaseline := func(composition *Composition, completionKey, pruneKey, ruleKey, queryKey, resultKey SemanticKey) bool {
		completion, completionOK := DeclareSupportCompletion(composition, completionKey)
		prune, pruneOK := DeclarePrune(completion, pruneKey)
		support, supportOK := DeclareSupportRule(composition, SupportRuleSpec{
			Semantic: ruleKey, Completion: completion, Prune: prune, Inputs: 0,
			Admission: testTrustedTheorem[Support](1722), Run: func(value Support) (Support, bool) { return value, true },
		})
		query, queryOK := DeclareSupportQuery(composition, queryKey, func(SupportObservation) uint64 { return 0 }, frozenColdResult(resultKey))
		return completionOK && pruneOK && supportOK && support != nil && queryOK && query != nil
	}
	orphan := NewComposition()
	if coldFactor(orphan, coldKey(722)) == nil {
		t.Fatal("orphan factor declaration")
	}
	if family, ok := DeclareActivationFamily(orphan, coldKey(723)); !ok || !family.available() ||
		!declareSealBaseline(orphan, coldKey(727), coldKey(728), coldKey(729), coldKey(730), coldKey(731)) {
		t.Fatal("orphan activation family declaration")
	}
	if orphan.Seal() {
		t.Fatal("orphan activation family sealed without an activation rule")
	}

	bound := NewComposition()
	if coldFactor(bound, coldKey(724)) == nil {
		t.Fatal("bound factor declaration")
	}
	family, familyOK := DeclareActivationFamily(bound, coldKey(725))
	rule, ruleOK := DeclareActivationRule(bound, ActivationRuleSpec{
		Semantic: coldKey(726), Family: family, Inputs: 0,
		Admission: testTrustedTheorem[ActivationResult](1726), Run: func(Activation) bool { return true },
	})
	if !familyOK || !ruleOK || rule == nil ||
		!declareSealBaseline(bound, coldKey(732), coldKey(733), coldKey(734), coldKey(735), coldKey(736)) || !bound.Seal() {
		t.Fatal("activation-bound family did not seal")
	}
}

func schemaBinderFixture(t testing.TB, reverseFactors bool) *Composition {
	t.Helper()
	composition := NewComposition()
	declare := func(semantic SemanticKey) *Factor[uint64, uint64] {
		factor, ok := DeclareFactor(composition, coldFactorSpec(semantic), func(*Factor[uint64, uint64]) bool { return true })
		if !ok || factor == nil {
			t.Fatal("factor declaration")
		}
		return factor
	}
	var output, input *Factor[uint64, uint64]
	if reverseFactors {
		input, output = declare(coldKey(701)), declare(coldKey(700))
	} else {
		output, input = declare(coldKey(700)), declare(coldKey(701))
	}
	inputRead, readOK := ExactReadForm(input)
	outputWrite, writeOK := ExactWriteForm(output)
	if !readOK || !writeOK {
		t.Fatal("factor forms")
	}
	rule, ruleOK := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent,
		Semantic: coldKey(702), Output: output.Output(), Inputs: 1,
		Admission: testTrustedTheorem[uint64](1702), Transfer: func(Access[uint64, ruleUnit]) bool { return true },
	}, func(rule *Rule[uint64, ruleUnit]) bool {
		port, portOK := rule.InputAt(0)
		if !portOK {
			return false
		}
		_, readDeclared := ReadFrom(rule, port, inputRead)
		_, writeDeclared := WriteTo(rule, outputWrite)
		return readDeclared && writeDeclared
	})
	if !ruleOK || rule == nil {
		t.Fatal("rule declaration")
	}
	query, queryOK := DeclareQuery(composition, QuerySpec[uint64]{
		Semantic: coldKey(703), Project: func(Observation) uint64 { return 0 }, Result: frozenColdResult(coldKey(704)),
	}, func(query *Query[uint64]) bool {
		_, ok := QueryReadFrom(query, inputRead)
		return ok
	})
	if !queryOK || query == nil {
		t.Fatal("query declaration")
	}
	return composition
}
