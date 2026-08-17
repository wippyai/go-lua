package module

import (
	"testing"

	domaincontract "github.com/wippyai/go-lua/domain/type/typecontract"
	"github.com/wippyai/go-lua/analysis/lua/lower"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	linkboundary "github.com/wippyai/go-lua/analysis/program/link/boundary"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	"github.com/wippyai/go-lua/analysis/program/target"
)

func moduleFixture(t *testing.T) (*linkproject.Component, *linkboundary.Component, Spec) {
	t.Helper()
	contract, err := target.Seal(&target.Spec{Semantics: domaincontract.NewSemantics(), Operations: []target.OperationSpec{
		{Bindings: []target.BindingSpec{{Namespace: target.BindingBuiltin, Member: []string{"require"}}}, Input: target.ValuesSpec{Tail: target.ValuesClosed}, Outcomes: []target.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: target.ValuesSpec{Tail: target.ValuesClosed}}}, Effects: target.RowSpec{Tail: target.RowClosed}},
		{Bindings: []target.BindingSpec{{Namespace: target.BindingProvider, Owner: []string{"host"}, Member: []string{"send"}}}, Input: target.ValuesSpec{Tail: target.ValuesClosed}, Outcomes: []target.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: target.ValuesSpec{Tail: target.ValuesClosed}}}, Effects: target.RowSpec{Tail: target.RowClosed}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	main, err := lower.Lower(lower.Source{Name: "module-main", Text: []byte(`require("dependency")`)})
	if err != nil {
		t.Fatal(err)
	}
	dependency, err := lower.Lower(lower.Source{Name: "module-dependency", Text: []byte(`return 1`)})
	if err != nil {
		t.Fatal(err)
	}
	projectDraft, err := linkproject.Build(linkproject.Input{Target: contract, Modules: []linkproject.Module{{Name: "main", Program: main}, {Name: "dependency", Program: dependency}}})
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
	importTerm, ok := main.Module().ImportAt(0)
	if !ok {
		t.Fatal("missing import")
	}
	return project, boundary, Spec{
		Actors:             []ActorSpec{{Name: "actor"}},
		ModuleCacheAliases: []ModuleCacheAliasClassSpec{{Actor: "actor", Instances: []string{"cache-main"}, Representative: "cache-main"}, {Actor: "actor", Instances: []string{"cache-dependency"}, Representative: "cache-dependency"}},
		AnalysisRoots:      []AnalysisRootSpec{{Name: "main", Module: "main", Actor: "actor", Instance: "cache-main"}, {Name: "dependency", Module: "dependency", Actor: "actor", Instance: "cache-dependency"}},
		ModuleCacheEntries: []ModuleCacheEntrySpec{{Module: "main", Import: importTerm.Term, FromRoot: "main", ToRoot: "dependency"}},
	}
}

func sealModuleFixture(t *testing.T) *Component {
	t.Helper()
	project, boundary, spec := moduleFixture(t)
	draft, err := Build(Input{Project: project, Boundary: boundary, Spec: spec})
	if err != nil {
		t.Fatal(err)
	}
	// Build must detach author-owned input before the caller regains control.
	spec.Actors[0].Name = "mutated-input"
	spec.ModuleCacheAliases[0].Instances[0] = "mutated-input"
	component, err := draft.Finalize()
	if err != nil {
		t.Fatal(err)
	}
	return component
}

func TestModuleColdLifecycleAndDetachedSpec(t *testing.T) {
	project, boundary, spec := moduleFixture(t)
	draft, err := Build(Input{Project: project, Boundary: boundary, Spec: spec})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := draft.Cold().Spec(); ok {
		t.Fatal("pre-final Cold was available")
	}
	copyDraft := *draft
	component, err := draft.Finalize()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := copyDraft.Finalize(); err == nil {
		t.Fatal("copied Draft finalized twice")
	}
	cold := component.Cold()
	saved, ok := cold.Spec()
	if !ok {
		t.Fatal("final Cold unavailable")
	}
	if saved.Actors[0].Name == "mutated-input" || saved.ModuleCacheAliases[0].Instances[0] == "mutated-input" {
		t.Fatal("component retained caller-owned module Spec storage")
	}
	saved.Actors[0].Name = "mutated"
	saved.ModuleCacheAliases[0].Instances[0] = "mutated"
	again, ok := component.Cold().Spec()
	if !ok || again.Actors[0].Name == "mutated" || again.ModuleCacheAliases[0].Instances[0] == "mutated" {
		t.Fatal("Cold Spec leaked mutable storage")
	}
}

