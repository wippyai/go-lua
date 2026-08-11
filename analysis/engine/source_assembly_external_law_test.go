package engine_test

import (
	"testing"

	engine "github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/lattice"
)

type facadeUnit struct{ digest [32]byte }

func facadeKey(value byte) engine.SemanticKey {
	var digest [32]byte
	digest[31] = value
	key, ok := engine.NewSemanticKey(digest, 1)
	if !ok {
		panic("facade semantic key")
	}
	return key
}

func facadeUnitFor(key engine.SemanticKey) facadeUnit { return facadeUnit{digest: key.Digest()} }

func facadeUnitContent(value facadeUnit) (facadeUnit, [32]byte, bool) {
	return value, value.digest, value.digest != [32]byte{}
}

func facadeLattice() lattice.Lattice[uint64] {
	return lattice.Lattice[uint64]{
		Bottom: func() uint64 { return 0 }, Top: func() uint64 { return ^uint64(0) },
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
		Narrow: func(left, right uint64) uint64 {
			if left < right {
				return left
			}
			return right
		},
	}
}

// TestExternalSourceAssemblyOwnsOneOpaqueTwoStageTopology proves the
// production cut can be used without importing equation or naming any of its
// values. The same source owner issues stage-one rows and stage-two opaque
// Point/Member/Group/Query capabilities.
func TestExternalSourceAssemblyOwnsOneOpaqueTwoStageTopology(t *testing.T) {
	composition := engine.NewComposition()
	factor, factorOK := engine.DeclareFactor(composition, engine.FactorSpec[uint64, uint64]{
		Semantic: facadeKey(1), KeyEnd: 1, Lattice: facadeLattice(), Default: 0,
		AdmitAt: func(uint64, uint64) bool { return true }, Fingerprint: func(value uint64) uint64 { return value },
		WidenRank:  engine.Measure[uint64, uint64]{Width: 1, At: func(_ uint64, value uint64, _ int) uint64 { return ^value }},
		NarrowRank: engine.Measure[uint64, uint64]{Width: 1, At: func(_ uint64, value uint64, _ int) uint64 { return value }},
	}, func(*engine.Factor[uint64, uint64]) bool { return true })
	read, readOK := engine.ExactReadForm(factor)
	write, writeOK := engine.ExactWriteForm(factor)
	var ruleWrite engine.Write[uint64]
	var ruleRead engine.Read[engine.OrderedCells[uint64]]
	rule, ruleOK := engine.DeclareRule(composition, engine.RuleSpec[uint64, facadeUnit]{
		Semantic: facadeKey(2), OperandFamily: facadeKey(3), OperandContent: facadeUnitContent,
		Output: factor.Output(), Inputs: 1,
		Admission: engine.AdmitRuleByTrustedTheorem[uint64, facadeUnit](facadeKey(4)),
		Transfer: func(value engine.Access[uint64, facadeUnit]) bool {
			return engine.Product(value, func(row engine.Row) bool { return engine.StageValue(value, row, 1) })
		},
	}, func(rule *engine.Rule[uint64, facadeUnit]) bool {
		input, inputOK := rule.InputAt(0)
		var readOK bool
		ruleRead, readOK = engine.ReadFrom(rule, input, read)
		var writeOK bool
		ruleWrite, writeOK = engine.WriteTo(rule, write)
		return inputOK && readOK && writeOK
	})
	var queryRead engine.QueryRead[engine.OrderedCells[uint64]]
	query, queryOK := engine.DeclareQuery(composition, engine.QuerySpec[uint64]{
		Semantic: facadeKey(5), Project: func(engine.Observation) uint64 { return 0 },
		Result: engine.FrozenResult[uint64]{
			Semantic: facadeKey(6), Freeze: func(value uint64) uint64 { return value }, Clone: func(value uint64) uint64 { return value },
			Equal: func(left, right uint64) bool { return left == right }, Fingerprint: func(value uint64) uint64 { return value },
		},
	}, func(value *engine.Query[uint64]) bool {
		var ok bool
		queryRead, ok = engine.QueryReadFrom(value, read)
		return ok
	})
	if !factorOK || factor == nil || !readOK || !writeOK || !ruleOK || rule == nil || !queryOK || query == nil || !composition.Seal() {
		t.Fatal("cold facade declaration")
	}
	ref, refOK := factor.Ref(0)
	instance, instanceOK := engine.NewRuleInstance(rule, facadeUnitFor(facadeKey(7)), func(binding *engine.RuleBinding[uint64, facadeUnit]) bool {
		return engine.InstanceRead(binding, ruleRead, ref) && engine.InstanceWrite(binding, ruleWrite, ref)
	})
	if !refOK || !instanceOK || instance == nil {
		t.Fatal("typed facade instance")
	}

	source := engine.NewSourceAssembly(composition)
	scope, scopeOK := source.Scope()
	truth, truthOK := source.TrueExpr()
	site, siteOK := source.Site(facadeKey(8), scope, truth, true)
	occurrence, occurrenceOK := source.At(site)
	prepared, operandOK := source.PrepareInstance(occurrence, instance)
	if prepared.Available() {
		t.Fatal("prepared source instance became available before source sealing")
	}
	reindex, reindexOK := source.IdentityReindex(scope)
	boundary, boundaryOK := source.Boundary(site, site, facadeKey(9), truth, reindex, truth)
	if boundary.Available() {
		t.Fatal("boundary became available before source sealing")
	}
	sealed := source.Seal()
	if !scopeOK || !truthOK || !siteOK || !occurrenceOK || !operandOK || !sealed || !prepared.Available() || !reindexOK || !boundaryOK || !boundary.Available() {
		t.Fatal("source stage")
	}

	var pointOK, memberOK, groupOK, queryInstanceOK, queryAttached, boundaryAttached bool
	var point engine.AssemblyPoint
	var member engine.AssemblyMember
	var group engine.AssemblyGroup
	var queryInstance *engine.QueryInstance[uint64]
	solver, assembled := source.Assemble(func(value *engine.Assembly) bool {
		point, pointOK = value.Point(site)
		member, memberOK = value.Member(point, prepared)
		group, groupOK = value.Group(point, member)
		queryInstance, queryInstanceOK = engine.NewQueryInstance(query, func(binding *engine.QueryBinding[uint64]) bool {
			return engine.InstanceQueryRead(binding, queryRead, ref)
		})
		_, queryAttached = value.Query(point, queryInstance)
		boundaryAttached = value.Boundary(group, boundary)
		return pointOK && memberOK && groupOK && queryInstanceOK && queryAttached && boundaryAttached
	})
	if !assembled || solver == nil {
		t.Fatalf("production source assembly assembled=%t solver=%p point=%t member=%t group=%t query-instance=%t query=%t boundary=%t", assembled, solver, pointOK, memberOK, groupOK, queryInstanceOK, queryAttached, boundaryAttached)
	}

	copyOfSource := *source
	if _, ok := copyOfSource.PrepareInstance(occurrence, instance); ok {
		t.Fatal("copied source authority prepared an instance")
	}
	if _, ok := copyOfSource.Assemble(func(*engine.Assembly) bool { return true }); ok {
		t.Fatal("copied source authority assembled")
	}
	foreign := engine.NewSourceAssembly(composition)
	if _, ok := foreign.PrepareInstance(occurrence, instance); ok {
		t.Fatal("foreign source accepted a prepared instance")
	}
	if _, ok := foreign.Boundary(site, site, facadeKey(10), truth, reindex, truth); ok {
		t.Fatal("foreign source accepted a Site")
	}
	if _, ok := source.Assemble(func(*engine.Assembly) bool { return true }); ok {
		t.Fatal("source assembled twice")
	}
}
