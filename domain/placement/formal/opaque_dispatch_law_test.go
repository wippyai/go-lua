package formal

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/operand"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/lua/lower"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	artifactcompiler "github.com/wippyai/go-lua/analysis/program/artifact/compiler"
	"github.com/wippyai/go-lua/analysis/program/link"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	targetcontract "github.com/wippyai/go-lua/analysis/program/target/contract"
	"github.com/wippyai/go-lua/analysis/schema/ingress"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	programmount "github.com/wippyai/go-lua/analysis/schema/programmount"
	calldomain "github.com/wippyai/go-lua/domain/call"
	"github.com/wippyai/go-lua/domain/call/calltest"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	"github.com/wippyai/go-lua/domain/materialization"
	packdomain "github.com/wippyai/go-lua/domain/pack"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
	staticdomain "github.com/wippyai/go-lua/domain/static"
	typeauthority "github.com/wippyai/go-lua/domain/type/authority"
	valuedomain "github.com/wippyai/go-lua/domain/value"
	"github.com/wippyai/go-lua/internal/testfixture"
)

type opaqueDispatchLawFixture struct {
	packs     *packdomain.Schema
	calls     *calldomain.Algebra
	placement placementdomain.Schema
	values    *valuedomain.Schema
	contract  *targetcontract.Contract
	mounted   calldomain.MountedCall
	key       calldomain.Key
	cells     []operand.MemberCell[valuedomain.Value]
}

var (
	opaqueDispatchPlanSink routePlan
	opaqueDispatchPlanOK   bool
)