func TestModuleEquivalentHandlesFenceAndRebind(t *testing.T) {
	left := sealModuleFixture(t)
	right := sealModuleFixture(t)
	if left.ContentID() != right.ContentID() {
		t.Fatal("equivalent Module reseal changed content")
	}
	actor, _ := left.Actors().At(0)
	if _, ok := right.Actors().Index(actor); ok {
		t.Fatal("foreign Actor accepted")
	}
	root, _ := left.Roots().At(0)
	if _, _, _, ok := right.Roots().Mapping(root); ok {
		t.Fatal("foreign Root accepted")
	}
	if _, ok := right.Roots().Rebind(root); !ok {
		t.Fatal("Root rebind failed")
	}
	instance, _ := left.Cache().InstanceAt(0)
	if _, ok := right.Cache().Representative(instance); ok {
		t.Fatal("foreign Instance accepted")
	}
	entry, _ := left.Cache().EntryAt(0)
	if _, _, _, ok := right.Cache().EntryMapping(entry); ok {
		t.Fatal("foreign Entry accepted")
	}
	if _, ok := right.Cache().RebindEntry(entry); !ok {
		t.Fatal("Entry rebind failed")
	}
	coordinate, _ := left.Coordinates().At(0)
	if _, _, _, ok := right.Coordinates().Mapping(coordinate); ok {
		t.Fatal("foreign Coordinate accepted")
	}
	if _, ok := right.Coordinates().Rebind(coordinate); !ok {
		t.Fatal("Coordinate rebind failed")
	}
	generation, _ := left.Generations().At(0)
	if _, _, _, _, ok := right.Generations().Entry(generation); ok {
		t.Fatal("foreign Generation accepted")
	}
	if _, ok := right.Generations().Rebind(generation); !ok {
		t.Fatal("Generation rebind failed")
	}
	outcome, _ := left.Outcomes().At(generation, 0)
	if _, ok := right.Outcomes().Kind(outcome); ok {
		t.Fatal("foreign Outcome accepted")
	}
	if _, ok := right.Outcomes().Rebind(outcome); !ok {
		t.Fatal("Outcome rebind failed")
	}
	terminal, _ := left.Terminals().At(0)
	if _, ok := right.Terminals().Outcome(terminal); ok {
		t.Fatal("foreign Terminal accepted")
	}
	if _, ok := right.Terminals().Rebind(terminal); !ok {
		t.Fatal("Terminal rebind failed")
	}
	if subject, ok := left.Outcomes().ReadySubject(outcome); ok {
		if _, ok := right.ReadySubjects().Value(subject); ok {
			t.Fatal("foreign ReadySubject accepted")
		}
	}
}

func TestModuleContentTracksOnlyModuleRelationAndSpec(t *testing.T) {
	project, boundary, spec := moduleFixture(t)
	seal := func(input Spec) *Component {
		t.Helper()
		draft, err := Build(Input{Project: project, Boundary: boundary, Spec: input})
		if err != nil {
			t.Fatal(err)
		}
		component, err := draft.Finalize()
		if err != nil {
			t.Fatal(err)
		}
		return component
	}
	base := seal(spec)
	changed := cloneSpec(spec)
	changed.Actors[0].Name = "other-actor"
	for index := range changed.ModuleCacheAliases {
		changed.ModuleCacheAliases[index].Actor = "other-actor"
	}
	for index := range changed.AnalysisRoots {
		changed.AnalysisRoots[index].Actor = "other-actor"
	}
	if seal(changed).ContentID() == base.ContentID() {
		t.Fatal("Module spec delta did not change content")
	}
	contract, ok := boundary.Target()
	if !ok {
		t.Fatal("missing fixture target")
	}
	endpointDraft, err := linkboundary.Build(linkboundary.Input{Project: project, Target: contract, EndpointRequests: []linkboundary.EndpointRequest{{Identity: "host.send", Binding: target.BindingSpec{Namespace: target.BindingProvider, Owner: []string{"host"}, Member: []string{"send"}}}}})
	if err != nil {
		t.Fatal(err)
	}
	endpointBoundary, err := endpointDraft.Finalize()
	if err != nil {
		t.Fatal(err)
	}
	draft, err := Build(Input{Project: project, Boundary: endpointBoundary, Spec: spec})
	if err != nil {
		t.Fatal(err)
	}
	endpointModule, err := draft.Finalize()
	if err != nil {
		t.Fatal(err)
	}
	if endpointModule.ContentID() != base.ContentID() {
		t.Fatal("endpoint-only Boundary delta churned Module content")
	}
}

