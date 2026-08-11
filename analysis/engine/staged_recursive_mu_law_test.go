package engine

import (
	"context"
	"reflect"
	"sync"
	"testing"
)

// TestStagedExactReadChangesRouteDuringRecursiveMuIteration is a public
// semantic law for a staged exact read in a genuine self-recursive equation.
// The sealed SCC has one recursive control coordinate. Its first productive
// Mu step observes route zero (payload 2) at control 1, then its next step
// observes route one (payload 3) at control 2. At control 3 the same second
// route is a post-fixpoint and emits NoCandidate. The fixture contains no
// activation family or plan, rebuild, or callback recursion: one cold
// assembly reaches the result through Solver.Solve alone.
func TestStagedExactReadChangesRouteDuringRecursiveMuIteration(t *testing.T) {
	first := solveStagedRecursiveMuLaw(t)
	second := solveStagedRecursiveMuLaw(t)

	wantSteps := []stagedRecursiveMuStep{
		{control: 1, tag: 1, value: 2},
		{control: 2, tag: 2, value: 3},
	}
	if first.result != 3 || !reflect.DeepEqual(first.steps, wantSteps) || first.terminalCalls == 0 {
		t.Fatalf("staged recursive Mu result=%d steps=%#v terminal=%d, want result 3, steps %#v, terminal > 0", first.result, first.steps, first.terminalCalls, wantSteps)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("staged recursive Mu Solve was not deterministic: first=%#v second=%#v", first, second)
	}
}

type stagedRecursiveMuStep struct {
	control uint64
	tag     uint64
	value   uint64
}

type stagedRecursiveMuOutcome struct {
	result        uint64
	steps         []stagedRecursiveMuStep
	locatorCalls  int
	transferCalls int
	terminalCalls int
}

