package transfer

import (
	"fmt"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/lua/lower"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	artifactcompiler "github.com/wippyai/go-lua/analysis/program/artifact/compiler"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/link"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	targetcompiler "github.com/wippyai/go-lua/analysis/program/target/compiler"
	"github.com/wippyai/go-lua/analysis/program/target/contract"
	"github.com/wippyai/go-lua/analysis/program/target/declaration"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/ingress"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	programmount "github.com/wippyai/go-lua/analysis/schema/programmount"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	schematype "github.com/wippyai/go-lua/analysis/schema/typecontract"
	calldomain "github.com/wippyai/go-lua/domain/call"
	callowner "github.com/wippyai/go-lua/domain/call/owner"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	"github.com/wippyai/go-lua/domain/materialization"
	packdomain "github.com/wippyai/go-lua/domain/pack"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
	placementowner "github.com/wippyai/go-lua/domain/placement/owner"
	"github.com/wippyai/go-lua/domain/runtimekind"
	staticdomain "github.com/wippyai/go-lua/domain/static"
	typeauthority "github.com/wippyai/go-lua/domain/type/authority"
	"github.com/wippyai/go-lua/domain/type/typecontract"
	valuedomain "github.com/wippyai/go-lua/domain/value"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
	"github.com/wippyai/go-lua/internal/testfixture"
)

// transferHotLawFixture is deliberately the smallest real owner join that
// reaches planFor: Link owns the Target Contract, Call owns the mounted
// application/candidate, Pack owns actual order and selectors, and Value/
// Placement own the exact allocation fact and root coordinate.
type transferHotLawFixture struct {
	packs        *packdomain.Schema
	calls        *calldomain.Algebra
	placement    placementdomain.Schema
	values       *valuedomain.Schema
	contract     *contract.Contract
	mounted      calldomain.MountedCall
	key          calldomain.Key
	callFact     calldomain.Value
	openFact     calldomain.Value
	payloadID    identity.ContentID
	payloadRoot  heapdomain.Key
	observations []actualObservation
}

// TestTransferMayDeliverDisplacesTheExactPayloadRootWithoutPublicationEffect
// is the strongest direct owner-bound hot proof available in this package. The
// Target operation has a TransferSpec and a closed empty invocation Effect
// row; the exact Call candidate and Pack actuals still drive one payload root
// to a Send route, whose Placement displacement is SharedHeap.
//
// This law intentionally stops at the owner-bound planner. The exact
// composite materializer E2E remains blocked by the shared artifact
// occurrence-mount seam, so this test does not claim a mounted solve/result
// publication.
func TestTransferMayDeliverDisplacesTheExactPayloadRootWithoutPublicationEffect(t *testing.T) {
	fixture := newTransferHotLawFixture(t, true, "transfer-deliver-hot")
	operation, operationOK := fixture.contract.Operations.Lookup(vocabulary.BindingSpec{
		Namespace: vocabulary.BindingBuiltin, Member: []string{"send"},
	})
	if !operationOK || fixture.contract.Operations.TransferCount(operation) != 1 || fixture.contract.Operations.EffectCount(operation) != 0 {
		t.Fatalf("transfer Target operation = %d/%t, transfers=%d effects=%d; want one TransferSpec and no PublicationEffect/invocation effect", operation, operationOK, fixture.contract.Operations.TransferCount(operation), fixture.contract.Operations.EffectCount(operation))
	}
	plan, planOK := planFor(fixture.packs, fixture.calls, fixture.placement, fixture.values, fixture.contract, fixture.mounted, fixture.callFact, fixture.observations)
	if !planOK || plan.routeCount() != 1 {
		t.Fatalf("MayDeliver transfer plan = %t/%d, want one exact route", planOK, plan.routeCount())
	}
	route, routeOK := plan.routeAt(0)
	if !routeOK || route.key != fixture.payloadRoot {
		t.Fatalf("MayDeliver route = %#v/%t, want exact payload root %v", route, routeOK, fixture.payloadRoot)
	}
	if displaced, displacementOK := placementdomain.DisplaceChecked(placementdomain.OwnedHeap, placementdomain.Send); !displacementOK || displaced != placementdomain.SharedHeap {
		t.Fatalf("Send displacement = %s/%t, want SharedHeap/true", displaced, displacementOK)
	}
}

