package engine_test

import (
	"testing"

	engine "github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/lattice"
)

// externalSummaryKey is deliberately private to a package outside engine.
// The law therefore proves that ClosedRefs does not require callers to expose
// K, recover a uint64 coordinate, or use an engine-private adapter merely to
// build a dynamic summary vector.
type externalSummaryKey uint32
type externalUnit struct{ content [32]byte }

func externalUnitForSemantic(semantic engine.SemanticKey) externalUnit {
	return externalUnit{content: semantic.Digest()}
}

func externalUnitContent(unit externalUnit) (externalUnit, [32]byte, bool) {
	return unit, unit.content, unit.content != [32]byte{}
}

func externalSummarySemantic(index byte) engine.SemanticKey {
	var digest [32]byte
	digest[31] = index
	key, ok := engine.NewSemanticKey(digest, 1)
	if !ok {
		panic("external summary semantic key")
	}
	return key
}

func externalSummaryLattice() lattice.Lattice[uint64] {
	return lattice.Lattice[uint64]{
		Bottom:   func() uint64 { return 0 },
		Top:      func() uint64 { return ^uint64(0) },
		Equal:    func(left, right uint64) bool { return left == right },
		LessOrEq: func(left, right uint64) bool { return left <= right },
		Join: func(left, right uint64) uint64 {
			if left > right {
				return left
			}
			return right
		},
		Widen: func(left, right uint64) uint64 {
			if left > right {
				return left
			}
			return right
		},
	}
}