// solveStagedRecursiveMuLaw intentionally uses the established WTO source
// fixture for a one-point recurrence. The only dynamic edge is SelectRead:
// its route is selected from the completed control read for the current Mu
// iteration, never from topology construction or a recursive Go callback.
func solveStagedRecursiveMuLaw(t testing.TB) stagedRecursiveMuOutcome {
	t.Helper()
	composition := NewComposition()
	controlSpec := coldFactorSpec(coldKey(121_000))
	controlSpec.KeyEnd = 1
	controlSpec.WidenRank = Measure[uint64, uint64]{
		Width: 1,
		At:    func(_ uint64, value uint64, _ int) uint64 { return ^value },
	}
	control, controlOK := DeclareFactor(composition, controlSpec, func(*Factor[uint64, uint64]) bool { return true })
	payload, payloadOK := DeclareFactor(composition, coldFactorSpec(coldKey(121_001)), func(*Factor[uint64, uint64]) bool { return true })
	controlRead, controlReadOK := ExactReadForm(control)
	controlWrite, controlWriteOK := ExactWriteForm(control)
	controlCarry, controlCarryOK := Carry(control)
	payloadRead, payloadReadOK := ExactReadForm(payload)
	payloadWrite, payloadWriteOK := ExactWriteForm(payload)
	if !controlOK || control == nil || !payloadOK || payload == nil || !controlReadOK || !controlWriteOK || !controlCarryOK || !payloadReadOK || !payloadWriteOK {
		t.Fatal("staged recursive Mu factors/forms")
	}

	var controlSeedWrite, payloadZeroWrite, payloadOneWrite Write[uint64]
	controlSeed, controlSeedOK := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{
		OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent,
		Semantic: coldKey(121_002), Output: control.Output(), Inputs: 0, Admission: testTrustedTheorem[uint64](121_102),
		Transfer: func(access Access[uint64, ruleUnit]) bool {
			return Product(access, func(row Row) bool { return StageValue(access, row, uint64(1)) })
		},
	}, func(rule *Rule[uint64, ruleUnit]) bool {
		var declared bool
		controlSeedWrite, declared = WriteTo(rule, controlWrite)
		return declared
	})
	payloadZero, payloadZeroOK := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{
		OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent,
		Semantic: coldKey(121_003), Output: payload.Output(), Inputs: 0, Admission: testTrustedTheorem[uint64](121_103),
		Transfer: func(access Access[uint64, ruleUnit]) bool {
			return Product(access, func(row Row) bool { return StageValue(access, row, uint64(2)) })
		},
	}, func(rule *Rule[uint64, ruleUnit]) bool {
		var declared bool
		payloadZeroWrite, declared = WriteTo(rule, payloadWrite)
		return declared
	})
	payloadOne, payloadOneOK := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{
		OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent,
		Semantic: coldKey(121_004), Output: payload.Output(), Inputs: 0, Admission: testTrustedTheorem[uint64](121_104),
		Transfer: func(access Access[uint64, ruleUnit]) bool {
			return Product(access, func(row Row) bool { return StageValue(access, row, uint64(3)) })
		},
	}, func(rule *Rule[uint64, ruleUnit]) bool {
		var declared bool
		payloadOneWrite, declared = WriteTo(rule, payloadWrite)
		return declared
	})

	var (
		controlValue   Read[OrderedCells[uint64]]
		selection      Read[Selection[uint64, OrderedCells[uint64]]]
		recursiveWrite Write[uint64]
		payloadZeroRef Ref[uint64]
		payloadOneRef  Ref[uint64]
		traceMu        sync.Mutex
		outcome        stagedRecursiveMuOutcome
	)
	recursive, recursiveOK := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{
		OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent,
		Semantic: coldKey(121_005), Output: control.Output(), Inputs: 1, Admission: testTrustedTheorem[uint64](121_105),
		Transfer: func(access Access[uint64, ruleUnit]) bool {
			traceMu.Lock()
			outcome.transferCalls++
			traceMu.Unlock()
			return Product(access, func(row Row) bool {
				controlCells, controlOK := ReadValue(access, row, controlValue)
				selected, selectionOK := ReadValue(access, row, selection)
				count, countOK := SelectionCount(access, row, selected)
				if !controlOK || !selectionOK || !countOK {
					return false
				}
				if count == 0 {
					return NoCandidate(access, row)
				}
				if count != 1 {
					return false
				}
				tag, payloadCells, selectedOK := SelectionAt(access, row, selected, 0)
				if !selectedOK {
					return false
				}
				control := waveCCell(controlCells)
				value := waveCCell(payloadCells)
				if value == 0 || value <= control {
					traceMu.Lock()
					outcome.terminalCalls++
					traceMu.Unlock()
					return NoCandidate(access, row)
				}
				traceMu.Lock()
				outcome.steps = append(outcome.steps, stagedRecursiveMuStep{control: control, tag: tag, value: value})
				traceMu.Unlock()
				return StageValue(access, row, value)
			})
		},
	}, func(rule *Rule[uint64, ruleUnit]) bool {
		input, inputOK := rule.InputAt(0)
		var controlOK, selectionOK, writeOK bool
		controlValue, controlOK = ReadFrom(rule, input, controlRead)
		selection, selectionOK = SelectRead[uint64, ruleUnit, uint64, OrderedCells[uint64], uint64](rule, input, payloadRead, []Dependency{ReadDependency(controlValue)}, func(context SelectorContext, _ ruleUnit) bool {
			traceMu.Lock()
			outcome.locatorCalls++
			traceMu.Unlock()
			controlCells, readable := SelectorRead(context, controlValue)
			if !readable {
				return false
			}
			switch waveCCell(controlCells) {
			case 0:
				return true
			case 1:
				return SelectRoute(context, payloadZeroRef, uint64(1))
			default:
				return SelectRoute(context, payloadOneRef, uint64(2))
			}
		})
		recursiveWrite, writeOK = WriteTo(rule, controlWrite)
		return inputOK && controlOK && selectionOK && writeOK && CarryFrom(rule, input, controlCarry)
	})

	var queryRead QueryRead[OrderedCells[uint64]]
	query, queryOK := DeclareQuery(composition, QuerySpec[uint64]{
		Semantic: coldKey(121_006),
		Project: func(observation Observation) uint64 {
			var result uint64
			rows := 0
			if !ProjectRows(observation, func(row QueryRow) bool {
				cells, readable := QueryValue(row, queryRead)
				value, present, valid := cells.At(0)
				if !readable || cells.Count() != 1 || !valid || !present {
					return false
				}
				result, rows = value, rows+1
				return true
			}) || rows != 1 {
				return 0
			}
			return result
		},
		Result: frozenColdResult(coldKey(121_106)),
	}, func(query *Query[uint64]) bool {
		var declared bool
		queryRead, declared = QueryReadFrom(query, controlRead)
		return declared
	})
	if !controlSeedOK || controlSeed == nil || !payloadZeroOK || payloadZero == nil || !payloadOneOK || payloadOne == nil || !recursiveOK || recursive == nil || !queryOK || query == nil || !composition.Seal() {
		t.Fatal("staged recursive Mu declarations")
	}
	controlRef, controlRefOK := control.Ref(0)
	var payloadZeroRefOK, payloadOneRefOK bool
	payloadZeroRef, payloadZeroRefOK = payload.Ref(0)
	payloadOneRef, payloadOneRefOK = payload.Ref(1)
	if !controlRefOK || !payloadZeroRefOK || !payloadOneRefOK {
		t.Fatal("staged recursive Mu refs")
	}

	rules := []*Rule[uint64, ruleUnit]{controlSeed, payloadZero, payloadOne, recursive}
	instances := make([]*RuleInstance[uint64, ruleUnit], len(rules))
	instanceOK := make([]bool, len(rules))
	instances[0], instanceOK[0] = NewRuleInstance(rules[0], ruleUnitForSemantic(coldKey(121_110)), func(binding *RuleBinding[uint64, ruleUnit]) bool {
		return InstanceWrite(binding, controlSeedWrite, controlRef)
	})
	instances[1], instanceOK[1] = NewRuleInstance(rules[1], ruleUnitForSemantic(coldKey(121_111)), func(binding *RuleBinding[uint64, ruleUnit]) bool {
		return InstanceWrite(binding, payloadZeroWrite, payloadZeroRef)
	})
	instances[2], instanceOK[2] = NewRuleInstance(rules[2], ruleUnitForSemantic(coldKey(121_112)), func(binding *RuleBinding[uint64, ruleUnit]) bool {
		return InstanceWrite(binding, payloadOneWrite, payloadOneRef)
	})
	instances[3], instanceOK[3] = NewRuleInstance(rules[3], ruleUnitForSemantic(coldKey(121_113)), func(binding *RuleBinding[uint64, ruleUnit]) bool {
		return InstanceRead(binding, controlValue, controlRef) &&
			InstanceSelectorRead(binding, selection, payloadRead) &&
			InstanceWrite(binding, recursiveWrite, controlRef)
	})
	for index, okay := range instanceOK {
		if !okay || instances[index] == nil {
			t.Fatalf("staged recursive Mu rule instance %d", index)
		}
	}

	batch, sites, occurrences, operands, batchOK := wtoSourceRows(121_120, 121_130, 1, []int{0, 0, 0, 0}, instances)
	if !batchOK || len(sites) != 1 || len(occurrences) != len(rules) || len(operands) != len(rules) {
		t.Fatal("staged recursive Mu source batch")
	}
	boundary, boundaryOK := wtoIdentityBoundary(sites[0], sites[0], coldKey(121_140))
	if !boundaryOK {
		t.Fatal("staged recursive Mu boundary")
	}
	var queryInstance *QueryInstance[uint64]
	solver, compiled := assemble(composition, batch, func(assembly *Assembly) bool {
		point := admitPoint(assembly, sites[0])
		members := make([]*assemblyMember, len(rules))
		for index := range rules {
			members[index] = admitInstance(assembly, point, occurrences[index], operands[index], instances[index])
			if point == nil || members[index] == nil {
				return false
			}
		}
		seedGroup := admitGroup(assembly, point, members[0])
		payloadZeroGroup := admitGroup(assembly, point, members[1])
		payloadOneGroup := admitGroup(assembly, point, members[2])
		recursiveGroup := admitGroup(assembly, point, members[3])
		var queryInstanceOK bool
		queryInstance, queryInstanceOK = NewQueryInstance(query, func(binding *QueryBinding[uint64]) bool {
			return InstanceQueryRead(binding, queryRead, controlRef)
		})
		return seedGroup != nil && payloadZeroGroup != nil && payloadOneGroup != nil && recursiveGroup != nil &&
			queryInstanceOK && admitQueryAt(assembly, point, queryInstance) != nil && admitBoundary(assembly, recursiveGroup, boundary)
	})
	if !compiled || solver == nil {
		t.Fatal("staged recursive Mu solver")
	}
	state, status := solver.Solve(context.Background())
	receipt, receiptOK := queryInstance.Receipt()
	result, readable := QueryResult(receipt, state)
	if status != SolveComplete || state == nil || !receiptOK || !readable {
		t.Fatalf("staged recursive Mu Solve = state:%v status:%v result:%d readable:%t", state, status, result, readable)
	}
	traceMu.Lock()
	outcome.result = result
	outcome.steps = append([]stagedRecursiveMuStep(nil), outcome.steps...)
	traceMu.Unlock()
	return outcome
}
