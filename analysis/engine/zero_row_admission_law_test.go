package engine

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
)

// zeroRowAdmissionSolver builds the smallest executable Rule whose one input
// and one exact read are scoped to an unreachable point. The checker is
// intentionally hostile to an empty derivation: a zero-row Product must
// finish as an engine-owned structural no-op, never reach this callback.
func zeroRowAdmissionSolver(t testing.TB, initial equation.InitDisposition, checks, transfers, rows *int) (*Solver, QueryReceipt[uint64]) {
	t.Helper()
	composition := NewComposition()
	inputFactor := coldFactor(composition, coldKey(11801))
	outputFactor := coldFactor(composition, coldKey(11802))
	inputRead, inputReadOK := ExactReadForm(inputFactor)
	outputRead, outputReadOK := ExactReadForm(outputFactor)
	outputWrite, outputWriteOK := ExactWriteForm(outputFactor)
	if inputFactor == nil || outputFactor == nil || !inputReadOK || !outputReadOK || !outputWriteOK {
		t.Fatal("zero-row factor forms")
	}
	var input Read[OrderedCells[uint64]]
	var write Write[uint64]
	rule, ruleOK := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{
		Semantic: coldKey(11803), OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent,
		Output: outputFactor.Output(), Inputs: 1,
		Admission: AdmitRuleByDerivation(coldKey(11804), func(derivation RuleDerivation[uint64, ruleUnit]) (RuleEvidence, bool) {
			(*checks)++
			// A real row must still reach the domain checker. Reject it so the
			// positive control proves this admission remains fail-closed.
			if derivation.DispositionCount() != 1 {
				return RuleEvidence{}, false
			}
			return RuleEvidence{}, false
		}),
		Transfer: func(access Access[uint64, ruleUnit]) bool {
			(*transfers)++
			return Product(access, func(row Row) bool {
				(*rows)++
				_, readable := ReadValue(access, row, input)
				return readable && StageValue(access, row, uint64(9))
			})
		},
	}, func(rule *Rule[uint64, ruleUnit]) bool {
		port, portOK := rule.InputAt(0)
		var readOK, writeOK bool
		input, readOK = ReadFrom(rule, port, inputRead)
		write, writeOK = WriteTo(rule, outputWrite)
		return portOK && readOK && writeOK
	})
	var queryRead QueryRead[OrderedCells[uint64]]
	query, queryOK := DeclareQuery(composition, QuerySpec[uint64]{
		Semantic: coldKey(11805),
		Project: func(observation Observation) uint64 {
			result, seen := uint64(0), 0
			if !ProjectRows(observation, func(row QueryRow) bool {
				cells, readable := QueryValue(row, queryRead)
				value, present, valid := cells.At(0)
				if !readable || !valid || !present {
					return false
				}
				result, seen = value, seen+1
				return true
			}) || seen != 1 {
				return 0
			}
			return result
		},
		Result: frozenColdResult(coldKey(11806)),
	}, func(query *Query[uint64]) bool {
		var readOK bool
		queryRead, readOK = QueryReadFrom(query, outputRead)
		return readOK
	})
	if !ruleOK || rule == nil || !queryOK || query == nil || !composition.Seal() {
		t.Fatal("zero-row declarations")
	}
	inputRef, inputRefOK := inputFactor.Ref(0)
	outputRef, outputRefOK := outputFactor.Ref(0)
	instance, instanceOK := NewRuleInstance(rule, ruleUnitForSemantic(coldKey(11807)), func(binding *RuleBinding[uint64, ruleUnit]) bool {
		return InstanceRead(binding, input, inputRef) && InstanceWrite(binding, write, outputRef)
	})
	if !inputRefOK || !outputRefOK || !instanceOK {
		t.Fatal("zero-row instance")
	}

	scope := equation.EmptyScope()
	batch := equation.NewBatch()
	init := equation.FalseExpr()
	if initial == equation.InitPresent {
		init = equation.TrueExpr()
	}
	inputSite, inputSiteOK := batch.AdmitSite(coldKey(11808).compositionKey(), scope, init, initial)
	targetSite, targetSiteOK := batch.AdmitSite(coldKey(11809).compositionKey(), scope, init, initial)
	occurrence, occurrenceOK := batch.Relation(targetSite, coldKey(11810).compositionKey())
	operand, operandOK := admitInstanceOperand(batch, occurrence, instance)
	if !scope.Available() || !inputSiteOK || !targetSiteOK || !occurrenceOK || !operandOK || !batch.Seal() {
		t.Fatal("zero-row source batch")
	}
	boundary := equation.BoundaryInput(inputSite, targetSite, coldKey(11811).compositionKey(), equation.TrueExpr(), equation.IdentityReindex(scope), equation.TrueExpr())
	if !boundary.Available() {
		t.Fatal("zero-row boundary")
	}
	var queryInstance *QueryInstance[uint64]
	solver, compiled := assemble(composition, batch, func(assembly *Assembly) bool {
		inputPoint, targetPoint := admitPoint(assembly, inputSite), admitPoint(assembly, targetSite)
		member := admitInstance(assembly, targetPoint, occurrence, operand, instance)
		group := admitGroup(assembly, targetPoint, member)
		var queryInstanceOK bool
		queryInstance, queryInstanceOK = NewQueryInstance(query, func(binding *QueryBinding[uint64]) bool {
			return InstanceQueryRead(binding, queryRead, outputRef)
		})
		observation := admitQueryAt(assembly, targetPoint, queryInstance)
		return inputPoint != nil && targetPoint != nil && member != nil && group != nil &&
			admitBoundary(assembly, group, boundary) && queryInstanceOK && observation != nil
	})
	if !compiled || solver == nil || queryInstance == nil {
		t.Fatal("zero-row solver compilation")
	}
	receipt, receiptOK := queryInstance.Receipt()
	if !receiptOK {
		t.Fatal("zero-row query receipt")
	}
	return solver, receipt
}

func TestZeroRowDerivationAdmissionIsStructuralAndSilent(t *testing.T) {
	checks, transfers, rows := 0, 0, 0
	solver, receipt := zeroRowAdmissionSolver(t, equation.InitAbsent, &checks, &transfers, &rows)
	state, status := solver.Solve(context.Background())
	if status != SolveComplete || state == nil {
		t.Fatalf("zero-row solve = state:%v status:%v checks:%d transfers:%d rows:%d", state, status, checks, transfers, rows)
	}
	result, readable := QueryResult(receipt, state)
	if !readable || result != 0 {
		t.Fatalf("zero-row query result = %d readable=%v, want empty projection", result, readable)
	}
	if checks != 0 {
		t.Fatalf("zero-row derivation checker calls = %d, want 0", checks)
	}
	if transfers != 1 || rows != 0 {
		t.Fatalf("zero-row callbacks = transfers:%d rows:%d, want 1:0", transfers, rows)
	}
}

func TestNonemptyDerivationAdmissionStillCallsCheckerAndRejects(t *testing.T) {
	checks, transfers, rows := 0, 0, 0
	solver, _ := zeroRowAdmissionSolver(t, equation.InitPresent, &checks, &transfers, &rows)
	state, status := solver.Solve(context.Background())
	if state != nil || status != SolveIncomplete {
		t.Fatalf("nonempty rejected solve = state:%v status:%v", state, status)
	}
	if checks != 1 {
		t.Fatalf("nonempty derivation checker calls = %d, want 1", checks)
	}
	if transfers != 1 || rows != 1 {
		t.Fatalf("nonempty callbacks = transfers:%d rows:%d, want 1:1", transfers, rows)
	}
}