// TestOpaqueCallDispatchHasNoFormalTargetAuthority proves that an
// owner-authenticated Open/Top Call value is a may-dispatch witness, not a
// free-standing formal Target witness. Formal rows are reduced only for
// explicitly known Target alternatives; an opaque-only value therefore keeps
// a valid no-route result, while known+opaque preserves the known route.
func TestOpaqueCallDispatchHasNoFormalTargetAuthority(t *testing.T) {
	fixture := newOpaqueDispatchLawFixture(t, "opaque-dispatch-local")
	keys := routePlanAllocationKeys(t, fixture.placement)
	if len(keys) < 2 {
		t.Fatal("opaque dispatch fixture needs at least two allocation roots")
	}
	atom, atomOK := fixture.values.Allocation(keys[0], materialization.Recent)
	fact, factOK := fixture.values.Singleton(atom)
	if !atomOK || !factOK {
		t.Fatalf("opaque dispatch reachable allocation fact = %t/%t", atomOK, factOK)
	}
	fixture.cells[0].Value = fact
	allocationCount := 0
	for dense := 0; dense < fixture.placement.DenseKeyCount(); dense++ {
		key, keyOK := fixture.placement.KeyAt(dense)
		if !keyOK {
			t.Fatalf("placement root %d", dense)
		}
		if key.Kind() == heapdomain.RootAllocation {
			allocationCount++
		}
	}
	if allocationCount == 0 {
		t.Fatal("opaque dispatch fixture has no allocation roots")
	}

	key := fixture.key
	open := mustOpenDispatchValue(t, fixture.calls, key)
	for _, test := range []struct {
		name string
		fact calldomain.Value
	}{
		{name: "open", fact: open},
		{name: "top", fact: fixture.calls.Top()},
	} {
		t.Run(test.name, func(t *testing.T) {
			plan, ok := planFor(fixture.packs, fixture.calls, fixture.placement, fixture.values, fixture.contract, fixture.mounted, test.fact, formalActuals(t, fixture.cells))
			if !ok {
				t.Fatal("authenticated opaque Call dispatch was refused")
			}
			if plan.routeCount() != 0 {
				t.Fatalf("opaque dispatch manufactured formal routes: route count = %d/all %d", plan.routeCount(), allocationCount)
			}
		})
	}

	knownTarget := findFormalTargetForOpaqueLaw(t, fixture)
	exact, exactOK := fixture.calls.DispatchValue(key, []calldomain.Target{knownTarget}, false)
	if !exactOK || exact.HasOpaqueAlternative() || exact.KnownTargetCount() != 1 {
		t.Fatal("known formal dispatch value")
	}
	knownAndOpaque, knownAndOpaqueOK := fixture.calls.DispatchValue(key, []calldomain.Target{knownTarget}, true)
	if !knownAndOpaqueOK || !knownAndOpaque.HasOpaqueAlternative() || knownAndOpaque.KnownTargetCount() != 1 {
		t.Fatal("known-plus-opaque formal dispatch value")
	}
	exactPlan, exactPlanOK := planFor(fixture.packs, fixture.calls, fixture.placement, fixture.values, fixture.contract, fixture.mounted, exact, formalActuals(t, fixture.cells))
	combinedPlan, combinedPlanOK := planFor(fixture.packs, fixture.calls, fixture.placement, fixture.values, fixture.contract, fixture.mounted, knownAndOpaque, formalActuals(t, fixture.cells))
	if !exactPlanOK || exactPlan.routeCount() == 0 || !combinedPlanOK || combinedPlan.routeCount() != exactPlan.routeCount() {
		t.Fatalf("known formal route changed by opaque arm: exact=%t/%d combined=%t/%d", exactPlanOK, exactPlan.routeCount(), combinedPlanOK, combinedPlan.routeCount())
	}
	for _, plan := range []routePlan{exactPlan, combinedPlan} {
		route, routeOK := plan.routeAt(0)
		if !routeOK || route.key != keys[0] {
			t.Fatalf("known formal route = %#v/%t, want reachable root %v", route, routeOK, keys[0])
		}
	}
	callFact := open
	// The vector is opened once, outside the probe: a member vector is a view
	// over cells the family already owns, and opening one per invocation would
	// measure the law's own scaffolding rather than the reduction.
	actuals := formalActuals(t, fixture.cells)
	allocations := testing.AllocsPerRun(100, func() {
		opaqueDispatchPlanSink, opaqueDispatchPlanOK = planFor(fixture.packs, fixture.calls, fixture.placement, fixture.values, fixture.contract, fixture.mounted, callFact, actuals)
	})
	if allocations != 0 {
		t.Fatalf("opaque no-route planner allocated %v times", allocations)
	}
	if !opaqueDispatchPlanOK || opaqueDispatchPlanSink.routeCount() != 0 {
		t.Fatalf("opaque allocation probe manufactured routes = %t/%d/all=%d", opaqueDispatchPlanOK, opaqueDispatchPlanSink.routeCount(), allocationCount)
	}
}

func findFormalTargetForOpaqueLaw(t testing.TB, fixture opaqueDispatchLawFixture) calldomain.Target {
	t.Helper()
	_, callID, module, _, _, identityOK := fixture.calls.MountedCallIdentity(fixture.mounted)
	actual, actualOK := fixture.packs.MountedActualProjection(module, callID)
	if !identityOK || !actualOK {
		t.Fatal("opaque dispatch mounted actual projection")
	}
	_, runtimeTail := actual.TailID()
	for index := 0; index < fixture.calls.SupportCount(fixture.key); index++ {
		target, targetOK := fixture.calls.SupportTargetAt(fixture.key, index)
		if !targetOK || !fixture.calls.OwnsTarget(target) {
			continue
		}
		operation, operationOK := target.Operation()
		if !operationOK {
			continue
		}
		var demands denseDemandScratch
		if !addFormalOperationDemand(fixture.placement, fixture.values, fixture.contract, operation, actual.ActualCount(), runtimeTail, formalActuals(t, fixture.cells), &demands) {
			continue
		}
		if demands.count == 0 && !demands.allUnknown {
			continue
		}
		return target
	}
	t.Fatal("opaque dispatch fixture has no known formal Target route")
	return calldomain.Target{}
}

