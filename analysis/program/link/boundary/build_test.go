package boundary

import (
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/lua/lower"
	"github.com/wippyai/go-lua/analysis/program"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	"github.com/wippyai/go-lua/analysis/program/target/compiler"
	"github.com/wippyai/go-lua/analysis/program/target/contract"
	"github.com/wippyai/go-lua/analysis/program/target/declaration"
	"github.com/wippyai/go-lua/domain/type/typ"
	domaincontract "github.com/wippyai/go-lua/domain/type/typecontract"
)

func TestBoundaryForeignAuthoritiesAndCardinalityFormula(t *testing.T) {
	contract := boundaryTarget(t, false)
	mainProgram := boundaryProgram(t)
	projectDraft, err := linkproject.Build(linkproject.Input{
		Modules: []linkproject.Module{{Name: "main", Program: mainProgram}},
		Target:  contract,
	})
	if err != nil {
		t.Fatal(err)
	}
	project, err := projectDraft.Finalize()
	if err != nil {
		t.Fatal(err)
	}
	draft, err := Build(Input{Project: project, Target: contract})
	if err != nil {
		t.Fatal(err)
	}
	component, err := draft.Finalize()
	if err != nil {
		t.Fatal(err)
	}
	require, ok := component.RequireOperation()
	if !ok || require == 0 {
		t.Fatal("missing scoped require operation")
	}

	applications := project.Applications()
	bases := applications.Bases()
	expected := 0
	for applicationIndex := 0; applicationIndex < bases.Count(); applicationIndex++ {
		application, ok := bases.At(applicationIndex)
		if !ok {
			t.Fatalf("base application %d unavailable", applicationIndex)
		}
		for operationIndex := 0; operationIndex < contract.Operations.OperationCount(); operationIndex++ {
			operation, ok := contract.Operations.OperationAt(operationIndex)
			if !ok {
				t.Fatalf("operation %d unavailable", operationIndex)
			}
			if component.ApplicationOperationAvailable(contract, application, operation) {
				expected++
			}
		}
	}
	got, ok := component.Cardinality()
	if !ok || got != expected {
		t.Fatalf("boundary cardinality = %d/%t, nested oracle = %d", got, ok, expected)
	}

	foreignTarget := boundaryTarget(t, false)
	if component.ApplicationOperationAvailable(foreignTarget, mustApplication(t, bases, 0), require) {
		t.Fatal("equivalent foreign Target crossed the Boundary authority fence")
	}
	foreignProjectDraft, err := linkproject.Build(linkproject.Input{
		Modules: []linkproject.Module{{Name: "main", Program: mainProgram}},
		Target:  foreignTarget,
	})
	if err != nil {
		t.Fatal(err)
	}
	foreignProject, err := foreignProjectDraft.Finalize()
	if err != nil {
		t.Fatal(err)
	}
	foreignApplication, ok := foreignProject.Applications().Bases().At(0)
	if !ok {
		t.Fatal("foreign base application unavailable")
	}
	if component.ApplicationOperationAvailable(contract, foreignApplication, require) {
		t.Fatal("equivalent foreign Project Application crossed the Boundary authority fence")
	}
	foreignBoundaryDraft, err := Build(Input{Project: foreignProject, Target: foreignTarget})
	if err != nil {
		t.Fatal(err)
	}
	foreignBoundary, err := foreignBoundaryDraft.Finalize()
	if err != nil {
		t.Fatal(err)
	}
	if foreignBoundary.ContentID() != component.ContentID() {
		t.Fatal("equivalent Boundary reseal changed its cold content identity")
	}
	if _, err := Build(Input{Project: project, Target: foreignTarget}); err == nil {
		t.Fatal("foreign Project/Target pair was accepted")
	}

}

func TestBoundaryRequireClassificationIsExactAndCallScoped(t *testing.T) {
	contract := boundaryTarget(t, false)
	mainProgram := boundaryProgram(t)
	draft, err := linkproject.Build(linkproject.Input{Modules: []linkproject.Module{{Name: "main", Program: mainProgram}}, Target: contract})
	if err != nil {
		t.Fatal(err)
	}
	project, err := draft.Finalize()
	if err != nil {
		t.Fatal(err)
	}
	boundaryDraft, err := Build(Input{Project: project, Target: contract})
	if err != nil {
		t.Fatal(err)
	}
	component, err := boundaryDraft.Finalize()
	if err != nil {
		t.Fatal(err)
	}
	require, ok := component.RequireOperation()
	if !ok {
		t.Fatal("require operation was not classified")
	}
	applications := project.Applications()
	for index := 0; index < applications.Count(); index++ {
		application, ok := applications.At(index)
		if !ok {
			t.Fatalf("application %d unavailable", index)
		}
		_, _, isCall := applications.Call(application)
		available := component.ApplicationOperationAvailable(contract, application, require)
		_, _, _, isImport := applications.Import(application)
		if isImport && available {
			t.Fatalf("Import application %d admitted scoped require", index)
		}
		if !isImport && !isCall && available {
			t.Fatalf("non-Call application %d admitted scoped require", index)
		}
	}
}