// TestTransferRejectOnlyHasNoPlacementRouteAndBadFactsRefuse proves that a
// reject-only relation is not a fallback route, while absent and foreign
// predecessor evidence cannot be compensated by the Target transfer row.
func TestTransferRejectOnlyHasNoPlacementRouteAndBadFactsRefuse(t *testing.T) {
	fixture := newTransferHotLawFixture(t, false, "transfer-reject-hot")
	plan, planOK := planFor(fixture.packs, fixture.calls, fixture.placement, fixture.values, fixture.contract, fixture.mounted, fixture.callFact, fixture.observations)
	if !planOK || plan.routeCount() != 0 {
		t.Fatalf("reject-only transfer plan = %t/%d, want valid no-route", planOK, plan.routeCount())
	}
	missing := append([]actualObservation(nil), fixture.observations...)
	if len(missing) == 0 {
		t.Fatal("reject fixture has no fixed actual for missing-evidence law")
	}
	missing[0] = actualObservation{}
	if _, missingOK := planFor(fixture.packs, fixture.calls, fixture.placement, fixture.values, fixture.contract, fixture.mounted, fixture.callFact, missing); missingOK {
		t.Fatal("missing Value fact was compensated into a reject-only plan")
	}
	foreign := newTransferHotLawFixture(t, true, "transfer-foreign-hot")
	if _, foreignOK := planFor(fixture.packs, fixture.calls, fixture.placement, fixture.values, foreign.contract, fixture.mounted, fixture.callFact, fixture.observations); foreignOK {
		t.Fatal("foreign Target Contract crossed the Call owner fence")
	}
	foreignValue := append([]actualObservation(nil), fixture.observations...)
	foreignValue[0].fact = foreign.values.Bottom()
	if _, foreignValueOK := planFor(fixture.packs, fixture.calls, fixture.placement, fixture.values, fixture.contract, fixture.mounted, fixture.callFact, foreignValue); foreignValueOK {
		t.Fatal("foreign Value fact crossed the coordinate owner fence")
	}
}

// TestTransferOpaqueDispatchHasNoDeliveryAuthority proves that Call's
// unenumerated alternative cannot manufacture Placement roots.  When an
// authenticated Target remains alongside that alternative, the exact
// declared payload route remains the complete result.
func TestTransferOpaqueDispatchHasNoDeliveryAuthority(t *testing.T) {
	fixture := newTransferHotLawFixture(t, true, "transfer-opaque-no-authority")
	exact, exactOK := planFor(fixture.packs, fixture.calls, fixture.placement, fixture.values, fixture.contract, fixture.mounted, fixture.callFact, fixture.observations)
	if !exactOK || exact.routeCount() != 1 {
		t.Fatalf("exact transfer plan = %t/%d, want one route", exactOK, exact.routeCount())
	}
	opaque, opaqueOK := planFor(fixture.packs, fixture.calls, fixture.placement, fixture.values, fixture.contract, fixture.mounted, fixture.openFact, fixture.observations)
	if !opaqueOK || opaque.routeCount() != 0 {
		t.Fatalf("opaque-only transfer plan = %t/%d, want valid no-route", opaqueOK, opaque.routeCount())
	}
	target, targetOK := fixture.callFact.KnownTargetAt(0)
	if !targetOK {
		t.Fatal("exact fixture has no authenticated target")
	}
	knownAndOpaque, dispatchOK := fixture.calls.DispatchValue(fixture.key, []calldomain.Target{target}, true)
	if !dispatchOK || !knownAndOpaque.HasOpaqueAlternative() || knownAndOpaque.KnownTargetCount() != 1 {
		t.Fatalf("known-plus-opaque dispatch = %t/%t/%d", dispatchOK, knownAndOpaque.HasOpaqueAlternative(), knownAndOpaque.KnownTargetCount())
	}
	combined, combinedOK := planFor(fixture.packs, fixture.calls, fixture.placement, fixture.values, fixture.contract, fixture.mounted, knownAndOpaque, fixture.observations)
	if !combinedOK || combined.routeCount() != exact.routeCount() {
		t.Fatalf("known-plus-opaque transfer plan = %t/%d, want exact %d", combinedOK, combined.routeCount(), exact.routeCount())
	}
	got, gotOK := combined.routeAt(0)
	want, wantOK := exact.routeAt(0)
	if !gotOK || !wantOK || got.key != want.key || got.tag != want.tag {
		t.Fatalf("known-plus-opaque route = %#v/%t, want exact %#v/%t", got, gotOK, want, wantOK)
	}
}

