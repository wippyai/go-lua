package engine

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
)

func supportAdmissionFixture(t testing.TB, admissionFor func(*Read[OrderedCells[uint64]]) RuleAdmission[Support, ruleUnit], observe func(Support), pruneSupport bool, project func(SupportObservation) uint64) (*Solver, QueryReceipt[uint64]) {
	t.Helper()
	if project == nil {
		project = func(SupportObservation) uint64 { return 0 }
	}
	composition := NewComposition()
	factor := coldFactor(composition, coldKey(19901))
	if factor == nil {
		t.Fatal("factor declaration")
	}
	read, readOK := ExactReadForm(factor)
	completion, completionOK := DeclareSupportCompletion(composition, coldKey(19902))
	prune, pruneOK := DeclarePrune(completion, coldKey(19903))
	if !readOK || !completionOK || !pruneOK || admissionFor == nil {
		t.Fatal("support declaration prerequisites")
	}
	var proofRead Read[OrderedCells[uint64]]
	rule, ruleOK := DeclareSupportRule(composition, SupportRuleSpec{
		Semantic: coldKey(19904), Completion: completion, Prune: prune, Inputs: 1, Admission: admissionFor(&proofRead),
		Declare: func(rule *SupportRule) bool {
			input, inputOK := rule.InputAt(0)
			var declared bool
			proofRead, declared = ReadFrom(rule, input, read)
			return inputOK && declared
		},
		Run: func(value Support) (Support, bool) {
			if observe != nil {
				observe(value)
			}
			cells, ok := SupportReadValue(value, proofRead)
			if !ok || cells.Count() != 1 {
				return Support{}, false
			}
			if pruneSupport {
				return value.Empty()
			}
			return value, true
		},
	})
	if !ruleOK || rule == nil {
		t.Fatal("support rule declaration")
	}
	query, queryOK := DeclareSupportQuery(composition, coldKey(19905), project, FrozenResult[uint64]{
		Semantic: coldKey(19906), Freeze: func(value uint64) uint64 { return value }, Clone: func(value uint64) uint64 { return value },
		Equal: func(left, right uint64) bool { return left == right }, Fingerprint: func(value uint64) uint64 { return value },
	})
	if !queryOK || query == nil || !composition.Seal() {
		t.Fatal("support composition seal")
	}
	scope := equation.EmptyScope()
	batch := equation.NewBatch()
	sourceSite, sourceSiteOK := batch.AdmitSite(coldKey(19907).compositionKey(), scope, equation.TrueExpr(), equation.InitPresent)
	outputSite, outputSiteOK := batch.AdmitSite(coldKey(19909).compositionKey(), scope, equation.FalseExpr(), equation.InitAbsent)
	occurrence, occurrenceOK := batch.At(outputSite)
	operand, operandOK := batch.AdmitOperand(occurrence, coldKey(19910).compositionKey())
	if !sourceSiteOK || !outputSiteOK || !occurrenceOK || !operandOK || !scope.Available() || !batch.Seal() {
		t.Fatal("support source batch")
	}
	boundary := equation.BoundaryInput(sourceSite, outputSite, coldKey(19911).compositionKey(), equation.TrueExpr(), equation.IdentityReindex(scope), equation.TrueExpr())
	if !boundary.Available() {
		t.Fatal("support boundary")
	}
	ref, issued := factor.Ref(0)
	var queryInstance *QueryInstance[uint64]
	solver, compiled := assemble(composition, batch, func(assembly *Assembly) bool {
		sourcePoint := admitPoint(assembly, sourceSite)
		outputPoint := admitPoint(assembly, outputSite)
		instance, instanceOK := NewSupportInstance(rule, func(binding *StructuralBinding) bool {
			return StructuralRead(binding, proofRead, ref)
		})
		member := admitStructural(assembly, outputPoint, occurrence, operand, instance)
		group := admitGroup(assembly, outputPoint, member)
		var queryInstanceOK bool
		queryInstance, queryInstanceOK = NewQueryInstance(query, func(*QueryBinding[uint64]) bool { return true })
		return sourcePoint != nil && outputPoint != nil && instanceOK && member != nil && issued && group != nil && queryInstanceOK && admitQueryAt(assembly, outputPoint, queryInstance) != nil &&
			admitBoundary(assembly, group, boundary)
	})
	if !compiled || solver == nil {
		t.Fatal("support solver assembly")
	}
	receipt, receiptOK := queryInstance.Receipt()
	if !receiptOK {
		t.Fatal("support query receipt")
	}
	return solver, receipt
}

func TestSupportDerivationCheckerSeesTypedReadAndRetainedGuard(t *testing.T) {
	checks := 0
	var callbackSupport Support
	solver, _ := supportAdmissionFixture(t, func(read *Read[OrderedCells[uint64]]) RuleAdmission[Support, ruleUnit] {
		return AdmitRuleByDerivation(coldKey(19908), func(derivation RuleDerivation[Support, ruleUnit]) (RuleEvidence, bool) {
			checks++
			input, inputOK := derivation.InputAt(0)
			disposition, dispositionOK := derivation.DispositionAt(0)
			cells, readOK := DerivationDispositionReadValue(derivation, disposition, *read)
			_, _, cellOK := cells.At(0)
			if !inputOK || !dispositionOK || !readOK || !cellOK || disposition.Kind() != RuleDispositionStaged || disposition.TargetCount() != 0 || !disposition.Guard().Same(input.Guard()) {
				return RuleEvidence{}, false
			}
			return derivation.Accept()
		})
	}, func(value Support) { callbackSupport = value }, false, nil)
	state, status := solver.Solve(context.Background())
	if status != SolveComplete || state == nil {
		t.Fatalf("support solve = %v", status)
	}
	if checks != 1 {
		t.Fatalf("support checker calls = %d, want 1", checks)
	}
	if _, live := callbackSupport.Empty(); live {
		t.Fatal("Support callback capability survived its Run callback")
	}
}

func TestSupportQueryObservesOnlyCompletedSupportTruth(t *testing.T) {
	for _, test := range []struct {
		name      string
		prune     bool
		reachable bool
	}{
		{name: "nonempty", reachable: true},
		{name: "empty", prune: true, reachable: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			var retained SupportObservation
			project := func(observation SupportObservation) uint64 {
				retained = observation
				reachable, ok := SupportReachable(observation)
				if !ok {
					return 2
				}
				if reachable {
					return 1
				}
				return 0
			}
			solver, receipt := supportAdmissionFixture(t, func(*Read[OrderedCells[uint64]]) RuleAdmission[Support, ruleUnit] {
				return testTrustedTheorem[Support](19908)
			}, nil, test.prune, project)
			if solver == nil || !receipt.Available() {
				t.Fatal("support fixture")
			}
			state, status := solver.Solve(context.Background())
			if status != SolveComplete || state == nil {
				t.Fatalf("support solve = %v", status)
			}
			value, ok := QueryResult(receipt, state)
			if !ok {
				t.Fatal("support result")
			}
			want := uint64(0)
			if test.reachable {
				want = 1
			}
			if value != want {
				t.Fatalf("support reachability = %d, want %d", value, want)
			}
			if _, live := SupportReachable(retained); live {
				t.Fatal("support observation survived its projector")
			}
			if _, foreign := SupportReachable(SupportObservation{}); foreign {
				t.Fatal("foreign support observation was accepted")
			}
		})
	}
}
