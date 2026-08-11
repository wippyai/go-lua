package engine_test

import (
	"testing"

	engine "github.com/wippyai/go-lua/analysis/engine"
)

type sourceInputCountFixture struct {
	source          *engine.SourceAssembly
	foreign         *engine.SourceAssembly
	prepared        engine.SourceInstance
	foreignPrepared engine.SourceInstance
}

func newSourceInputCountFixture(t testing.TB, inputs int) sourceInputCountFixture {
	t.Helper()
	composition := engine.NewComposition()
	factor, factorOK := engine.DeclareFactor(composition, engine.FactorSpec[uint64, uint64]{
		Semantic: sourceInputCountKey(1), KeyEnd: 1, Lattice: facadeLattice(), Default: 0,
		AdmitAt: func(uint64, uint64) bool { return true }, Fingerprint: func(value uint64) uint64 { return value },
		WidenRank:  engine.Measure[uint64, uint64]{Width: 1, At: func(_ uint64, value uint64, _ int) uint64 { return ^value }},
		NarrowRank: engine.Measure[uint64, uint64]{Width: 1, At: func(_ uint64, value uint64, _ int) uint64 { return value }},
	}, func(*engine.Factor[uint64, uint64]) bool { return true })
	write, writeOK := engine.ExactWriteForm(factor)
	var ruleWrite engine.Write[uint64]
	rule, ruleOK := engine.DeclareRule(composition, engine.RuleSpec[uint64, facadeUnit]{
		Semantic: sourceInputCountKey(2), OperandFamily: sourceInputCountKey(3), OperandContent: facadeUnitContent,
		Output: factor.Output(), Inputs: inputs, Admission: engine.AdmitRuleByTrustedTheorem[uint64, facadeUnit](sourceInputCountKey(4)),
		Transfer: func(engine.Access[uint64, facadeUnit]) bool { return true },
	}, func(rule *engine.Rule[uint64, facadeUnit]) bool {
		var declared bool
		ruleWrite, declared = engine.WriteTo(rule, write)
		return declared
	})
	read, readOK := engine.ExactReadForm(factor)
	query, queryOK := engine.DeclareQuery(composition, engine.QuerySpec[uint64]{
		Semantic: sourceInputCountKey(9),
		Project:  func(engine.Observation) uint64 { return 0 },
		Result: engine.FrozenResult[uint64]{
			Semantic: sourceInputCountKey(10), Freeze: func(value uint64) uint64 { return value }, Clone: func(value uint64) uint64 { return value },
			Equal: func(left, right uint64) bool { return left == right }, Fingerprint: func(value uint64) uint64 { return value },
		},
	}, func(value *engine.Query[uint64]) bool {
		_, declared := engine.QueryReadFrom(value, read)
		return declared
	})
	if !factorOK || factor == nil || !writeOK || !ruleOK || rule == nil || !readOK || !queryOK || query == nil || !composition.Seal() {
		t.Fatalf("input-count cold declaration inputs=%d factor=%t write=%t rule=%t read=%t query=%t", inputs, factorOK, writeOK, ruleOK, readOK, queryOK)
	}
	ref, refOK := factor.Ref(0)
	first, firstOK := engine.NewRuleInstance(rule, facadeUnitFor(sourceInputCountKey(5)), func(binding *engine.RuleBinding[uint64, facadeUnit]) bool {
		return engine.InstanceWrite(binding, ruleWrite, ref)
	})
	second, secondOK := engine.NewRuleInstance(rule, facadeUnitFor(sourceInputCountKey(6)), func(binding *engine.RuleBinding[uint64, facadeUnit]) bool {
		return engine.InstanceWrite(binding, ruleWrite, ref)
	})
	if !refOK || !firstOK || first == nil || !secondOK || second == nil {
		t.Fatalf("input-count typed instances inputs=%d ref=%t first=%t second=%t", inputs, refOK, firstOK, secondOK)
	}
	source := engine.NewSourceAssembly(composition)
	foreign := engine.NewSourceAssembly(composition)
	if source == nil || foreign == nil {
		t.Fatalf("input-count source assemblies inputs=%d", inputs)
	}
	scope, scopeOK := source.Scope()
	truth, truthOK := source.TrueExpr()
	site, siteOK := source.Site(sourceInputCountKey(7), scope, truth, true)
	occurrence, occurrenceOK := source.At(site)
	foreignScope, foreignScopeOK := foreign.Scope()
	foreignTruth, foreignTruthOK := foreign.TrueExpr()
	foreignSite, foreignSiteOK := foreign.Site(sourceInputCountKey(8), foreignScope, foreignTruth, true)
	foreignOccurrence, foreignOccurrenceOK := foreign.At(foreignSite)
	if !scopeOK || !truthOK || !siteOK || !occurrenceOK || !foreignScopeOK || !foreignTruthOK || !foreignSiteOK || !foreignOccurrenceOK {
		t.Fatalf("input-count source rows inputs=%d source=(%t,%t,%t,%t) foreign=(%t,%t,%t,%t)", inputs, scopeOK, truthOK, siteOK, occurrenceOK, foreignScopeOK, foreignTruthOK, foreignSiteOK, foreignOccurrenceOK)
	}
	// A source occurrence is owner-fenced before admission.  These failed
	// attempts must not consume either typed instance, leaving the two exact
	// owner-local admissions below available.
	if _, accepted := source.PrepareInstance(foreignOccurrence, first); accepted {
		t.Fatal("source accepted a foreign occurrence")
	}
	if _, accepted := foreign.PrepareInstance(occurrence, second); accepted {
		t.Fatal("foreign source accepted a source occurrence")
	}
	prepared, preparedOK := source.PrepareInstance(occurrence, first)
	foreignPrepared, foreignPreparedOK := foreign.PrepareInstance(foreignOccurrence, second)
	if !preparedOK || !foreignPreparedOK {
		t.Fatalf("input-count owner-local admissions inputs=%d source=%t foreign=%t", inputs, preparedOK, foreignPreparedOK)
	}
	return sourceInputCountFixture{source: source, foreign: foreign, prepared: prepared, foreignPrepared: foreignPrepared}
}

