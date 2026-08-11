package engine

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
)

func sealAdmissionFixture(t testing.TB, admission RuleAdmission[uint64, ruleUnit]) *Composition {
	t.Helper()
	composition := NewComposition()
	factor := coldFactor(composition, coldKey(11001))
	if factor == nil {
		t.Fatal("factor declaration")
	}
	write, writeOK := ExactWriteForm(factor)
	if !writeOK {
		t.Fatal("write form")
	}
	rule, ruleOK := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent,
		Semantic: coldKey(11002), Output: factor.Output(), Inputs: 0,
		Admission: admission,
		Transfer:  func(Access[uint64, ruleUnit]) bool { return true },
	}, func(rule *Rule[uint64, ruleUnit]) bool {
		_, declared := WriteTo(rule, write)
		return declared
	})
	if !ruleOK || rule == nil {
		t.Fatal("rule declaration")
	}
	read, readOK := ExactReadForm(factor)
	if !readOK {
		t.Fatal("read form")
	}
	if query, queryOK := declareColdQuery(composition, coldKey(11003), coldKey(11004), read); !queryOK || query == nil {
		t.Fatal("query declaration")
	}
	if !composition.Seal() {
		t.Fatal("composition seal")
	}
	return composition
}

func admissionSolverFixture(t testing.TB, admission RuleAdmission[uint64, ruleUnit]) (*Solver, QueryReceipt[uint64]) {
	t.Helper()
	composition := NewComposition()
	factorSemantic := coldKey(11301)
	factor := coldFactor(composition, factorSemantic)
	read, readOK := ExactReadForm(factor)
	write, writeOK := ExactWriteForm(factor)
	if factor == nil || !readOK || !writeOK {
		t.Fatal("factor forms")
	}
	ruleSemantic := coldKey(11302)
	var ruleWrite Write[uint64]
	rule, ruleOK := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent,
		Semantic: ruleSemantic, Output: factor.Output(), Inputs: 0, Admission: admission,
		Transfer: func(access Access[uint64, ruleUnit]) bool {
			return Product(access, func(row Row) bool { return StageValue(access, row, uint64(1)) })
		},
	}, func(rule *Rule[uint64, ruleUnit]) bool {
		var declared bool
		ruleWrite, declared = WriteTo(rule, write)
		return declared
	})
	var token QueryRead[OrderedCells[uint64]]
	querySemantic := coldKey(11303)
	query, queryOK := DeclareQuery(composition, QuerySpec[uint64]{
		Semantic: querySemantic,
		Project: func(observation Observation) uint64 {
			result := uint64(0)
			if !ProjectRows(observation, func(row QueryRow) bool {
				cells, resolved := QueryValue(row, token)
				if !resolved || cells.Count() != 1 {
					return false
				}
				value, present, resolved := cells.At(0)
				if !resolved || !present {
					return false
				}
				result = value
				return true
			}) {
				return 0
			}
			return result
		},
		Result: FrozenResult[uint64]{
			Semantic: coldKey(11304), Freeze: func(value uint64) uint64 { return value }, Clone: func(value uint64) uint64 { return value },
			Equal: func(left, right uint64) bool { return left == right }, Fingerprint: func(value uint64) uint64 { return value },
		},
	}, func(query *Query[uint64]) bool {
		var declared bool
		token, declared = QueryReadFrom(query, read)
		return declared
	})
	if !ruleOK || rule == nil || !queryOK || query == nil || !composition.Seal() {
		t.Fatal("admission declarations did not seal")
	}
	readRef, readRefOK := factor.Ref(0)
	writeRef, writeRefOK := factor.Ref(0)
	instance, instanceOK := NewRuleInstance(rule, ruleUnitForSemantic(coldKey(11307)), func(binding *RuleBinding[uint64, ruleUnit]) bool {
		return InstanceWrite(binding, ruleWrite, writeRef)
	})
	batch := equation.NewBatch()
	scope := equation.EmptyScope()
	site, siteOK := batch.AdmitSite(coldKey(11305).compositionKey(), scope, equation.TrueExpr(), equation.InitPresent)
	occurrence, occurrenceOK := batch.Relation(site, coldKey(11306).compositionKey())
	operand, operandOK := admitInstanceOperand(batch, occurrence, instance)
	if !scope.Available() || !siteOK || !occurrenceOK || !operandOK || !instanceOK || !batch.Seal() || !readRefOK || !writeRefOK {
		t.Fatal("admission source")
	}
	var queryInstance *QueryInstance[uint64]
	solver, compiled := assemble(composition, batch, func(assembly *Assembly) bool {
		point := admitPoint(assembly, site)
		member := admitInstance(assembly, point, occurrence, operand, instance)
		if point == nil || member == nil || admitGroup(assembly, point, member) == nil {
			t.Fatal("admission rule assembly")
		}
		var queryInstanceOK bool
		queryInstance, queryInstanceOK = NewQueryInstance(query, func(binding *QueryBinding[uint64]) bool {
			return InstanceQueryRead(binding, token, readRef)
		})
		observation := admitQueryAt(assembly, point, queryInstance)
		if !queryInstanceOK || observation == nil {
			t.Fatal("admission query assembly")
		}
		return true
	})
	if !compiled || solver == nil {
		t.Fatal("admission solver compilation")
	}
	receipt, receiptOK := queryInstance.Receipt()
	if !receiptOK {
		t.Fatal("admission query receipt")
	}
	return solver, receipt
}

