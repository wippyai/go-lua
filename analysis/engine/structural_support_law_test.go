package engine

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
)

// TestStructuralContradictionPrunesCompletedSuccessorSupport exercises the
// public structural path end to end.  It intentionally declares no Factor:
// the typed Support callback is the successor capability, its Empty result is
// the contradiction, and the completed support Query is the only observer.
func TestStructuralContradictionPrunesCompletedSuccessorSupport(t *testing.T) {
	cold := NewComposition()
	completion, completionOK := DeclareSupportCompletion(cold, coldKey(99_601))
	prune, pruneOK := DeclarePrune(completion, coldKey(99_602))
	runs := 0
	rule, ruleOK := DeclareSupportRule(cold, SupportRuleSpec{
		Semantic: coldKey(99_603), Completion: completion, Prune: prune, Inputs: 1,
		Admission: testTrustedTheorem[Support](99_604),
		Run: func(successor Support) (Support, bool) {
			runs++
			return successor.Empty()
		},
	})
	query, queryOK := DeclareSupportQuery(cold, coldKey(99_605), func(observation SupportObservation) bool {
		reachable, ok := SupportReachable(observation)
		return ok && reachable
	}, FrozenResult[bool]{
		Semantic: coldKey(99_606),
		Freeze:   func(value bool) bool { return value },
		Clone:    func(value bool) bool { return value },
		Equal:    func(left, right bool) bool { return left == right },
		Fingerprint: func(value bool) uint64 {
			if value {
				return 1
			}
			return 0
		},
	})
	if !completionOK || !pruneOK || !ruleOK || rule == nil || !queryOK || query == nil || !cold.Seal() {
		t.Fatal("factor-free structural declaration")
	}

	scope := equation.EmptyScope()
	batch := equation.NewBatch()
	sourceSite, sourceOK := batch.AdmitSite(coldKey(99_607).compositionKey(), scope, equation.TrueExpr(), equation.InitPresent)
	successorSite, successorOK := batch.AdmitSite(coldKey(99_608).compositionKey(), scope, equation.FalseExpr(), equation.InitAbsent)
	occurrence, occurrenceOK := batch.At(successorSite)
	operand, operandOK := batch.AdmitOperand(occurrence, coldKey(99_609).compositionKey())
	if !scope.Available() || !sourceOK || !successorOK || !occurrenceOK || !operandOK || !batch.Seal() {
		t.Fatal("structural source batch")
	}
	boundary := equation.BoundaryInput(sourceSite, successorSite, coldKey(99_610).compositionKey(), equation.TrueExpr(), equation.IdentityReindex(scope), equation.TrueExpr())
	if !boundary.Available() {
		t.Fatal("structural boundary")
	}
	var queryInstance *QueryInstance[bool]
	solver, compiled := assemble(cold, batch, func(assembly *Assembly) bool {
		sourcePoint := admitPoint(assembly, sourceSite)
		successorPoint := admitPoint(assembly, successorSite)
		instance, instanceOK := NewSupportInstance(rule, func(*StructuralBinding) bool { return true })
		member := admitStructural(assembly, successorPoint, occurrence, operand, instance)
		group := admitGroup(assembly, successorPoint, member)
		var queryInstanceOK bool
		queryInstance, queryInstanceOK = NewQueryInstance(query, func(*QueryBinding[bool]) bool { return true })
		return sourcePoint != nil && successorPoint != nil && instanceOK && member != nil && group != nil && queryInstanceOK && admitQueryAt(assembly, successorPoint, queryInstance) != nil && admitBoundary(assembly, group, boundary)
	})
	if !compiled || solver == nil {
		t.Fatal("factor-free structural solver")
	}
	state, status := solver.Solve(context.Background())
	if status != SolveComplete || state == nil || runs != 1 {
		t.Fatalf("structural contradiction solve = state:%v status:%v runs:%d", state, status, runs)
	}
	receipt, receiptOK := queryInstance.Receipt()
	if reachable, ok := QueryResult(receipt, state); !receiptOK || !ok || reachable {
		t.Fatalf("contradicted successor reachable=%t ok=%t", reachable, ok)
	}
}