func TestBoundarySeedsAndEndpointsAreFencedAndCanonical(t *testing.T) {
	contract := boundaryEndpointTarget(t)
	program := boundaryProgram(t)
	projectDraft, err := linkproject.Build(linkproject.Input{Modules: []linkproject.Module{{Name: "main", Program: program}}, Target: contract})
	if err != nil {
		t.Fatal(err)
	}
	project, err := projectDraft.Finalize()
	if err != nil {
		t.Fatal(err)
	}
	provider := vocabulary.BindingSpec{Namespace: vocabulary.BindingProvider, Owner: []string{"host", "pkg"}, Member: []string{"service", "f"}}
	input := Input{Project: project, Target: contract, EndpointRequests: []EndpointRequest{{Identity: "second", Binding: provider}, {Identity: "first", Binding: provider}}}
	draft, err := Build(input)
	if err != nil {
		t.Fatal(err)
	}
	input.EndpointRequests[0].Binding.Owner[0] = "caller-mutated"
	input.EndpointRequests[0].Binding.Owner[1] = "caller-mutated"
	input.EndpointRequests[0].Binding.Member[0] = "caller-mutated"
	input.EndpointRequests[0].Binding.Member[1] = "caller-mutated"
	component, err := draft.Finalize()
	if err != nil {
		t.Fatal(err)
	}
	seeds, endpoints := component.Seeds(), component.Endpoints()
	if endpoints.Count() != 2 {
		t.Fatalf("endpoint count = %d, want 2", endpoints.Count())
	}
	first, ok := endpoints.At(0)
	if !ok {
		t.Fatal("first endpoint unavailable")
	}
	second, ok := endpoints.At(1)
	if !ok {
		t.Fatal("second endpoint unavailable")
	}
	firstSeed, ok := endpoints.Seed(first)
	if !ok {
		t.Fatal("first endpoint seed unavailable")
	}
	secondSeed, ok := endpoints.Seed(second)
	if !ok {
		t.Fatal("second endpoint seed unavailable")
	}
	if order, ok := seeds.Compare(firstSeed, secondSeed); !ok || order >= 0 {
		t.Fatalf("endpoint seed order = %d/%t", order, ok)
	}
	firstSeedID, ok := seeds.ID(firstSeed)
	if !ok {
		t.Fatal("first endpoint seed id unavailable")
	}
	firstID, ok := endpoints.ID(first)
	if !ok {
		t.Fatal("first endpoint id unavailable")
	}
	secondID, ok := endpoints.ID(second)
	if !ok || firstID == secondID {
		t.Fatal("same-operation endpoint identities collapsed")
	}
	if firstID != firstSeedID {
		t.Fatal("Endpoint ID and its nominal Seed ID diverged")
	}
	provider = vocabulary.BindingSpec{Namespace: vocabulary.BindingProvider, Owner: []string{"host", "pkg"}, Member: []string{"service", "f"}}
	providerOp, ok := contract.Operations.Lookup(provider)
	if !ok {
		t.Fatal("provider operation unavailable")
	}
	if got, ok := endpoints.Operation(first); !ok || got != providerOp {
		t.Fatalf("endpoint operation = %v/%t", got, ok)
	}
	requests := component.EndpointRequests()
	firstRequest, ok := requests.At(0)
	if !ok || firstRequest.Identity != "first" {
		t.Fatalf("canonical endpoint request = %#v/%t", firstRequest, ok)
	}
	firstRequest.Binding.Owner[0] = "mutated"
	replayed, ok := requests.At(0)
	if !ok || !reflect.DeepEqual(replayed.Binding.Owner, []string{"host", "pkg"}) {
		t.Fatal("endpoint replay projection retained caller-visible binding storage")
	}
	firstRequest.Binding.Member[0] = "mutated"
	replayed, ok = requests.At(0)
	if !ok || !reflect.DeepEqual(replayed.Binding.Member, []string{"service", "f"}) {
		t.Fatal("endpoint replay projection retained caller-visible member storage")
	}
	if _, ok := seeds.ForOperation(providerOp); !ok {
		t.Fatal("ordinary provider operation seed unavailable")
	}
	require, ok := component.RequireOperation()
	if !ok {
		t.Fatal("require unavailable")
	}
	if _, ok := seeds.ForOperation(require); ok {
		t.Fatal("scoped require gained a fabricated global operation seed")
	}
	shard, ok := project.Mounts().At(0)
	if !ok {
		t.Fatal("mount unavailable")
	}
	loader, ok := seeds.ScopedLoader(shard)
	if !ok {
		t.Fatal("scoped loader unavailable")
	}
	if got, ok := seeds.Operation(loader); !ok || got != require {
		t.Fatalf("loader operation = %v/%t", got, ok)
	}
	_, denied, _, _, ok := contract.InitialBinding("load")
	if !ok {
		t.Fatal("denied initial unavailable")
	}
	deniedSeed, disposition, ok := seeds.BootstrapCallable(denied)
	if !ok || disposition != CallableDeniedTarget {
		t.Fatalf("denied callable = %v/%v/%t", deniedSeed, disposition, ok)
	}
	if _, ok := seeds.Operation(deniedSeed); ok {
		t.Fatal("denied seed admitted as operation")
	}
	if _, ok := seeds.Loader(deniedSeed); ok {
		t.Fatal("denied seed admitted as loader")
	}
	_, ordinaryInitial, _, _, ok := contract.InitialBinding("op")
	if !ok {
		t.Fatal("ordinary initial unavailable")
	}
	ordinarySeed, disposition, ok := seeds.BootstrapCallable(ordinaryInitial)
	if !ok || disposition != CallableAdmittedOperation {
		t.Fatalf("ordinary bootstrap = %v/%v/%t", ordinarySeed, disposition, ok)
	}
	_, requireInitial, _, _, ok := contract.InitialBinding("require")
	if !ok {
		t.Fatal("scoped require initial unavailable")
	}
	if _, disposition, ok := seeds.BootstrapCallable(requireInitial); ok || disposition != CallableInvalid {
		t.Fatal("scoped require bootstrap gained a global callable seed")
	}
	absent, ok := contract.InitialAbsent()
	if !ok {
		t.Fatal("absent initial unavailable")
	}
	if _, disposition, ok := seeds.BootstrapCallable(absent); ok || disposition != CallableInvalid {
		t.Fatal("noncallable bootstrap admitted")
	}
	foreignDraft, err := Build(Input{Project: project, Target: contract, EndpointRequests: []EndpointRequest{{Identity: "first", Binding: provider}, {Identity: "second", Binding: provider}}})
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := foreignDraft.Finalize()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := foreign.Seeds().Operation(firstSeed); ok {
		t.Fatal("foreign equivalent reseal accepted seed")
	}
	if _, ok := foreign.Endpoints().Operation(first); ok {
		t.Fatal("foreign equivalent reseal accepted endpoint")
	}
	foreignProjectDraft, err := linkproject.Build(linkproject.Input{Modules: []linkproject.Module{{Name: "main", Program: program}}, Target: contract})
	if err != nil {
		t.Fatal(err)
	}
	foreignProject, err := foreignProjectDraft.Finalize()
	if err != nil {
		t.Fatal(err)
	}
	foreignShard, ok := foreignProject.Mounts().At(0)
	if !ok {
		t.Fatal("foreign mount unavailable")
	}
	if _, ok := seeds.ScopedLoader(foreignShard); ok {
		t.Fatal("foreign shard crossed Boundary Project fence")
	}
	if _, err := Build(Input{Project: project, Target: contract, EndpointRequests: []EndpointRequest{{Identity: "dup", Binding: provider}, {Identity: "dup", Binding: provider}}}); err == nil {
		t.Fatal("duplicate endpoint identity admitted")
	}
	if _, err := Build(Input{Project: project, Target: contract, EndpointRequests: []EndpointRequest{{Identity: "bad", Binding: vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"op"}}}}}); err == nil {
		t.Fatal("non-provider endpoint admitted")
	}
	if _, err := Build(Input{Project: project, Target: contract, EndpointRequests: []EndpointRequest{{Identity: "unknown", Binding: vocabulary.BindingSpec{Namespace: vocabulary.BindingProvider, Owner: []string{"host"}, Member: []string{"missing"}}}}}); err == nil {
		t.Fatal("unknown provider endpoint admitted")
	}
	permutedDraft, err := Build(Input{Project: project, Target: contract, EndpointRequests: []EndpointRequest{{Identity: "first", Binding: provider}, {Identity: "second", Binding: provider}}})
	if err != nil {
		t.Fatal(err)
	}
	permuted, err := permutedDraft.Finalize()
	if err != nil {
		t.Fatal(err)
	}
	if permuted.ContentID() != component.ContentID() {
		t.Fatal("endpoint input permutation changed Boundary identity")
	}
	permutedEndpoint, ok := permuted.Endpoints().At(0)
	if !ok {
		t.Fatal("permuted endpoint unavailable")
	}
	permutedEndpointID, ok := permuted.Endpoints().ID(permutedEndpoint)
	if !ok || permutedEndpointID != firstID {
		t.Fatal("endpoint permutation changed nominal identity")
	}
	permutedSeed, ok := permuted.Endpoints().Seed(permutedEndpoint)
	if !ok {
		t.Fatal("permuted endpoint seed unavailable")
	}
	permutedSeedID, ok := permuted.Seeds().ID(permutedSeed)
	if !ok || permutedSeedID != permutedEndpointID {
		t.Fatal("permuted Endpoint/Seed identities diverged")
	}
	plainDraft, err := Build(Input{Project: project, Target: contract})
	if err != nil {
		t.Fatal(err)
	}
	plain, err := plainDraft.Finalize()
	if err != nil {
		t.Fatal(err)
	}
	if plain.ContentID() == component.ContentID() {
		t.Fatal("endpoint delta did not change Boundary content")
	}
	plainValue, ok := plain.Values().At(0)
	if !ok {
		t.Fatal("plain value unavailable")
	}
	endpointValue, ok := component.Values().At(0)
	if !ok {
		t.Fatal("endpoint value unavailable")
	}
	plainValueID, _ := plain.Values().ID(plainValue)
	endpointValueID, _ := component.Values().ID(endpointValue)
	if plainValueID != endpointValueID {
		t.Fatal("endpoint delta churned Value relation identity")
	}
	plainProvider, ok := plain.Seeds().ForOperation(providerOp)
	if !ok {
		t.Fatal("plain provider seed unavailable")
	}
	endpointProvider, ok := seeds.ForOperation(providerOp)
	if !ok {
		t.Fatal("endpoint provider seed unavailable")
	}
	plainProviderID, _ := plain.Seeds().ID(plainProvider)
	endpointProviderID, _ := seeds.ID(endpointProvider)
	if plainProviderID != endpointProviderID {
		t.Fatal("endpoint delta churned operation seed identity")
	}
	plainLoader, ok := plain.Seeds().ScopedLoader(shard)
	if !ok {
		t.Fatal("plain loader unavailable")
	}
	plainLoaderID, _ := plain.Seeds().ID(plainLoader)
	endpointLoaderID, _ := seeds.ID(loader)
	if plainLoaderID != endpointLoaderID {
		t.Fatal("endpoint delta churned loader seed identity")
	}
	plainDenied, _, ok := plain.Seeds().BootstrapCallable(denied)
	if !ok {
		t.Fatal("plain denied seed unavailable")
	}
	plainDeniedID, _ := plain.Seeds().ID(plainDenied)
	endpointDeniedID, _ := seeds.ID(deniedSeed)
	if plainDeniedID != endpointDeniedID {
		t.Fatal("endpoint delta churned denied seed identity")
	}
}

