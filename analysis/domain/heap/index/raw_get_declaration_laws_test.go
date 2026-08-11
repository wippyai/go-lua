package index

import (
	"testing"

	callowner "github.com/wippyai/go-lua/analysis/domain/call/owner"
	heapdomain "github.com/wippyai/go-lua/analysis/domain/heap"
	heapowner "github.com/wippyai/go-lua/analysis/domain/heap/owner"
	"github.com/wippyai/go-lua/analysis/domain/pack"
	packowner "github.com/wippyai/go-lua/analysis/domain/pack/owner"
	"github.com/wippyai/go-lua/analysis/domain/static"
	valuedomain "github.com/wippyai/go-lua/analysis/domain/value"
	valueowner "github.com/wippyai/go-lua/analysis/domain/value/owner"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/semantic/typeauthority"
)

func TestRawGetDeclarationCarriesValueSelfDependence(t *testing.T) {
	composition, values, _ := rawGetDeclaration(t)
	if !declareRawGetDeclarationQuery(composition, values) || !composition.Seal() {
		t.Fatal("raw-get declaration composition seal")
	}
	report, ok := composition.SemanticReport()
	if !ok {
		t.Fatal("raw-get declaration semantic report")
	}
	want := engine.FactorIncidence{Read: rawGetKey(201), Write: rawGetKey(201)}
	for _, incidence := range report.Incidences {
		if incidence == want {
			return
		}
	}
	t.Fatal("raw-get ordinary Value carry did not publish Value self-dependence")
}

func TestRawGetDeclarationRejectsForgedZeroDerivation(t *testing.T) {
	_, _, rule := rawGetDeclaration(t)
	if evidence, accepted := rule.check(rawGetKey(206))(engine.RuleDerivation[valuedomain.Value, Access]{}); accepted || evidence != (engine.RuleEvidence{}) {
		t.Fatal("forged zero derivation minted raw-get evidence")
	}
}

func TestRawGetDeclarationRejectsSameLinkResealedHeap(t *testing.T) {
	linked, heapSchema, valueSchema, callSchema, packSchema, topology, _, _ := rawGetFieldFixture(t)
	resealed, resealedOK := heapdomain.Seal(linked)
	if !resealedOK || valueSchema.OwnsHeapSchema(resealed) {
		t.Fatal("independently resealed same-Link Heap was not distinguished")
	}

	composition := engine.NewComposition()
	values, valuesOK := valueowner.Declare(composition, rawGetKey(230), rawGetKey(231), valueSchema)
	calls, callsOK := callowner.Declare(composition, rawGetKey(232), callSchema)
	wrongHeap, wrongHeapOK := heapowner.Declare(composition, rawGetKey(233), resealed)
	packs, packsOK := packowner.Declare(composition, rawGetKey(234), packSchema)
	if !valuesOK || !callsOK || !wrongHeapOK || !packsOK {
		t.Fatal("raw-get owner declarations for resealed Heap")
	}
	if rule, ok := DeclareRawGet(composition, rawGetKey(235), rawGetKey(236), rawGetKey(237), topology, values, calls, wrongHeap, packs); ok || rule != nil {
		t.Fatal("RawGet accepted independently resealed Heap owner")
	}

	exactHeap, exactHeapOK := heapowner.Declare(composition, rawGetKey(238), heapSchema)
	if !exactHeapOK {
		t.Fatal("exact Heap owner declaration")
	}

	foreignLink := sameContentLink(t, linked)
	types, typesOK := typeauthority.Seal(foreignLink)
	statics, _, staticErr := static.Seal(foreignLink, types)
	foreignPack, foreignPackOK := pack.Seal(foreignLink, statics)
	foreignPacks, foreignPacksOK := packowner.Declare(composition, rawGetKey(242), foreignPack)
	if !typesOK || staticErr != nil || !foreignPackOK || !foreignPacksOK || foreignLink.ContentID() != linked.ContentID() || foreignLink == linked {
		t.Fatalf("same-content independent Pack fixture types=%t staticErr=%v pack=%t owner=%t sameID=%t sameLink=%t", typesOK, staticErr, foreignPackOK, foreignPacksOK, foreignLink.ContentID() == linked.ContentID(), foreignLink == linked)
	}
	if rule, ok := DeclareRawGet(composition, rawGetKey(239), rawGetKey(240), rawGetKey(241), topology, values, calls, exactHeap, packs); !ok || rule == nil {
		t.Fatal("RawGet rejected exact Value/Heap schema pair")
	}
	if rule, ok := DeclareRawGet(composition, rawGetKey(243), rawGetKey(244), rawGetKey(245), topology, values, calls, exactHeap, foreignPacks); ok || rule != nil {
		t.Fatal("RawGet accepted same-content independent Pack schema")
	}
}

