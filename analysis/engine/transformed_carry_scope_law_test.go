package engine

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
)

// carryLimitOperand is deliberately an ordinary immutable Rule operand.  Its
// limit is the declared transform parameter, not mutable solver state.
type carryLimitOperand struct{ limit uint64 }

func carryLimitOperandContent(value carryLimitOperand) (carryLimitOperand, [32]byte, bool) {
	if value.limit == 0 {
		return carryLimitOperand{}, [32]byte{}, false
	}
	return value, coldKey(173_000 + value.limit).Digest(), true
}

type transformedCarryPairQuery struct {
	query       *Query[uint64]
	left, right QueryRead[OrderedCells[uint64]]
}

func declareTransformedCarryPairQuery(t testing.TB, composition *Composition, semantic uint64, read ReadForm[uint64, OrderedCells[uint64]]) transformedCarryPairQuery {
	t.Helper()
	var left, right QueryRead[OrderedCells[uint64]]
	query, declared := DeclareQuery(composition, QuerySpec[uint64]{
		Semantic: coldKey(semantic),
		Project: func(observation Observation) uint64 {
			value, rows := uint64(0), 0
			if !ProjectRows(observation, func(row QueryRow) bool {
				leftCells, leftOK := QueryValue(row, left)
				rightCells, rightOK := QueryValue(row, right)
				leftValue, leftPresent, leftValid := leftCells.At(0)
				rightValue, rightPresent, rightValid := rightCells.At(0)
				if !leftOK || !rightOK || !leftValid || !rightValid || !leftPresent || !rightPresent {
					return false
				}
				value, rows = leftValue*100+rightValue, rows+1
				return true
			}) || rows != 1 {
				return 0
			}
			return value
		},
		Result: frozenColdResult(coldKey(semantic + 1)),
	}, func(query *Query[uint64]) bool {
		var leftOK, rightOK bool
		left, leftOK = QueryReadFrom(query, read)
		right, rightOK = QueryReadFrom(query, read)
		return leftOK && rightOK
	})
	if !declared || query == nil {
		t.Fatal("transformed carry pair query")
	}
	return transformedCarryPairQuery{query: query, left: left, right: right}
}