// TestTransferAliasIsDescriptiveMetadataOnly pins the Target vocabulary's
// Alias meaning. Even when Alias names a distinct valid input graph, this
// consumer's Placement demand remains payload-derived; the alias does not
// become a second move/copy or actor/context authority.
func TestTransferAliasIsDescriptiveMetadataOnly(t *testing.T) {
	fixture := newTransferHotLawFixtureWithAlias(t, true, "transfer-alias-descriptive-hot", 0)
	plan, planOK := planFor(fixture.packs, fixture.calls, fixture.placement, fixture.values, fixture.contract, fixture.mounted, fixture.callFact, fixture.observations)
	if !planOK || plan.routeCount() != 1 {
		t.Fatalf("descriptive-alias plan = %t/%d, want one payload route", planOK, plan.routeCount())
	}
	route, routeOK := plan.routeAt(0)
	if !routeOK || route.key != fixture.payloadRoot {
		t.Fatalf("descriptive-alias route = %#v/%t, want payload root %v only", route, routeOK, fixture.payloadRoot)
	}
}

// TestTransferStaticMetadataDoesNotInventRuntimeDestination proves that the
// static Target description only admits the payload's Send route. Endpoint
// spelling, source identity, and capability labels do not create a destination
// route or select a runtime copy/move strategy.
func TestTransferStaticMetadataDoesNotInventRuntimeDestination(t *testing.T) {
	variants := []struct {
		name         string
		endpoint     vocabulary.TransferEndpoint
		identity     vocabulary.TransferIdentity
		capabilities vocabulary.TransferCapabilities
	}{
		{
			name:         "external-unspecified",
			endpoint:     vocabulary.TransferEndpoint{Kind: vocabulary.TransferEndpointExternal},
			identity:     vocabulary.TransferIdentityUnspecified,
			capabilities: vocabulary.TransferCapabilitiesUnspecified,
		},
		{
			name:         "input-same-preserved",
			endpoint:     vocabulary.TransferEndpoint{Kind: vocabulary.TransferEndpointInput, Input: 0},
			identity:     vocabulary.TransferIdentitySame,
			capabilities: vocabulary.TransferCapabilitiesPreserveAll,
		},
		{
			name:         "input-distinct-lost",
			endpoint:     vocabulary.TransferEndpoint{Kind: vocabulary.TransferEndpointInput, Input: 0},
			identity:     vocabulary.TransferIdentityDistinct,
			capabilities: vocabulary.TransferCapabilitiesLoseAll,
		},
	}
	for _, variant := range variants {
		t.Run(variant.name, func(t *testing.T) {
			target := transferHotLawContractWithMetadata(t, true, 1, variant.endpoint, variant.identity, variant.capabilities)
			fixture := newTransferHotLawFixtureWithTarget(t, target, "transfer-static-metadata-"+variant.name)
			plan, planOK := planFor(fixture.packs, fixture.calls, fixture.placement, fixture.values, fixture.contract, fixture.mounted, fixture.callFact, fixture.observations)
			if !planOK || plan.routeCount() != 1 {
				t.Fatalf("static metadata plan = %t/%d, want one payload route", planOK, plan.routeCount())
			}
			route, routeOK := plan.routeAt(0)
			if !routeOK || route.key != fixture.payloadRoot {
				t.Fatalf("static metadata route = %#v/%t, want exact payload Send route", route, routeOK)
			}
		})
	}
}