func TestBoundarySeedIDsIgnoreUnrelatedMountDelta(t *testing.T) {
	contract := boundaryEndpointTarget(t)
	program := boundaryProgram(t)
	buildProject := func(modules []linkproject.Module) *linkproject.Component {
		draft, err := linkproject.Build(linkproject.Input{Modules: modules, Target: contract})
		if err != nil {
			t.Fatal(err)
		}
		project, err := draft.Finalize()
		if err != nil {
			t.Fatal(err)
		}
		return project
	}
	oneProject := buildProject([]linkproject.Module{{Name: "main", Program: program}})
	twoProject := buildProject([]linkproject.Module{{Name: "extra", Program: program}, {Name: "main", Program: program}})
	buildBoundary := func(project *linkproject.Component) *Component {
		draft, err := Build(Input{Project: project, Target: contract})
		if err != nil {
			t.Fatal(err)
		}
		component, err := draft.Finalize()
		if err != nil {
			t.Fatal(err)
		}
		return component
	}
	one, two := buildBoundary(oneProject), buildBoundary(twoProject)
	provider := vocabulary.BindingSpec{Namespace: vocabulary.BindingProvider, Owner: []string{"host", "pkg"}, Member: []string{"service", "f"}}
	op, ok := contract.Operations.Lookup(provider)
	if !ok {
		t.Fatal("provider operation unavailable")
	}
	oneOperation, ok := one.Seeds().ForOperation(op)
	if !ok {
		t.Fatal("one operation unavailable")
	}
	twoOperation, ok := two.Seeds().ForOperation(op)
	if !ok {
		t.Fatal("two operation unavailable")
	}
	oneOperationID, _ := one.Seeds().ID(oneOperation)
	twoOperationID, _ := two.Seeds().ID(twoOperation)
	if oneOperationID != twoOperationID {
		t.Fatal("mount delta churned operation seed")
	}
	_, denied, _, _, ok := contract.InitialBinding("load")
	if !ok {
		t.Fatal("denied unavailable")
	}
	oneDenied, _, ok := one.Seeds().BootstrapCallable(denied)
	if !ok {
		t.Fatal("one denied unavailable")
	}
	twoDenied, _, ok := two.Seeds().BootstrapCallable(denied)
	if !ok {
		t.Fatal("two denied unavailable")
	}
	oneDeniedID, _ := one.Seeds().ID(oneDenied)
	twoDeniedID, _ := two.Seeds().ID(twoDenied)
	if oneDeniedID != twoDeniedID {
		t.Fatal("mount delta churned denied seed")
	}
	shardForName := func(project *linkproject.Component, name string) linkproject.Shard {
		for index := 0; index < project.Mounts().Count(); index++ {
			shard, ok := project.Mounts().At(index)
			if !ok {
				t.Fatal("mount unavailable")
			}
			got, ok := project.Mounts().Name(shard)
			if !ok {
				t.Fatal("mount name unavailable")
			}
			if got == name {
				return shard
			}
		}
		t.Fatalf("mount %q unavailable", name)
		return linkproject.Shard{}
	}
	oneLoader, ok := one.Seeds().ScopedLoader(shardForName(oneProject, "main"))
	if !ok {
		t.Fatal("one main loader unavailable")
	}
	twoLoader, ok := two.Seeds().ScopedLoader(shardForName(twoProject, "main"))
	if !ok {
		t.Fatal("two main loader unavailable")
	}
	oneLoaderID, _ := one.Seeds().ID(oneLoader)
	twoLoaderID, _ := two.Seeds().ID(twoLoader)
	if oneLoaderID != twoLoaderID {
		t.Fatal("unrelated mount insertion churned main loader seed")
	}
}

func TestBoundaryDeniedBootstrapSegmentFirstLastAndMissing(t *testing.T) {
	contract := boundaryEndpointTarget(t)
	program := boundaryProgram(t)
	projectDraft, err := linkproject.Build(linkproject.Input{Modules: []linkproject.Module{{Name: "main", Program: program}}, Target: contract})
	if err != nil {
		t.Fatal(err)
	}
	project, err := projectDraft.Finalize()
	if err != nil {
		t.Fatal(err)
	}
	draft, err := Build(Input{Project: project, Target: contract})
	if err != nil {
		t.Fatal(err)
	}
	component, err := draft.Finalize()
	if err != nil {
		t.Fatal(err)
	}
	if component.authority.seedTable.deniedCount < 2 {
		t.Fatal("test target did not create a denied search segment")
	}
	_, first, _, _, ok := contract.InitialBinding("load")
	if !ok {
		t.Fatal("first denied unavailable")
	}
	_, last, _, _, ok := contract.InitialBinding("load2")
	if !ok {
		t.Fatal("last denied unavailable")
	}
	for _, value := range []vocabulary.InitialValue{first, last} {
		if _, disposition, ok := component.Seeds().BootstrapCallable(value); !ok || disposition != CallableDeniedTarget {
			t.Fatalf("denied segment lookup %d = %v/%t", value, disposition, ok)
		}
	}
	if _, disposition, ok := component.Seeds().BootstrapCallable(vocabulary.InitialValue(^uint32(0))); ok || disposition != CallableInvalid {
		t.Fatal("missing denied segment value admitted")
	}
}

