package host

import (
	"reflect"
	"testing"

	flowkind "github.com/wippyai/go-lua/program/flow/kind"
	linkboundary "github.com/wippyai/go-lua/program/link/boundary"
	linkmodule "github.com/wippyai/go-lua/program/link/module"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	"github.com/wippyai/go-lua/program/lower"
	"github.com/wippyai/go-lua/program/target"
)

func hostFixture(t testing.TB) (*linkproject.Component, *linkboundary.Component, *linkmodule.Component) {
	t.Helper()
	closed := target.OperationSpec{Input: target.ValuesSpec{Tail: target.ValuesClosed}, Outcomes: []target.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: target.ValuesSpec{Tail: target.ValuesClosed}}}, Effects: target.RowSpec{Tail: target.RowClosed}}
	contract, err := target.Seal(&target.Spec{Operations: []target.OperationSpec{
		{Bindings: []target.BindingSpec{{Namespace: target.BindingBuiltin, Member: []string{"require"}}}, Input: closed.Input, Outcomes: closed.Outcomes, Effects: closed.Effects},
		{Bindings: []target.BindingSpec{{Namespace: target.BindingProvider, Owner: []string{"law"}, Member: []string{"endpoint"}}}, Input: closed.Input, Outcomes: closed.Outcomes, Effects: closed.Effects},
	}, InitialRoots: []target.InitialRootSpec{{Identity: "GlobalEnvRoot", Shape: target.BootShapeSpec{Aggregate: target.BootAggregateTable, Value: target.InitialValueSpec{Kind: target.InitialValueRoot, Root: "GlobalEnvRoot"}}}}})
	if err != nil {
		t.Fatal(err)
	}
	program, err := lower.Lower(lower.Source{Name: "host-law", Text: []byte("return 1")})
	if err != nil {
		t.Fatal(err)
	}
	pd, err := linkproject.Build(linkproject.Input{Target: contract, Modules: []linkproject.Module{{Name: "main", Program: program}}})
	if err != nil {
		t.Fatal(err)
	}
	project, err := pd.Finalize()
	if err != nil {
		t.Fatal(err)
	}
	bd, err := linkboundary.Build(linkboundary.Input{Project: project, Target: contract})
	if err != nil {
		t.Fatal(err)
	}
	boundary, err := bd.Finalize()
	if err != nil {
		t.Fatal(err)
	}
	md, err := linkmodule.Build(linkmodule.Input{Project: project, Boundary: boundary})
	if err != nil {
		t.Fatal(err)
	}
	module, err := md.Finalize()
	if err != nil {
		t.Fatal(err)
	}
	return project, boundary, module
}

func TestColdLifecycleAndExactPrerequisites(t *testing.T) {
	project, boundary, module := hostFixture(t)
	draft, err := Build(Input{Project: project, Boundary: boundary, Module: module})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := draft.Cold().ReplaySpec(); ok {
		t.Fatal("draft Cold leaked replay data")
	}
	copyDraft := *draft
	component, err := draft.Finalize()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := copyDraft.Finalize(); err == nil {
		t.Fatal("copied Draft finalized twice")
	}
	if !component.ContentID().Available() {
		t.Fatal("Host lacks content identity")
	}
	if _, ok := component.Cold().ReplaySpec(); !ok {
		t.Fatal("final Host Cold unavailable")
	}
	if _, err := Build(Input{Project: project, Boundary: boundary, Module: nil}); err == nil {
		t.Fatal("nil Module admitted")
	}
	// A separately sealed Boundary carries the same input content but not the
	// Project handles consumed by this Host.  Exact authority is required.
	foreignDraft, err := linkboundary.Build(linkboundary.Input{Project: project, Target: mustTarget(boundary)})
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := foreignDraft.Finalize()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Build(Input{Project: project, Boundary: foreign, Module: module}); err == nil {
		t.Fatal("foreign equivalent Boundary admitted")
	}
}

func mustTarget(boundary *linkboundary.Component) *target.Contract {
	contract, ok := boundary.Target()
	if !ok {
		panic("missing Boundary Target")
	}
	return contract
}

