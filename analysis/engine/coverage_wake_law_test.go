package engine

import (
	"context"
	"testing"
)

// TestCoverageOnlyDefaultPublicationWakesDependentCarry is a black-box law
// for the solver lifecycle distinction between NoCandidate and an authored
// Default. The control write first makes the conditional producer re-evaluate
// from NoCandidate to Default at the left Point. That publication changes no
// semantic Factor value: the left Point already denotes Default. Its exact
// authored coverage must nevertheless advance the Point and wake the carry at
// the right Point. There Default then participates in Join with 4, producing
// the sparse Default result 7. Without the coverage-only version/wake path the
// right Point remains 4 and this query fails.
func TestCoverageOnlyDefaultPublicationWakesDependentCarry(t *testing.T) {
	composition := NewComposition()
	valueSpec := coldFactorSpec(coldKey(99_001))
	valueSpec.KeyEnd, valueSpec.Default = 1, 7
	valueSpec.WidenRank = Measure[uint64, uint64]{Width: 1, At: func(_ uint64, value uint64, _ int) uint64 { return ^value }}
	value, valueOK := DeclareFactor(composition, valueSpec, func(*Factor[uint64, uint64]) bool { return true })
	controlSpec := coldFactorSpec(coldKey(99_002))
	controlSpec.KeyEnd = 1
	controlSpec.WidenRank = Measure[uint64, uint64]{Width: 1, At: func(_ uint64, value uint64, _ int) uint64 { return ^value }}
	control, controlOK := DeclareFactor(composition, controlSpec, func(*Factor[uint64, uint64]) bool { return true })
	valueRead, valueReadOK := ExactReadForm(value)
	valueWrite, valueWriteOK := ExactWriteForm(value)
	valueCarry, valueCarryOK := Carry(value)
	controlRead, controlReadOK := ExactReadForm(control)
	controlWrite, controlWriteOK := ExactWriteForm(control)
	if !valueOK || value == nil || !controlOK || control == nil || !valueReadOK || !valueWriteOK || !valueCarryOK || !controlReadOK || !controlWriteOK {
		t.Fatal("coverage wake factors/forms")
	}

	var seedWrite Write[uint64]
	seed, seedOK := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{
		OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent,
		Semantic: coldKey(99_003), Output: control.Output(), Inputs: 0, Admission: testTrustedTheorem[uint64](199_003),
		Transfer: func(access Access[uint64, ruleUnit]) bool {
			return Product(access, func(row Row) bool { return StageValue(access, row, uint64(1)) })
		},
	}, func(rule *Rule[uint64, ruleUnit]) bool {
		var ok bool
		seedWrite, ok = WriteTo(rule, controlWrite)
		return ok
	})

	producerTransfers := 0
	var producerRead Read[OrderedCells[uint64]]
	var producerWrite Write[uint64]
	producer, producerOK := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{
		OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent,
		Semantic: coldKey(99_004), Output: value.Output(), Inputs: 2, Admission: testTrustedTheorem[uint64](199_004),
		Transfer: func(access Access[uint64, ruleUnit]) bool {
			producerTransfers++
			return Product(access, func(row Row) bool {
				cells, readable := ReadValue(access, row, producerRead)
				_, present, valid := cells.At(0)
				if !readable || cells.Count() != 1 || !valid {
					return false
				}
				if !present {
					return NoCandidate(access, row)
				}
				return StageValue(access, row, uint64(7))
			})
		},
	}, func(rule *Rule[uint64, ruleUnit]) bool {
		controlInput, controlInputOK := rule.InputAt(0)
		_, orderingInputOK := rule.InputAt(1)
		var readOK, writeOK bool
		producerRead, readOK = ReadFrom(rule, controlInput, controlRead)
		producerWrite, writeOK = WriteTo(rule, valueWrite)
		return controlInputOK && orderingInputOK && readOK && writeOK
	})

	var priorWrite Write[uint64]
	prior, priorOK := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{
		OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent,
		Semantic: coldKey(99_005), Output: value.Output(), Inputs: 0, Admission: testTrustedTheorem[uint64](199_005),
		Transfer: func(access Access[uint64, ruleUnit]) bool {
			return Product(access, func(row Row) bool { return StageValue(access, row, uint64(4)) })
		},
	}, func(rule *Rule[uint64, ruleUnit]) bool {
		var ok bool
		priorWrite, ok = WriteTo(rule, valueWrite)
		return ok
	})

	carryTransfers := 0
	carryRule, carryOK := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{
		OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent,
		Semantic: coldKey(99_006), Output: value.Output(), Inputs: 1, Admission: testTrustedTheorem[uint64](199_006),
		Transfer: func(access Access[uint64, ruleUnit]) bool {
			carryTransfers++
			return Product(access, func(Row) bool { return true })
		},
	}, func(rule *Rule[uint64, ruleUnit]) bool {
		input, inputOK := rule.InputAt(0)
		return inputOK && CarryFrom(rule, input, valueCarry)
	})

	var queryRead QueryRead[OrderedCells[uint64]]
	query, queryOK := DeclareQuery(composition, QuerySpec[uint64]{
		Semantic: coldKey(99_007),
		Project: func(observation Observation) uint64 {
			result, rows := uint64(0), 0
			if !ProjectRows(observation, func(row QueryRow) bool {
				cells, resolved := QueryValue(row, queryRead)
				cell, present, valid := cells.At(0)
				if !resolved || cells.Count() != 1 || !valid || present {
					return false
				}
				result, rows = cell, rows+1
				return true
			}) || rows != 1 {
				return 0
			}
			return result
		},
		Result: frozenColdResult(coldKey(99_008)),
	}, func(query *Query[uint64]) bool {
		var ok bool
		queryRead, ok = QueryReadFrom(query, valueRead)
		return ok
	})
	if !seedOK || seed == nil || !producerOK || producer == nil || !priorOK || prior == nil || !carryOK || carryRule == nil || !queryOK || query == nil || !composition.Seal() {
		t.Fatal("coverage wake declarations")
	}

	valueRef, valueRefOK := value.Ref(0)
	controlRef, controlRefOK := control.Ref(0)
	seedInstance, seedInstanceOK := NewRuleInstance(seed, ruleUnitForSemantic(coldKey(99_009)), func(binding *RuleBinding[uint64, ruleUnit]) bool {
		return InstanceWrite(binding, seedWrite, controlRef)
	})
	producerInstance, producerInstanceOK := NewRuleInstance(producer, ruleUnitForSemantic(coldKey(99_010)), func(binding *RuleBinding[uint64, ruleUnit]) bool {
		return InstanceRead(binding, producerRead, controlRef) && InstanceWrite(binding, producerWrite, valueRef)
	})
	priorInstance, priorInstanceOK := NewRuleInstance(prior, ruleUnitForSemantic(coldKey(99_011)), func(binding *RuleBinding[uint64, ruleUnit]) bool {
		return InstanceWrite(binding, priorWrite, valueRef)
	})
	carryInstance, carryInstanceOK := NewRuleInstance(carryRule, ruleUnitForSemantic(coldKey(99_012)), func(*RuleBinding[uint64, ruleUnit]) bool { return true })
	if !valueRefOK || !controlRefOK || !seedInstanceOK || !producerInstanceOK || !priorInstanceOK || !carryInstanceOK {
		t.Fatal("coverage wake instances")
	}

	source := NewSourceAssembly(composition)
	scope, scopeOK := source.Scope()
	truth, truthOK := source.TrueExpr()
	leftSite, leftSiteOK := source.Site(coldKey(99_013), scope, truth, true)
	rightSite, rightSiteOK := source.Site(coldKey(99_014), scope, truth, true)
	seedOccurrence, seedOccurrenceOK := source.Relation(leftSite, coldKey(99_015))
	producerOccurrence, producerOccurrenceOK := source.Relation(leftSite, coldKey(99_016))
	priorOccurrence, priorOccurrenceOK := source.Relation(rightSite, coldKey(99_017))
	carryOccurrence, carryOccurrenceOK := source.Relation(rightSite, coldKey(99_018))
	seedPrepared, seedPreparedOK := source.PrepareInstance(seedOccurrence, seedInstance)
	producerPrepared, producerPreparedOK := source.PrepareInstance(producerOccurrence, producerInstance)
	priorPrepared, priorPreparedOK := source.PrepareInstance(priorOccurrence, priorInstance)
	carryPrepared, carryPreparedOK := source.PrepareInstance(carryOccurrence, carryInstance)
	reindex, reindexOK := source.IdentityReindex(scope)
	leftSelf, leftSelfOK := source.Boundary(leftSite, leftSite, coldKey(99_019), truth, reindex, truth)
	rightToLeft, rightToLeftOK := source.Boundary(rightSite, leftSite, coldKey(99_020), truth, reindex, truth)
	leftToRight, leftToRightOK := source.Boundary(leftSite, rightSite, coldKey(99_021), truth, reindex, truth)
	if source == nil || !scopeOK || !truthOK || !leftSiteOK || !rightSiteOK || !seedOccurrenceOK || !producerOccurrenceOK || !priorOccurrenceOK || !carryOccurrenceOK || !seedPreparedOK || !producerPreparedOK || !priorPreparedOK || !carryPreparedOK || !reindexOK || !leftSelfOK || !rightToLeftOK || !leftToRightOK || !source.Seal() {
		t.Fatal("coverage wake SourceAssembly")
	}

	var queryInstance *QueryInstance[uint64]
	solver, assembled := source.Assemble(func(assembly *Assembly) bool {
		leftPoint, leftPointOK := assembly.Point(leftSite)
		rightPoint, rightPointOK := assembly.Point(rightSite)
		seedMember, seedMemberOK := assembly.Member(leftPoint, seedPrepared)
		producerMember, producerMemberOK := assembly.Member(leftPoint, producerPrepared)
		priorMember, priorMemberOK := assembly.Member(rightPoint, priorPrepared)
		carryMember, carryMemberOK := assembly.Member(rightPoint, carryPrepared)
		_, seedGroupOK := assembly.Group(leftPoint, seedMember)
		producerGroup, producerGroupOK := assembly.Group(leftPoint, producerMember)
		_, priorGroupOK := assembly.Group(rightPoint, priorMember)
		carryGroup, carryGroupOK := assembly.Group(rightPoint, carryMember)
		producerSelfOK := assembly.Boundary(producerGroup, leftSelf)
		producerOrderingOK := assembly.Boundary(producerGroup, rightToLeft)
		carryBoundaryOK := assembly.Boundary(carryGroup, leftToRight)
		var queryInstanceOK bool
		queryInstance, queryInstanceOK = NewQueryInstance(query, func(binding *QueryBinding[uint64]) bool {
			return InstanceQueryRead(binding, queryRead, valueRef)
		})
		_, observationOK := assembly.Query(rightPoint, queryInstance)
		return leftPointOK && rightPointOK && seedMemberOK && producerMemberOK && priorMemberOK && carryMemberOK && seedGroupOK && producerGroupOK && priorGroupOK && carryGroupOK && producerSelfOK && producerOrderingOK && carryBoundaryOK && queryInstanceOK && observationOK
	})
	if !assembled || solver == nil || queryInstance == nil {
		t.Fatal("coverage wake SourceAssembly compile")
	}

	state, status := solver.Solve(context.Background())
	receipt, receiptOK := queryInstance.Receipt()
	result, readable := QueryResult(receipt, state)
	if status != SolveComplete || state == nil || !receiptOK || !readable || result != 7 || producerTransfers < 2 || carryTransfers < 2 {
		t.Fatalf("coverage-only wake = state:%v status:%v result:%d readable:%t producer:%d carry:%d", state, status, result, readable, producerTransfers, carryTransfers)
	}
}