func TestBoundaryDraftCopiesCannotFinalizeTwice(t *testing.T) {
	contract := boundaryEndpointTarget(t)
	program := boundaryProgram(t)
	projectDraft, err := linkproject.Build(linkproject.Input{Modules: []linkproject.Module{{Name: "main", Program: program}}, Target: contract})
	if err != nil {
		t.Fatal(err)
	}
	project, err := projectDraft.Finalize()
	if err != nil {
		t.Fatal(err)
	}
	draft, err := Build(Input{Project: project, Target: contract})
	if err != nil {
		t.Fatal(err)
	}
	copyDraft := *draft
	if _, err := draft.Finalize(); err != nil {
		t.Fatal(err)
	}
	if _, err := copyDraft.Finalize(); err == nil {
		t.Fatal("copied draft finalized twice")
	}
}

func TestBoundaryModuleRelationProjection(t *testing.T) {
	if _, ok := (*Component)(nil).ModuleRelationID(); ok {
		t.Fatal("nil component exposed Module relation")
	}
	program := boundaryProgram(t)
	provider := vocabulary.BindingSpec{Namespace: vocabulary.BindingProvider, Owner: []string{"host", "pkg"}, Member: []string{"service", "f"}}
	build := func(contract *contract.Contract, modules []linkproject.Module, endpoints []EndpointRequest) *Component {
		projectDraft, err := linkproject.Build(linkproject.Input{Modules: modules, Target: contract})
		if err != nil {
			t.Fatal(err)
		}
		project, err := projectDraft.Finalize()
		if err != nil {
			t.Fatal(err)
		}
		draft, err := Build(Input{Project: project, Target: contract, EndpointRequests: endpoints})
		if err != nil {
			t.Fatal(err)
		}
		component, err := draft.Finalize()
		if err != nil {
			t.Fatal(err)
		}
		return component
	}
	contract := boundaryEndpointTarget(t)
	base := build(contract, []linkproject.Module{{Name: "main", Program: program}}, nil)
	withEndpoints := build(contract, []linkproject.Module{{Name: "main", Program: program}}, []EndpointRequest{{Identity: "endpoint", Binding: provider}})
	baseID, ok := base.ModuleRelationID()
	if !ok {
		t.Fatal("base Module relation unavailable")
	}
	endpointID, ok := withEndpoints.ModuleRelationID()
	if !ok || endpointID != baseID {
		t.Fatal("endpoint geometry churned Module relation")
	}
	withoutExtraDenied := boundaryEndpointTargetWithSecondDenied(t, false)
	deniedDelta := build(withoutExtraDenied, []linkproject.Module{{Name: "main", Program: program}}, nil)
	deniedID, ok := deniedDelta.ModuleRelationID()
	if !ok || deniedID != baseID {
		t.Fatal("denied bootstrap delta churned Module relation")
	}
	mountDelta := build(contract, []linkproject.Module{{Name: "extra", Program: program}, {Name: "main", Program: program}}, nil)
	mountID, ok := mountDelta.ModuleRelationID()
	if !ok || mountID == baseID {
		t.Fatal("Value/mount delta did not change Module relation")
	}
	noRequire := boundaryTargetWithRequireBindings(t)
	requireDelta := build(noRequire, []linkproject.Module{{Name: "main", Program: program}}, nil)
	requireID, ok := requireDelta.ModuleRelationID()
	if !ok || requireID == baseID {
		t.Fatal("scoped require disposition did not change Module relation")
	}
	equivalent := build(contract, []linkproject.Module{{Name: "main", Program: program}}, nil)
	equivalentID, ok := equivalent.ModuleRelationID()
	if !ok || equivalentID != baseID {
		t.Fatal("equivalent Module relation reseal was nondeterministic")
	}
}

func TestBoundaryMatchesProjectIsAnExactOwnerFence(t *testing.T) {
	contract := boundaryEndpointTarget(t)
	program := boundaryProgram(t)
	buildProject := func() *linkproject.Component {
		draft, err := linkproject.Build(linkproject.Input{Modules: []linkproject.Module{{Name: "main", Program: program}}, Target: contract})
		if err != nil {
			t.Fatal(err)
		}
		project, err := draft.Finalize()
		if err != nil {
			t.Fatal(err)
		}
		return project
	}
	project := buildProject()
	draft, err := Build(Input{Project: project, Target: contract})
	if err != nil {
		t.Fatal(err)
	}
	component, err := draft.Finalize()
	if err != nil {
		t.Fatal(err)
	}
	if !component.MatchesProject(project) {
		t.Fatal("exact Project owner rejected")
	}
	equivalent := buildProject()
	equivalentID, equivalentOK := equivalent.MountRelationID()
	projectID, projectOK := project.MountRelationID()
	if !equivalentOK || !projectOK || equivalentID != projectID {
		t.Fatal("test Projects not content-equivalent")
	}
	if component.MatchesProject(equivalent) {
		t.Fatal("equivalent resealed Project crossed exact owner fence")
	}
	if component.MatchesProject(nil) || component.MatchesProject(&linkproject.Component{}) {
		t.Fatal("nil or unfinalized Project accepted")
	}
}

func TestBoundaryTargetIsFinalizedExactAuthority(t *testing.T) {
	if target, ok := (*Component)(nil).Target(); ok || target != nil {
		t.Fatal("nil component exposed Target")
	}
	contract := boundaryEndpointTarget(t)
	program := boundaryProgram(t)
	projectDraft, err := linkproject.Build(linkproject.Input{Modules: []linkproject.Module{{Name: "main", Program: program}}, Target: contract})
	if err != nil {
		t.Fatal(err)
	}
	project, err := projectDraft.Finalize()
	if err != nil {
		t.Fatal(err)
	}
	draft, err := Build(Input{Project: project, Target: contract})
	if err != nil {
		t.Fatal(err)
	}
	if target, ok := (&Component{authority: draft.state.authority}).Target(); ok || target != nil {
		t.Fatal("draft authority exposed Target through an unfinalized component")
	}
	component, err := draft.Finalize()
	if err != nil {
		t.Fatal(err)
	}
	got, ok := component.Target()
	if !ok || got != contract {
		t.Fatal("final Boundary did not return exact Target pointer")
	}
	foreign := boundaryEndpointTarget(t)
	if got == foreign {
		t.Fatal("test foreign Target was not distinct")
	}
}