// TestTransformedCarryBindsEachRuleOperandExactlyOnce exercises two instances
// of one declaration in one assembly.  Their transform form is shared, but
// their frozen operand selects a distinct limit.  A binder that retained a
// declaration-global transform closure would make one target observe the
// other target's limit.
func TestTransformedCarryBindsEachRuleOperandExactlyOnce(t *testing.T) {
	composition := NewComposition()
	spec := coldFactorSpec(coldKey(173_010))
	spec.KeyEnd = 2
	factor, factorOK := DeclareFactor(composition, spec, func(*Factor[uint64, uint64]) bool { return true })
	read, readOK := ExactReadForm(factor)
	write, writeOK := ExactWriteForm(factor)
	carryForm, carryFormOK := Carry(factor)
	if !factorOK || factor == nil || !readOK || !writeOK || !carryFormOK {
		t.Fatal("transformed carry operand-isolation factor/forms")
	}

	var sourceLeft, sourceRight Write[uint64]
	source, sourceOK := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{
		Semantic: coldKey(173_012), OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent,
		Output: factor.Output(), Inputs: 0, Admission: testTrustedTheorem[uint64](173_013),
		Transfer: func(access Access[uint64, ruleUnit]) bool {
			return Product(access, func(row Row) bool { return StageValue(access, row, uint64(3)) })
		},
	}, func(rule *Rule[uint64, ruleUnit]) bool {
		var leftOK, rightOK bool
		sourceLeft, leftOK = WriteTo(rule, write)
		sourceRight, rightOK = WriteTo(rule, write)
		return leftOK && rightOK
	})
	carry, carryOK := DeclareRule(composition, RuleSpec[uint64, carryLimitOperand]{
		Semantic: coldKey(173_014), OperandFamily: coldKey(173_015), OperandContent: carryLimitOperandContent,
		Output: factor.Output(), Inputs: 1, Admission: AdmitRuleByTrustedTheorem[uint64, carryLimitOperand](coldKey(173_016)),
		Transfer: func(access Access[uint64, carryLimitOperand]) bool {
			return Product(access, func(row Row) bool { return StageTransform(access, row) })
		},
	}, func(rule *Rule[uint64, carryLimitOperand]) bool {
		input, inputOK := rule.InputAt(0)
		return inputOK && TransformCarryFrom(rule, input, carryForm, coldKey(173_011), func(operand carryLimitOperand, value uint64) (uint64, bool) {
			if value == 0 {
				return 0, true
			}
			if value > operand.limit {
				return operand.limit, true
			}
			return value, true
		})
	})
	queryOne := declareTransformedCarryPairQuery(t, composition, 173_017, read)
	queryTwo := declareTransformedCarryPairQuery(t, composition, 173_019, read)
	if !sourceOK || source == nil || !carryOK || carry == nil || !composition.Seal() {
		t.Fatal("transformed carry operand-isolation declarations")
	}

	leftRef, leftRefOK := factor.Ref(0)
	rightRef, rightRefOK := factor.Ref(1)
	sourceOne, sourceOneOK := NewRuleInstance(source, ruleUnitForSemantic(coldKey(173_020)), func(binding *RuleBinding[uint64, ruleUnit]) bool {
		return InstanceWrite(binding, sourceLeft, leftRef) && InstanceWrite(binding, sourceRight, rightRef)
	})
	sourceTwo, sourceTwoOK := NewRuleInstance(source, ruleUnitForSemantic(coldKey(173_021)), func(binding *RuleBinding[uint64, ruleUnit]) bool {
		return InstanceWrite(binding, sourceLeft, leftRef) && InstanceWrite(binding, sourceRight, rightRef)
	})
	carryOne, carryOneOK := NewRuleInstance(carry, carryLimitOperand{limit: 1}, func(*RuleBinding[uint64, carryLimitOperand]) bool { return true })
	carryTwo, carryTwoOK := NewRuleInstance(carry, carryLimitOperand{limit: 2}, func(*RuleBinding[uint64, carryLimitOperand]) bool { return true })
	if !leftRefOK || !rightRefOK || !sourceOneOK || !sourceTwoOK || !carryOneOK || !carryTwoOK {
		t.Fatal("transformed carry operand-isolation instances")
	}

	batch := equation.NewBatch()
	scope := equation.EmptyScope()
	sourceOneSite, sourceOneSiteOK := batch.AdmitSite(coldKey(173_030).compositionKey(), scope, equation.TrueExpr(), equation.InitPresent)
	sourceTwoSite, sourceTwoSiteOK := batch.AdmitSite(coldKey(173_031).compositionKey(), scope, equation.TrueExpr(), equation.InitPresent)
	targetOneSite, targetOneSiteOK := batch.AdmitSite(coldKey(173_032).compositionKey(), scope, equation.FalseExpr(), equation.InitAbsent)
	targetTwoSite, targetTwoSiteOK := batch.AdmitSite(coldKey(173_033).compositionKey(), scope, equation.FalseExpr(), equation.InitAbsent)
	sourceOneOccurrence, sourceOneOccurred := batch.Relation(sourceOneSite, coldKey(173_034).compositionKey())
	sourceTwoOccurrence, sourceTwoOccurred := batch.Relation(sourceTwoSite, coldKey(173_035).compositionKey())
	targetOneOccurrence, targetOneOccurred := batch.Relation(targetOneSite, coldKey(173_036).compositionKey())
	targetTwoOccurrence, targetTwoOccurred := batch.Relation(targetTwoSite, coldKey(173_037).compositionKey())
	sourceOneOperand, sourceOneOperandOK := admitInstanceOperand(batch, sourceOneOccurrence, sourceOne)
	sourceTwoOperand, sourceTwoOperandOK := admitInstanceOperand(batch, sourceTwoOccurrence, sourceTwo)
	targetOneOperand, targetOneOperandOK := admitInstanceOperand(batch, targetOneOccurrence, carryOne)
	targetTwoOperand, targetTwoOperandOK := admitInstanceOperand(batch, targetTwoOccurrence, carryTwo)
	if !scope.Available() || !sourceOneSiteOK || !sourceTwoSiteOK || !targetOneSiteOK || !targetTwoSiteOK || !sourceOneOccurred || !sourceTwoOccurred || !targetOneOccurred || !targetTwoOccurred || !sourceOneOperandOK || !sourceTwoOperandOK || !targetOneOperandOK || !targetTwoOperandOK || !batch.Seal() {
		t.Fatal("transformed carry operand-isolation batch")
	}
	firstBoundary, firstBoundaryOK := wtoIdentityBoundary(sourceOneSite, targetOneSite, coldKey(173_038))
	secondBoundary, secondBoundaryOK := wtoIdentityBoundary(sourceTwoSite, targetTwoSite, coldKey(173_039))
	var firstQueryInstance, secondQueryInstance *QueryInstance[uint64]
	solver, compiled := assemble(composition, batch, func(assembly *Assembly) bool {
		sourceOnePoint, sourceTwoPoint := admitPoint(assembly, sourceOneSite), admitPoint(assembly, sourceTwoSite)
		targetOnePoint, targetTwoPoint := admitPoint(assembly, targetOneSite), admitPoint(assembly, targetTwoSite)
		sourceOneMember := admitInstance(assembly, sourceOnePoint, sourceOneOccurrence, sourceOneOperand, sourceOne)
		sourceTwoMember := admitInstance(assembly, sourceTwoPoint, sourceTwoOccurrence, sourceTwoOperand, sourceTwo)
		targetOneMember := admitInstance(assembly, targetOnePoint, targetOneOccurrence, targetOneOperand, carryOne)
		targetTwoMember := admitInstance(assembly, targetTwoPoint, targetTwoOccurrence, targetTwoOperand, carryTwo)
		firstGroup := admitGroup(assembly, targetOnePoint, targetOneMember)
		secondGroup := admitGroup(assembly, targetTwoPoint, targetTwoMember)
		var firstQueryOK, secondQueryOK bool
		firstQueryInstance, firstQueryOK = NewQueryInstance(queryOne.query, func(binding *QueryBinding[uint64]) bool {
			return InstanceQueryRead(binding, queryOne.left, leftRef) && InstanceQueryRead(binding, queryOne.right, rightRef)
		})
		secondQueryInstance, secondQueryOK = NewQueryInstance(queryTwo.query, func(binding *QueryBinding[uint64]) bool {
			return InstanceQueryRead(binding, queryTwo.left, leftRef) && InstanceQueryRead(binding, queryTwo.right, rightRef)
		})
		return sourceOnePoint != nil && sourceTwoPoint != nil && targetOnePoint != nil && targetTwoPoint != nil && sourceOneMember != nil && sourceTwoMember != nil && targetOneMember != nil && targetTwoMember != nil &&
			admitGroup(assembly, sourceOnePoint, sourceOneMember) != nil && admitGroup(assembly, sourceTwoPoint, sourceTwoMember) != nil && firstGroup != nil && secondGroup != nil && firstBoundaryOK && secondBoundaryOK &&
			admitBoundary(assembly, firstGroup, firstBoundary) && admitBoundary(assembly, secondGroup, secondBoundary) && firstQueryOK && secondQueryOK && admitQueryAt(assembly, targetOnePoint, firstQueryInstance) != nil && admitQueryAt(assembly, targetTwoPoint, secondQueryInstance) != nil
	})
	if !compiled || solver == nil {
		t.Fatal("transformed carry operand-isolation assembly")
	}
	state, status := solver.Solve(context.Background())
	firstReceipt, firstReceiptOK := firstQueryInstance.Receipt()
	secondReceipt, secondReceiptOK := secondQueryInstance.Receipt()
	first, firstOK := QueryResult(firstReceipt, state)
	second, secondOK := QueryResult(secondReceipt, state)
	if status != SolveComplete || state == nil || !firstReceiptOK || !secondReceiptOK || !firstOK || !secondOK || first != 101 || second != 202 {
		t.Fatalf("transformed carry operand isolation = state:%v status:%v first:%d/%t second:%d/%t, want 101 and 202", state, status, first, firstOK, second, secondOK)
	}
}

