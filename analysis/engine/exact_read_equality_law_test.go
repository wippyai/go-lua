package engine

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/lattice"
)

// TestExactReadEqualityDistinguishesCellsByPresenceAndLatticeValue fixes the
// semantic equality used by exact Product and Query reads. Equal records may
// share neither allocation nor concrete witness; differing present values and
// presence states must remain distinct product branches.
func TestExactReadEqualityDistinguishesCellsByPresenceAndLatticeValue(t *testing.T) {
	composition := NewComposition()
	factor, declared := DeclareFactor(composition, FactorSpec[uint64, exactReadEqualityValue]{
		Semantic: coldKey(91_800), KeyEnd: 1,
		Lattice: lattice.Lattice[exactReadEqualityValue]{
			Bottom: func() exactReadEqualityValue { return exactReadEqualityValue{} },
			Top:    func() exactReadEqualityValue { return exactReadEqualityValue{class: 2} },
			Equal: func(left, right exactReadEqualityValue) bool {
				return left.class == right.class
			},
			LessOrEq: func(left, right exactReadEqualityValue) bool {
				return left.class <= right.class
			},
			Join: func(left, right exactReadEqualityValue) exactReadEqualityValue {
				if left.class >= right.class {
					return left
				}
				return right
			},
			Widen: func(left, right exactReadEqualityValue) exactReadEqualityValue {
				if left.class >= right.class {
					return left
				}
				return right
			},
		},
		Default: exactReadEqualityValue{},
		AdmitAt: func(uint64, exactReadEqualityValue) bool { return true },
		Fingerprint: func(value exactReadEqualityValue) uint64 {
			return value.class
		},
	}, func(*Factor[uint64, exactReadEqualityValue]) bool { return true })
	if !declared || factor == nil {
		t.Fatal("exact equality Factor declaration")
	}
	read, readOK := ExactReadForm(factor)
	if !readOK || read.equal == nil || read.fingerprint == nil {
		t.Fatal("exact equality ReadForm")
	}

	cells := func(present bool, value exactReadEqualityValue) OrderedCells[exactReadEqualityValue] {
		return OrderedCells[exactReadEqualityValue]{record: newOrderedCellsRecord([]summaryCell[exactReadEqualityValue]{{present: present, value: value}})}
	}
	left := cells(true, exactReadEqualityValue{class: 1, witness: "left"})
	latticeEqual := cells(true, exactReadEqualityValue{class: 1, witness: "right"})
	differentValue := cells(true, exactReadEqualityValue{class: 2, witness: "other"})
	absent := cells(false, exactReadEqualityValue{class: 1, witness: "left"})

	if !read.equal(left, latticeEqual) {
		t.Fatal("exact equality rejected separately allocated lattice-equal cells")
	}
	if read.fingerprint(left) != read.fingerprint(latticeEqual) {
		t.Fatal("lattice-equal exact cells entered different fingerprint buckets")
	}
	if read.equal(left, differentValue) {
		t.Fatal("exact equality merged distinct present lattice values")
	}
	if read.equal(left, absent) {
		t.Fatal("exact equality merged present and absent cells")
	}
}

