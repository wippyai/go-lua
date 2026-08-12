package dispatch

import (
	"testing"

	calldomain "github.com/wippyai/go-lua/analysis/domain/call"
	callowner "github.com/wippyai/go-lua/analysis/domain/call/owner"
	heapdomain "github.com/wippyai/go-lua/analysis/domain/heap"
	"github.com/wippyai/go-lua/analysis/domain/materialization"
	packdomain "github.com/wippyai/go-lua/analysis/domain/pack"
	"github.com/wippyai/go-lua/analysis/domain/runtimekind"
	staticdomain "github.com/wippyai/go-lua/analysis/domain/static"
	valuedomain "github.com/wippyai/go-lua/analysis/domain/value"
	valueowner "github.com/wippyai/go-lua/analysis/domain/value/owner"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/semantic/typeauthority"
	"github.com/wippyai/go-lua/program"
	flowkind "github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/link"
	linkboundary "github.com/wippyai/go-lua/program/link/boundary"
	linkmodule "github.com/wippyai/go-lua/program/link/module"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	programlower "github.com/wippyai/go-lua/program/lower"
	"github.com/wippyai/go-lua/program/target"
)

type dispatchFixture struct {
	linked  *link.Link
	program *program.Program
	algebra *calldomain.Algebra
	heaps   heapdomain.Schema
	values  *valuedomain.Schema
	packs   *packdomain.Schema
	app     linkproject.Application
}

func newDispatchFixture(t *testing.T, contract *target.Contract, endpoints []linkboundary.EndpointRequest) dispatchFixture {
	t.Helper()
	p, err := programlower.Lower(programlower.Source{Name: "call_dispatch", Text: []byte(`
local function exact() return 1 end
exact()
`)})
	if err != nil {
		t.Fatal(err)
	}
	if contract == nil {
		contract, err = target.Seal(&target.Spec{})
		if err != nil {
			t.Fatal(err)
		}
	}
	linked, err := link.Seal(&link.Spec{
		Target:           contract,
		Modules:          []linkproject.Module{{Name: "call_dispatch", Program: p}},
		EndpointRequests: endpoints,
	})
	if err != nil {
		t.Fatal(err)
	}
	heaps, ok := heapdomain.Seal(linked)
	if !ok {
		t.Fatal("Heap schema")
	}
	values, ok := valuedomain.Seal(linked, heaps)
	if !ok {
		t.Fatal("Value schema")
	}
	types, ok := typeauthority.Seal(linked)
	if !ok {
		t.Fatal("type authority")
	}
	statics, _, err := staticdomain.Seal(linked, types)
	if err != nil {
		t.Fatal(err)
	}
	packs, ok := packdomain.Seal(linked, statics)
	if !ok {
		t.Fatal("Pack schema")
	}
	algebra, ok := calldomain.New(linked)
	if !ok {
		t.Fatal("Call algebra")
	}
	app := applicationFor(t, linked, p, 0)
	return dispatchFixture{linked: linked, program: p, algebra: algebra, heaps: heaps, values: values, packs: packs, app: app}
}

func (fixture dispatchFixture) siteFor(t *testing.T) site {
	t.Helper()
	bound, ok := newSite(fixture.algebra, fixture.values, fixture.heaps, fixture.packs, fixture.app)
	if !ok {
		t.Fatal("dispatch site")
	}
	return bound
}

func applicationFor(t *testing.T, source *link.Link, p *program.Program, ordinal int) linkproject.Application {
	t.Helper()
	want, ok := p.Flow().Authored().Calls().At(ordinal)
	if !ok {
		t.Fatalf("CallAt(%d)", ordinal)
	}
	calls := source.Project().Applications().Calls()
	applications := source.Project().Applications()
	for index := 0; index < calls.Count(); index++ {
		application, present := calls.At(index)
		_, occurrence, mapped := applications.Call(application)
		if !present || !mapped {
			t.Fatal("malformed Link Call application table")
		}
		if occurrence == want {
			return application
		}
	}
	t.Fatalf("CallAt(%d) not linked", ordinal)
	return linkproject.Application{}
}