func TestRuleAdmissionIsRequiredAndFailClosed(t *testing.T) {
	composition := NewComposition()
	factor := coldFactor(composition, coldKey(11101))
	if factor == nil {
		t.Fatal("factor declaration")
	}
	if rule, ok := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent,
		Semantic: coldKey(11102), Output: factor.Output(), Inputs: 0,
		Transfer: func(Access[uint64, ruleUnit]) bool { return true },
	}, func(*Rule[uint64, ruleUnit]) bool { return true }); ok || rule != nil || composition.Seal() {
		t.Fatal("missing rule admission did not fail closed")
	}
}

func TestTrustedTheoremIdentityAndBasisEnterCompositionIdentity(t *testing.T) {
	identity := coldKey(11201)
	trusted := sealAdmissionFixture(t, AdmitRuleByTrustedTheorem[uint64, ruleUnit](identity))
	changedIdentity := sealAdmissionFixture(t, AdmitRuleByTrustedTheorem[uint64, ruleUnit](coldKey(11202)))
	derivation := sealAdmissionFixture(t, AdmitRuleByDerivation(identity, func(derivation RuleDerivation[uint64, ruleUnit]) (RuleEvidence, bool) {
		return derivation.Accept()
	}))
	if trusted.ID() == changedIdentity.ID() {
		t.Fatal("trusted theorem identity was omitted from composition identity")
	}
	if trusted.ID() == derivation.ID() {
		t.Fatal("admission basis was omitted from composition identity")
	}
}

func TestRuleAdmissionInventoryReportsTrustedTCBAndDerivationBasis(t *testing.T) {
	trustedIdentity := coldKey(11211)
	trusted := sealAdmissionFixture(t, AdmitRuleByTrustedTheorem[uint64, ruleUnit](trustedIdentity))
	trustedReport, trustedOK := trusted.RuleAdmissionInventory()
	if !trustedOK || trustedReport.ID != trusted.ID() || len(trustedReport.Rules) != 1 {
		t.Fatal("trusted theorem admission inventory")
	}
	trustedRow := trustedReport.Rules[0]
	if trustedRow.Rule != coldKey(11002) || trustedRow.Basis != RuleAdmissionBasisTrustedTheorem || trustedRow.Identity != trustedIdentity {
		t.Fatal("trusted theorem TCB provenance was not reported canonically")
	}
	trustedReport.Rules[0] = RuleAdmissionRecord{}
	again, againOK := trusted.RuleAdmissionInventory()
	if !againOK || len(again.Rules) != 1 || again.Rules[0] != trustedRow {
		t.Fatal("admission inventory was not an immutable snapshot")
	}

	derivationIdentity := coldKey(11212)
	derived := sealAdmissionFixture(t, AdmitRuleByDerivation(derivationIdentity, func(derivation RuleDerivation[uint64, ruleUnit]) (RuleEvidence, bool) {
		return derivation.Accept()
	}))
	derivedReport, derivedOK := derived.RuleAdmissionInventory()
	if !derivedOK || derivedReport.ID != derived.ID() || len(derivedReport.Rules) != 1 ||
		derivedReport.Rules[0].Basis != RuleAdmissionBasisDerivation || derivedReport.Rules[0].Identity != derivationIdentity {
		t.Fatal("local derivation provenance was not distinguished from trusted TCB")
	}
}