func TestRawGetRejectsDuplicateSameContentTopologyAccess(t *testing.T) {
	_, heapSchema, valueSchema, callSchema, packSchema, topology, access, _ := rawGetFieldFixture(t)
	duplicate, duplicateOK := Seal(heapSchema, valueSchema, callSchema)
	indexAccess, indexAccessOK := access.IndexAccess()
	foreignAccess, foreignAccessOK := duplicate.Access(indexAccess)
	if !duplicateOK || duplicate == nil || !indexAccessOK || !foreignAccessOK {
		t.Fatal("duplicate topology access fixture")
	}
	composition := engine.NewComposition()
	values, valuesOK := valueowner.Declare(composition, rawGetKey(150), rawGetKey(151), valueSchema)
	calls, callsOK := callowner.Declare(composition, rawGetKey(152), callSchema)
	heap, heapOK := heapowner.Declare(composition, rawGetKey(153), heapSchema)
	packs, packsOK := packowner.Declare(composition, rawGetKey(154), packSchema)
	rule, ruleOK := DeclareRawGet(composition, rawGetKey(155), rawGetKey(156), rawGetKey(157), topology, values, calls, heap, packs)
	if !valuesOK || !callsOK || !heapOK || !packsOK || !ruleOK || rule == nil {
		t.Fatal("raw-get duplicate topology declaration")
	}
	if !declareRawGetDeclarationQuery(composition, values) || !composition.Seal() {
		t.Fatal("raw-get duplicate topology composition seal")
	}
	if _, ok := rule.Instance(access); !ok {
		t.Fatal("RawGet rejected access from issuing topology")
	}
	if _, ok := rule.Instance(foreignAccess); ok {
		t.Fatal("RawGet accepted duplicate same-content topology access")
	}
}

func rawGetDeclaration(t testing.TB) (*engine.Composition, *valueowner.Owner, *RawGetRule) {
	t.Helper()
	_, heapSchema, valueSchema, callSchema, packSchema, topology, _, _ := rawGetFieldFixture(t)
	composition := engine.NewComposition()
	values, valuesOK := valueowner.Declare(composition, rawGetKey(201), rawGetKey(202), valueSchema)
	calls, callsOK := callowner.Declare(composition, rawGetKey(203), callSchema)
	heap, heapOK := heapowner.Declare(composition, rawGetKey(204), heapSchema)
	packs, packsOK := packowner.Declare(composition, rawGetKey(205), packSchema)
	rule, ruleOK := DeclareRawGet(composition, rawGetKey(206), rawGetKey(207), rawGetKey(208), topology, values, calls, heap, packs)
	if !valuesOK || !callsOK || !heapOK || !packsOK || !ruleOK || rule == nil {
		t.Fatal("raw-get declaration")
	}
	return composition, values, rule
}

func declareRawGetDeclarationQuery(composition *engine.Composition, values *valueowner.Owner) bool {
	query, ok := engine.DeclareQuery(composition, engine.QuerySpec[bool]{
		Semantic: rawGetKey(209),
		Project:  func(engine.Observation) bool { return true },
		Result: engine.FrozenResult[bool]{
			Semantic:    rawGetKey(210),
			Freeze:      func(value bool) bool { return value },
			Clone:       func(value bool) bool { return value },
			Equal:       func(left, right bool) bool { return left == right },
			Fingerprint: func(value bool) uint64 { return uint64(0) },
		},
	}, func(query *engine.Query[bool]) bool {
		_, ok := engine.QueryReadFrom(query, values.ExactRead())
		return ok
	})
	return ok && query != nil
}