func closureFact(t *testing.T, fixture dispatchFixture) valuedomain.Value {
	t.Helper()
	shard, ok := fixture.linked.Project().Mounts().At(0)
	if !ok {
		t.Fatal("shard")
	}
	function, ok := fixture.program.Flow().Authored().Functions().At(0)
	if !ok {
		t.Fatal("function")
	}
	for index := 0; index < fixture.heaps.KeyCount(); index++ {
		key, keyOK := fixture.heaps.KeyAt(index)
		candidateShard, candidateTerm, kind, allocationOK := key.ProgramAllocation()
		if !keyOK || !allocationOK || candidateShard != shard || candidateTerm != function || kind != heapdomain.AllocationClosure {
			continue
		}
		atom, atomOK := fixture.values.Allocation(key, materialization.Recent)
		fact, factOK := fixture.values.Singleton(atom)
		if !atomOK || !factOK {
			t.Fatal("closure fact")
		}
		return fact
	}
	t.Fatal("closure allocation")
	return valuedomain.Value{}
}

func TestDispatchExactClosureIsCompleteAndFunctionSelected(t *testing.T) {
	fixture := newDispatchFixture(t, nil, nil)
	bound := fixture.siteFor(t)
	fact := closureFact(t, fixture)
	result, ok := reduce(bound, fact)
	if !ok || !result.IsComplete() || result.HasOpaqueAlternative() || result.KnownTargetCount() != 1 {
		t.Fatalf("closure dispatch = complete:%v opaque:%v targets:%d", result.IsComplete(), result.HasOpaqueAlternative(), result.KnownTargetCount())
	}
	shard, _ := fixture.linked.Project().Mounts().At(0)
	function, _ := fixture.program.Flow().Authored().Functions().At(0)
	target, targetOK := fixture.algebra.TargetForFunction(shard, function)
	if !targetOK {
		t.Fatal("closure selector missing")
	}
	if !result.HasTarget(target) || !target.Valid() {
		t.Fatal("closure target capability missing")
	}
}

func TestDispatchQuotientsRecentAndSummaryToOneClosureTarget(t *testing.T) {
	fixture := newDispatchFixture(t, nil, nil)
	shard, ok := fixture.linked.Project().Mounts().At(0)
	if !ok {
		t.Fatal("shard")
	}
	function, ok := fixture.program.Flow().Authored().Functions().At(0)
	if !ok {
		t.Fatal("function")
	}
	var closureKey heapdomain.Key
	for index := 0; index < fixture.heaps.KeyCount(); index++ {
		candidate, candidateOK := fixture.heaps.KeyAt(index)
		candidateShard, candidateTerm, kind, allocationOK := candidate.ProgramAllocation()
		if candidateOK && allocationOK && candidateShard == shard && candidateTerm == function && kind == heapdomain.AllocationClosure {
			closureKey = candidate
			break
		}
	}
	if !closureKey.Valid() {
		t.Fatal("closure key")
	}
	recent, recentOK := fixture.values.Allocation(closureKey, materialization.Recent)
	summary, summaryOK := fixture.values.Allocation(closureKey, materialization.Summary)
	recentValue, recentValueOK := fixture.values.Singleton(recent)
	summaryValue, summaryValueOK := fixture.values.Singleton(summary)
	joined, joinedOK := fixture.values.Join(recentValue, summaryValue)
	if !recentOK || !summaryOK || !recentValueOK || !summaryValueOK || !joinedOK || fixture.values.Equal(recentValue, summaryValue) {
		t.Fatal("distinct closure materialization atoms")
	}
	if !fixture.values.LessOrEq(joined, fixture.values.Top()) {
		t.Fatal("finite callee was not below Value Top")
	}
	result, resultOK := reduce(fixture.siteFor(t), joined)
	target, targetOK := fixture.algebra.TargetForFunction(shard, function)
	if !resultOK || !result.IsComplete() || result.HasOpaqueAlternative() || !targetOK || result.KnownTargetCount() != 1 || !result.HasTarget(target) {
		t.Fatal("duplicate closure targets were not quotiented")
	}
	if !fixture.algebra.LessOrEq(result, fixture.algebra.Top()) {
		t.Fatal("finite dispatch was not below Call Top")
	}
}

