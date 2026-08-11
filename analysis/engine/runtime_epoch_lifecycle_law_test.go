package engine

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
)

// lifecycleLawCallbacks are deliberately ordinary user callbacks. Tests alter
// their behavior between attempts rather than reaching into a compiled
// Solver, so every assertion below is visible through Solve and QueryResult.
type lifecycleLawCallbacks struct {
	transfer  func(Access[uint64, ruleUnit]) bool
	transfers int
	projects  int
	freezes   int
}

// lifecycleLawBatch is the source-side declaration for these runtime laws.
// Sites, occurrences, and operands are all issued before the immutable batch
// crosses into the engine assembly.
func lifecycleLawBatch(sources, occurrences []SemanticKey, instances []*RuleInstance[uint64, ruleUnit], dispositions []equation.InitDisposition) (*equation.Batch, []equation.Site, []equation.Occurrence, []equation.Operand, bool) {
	if len(sources) == 0 || len(sources) != len(occurrences) || len(sources) != len(instances) || len(sources) != len(dispositions) {
		return nil, nil, nil, nil, false
	}
	batch := equation.NewBatch()
	scope := equation.EmptyScope()
	if batch == nil || !scope.Available() {
		return nil, nil, nil, nil, false
	}
	sites := make([]equation.Site, len(sources))
	resolvedOccurrences := make([]equation.Occurrence, len(sources))
	resolvedOperands := make([]equation.Operand, len(sources))
	for index := range sources {
		site, siteOK := batch.AdmitSite(sources[index].compositionKey(), scope, equation.TrueExpr(), dispositions[index])
		occurrence, occurrenceOK := batch.Relation(site, occurrences[index].compositionKey())
		operand, operandOK := admitInstanceOperand(batch, occurrence, instances[index])
		if !siteOK || !occurrenceOK || !operandOK {
			return nil, nil, nil, nil, false
		}
		sites[index], resolvedOccurrences[index], resolvedOperands[index] = site, occurrence, operand
	}
	if !batch.Seal() {
		return nil, nil, nil, nil, false
	}
	return batch, sites, resolvedOccurrences, resolvedOperands, true
}

// lifecycleLawBatchForSites admits several occurrences at each source Site.
// It is used when one simultaneous source publication owns several rules.
func lifecycleLawBatchForSites(sources, occurrences []SemanticKey, instances []*RuleInstance[uint64, ruleUnit], occurrenceSites []int, dispositions []equation.InitDisposition) (*equation.Batch, []equation.Site, []equation.Occurrence, []equation.Operand, bool) {
	if len(sources) == 0 || len(sources) != len(dispositions) || len(occurrences) == 0 || len(occurrences) != len(instances) || len(occurrences) != len(occurrenceSites) {
		return nil, nil, nil, nil, false
	}
	batch := equation.NewBatch()
	scope := equation.EmptyScope()
	if batch == nil || !scope.Available() {
		return nil, nil, nil, nil, false
	}
	sites := make([]equation.Site, len(sources))
	for index := range sources {
		init := equation.TrueExpr()
		if dispositions[index] == equation.InitAbsent {
			init = equation.FalseExpr()
		}
		site, admitted := batch.AdmitSite(sources[index].compositionKey(), scope, init, dispositions[index])
		if !admitted {
			return nil, nil, nil, nil, false
		}
		sites[index] = site
	}
	resolvedOccurrences := make([]equation.Occurrence, len(occurrences))
	resolvedOperands := make([]equation.Operand, len(occurrences))
	for index := range occurrences {
		siteIndex := occurrenceSites[index]
		if siteIndex < 0 || siteIndex >= len(sites) {
			return nil, nil, nil, nil, false
		}
		occurrence, occurred := batch.Relation(sites[siteIndex], occurrences[index].compositionKey())
		operand, admitted := admitInstanceOperand(batch, occurrence, instances[index])
		if !occurred || !admitted {
			return nil, nil, nil, nil, false
		}
		resolvedOccurrences[index], resolvedOperands[index] = occurrence, operand
	}
	if !batch.Seal() {
		return nil, nil, nil, nil, false
	}
	return batch, sites, resolvedOccurrences, resolvedOperands, true
}

