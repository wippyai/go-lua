package engine_test

import (
	"context"
	"testing"

	engine "github.com/wippyai/go-lua/analysis/engine"
)

func TestFactorEdgeProjectsOneTransportedFactorAtTarget(t *testing.T) {
	composition := engine.NewComposition()
	factor, factorOK := engine.DeclareFactor(composition, engine.FactorSpec[uint64, uint64]{
		Semantic: facadeKey(151), KeyEnd: 1, Default: 0,
		Lattice: facadeLattice(), AdmitAt: func(uint64, uint64) bool { return true }, Fingerprint: func(value uint64) uint64 { return value },
		WidenRank:  engine.Measure[uint64, uint64]{Width: 1, At: func(_ uint64, value uint64, _ int) uint64 { return ^value }},
		NarrowRank: engine.Measure[uint64, uint64]{Width: 1, At: func(_ uint64, value uint64, _ int) uint64 { return value }},
	}, func(*engine.Factor[uint64, uint64]) bool { return true })
	other, otherOK := engine.DeclareFactor(composition, engine.FactorSpec[uint64, uint64]{
		Semantic: facadeKey(162), KeyEnd: 1, Default: 0,
		Lattice: facadeLattice(), AdmitAt: func(uint64, uint64) bool { return true }, Fingerprint: func(value uint64) uint64 { return value },
		WidenRank:  engine.Measure[uint64, uint64]{Width: 1, At: func(_ uint64, value uint64, _ int) uint64 { return ^value }},
		NarrowRank: engine.Measure[uint64, uint64]{Width: 1, At: func(_ uint64, value uint64, _ int) uint64 { return value }},
	}, func(*engine.Factor[uint64, uint64]) bool { return true })
	read, readOK := engine.ExactReadForm(factor)
	write, writeOK := engine.ExactWriteForm(factor)
	otherRead, otherReadOK := engine.ExactReadForm(other)
	if !factorOK || !otherOK || !readOK || !writeOK || !otherReadOK {
		t.Fatal("factor")
	}
	var output engine.Write[uint64]
	rule, ruleOK := engine.DeclareRule(composition, engine.RuleSpec[uint64, facadeUnit]{
		Semantic: facadeKey(152), OperandFamily: facadeKey(153), OperandContent: facadeUnitContent, Output: factor.Output(), Inputs: 0,
		Admission: engine.AdmitRuleByTrustedTheorem[uint64, facadeUnit](facadeKey(154)),
		Transfer: func(access engine.Access[uint64, facadeUnit]) bool {
			return engine.Product(access, func(row engine.Row) bool { return engine.StageValue(access, row, 7) })
		},
	}, func(rule *engine.Rule[uint64, facadeUnit]) bool {
		var ok bool
		output, ok = engine.WriteTo(rule, write)
		return ok
	})
	if !ruleOK || rule == nil {
		t.Fatal("rule")
	}
	var queryRead engine.QueryRead[engine.OrderedCells[uint64]]
	query, queryOK := engine.DeclareQuery(composition, engine.QuerySpec[uint64]{
		Semantic: facadeKey(155),
		Project: func(observation engine.Observation) uint64 {
			result, rows := uint64(0), 0
			if !engine.ProjectRows(observation, func(row engine.QueryRow) bool {
				cells, resolved := engine.QueryValue(row, queryRead)
				value, present, valid := cells.At(0)
				if !resolved || !present || !valid {
					return false
				}
				result, rows = value, rows+1
				return true
			}) || rows != 1 {
				return 0
			}
			return result
		},
		Result: engine.FrozenResult[uint64]{Semantic: facadeKey(156), Freeze: func(value uint64) uint64 { return value }, Clone: func(value uint64) uint64 { return value }, Equal: func(left, right uint64) bool { return left == right }, Fingerprint: func(value uint64) uint64 { return value }},
	}, func(value *engine.Query[uint64]) bool {
		var ok bool
		queryRead, ok = engine.QueryReadFrom(value, read)
		return ok
	})
	if !queryOK || query == nil {
		t.Fatal("cold")
	}
	var otherQueryRead engine.QueryRead[engine.OrderedCells[uint64]]
	otherQuery, otherQueryOK := engine.DeclareQuery(composition, engine.QuerySpec[uint64]{
		Semantic: facadeKey(163),
		Project: func(observation engine.Observation) uint64 {
			result, rows := uint64(0), 0
			if !engine.ProjectRows(observation, func(row engine.QueryRow) bool {
				cells, resolved := engine.QueryValue(row, otherQueryRead)
				if !resolved {
					rows++
					return true
				}
				value, present, valid := cells.At(0)
				if !present || !valid {
					rows++
					return true
				}
				result, rows = value, rows+1
				return true
			}) || rows != 1 {
				return 0
			}
			return result
		},
		Result: engine.FrozenResult[uint64]{Semantic: facadeKey(164), Freeze: func(value uint64) uint64 { return value }, Clone: func(value uint64) uint64 { return value }, Equal: func(left, right uint64) bool { return left == right }, Fingerprint: func(value uint64) uint64 { return value }},
	}, func(value *engine.Query[uint64]) bool {
		var ok bool
		otherQueryRead, ok = engine.QueryReadFrom(value, otherRead)
		return ok
	})
	if !otherQueryOK || otherQuery == nil || !composition.Seal() {
		t.Fatal("other query")
	}
	ref, refOK := factor.Ref(0)
	otherRef, otherRefOK := other.Ref(0)
	instance, instanceOK := engine.NewRuleInstance(rule, facadeUnitFor(facadeKey(157)), func(binding *engine.RuleBinding[uint64, facadeUnit]) bool {
		return engine.InstanceWrite(binding, output, ref)
	})
	if !refOK || !otherRefOK || !instanceOK {
		t.Fatal("instance")
	}
	source := engine.NewSourceAssembly(composition)
	scope, scopeOK := source.Scope()
	truth, truthOK := source.TrueExpr()
	falsity, falseOK := source.FalseExpr()
	sourceSite, sourceSiteOK := source.Site(facadeKey(158), scope, truth, true)
	targetSite, targetSiteOK := source.Site(facadeKey(159), scope, falsity, false)
	occurrence, occurrenceOK := source.Relation(sourceSite, facadeKey(160))
	prepared, preparedOK := source.PrepareInstance(occurrence, instance)
	reindex, reindexOK := source.IdentityReindex(scope)
	const edgeCount = 64
	boundaries := make([]engine.SourceBoundary, edgeCount)
	boundaryOK := true
	for index := range boundaries {
		boundaries[index], boundaryOK = source.Boundary(sourceSite, targetSite, facadeKey(byte(161+index)), truth, reindex, truth)
		if !boundaryOK {
			break
		}
	}
	if !scopeOK || !truthOK || !falseOK || !sourceSiteOK || !targetSiteOK || !occurrenceOK || !preparedOK || !reindexOK || !boundaryOK || !source.Seal() {
		t.Fatal("source")
	}
	carry, carryOK := engine.Carry(factor)
	if !carryOK {
		t.Fatal("carry")
	}
	queryInstance, queryInstanceOK := engine.NewQueryInstance(query, func(binding *engine.QueryBinding[uint64]) bool {
		return engine.InstanceQueryRead(binding, queryRead, ref)
	})
	otherQueryInstance, otherQueryInstanceOK := engine.NewQueryInstance(otherQuery, func(binding *engine.QueryBinding[uint64]) bool {
		return engine.InstanceQueryRead(binding, otherQueryRead, otherRef)
	})
	sourceQueryInstance, sourceQueryInstanceOK := engine.NewQueryInstance(query, func(binding *engine.QueryBinding[uint64]) bool {
		return engine.InstanceQueryRead(binding, queryRead, ref)
	})
	if !queryInstanceOK || !otherQueryInstanceOK || !sourceQueryInstanceOK {
		t.Fatal("query instance")
	}
	solver, assembled := source.Assemble(func(assembly *engine.Assembly) bool {
		sourcePoint, sourcePointOK := assembly.Point(sourceSite)
		targetPoint, targetPointOK := assembly.Point(targetSite)
		member, memberOK := assembly.Member(sourcePoint, prepared)
		_, groupOK := assembly.Group(sourcePoint, member)
		edgeOK := true
		for _, boundary := range boundaries {
			if !assembly.FactorEdge(targetPoint, boundary, carry) {
				edgeOK = false
				break
			}
		}
		_, observationOK := assembly.Query(targetPoint, queryInstance)
		_, otherObservationOK := assembly.Query(targetPoint, otherQueryInstance)
		_, sourceObservationOK := assembly.Query(sourcePoint, sourceQueryInstance)
		return sourcePointOK && targetPointOK && memberOK && groupOK && edgeOK && observationOK && otherObservationOK && sourceObservationOK
	})
	if !assembled || solver == nil {
		t.Fatal("assembly")
	}
	state, status := solver.Solve(context.Background())
	receipt, receiptOK := queryInstance.Receipt()
	otherReceipt, otherReceiptOK := otherQueryInstance.Receipt()
	sourceReceipt, sourceReceiptOK := sourceQueryInstance.Receipt()
	result, readable := engine.QueryResult(receipt, state)
	otherResult, otherReadable := engine.QueryResult(otherReceipt, state)
	sourceResult, sourceReadable := engine.QueryResult(sourceReceipt, state)
	if status != engine.SolveComplete || !receiptOK || !readable || !otherReceiptOK || !otherReadable || !sourceReceiptOK || !sourceReadable || sourceResult != 7 || result != 7 || otherResult != 0 {
		t.Fatalf("result=%d other=%d source=%d status=%v receipt=%t readable=%t otherReceipt=%t otherReadable=%t sourceReceipt=%t sourceReadable=%t", result, otherResult, sourceResult, status, receiptOK, readable, otherReceiptOK, otherReadable, sourceReceiptOK, sourceReadable)
	}
}