func sourceInputCountKey(value byte) engine.SemanticKey {
	var digest [32]byte
	digest[30] = 0x5a
	digest[31] = value
	key, ok := engine.NewSemanticKey(digest, 1)
	if !ok {
		panic("source input-count semantic key")
	}
	return key
}

func TestExternalSourceInstanceInputCountProjectionLaws(t *testing.T) {
	var zero engine.SourceInstance

	for _, want := range []int{0, 1, 4} {
		fixture := newSourceInputCountFixture(t, want)
		if count, ok := fixture.source.InputCount(zero); ok || count != 0 {
			t.Fatalf("unprepared source instance inputs=%d count=%d ok=%t", want, count, ok)
		}
		if count, ok := fixture.source.InputCount(fixture.foreignPrepared); ok || count != 0 {
			t.Fatalf("foreign source instance inputs=%d count=%d ok=%t", want, count, ok)
		}
		if count, ok := fixture.foreign.InputCount(fixture.prepared); ok || count != 0 {
			t.Fatalf("foreign assembly instance inputs=%d count=%d ok=%t", want, count, ok)
		}
		for _, fixtureCase := range []struct {
			name     string
			owner    *engine.SourceAssembly
			prepared engine.SourceInstance
		}{
			{name: "source", owner: fixture.source, prepared: fixture.prepared},
			{name: "foreign", owner: fixture.foreign, prepared: fixture.foreignPrepared},
		} {
			name, owner, prepared := fixtureCase.name, fixtureCase.owner, fixtureCase.prepared
			count, ok := owner.InputCount(prepared)
			if !ok || count != want {
				t.Fatalf("%s open source instance inputs=%d count=%d ok=%t", name, want, count, ok)
			}
			for repeat := 0; repeat < 3; repeat++ {
				if repeated, repeatedOK := owner.InputCount(prepared); !repeatedOK || repeated != want {
					t.Fatalf("%s unstable source instance inputs=%d repeat=%d count=%d ok=%t", name, want, repeat, repeated, repeatedOK)
				}
			}
			allocations := testing.AllocsPerRun(256, func() {
				count, ok = owner.InputCount(prepared)
			})
			if allocations != 0 || !ok || count != want {
				t.Fatalf("%s source input-count projection inputs=%d count=%d ok=%t allocations=%v", name, want, count, ok, allocations)
			}
		}
		if !fixture.source.Seal() || !fixture.foreign.Seal() {
			t.Fatalf("source sealing inputs=%d", want)
		}
		if count, ok := fixture.source.InputCount(fixture.prepared); ok || count != 0 {
			t.Fatalf("post-seal source instance inputs=%d count=%d ok=%t", want, count, ok)
		}
		if count, ok := fixture.foreign.InputCount(fixture.foreignPrepared); ok || count != 0 {
			t.Fatalf("post-seal foreign instance inputs=%d count=%d ok=%t", want, count, ok)
		}
	}
}