func newLifecycleLawSolver(t *testing.T, callbacks *lifecycleLawCallbacks) (*Solver, QueryReceipt[uint64]) {
	t.Helper()
	if callbacks == nil {
		t.Fatal("callbacks")
	}
	cold := NewComposition()
	factorSemantic := coldKey(99_100)
	ruleSemantic := coldKey(99_101)
	querySemantic := coldKey(99_102)
	freezerSemantic := coldKey(99_103)
	anchorSemantic := coldKey(99_104)
	factor, factorOK := DeclareFactor(cold, coldFactorSpec(factorSemantic), func(*Factor[uint64, uint64]) bool { return true })
	read, readOK := ExactReadForm(factor)
	write, writeOK := ExactWriteForm(factor)
	if !factorOK || factor == nil || !readOK || !writeOK {
		t.Fatal("factor declaration")
	}
	var ruleWrite Write[uint64]
	rule, ruleOK := DeclareRule(cold, RuleSpec[uint64, ruleUnit]{OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent,
		Semantic: ruleSemantic, Output: factor.Output(), Inputs: 0, Admission: testTrustedTheorem[uint64](99_105),
		Transfer: func(access Access[uint64, ruleUnit]) bool {
			callbacks.transfers++
			if callbacks.transfer != nil {
				return callbacks.transfer(access)
			}
			return Product(access, func(row Row) bool { return StageValue(access, row, uint64(1)) })
		},
	}, func(rule *Rule[uint64, ruleUnit]) bool {
		var declared bool
		ruleWrite, declared = WriteTo(rule, write)
		return declared
	})
	var token QueryRead[OrderedCells[uint64]]
	query, queryOK := DeclareQuery(cold, QuerySpec[uint64]{
		Semantic: querySemantic,
		Project: func(observation Observation) uint64 {
			callbacks.projects++
			var value uint64
			if !ProjectRows(observation, func(row QueryRow) bool {
				cells, ok := QueryValue(row, token)
				if !ok || cells.Count() != 1 {
					return false
				}
				entry, present, ok := cells.At(0)
				if !ok || !present {
					return false
				}
				value = entry
				return true
			}) {
				return 0
			}
			return value
		},
		Result: FrozenResult[uint64]{
			Semantic: freezerSemantic,
			Freeze: func(value uint64) uint64 {
				callbacks.freezes++
				return value
			},
			Clone:       func(value uint64) uint64 { return value },
			Equal:       func(left, right uint64) bool { return left == right },
			Fingerprint: func(value uint64) uint64 { return value },
		},
	}, func(query *Query[uint64]) bool {
		var declared bool
		token, declared = QueryReadFrom(query, read)
		return declared
	})
	if !ruleOK || rule == nil || !queryOK || query == nil || !cold.Seal() {
		t.Fatal("cold declarations")
	}
	readRef, readRefOK := factor.Ref(0)
	writeRef, writeRefOK := factor.Ref(0)
	instance, instanceOK := NewRuleInstance(rule, ruleUnitForSemantic(coldKey(99_106)), func(binding *RuleBinding[uint64, ruleUnit]) bool {
		return InstanceWrite(binding, ruleWrite, writeRef)
	})
	batch, sites, occurrences, operands, admitted := lifecycleLawBatch(
		[]SemanticKey{anchorSemantic}, []SemanticKey{ruleSemantic}, []*RuleInstance[uint64, ruleUnit]{instance}, []equation.InitDisposition{equation.InitPresent},
	)
	if !readRefOK || !writeRefOK || !instanceOK || !admitted {
		t.Fatal("source batch")
	}
	var queryInstance *QueryInstance[uint64]
	solver, compiled := assemble(cold, batch, func(assembly *Assembly) bool {
		point := admitPoint(assembly, sites[0])
		member := admitInstance(assembly, point, occurrences[0], operands[0], instance)
		group := admitGroup(assembly, point, member)
		var queryDeclared bool
		queryInstance, queryDeclared = NewQueryInstance(query, func(binding *QueryBinding[uint64]) bool {
			return InstanceQueryRead(binding, token, readRef)
		})
		observation := admitQueryAt(assembly, point, queryInstance)
		return point != nil && queryDeclared && member != nil && group != nil && observation != nil
	})
	if !compiled || solver == nil {
		t.Fatal("solver compile")
	}
	receipt, receiptOK := queryInstance.Receipt()
	if !receiptOK {
		t.Fatal("query receipt")
	}
	return solver, receipt
}