func TestFactorEdgeOnlySCCCarriesFactorThroughRecurrence(t *testing.T) {
	composition := engine.NewComposition()
	factor, factorOK := engine.DeclareFactor(composition, engine.FactorSpec[uint64, uint64]{
		Semantic: facadeKey(171), KeyEnd: 1, Default: 0,
		Lattice: facadeLattice(), AdmitAt: func(uint64, uint64) bool { return true }, Fingerprint: func(value uint64) uint64 { return value },
		WidenRank:  engine.Measure[uint64, uint64]{Width: 1, At: func(_ uint64, value uint64, _ int) uint64 { return ^value }},
		NarrowRank: engine.Measure[uint64, uint64]{Width: 1, At: func(_ uint64, value uint64, _ int) uint64 { return value }},
	}, func(*engine.Factor[uint64, uint64]) bool { return true })
	read, readOK := engine.ExactReadForm(factor)
	write, writeOK := engine.ExactWriteForm(factor)
	if !factorOK || !readOK || !writeOK {
		t.Fatal("factor")
	}
	var output engine.Write[uint64]
	rule, ruleOK := engine.DeclareRule(composition, engine.RuleSpec[uint64, facadeUnit]{
		Semantic: facadeKey(172), OperandFamily: facadeKey(173), OperandContent: facadeUnitContent, Output: factor.Output(), Inputs: 0,
		Admission: engine.AdmitRuleByTrustedTheorem[uint64, facadeUnit](facadeKey(174)),
		Transfer: func(access engine.Access[uint64, facadeUnit]) bool {
			return engine.Product(access, func(row engine.Row) bool { return engine.StageValue(access, row, 1) })
		},
	}, func(rule *engine.Rule[uint64, facadeUnit]) bool {
		var ok bool
		output, ok = engine.WriteTo(rule, write)
		return ok
	})
	var queryRead engine.QueryRead[engine.OrderedCells[uint64]]
	query, queryOK := engine.DeclareQuery(composition, engine.QuerySpec[uint64]{
		Semantic: facadeKey(175),
		Project: func(observation engine.Observation) uint64 {
			result, rows := uint64(0), 0
			if !engine.ProjectRows(observation, func(row engine.QueryRow) bool {
				cells, resolved := engine.QueryValue(row, queryRead)
				value, present, valid := cells.At(0)
				if !resolved || !present || !valid {
					return false
				}
				result, rows = value, rows+1
				return true
			}) || rows != 1 {
				return 0
			}
			return result
		},
		Result: engine.FrozenResult[uint64]{Semantic: facadeKey(176), Freeze: func(value uint64) uint64 { return value }, Clone: func(value uint64) uint64 { return value }, Equal: func(left, right uint64) bool { return left == right }, Fingerprint: func(value uint64) uint64 { return value }},
	}, func(value *engine.Query[uint64]) bool {
		var ok bool
		queryRead, ok = engine.QueryReadFrom(value, read)
		return ok
	})
	if !ruleOK || rule == nil || !queryOK || query == nil || !composition.Seal() {
		t.Fatal("cold")
	}
	ref, refOK := factor.Ref(0)
	instance, instanceOK := engine.NewRuleInstance(rule, facadeUnitFor(facadeKey(177)), func(binding *engine.RuleBinding[uint64, facadeUnit]) bool {
		return engine.InstanceWrite(binding, output, ref)
	})
	if !refOK || !instanceOK {
		t.Fatal("instance")
	}
	source := engine.NewSourceAssembly(composition)
	scope, scopeOK := source.Scope()
	truth, truthOK := source.TrueExpr()
	falsity, falseOK := source.FalseExpr()
	firstSite, firstSiteOK := source.Site(facadeKey(178), scope, truth, true)
	secondSite, secondSiteOK := source.Site(facadeKey(179), scope, falsity, false)
	occurrence, occurrenceOK := source.Relation(firstSite, facadeKey(180))
	prepared, preparedOK := source.PrepareInstance(occurrence, instance)
	reindex, reindexOK := source.IdentityReindex(scope)
	firstBoundary, firstBoundaryOK := source.Boundary(firstSite, secondSite, facadeKey(181), truth, reindex, truth)
	secondBoundary, secondBoundaryOK := source.Boundary(secondSite, firstSite, facadeKey(182), truth, reindex, truth)
	if !scopeOK || !truthOK || !falseOK || !firstSiteOK || !secondSiteOK || !occurrenceOK || !preparedOK || !reindexOK || !firstBoundaryOK || !secondBoundaryOK || !source.Seal() {
		t.Fatal("source")
	}
	carry, carryOK := engine.Carry(factor)
	queryInstance, queryInstanceOK := engine.NewQueryInstance(query, func(binding *engine.QueryBinding[uint64]) bool {
		return engine.InstanceQueryRead(binding, queryRead, ref)
	})
	if !carryOK || !queryInstanceOK {
		t.Fatal("query/carry")
	}
	solver, assembled := source.Assemble(func(assembly *engine.Assembly) bool {
		firstPoint, firstPointOK := assembly.Point(firstSite)
		secondPoint, secondPointOK := assembly.Point(secondSite)
		member, memberOK := assembly.Member(firstPoint, prepared)
		_, groupOK := assembly.Group(firstPoint, member)
		firstEdgeOK := assembly.FactorEdge(secondPoint, firstBoundary, carry)
		secondEdgeOK := assembly.FactorEdge(firstPoint, secondBoundary, carry)
		_, observationOK := assembly.Query(secondPoint, queryInstance)
		return firstPointOK && secondPointOK && memberOK && groupOK && firstEdgeOK && secondEdgeOK && observationOK
	})
	if !assembled || solver == nil {
		t.Fatal("assembly")
	}
	state, status := solver.Solve(context.Background())
	receipt, receiptOK := queryInstance.Receipt()
	result, readable := engine.QueryResult(receipt, state)
	if status != engine.SolveComplete || !receiptOK || !readable || result != 1 {
		t.Fatalf("factor-edge SCC result=%d status=%v receipt=%t readable=%t", result, status, receiptOK, readable)
	}
}