func TestModulePrerequisiteFencesAndCanonicalPermutation(t *testing.T) {
	project, boundary, spec := moduleFixture(t)
	draft, err := Build(Input{Project: project, Boundary: boundary, Spec: spec})
	if err != nil {
		t.Fatal(err)
	}
	component, err := draft.Finalize()
	if err != nil {
		t.Fatal(err)
	}
	if !component.MatchesProject(project) || !component.MatchesBoundary(boundary) || component.MatchesProject(nil) || component.MatchesBoundary(nil) {
		t.Fatal("exact prerequisite fences malformed")
	}
	permuted := cloneSpec(spec)
	permuted.AnalysisRoots[0], permuted.AnalysisRoots[1] = permuted.AnalysisRoots[1], permuted.AnalysisRoots[0]
	permuted.ModuleCacheAliases[0], permuted.ModuleCacheAliases[1] = permuted.ModuleCacheAliases[1], permuted.ModuleCacheAliases[0]
	draft, err = Build(Input{Project: project, Boundary: boundary, Spec: permuted})
	if err != nil {
		t.Fatal(err)
	}
	other, err := draft.Finalize()
	if err != nil {
		t.Fatal(err)
	}
	if other.ContentID() != component.ContentID() {
		t.Fatal("canonical module input permutation changed content")
	}
	foreignProject, foreignBoundary, _ := moduleFixture(t)
	if component.MatchesProject(foreignProject) || component.MatchesBoundary(foreignBoundary) {
		t.Fatal("equivalent foreign prerequisite crossed fence")
	}
}

func TestModuleGenerationHashAndTerminalIndexAreExact(t *testing.T) {
	component := sealModuleFixture(t)
	for index := 0; index < component.Terminals().Count(); index++ {
		terminal, ok := component.Terminals().At(index)
		if !ok {
			t.Fatalf("terminal %d unavailable", index)
		}
		id, ok := component.Terminals().ID(terminal)
		if !ok {
			t.Fatalf("terminal %d lacks ID", index)
		}
		found, ok := component.Terminals().Find(id)
		if !ok {
			t.Fatalf("terminal %d index miss", index)
		}
		if got, ok := component.Terminals().ID(found); !ok || got != id {
			t.Fatalf("terminal %d index mismatch", index)
		}
	}
	first := ModuleInitGenerationRef{component: component.ContentID(), entry: [32]byte{1}}
	second := first
	second.entry[31] = 1
	if hashGeneration(first) == hashGeneration(second) {
		t.Fatal("generation hash truncated EntryID")
	}
}

func TestModuleOutcomeValidationIsDirectAtBounds(t *testing.T) {
	component := sealModuleFixture(t)
	generation, ok := component.Generations().At(0)
	if !ok {
		t.Fatal("missing generation")
	}
	count := component.Outcomes().Count(generation)
	if count == 0 {
		t.Fatal("missing outcomes")
	}
	first, ok := component.Outcomes().At(generation, 0)
	if !ok || !component.validOutcome(first) {
		t.Fatal("first outcome rejected")
	}
	last, ok := component.Outcomes().At(generation, count-1)
	if !ok || !component.validOutcome(last) {
		t.Fatal("last outcome rejected")
	}
	if component.validOutcome(ModuleInitOutcome{component: component, generation: generation.ordinal, kind: flowkind.OutcomeReturn, ordinal: ^uint32(0)}) {
		t.Fatal("out-of-range return accepted")
	}
	if component.validOutcome(ModuleInitOutcome{component: component, generation: generation.ordinal, kind: flowkind.OutcomeKind(255)}) {
		t.Fatal("invalid outcome kind accepted")
	}
}

func TestHostRelationIDExcludesCacheOnlyGeometry(t *testing.T) {
	project, boundary, spec := moduleFixture(t)
	seal := func(input Spec) *Component {
		draft, err := Build(Input{Project: project, Boundary: boundary, Spec: input})
		if err != nil {
			t.Fatal(err)
		}
		component, err := draft.Finalize()
		if err != nil {
			t.Fatal(err)
		}
		return component
	}
	base := seal(spec)
	baseID, ok := base.HostRelationID()
	if !ok {
		t.Fatal("missing Host relation")
	}
	cacheOnly := cloneSpec(spec)
	// Rename both cache instances and their root/cache references without
	// changing actor or analysis-root identity/mount placement.
	cacheOnly.ModuleCacheAliases[0].Instances[0] = "other-main"
	cacheOnly.ModuleCacheAliases[0].Representative = "other-main"
	cacheOnly.ModuleCacheAliases[1].Instances[0] = "other-dependency"
	cacheOnly.ModuleCacheAliases[1].Representative = "other-dependency"
	cacheOnly.AnalysisRoots[0].Instance = "other-main"
	cacheOnly.AnalysisRoots[1].Instance = "other-dependency"
	if id, ok := seal(cacheOnly).HostRelationID(); !ok || id != baseID {
		t.Fatal("cache-only delta churned Host relation")
	}
	actorDelta := cloneSpec(spec)
	actorDelta.Actors[0].Name = "other"
	for i := range actorDelta.ModuleCacheAliases {
		actorDelta.ModuleCacheAliases[i].Actor = "other"
	}
	for i := range actorDelta.AnalysisRoots {
		actorDelta.AnalysisRoots[i].Actor = "other"
	}
	if id, ok := seal(actorDelta).HostRelationID(); !ok || id == baseID {
		t.Fatal("actor delta did not change Host relation")
	}
}