func TestBoundaryValueAndEndpointRelationProjectionIsolation(t *testing.T) {
	if _, ok := (*Component)(nil).ValueRelationID(); ok {
		t.Fatal("nil component exposed Value relation")
	}
	if _, ok := (&Component{}).EndpointRelationID(); ok {
		t.Fatal("unfinalized component exposed Endpoint relation")
	}
	mainProgram := boundaryProgram(t)
	provider := vocabulary.BindingSpec{Namespace: vocabulary.BindingProvider, Owner: []string{"host", "pkg"}, Member: []string{"service", "f"}}
	build := func(contract *contract.Contract, source *program.Program, endpoints []EndpointRequest) *Component {
		projectDraft, err := linkproject.Build(linkproject.Input{Modules: []linkproject.Module{{Name: "main", Program: source}}, Target: contract})
		if err != nil {
			t.Fatal(err)
		}
		project, err := projectDraft.Finalize()
		if err != nil {
			t.Fatal(err)
		}
		draft, err := Build(Input{Project: project, Target: contract, EndpointRequests: endpoints})
		if err != nil {
			t.Fatal(err)
		}
		component, err := draft.Finalize()
		if err != nil {
			t.Fatal(err)
		}
		return component
	}
	contract := boundaryEndpointTarget(t)
	first := build(contract, mainProgram, []EndpointRequest{{Identity: "first", Binding: provider}})
	second := build(contract, mainProgram, []EndpointRequest{{Identity: "second", Binding: provider}})
	firstValue, ok := first.ValueRelationID()
	if !ok {
		t.Fatal("first Value relation unavailable")
	}
	secondValue, ok := second.ValueRelationID()
	if !ok || secondValue != firstValue {
		t.Fatal("endpoint delta churned Value relation")
	}
	firstEndpoint, ok := first.EndpointRelationID()
	if !ok {
		t.Fatal("first Endpoint relation unavailable")
	}
	secondEndpoint, ok := second.EndpointRelationID()
	if !ok || secondEndpoint == firstEndpoint {
		t.Fatal("endpoint identity delta did not change Endpoint relation")
	}
	noSecondDenied := build(boundaryEndpointTargetWithSecondDenied(t, false), mainProgram, []EndpointRequest{{Identity: "first", Binding: provider}})
	noSecondValue, _ := noSecondDenied.ValueRelationID()
	noSecondEndpoint, _ := noSecondDenied.EndpointRelationID()
	if noSecondValue != firstValue || noSecondEndpoint != firstEndpoint {
		t.Fatal("unrelated denied Seed delta churned observed relation")
	}
	changedProgram, err := lower.Lower(lower.Source{Name: "boundary-value-delta", Text: []byte(`local x = 1; return x`)})
	if err != nil {
		t.Fatal(err)
	}
	changedValue := build(contract, changedProgram, []EndpointRequest{{Identity: "first", Binding: provider}})
	changedValueID, ok := changedValue.ValueRelationID()
	if !ok || changedValueID == firstValue {
		t.Fatal("exact Value source delta did not change Value relation")
	}
}

func TestBoundaryPortableValueAndEndpointFindID(t *testing.T) {
	contract := boundaryEndpointTarget(t)
	mainProgram := boundaryProgram(t)
	provider := vocabulary.BindingSpec{Namespace: vocabulary.BindingProvider, Owner: []string{"host", "pkg"}, Member: []string{"service", "f"}}
	build := func(source *program.Program, endpoints []EndpointRequest) *Component {
		projectDraft, err := linkproject.Build(linkproject.Input{Modules: []linkproject.Module{{Name: "main", Program: source}}, Target: contract})
		if err != nil {
			t.Fatal(err)
		}
		project, err := projectDraft.Finalize()
		if err != nil {
			t.Fatal(err)
		}
		draft, err := Build(Input{Project: project, Target: contract, EndpointRequests: endpoints})
		if err != nil {
			t.Fatal(err)
		}
		component, err := draft.Finalize()
		if err != nil {
			t.Fatal(err)
		}
		return component
	}
	base := build(mainProgram, []EndpointRequest{{Identity: "first", Binding: provider}})
	value, ok := base.Values().At(0)
	if !ok {
		t.Fatal("base Value unavailable")
	}
	valueID, ok := base.Values().ID(value)
	if !ok {
		t.Fatal("base Value ID unavailable")
	}
	foundValue, ok := base.Values().FindID(valueID)
	if !ok {
		t.Fatal("base Value FindID unavailable")
	}
	if order, ok := base.Values().Compare(value, foundValue); !ok || order != 0 {
		t.Fatal("Value FindID did not rebind exact row")
	}
	endpoint, ok := base.Endpoints().At(0)
	if !ok {
		t.Fatal("base Endpoint unavailable")
	}
	endpointID, ok := base.Endpoints().ID(endpoint)
	if !ok {
		t.Fatal("base Endpoint ID unavailable")
	}
	foundEndpoint, ok := base.Endpoints().FindID(endpointID)
	if !ok || foundEndpoint != endpoint {
		t.Fatal("Endpoint FindID did not rebind exact row")
	}
	endpointSeed, _ := base.Endpoints().Seed(endpoint)
	endpointSeedID, _ := base.Seeds().ID(endpointSeed)
	if endpointSeedID != endpointID {
		t.Fatal("Endpoint and endpoint Seed ID diverged")
	}
	equivalent := build(mainProgram, []EndpointRequest{{Identity: "first", Binding: provider}})
	equivalentValue, ok := equivalent.Values().FindID(valueID)
	if !ok {
		t.Fatal("equivalent Value replay unavailable")
	}
	if _, ok := base.Values().Compare(equivalentValue, value); ok {
		t.Fatal("foreign equivalent Value handle crossed owner fence")
	}
	equivalentEndpoint, ok := equivalent.Endpoints().FindID(endpointID)
	if !ok {
		t.Fatal("equivalent Endpoint replay unavailable")
	}
	if _, ok := base.Endpoints().Operation(equivalentEndpoint); ok {
		t.Fatal("foreign equivalent Endpoint handle crossed owner fence")
	}
	endpointInsertion := build(mainProgram, []EndpointRequest{{Identity: "first", Binding: provider}, {Identity: "second", Binding: provider}})
	insertedValue, ok := endpointInsertion.Values().FindID(valueID)
	if !ok {
		t.Fatal("endpoint insertion churned Value FindID")
	}
	insertedValueID, _ := endpointInsertion.Values().ID(insertedValue)
	if insertedValueID != valueID {
		t.Fatal("endpoint insertion churned Value ID")
	}
	insertedEndpoint, ok := endpointInsertion.Endpoints().FindID(endpointID)
	if !ok {
		t.Fatal("unrelated endpoint insertion churned existing Endpoint FindID")
	}
	insertedEndpointID, _ := endpointInsertion.Endpoints().ID(insertedEndpoint)
	if insertedEndpointID != endpointID {
		t.Fatal("unrelated endpoint insertion churned existing Endpoint ID")
	}
	changedProgram, err := lower.Lower(lower.Source{Name: "boundary-find-value", Text: []byte(`local x = 1; return x`)})
	if err != nil {
		t.Fatal(err)
	}
	valueInsertion := build(changedProgram, []EndpointRequest{{Identity: "first", Binding: provider}})
	insertedByValue, ok := valueInsertion.Endpoints().FindID(endpointID)
	if !ok {
		t.Fatal("unrelated Value insertion churned Endpoint FindID")
	}
	insertedByValueID, _ := valueInsertion.Endpoints().ID(insertedByValue)
	if insertedByValueID != endpointID {
		t.Fatal("unrelated Value insertion churned Endpoint ID")
	}
	if _, ok := base.Values().FindID(identity.ContentID{}); ok {
		t.Fatal("unavailable Value ID resolved")
	}
	if _, ok := base.Endpoints().FindID(identity.ContentID{}); ok {
		t.Fatal("unavailable Endpoint ID resolved")
	}
}