// TestTransferPlannerRoutesOnlyAuthenticatedPayloadAllocation proves that a
// valid route tag round-trips only for the exact Heap allocation selected by
// Value. A tag for another owner-issued allocation must not be compensated into
// a payload route.
func TestTransferPlannerRoutesOnlyAuthenticatedPayloadAllocation(t *testing.T) {
	fixture := newTransferHotLawFixture(t, true, "transfer-exact-allocation-route")
	plan, planOK := planFor(fixture.packs, fixture.calls, fixture.placement, fixture.values, fixture.contract, fixture.mounted, fixture.callFact, fixture.observations)
	if !planOK || plan.routeCount() != 1 {
		t.Fatalf("exact allocation plan = %t/%d, want one route", planOK, plan.routeCount())
	}
	route, routeOK := plan.routeAt(0)
	if !routeOK || route.key != fixture.payloadRoot {
		t.Fatalf("exact allocation route = %#v/%t, want payload root", route, routeOK)
	}
	roundTrip, roundTripOK := routeForTag(plan, route.tag)
	if !roundTripOK || roundTrip.key != fixture.payloadRoot {
		t.Fatalf("exact allocation tag round-trip = %#v/%t", roundTrip, roundTripOK)
	}
	foreignFound := false
	for dense := 0; dense < fixture.placement.DenseKeyCount(); dense++ {
		candidate, candidateOK := fixture.placement.KeyAt(dense)
		if !candidateOK || candidate == fixture.payloadRoot {
			continue
		}
		foreignTag, foreignTagOK := routeTagFor(plan.schema, candidate)
		if !foreignTagOK {
			continue
		}
		if _, admitted := routeForTag(plan, foreignTag); admitted {
			t.Fatalf("foreign allocation %v crossed exact route fence", candidate)
		}
		foreignFound = true
		break
	}
	if !foreignFound {
		t.Fatal("fixture did not expose a second allocation for exact-route law")
	}
}

// TestTransferHotBindRequiresTheExactLinkAuthorityJoin proves the runtime
// binder itself is fenced to the same Placement/Value/Call owners and exact
// Target Contract/Pack Schema that planFor consumes.
func TestTransferHotBindRequiresTheExactLinkAuthorityJoin(t *testing.T) {
	fixture := newTransferHotLawFixture(t, true, "transfer-hot-bind")
	builder := engine.NewSchema()
	callFragment, callOK := callowner.DeclareSchema(builder, transferRegistrationSemantic(21))
	valueFragment, valueOK := valueowner.DeclareSchema(builder, transferRegistrationSemantic(22), transferRegistrationSemantic(23), transferRegistrationSemantic(24))
	placementFragment, placementOK := placementowner.DeclareSchema(builder, transferRegistrationSemantic(25), transferRegistrationSemantic(26))
	transferFragment, transferOK := DeclareSchema(builder, transferRegistrationSemantic(27), transferRegistrationSemantic(28), valueFragment, callFragment, placementFragment)
	cold, coldOK := builder.Seal()
	if !callOK || !valueOK || !placementOK || !transferOK || !coldOK || cold == nil {
		t.Fatalf("transfer hot declaration call=%t value=%t placement=%t transfer=%t cold=%t", callOK, valueOK, placementOK, transferOK, coldOK)
	}
	binding := engine.NewSchemaBinding(cold)
	callHot, callHotOK := callowner.BindHot(binding, callFragment, fixture.calls)
	valueHot, valueHotOK := valueowner.BindHot(binding, valueFragment, fixture.values)
	placementHot, placementHotOK := placementowner.BindHot(binding, placementFragment, fixture.placement)
	if !callHotOK || !valueHotOK || !placementHotOK || callHot == nil || valueHot == nil || placementHot == nil {
		t.Fatalf("transfer hot owner bind call=%t value=%t placement=%t", callHotOK, valueHotOK, placementHotOK)
	}
	hot, hotOK := BindHot(binding, transferFragment, placementHot, valueHot, callHot, fixture.contract, fixture.packs)
	if !hotOK || hot == nil {
		t.Fatal("transfer hot bind rejected the exact Link authority join")
	}
	if _, registered := engine.RegisterMountedSlot(binding, transferFragment.RuleSlot()); !registered {
		t.Fatal("transfer hot rule slot registration")
	}
	if !binding.Seal() {
		t.Fatal("transfer hot binding seal")
	}
	if implementation, implementationOK := hot.Implementation(); !implementationOK || implementation == nil {
		t.Fatal("transfer hot bind did not publish an owner-fenced implementation")
	}
	foreign := newTransferHotLawFixture(t, true, "transfer-hot-bind-foreign")
	if _, foreignOK := BindHot(binding, transferFragment, placementHot, valueHot, callHot, foreign.contract, fixture.packs); foreignOK {
		t.Fatal("transfer hot bind accepted a foreign Target Contract")
	}
}