func TestDispatchKnownNonFunctionIsCompleteEmpty(t *testing.T) {
	fixture := newDispatchFixture(t, nil, nil)
	atom, ok := fixture.values.OpaqueKind(runtimekind.Number)
	if !ok {
		t.Fatal("number atom")
	}
	fact, ok := fixture.values.Singleton(atom)
	if !ok {
		t.Fatal("number fact")
	}
	result, ok := reduce(fixture.siteFor(t), fact)
	if !ok || !result.IsComplete() || result.HasOpaqueAlternative() || result.KnownTargetCount() != 0 {
		t.Fatalf("non-function dispatch = complete:%v opaque:%v targets:%d", result.IsComplete(), result.HasOpaqueAlternative(), result.KnownTargetCount())
	}
}

func TestDispatchTopReturnsCallTop(t *testing.T) {
	fixture := newDispatchFixture(t, nil, nil)
	result, ok := reduce(fixture.siteFor(t), fixture.values.Top())
	if !ok || !result.IsTop() || !fixture.algebra.Equal(result, fixture.algebra.Top()) || !result.HasOpaqueAlternative() {
		t.Fatal("Top callee did not preserve Call Top")
	}
}

func TestDispatchUnresolvedCallableRetainsOpaqueAlternative(t *testing.T) {
	fixture := newDispatchFixture(t, nil, nil)
	atom, ok := fixture.values.OpaqueKind(runtimekind.Function)
	if !ok {
		t.Fatal("function atom")
	}
	fact, ok := fixture.values.Singleton(atom)
	if !ok {
		t.Fatal("function fact")
	}
	result, ok := reduce(fixture.siteFor(t), fact)
	if !ok || !result.IsOpen() || !result.HasOpaqueAlternative() {
		t.Fatal("unresolved callable was incorrectly closed")
	}
}

func TestDispatchEndpointUsesNominalSeed(t *testing.T) {
	contract := callBoundarySeedContract(t)
	provider := target.BindingSpec{Namespace: target.BindingProvider, Owner: []string{"host"}, Member: []string{"dispatch"}}
	fixture := newDispatchFixture(t, contract, []linkboundary.EndpointRequest{{Identity: "first", Binding: provider}})
	endpoint, ok := fixture.linked.Boundary().Endpoints().At(0)
	if !ok {
		t.Fatal("endpoint")
	}
	atom, ok := fixture.values.Endpoint(endpoint)
	if !ok {
		t.Fatal("endpoint atom")
	}
	fact, ok := fixture.values.Singleton(atom)
	if !ok {
		t.Fatal("endpoint fact")
	}
	result, ok := reduce(fixture.siteFor(t), fact)
	seed, seedOK := fixture.linked.Boundary().Endpoints().Seed(endpoint)
	target, targetOK := fixture.algebra.TargetForSeed(seed)
	if !ok || !result.IsComplete() || result.HasOpaqueAlternative() || !seedOK || !targetOK || !result.HasTarget(target) {
		t.Fatal("endpoint selector was not preserved")
	}
}

