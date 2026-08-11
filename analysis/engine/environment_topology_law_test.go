package engine_test

import (
	"context"
	"testing"

	engine "github.com/wippyai/go-lua/analysis/engine"
)

// TestSeparateEnvironmentInputLetsAZeroInputRuleTransformTheWholePoint
// exercises the ownership cut directly: the target Rule has zero declared
// dependency ports, yet its Group receives one extra environment boundary.
// The strong write changes key 0 while key 1 from the same Factor survives.
func TestSeparateEnvironmentInputLetsAZeroInputRuleTransformTheWholePoint(t *testing.T) {
	composition := engine.NewComposition()
	factor, factorOK := engine.DeclareFactor(composition, engine.FactorSpec[uint64, uint64]{
		Semantic: facadeKey(31), KeyEnd: 2, Default: 0,
		Lattice: facadeLattice(), AdmitAt: func(uint64, uint64) bool { return true }, Fingerprint: func(value uint64) uint64 { return value },
		WidenRank:  engine.Measure[uint64, uint64]{Width: 1, At: func(_ uint64, value uint64, _ int) uint64 { return ^value }},
		NarrowRank: engine.Measure[uint64, uint64]{Width: 1, At: func(_ uint64, value uint64, _ int) uint64 { return value }},
	}, func(*engine.Factor[uint64, uint64]) bool { return true })
	read, readOK := engine.ExactReadForm(factor)
	write, writeOK := engine.ExactWriteForm(factor)
	if !factorOK || factor == nil || !readOK || !writeOK {
		t.Fatal("factor")
	}

	var firstWrite, secondWrite, transformWrite engine.Write[uint64]
	first, firstOK := engine.DeclareRule(composition, engine.RuleSpec[uint64, facadeUnit]{
		Semantic: facadeKey(33), OperandFamily: facadeKey(32), OperandContent: facadeUnitContent, Output: factor.Output(), Inputs: 0,
		Admission: engine.AdmitRuleByTrustedTheorem[uint64, facadeUnit](facadeKey(33)),
		Transfer: func(access engine.Access[uint64, facadeUnit]) bool {
			return engine.Product(access, func(row engine.Row) bool { return engine.StageValue(access, row, 2) })
		},
	}, func(rule *engine.Rule[uint64, facadeUnit]) bool {
		var ok bool
		firstWrite, ok = engine.WriteTo(rule, write)
		return ok
	})
	second, secondOK := engine.DeclareRule(composition, engine.RuleSpec[uint64, facadeUnit]{
		Semantic: facadeKey(34), OperandFamily: facadeKey(32), OperandContent: facadeUnitContent, Output: factor.Output(), Inputs: 0,
		Admission: engine.AdmitRuleByTrustedTheorem[uint64, facadeUnit](facadeKey(34)),
		Transfer: func(access engine.Access[uint64, facadeUnit]) bool {
			return engine.Product(access, func(row engine.Row) bool { return engine.StageValue(access, row, 3) })
		},
	}, func(rule *engine.Rule[uint64, facadeUnit]) bool {
		var ok bool
		secondWrite, ok = engine.WriteTo(rule, write)
		return ok
	})
	transform, transformOK := engine.DeclareRule(composition, engine.RuleSpec[uint64, facadeUnit]{
		Semantic: facadeKey(35), OperandFamily: facadeKey(32), OperandContent: facadeUnitContent, Output: factor.Output(), Inputs: 0,
		Admission: engine.AdmitRuleByTrustedTheorem[uint64, facadeUnit](facadeKey(35)),
		Transfer: func(access engine.Access[uint64, facadeUnit]) bool {
			return engine.Product(access, func(row engine.Row) bool { return engine.StageValue(access, row, 4) })
		},
	}, func(rule *engine.Rule[uint64, facadeUnit]) bool {
		var ok bool
		transformWrite, ok = engine.WriteTo(rule, write)
		return ok
	})
	if !firstOK || first == nil || !secondOK || second == nil || !transformOK || transform == nil {
		t.Fatal("rules")
	}

	var leftQueryRead, rightQueryRead engine.QueryRead[engine.OrderedCells[uint64]]
	query, queryOK := engine.DeclareQuery(composition, engine.QuerySpec[uint64]{
		Semantic: facadeKey(36),
		Project: func(observation engine.Observation) uint64 {
			result, rows := uint64(0), 0
			if !engine.ProjectRows(observation, func(row engine.QueryRow) bool {
				leftCells, leftResolved := engine.QueryValue(row, leftQueryRead)
				rightCells, rightResolved := engine.QueryValue(row, rightQueryRead)
				left, leftPresent, leftValid := leftCells.At(0)
				right, rightPresent, rightValid := rightCells.At(0)
				if !leftResolved || !rightResolved || !leftPresent || !rightPresent || !leftValid || !rightValid {
					return false
				}
				result, rows = left*10+right, rows+1
				return true
			}) || rows != 1 {
				return 0
			}
			return result
		},
		Result: engine.FrozenResult[uint64]{Semantic: facadeKey(37), Freeze: func(value uint64) uint64 { return value }, Clone: func(value uint64) uint64 { return value }, Equal: func(left, right uint64) bool { return left == right }, Fingerprint: func(value uint64) uint64 { return value }},
	}, func(value *engine.Query[uint64]) bool {
		var leftOK, rightOK bool
		leftQueryRead, leftOK = engine.QueryReadFrom(value, read)
		rightQueryRead, rightOK = engine.QueryReadFrom(value, read)
		return leftOK && rightOK
	})
	if !queryOK || query == nil || !composition.Seal() {
		t.Fatal("cold declarations")
	}
	firstRef, firstRefOK := factor.Ref(0)
	secondRef, secondRefOK := factor.Ref(1)
	firstInstance, firstInstanceOK := engine.NewRuleInstance(first, facadeUnitFor(facadeKey(38)), func(binding *engine.RuleBinding[uint64, facadeUnit]) bool {
		return engine.InstanceWrite(binding, firstWrite, firstRef)
	})
	secondInstance, secondInstanceOK := engine.NewRuleInstance(second, facadeUnitFor(facadeKey(39)), func(binding *engine.RuleBinding[uint64, facadeUnit]) bool {
		return engine.InstanceWrite(binding, secondWrite, secondRef)
	})
	transformInstance, transformInstanceOK := engine.NewRuleInstance(transform, facadeUnitFor(facadeKey(40)), func(binding *engine.RuleBinding[uint64, facadeUnit]) bool {
		return engine.InstanceWrite(binding, transformWrite, firstRef)
	})
	if !firstRefOK || !secondRefOK || !firstInstanceOK || !secondInstanceOK || !transformInstanceOK {
		t.Fatal("instances")
	}

	source := engine.NewSourceAssembly(composition)
	scope, scopeOK := source.Scope()
	truth, truthOK := source.TrueExpr()
	falsity, falseOK := source.FalseExpr()
	sourceSite, sourceSiteOK := source.Site(facadeKey(41), scope, truth, true)
	targetSite, targetSiteOK := source.Site(facadeKey(42), scope, falsity, false)
	firstOccurrence, firstOccurrenceOK := source.Relation(sourceSite, facadeKey(43))
	secondOccurrence, secondOccurrenceOK := source.Relation(sourceSite, facadeKey(44))
	transformOccurrence, transformOccurrenceOK := source.Relation(targetSite, facadeKey(45))
	firstPrepared, firstPreparedOK := source.PrepareInstance(firstOccurrence, firstInstance)
	secondPrepared, secondPreparedOK := source.PrepareInstance(secondOccurrence, secondInstance)
	transformPrepared, transformPreparedOK := source.PrepareInstance(transformOccurrence, transformInstance)
	reindex, reindexOK := source.IdentityReindex(scope)
	environmentBoundary, environmentBoundaryOK := source.Boundary(sourceSite, targetSite, facadeKey(46), truth, reindex, truth)
	sealed := source.Seal()
	if !scopeOK || !truthOK || !falseOK || !sourceSiteOK || !targetSiteOK || !firstOccurrenceOK || !secondOccurrenceOK || !transformOccurrenceOK || !firstPreparedOK || !secondPreparedOK || !transformPreparedOK || !reindexOK || !environmentBoundaryOK || !sealed {
		t.Fatalf("source scope=%t truth=%t false=%t sites=%t/%t occ=%t/%t/%t prep=%t/%t/%t reindex=%t boundary=%t sealed=%t", scopeOK, truthOK, falseOK, sourceSiteOK, targetSiteOK, firstOccurrenceOK, secondOccurrenceOK, transformOccurrenceOK, firstPreparedOK, secondPreparedOK, transformPreparedOK, reindexOK, environmentBoundaryOK, sealed)
	}

	var queryInstance *engine.QueryInstance[uint64]
	var assemblyStatus [10]bool
	solver, assembled := source.Assemble(func(assembly *engine.Assembly) bool {
		sourcePoint, sourcePointOK := assembly.Point(sourceSite)
		targetPoint, targetPointOK := assembly.Point(targetSite)
		firstMember, firstMemberOK := assembly.Member(sourcePoint, firstPrepared)
		secondMember, secondMemberOK := assembly.Member(sourcePoint, secondPrepared)
		transformMember, transformMemberOK := assembly.Member(targetPoint, transformPrepared)
		_, firstGroupOK := assembly.Group(sourcePoint, firstMember)
		_, secondGroupOK := assembly.Group(sourcePoint, secondMember)
		transformGroup, transformGroupOK := assembly.Group(targetPoint, transformMember)
		if transformGroupOK {
			transformGroupOK = assembly.EnvironmentInput(transformGroup, environmentBoundary)
		}
		var queryOK, observationOK bool
		queryInstance, queryOK = engine.NewQueryInstance(query, func(binding *engine.QueryBinding[uint64]) bool {
			return engine.InstanceQueryRead(binding, leftQueryRead, firstRef) && engine.InstanceQueryRead(binding, rightQueryRead, secondRef)
		})
		if queryOK {
			_, observationOK = assembly.Query(targetPoint, queryInstance)
		}
		assemblyStatus = [10]bool{sourcePointOK, targetPointOK, firstMemberOK, secondMemberOK, transformMemberOK, firstGroupOK, secondGroupOK, transformGroupOK, queryOK, observationOK}
		return sourcePointOK && targetPointOK && firstMemberOK && secondMemberOK && transformMemberOK && firstGroupOK && secondGroupOK && transformGroupOK && queryOK && observationOK
	})
	if !assembled || solver == nil {
		t.Fatalf("assembly assembled=%t solver=%p query=%t statuses=%v", assembled, solver, queryInstance != nil, assemblyStatus)
	}
	state, status := solver.Solve(context.Background())
	receipt, receiptOK := queryInstance.Receipt()
	result, readable := engine.QueryResult(receipt, state)
	if status != engine.SolveComplete || !receiptOK || !readable || result != 43 {
		t.Fatalf("environment result=%d status=%v receipt=%t readable=%t", result, status, receiptOK, readable)
	}
}