// TestIdentityNormalizerIsExactlyTheExactOrderedCellLaw fixes the sole
// variable-arity identity summary.  It must not grow a domain-local copy of
// ordered-cell comparison merely because a Rule consumes more than one exact
// coordinate.
func TestIdentityNormalizerIsExactlyTheExactOrderedCellLaw(t *testing.T) {
	composition := NewComposition()
	var exact, summary ReadForm[exactReadEqualityValue, OrderedCells[exactReadEqualityValue]]
	factor, declared := DeclareFactor(composition, FactorSpec[uint64, exactReadEqualityValue]{
		Semantic: coldKey(91_810), KeyEnd: 2,
		Lattice: lattice.Lattice[exactReadEqualityValue]{
			Bottom:   func() exactReadEqualityValue { return exactReadEqualityValue{} },
			Top:      func() exactReadEqualityValue { return exactReadEqualityValue{class: 2} },
			Equal:    func(left, right exactReadEqualityValue) bool { return left.class == right.class },
			LessOrEq: func(left, right exactReadEqualityValue) bool { return left.class <= right.class },
			Join: func(left, right exactReadEqualityValue) exactReadEqualityValue {
				if left.class >= right.class {
					return left
				}
				return right
			},
			Widen: func(left, right exactReadEqualityValue) exactReadEqualityValue {
				if left.class >= right.class {
					return left
				}
				return right
			},
		},
		Default: exactReadEqualityValue{}, AdmitAt: func(uint64, exactReadEqualityValue) bool { return true },
		Fingerprint: func(value exactReadEqualityValue) uint64 { return value.class },
	}, func(factor *Factor[uint64, exactReadEqualityValue]) bool {
		var ok bool
		exact, ok = ExactReadForm(factor)
		if !ok {
			return false
		}
		normalizer, ok := DeclareIdentityNormalizer(factor, coldKey(91_811))
		if !ok {
			return false
		}
		summary, ok = SummaryReadForm(normalizer)
		return ok
	})
	if !declared || factor == nil || exact.equal == nil || summary.equal == nil || exact.fingerprint == nil || summary.fingerprint == nil {
		t.Fatal("identity normalizer declaration")
	}
	cells := func(values ...summaryCell[exactReadEqualityValue]) OrderedCells[exactReadEqualityValue] {
		return OrderedCells[exactReadEqualityValue]{record: newOrderedCellsRecord(values)}
	}
	left := cells(
		summaryCell[exactReadEqualityValue]{present: true, value: exactReadEqualityValue{class: 1, witness: "left"}},
		summaryCell[exactReadEqualityValue]{present: false},
	)
	equal := cells(
		summaryCell[exactReadEqualityValue]{present: true, value: exactReadEqualityValue{class: 1, witness: "equal"}},
		summaryCell[exactReadEqualityValue]{present: false},
	)
	different := cells(
		summaryCell[exactReadEqualityValue]{present: true, value: exactReadEqualityValue{class: 2, witness: "different"}},
		summaryCell[exactReadEqualityValue]{present: false},
	)
	if summary.normalize(left).record != left.record {
		t.Fatal("identity normalizer copied or changed ordered cells")
	}
	ordered := cells(
		summaryCell[exactReadEqualityValue]{present: true, value: exactReadEqualityValue{class: 1, witness: "first"}},
		summaryCell[exactReadEqualityValue]{present: true, value: exactReadEqualityValue{class: 2, witness: "second"}},
	)
	identity := summary.normalize(ordered)
	first, firstPresent, firstOK := identity.At(0)
	second, secondPresent, secondOK := identity.At(1)
	if !firstOK || !firstPresent || !secondOK || !secondPresent || first.witness != "first" || second.witness != "second" {
		t.Fatal("identity summary changed ordered row values")
	}
	for _, right := range []OrderedCells[exactReadEqualityValue]{left, equal, different} {
		if exact.equal(left, right) != summary.equal(left, right) || exact.fingerprint(right) != summary.fingerprint(right) {
			t.Fatal("identity summary diverged from exact ordered-cell law")
		}
	}
}

func TestIdentityNormalizerRejectsDuplicateAndForeignSemantics(t *testing.T) {
	duplicate := NewComposition()
	if factor, ok := DeclareFactor(duplicate, coldFactorSpec(coldKey(91_820)), func(factor *Factor[uint64, uint64]) bool {
		_, first := DeclareIdentityNormalizer(factor, coldKey(91_821))
		_, second := DeclareIdentityNormalizer(factor, coldKey(91_821))
		return first && second
	}); ok || factor != nil {
		t.Fatal("identity normalizer accepted its duplicate semantic")
	}

	foreign := NewComposition()
	if first, ok := DeclareFactor(foreign, coldFactorSpec(coldKey(91_830)), func(factor *Factor[uint64, uint64]) bool {
		_, declared := DeclareIdentityNormalizer(factor, coldKey(91_831))
		return declared
	}); !ok || first == nil {
		t.Fatal("first Factor declaration")
	}
	if second, ok := DeclareFactor(foreign, coldFactorSpec(coldKey(91_832)), func(factor *Factor[uint64, uint64]) bool {
		_, declared := DeclareIdentityNormalizer(factor, coldKey(91_831))
		return declared
	}); ok || second != nil {
		t.Fatal("foreign Factor claimed an existing identity summary semantic")
	}
}

func TestIdentityNormalizerClosedRefsCanonicalizeAndDedupe(t *testing.T) {
	composition := NewComposition()
	var exact ReadForm[uint64, OrderedCells[uint64]]
	factor, declared := DeclareFactor(composition, coldFactorSpec(coldKey(91_840)), func(factor *Factor[uint64, uint64]) bool {
		var ok bool
		exact, ok = ExactReadForm(factor)
		if !ok {
			return false
		}
		_, ok = DeclareIdentityNormalizer(factor, coldKey(91_841))
		return ok
	})
	if !declared || factor == nil {
		t.Fatal("identity summary Factor declaration")
	}
	write, writeOK := ExactWriteForm(factor)
	var target Write[uint64]
	rule, ruleOK := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{
		Semantic: coldKey(91_842), OperandFamily: coldKey(91_843), OperandContent: ruleUnitContent,
		Output: factor.Output(), Inputs: 0, Admission: testTrustedTheorem[uint64](91_844), Transfer: func(Access[uint64, ruleUnit]) bool { return true },
	}, func(rule *Rule[uint64, ruleUnit]) bool {
		var ok bool
		target, ok = WriteTo(rule, write)
		return ok
	})
	query, _, queryOK := declareColdQueryInstance(composition, coldKey(91_845), coldKey(91_846), exact)
	if !writeOK || !ruleOK || rule == nil || target.index != 0 || !queryOK || query == nil || !composition.Seal() {
		t.Fatal("identity summary composition seal")
	}
	refs := factor.NewClosedRefs()
	zero, zeroOK := factor.Ref(0)
	one, oneOK := factor.Ref(1)
	if refs == nil || !zeroOK || !oneOK || !refs.Append(one) || !refs.Append(zero) {
		t.Fatal("identity summary refs append")
	}
	if refs.Append(one) {
		t.Fatal("identity summary refs accepted a duplicate")
	}
	if !refs.Close() {
		t.Fatal("identity summary refs close")
	}
	sealed, sealedOK := refs.sealedRefsForAssembly(factor.schema)
	if !sealedOK || len(sealed) != 2 || sealed[0].raw != 0 || sealed[1].raw != 1 {
		t.Fatal("identity summary refs were not canonicalized by coordinate")
	}
}