func TestRuleAdmissionInventoryEnumeratesEveryRuleCanonically(t *testing.T) {
	composition := NewComposition()
	factor := coldFactor(composition, coldKey(11221))
	if factor == nil {
		t.Fatal("factor declaration")
	}
	write, writeOK := ExactWriteForm(factor)
	read, readOK := ExactReadForm(factor)
	if !writeOK || !readOK {
		t.Fatal("factor forms")
	}
	trustedRule, derivationRule := coldKey(11223), coldKey(11222)
	trustedIdentity, derivationIdentity := coldKey(11224), coldKey(11225)
	declare := func(semantic SemanticKey, admission RuleAdmission[uint64, ruleUnit]) bool {
		rule, declared := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent,
			Semantic: semantic, Output: factor.Output(), Inputs: 0, Admission: admission,
			Transfer: func(Access[uint64, ruleUnit]) bool { return true },
		}, func(rule *Rule[uint64, ruleUnit]) bool {
			_, declared := WriteTo(rule, write)
			return declared
		})
		return declared && rule != nil
	}
	if !declare(trustedRule, AdmitRuleByTrustedTheorem[uint64, ruleUnit](trustedIdentity)) ||
		!declare(derivationRule, AdmitRuleByDerivation(derivationIdentity, func(derivation RuleDerivation[uint64, ruleUnit]) (RuleEvidence, bool) {
			return derivation.Accept()
		})) {
		t.Fatal("rule declarations")
	}
	if _, queryOK := declareColdQuery(composition, coldKey(11226), coldKey(11227), read); !queryOK || !composition.Seal() {
		t.Fatal("composition seal")
	}
	report, reported := composition.RuleAdmissionInventory()
	if !reported || report.ID != composition.ID() || len(report.Rules) != 2 {
		t.Fatal("complete admission inventory")
	}
	if report.Rules[0] != (RuleAdmissionRecord{Rule: derivationRule, Basis: RuleAdmissionBasisDerivation, Identity: derivationIdentity}) ||
		report.Rules[1] != (RuleAdmissionRecord{Rule: trustedRule, Basis: RuleAdmissionBasisTrustedTheorem, Identity: trustedIdentity}) {
		t.Fatal("inventory did not retain every Rule in canonical semantic order")
	}
}

func TestRuleDerivationAdmissionFailsClosedAtSolveBoundary(t *testing.T) {
	tests := []struct {
		name     string
		check    func(RuleDerivation[uint64, ruleUnit]) (RuleEvidence, bool)
		complete bool
	}{
		{
			name: "accepted",
			check: func(derivation RuleDerivation[uint64, ruleUnit]) (RuleEvidence, bool) {
				if derivation.Rule() != coldKey(11302) || !derivation.Composition().Available() || !derivation.Anchor().Available() || derivation.DispositionCount() != 1 {
					return RuleEvidence{}, false
				}
				return derivation.Accept()
			},
			complete: true,
		},
		{
			name:  "rejected callback",
			check: func(RuleDerivation[uint64, ruleUnit]) (RuleEvidence, bool) { return RuleEvidence{}, false },
		},
		{
			name:  "unbound evidence",
			check: func(RuleDerivation[uint64, ruleUnit]) (RuleEvidence, bool) { return RuleEvidence{}, true },
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			checks := 0
			solver, receipt := admissionSolverFixture(t, AdmitRuleByDerivation(coldKey(11306), func(derivation RuleDerivation[uint64, ruleUnit]) (RuleEvidence, bool) {
				checks++
				return test.check(derivation)
			}))
			state, status := solver.Solve(context.Background())
			if checks != 1 {
				t.Fatalf("checker calls = %d, want 1", checks)
			}
			if !test.complete {
				if state != nil || status != SolveIncomplete {
					t.Fatalf("rejected evidence published state=%v status=%v", state, status)
				}
				return
			}
			if state == nil || status != SolveComplete {
				t.Fatalf("accepted evidence solve = state:%v status:%v", state, status)
			}
			result, readable := QueryResult(receipt, state)
			if !readable || result != 1 {
				t.Fatalf("accepted evidence result = %d, readable=%v", result, readable)
			}
		})
	}
}

func TestSupportRuleAdmissionIsRequired(t *testing.T) {
	composition := NewComposition()
	if factor := coldFactor(composition, coldKey(11401)); factor == nil {
		t.Fatal("factor declaration")
	}
	completion, completionOK := DeclareSupportCompletion(composition, coldKey(11402))
	prune, pruneOK := DeclarePrune(completion, coldKey(11403))
	if !completionOK || !pruneOK {
		t.Fatal("support declarations")
	}
	if rule, ok := DeclareSupportRule(composition, SupportRuleSpec{
		Semantic: coldKey(11404), Completion: completion, Prune: prune, Inputs: 0,
		Run: func(value Support) (Support, bool) { return value, true },
	}); ok || rule != nil || composition.Seal() {
		t.Fatal("support rule without transfer admission did not fail closed")
	}
}