// TestTransformedCarryClosesRouteUniverseOnce is the public route-closure
// law.  The route rule retains its static heap input and also emits all three
// keys through a dynamic selector.  The next transformed carry must age the
// whole resulting closure, and identical static/route terminal values are
// transformed once per patch rather than once per occurrence.
func TestTransformedCarryClosesRouteUniverseOnce(t *testing.T) {
	composition := NewComposition()
	controlSpec := coldFactorSpec(coldKey(173_100))
	controlSpec.KeyEnd = 1
	control, controlOK := DeclareFactor(composition, controlSpec, func(*Factor[uint64, uint64]) bool { return true })
	heapSpec := coldFactorSpec(coldKey(173_101))
	// Keep one route-only coordinate beyond the authored static carry closure.
	// The transformed member must retain the static closure plus a route bit;
	// retaining the route universe in carryTargets would make this member's
	// target vector grow from 3 to 4 here.
	heapSpec.KeyEnd = 4
	mappedShared := 0
	heap, heapOK := DeclareFactor(composition, heapSpec, func(*Factor[uint64, uint64]) bool { return true })
	controlRead, controlReadOK := ExactReadForm(control)
	controlWrite, controlWriteOK := ExactWriteForm(control)
	heapRead, heapReadOK := ExactReadForm(heap)
	heapWrite, heapWriteOK := ExactWriteForm(heap)
	heapCarry, heapCarryOK := Carry(heap)
	if !controlOK || control == nil || !heapOK || heap == nil || !controlReadOK || !controlWriteOK || !heapReadOK || !heapWriteOK || !heapCarryOK {
		t.Fatal("transformed route closure factors/forms")
	}

	var controlToken, heapZeroToken, heapOneToken, heapTwoToken Write[uint64]
	controlSeed, controlSeedOK := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{
		Semantic: coldKey(173_103), OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent,
		Output: control.Output(), Inputs: 0, Admission: testTrustedTheorem[uint64](173_104),
		Transfer: func(access Access[uint64, ruleUnit]) bool {
			return Product(access, func(row Row) bool { return StageValue(access, row, uint64(1)) })
		},
	}, func(rule *Rule[uint64, ruleUnit]) bool {
		var declared bool
		controlToken, declared = WriteTo(rule, controlWrite)
		return declared
	})
	heapSeed, heapSeedOK := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{
		Semantic: coldKey(173_105), OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent,
		Output: heap.Output(), Inputs: 0, Admission: testTrustedTheorem[uint64](173_106),
		Transfer: func(access Access[uint64, ruleUnit]) bool {
			return Product(access, func(row Row) bool { return StageValue(access, row, uint64(2)) })
		},
	}, func(rule *Rule[uint64, ruleUnit]) bool {
		var zeroOK, oneOK, twoOK bool
		heapZeroToken, zeroOK = WriteTo(rule, heapWrite)
		heapOneToken, oneOK = WriteTo(rule, heapWrite)
		heapTwoToken, twoOK = WriteTo(rule, heapWrite)
		return zeroOK && oneOK && twoOK
	})

	var trigger Read[OrderedCells[uint64]]
	var selection Read[Selection[uint64, OrderedCells[uint64]]]
	var routeToken Write[uint64]
	var routeRefs []Ref[uint64]
	route, routeOK := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{
		Semantic: coldKey(173_107), OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent,
		Output: heap.Output(), Inputs: 2, Admission: testTrustedTheorem[uint64](173_108),
		Transfer: func(access Access[uint64, ruleUnit]) bool {
			return Product(access, func(row Row) bool {
				selected, selectedOK := ReadValue(access, row, selection)
				count, countOK := SelectionCount(access, row, selected)
				if !selectedOK || !countOK || count != len(routeRefs) {
					return false
				}
				return StageSelection(access, row, selected, func(uint64, OrderedCells[uint64]) (uint64, bool) { return 2, true })
			})
		},
	}, func(rule *Rule[uint64, ruleUnit]) bool {
		controlInput, controlInputOK := rule.InputAt(0)
		heapInput, heapInputOK := rule.InputAt(1)
		var triggerOK, selectionOK, routeOK bool
		trigger, triggerOK = ReadFrom(rule, controlInput, controlRead)
		selection, selectionOK = SelectRead[uint64, ruleUnit, uint64, OrderedCells[uint64], uint64](rule, heapInput, heapRead, []Dependency{ReadDependency(trigger)}, func(selector SelectorContext, _ ruleUnit) bool {
			cells, readable := SelectorRead(selector, trigger)
			if !readable || waveCCell(cells) != 1 || len(routeRefs) != 3 {
				return false
			}
			for index, ref := range routeRefs {
				if !SelectRoute(selector, ref, uint64(index+1)) {
					return false
				}
			}
			return true
		})
		routeToken, routeOK = RouteWrite(rule, selection)
		return controlInputOK && heapInputOK && triggerOK && selectionOK && routeOK && CarryFrom(rule, heapInput, heapCarry)
	})
	aged, agedOK := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{
		Semantic: coldKey(173_109), OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent,
		Output: heap.Output(), Inputs: 1, Admission: testTrustedTheorem[uint64](173_110),
		Transfer: func(access Access[uint64, ruleUnit]) bool {
			return Product(access, func(row Row) bool { return StageTransform(access, row) })
		},
	}, func(rule *Rule[uint64, ruleUnit]) bool {
		input, inputOK := rule.InputAt(0)
		return inputOK && TransformCarryFrom(rule, input, heapCarry, coldKey(173_102), func(_ ruleUnit, value uint64) (uint64, bool) {
			switch value {
			case 0:
				return 0, true
			case 2:
				mappedShared++
				return 1, true
			case 1:
				return 1, true
			default:
				return 0, false
			}
		})
	})
	if !controlSeedOK || controlSeed == nil || !heapSeedOK || heapSeed == nil || !routeOK || route == nil || !agedOK || aged == nil {
		t.Fatal("transformed route closure rules")
	}

	var zeroRead, oneRead, twoRead QueryRead[OrderedCells[uint64]]
	query, queryOK := DeclareQuery(composition, QuerySpec[uint64]{
		Semantic: coldKey(173_111),
		Project: func(observation Observation) uint64 {
			result, rows := uint64(0), 0
			if !ProjectRows(observation, func(row QueryRow) bool {
				zeroCells, zeroOK := QueryValue(row, zeroRead)
				oneCells, oneOK := QueryValue(row, oneRead)
				twoCells, twoOK := QueryValue(row, twoRead)
				zero, zeroPresent, zeroValid := zeroCells.At(0)
				one, onePresent, oneValid := oneCells.At(0)
				two, twoPresent, twoValid := twoCells.At(0)
				if !zeroOK || !oneOK || !twoOK || !zeroValid || !oneValid || !twoValid || !zeroPresent || !onePresent || !twoPresent {
					return false
				}
				result, rows = zero<<16|one<<8|two, rows+1
				return true
			}) || rows != 1 {
				return 0
			}
			return result
		},
		Result: frozenColdResult(coldKey(173_112)),
	}, func(query *Query[uint64]) bool {
		var zeroOK, oneOK, twoOK bool
		zeroRead, zeroOK = QueryReadFrom(query, heapRead)
		oneRead, oneOK = QueryReadFrom(query, heapRead)
		twoRead, twoOK = QueryReadFrom(query, heapRead)
		return zeroOK && oneOK && twoOK
	})
	if !queryOK || query == nil || !composition.Seal() {
		t.Fatal("transformed route closure query/seal")
	}
	controlRef, controlRefOK := control.Ref(0)
	heapZero, heapZeroOK := heap.Ref(0)
	heapOne, heapOneOK := heap.Ref(1)
	heapTwo, heapTwoOK := heap.Ref(2)
	routeRefs = []Ref[uint64]{heapZero, heapOne, heapTwo}
	if !controlRefOK || !heapZeroOK || !heapOneOK || !heapTwoOK {
		t.Fatal("transformed route closure refs")
	}

	batch := equation.NewBatch()
	scope := equation.EmptyScope()
	sourceSite, sourceSiteOK := batch.AdmitSite(coldKey(173_120).compositionKey(), scope, equation.TrueExpr(), equation.InitPresent)
	routeSite, routeSiteOK := batch.AdmitSite(coldKey(173_121).compositionKey(), scope, equation.FalseExpr(), equation.InitAbsent)
	agedSite, agedSiteOK := batch.AdmitSite(coldKey(173_122).compositionKey(), scope, equation.FalseExpr(), equation.InitAbsent)
	controlOccurrence, controlOccurred := batch.Relation(sourceSite, coldKey(173_123).compositionKey())
	heapOccurrence, heapOccurred := batch.Relation(sourceSite, coldKey(173_124).compositionKey())
	routeOccurrence, routeOccurred := batch.Relation(routeSite, coldKey(173_125).compositionKey())
	agedOccurrence, agedOccurred := batch.Relation(agedSite, coldKey(173_126).compositionKey())
	controlInstance, controlInstanceOK := NewRuleInstance(controlSeed, ruleUnitForSemantic(coldKey(173_127)), func(binding *RuleBinding[uint64, ruleUnit]) bool {
		return InstanceWrite(binding, controlToken, controlRef)
	})
	heapInstance, heapInstanceOK := NewRuleInstance(heapSeed, ruleUnitForSemantic(coldKey(173_128)), func(binding *RuleBinding[uint64, ruleUnit]) bool {
		return InstanceWrite(binding, heapZeroToken, heapZero) && InstanceWrite(binding, heapOneToken, heapOne) && InstanceWrite(binding, heapTwoToken, heapTwo)
	})
	routeInstance, routeInstanceOK := NewRuleInstance(route, ruleUnitForSemantic(coldKey(173_129)), func(binding *RuleBinding[uint64, ruleUnit]) bool {
		return InstanceRead(binding, trigger, controlRef) && InstanceSelectorRead(binding, selection, heapRead) && InstanceRouteWrite(binding, routeToken, selection)
	})
	agedInstance, agedInstanceOK := NewRuleInstance(aged, ruleUnitForSemantic(coldKey(173_130)), func(*RuleBinding[uint64, ruleUnit]) bool { return true })
	controlOperand, controlOperandOK := admitInstanceOperand(batch, controlOccurrence, controlInstance)
	heapOperand, heapOperandOK := admitInstanceOperand(batch, heapOccurrence, heapInstance)
	routeOperand, routeOperandOK := admitInstanceOperand(batch, routeOccurrence, routeInstance)
	agedOperand, agedOperandOK := admitInstanceOperand(batch, agedOccurrence, agedInstance)
	if !scope.Available() || !sourceSiteOK || !routeSiteOK || !agedSiteOK || !controlOccurred || !heapOccurred || !routeOccurred || !agedOccurred || !controlInstanceOK || !heapInstanceOK || !routeInstanceOK || !agedInstanceOK || !controlOperandOK || !heapOperandOK || !routeOperandOK || !agedOperandOK || !batch.Seal() {
		t.Fatal("transformed route closure batch")
	}
	controlBoundary, controlBoundaryOK := wtoIdentityBoundary(sourceSite, routeSite, coldKey(173_131))
	heapBoundary, heapBoundaryOK := wtoIdentityBoundary(sourceSite, routeSite, coldKey(173_132))
	agedBoundary, agedBoundaryOK := wtoIdentityBoundary(routeSite, agedSite, coldKey(173_133))
	var queryInstance *QueryInstance[uint64]
	solver, compiled := assemble(composition, batch, func(assembly *Assembly) bool {
		sourcePoint, routePoint, agedPoint := admitPoint(assembly, sourceSite), admitPoint(assembly, routeSite), admitPoint(assembly, agedSite)
		controlMember := admitInstance(assembly, sourcePoint, controlOccurrence, controlOperand, controlInstance)
		heapMember := admitInstance(assembly, sourcePoint, heapOccurrence, heapOperand, heapInstance)
		routeMember := admitInstance(assembly, routePoint, routeOccurrence, routeOperand, routeInstance)
		agedMember := admitInstance(assembly, agedPoint, agedOccurrence, agedOperand, agedInstance)
		routeGroup, agedGroup := admitGroup(assembly, routePoint, routeMember), admitGroup(assembly, agedPoint, agedMember)
		var queryInstanceOK bool
		queryInstance, queryInstanceOK = NewQueryInstance(query, func(binding *QueryBinding[uint64]) bool {
			return InstanceQueryRead(binding, zeroRead, heapZero) && InstanceQueryRead(binding, oneRead, heapOne) && InstanceQueryRead(binding, twoRead, heapTwo)
		})
		return sourcePoint != nil && routePoint != nil && agedPoint != nil && controlMember != nil && heapMember != nil && routeMember != nil && agedMember != nil && admitGroup(assembly, sourcePoint, controlMember, heapMember) != nil && routeGroup != nil && agedGroup != nil &&
			controlBoundaryOK && heapBoundaryOK && agedBoundaryOK && admitBoundary(assembly, routeGroup, controlBoundary) && admitBoundary(assembly, routeGroup, heapBoundary) && admitBoundary(assembly, agedGroup, agedBoundary) && queryInstanceOK && admitQueryAt(assembly, agedPoint, queryInstance) != nil
	})
	if !compiled || solver == nil {
		t.Fatal("transformed route closure assembly")
	}
	routeTransformMembers := 0
	for _, producer := range solver.runtime.producers {
		for _, member := range producer.members {
			bound, boundOK := member.(*boundRuleMember[uint64, ruleUnit])
			if !boundOK || bound == nil || bound.rule == nil || !bound.rule.routeTransform {
				continue
			}
			routeTransformMembers++
			owner := bound.routeScope()
			if owner == nil || !owner.hasRouteUniverse() || len(bound.rule.carryTargets) >= len(owner.routeUniverse()) {
				t.Fatalf("transformed member retained route universe: owner=%t authored=%d route=%d", owner != nil && owner.hasRouteUniverse(), len(bound.rule.carryTargets), len(owner.routeUniverse()))
			}
		}
	}
	if routeTransformMembers != 1 {
		t.Fatalf("transformed route members=%d, want one owner-bit member", routeTransformMembers)
	}
	state, status := solver.Solve(context.Background())
	receipt, receiptOK := queryInstance.Receipt()
	result, readable := QueryResult(receipt, state)
	if status != SolveComplete || state == nil || !receiptOK || !readable || result != 0x010101 || mappedShared != 1 {
		t.Fatalf("transformed route closure = state:%v status:%v result:%#x readable:%t shared-maps:%d, want 0x010101 and one map", state, status, result, readable, mappedShared)
	}
}