func TestBoundaryPortableSeedAndEndpointIdentitiesIgnoreUnrelatedTargetDelta(t *testing.T) {
	program := boundaryProgram(t)
	provider := vocabulary.BindingSpec{Namespace: vocabulary.BindingProvider, Owner: []string{"host", "pkg"}, Member: []string{"service", "f"}}
	build := func(contract *contract.Contract) *Component {
		projectDraft, err := linkproject.Build(linkproject.Input{Modules: []linkproject.Module{{Name: "main", Program: program}}, Target: contract})
		if err != nil {
			t.Fatal(err)
		}
		project, err := projectDraft.Finalize()
		if err != nil {
			t.Fatal(err)
		}
		draft, err := Build(Input{Project: project, Target: contract, EndpointRequests: []EndpointRequest{{Identity: "endpoint", Binding: provider}}})
		if err != nil {
			t.Fatal(err)
		}
		component, err := draft.Finalize()
		if err != nil {
			t.Fatal(err)
		}
		return component
	}
	baseContract, unrelatedContract := boundaryEndpointTarget(t), boundaryEndpointTargetVariant(t, true, true, false, false)
	base, unrelated := build(baseContract), build(unrelatedContract)
	operation := func(contract *contract.Contract) vocabulary.Operation {
		op, ok := contract.Operations.Lookup(vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"op"}})
		if !ok {
			t.Fatal("operation unavailable")
		}
		return op
	}
	baseOperation, ok := base.Seeds().ForOperation(operation(baseContract))
	if !ok {
		t.Fatal("base operation seed unavailable")
	}
	unrelatedOperation, ok := unrelated.Seeds().ForOperation(operation(unrelatedContract))
	if !ok {
		t.Fatal("unrelated operation seed unavailable")
	}
	baseOperationID, _ := base.Seeds().ID(baseOperation)
	unrelatedOperationID, _ := unrelated.Seeds().ID(unrelatedOperation)
	if baseOperationID != unrelatedOperationID {
		t.Fatal("unrelated Target delta churned operation ID")
	}
	baseShard, _ := base.authority.project.Mounts().At(0)
	unrelatedShard, _ := unrelated.authority.project.Mounts().At(0)
	baseLoader, ok := base.Seeds().ScopedLoader(baseShard)
	if !ok {
		t.Fatal("base loader unavailable")
	}
	unrelatedLoader, ok := unrelated.Seeds().ScopedLoader(unrelatedShard)
	if !ok {
		t.Fatal("unrelated loader unavailable")
	}
	baseLoaderID, _ := base.Seeds().ID(baseLoader)
	unrelatedLoaderID, _ := unrelated.Seeds().ID(unrelatedLoader)
	if baseLoaderID != unrelatedLoaderID {
		t.Fatal("unrelated Target delta churned loader ID")
	}
	denied := func(contract *contract.Contract) vocabulary.InitialValue {
		_, value, _, _, ok := contract.InitialBinding("load")
		if !ok {
			t.Fatal("denied unavailable")
		}
		return value
	}
	baseDenied, _, ok := base.Seeds().BootstrapCallable(denied(baseContract))
	if !ok {
		t.Fatal("base denied unavailable")
	}
	unrelatedDenied, _, ok := unrelated.Seeds().BootstrapCallable(denied(unrelatedContract))
	if !ok {
		t.Fatal("unrelated denied unavailable")
	}
	baseDeniedID, _ := base.Seeds().ID(baseDenied)
	unrelatedDeniedID, _ := unrelated.Seeds().ID(unrelatedDenied)
	if baseDeniedID != unrelatedDeniedID {
		t.Fatal("unrelated Target delta churned denied ID")
	}
	baseEndpoint, _ := base.Endpoints().At(0)
	unrelatedEndpoint, _ := unrelated.Endpoints().At(0)
	baseProviderOp, ok := baseContract.Operations.Lookup(provider)
	if !ok {
		t.Fatal("base provider operation unavailable")
	}
	unrelatedProviderOp, ok := unrelatedContract.Operations.Lookup(provider)
	if !ok {
		t.Fatal("unrelated provider operation unavailable")
	}
	baseProviderOpID, _ := baseContract.OperationContentID(baseProviderOp)
	unrelatedProviderOpID, _ := unrelatedContract.OperationContentID(unrelatedProviderOp)
	if baseProviderOpID != unrelatedProviderOpID {
		t.Fatal("unrelated Target delta churned provider OperationContentID")
	}
	baseEndpointID, _ := base.Endpoints().ID(baseEndpoint)
	unrelatedEndpointID, _ := unrelated.Endpoints().ID(unrelatedEndpoint)
	if baseEndpointID != unrelatedEndpointID {
		t.Fatal("unrelated Target delta churned endpoint ID")
	}
	baseModuleID, _ := base.ModuleRelationID()
	unrelatedModuleID, _ := unrelated.ModuleRelationID()
	if baseModuleID != unrelatedModuleID {
		t.Fatal("unrelated Target delta churned Module relation")
	}
	changedOperationContract := boundaryEndpointTargetVariant(t, true, false, true, false)
	changedOperation := build(changedOperationContract)
	changedSeed, ok := changedOperation.Seeds().ForOperation(operation(changedOperationContract))
	if !ok {
		t.Fatal("changed operation seed unavailable")
	}
	changedOperationID, _ := changedOperation.Seeds().ID(changedSeed)
	if changedOperationID == baseOperationID {
		t.Fatal("exact operation change did not change operation ID")
	}
	changedDeniedContract := boundaryEndpointTargetVariant(t, true, false, false, true)
	changedDenied := build(changedDeniedContract)
	changedDeniedValue := denied(changedDeniedContract)
	changedDeniedSeed, _, ok := changedDenied.Seeds().BootstrapCallable(changedDeniedValue)
	if !ok {
		t.Fatal("changed denied unavailable")
	}
	changedDeniedID, _ := changedDenied.Seeds().ID(changedDeniedSeed)
	if changedDeniedID == baseDeniedID {
		t.Fatal("exact denied binding change did not change denied ID")
	}
}