// TestOpaqueCallDispatchRejectsForeignAndMalformedFacts keeps opaque Call
// uncertainty from becoming a compensation path. Call ownership, Value
// ownership, and fixed observation presence must still be authenticated even
// when the resulting opaque-only plan is a valid no-route result.
func TestOpaqueCallDispatchRejectsForeignAndMalformedFacts(t *testing.T) {
	local := newOpaqueDispatchLawFixture(t, "opaque-dispatch-local")
	foreign := newOpaqueDispatchLawFixture(t, "opaque-dispatch-foreign")
	open := mustOpenDispatchValue(t, local.calls, local.key)

	tests := []struct {
		name  string
		calls *calldomain.Algebra
		fact  calldomain.Value
		cells []operand.MemberCell[valuedomain.Value]
	}{
		{name: "foreign-call-top", calls: local.calls, fact: foreign.calls.Top(), cells: local.cells},
		{name: "zero-call-fact", calls: local.calls, fact: calldomain.Value{}, cells: local.cells},
		{name: "missing-observation", calls: local.calls, fact: open, cells: append([]operand.MemberCell[valuedomain.Value](nil), local.cells...)},
		{name: "foreign-value-observation", calls: local.calls, fact: open, cells: append([]operand.MemberCell[valuedomain.Value](nil), local.cells...)},
	}
	tests[2].cells = tests[2].cells[:0]
	if len(tests[3].cells) == 0 {
		t.Fatal("opaque dispatch fixture has no fixed actual for malformed observation law")
	}
	for index := range tests[3].cells {
		tests[3].cells[index].Value = foreign.values.Bottom()
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, ok := planFor(local.packs, local.calls, local.placement, local.values, local.contract, local.mounted, test.fact, formalActuals(t, test.cells)); ok {
				t.Fatal("foreign or malformed opaque predecessor was widened instead of refused")
			}
		})
	}
}

func mustOpenDispatchValue(t testing.TB, calls *calldomain.Algebra, key calldomain.Key) calldomain.Value {
	t.Helper()
	fact, ok := calls.DispatchValue(key, nil, true)
	if !ok || !fact.HasOpaqueAlternative() {
		t.Fatal("open Call dispatch value")
	}
	return fact
}