// TestTransformedCarryRejectsNonmonotoneSelfSCCWithoutRestart proves that a
// transformed carry does not license a nonmonotone enclosing Rule. Although
// the private map 0->0, 2->1, 1->1 is monotone and idempotent, the Rule as a
// whole computes F(0)=2 and F(2)=1. Under the declared uint order that is a
// strict ascent-law violation. The solver must fail the episode once; it must
// never restart the identical 0->2->1 recurrence.
func TestTransformedCarryRejectsNonmonotoneSelfSCCWithoutRestart(t *testing.T) {
	composition := NewComposition()
	spec := coldFactorSpec(coldKey(173_200))
	spec.KeyEnd = 1
	spec.WidenRank = Measure[uint64, uint64]{Width: 1, At: func(_ uint64, value uint64, _ int) uint64 { return ^value }}
	factor, factorOK := DeclareFactor(composition, spec, func(*Factor[uint64, uint64]) bool { return true })
	read, readOK := ExactReadForm(factor)
	write, writeOK := ExactWriteForm(factor)
	carry, carryOK := Carry(factor)
	if !factorOK || factor == nil || !readOK || !writeOK || !carryOK {
		t.Fatal("transformed carry self-SCC factor/forms")
	}

	agedTwo, stableOne := 0, 0
	var inputRead Read[OrderedCells[uint64]]
	var seedWrite Write[uint64]
	loop, loopOK := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{
		Semantic: coldKey(173_201), OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent,
		Output: factor.Output(), Inputs: 1, Admission: testTrustedTheorem[uint64](173_202),
		Transfer: func(access Access[uint64, ruleUnit]) bool {
			return Product(access, func(row Row) bool {
				cells, readable := ReadValue(access, row, inputRead)
				value, present, valid := cells.At(0)
				if !readable || !valid {
					return false
				}
				if !present || value == 0 {
					return StageValue(access, row, uint64(2))
				}
				return StageTransform(access, row)
			})
		},
	}, func(rule *Rule[uint64, ruleUnit]) bool {
		input, inputOK := rule.InputAt(0)
		var readDeclared, writeDeclared bool
		inputRead, readDeclared = ReadFrom(rule, input, read)
		seedWrite, writeDeclared = WriteTo(rule, write)
		return inputOK && readDeclared && writeDeclared && TransformCarryFrom(rule, input, carry, coldKey(173_203), func(_ ruleUnit, value uint64) (uint64, bool) {
			switch value {
			case 0:
				return 0, true
			case 2:
				agedTwo++
				return 1, true
			case 1:
				stableOne++
				return 1, true
			default:
				return value, true
			}
		})
	})
	var queryRead QueryRead[OrderedCells[uint64]]
	query, queryOK := DeclareQuery(composition, QuerySpec[uint64]{
		Semantic: coldKey(173_204),
		Project: func(observation Observation) uint64 {
			value, rows := uint64(0), 0
			if !ProjectRows(observation, func(row QueryRow) bool {
				cells, readable := QueryValue(row, queryRead)
				cell, present, valid := cells.At(0)
				if !readable || !valid || !present {
					return false
				}
				value, rows = cell, rows+1
				return true
			}) || rows != 1 {
				return 0
			}
			return value
		},
		Result: frozenColdResult(coldKey(173_205)),
	}, func(query *Query[uint64]) bool {
		var declared bool
		queryRead, declared = QueryReadFrom(query, read)
		return declared
	})
	if !loopOK || loop == nil || !queryOK || query == nil || !composition.Seal() {
		t.Fatal("transformed carry self-SCC declarations")
	}
	ref, refOK := factor.Ref(0)
	instance, instanceOK := NewRuleInstance(loop, ruleUnitForSemantic(coldKey(173_206)), func(binding *RuleBinding[uint64, ruleUnit]) bool {
		return InstanceRead(binding, inputRead, ref) && InstanceWrite(binding, seedWrite, ref)
	})
	if !refOK || !instanceOK || instance == nil {
		t.Fatal("transformed carry self-SCC instance")
	}

	batch := equation.NewBatch()
	scope := equation.EmptyScope()
	site, siteOK := batch.AdmitSite(coldKey(173_207).compositionKey(), scope, equation.TrueExpr(), equation.InitPresent)
	occurrence, occurred := batch.Relation(site, coldKey(173_208).compositionKey())
	operand, operandOK := admitInstanceOperand(batch, occurrence, instance)
	if !scope.Available() || !siteOK || !occurred || !operandOK || !batch.Seal() {
		t.Fatal("transformed carry self-SCC batch")
	}
	boundary, boundaryOK := wtoIdentityBoundary(site, site, coldKey(173_209))
	solver, compiled := assemble(composition, batch, func(assembly *Assembly) bool {
		point := admitPoint(assembly, site)
		member := admitInstance(assembly, point, occurrence, operand, instance)
		group := admitGroup(assembly, point, member)
		queryInstance, queryInstanceOK := NewQueryInstance(query, func(binding *QueryBinding[uint64]) bool {
			return InstanceQueryRead(binding, queryRead, ref)
		})
		return point != nil && member != nil && group != nil && boundaryOK && admitBoundary(assembly, group, boundary) && queryInstanceOK && admitQueryAt(assembly, point, queryInstance) != nil
	})
	if !compiled || solver == nil {
		t.Fatal("transformed carry self-SCC assembly")
	}
	state, status := solver.Solve(context.Background())
	if state != nil || status != SolveIncomplete || agedTwo != 1 || stableOne != 0 {
		t.Fatalf("nonmonotone transformed carry = state:%v status:%v transforms:%d/%d, want one 2->1 observation and closed failure", state, status, agedTwo, stableOne)
	}
}