func TestExternalPrivateKeyClosedRefsBuildSealConsumeAndRejectMutation(t *testing.T) {
	composition := engine.NewComposition()
	var summary engine.ReadForm[uint64, uint64]
	factor, declared := engine.DeclareFactor(composition, engine.FactorSpec[externalSummaryKey, uint64]{
		Semantic: externalSummarySemantic(1), KeyEnd: 3, Lattice: externalSummaryLattice(), Default: 0,
		AdmitAt: func(externalSummaryKey, uint64) bool { return true }, Fingerprint: func(value uint64) uint64 { return value },
	}, func(factor *engine.Factor[externalSummaryKey, uint64]) bool {
		normalizer, normalizerOK := engine.DeclareNormalizer(factor, externalSummarySemantic(2), func(cells engine.OrderedCells[uint64]) uint64 {
			return uint64(cells.Count())
		}, func(left, right uint64) bool { return left == right }, func(value uint64) uint64 { return value })
		if !normalizerOK {
			return false
		}
		var summaryOK bool
		summary, summaryOK = engine.SummaryReadForm(normalizer)
		return summaryOK
	})
	if !declared || factor == nil {
		t.Fatal("external Factor declaration")
	}
	read, readOK := engine.ExactReadForm(factor)
	write, writeOK := engine.ExactWriteForm(factor)
	var seedWrites [3]engine.Write[uint64]
	seed, seedOK := engine.DeclareRule(composition, engine.RuleSpec[uint64, externalUnit]{
		OperandFamily: externalSummarySemantic(3), OperandContent: externalUnitContent, Semantic: externalSummarySemantic(4), Output: factor.Output(), Inputs: 0,
		Admission: engine.AdmitRuleByTrustedTheorem[uint64, externalUnit](externalSummarySemantic(5)),
		Transfer:  func(engine.Access[uint64, externalUnit]) bool { return true },
	}, func(rule *engine.Rule[uint64, externalUnit]) bool {
		for index := 0; index < 3; index++ {
			var written bool
			seedWrites[index], written = engine.WriteTo(rule, write)
			if !written {
				return false
			}
		}
		return true
	})
	var whole engine.Read[uint64]
	var projectWrite engine.Write[uint64]
	project, projectOK := engine.DeclareRule(composition, engine.RuleSpec[uint64, externalUnit]{
		OperandFamily: externalSummarySemantic(6), OperandContent: externalUnitContent, Semantic: externalSummarySemantic(7), Output: factor.Output(), Inputs: 1,
		Admission: engine.AdmitRuleByTrustedTheorem[uint64, externalUnit](externalSummarySemantic(8)),
		Transfer:  func(engine.Access[uint64, externalUnit]) bool { return true },
	}, func(rule *engine.Rule[uint64, externalUnit]) bool {
		input, inputOK := rule.InputAt(0)
		var summaryOK bool
		whole, summaryOK = engine.ReadFrom(rule, input, summary)
		var writeOK bool
		projectWrite, writeOK = engine.WriteTo(rule, write)
		return inputOK && summaryOK && writeOK
	})
	var token engine.QueryRead[engine.OrderedCells[uint64]]
	query, queryOK := engine.DeclareQuery(composition, engine.QuerySpec[uint64]{
		Semantic: externalSummarySemantic(9), Project: func(engine.Observation) uint64 { return 0 },
		Result: engine.FrozenResult[uint64]{
			Semantic: externalSummarySemantic(10), Freeze: func(value uint64) uint64 { return value }, Clone: func(value uint64) uint64 { return value },
			Equal: func(left, right uint64) bool { return left == right }, Fingerprint: func(value uint64) uint64 { return value },
		},
	}, func(query *engine.Query[uint64]) bool {
		var tokenOK bool
		token, tokenOK = engine.QueryReadFrom(query, read)
		return tokenOK
	})
	if !readOK || !writeOK || !seedOK || seed == nil || !projectOK || project == nil || !queryOK || query == nil || !composition.Seal() {
		t.Fatal("external closed refs declaration")
	}
	_ = whole

	closed := factor.NewClosedRefs()
	zero, zeroOK := factor.Ref(0)
	one, oneOK := factor.Ref(1)
	two, twoOK := factor.Ref(2)
	if closed == nil || !zeroOK || !oneOK || !twoOK || !closed.Append(two) || !closed.Append(zero) || !closed.Append(one) {
		t.Fatal("external private-key vector build")
	}
	if closed.Append(one) {
		t.Fatal("ClosedRefs accepted a duplicate private key")
	}
	if !closed.Close() {
		t.Fatal("external private-key vector seal")
	}
	if closed.Close() || closed.Append(zero) {
		t.Fatal("ClosedRefs accepted post-seal mutation")
	}
	seedInstance, seeded := engine.NewRuleInstance(seed, externalUnitForSemantic(externalSummarySemantic(15)), func(binding *engine.RuleBinding[uint64, externalUnit]) bool {
		return engine.InstanceWrite(binding, seedWrites[0], zero) &&
			engine.InstanceWrite(binding, seedWrites[1], one) &&
			engine.InstanceWrite(binding, seedWrites[2], two)
	})
	projectInstance, projected := engine.NewRuleInstance(project, externalUnitForSemantic(externalSummarySemantic(16)), func(binding *engine.RuleBinding[uint64, externalUnit]) bool {
		return engine.InstanceSummaryRead(binding, whole, summary, closed) && engine.InstanceWrite(binding, projectWrite, zero)
	})

	source := engine.NewSourceAssembly(composition)
	scope, scopeOK := source.Scope()
	truth, truthOK := source.TrueExpr()
	falsity, falseOK := source.FalseExpr()
	seedSite, seedSiteOK := source.Site(externalSummarySemantic(11), scope, truth, true)
	projectSite, projectSiteOK := source.Site(externalSummarySemantic(12), scope, falsity, false)
	seedOccurrence, seedOccurred := source.Relation(seedSite, externalSummarySemantic(13))
	projectOccurrence, projectOccurred := source.Relation(projectSite, externalSummarySemantic(14))
	seedPrepared, seedPreparedOK := source.PrepareInstance(seedOccurrence, seedInstance)
	projectPrepared, projectPreparedOK := source.PrepareInstance(projectOccurrence, projectInstance)
	reindex, reindexOK := source.IdentityReindex(scope)
	boundary, boundaryOK := source.Boundary(seedSite, projectSite, externalSummarySemantic(17), truth, reindex, truth)
	if source == nil || !scopeOK || !truthOK || !falseOK || !seedSiteOK || !projectSiteOK || !seedOccurred || !projectOccurred || !seeded || !projected || !seedPreparedOK || !projectPreparedOK || !reindexOK || !boundaryOK || !source.Seal() {
		t.Fatal("external source assembly")
	}
	if solver, compiled := source.Assemble(func(assembly *engine.Assembly) bool {
		seedPoint, seedPointOK := assembly.Point(seedSite)
		projectPoint, projectPointOK := assembly.Point(projectSite)
		seedMember, seedMemberOK := assembly.Member(seedPoint, seedPrepared)
		projectMember, projectMemberOK := assembly.Member(projectPoint, projectPrepared)
		_, seedGroupOK := assembly.Group(seedPoint, seedMember)
		projectGroup, projectGroupOK := assembly.Group(projectPoint, projectMember)
		queryInstance, observed := engine.NewQueryInstance(query, func(binding *engine.QueryBinding[uint64]) bool {
			return engine.InstanceQueryRead(binding, token, zero)
		})
		_, observationOK := assembly.Query(projectPoint, queryInstance)
		return seedPointOK && projectPointOK && seeded && projected && observed && queryInstance != nil &&
			seedMemberOK && projectMemberOK && seedGroupOK && projectGroupOK && observationOK &&
			assembly.Boundary(projectGroup, boundary)
	}); !compiled || solver == nil {
		t.Fatal("external private-key vector did not compile through Assembly")
	}
}