var (
	transferBenchmarkPlan routePlan
	transferBenchmarkOK   bool
)

// TestTransferPlannerExactAndOpaqueAreZeroAllocation makes the hot-path budget
// executable rather than leaving it only in benchmark output.  The planner is
// invocation-local: all scratch state is stack-resident for the bounded exact
// fixture, while an opaque-only dispatch remains a valid no-route result.
func TestTransferPlannerExactAndOpaqueAreZeroAllocation(t *testing.T) {
	fixture := newTransferHotLawFixture(t, true, "transfer-planner-alloc-law")
	for _, test := range []struct {
		name      string
		fact      calldomain.Value
		wantRoute bool
	}{
		{name: "exact", fact: fixture.callFact, wantRoute: true},
		{name: "opaque-only", fact: fixture.openFact},
	} {
		t.Run(test.name, func(t *testing.T) {
			allocs := testing.AllocsPerRun(100, func() {
				plan, ok := planFor(fixture.packs, fixture.calls, fixture.placement, fixture.values, fixture.contract, fixture.mounted, test.fact, fixture.observations)
				if !ok || test.wantRoute && plan.routeCount() == 0 || !test.wantRoute && plan.routeCount() != 0 {
					t.Fatalf("transfer planner result = %t/%d", ok, plan.routeCount())
				}
				transferBenchmarkPlan, transferBenchmarkOK = plan, ok
			})
			if allocs != 0 {
				t.Fatalf("transfer planner allocations = %f, want 0", allocs)
			}
		})
	}
}

// BenchmarkTransferPlannerExactAndOpaqueBounds the invocation-local planner at
// both ordinary exact dispatch and opaque-only dispatch. The real
// Link/Call/Pack/Value/Placement fixture is sealed before timing.
func BenchmarkTransferPlannerExactAndOpaque(b *testing.B) {
	fixture := newTransferHotLawFixture(b, true, "transfer-planner-benchmark")
	for _, test := range []struct {
		name string
		fact calldomain.Value
	}{
		{name: "exact", fact: fixture.callFact},
		{name: "opaque-only", fact: fixture.openFact},
	} {
		test := test
		b.Run(test.name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for index := 0; index < b.N; index++ {
				transferBenchmarkPlan, transferBenchmarkOK = planFor(fixture.packs, fixture.calls, fixture.placement, fixture.values, fixture.contract, fixture.mounted, test.fact, fixture.observations)
			}
			if !transferBenchmarkOK {
				b.Fatalf("transfer planner result = %t/%d", transferBenchmarkOK, transferBenchmarkPlan.routeCount())
			}
		})
	}
}

func transferAnyType(t testing.TB) schematype.Type {
	t.Helper()
	value, ok := schematype.NewPrimitive(schematype.PrimitiveAny)
	if !ok {
		t.Fatal("portable any type")
	}
	return value
}

func transferHotLawContract(t testing.TB, mayDeliver bool) *contract.Contract {
	return transferHotLawContractWithAlias(t, mayDeliver, 1)
}

func transferHotLawContractWithAlias(t testing.TB, mayDeliver bool, aliasOrdinal vocabulary.ValueFormal) *contract.Contract {
	return transferHotLawContractWithMetadata(t, mayDeliver, aliasOrdinal,
		vocabulary.TransferEndpoint{Kind: vocabulary.TransferEndpointExternal},
		vocabulary.TransferIdentityUnspecified, vocabulary.TransferCapabilitiesUnspecified)
}