// TestSolvePanicPublishesNoStateAndLaterAttemptIsFresh keeps the recovery
// contract entirely at the public boundary.
func TestSolvePanicPublishesNoStateAndLaterAttemptIsFresh(t *testing.T) {
	callbacks := &lifecycleLawCallbacks{transfer: func(Access[uint64, ruleUnit]) bool { panic("rule panic") }}
	solver, receipt := newLifecycleLawSolver(t, callbacks)

	state, status := solver.Solve(context.Background())
	if state != nil || status != SolvePanicked {
		t.Fatalf("panicked solve = state:%v status:%v", state, status)
	}
	if _, readable := QueryResult(receipt, state); readable {
		t.Fatal("panicked solve exposed a result")
	}

	callbacks.transfer = nil
	state, status = solver.Solve(context.Background())
	if state == nil || status != SolveComplete || callbacks.transfers != 2 {
		t.Fatalf("fresh solve after panic = state:%v status:%v transfers:%d", state, status, callbacks.transfers)
	}
	if value, readable := QueryResult(receipt, state); !readable || value != 1 {
		t.Fatalf("fresh result after panic = %d/%v", value, readable)
	}
}

// TestNoncompleteSolveDoesNotPublishReusableResult covers both ordinary
// non-convergence and cooperative cancellation. A later normal Solve must
// execute its callbacks again before it can publish a result.
func TestNoncompleteSolveDoesNotPublishReusableResult(t *testing.T) {
	tests := []struct {
		name    string
		attempt func(context.CancelFunc) func(Access[uint64, ruleUnit]) bool
		status  SolveStatus
	}{
		{name: "incomplete", attempt: func(context.CancelFunc) func(Access[uint64, ruleUnit]) bool {
			return func(Access[uint64, ruleUnit]) bool { return false }
		}, status: SolveIncomplete},
		{name: "canceled", attempt: func(cancel context.CancelFunc) func(Access[uint64, ruleUnit]) bool {
			return func(Access[uint64, ruleUnit]) bool { cancel(); return false }
		}, status: SolveCanceled},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			callbacks := &lifecycleLawCallbacks{}
			solver, receipt := newLifecycleLawSolver(t, callbacks)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			callbacks.transfer = test.attempt(cancel)

			state, status := solver.Solve(ctx)
			if state != nil || status != test.status || callbacks.transfers != 1 {
				t.Fatalf("noncomplete solve = state:%v status:%v transfers:%d", state, status, callbacks.transfers)
			}
			if _, readable := QueryResult(receipt, state); readable {
				t.Fatal("noncomplete solve exposed a result")
			}

			callbacks.transfer = nil
			state, status = solver.Solve(context.Background())
			if state == nil || status != SolveComplete || callbacks.transfers != 2 {
				t.Fatalf("fresh solve = state:%v status:%v transfers:%d", state, status, callbacks.transfers)
			}
			if value, readable := QueryResult(receipt, state); !readable || value != 1 {
				t.Fatalf("fresh result = %d/%v", value, readable)
			}
		})
	}
}