func newOpaqueDispatchLawFixture(t testing.TB, name string) opaqueDispatchLawFixture {
	t.Helper()
	program, err := lower.Lower(lower.Source{Name: name + ".lua", Text: []byte("local hidden = {}; return require({})")})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := testfixture.StandardLibraryTarget()
	if err != nil || contract == nil {
		t.Fatalf("seal opaque dispatch Target: %v", err)
	}
	linked, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: name, Program: program}}})
	if err != nil || linked == nil {
		t.Fatalf("seal opaque dispatch Link: %v", err)
	}
	grammar, grammarOK := programartifact.NewExecutionSchemaID(identity.ContentID{1}, identity.ContentID{2}, programartifact.GrammarABIVersion)
	if !grammarOK {
		t.Fatal("opaque dispatch artifact grammar")
	}
	mounts := linked.Project().Mounts()
	shard, shardOK := mounts.At(0)
	published, programOK := mounts.Program(shard)
	module, moduleOK := linked.Project().ModuleKey(shard)
	_, programIDOK := mounts.ProgramID(shard)
	if !shardOK || !programOK || published == nil || !moduleOK || !programIDOK {
		t.Fatal("opaque dispatch mount")
	}
	artifact, failure := artifactcompiler.CompileDetailed(published, grammar, testfixture.EmptyProgramIssuancePlan(t))
	if failure.Available() || artifact == nil || !artifact.Available() {
		t.Fatalf("compile opaque dispatch artifact: %s", failure.Error())
	}
	structural := formalSoundnessStructuralVocabulary(t)
	snapshot, lowered := ingress.Lower(artifact, structural)
	if !lowered {
		t.Fatal("opaque dispatch ingress")
	}
	mountedProgram := programmount.Program{ModuleKey: module, Program: artifact.Program()}
	heapMount, heapMountOK := programmount.MountedArtifactFromSnapshot(snapshot, module)
	valueMount, valueMountOK := programmount.MountedArtifactFromSnapshot(snapshot, module)
	packMount, packMountOK := programmount.MountedArtifactFromSnapshot(snapshot, module)
	heapSchema, heapFailure := heapdomain.SealWithArtifacts(linked, []programmount.MountedArtifact{heapMount})
	placementSchema, placementOK := placementdomain.NewSchema(heapSchema)
	values, valueFailure := valuedomain.SealWithFailure(linked, heapSchema, calltest.MustSeal(t, linked, []programmount.MountedArtifact{valueMount}), []programmount.MountedArtifact{valueMount}, structural)
	if !heapMountOK || !valueMountOK || !packMountOK || heapFailure != heapdomain.SealFailureNone || !placementOK || valueFailure != valuedomain.SealFailureNone || values == nil {
		t.Fatalf("opaque dispatch Heap/Value schemas heapMount=%t valueMount=%t packMount=%t heap=%v placement=%t value=%v", heapMountOK, valueMountOK, packMountOK, heapFailure, placementOK, valueFailure)
	}
	programRows, programRowsOK := artifact.Program().CallCount()
	if !programRowsOK || programRows == 0 {
		t.Fatal("opaque dispatch artifact has no Call rows")
	}
	types, typesErr := typeauthority.SealProgramRows(linked.ContentID(), []programschema.Program{artifact.Program()}, nil)
	if typesErr != nil || types == nil {
		t.Fatalf("opaque dispatch Type authority: %v", typesErr)
	}
	if !mountedProgram.Available() {
		t.Fatal("opaque dispatch program mount")
	}
	statics, _, staticErr := staticdomain.SealMountedPrograms(staticdomain.MountContext{LinkID: linked.ContentID(), Target: contract}, types, []staticdomain.MountedProgram{{Program: mountedProgram.Program, ModuleID: module, NamespaceID: module}})
	if staticErr != nil || statics == nil {
		t.Fatalf("opaque dispatch Static authority: %v", staticErr)
	}
	packs, packsOK := packdomain.SealMountedArtifacts(linked, statics, []programmount.MountedArtifact{packMount})
	calls, callsOK := calldomain.NewWithMountedArtifacts(linked, []calldomain.MountedArtifact{{Program: mountedProgram, Snapshot: snapshot}})
	if !packsOK || packs == nil || !callsOK || calls == nil {
		t.Fatalf("opaque dispatch Pack/Call authorities packs=%t/%v calls=%t/%v", packsOK, packs, callsOK, calls)
	}
	var mounted calldomain.MountedCall
	var key calldomain.Key
	var cells []operand.MemberCell[valuedomain.Value]
	found := false
	for index := 0; index < calls.MountedCallCount(); index++ {
		candidate, candidateOK := calls.MountedCallAtHandle(index)
		_, callID, callModule, _, _, identityOK := calls.MountedCallIdentity(candidate)
		actual, actualOK := packs.MountedActualProjection(callModule, callID)
		candidateKey, keyOK := calls.KeyForMountedCall(candidate)
		if !candidateOK || !identityOK || !actualOK || !keyOK || actual.ActualCount() == 0 {
			continue
		}
		mounted, key = candidate, candidateKey
		cells = make([]operand.MemberCell[valuedomain.Value], actual.ActualCount())
		for index := range cells {
			cells[index] = operand.MemberCell[valuedomain.Value]{Value: values.Bottom(), Present: true}
		}
		found = true
		break
	}
	if !found {
		t.Fatal("opaque dispatch fixture has no mounted call with a fixed actual")
	}
	return opaqueDispatchLawFixture{packs: packs, calls: calls, placement: placementSchema, values: values, contract: contract, mounted: mounted, key: key, cells: cells}
}

// formalActuals views a fixture's own member cells as the whole vector the
// declaration delivers. A vector is a view over caller-owned cells, so a law
// that varies one cell copies the slice and views the copy.
func formalActuals(t testing.TB, cells []operand.MemberCell[valuedomain.Value]) operand.SummaryVector[valuedomain.Value] {
	t.Helper()
	vector, ok := operand.NewMemberVector(cells)
	if !ok {
		t.Fatal("mounted actual member vector")
	}
	return vector
}