func TestContentExcludesUnobservedEndpointAdmission(t *testing.T) {
	project, boundary, module := hostFixture(t)
	firstDraft, err := Build(Input{Project: project, Boundary: boundary, Module: module})
	if err != nil {
		t.Fatal(err)
	}
	first, err := firstDraft.Finalize()
	if err != nil {
		t.Fatal(err)
	}
	contract := mustTarget(boundary)
	endpointDraft, err := linkboundary.Build(linkboundary.Input{Project: project, Target: contract, EndpointRequests: []linkboundary.EndpointRequest{{Identity: "unobserved", Binding: target.BindingSpec{Namespace: target.BindingProvider, Owner: []string{"law"}, Member: []string{"endpoint"}}}}})
	if err != nil {
		t.Fatal(err)
	}
	endpointBoundary, err := endpointDraft.Finalize()
	if err != nil {
		t.Fatal(err)
	}
	moduleDraft, err := linkmodule.Build(linkmodule.Input{Project: project, Boundary: endpointBoundary})
	if err != nil {
		t.Fatal(err)
	}
	endpointModule, err := moduleDraft.Finalize()
	if err != nil {
		t.Fatal(err)
	}
	secondDraft, err := Build(Input{Project: project, Boundary: endpointBoundary, Module: endpointModule})
	if err != nil {
		t.Fatal(err)
	}
	second, err := secondDraft.Finalize()
	if err != nil {
		t.Fatal(err)
	}
	if first.ContentID() != second.ContentID() {
		t.Fatal("unobserved endpoint admission changed Host content")
	}
}

func TestBootRootIDIsOwnerFencedAndReplayStable(t *testing.T) {
	project, boundary, module := hostFixture(t)
	firstDraft, err := Build(Input{Project: project, Boundary: boundary, Module: module})
	if err != nil {
		t.Fatal(err)
	}
	first, err := firstDraft.Finalize()
	if err != nil {
		t.Fatal(err)
	}
	root, ok := first.BootRoots().At(0)
	if !ok {
		t.Fatal("missing boot root")
	}
	id, ok := first.BootRoots().ID(root)
	if !ok || !id.Available() {
		t.Fatal("missing boot root ID")
	}
	secondDraft, err := Build(Input{Project: project, Boundary: boundary, Module: module})
	if err != nil {
		t.Fatal(err)
	}
	second, err := secondDraft.Finalize()
	if err != nil {
		t.Fatal(err)
	}
	secondRoot, ok := second.BootRoots().At(0)
	if !ok {
		t.Fatal("missing resealed boot root")
	}
	secondID, ok := second.BootRoots().ID(secondRoot)
	if !ok || secondID != id {
		t.Fatal("replay changed boot root ID")
	}
	if _, ok := second.BootRoots().ID(root); ok {
		t.Fatal("foreign equivalent boot root accepted")
	}
}