func TestDispatchScopedRequireUsesBoundApplicationShard(t *testing.T) {
	require := target.BindingSpec{Namespace: target.BindingBuiltin, Member: []string{"require"}}
	operation := target.OperationSpec{
		Bindings: []target.BindingSpec{require},
		Input:    target.ValuesSpec{Tail: target.ValuesClosed},
		Outcomes: []target.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: target.ValuesSpec{Tail: target.ValuesClosed}}},
		Effects:  target.RowSpec{Tail: target.RowClosed},
	}
	contract, err := target.Seal(&target.Spec{
		Operations:   []target.OperationSpec{operation},
		InitialRoots: []target.InitialRootSpec{{Identity: "GlobalEnvRoot", Shape: target.BootShapeSpec{Aggregate: target.BootAggregateTable, Value: target.InitialValueSpec{Kind: target.InitialValueRoot, Root: "GlobalEnvRoot"}}}},
		InitialEntries: []target.InitialEntrySpec{
			{Root: "GlobalEnvRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "_G"}, Value: target.InitialValueSpec{Kind: target.InitialValueRoot, Root: "GlobalEnvRoot"}, Mutability: target.InitialMutable},
			{Root: "GlobalEnvRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "__link_absent"}, Value: target.InitialValueSpec{Kind: target.InitialValueAbsent}, Mutability: target.InitialMutable},
			{Root: "GlobalEnvRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "require"}, Value: target.InitialValueSpec{Kind: target.InitialValueOperation, Operation: require}, Mutability: target.InitialMutable},
		},
		InitialBindings: []target.InitialBindingSpec{
			{Name: "_G", Root: "GlobalEnvRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "_G"}},
			{Name: "__link_absent", Root: "GlobalEnvRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "__link_absent"}},
			{Name: "require", Root: "GlobalEnvRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "require"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	alpha, err := programlower.Lower(programlower.Source{Name: "scoped_loader_alpha.lua", Text: []byte(`require("external-alpha")`)})
	if err != nil {
		t.Fatal(err)
	}
	beta, err := programlower.Lower(programlower.Source{Name: "scoped_loader_beta.lua", Text: []byte(`require("external-beta")`)})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{
		Target:  contract,
		Modules: []linkproject.Module{{Name: "alpha", Program: alpha}, {Name: "beta", Program: beta}},
		Module: linkmodule.Spec{
			Actors: []linkmodule.ActorSpec{{Name: "actor"}},
			ModuleCacheAliases: []linkmodule.ModuleCacheAliasClassSpec{
				{Actor: "actor", Instances: []string{"cache-alpha"}, Representative: "cache-alpha"},
				{Actor: "actor", Instances: []string{"cache-beta"}, Representative: "cache-beta"},
			},
			AnalysisRoots: []linkmodule.AnalysisRootSpec{
				{Name: "alpha-root", Module: "alpha", Actor: "actor", Instance: "cache-alpha"},
				{Name: "beta-root", Module: "beta", Actor: "actor", Instance: "cache-beta"},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	heaps, heapsOK := heapdomain.Seal(linked)
	values, valuesOK := valuedomain.Seal(linked, heaps)
	types, typesOK := typeauthority.Seal(linked)
	statics, _, staticErr := staticdomain.Seal(linked, types)
	packs, packsOK := packdomain.Seal(linked, statics)
	algebra, algebraOK := calldomain.New(linked)
	if !heapsOK || !valuesOK || !typesOK || staticErr != nil || !packsOK || !algebraOK {
		t.Fatal("scoped loader schemas")
	}
	_, initial, _, _, initialOK := contract.InitialBinding("require")
	boot, bootOK := linked.Host().BootRoots().At(0)
	if !initialOK || !bootOK {
		t.Fatal("scoped loader bootstrap")
	}
	fact, factOK := values.TargetInitial(boot, initial)
	atoms, atomsOK := values.Atoms(fact)
	if !factOK || !atomsOK || len(atoms) != 1 {
		t.Fatal("scoped loader Value marker")
	}
	markerRef, _, markerOK := atoms[0].Reference()
	markerOperation, markerScoped := markerRef.ScopedLoader()
	if !markerOK || !markerScoped || markerOperation == 0 {
		t.Fatal("scoped loader marker")
	}
	calls := linked.Project().Applications().Calls()
	if calls.Count() < 2 {
		t.Fatalf("call applications=%d, want two shards", calls.Count())
	}
	selected := make([]calldomain.Target, 2)
	for index := 0; index < 2; index++ {
		application, applicationOK := calls.At(index)
		bound, boundOK := newSite(algebra, values, heaps, packs, application)
		shard, _, callOK := linked.Project().Applications().Call(application)
		seed, seedOK := linked.Boundary().Seeds().ScopedLoader(shard)
		expected, expectedOK := algebra.TargetForSeed(seed)
		result, resultOK := reduce(bound, fact)
		if !applicationOK || !boundOK || !callOK || !seedOK || !expectedOK || !resultOK || !result.IsComplete() || result.HasOpaqueAlternative() || result.KnownTargetCount() != 1 || !result.HasTarget(expected) {
			t.Fatalf("shard %d scoped dispatch complete=%t opaque=%t targets=%d", index, result.IsComplete(), result.HasOpaqueAlternative(), result.KnownTargetCount())
		}
		selected[index] = expected
	}
	if selected[0].Same(selected[1]) {
		t.Fatal("distinct application shards collapsed to one scoped loader target")
	}
	foreignHeaps, foreignHeapsOK := heapdomain.Seal(linked)
	foreignValues, foreignValuesOK := valuedomain.Seal(linked, foreignHeaps)
	foreignFact, foreignFactOK := foreignValues.TargetInitial(boot, initial)
	bound, boundOK := newSite(algebra, values, heaps, packs, mustCallApplication(t, calls, 0))
	if !foreignHeapsOK || !foreignValuesOK || !foreignFactOK || !boundOK {
		t.Fatal("foreign scoped loader setup")
	}
	if _, crossed := reduce(bound, foreignFact); crossed {
		t.Fatal("foreign Value scoped loader marker crossed dispatch owner fence")
	}
}

func mustCallApplication(t testing.TB, calls linkproject.Calls, index int) linkproject.Application {
	t.Helper()
	application, ok := calls.At(index)
	if !ok {
		t.Fatalf("call application %d", index)
	}
	return application
}

func TestDispatchDeniedCallableDoesNotBecomeOpaque(t *testing.T) {
	contract := callBoundarySeedContract(t)
	fixture := newDispatchFixture(t, contract, nil)
	var denied target.InitialValue
	for index := 0; index < contract.InitialEntryCount(); index++ {
		_, key, value, _, ok := contract.InitialEntryAt(index)
		literal, literalOK := contract.ExactKeyValue(key)
		kind, kindOK := contract.InitialValueKind(value)
		if ok && literalOK && literal.Kind == keyspace.LiteralString && literal.String == "load" && kindOK && kind == target.InitialValueDeniedOperation {
			denied = value
			break
		}
	}
	seed, disposition, ok := fixture.linked.Boundary().Seeds().BootstrapCallable(denied)
	if !ok || disposition != linkboundary.CallableDeniedTarget {
		t.Fatal("denied seed")
	}
	atom, ok := fixture.values.Callable(seed)
	if !ok {
		t.Fatal("denied callable atom")
	}
	fact, ok := fixture.values.Singleton(atom)
	if !ok {
		t.Fatal("denied callable fact")
	}
	result, ok := reduce(fixture.siteFor(t), fact)
	if !ok || !result.IsComplete() || result.HasOpaqueAlternative() || result.KnownTargetCount() != 0 {
		t.Fatal("denied callable became opaque or selected")
	}
}

func TestDispatchRejectsForeignValueSchema(t *testing.T) {
	left := newDispatchFixture(t, nil, nil)
	right := newDispatchFixture(t, nil, nil)
	if _, ok := reduce(left.siteFor(t), closureFact(t, right)); ok {
		t.Fatal("foreign Value schema crossed dispatch owner fence")
	}
}

func TestDispatchRuleDeclaresExactAuthoritiesAndRejectsForeignApplication(t *testing.T) {
	left := newDispatchFixture(t, nil, nil)
	right := newDispatchFixture(t, nil, nil)
	composition := engine.NewComposition()
	values, ok := valueowner.Declare(composition, dispatchSemantic(1), dispatchSemantic(2), left.values)
	if !ok {
		t.Fatal("Value owner")
	}
	calls, ok := callowner.Declare(composition, dispatchSemantic(3), left.algebra)
	if !ok {
		t.Fatal("Call owner")
	}
	rule, ok := Declare(composition, dispatchSemantic(4), dispatchSemantic(5), dispatchSemantic(6), values, left.heaps, left.packs, calls)
	if !ok || rule == nil {
		t.Fatal("dispatch Rule")
	}
	if _, ok := rule.Instance(right.app); ok {
		t.Fatal("foreign application crossed dispatch site owner fence")
	}
	foreignComposition := engine.NewComposition()
	foreignValues, ok := valueowner.Declare(foreignComposition, dispatchSemantic(11), dispatchSemantic(12), right.values)
	if !ok {
		t.Fatal("foreign Value owner")
	}
	foreignCalls, ok := callowner.Declare(foreignComposition, dispatchSemantic(13), right.algebra)
	if !ok {
		t.Fatal("foreign Call owner")
	}
	if _, ok := Declare(foreignComposition, dispatchSemantic(14), dispatchSemantic(15), dispatchSemantic(16), foreignValues, left.heaps, right.packs, foreignCalls); ok {
		t.Fatal("foreign Heap schema admitted")
	}
}

func TestDispatchRejectsSameLinkResealedHeapAndPackSchemas(t *testing.T) {
	fixture := newDispatchFixture(t, nil, nil)
	resealedHeaps, ok := heapdomain.Seal(fixture.linked)
	if !ok {
		t.Fatal("resealed Heap schema")
	}
	resealedValues, ok := valuedomain.Seal(fixture.linked, resealedHeaps)
	if !ok {
		t.Fatal("resealed Value schema")
	}
	types, ok := typeauthority.Seal(fixture.linked)
	if !ok {
		t.Fatal("resealed type authority")
	}
	statics, _, err := staticdomain.Seal(fixture.linked, types)
	if err != nil {
		t.Fatal(err)
	}
	resealedPacks, ok := packdomain.Seal(fixture.linked, statics)
	if !ok {
		t.Fatal("resealed Pack schema")
	}
	bound := fixture.siteFor(t)
	bound.heaps = resealedHeaps
	if bound.valid() {
		t.Fatal("same-Link resealed Heap schema crossed Value owner fence")
	}
	bound = fixture.siteFor(t)
	bound.packs = resealedPacks
	if bound.matchesSchemas(fixture.heaps, fixture.packs) {
		t.Fatal("same-Link resealed Pack schema crossed Rule owner fence")
	}
	composition := engine.NewComposition()
	valuesOwner, valuesOK := valueowner.Declare(composition, dispatchSemantic(21), dispatchSemantic(22), fixture.values)
	callsOwner, callsOK := callowner.Declare(composition, dispatchSemantic(23), fixture.algebra)
	if !valuesOK || !callsOK {
		t.Fatal("reseal declaration owners")
	}
	if _, declared := Declare(composition, dispatchSemantic(24), dispatchSemantic(25), dispatchSemantic(26), valuesOwner, resealedHeaps, fixture.packs, callsOwner); declared {
		t.Fatal("same-Link resealed Heap schema admitted dispatch Rule")
	}
	if resealedValues.Link() != fixture.values.Link() || resealedPacks.Link() != fixture.packs.Link() {
		t.Fatal("resealed schemas did not preserve Link identity")
	}
}

func dispatchSemantic(value byte) engine.SemanticKey {
	var digest [32]byte
	digest[len(digest)-1] = value
	key, ok := engine.NewSemanticKey(digest, 1)
	if !ok {
		panic("dispatch semantic key")
	}
	return key
}

func callBoundarySeedContract(t *testing.T) *target.Contract {
	t.Helper()
	op := target.BindingSpec{Namespace: target.BindingBuiltin, Member: []string{"op"}}
	require := target.BindingSpec{Namespace: target.BindingBuiltin, Member: []string{"require"}}
	denied := target.BindingSpec{Namespace: target.BindingBuiltin, Member: []string{"load"}}
	provider := target.BindingSpec{Namespace: target.BindingProvider, Owner: []string{"host"}, Member: []string{"dispatch"}}
	operation := func(binding target.BindingSpec) target.OperationSpec {
		return target.OperationSpec{Bindings: []target.BindingSpec{binding}, Input: target.ValuesSpec{Tail: target.ValuesClosed}, Outcomes: []target.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: target.ValuesSpec{Tail: target.ValuesClosed}}}, Effects: target.RowSpec{Tail: target.RowClosed}}
	}
	contract, err := target.Seal(&target.Spec{
		Operations:   []target.OperationSpec{operation(op), operation(require), operation(provider)},
		InitialRoots: []target.InitialRootSpec{{Identity: "GlobalEnvRoot", Shape: target.BootShapeSpec{Aggregate: target.BootAggregateTable, Value: target.InitialValueSpec{Kind: target.InitialValueRoot, Root: "GlobalEnvRoot"}}}},
		InitialEntries: []target.InitialEntrySpec{
			{Root: "GlobalEnvRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "_G"}, Value: target.InitialValueSpec{Kind: target.InitialValueRoot, Root: "GlobalEnvRoot"}, Mutability: target.InitialMutable},
			{Root: "GlobalEnvRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "__link_absent"}, Value: target.InitialValueSpec{Kind: target.InitialValueAbsent}, Mutability: target.InitialMutable},
			{Root: "GlobalEnvRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "load"}, Value: target.InitialValueSpec{Kind: target.InitialValueDeniedOperation, Operation: denied}, Mutability: target.InitialMutable},
		},
		InitialBindings: []target.InitialBindingSpec{
			{Name: "_G", Root: "GlobalEnvRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "_G"}},
			{Name: "__link_absent", Root: "GlobalEnvRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "__link_absent"}},
			{Name: "load", Root: "GlobalEnvRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "load"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return contract
}
