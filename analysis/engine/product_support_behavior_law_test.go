package engine

import (
	"context"
	"strconv"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
)

// supportProductSolver is a black-box multi-input support product. It keeps
// all support observations behind ordinary declaration, compilation, Solve,
// and QueryResult boundaries.
func supportProductSolver(tb testing.TB, inputs, members int, calls *int) (*Solver, QueryReceipt[bool]) {
	tb.Helper()
	if inputs < 1 || members < 1 {
		tb.Fatal("product dimensions")
	}
	cold := NewComposition()
	if coldFactor(cold, coldKey(99_701)) == nil {
		tb.Fatal("factor declaration")
	}
	completion, completionOK := DeclareSupportCompletion(cold, coldKey(99_702))
	prune, pruneOK := DeclarePrune(completion, coldKey(99_703))
	rules := make([]*SupportRule, members)
	for index := range rules {
		rule, ruleOK := DeclareSupportRule(cold, SupportRuleSpec{
			Semantic: coldKey(99_704 + index), Completion: completion, Prune: prune, Inputs: inputs,
			Admission: testTrustedTheorem[Support](uint64(99_800 + index)),
			Run: func(value Support) (Support, bool) {
				(*calls)++
				return value, true
			},
		})
		if !ruleOK || rule == nil {
			tb.Fatal("support rule declaration")
		}
		rules[index] = rule
	}
	query, queryOK := DeclareSupportQuery(cold, coldKey(99_900), func(observation SupportObservation) bool {
		reachable, ok := SupportReachable(observation)
		return ok && reachable
	}, FrozenResult[bool]{
		Semantic: coldKey(99_901), Freeze: func(value bool) bool { return value }, Clone: func(value bool) bool { return value },
		Equal: func(left, right bool) bool { return left == right }, Fingerprint: func(value bool) uint64 {
			if value {
				return 1
			}
			return 0
		},
	})
	if !completionOK || !pruneOK || !queryOK || query == nil || !cold.Seal() {
		tb.Fatal("cold declaration")
	}
	scope := equation.EmptyScope()
	batch := equation.NewBatch()
	sites := make([]equation.Site, inputs+1)
	for index := 0; index < inputs; index++ {
		site, ok := batch.AdmitSite(coldKey(99_950+index).compositionKey(), scope, equation.TrueExpr(), equation.InitPresent)
		if !ok {
			tb.Fatal("input source site")
		}
		sites[index] = site
	}
	outputSite, outputSiteOK := batch.AdmitSite(coldKey(99_990).compositionKey(), scope, equation.FalseExpr(), equation.InitAbsent)
	if !scope.Available() || !outputSiteOK {
		tb.Fatal("output source site")
	}
	sites[inputs] = outputSite
	occurrences := make([]equation.Occurrence, members)
	operands := make([]equation.Operand, members)
	for index := range rules {
		occurrence, occurrenceOK := batch.At(outputSite)
		operand, operandOK := batch.AdmitOperand(occurrence, coldKey(100_000+index).compositionKey())
		if !occurrenceOK || !operandOK {
			tb.Fatal("support source operand")
		}
		occurrences[index], operands[index] = occurrence, operand
	}
	if !batch.Seal() {
		tb.Fatal("support source batch")
	}
	var queryInstance *QueryInstance[bool]
	solver, compiled := assemble(cold, batch, func(assembly *Assembly) bool {
		points := make([]*assemblyPoint, inputs+1)
		for index, site := range sites {
			points[index] = admitPoint(assembly, site)
			if points[index] == nil {
				tb.Fatal("support point assembly")
			}
		}
		membersAt := make([]*assemblyMember, len(rules))
		for index, rule := range rules {
			instance, instanceOK := NewSupportInstance(rule, func(*StructuralBinding) bool { return true })
			membersAt[index] = admitStructural(assembly, points[inputs], occurrences[index], operands[index], instance)
			if !instanceOK || membersAt[index] == nil {
				tb.Fatal("support member assembly")
			}
		}
		group := admitGroup(assembly, points[inputs], membersAt...)
		if group == nil {
			tb.Fatal("support group assembly")
		}
		for index := 0; index < inputs; index++ {
			boundary := equation.BoundaryInput(sites[index], outputSite, coldKey(100_100+index).compositionKey(), equation.TrueExpr(), equation.IdentityReindex(scope), equation.TrueExpr())
			if !admitBoundary(assembly, group, boundary) {
				tb.Fatal("support boundary assembly")
			}
		}
		var queryInstanceOK bool
		queryInstance, queryInstanceOK = NewQueryInstance(query, func(*QueryBinding[bool]) bool { return true })
		if !queryInstanceOK || admitQueryAt(assembly, points[inputs], queryInstance) == nil {
			tb.Fatal("support query assembly")
		}
		return true
	})
	if !compiled || solver == nil {
		tb.Fatal("solver compilation")
	}
	receipt, receiptOK := queryInstance.Receipt()
	if !receiptOK {
		tb.Fatal("support query receipt")
	}
	return solver, receipt
}

func TestTwoInputSupportProductPreservesReachability(t *testing.T) {
	calls := 0
	solver, receipt := supportProductSolver(t, 2, 1, &calls)
	state, status := solver.Solve(context.Background())
	if status != SolveComplete || state == nil {
		t.Fatalf("solve = state:%v status:%v", state, status)
	}
	if calls != 1 {
		t.Fatalf("support product callbacks = %d, want one shared-support row", calls)
	}
	if reachable, ok := QueryResult(receipt, state); !ok || !reachable {
		t.Fatalf("completed shared support reachable=%v ok=%v", reachable, ok)
	}
}

func BenchmarkFourInputSupportProductMembers(b *testing.B) {
	for _, members := range []int{1, 8, 32} {
		b.Run("members="+strconv.Itoa(members), func(b *testing.B) {
			b.ReportAllocs()
			for index := 0; index < b.N; index++ {
				calls := 0
				solver, receipt := supportProductSolver(b, 4, members, &calls)
				state, status := solver.Solve(context.Background())
				if status != SolveComplete || state == nil || calls != members {
					b.Fatal("solve")
				}
				if reachable, ok := QueryResult(receipt, state); !ok || !reachable {
					b.Fatal("query")
				}
			}
		})
	}
}