func TestSummaryReadInternsOnlyExactClosedVectorPointer(t *testing.T) {
	composition := NewComposition()
	var exact ReadForm[uint64, OrderedCells[uint64]]
	var summary ReadForm[uint64, OrderedCells[uint64]]
	var write WriteForm[uint64]
	factor, declared := DeclareFactor(composition, coldFactorSpec(coldKey(91_850)), func(factor *Factor[uint64, uint64]) bool {
		var exactOK, summaryOK, writeOK bool
		exact, exactOK = ExactReadForm(factor)
		write, writeOK = ExactWriteForm(factor)
		normalizer, normalizerOK := DeclareIdentityNormalizer(factor, coldKey(91_851))
		if !normalizerOK {
			return false
		}
		summary, summaryOK = SummaryReadForm(normalizer)
		return exactOK && summaryOK && writeOK
	})
	var ruleWrite Write[uint64]
	rule, ruleOK := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{
		Semantic: coldKey(91_852), OperandFamily: coldKey(91_853), OperandContent: ruleUnitContent,
		Output: factor.Output(), Inputs: 0, Admission: testTrustedTheorem[uint64](91_854), Transfer: func(Access[uint64, ruleUnit]) bool { return true },
	}, func(rule *Rule[uint64, ruleUnit]) bool {
		var ok bool
		ruleWrite, ok = WriteTo(rule, write)
		return ok
	})
	query, queryOK := DeclareQuery(composition, QuerySpec[uint64]{
		Semantic: coldKey(91_855), Project: func(Observation) uint64 { return 0 },
		Result: FrozenResult[uint64]{
			Semantic: coldKey(91_856), Freeze: func(value uint64) uint64 { return value }, Clone: func(value uint64) uint64 { return value },
			Equal: func(left, right uint64) bool { return left == right }, Fingerprint: func(value uint64) uint64 { return value },
		},
	}, func(query *Query[uint64]) bool {
		_, ok := QueryReadFrom(query, exact)
		return ok
	})
	if !declared || factor == nil || !ruleOK || rule == nil || !queryOK || query == nil || !composition.Seal() {
		t.Fatal("summary interning declaration")
	}
	zero, zeroOK := factor.Ref(0)
	one, oneOK := factor.Ref(1)
	shared := factor.NewClosedRefs()
	equalButDistinct := factor.NewClosedRefs()
	if !zeroOK || !oneOK || shared == nil || equalButDistinct == nil || !shared.Append(one) || !shared.Append(zero) || !shared.Close() || !equalButDistinct.Append(zero) || !equalButDistinct.Append(one) || !equalButDistinct.Close() {
		t.Fatal("summary interning vectors")
	}
	instance, instanceOK := NewRuleInstance(rule, ruleUnitForSemantic(coldKey(91_857)), func(binding *RuleBinding[uint64, ruleUnit]) bool {
		return InstanceWrite(binding, ruleWrite, zero)
	})
	batch, _, _, _, batchOK := coldSourceBatch(coldKey(91_858), coldKey(91_859), instance)
	assembly := newAssembly(composition, batch)
	if !instanceOK || !batchOK || assembly == nil || batch == nil || !batch.Sealed() {
		t.Fatal("summary interning Assembly")
	}
	first := admitSummary(assembly, summary, shared)
	second := admitSummary(assembly, summary, shared)
	third := admitSummary(assembly, summary, equalButDistinct)
	if first == nil || second != first || third == nil || third == first || len(assembly.summaries) != 2 || assembly.summaryLocal != 2 || len(assembly.summaryIntern) != 2 {
		t.Fatalf("summary interning: mappings=%d locals=%d cache=%d", len(assembly.summaries), assembly.summaryLocal, len(assembly.summaryIntern))
	}
	if first.surface != second.surface || first.surface == third.surface {
		t.Fatal("summary intern surface identity")
	}
}

type exactReadEqualityValue struct {
	class   uint64
	witness string
}