func TestBoundaryRejectsNonScopedRequireShape(t *testing.T) {
	contract := boundaryTarget(t, true)
	program := boundaryProgram(t)
	draft, err := linkproject.Build(linkproject.Input{Modules: []linkproject.Module{{Name: "main", Program: program}}, Target: contract})
	if err != nil {
		t.Fatal(err)
	}
	project, err := draft.Finalize()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Build(Input{Project: project, Target: contract}); err == nil {
		t.Fatal("require operation with a second Target ingress was accepted")
	}
}

func TestBoundaryCardinalityFormulaRejectsOverflowAndBadPartition(t *testing.T) {
	maximum := int(^uint(0) >> 1)
	cases := []struct {
		name          string
		bases         int
		calls         int
		operations    int
		scopedRequire bool
		want          int
		ok            bool
	}{
		{name: "ordinary product", bases: 3, calls: 1, operations: 4, want: 12, ok: true},
		{name: "require partition", bases: 3, calls: 1, operations: 4, scopedRequire: true, want: 10, ok: true},
		{name: "call partition overflow", bases: 3, calls: 4, operations: 4, scopedRequire: true, ok: false},
		{name: "product overflow", bases: maximum, calls: 0, operations: 2, ok: false},
		{name: "subtraction underflow partition", bases: 3, calls: 0, operations: 0, scopedRequire: true, ok: false},
	}
	for _, test := range cases {
		got, ok := checkedCardinality(test.bases, test.calls, test.operations, test.scopedRequire)
		if ok != test.ok || (ok && got != test.want) {
			t.Errorf("%s: checkedCardinality = %d/%t, want %d/%t", test.name, got, ok, test.want, test.ok)
		}
	}
}

func TestBoundaryRejectsOwnerPrefixedAndConflictingRequire(t *testing.T) {
	if accepted, err := classifyRequireBinding(vocabulary.BindingBuiltin, 1, 1, "require"); err == nil || accepted {
		t.Fatalf("owner-bearing builtin require classification = %t/%v, want rejection", accepted, err)
	}

	program := boundaryProgram(t)
	conflictTarget := boundaryTargetWithRequireBindings(t,
		[]vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{"require"}}},
		[]vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{"require", "nested"}}},
	)
	conflictDraft, err := linkproject.Build(linkproject.Input{Modules: []linkproject.Module{{Name: "main", Program: program}}, Target: conflictTarget})
	if err != nil {
		t.Fatal(err)
	}
	conflictProject, err := conflictDraft.Finalize()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Build(Input{Project: conflictProject, Target: conflictTarget}); err == nil {
		t.Fatal("conflicting require operations were accepted")
	}
}

func mustApplication(t testing.TB, applications linkproject.Bases, index int) linkproject.Application {
	t.Helper()
	application, ok := applications.At(index)
	if !ok {
		t.Fatalf("application %d unavailable", index)
	}
	return application
}

func boundaryProgram(t testing.TB) *program.Program {
	t.Helper()
	p, err := lower.Lower(lower.Source{Name: "boundary-law", Text: []byte(`local function f() end; f(); local x = 1 + 2; require("dependency"); for k, v in pairs({}) do x = v end; return x`)})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func boundaryTarget(t testing.TB, extraRequireBinding bool) *contract.Contract {
	t.Helper()
	require := vocabulary.OperationSpec{
		Bindings: []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{"require"}}},
		Input:    vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed},
		Outcomes: []vocabulary.OutcomeSpec{{Kind: kind.OutcomeNormal, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}}},
		Effects:  vocabulary.RowSpec{Tail: vocabulary.RowClosed},
	}
	if extraRequireBinding {
		require.Bindings = append(require.Bindings, vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"other"}})
	}
	spec := declaration.Spec{
		Semantics: domaincontract.NewSemantics(),
		Operations: []vocabulary.OperationSpec{
			{Bindings: []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{"op"}}}, Input: vocabulary.ValuesSpec{Fixed: neutralTypes(t, typ.Any), Tail: vocabulary.ValuesClosed}, Outcomes: []vocabulary.OutcomeSpec{{Kind: kind.OutcomeNormal, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}}}, Effects: vocabulary.RowSpec{Tail: vocabulary.RowClosed}},
			require,
		},
		InitialRoots: []vocabulary.InitialRootSpec{{Identity: "GlobalEnvRoot", Shape: vocabulary.BootShapeSpec{Aggregate: vocabulary.BootAggregateTable, Value: vocabulary.InitialValueSpec{Kind: vocabulary.InitialValueRoot, Root: "GlobalEnvRoot"}}}},
		InitialEntries: []vocabulary.InitialEntrySpec{
			{Root: "GlobalEnvRoot", Key: boundaryStringKey("_G"), Value: vocabulary.InitialValueSpec{Kind: vocabulary.InitialValueRoot, Root: "GlobalEnvRoot"}, Mutability: vocabulary.InitialMutable},
			{Root: "GlobalEnvRoot", Key: boundaryStringKey("__link_absent"), Value: vocabulary.InitialValueSpec{Kind: vocabulary.InitialValueAbsent}, Mutability: vocabulary.InitialMutable},
			{Root: "GlobalEnvRoot", Key: boundaryStringKey("op"), Value: vocabulary.InitialValueSpec{Kind: vocabulary.InitialValueOperation, Operation: vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"op"}}}, Mutability: vocabulary.InitialMutable},
			{Root: "GlobalEnvRoot", Key: boundaryStringKey("require"), Value: vocabulary.InitialValueSpec{Kind: vocabulary.InitialValueOperation, Operation: vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"require"}}}, Mutability: vocabulary.InitialMutable},
		},
		InitialBindings: []vocabulary.InitialBindingSpec{
			{Name: "_G", Root: "GlobalEnvRoot", Key: boundaryStringKey("_G")},
			{Name: "__link_absent", Root: "GlobalEnvRoot", Key: boundaryStringKey("__link_absent")},
			{Name: "op", Root: "GlobalEnvRoot", Key: boundaryStringKey("op")},
			{Name: "require", Root: "GlobalEnvRoot", Key: boundaryStringKey("require")},
		},
	}
	contract, err := compiler.Seal(&spec)
	if err != nil {
		t.Fatal(err)
	}
	return contract
}

func boundaryEndpointTarget(t testing.TB) *contract.Contract {
	return boundaryEndpointTargetVariant(t, true, false, false, false)
}

func boundaryEndpointTargetWithSecondDenied(t testing.TB, secondDenied bool) *contract.Contract {
	return boundaryEndpointTargetVariant(t, secondDenied, false, false, false)
}

