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

// TestNonIdentityEnvironmentInputBackEdgeReevaluatesWithoutCandidateFailure
// exercises the recursive EnvironmentInput path with distinct source and
// target guard coordinates.  The two zero-input structural Groups form one
// WTO cycle; the callbacks simply retain the transported environment support.
// When the target publishes, the source Group is reevaluated with a changed
// environment token but an equal candidate.  That is a valid monotone update,
// not a candidate-order violation.
func TestNonIdentityEnvironmentInputBackEdgeReevaluatesWithoutCandidateFailure(t *testing.T) {
	composition := engine.NewComposition()
	completion, completionOK := engine.DeclareSupportCompletion(composition, facadeKey(101))
	prune, pruneOK := engine.DeclarePrune(completion, facadeKey(102))
	runs := 0
	rule, ruleOK := engine.DeclareSupportRule(composition, engine.SupportRuleSpec{
		Semantic: facadeKey(103), Completion: completion, Prune: prune, Inputs: 0,
		Admission: engine.AdmitSupportByTrustedTheorem(facadeKey(104)),
		Run: func(value engine.Support) (engine.Support, bool) {
			runs++
			return value, true
		},
	})
	query, queryOK := engine.DeclareSupportQuery(composition, facadeKey(105), func(observation engine.SupportObservation) bool {
		reachable, ok := engine.SupportReachable(observation)
		return ok && reachable
	}, engine.FrozenResult[bool]{
		Semantic: facadeKey(106),
		Freeze:   func(value bool) bool { return value }, Clone: func(value bool) bool { return value },
		Equal: func(left, right bool) bool { return left == right },
		Fingerprint: func(value bool) uint64 {
			if value {
				return 1
			}
			return 0
		},
	})
	if !completionOK || !pruneOK || !ruleOK || rule == nil || !queryOK || query == nil || !composition.Seal() {
		t.Fatal("recursive nonidentity support declaration")
	}

	source := engine.NewSourceAssembly(composition)
	raw, rawOK := source.Decision(facadeKey(107))
	fresh, freshOK := source.Decision(facadeKey(108))
	sourceScope, sourceScopeOK := source.Scope(raw)
	targetScope, targetScopeOK := source.Scope(fresh)
	truth, truthOK := source.TrueExpr()
	falsity, falseOK := source.FalseExpr()
	sourceSite, sourceSiteOK := source.Site(facadeKey(109), sourceScope, truth, true)
	targetSite, targetSiteOK := source.Site(facadeKey(110), targetScope, falsity, false)
	sourceOccurrence, sourceOccurrenceOK := source.Relation(sourceSite, facadeKey(111))
	targetOccurrence, targetOccurrenceOK := source.Relation(targetSite, facadeKey(112))
	sourceInstance, sourceInstanceOK := engine.NewSupportInstance(rule, func(*engine.StructuralBinding) bool { return true })
	targetInstance, targetInstanceOK := engine.NewSupportInstance(rule, func(*engine.StructuralBinding) bool { return true })
	sourcePrepared, sourcePreparedOK := source.PrepareStructural(sourceOccurrence, facadeKey(113), sourceInstance)
	targetPrepared, targetPreparedOK := source.PrepareStructural(targetOccurrence, facadeKey(114), targetInstance)
	rawExpr, rawExprOK := source.DecisionExpr(raw)
	freshExpr, freshExprOK := source.DecisionExpr(fresh)
	rawToFreshMap, rawToFreshMapOK := source.RenameMap(raw, fresh)
	freshToRawMap, freshToRawMapOK := source.RenameMap(fresh, raw)
	rawToFresh, rawToFreshOK := source.Reindex(sourceScope, targetScope, rawToFreshMap)
	freshToRaw, freshToRawOK := source.Reindex(targetScope, sourceScope, freshToRawMap)
	forward, forwardOK := source.Boundary(sourceSite, targetSite, facadeKey(115), rawExpr, rawToFresh, freshExpr)
	back, backOK := source.Boundary(targetSite, sourceSite, facadeKey(116), freshExpr, freshToRaw, rawExpr)
	sealed := source.Seal()
	if !rawOK || !freshOK || !sourceScopeOK || !targetScopeOK || !truthOK || !falseOK || !sourceSiteOK || !targetSiteOK ||
		!sourceOccurrenceOK || !targetOccurrenceOK || !sourceInstanceOK || !targetInstanceOK || !sourcePreparedOK || !targetPreparedOK ||
		!rawExprOK || !freshExprOK || !rawToFreshMapOK || !freshToRawMapOK || !rawToFreshOK || !freshToRawOK || !forwardOK || !backOK || !sealed {
		t.Fatal("recursive nonidentity source topology")
	}

	queryInstance, queryInstanceOK := engine.NewQueryInstance(query, func(*engine.QueryBinding[bool]) bool { return true })
	if !queryInstanceOK || queryInstance == nil {
		t.Fatal("recursive nonidentity query instance")
	}
	solver, assembled := source.Assemble(func(assembly *engine.Assembly) bool {
		sourcePoint, sourcePointOK := assembly.Point(sourceSite)
		targetPoint, targetPointOK := assembly.Point(targetSite)
		sourceMember, sourceMemberOK := assembly.Member(sourcePoint, sourcePrepared)
		targetMember, targetMemberOK := assembly.Member(targetPoint, targetPrepared)
		sourceGroup, sourceGroupOK := assembly.Group(sourcePoint, sourceMember)
		targetGroup, targetGroupOK := assembly.Group(targetPoint, targetMember)
		if sourceGroupOK {
			sourceGroupOK = assembly.EnvironmentInput(sourceGroup, back)
		}
		if targetGroupOK {
			targetGroupOK = assembly.EnvironmentInput(targetGroup, forward)
		}
		_, observationOK := assembly.Query(targetPoint, queryInstance)
		return sourcePointOK && targetPointOK && sourceMemberOK && targetMemberOK && sourceGroupOK && targetGroupOK && observationOK
	})
	if !assembled || solver == nil {
		t.Fatalf("recursive nonidentity assembly assembled=%t solver=%p", assembled, solver)
	}
	state, status, report := solver.SolveWithReport(context.Background())
	receipt, receiptOK := queryInstance.Receipt()
	reachable, readable := engine.QueryResult(receipt, state)
	if !assembled || solver == nil || status != engine.SolveComplete || state == nil || !receiptOK || !readable || !reachable || runs < 2 {
		t.Fatalf("recursive nonidentity solve state=%v status=%v reason=%v phase=%v assembled=%t receipt=%t readable=%t reachable=%t runs=%d", state, status, report.Reason(), report.Phase(), assembled, receiptOK, readable, reachable, runs)
	}
}