func transferHotLawContractWithMetadata(t testing.TB, mayDeliver bool, aliasOrdinal vocabulary.ValueFormal, endpoint vocabulary.TransferEndpoint, identityValue vocabulary.TransferIdentity, capabilities vocabulary.TransferCapabilities) *contract.Contract {
	t.Helper()
	deliver := vocabulary.TransferMayReject
	if mayDeliver {
		deliver = vocabulary.TransferMayDeliver
	}
	sealed, err := targetcompiler.Seal(&declaration.Spec{
		Semantics: typecontract.NewSemantics(),
		Operations: []vocabulary.OperationSpec{
			{
				Bindings: []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{"require"}}},
				Input:    vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed},
				Outcomes: []vocabulary.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}}},
				Effects:  vocabulary.RowSpec{Tail: vocabulary.RowClosed},
			},
			{
				Bindings: []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{"send"}}},
				Input:    vocabulary.ValuesSpec{Fixed: []schematype.Type{transferAnyType(t), transferAnyType(t)}, Tail: vocabulary.ValuesClosed},
				Outcomes: []vocabulary.OutcomeSpec{
					{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}},
					{Kind: flowkind.OutcomeThrow, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}},
				},
				Transfers: []vocabulary.TransferSpec{{
					Endpoint:     endpoint,
					Payload:      vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal, Ordinal: 1},
					Alias:        vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal, Ordinal: uint32(aliasOrdinal)},
					Identity:     identityValue,
					Capabilities: capabilities,
					Outcomes: []vocabulary.TransferOutcomeSpec{
						{Outcome: 0, Possibility: deliver},
						{Outcome: 1, Possibility: vocabulary.TransferMayReject},
					},
				}},
				Effects: vocabulary.RowSpec{Tail: vocabulary.RowClosed},
			},
		},
	})
	if err != nil || sealed == nil {
		t.Fatalf("seal transfer Target: %v", err)
	}
	return sealed
}

func newTransferHotLawFixture(t testing.TB, mayDeliver bool, name string) transferHotLawFixture {
	return newTransferHotLawFixtureWithAlias(t, mayDeliver, name, 1)
}

func newTransferHotLawFixtureWithAlias(t testing.TB, mayDeliver bool, name string, aliasOrdinal vocabulary.ValueFormal) transferHotLawFixture {
	return newTransferHotLawFixtureWithTarget(t, transferHotLawContractWithAlias(t, mayDeliver, aliasOrdinal), name)
}