// The Group edge N→H contributes Factor X while the structural H→N edge
// contributes only Factor Y. H is the dense lower point and therefore the WTO
// head; Y is internal to the Region but is not a head-targeted back edge.
func TestFactorEdgeInternalNonHeadFactorSeenAlongsideGroupFactor(t *testing.T) {
	composition := engine.NewComposition()
	factorX, xOK := engine.DeclareFactor(composition, engine.FactorSpec[uint64, uint64]{
		Semantic: facadeKey(211), KeyEnd: 1, Default: 0,
		Lattice: facadeLattice(), AdmitAt: func(uint64, uint64) bool { return true }, Fingerprint: func(value uint64) uint64 { return value },
		WidenRank:  engine.Measure[uint64, uint64]{Width: 1, At: func(_ uint64, value uint64, _ int) uint64 { return ^value }},
		NarrowRank: engine.Measure[uint64, uint64]{Width: 1, At: func(_ uint64, value uint64, _ int) uint64 { return value }},
	}, func(*engine.Factor[uint64, uint64]) bool { return true })
	factorY, yOK := engine.DeclareFactor(composition, engine.FactorSpec[uint64, uint64]{
		Semantic: facadeKey(212), KeyEnd: 1, Default: 0,
		Lattice: facadeLattice(), AdmitAt: func(uint64, uint64) bool { return true }, Fingerprint: func(value uint64) uint64 { return value },
		WidenRank:  engine.Measure[uint64, uint64]{Width: 1, At: func(_ uint64, value uint64, _ int) uint64 { return ^value }},
		NarrowRank: engine.Measure[uint64, uint64]{Width: 1, At: func(_ uint64, value uint64, _ int) uint64 { return value }},
	}, func(*engine.Factor[uint64, uint64]) bool { return true })
	readX, readXOK := engine.ExactReadForm(factorX)
	writeX, writeXOK := engine.ExactWriteForm(factorX)
	if !xOK || !yOK || !readXOK || !writeXOK {
		t.Fatal("factors")
	}
	var output engine.Write[uint64]
	rule, ruleOK := engine.DeclareRule(composition, engine.RuleSpec[uint64, facadeUnit]{
		Semantic: facadeKey(213), OperandFamily: facadeKey(214), OperandContent: facadeUnitContent, Output: factorX.Output(), Inputs: 1,
		Admission: engine.AdmitRuleByTrustedTheorem[uint64, facadeUnit](facadeKey(215)),
		Transfer: func(access engine.Access[uint64, facadeUnit]) bool {
			return engine.Product(access, func(row engine.Row) bool { return engine.StageValue(access, row, 5) })
		},
	}, func(rule *engine.Rule[uint64, facadeUnit]) bool {
		var ok bool
		output, ok = engine.WriteTo(rule, writeX)
		return ok
	})
	var queryRead engine.QueryRead[engine.OrderedCells[uint64]]
	query, queryOK := engine.DeclareQuery(composition, engine.QuerySpec[uint64]{
		Semantic: facadeKey(216),
		Project: func(observation engine.Observation) uint64 {
			result, rows := uint64(0), 0
			if !engine.ProjectRows(observation, func(row engine.QueryRow) bool {
				cells, resolved := engine.QueryValue(row, queryRead)
				value, present, valid := cells.At(0)
				if !resolved || !present || !valid {
					return false
				}
				result, rows = value, rows+1
				return true
			}) || rows != 1 {
				return 0
			}
			return result
		},
		Result: engine.FrozenResult[uint64]{Semantic: facadeKey(217), Freeze: func(value uint64) uint64 { return value }, Clone: func(value uint64) uint64 { return value }, Equal: func(left, right uint64) bool { return left == right }, Fingerprint: func(value uint64) uint64 { return value }},
	}, func(value *engine.Query[uint64]) bool {
		var ok bool
		queryRead, ok = engine.QueryReadFrom(value, readX)
		return ok
	})
	if !ruleOK || rule == nil || !queryOK || query == nil || !composition.Seal() {
		t.Fatal("cold")
	}
	refX, refXOK := factorX.Ref(0)
	instance, instanceOK := engine.NewRuleInstance(rule, facadeUnitFor(facadeKey(218)), func(binding *engine.RuleBinding[uint64, facadeUnit]) bool {
		return engine.InstanceWrite(binding, output, refX)
	})
	if !refXOK || !instanceOK {
		t.Fatal("instance")
	}
	source := engine.NewSourceAssembly(composition)
	scope, scopeOK := source.Scope()
	truth, truthOK := source.TrueExpr()
	falsity, falseOK := source.FalseExpr()
	// H sorts before N in the dense equation order and becomes the WTO head.
	headSite, headOK := source.Site(facadeKey(219), scope, falsity, false)
	nonHeadSite, nonHeadOK := source.Site(facadeKey(220), scope, truth, true)
	occurrence, occurrenceOK := source.Relation(headSite, facadeKey(221))
	prepared, preparedOK := source.PrepareInstance(occurrence, instance)
	reindex, reindexOK := source.IdentityReindex(scope)
	nToH, nToHOK := source.Boundary(nonHeadSite, headSite, facadeKey(222), truth, reindex, truth)
	hToN, hToNOK := source.Boundary(headSite, nonHeadSite, facadeKey(223), truth, reindex, truth)
	if !scopeOK || !truthOK || !falseOK || !headOK || !nonHeadOK || !occurrenceOK || !preparedOK || !reindexOK || !nToHOK || !hToNOK || !source.Seal() {
		t.Fatal("source")
	}
	carryY, carryYOK := engine.Carry(factorY)
	queryInstance, queryInstanceOK := engine.NewQueryInstance(query, func(binding *engine.QueryBinding[uint64]) bool {
		return engine.InstanceQueryRead(binding, queryRead, refX)
	})
	if !carryYOK || !queryInstanceOK {
		t.Fatal("carry/query")
	}
	solver, assembled := source.Assemble(func(assembly *engine.Assembly) bool {
		headPoint, headPointOK := assembly.Point(headSite)
		nonHeadPoint, nonHeadPointOK := assembly.Point(nonHeadSite)
		member, memberOK := assembly.Member(headPoint, prepared)
		group, groupOK := assembly.Group(headPoint, member)
		boundaryOK := assembly.Boundary(group, nToH)
		edgeOK := assembly.FactorEdge(nonHeadPoint, hToN, carryY)
		_, observationOK := assembly.Query(headPoint, queryInstance)
		return headPointOK && nonHeadPointOK && memberOK && groupOK && boundaryOK && edgeOK && observationOK
	})
	if !assembled || solver == nil {
		t.Fatal("assembly")
	}
	state, status := solver.Solve(context.Background())
	receipt, receiptOK := queryInstance.Receipt()
	result, readable := engine.QueryResult(receipt, state)
	if status != engine.SolveComplete || !receiptOK || !readable || result != 5 {
		t.Fatalf("mixed internal factor result=%d status=%v receipt=%t readable=%t", result, status, receiptOK, readable)
	}
}