// TestNonIdentityEnvironmentEdgeBackEdgeBindsAndConverges proves the same
// transport law for the control-only structural edge path.  Unlike the
// preceding Group-environment test, neither edge is attached to a Group:
// both directions are EnvironmentEdges and the reverse edge is a WTO back
// edge with a non-identity guard reindex.
func TestNonIdentityEnvironmentEdgeBackEdgeBindsAndConverges(t *testing.T) {
	composition := engine.NewComposition()
	completion, completionOK := engine.DeclareSupportCompletion(composition, facadeKey(121))
	prune, pruneOK := engine.DeclarePrune(completion, facadeKey(122))
	runs := 0
	rule, ruleOK := engine.DeclareSupportRule(composition, engine.SupportRuleSpec{
		Semantic: facadeKey(123), Completion: completion, Prune: prune, Inputs: 0,
		Admission: engine.AdmitSupportByTrustedTheorem(facadeKey(124)),
		Run: func(value engine.Support) (engine.Support, bool) {
			runs++
			return value, true
		},
	})
	query, queryOK := engine.DeclareSupportQuery(composition, facadeKey(125), func(observation engine.SupportObservation) bool {
		reachable, ok := engine.SupportReachable(observation)
		return ok && reachable
	}, engine.FrozenResult[bool]{
		Semantic: facadeKey(126),
		Freeze:   func(value bool) bool { return value }, Clone: func(value bool) bool { return value },
		Equal: func(left, right bool) bool { return left == right },
		Fingerprint: func(value bool) uint64 {
			if value {
				return 1
			}
			return 0
		},
	})
	if !completionOK || !pruneOK || !ruleOK || rule == nil || !queryOK || query == nil || !composition.Seal() {
		t.Fatal("recursive nonidentity edge declaration")
	}

	source := engine.NewSourceAssembly(composition)
	raw, rawOK := source.Decision(facadeKey(127))
	fresh, freshOK := source.Decision(facadeKey(128))
	sourceScope, sourceScopeOK := source.Scope(raw)
	targetScope, targetScopeOK := source.Scope(fresh)
	truth, truthOK := source.TrueExpr()
	falsity, falseOK := source.FalseExpr()
	sourceSite, sourceSiteOK := source.Site(facadeKey(129), sourceScope, truth, true)
	targetSite, targetSiteOK := source.Site(facadeKey(130), targetScope, falsity, false)
	sourceOccurrence, sourceOccurrenceOK := source.Relation(sourceSite, facadeKey(131))
	targetOccurrence, targetOccurrenceOK := source.Relation(targetSite, facadeKey(132))
	sourceInstance, sourceInstanceOK := engine.NewSupportInstance(rule, func(*engine.StructuralBinding) bool { return true })
	targetInstance, targetInstanceOK := engine.NewSupportInstance(rule, func(*engine.StructuralBinding) bool { return true })
	sourcePrepared, sourcePreparedOK := source.PrepareStructural(sourceOccurrence, facadeKey(133), sourceInstance)
	targetPrepared, targetPreparedOK := source.PrepareStructural(targetOccurrence, facadeKey(134), targetInstance)
	rawExpr, rawExprOK := source.DecisionExpr(raw)
	freshExpr, freshExprOK := source.DecisionExpr(fresh)
	rawToFreshMap, rawToFreshMapOK := source.RenameMap(raw, fresh)
	freshToRawMap, freshToRawMapOK := source.RenameMap(fresh, raw)
	rawToFresh, rawToFreshOK := source.Reindex(sourceScope, targetScope, rawToFreshMap)
	freshToRaw, freshToRawOK := source.Reindex(targetScope, sourceScope, freshToRawMap)
	forward, forwardOK := source.Boundary(sourceSite, targetSite, facadeKey(135), rawExpr, rawToFresh, freshExpr)
	back, backOK := source.Boundary(targetSite, sourceSite, facadeKey(136), freshExpr, freshToRaw, rawExpr)
	sealed := source.Seal()
	if !rawOK || !freshOK || !sourceScopeOK || !targetScopeOK || !truthOK || !falseOK || !sourceSiteOK || !targetSiteOK ||
		!sourceOccurrenceOK || !targetOccurrenceOK || !sourceInstanceOK || !targetInstanceOK || !sourcePreparedOK || !targetPreparedOK ||
		!rawExprOK || !freshExprOK || !rawToFreshMapOK || !freshToRawMapOK || !rawToFreshOK || !freshToRawOK || !forwardOK || !backOK || !sealed {
		t.Fatal("recursive nonidentity edge source topology")
	}
	queryInstance, queryInstanceOK := engine.NewQueryInstance(query, func(*engine.QueryBinding[bool]) bool { return true })
	if !queryInstanceOK || queryInstance == nil {
		t.Fatal("recursive nonidentity edge query instance")
	}
	solver, assembled := source.Assemble(func(assembly *engine.Assembly) bool {
		sourcePoint, sourcePointOK := assembly.Point(sourceSite)
		targetPoint, targetPointOK := assembly.Point(targetSite)
		sourceMember, sourceMemberOK := assembly.Member(sourcePoint, sourcePrepared)
		targetMember, targetMemberOK := assembly.Member(targetPoint, targetPrepared)
		_, sourceGroupOK := assembly.Group(sourcePoint, sourceMember)
		_, targetGroupOK := assembly.Group(targetPoint, targetMember)
		forwardEdgeOK := assembly.EnvironmentEdge(targetPoint, forward)
		backEdgeOK := assembly.EnvironmentEdge(sourcePoint, back)
		_, observationOK := assembly.Query(targetPoint, queryInstance)
		return sourcePointOK && targetPointOK && sourceMemberOK && targetMemberOK && sourceGroupOK && targetGroupOK && forwardEdgeOK && backEdgeOK && observationOK
	})
	if !assembled || solver == nil {
		t.Fatalf("recursive nonidentity edge assembly assembled=%t solver=%p", assembled, solver)
	}
	state, status := solver.Solve(context.Background())
	receipt, receiptOK := queryInstance.Receipt()
	reachable, readable := engine.QueryResult(receipt, state)
	if status != engine.SolveComplete || state == nil || !receiptOK || !readable || !reachable || runs < 2 {
		t.Fatalf("recursive nonidentity edge solve state=%v status=%v receipt=%t readable=%t reachable=%t runs=%d", state, status, receiptOK, readable, reachable, runs)
	}
}