func newTransferHotLawFixtureWithTarget(t testing.TB, target *contract.Contract, name string) transferHotLawFixture {
	t.Helper()
	program, err := lower.Lower(lower.Source{Name: name + ".lua", Text: []byte(`local receiver = {}
local payload = {value = 1}
receiver:send(payload)
return payload`)})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{Target: target, Modules: []linkproject.Module{{Name: name, Program: program}}})
	if err != nil || linked == nil {
		t.Fatalf("seal transfer Link: %v", err)
	}
	grammar, grammarOK := programartifact.NewExecutionSchemaID(identity.ContentID{1}, identity.ContentID{2}, programartifact.GrammarABIVersion)
	issuance := testfixture.EmptyProgramIssuancePlan(t)
	mounts := linked.Project().Mounts()
	shard, shardOK := mounts.At(0)
	published, programOK := mounts.Program(shard)
	module, moduleOK := linked.Project().ModuleKey(shard)
	_, programIDOK := mounts.ProgramID(shard)
	artifact, failure := artifactcompiler.CompileDetailed(published, grammar, issuance)
	structural := transferStructuralVocabulary(t)
	snapshot, lowered := ingress.Lower(artifact, structural)
	if !grammarOK || !grammar.Available() || !shardOK || !programOK || published == nil || !moduleOK || !programIDOK || failure.Available() || artifact == nil || !lowered {
		t.Fatalf("transfer fixture grammar=%t shard=%t program=%t module=%t programID=%t failure=%v artifact=%v ingress=%t", grammarOK, shardOK, programOK, moduleOK, programIDOK, failure, artifact, lowered)
	}
	mountedProgram := programmount.Program{ModuleKey: module, Program: artifact.Program()}
	heapMount, heapMountOK := programmount.MountedArtifactFromSnapshot(snapshot, module)
	valueMount, valueMountOK := programmount.MountedArtifactFromSnapshot(snapshot, module)
	packMount, packMountOK := programmount.MountedArtifactFromSnapshot(snapshot, module)
	heapSchema, heapFailure := heapdomain.SealWithArtifacts(linked, []programmount.MountedArtifact{heapMount})
	placementSchema, placementOK := placementdomain.NewSchema(heapSchema)
	values, valueFailure := valuedomain.SealWithFailure(linked, heapSchema, []programmount.MountedArtifact{valueMount}, structural)
	if !heapMountOK || !valueMountOK || !packMountOK || heapFailure != heapdomain.SealFailureNone || !placementOK || valueFailure != valuedomain.SealFailureNone || values == nil {
		t.Fatalf("transfer fixture Heap/Value schemas heapMount=%t valueMount=%t packMount=%t heap=%v placement=%t value=%v", heapMountOK, valueMountOK, packMountOK, heapFailure, placementOK, valueFailure)
	}
	types, typesErr := typeauthority.SealProgramRows(linked.ContentID(), []programschema.Program{artifact.Program()})
	if typesErr != nil || types == nil {
		t.Fatalf("transfer fixture Type authority: %v", typesErr)
	}
	statics, _, staticErr := staticdomain.SealMountedPrograms(staticdomain.MountContext{LinkID: linked.ContentID(), Target: target}, types, []staticdomain.MountedProgram{{Program: mountedProgram.Program, ModuleID: module, NamespaceID: module}})
	if staticErr != nil || statics == nil {
		t.Fatalf("transfer fixture Static authority: %v", staticErr)
	}
	packs, packsOK := packdomain.SealMountedArtifacts(linked, statics, []programmount.MountedArtifact{packMount})
	calls, callsOK := calldomain.NewWithMountedArtifacts(linked, []calldomain.MountedArtifact{{Program: mountedProgram, Snapshot: snapshot}})
	if !packsOK || packs == nil || !callsOK || calls == nil {
		t.Fatalf("transfer fixture Pack/Call authorities packs=%t/%v calls=%t/%v", packsOK, packs, callsOK, calls)
	}
	operation, operationOK := target.Operations.Lookup(vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"send"}})
	if !operationOK || target.Operations.TransferCount(operation) != 1 || target.Operations.EffectCount(operation) != 0 {
		t.Fatalf("transfer fixture Target operation = %d/%t transfers=%d effects=%d", operation, operationOK, target.Operations.TransferCount(operation), target.Operations.EffectCount(operation))
	}
	var mounted calldomain.MountedCall
	var key calldomain.Key
	var callFact calldomain.Value
	var callFactOK bool
	var payloadID identity.ContentID
	var observations []actualObservation
	var payloadRoot heapdomain.Key
	found := false
	callCount, callCountOK := artifact.Program().CallCount()
	if !callCountOK {
		t.Fatal("transfer fixture has no Call rows")
	}
	for callIndex := 0; callIndex < callCount; callIndex++ {
		call, callOK := artifact.Program().CallAt(callIndex)
		if !callOK || call.Form() != programschema.CallFormMethod {
			continue
		}
		argument, argumentOK := artifact.Program().CallArgumentFor(callIndex, 0)
		if !argumentOK {
			continue
		}
		payloadID = argument.ValueID()
		candidate, candidateOK := calls.MountedCallForOccurrence(module, call.ID())
		candidateKey, keyOK := calls.KeyForMountedCall(candidate)
		actual, actualOK := packs.MountedActualProjection(module, call.ID())
		if !candidateOK || !keyOK || !actualOK || actual.ActualCount() < 2 {
			continue
		}
		for dense := 0; dense < placementSchema.DenseKeyCount(); dense++ {
			root, rootOK := placementSchema.KeyAt(dense)
			_, allocationOK := values.AllocationResultFor(root)
			if rootOK && allocationOK {
				payloadRoot = root
				break
			}
		}
		if !payloadRoot.Valid() {
			continue
		}
		observations = make([]actualObservation, actual.ActualCount())
		for ordinal := range observations {
			observations[ordinal] = actualObservation{fact: values.Bottom(), present: true, valid: true}
			source, sourceOK := actual.ActualAt(ordinal)
			if !sourceOK {
				observations = nil
				break
			}
			if source.ID() == payloadID {
				atom, atomOK := values.Allocation(payloadRoot, materialization.Recent)
				fact, factOK := values.Singleton(atom)
				if !atomOK || !factOK {
					observations = nil
					break
				}
				observations[ordinal] = actualObservation{fact: fact, present: true, valid: true}
			}
		}
		if len(observations) != actual.ActualCount() {
			continue
		}
		candidateTargets := make([]calldomain.Target, 0, calls.SupportCount(candidateKey))
		for supportIndex := 0; supportIndex < calls.SupportCount(candidateKey); supportIndex++ {
			candidateTarget, targetOK := calls.SupportTargetAt(candidateKey, supportIndex)
			candidateOperation, candidateOperationOK := candidateTarget.Operation()
			if targetOK && candidateOperationOK && candidateOperation == operation {
				candidateTargets = append(candidateTargets, candidateTarget)
			}
		}
		callFact, callFactOK = calls.DispatchValue(candidateKey, candidateTargets, false)
		if !callFactOK || callFact.HasOpaqueAlternative() || callFact.KnownTargetCount() == 0 {
			continue
		}
		mounted, key, found = candidate, candidateKey, true
		break
	}
	if !found {
		t.Fatalf("transfer fixture has no exact mounted send call with payload allocation: calls=%d programCalls=%d operation=%d placementRoots=%d payloadID=%s", calls.MountedCallCount(), callCount, operation, placementSchema.DenseKeyCount(), payloadID)
	}
	openFact, openOK := calls.DispatchValue(key, nil, true)
	if !openOK || !openFact.HasOpaqueAlternative() {
		t.Fatal("transfer fixture open Call dispatch")
	}
	return transferHotLawFixture{packs: packs, calls: calls, placement: placementSchema, values: values, contract: target, mounted: mounted, key: key, callFact: callFact, openFact: openFact, payloadID: payloadID, payloadRoot: payloadRoot, observations: observations}
}