func TestReplayBuildSharesResolvedHostRowsAndRejectsCorruptReferences(t *testing.T) {
	project, boundary, module := hostFixture(t)
	authored, err := Build(Input{Project: project, Boundary: boundary, Module: module, Spec: Spec{
		ProviderCapabilities: []ProviderCapabilitySpec{{Identity: "global"}},
		ProviderCapabilitySeeds: []ProviderCapabilitySeedSpec{{
			Capability: "global", Source: ProviderCapabilitySourceInitialRoot, InitialRoot: "GlobalEnvRoot",
		}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	component, err := authored.Finalize()
	if err != nil {
		t.Fatal(err)
	}
	replay, ok := component.Cold().ReplaySpec()
	if !ok {
		t.Fatal("final Host omitted replay contract")
	}
	replayedDraft, err := BuildReplay(Input{Project: project, Boundary: boundary, Module: module}, replay)
	if err != nil {
		t.Fatalf("portable replay rejected: %v", err)
	}
	replayed, err := replayedDraft.Finalize()
	if err != nil {
		t.Fatal(err)
	}
	assertNoRetainedAuthoredSpec := func(label string, sealed *Component) {
		t.Helper()
		if sealed == nil || sealed.authority == nil {
			t.Fatalf("%s Host authority unavailable", label)
		}
		// Keep this structural check beside the package-private authority: a
		// future raw-coordinate field cannot silently remain populated after the
		// portable ReplaySpec is minted.
		raw := reflect.ValueOf(sealed.authority.spec)
		for index := 0; index < raw.NumField(); index++ {
			field := raw.Field(index)
			if field.Kind() == reflect.Slice && field.Len() != 0 {
				t.Fatalf("%s finalized Host retained raw Spec field %q", label, raw.Type().Field(index).Name)
			}
		}
	}
	assertNoRetainedAuthoredSpec("authored", component)
	assertNoRetainedAuthoredSpec("replayed", replayed)
	if replayed.ContentID() != component.ContentID() {
		t.Fatal("authored/replay ContentID diverged")
	}
	seed, ok := component.CapabilitySeeds().At(0)
	if !ok {
		t.Fatal("authored seed unavailable")
	}
	replaySeed, ok := replayed.CapabilitySeeds().At(0)
	if !ok {
		t.Fatal("replayed seed unavailable")
	}
	if source, ok := component.CapabilitySeeds().Source(seed); !ok || source != ProviderCapabilitySourceInitialRoot {
		t.Fatal("authored source changed")
	}
	if source, ok := replayed.CapabilitySeeds().Source(replaySeed); !ok || source != ProviderCapabilitySourceInitialRoot {
		t.Fatal("replay source changed")
	}

	corrupt := replay
	corrupt.Seeds[0].InitialRoot = "not-a-sealed-root"
	if _, err := BuildReplay(Input{Project: project, Boundary: boundary, Module: module}, corrupt); err == nil {
		t.Fatal("corrupt root reference admitted")
	}
	corrupt = replay
	corrupt.Seeds[0].Value[0] = 1
	if _, err := BuildReplay(Input{Project: project, Boundary: boundary, Module: module}, corrupt); err == nil {
		t.Fatal("cross-source corrupt replay admitted")
	}
	corrupt = replay
	corrupt.Exposures = append(corrupt.Exposures, ReplaySelector{Dispatch: HostDispatchLookup})
	if _, err := BuildReplay(Input{Project: project, Boundary: boundary, Module: module}, corrupt); err == nil {
		t.Fatal("unissued selector identities admitted")
	}
}

func TestColdIsDetachedAndHotQueriesAllocateNothing(t *testing.T) {
	project, boundary, module := hostFixture(t)
	draft, err := Build(Input{Project: project, Boundary: boundary, Module: module})
	if err != nil {
		t.Fatal(err)
	}
	component, err := draft.Finalize()
	if err != nil {
		t.Fatal(err)
	}
	cold := component.Cold()
	for i := 0; i < reflect.TypeOf(cold).NumField(); i++ {
		field := reflect.TypeOf(cold).Field(i)
		if field.Type.String() == "*host.authority" || field.Type.String() == "*authority" {
			t.Fatalf("Cold retained hot authority field %q", field.Name)
		}
	}
	if allocations := testing.AllocsPerRun(100, func() {
		_ = component.ContentID()
		_, _ = component.BootRoots().At(0)
		_, _ = component.Globals().At(0)
	}); allocations != 0 {
		t.Fatalf("hot Host queries allocated: %g", allocations)
	}
}

func TestEmptyHostAdmitsOrdinaryStringLiteralsAcrossMounts(t *testing.T) {
	_, fixtureBoundary, _ := hostFixture(t)
	contract := mustTarget(fixtureBoundary)
	left, err := lower.Lower(lower.Source{Name: "left", Text: []byte(`return 1, "same"`)})
	if err != nil {
		t.Fatal(err)
	}
	right, err := lower.Lower(lower.Source{Name: "right", Text: []byte(`return 2, "same"`)})
	if err != nil {
		t.Fatal(err)
	}
	projectDraft, err := linkproject.Build(linkproject.Input{Target: contract, Modules: []linkproject.Module{{Name: "left", Program: left}, {Name: "right", Program: right}}})
	if err != nil {
		t.Fatal(err)
	}
	project, err := projectDraft.Finalize()
	if err != nil {
		t.Fatal(err)
	}
	boundaryDraft, err := linkboundary.Build(linkboundary.Input{Project: project, Target: contract})
	if err != nil {
		t.Fatal(err)
	}
	boundary, err := boundaryDraft.Finalize()
	if err != nil {
		t.Fatal(err)
	}
	moduleDraft, err := linkmodule.Build(linkmodule.Input{Project: project, Boundary: boundary})
	if err != nil {
		t.Fatal(err)
	}
	module, err := moduleDraft.Finalize()
	if err != nil {
		t.Fatal(err)
	}
	draft, err := Build(Input{Project: project, Boundary: boundary, Module: module})
	if err != nil {
		t.Fatalf("empty Host rejected ordinary literals: %v", err)
	}
	if _, err := draft.Finalize(); err != nil {
		t.Fatal(err)
	}
}