func boundaryEndpointTargetVariant(t testing.TB, secondDenied, unrelated, changeOperation, changeDenied bool) *contract.Contract {
	t.Helper()
	spec := declaration.Spec{Semantics: domaincontract.NewSemantics(), Operations: []vocabulary.OperationSpec{
		{Bindings: []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{"op"}}}, Input: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}, Outcomes: []vocabulary.OutcomeSpec{{Kind: kind.OutcomeNormal, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}}}, Effects: vocabulary.RowSpec{Tail: vocabulary.RowClosed}},
		{Bindings: []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{"require"}}}, Input: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}, Outcomes: []vocabulary.OutcomeSpec{{Kind: kind.OutcomeNormal, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}}}, Effects: vocabulary.RowSpec{Tail: vocabulary.RowClosed}},
		{Bindings: []vocabulary.BindingSpec{{Namespace: vocabulary.BindingProvider, Owner: []string{"host", "pkg"}, Member: []string{"service", "f"}}}, Input: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}, Outcomes: []vocabulary.OutcomeSpec{{Kind: kind.OutcomeNormal, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}}}, Effects: vocabulary.RowSpec{Tail: vocabulary.RowClosed}},
	}, InitialRoots: []vocabulary.InitialRootSpec{{Identity: "GlobalEnvRoot", Shape: vocabulary.BootShapeSpec{Aggregate: vocabulary.BootAggregateTable, Value: vocabulary.InitialValueSpec{Kind: vocabulary.InitialValueRoot, Root: "GlobalEnvRoot"}}}}, InitialEntries: []vocabulary.InitialEntrySpec{
		{Root: "GlobalEnvRoot", Key: boundaryStringKey("_G"), Value: vocabulary.InitialValueSpec{Kind: vocabulary.InitialValueRoot, Root: "GlobalEnvRoot"}, Mutability: vocabulary.InitialMutable},
		{Root: "GlobalEnvRoot", Key: boundaryStringKey("__link_absent"), Value: vocabulary.InitialValueSpec{Kind: vocabulary.InitialValueAbsent}, Mutability: vocabulary.InitialMutable},
		{Root: "GlobalEnvRoot", Key: boundaryStringKey("op"), Value: vocabulary.InitialValueSpec{Kind: vocabulary.InitialValueOperation, Operation: vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"op"}}}, Mutability: vocabulary.InitialMutable},
		{Root: "GlobalEnvRoot", Key: boundaryStringKey("require"), Value: vocabulary.InitialValueSpec{Kind: vocabulary.InitialValueOperation, Operation: vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"require"}}}, Mutability: vocabulary.InitialMutable},
		{Root: "GlobalEnvRoot", Key: boundaryStringKey("load"), Value: vocabulary.InitialValueSpec{Kind: vocabulary.InitialValueDeniedOperation, Operation: vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"load"}}}, Mutability: vocabulary.InitialMutable},
		{Root: "GlobalEnvRoot", Key: boundaryStringKey("load2"), Value: vocabulary.InitialValueSpec{Kind: vocabulary.InitialValueDeniedOperation, Operation: vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"load2"}}}, Mutability: vocabulary.InitialMutable},
	}, InitialBindings: []vocabulary.InitialBindingSpec{{Name: "_G", Root: "GlobalEnvRoot", Key: boundaryStringKey("_G")}, {Name: "__link_absent", Root: "GlobalEnvRoot", Key: boundaryStringKey("__link_absent")}, {Name: "op", Root: "GlobalEnvRoot", Key: boundaryStringKey("op")}, {Name: "require", Root: "GlobalEnvRoot", Key: boundaryStringKey("require")}, {Name: "load", Root: "GlobalEnvRoot", Key: boundaryStringKey("load")}, {Name: "load2", Root: "GlobalEnvRoot", Key: boundaryStringKey("load2")}}}
	if !secondDenied {
		spec.InitialEntries = spec.InitialEntries[:len(spec.InitialEntries)-1]
		spec.InitialBindings = spec.InitialBindings[:len(spec.InitialBindings)-1]
	}
	if changeOperation {
		spec.Operations[0].Input = vocabulary.ValuesSpec{Fixed: neutralTypes(t, typ.Any), Tail: vocabulary.ValuesClosed}
	}
	if changeDenied {
		spec.InitialEntries[4].Value.Operation = vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"load_changed"}}
	}
	if unrelated {
		spec.Operations = append(spec.Operations, vocabulary.OperationSpec{Bindings: []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{"unrelated"}}}, Input: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}, Outcomes: []vocabulary.OutcomeSpec{{Kind: kind.OutcomeNormal, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}}}, Effects: vocabulary.RowSpec{Tail: vocabulary.RowClosed}})
		spec.InitialEntries = append(spec.InitialEntries, vocabulary.InitialEntrySpec{Root: "GlobalEnvRoot", Key: boundaryStringKey("unrelated_boot"), Value: vocabulary.InitialValueSpec{Kind: vocabulary.InitialValueString, String: "unrelated"}, Mutability: vocabulary.InitialMutable})
	}
	contract, err := compiler.Seal(&spec)
	if err != nil {
		t.Fatal(err)
	}
	return contract
}

func boundaryTargetWithRequireBindings(t testing.TB, requireBindings ...[]vocabulary.BindingSpec) *contract.Contract {
	t.Helper()
	operations := []vocabulary.OperationSpec{
		{Bindings: []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{"op"}}}, Input: vocabulary.ValuesSpec{Fixed: neutralTypes(t, typ.Any), Tail: vocabulary.ValuesClosed}, Outcomes: []vocabulary.OutcomeSpec{{Kind: kind.OutcomeNormal, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}}}, Effects: vocabulary.RowSpec{Tail: vocabulary.RowClosed}},
	}
	for _, bindings := range requireBindings {
		operations = append(operations, vocabulary.OperationSpec{
			Bindings: bindings,
			Input:    vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed},
			Outcomes: []vocabulary.OutcomeSpec{{Kind: kind.OutcomeNormal, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}}},
			Effects:  vocabulary.RowSpec{Tail: vocabulary.RowClosed},
		})
	}
	spec := declaration.Spec{
		Semantics:    domaincontract.NewSemantics(),
		Operations:   operations,
		InitialRoots: []vocabulary.InitialRootSpec{{Identity: "GlobalEnvRoot", Shape: vocabulary.BootShapeSpec{Aggregate: vocabulary.BootAggregateTable, Value: vocabulary.InitialValueSpec{Kind: vocabulary.InitialValueRoot, Root: "GlobalEnvRoot"}}}},
		InitialEntries: []vocabulary.InitialEntrySpec{
			{Root: "GlobalEnvRoot", Key: boundaryStringKey("_G"), Value: vocabulary.InitialValueSpec{Kind: vocabulary.InitialValueRoot, Root: "GlobalEnvRoot"}, Mutability: vocabulary.InitialMutable},
			{Root: "GlobalEnvRoot", Key: boundaryStringKey("__link_absent"), Value: vocabulary.InitialValueSpec{Kind: vocabulary.InitialValueAbsent}, Mutability: vocabulary.InitialMutable},
		},
		InitialBindings: []vocabulary.InitialBindingSpec{
			{Name: "_G", Root: "GlobalEnvRoot", Key: boundaryStringKey("_G")},
			{Name: "__link_absent", Root: "GlobalEnvRoot", Key: boundaryStringKey("__link_absent")},
		},
	}
	contract, err := compiler.Seal(&spec)
	if err != nil {
		t.Fatal(err)
	}
	return contract
}

func boundaryStringKey(value string) keyspace.LiteralValue {
	return keyspace.LiteralValue{Kind: keyspace.LiteralString, String: value}
}