// transferStructuralVocabulary supplies only the neutral structural keys
// needed by the direct owner fixture. It is intentionally independent of the
// composite catalog so this package's tests do not create a production import
// cycle (composite itself registers placement-transfer).
func transferStructuralVocabulary(t testing.TB) structure.Table {
	t.Helper()
	counts := func(category structure.Category) int {
		switch category {
		case structure.CategoryArm:
			return 8
		case structure.CategoryEvent:
			return 3
		case structure.CategoryOutcome:
			return 7
		case structure.CategoryRuntimeKind:
			return int(runtimekind.Count) - 1
		case structure.CategoryOccurrenceKind:
			return 32
		default:
			return 1
		}
	}
	var specs []structure.Spec
	for category := structure.CategoryArm; category.Available(); category++ {
		for ordinal := 1; ordinal <= counts(category); ordinal++ {
			spelling := fmt.Sprintf("transfer-hot/%d/%d", category, ordinal)
			specs = append(specs, structure.Spec{Key: schema.Key(spelling), Category: category, Ordinal: uint16(ordinal), Spelling: spelling, Accepted: true})
		}
	}
	entries, entriesOK := structure.Collect(specs)
	if !entriesOK {
		t.Fatal("transfer structural declarations")
	}
	builder := schema.NewBuilder()
	if !builder.Register(structure.NewSurface(entries)) {
		t.Fatal("transfer structural surface")
	}
	for kind := schema.SurfaceKindAxis; kind <= schema.SurfaceKindObservation; kind++ {
		if !builder.Register(transferEmptySurface{kind: kind}) {
			t.Fatalf("transfer surface %d", kind)
		}
	}
	sealed, failure := builder.Seal()
	if failure.Available() || sealed == nil {
		t.Fatalf("transfer structural schema: %v", failure)
	}
	view, viewOK := sealed.Surface(schema.SurfaceKindStructure)
	if !viewOK {
		t.Fatal("transfer structural view")
	}
	table, tableOK := structure.NewTable(view)
	if !tableOK {
		t.Fatal("transfer structural table")
	}
	return table
}

type transferEmptySurface struct{ kind schema.SurfaceKind }

func (surface transferEmptySurface) Kind() schema.SurfaceKind { return surface.kind }
func (surface transferEmptySurface) Entries() []schema.Entry  { return nil }
func (surface transferEmptySurface) Seal(schema.View, schema.Sealed) schema.SealFailure {
	return schema.SealFailure{}
}
