package analysis

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/lua/lower"
	"github.com/wippyai/go-lua/analysis/program"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/link"
	linkmodule "github.com/wippyai/go-lua/analysis/program/link/module"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	"github.com/wippyai/go-lua/analysis/program/target/contract"
	"github.com/wippyai/go-lua/analysis/result"
	"github.com/wippyai/go-lua/analysis/schema/executioncontext"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	"github.com/wippyai/go-lua/domain/placement"
	placementpublication "github.com/wippyai/go-lua/domain/placement/publication"
)

// TestSharedMountedArtifactAcrossActorContextsKeepsContextualPlacement
// proves the cross-plane join at the narrowest public boundary available to
// analysis tests: one mounted Program is compiled into one cached Artifact
// across two Links, each Link derives an exact actor-qualified context for
// that module, and detached Placement query rows retain distinct
// context-qualified Site identities and placement classes. The detached
// result carries ContextID directly; no pre-detach correlation table or
// opaque SiteID recovery participates in the proof.
func TestSharedMountedArtifactAcrossActorContextsKeepsContextualPlacement(t *testing.T) {
	contract := fixtureContract(t)
	firstLink, secondLink, libProgram := sharedLibraryActorLinks(t, contract)
	workspace := NewWorkspace()
	t.Cleanup(func() { _ = workspace.Close() })
	first, firstStatus, firstDiagnostics := workspace.CompileWithDiagnostics(firstLink)
	if firstStatus != CompileComplete || first == nil || first.state == nil || first.state.artifacts == nil {
		t.Fatalf("compile first shared-library actor link = %v/%v diagnostics=%+v", firstStatus, first, firstDiagnostics)
	}
	t.Cleanup(func() { _ = first.Close() })
	second, secondStatus, secondDiagnostics := workspace.CompileWithDiagnostics(secondLink)
	if secondStatus != CompileComplete || second == nil || second.state == nil || second.state.artifacts == nil {
		t.Fatalf("compile second shared-library actor link = %v/%v diagnostics=%+v", secondStatus, second, secondDiagnostics)
	}
	t.Cleanup(func() { _ = second.Close() })
	firstProduct, firstProductOK := first.state.artifacts.products[libProgram.ContentID()]
	secondProduct, secondProductOK := second.state.artifacts.products[libProgram.ContentID()]
	if mountedProgramByName(t, firstLink, "lib") != libProgram || mountedProgramByName(t, secondLink, "lib") != libProgram {
		t.Fatal("the two Link mounts did not retain one immutable library Program")
	}
	if !firstProductOK || !secondProductOK || firstProduct.Artifact == nil || firstProduct.Artifact != secondProduct.Artifact {
		t.Fatal("the two Link mounts did not reuse one immutable library Artifact")
	}
	if firstLink.ContentID() == secondLink.ContentID() {
		t.Fatal("actor-specific Links collapsed to one Link identity")
	}
	firstPlacementSchema, firstPlacementOK := first.PlacementSchema()
	secondPlacementSchema, secondPlacementOK := second.PlacementSchema()
	if !firstPlacementOK || !secondPlacementOK || !firstPlacementSchema.Valid() || !secondPlacementSchema.Valid() || firstPlacementSchema.ContentID() == secondPlacementSchema.ContentID() {
		t.Fatal("actor-specific Plans collapsed to one Link-bound Placement schema")
	}
	libProgramID := libProgram.ContentID()
	libArtifactID := firstProduct.Artifact.ID()
	firstContext := libraryContext(t, firstLink, "lib")
	secondContext := libraryContext(t, secondLink, "lib")
	if firstContext.ID() == secondContext.ID() || firstContext.ActorID() == secondContext.ActorID() {
		t.Fatal("the two library placements did not retain distinct actor-qualified contexts")
	}

	firstResult, firstSolveStatus, firstSolveDiagnostics := first.SolveWithDiagnostics(context.Background(), fixtureSolveOptions())
	if firstSolveStatus != AnalyzeComplete || firstResult == nil {
		refusal, stage, rule, runtimeOK := first.state.instantiateRuntimeTopology()
		constructionRow, constructionRowOK := refusal.ConstructionRow()
		t.Fatalf("solve first shared-library actor link = %v/%v diagnostics=%+v runtime=%v stage=%v rule=%v refusal-stage=%v commit=%v row=%v/%v", firstSolveStatus, firstResult, firstSolveDiagnostics, runtimeOK, stage, rule, refusal.Stage(), refusal.Commit(), constructionRow, constructionRowOK)
	}
	secondResult, secondSolveStatus, secondSolveDiagnostics := second.SolveWithDiagnostics(context.Background(), fixtureSolveOptions())
	if secondSolveStatus != AnalyzeComplete || secondResult == nil {
		refusal, stage, rule, runtimeOK := second.state.instantiateRuntimeTopology()
		constructionRow, constructionRowOK := refusal.ConstructionRow()
		t.Fatalf("solve second shared-library actor link = %v/%v diagnostics=%+v runtime=%v stage=%v rule=%v refusal-stage=%v commit=%v row=%v/%v", secondSolveStatus, secondResult, secondSolveDiagnostics, runtimeOK, stage, rule, refusal.Stage(), refusal.Commit(), constructionRow, constructionRowOK)
	}
	if firstResult.ContentID() == secondResult.ContentID() {
		t.Fatal("contextual result identities collapsed across actor-specific Links")
	}
	firstClasses := libraryPlacementClasses(t, first, firstResult, firstLink, "lib")
	secondClasses := libraryPlacementClasses(t, second, secondResult, secondLink, "lib")
	placementVaries := false
	for point, firstPointClasses := range firstClasses {
		secondPointClasses, secondPointOK := secondClasses[point]
		if !secondPointOK || !firstPointClasses[placement.SharedHeap] || !secondPointClasses[placement.OwnedHeap] || placementClassSetsEqual(firstPointClasses, secondPointClasses) {
			continue
		}
		placementVaries = true
		break
	}
	if !placementVaries {
		t.Fatalf("contextual library placement did not vary at one shared Program point: first=%v second=%v", firstClasses, secondClasses)
	}
	if libProgram.ContentID() != libProgramID || firstProduct.Artifact.ID() != libArtifactID || firstProduct.Artifact != secondProduct.Artifact {
		t.Fatal("solving contextual Links mutated or replaced the shared library Artifact")
	}
	firstLibraryKey := moduleKeyByName(t, firstLink, "lib")
	secondLibraryKey := moduleKeyByName(t, secondLink, "lib")
	firstLibrarySiteIDs := make(map[identity.ContentID]struct{})
	firstLibraryPublicationKeys := make(map[identity.ContentID]struct{})
	firstLibrarySites := 0
	for _, site := range first.state.querySites {
		if site.Context.ModuleKey() == firstLibraryKey && site.Context.ID() != firstContext.ID() {
			t.Fatal("first Link query site lost its exact library context")
		}
		if site.Context.ModuleKey() == firstLibraryKey {
			firstLibrarySites++
			firstLibrarySiteIDs[site.ID] = struct{}{}
			query, queryOK := first.state.committed.Query(site.ID)
			publicationKey, publicationKeyOK := query.PublicationKey()
			if !queryOK || query.ContextID() != site.Context.ID() || !publicationKeyOK {
				t.Fatal("first Link committed query lost its exact context/publication identity")
			}
			firstLibraryPublicationKeys[publicationKey] = struct{}{}
		}
	}
	secondLibrarySiteIDs := make(map[identity.ContentID]struct{})
	secondLibraryPublicationKeys := make(map[identity.ContentID]struct{})
	secondLibrarySites := 0
	for _, site := range second.state.querySites {
		if site.Context.ModuleKey() == secondLibraryKey && site.Context.ID() != secondContext.ID() {
			t.Fatal("second Link query site lost its exact library context")
		}
		if site.Context.ModuleKey() == secondLibraryKey {
			secondLibrarySites++
			secondLibrarySiteIDs[site.ID] = struct{}{}
			query, queryOK := second.state.committed.Query(site.ID)
			publicationKey, publicationKeyOK := query.PublicationKey()
			if !queryOK || query.ContextID() != site.Context.ID() || !publicationKeyOK {
				t.Fatal("second Link committed query lost its exact context/publication identity")
			}
			secondLibraryPublicationKeys[publicationKey] = struct{}{}
		}
	}
	if firstLibrarySites == 0 || secondLibrarySites == 0 {
		t.Fatalf("query sites omitted mounted library: first=%d second=%d", firstLibrarySites, secondLibrarySites)
	}
	for siteID := range firstLibrarySiteIDs {
		if _, shared := secondLibrarySiteIDs[siteID]; shared {
			t.Fatalf("actor-specific library query sites reused SiteID %v", siteID)
		}
	}
	for publicationKey := range firstLibraryPublicationKeys {
		if _, shared := secondLibraryPublicationKeys[publicationKey]; shared {
			t.Fatalf("actor-specific library query rows reused publication key %v", publicationKey)
		}
	}
}

func sharedLibraryActorLinks(t testing.TB, target *contract.Contract) (*link.Link, *link.Link, *program.Program) {
	t.Helper()
	base := fixtureLink(t, "transitive-libs/shared-lib-divergent-consumers")
	programs := map[string]*program.Program{
		"lib":            mountedProgramByName(t, base, "lib"),
		"consumer_send":  mountedProgramByName(t, base, "consumer_send"),
		"consumer_store": mountedProgramByName(t, base, "consumer_store"),
	}
	mainA, err := lower.Lower(lower.Source{Name: "main-a.lua", Text: []byte(`local send = require("consumer_send")
send.run()
return true`)})
	if err != nil {
		t.Fatalf("lower main-a.lua: %v", err)
	}
	mainB, err := lower.Lower(lower.Source{Name: "main-b.lua", Text: []byte(`local store = require("consumer_store")
store.run()
return true`)})
	if err != nil {
		t.Fatalf("lower main-b.lua: %v", err)
	}
	programs["main-a"], programs["main-b"] = mainA, mainB
	build := func(actor string, names []string, mainName string, mainProgram *program.Program) *link.Link {
		modules := make([]linkproject.Module, 0, len(names))
		for _, name := range names {
			modules = append(modules, linkproject.Module{Name: name, Program: programs[name]})
		}
		modules = append(modules, linkproject.Module{Name: mainName, Program: mainProgram})
		aliases := make([]linkmodule.ModuleCacheAliasClassSpec, 0, len(modules))
		roots := make([]linkmodule.AnalysisRootSpec, 0, len(modules))
		rootName := make(map[string]string, len(modules))
		for _, module := range modules {
			instance := "instance:" + actor + ":" + module.Name
			root := "root:" + actor + ":" + module.Name
			aliases = append(aliases, linkmodule.ModuleCacheAliasClassSpec{Actor: actor, Instances: []string{instance}, Representative: instance})
			roots = append(roots, linkmodule.AnalysisRootSpec{Name: root, Module: module.Name, Actor: actor, Instance: instance})
			rootName[module.Name] = root
		}
		entries := make([]linkmodule.ModuleCacheEntrySpec, 0)
		for _, module := range modules {
			imports := module.Program.Flow().Authored().Imports()
			for index := 0; index < imports.Count(); index++ {
				item, itemOK := imports.ImportAt(index)
				if !itemOK || item.Call == 0 || item.Request == 0 || !module.Program.Flow().Executable().Contains(item.Call) {
					continue
				}
				_, _, requested, requestedOK := module.Program.Source().Literals().Strings().At(int(keyspace.TermOrdinal(item.Request) - 1))
				if !requestedOK {
					t.Fatalf("%s import request unavailable", module.Name)
				}
				from, fromOK := rootName[module.Name]
				to, toOK := rootName[requested]
				if !fromOK || !toOK {
					t.Fatalf("%s import %q lacks actor-local roots", module.Name, requested)
				}
				entries = append(entries, linkmodule.ModuleCacheEntrySpec{Module: module.Name, Import: item.Term, FromRoot: from, ToRoot: to})
			}
		}
		linked, err := link.Seal(&link.Spec{Target: target, Modules: modules, Module: linkmodule.Spec{
			Actors: []linkmodule.ActorSpec{{Name: actor}}, ModuleCacheAliases: aliases, AnalysisRoots: roots, ModuleCacheEntries: entries,
		}})
		if err != nil {
			t.Fatalf("seal %s shared-library actor link: %v", actor, err)
		}
		return linked
	}
	first := build("actor-a", []string{"lib", "consumer_send"}, "main-a", mainA)
	second := build("actor-b", []string{"lib", "consumer_store"}, "main-b", mainB)
	return first, second, programs["lib"]
}

func mountedProgramByName(t testing.TB, linked *link.Link, name string) *program.Program {
	t.Helper()
	if linked == nil || linked.Project() == nil {
		t.Fatal("Link project unavailable")
	}
	mounts := linked.Project().Mounts()
	for index := 0; index < mounts.Count(); index++ {
		shard, shardOK := mounts.At(index)
		mountName, nameOK := mounts.Name(shard)
		mounted, programOK := mounts.Program(shard)
		if shardOK && nameOK && programOK && mountName == name {
			return mounted
		}
	}
	t.Fatalf("missing mounted Program %q", name)
	return nil
}

func libraryContext(t testing.TB, linked *link.Link, name string) executioncontext.Context {
	t.Helper()
	directory := linked.ContextDirectory()
	if !directory.Available() {
		t.Fatal("Link did not publish its execution-context directory")
	}
	key := moduleKeyByName(t, linked, name)
	var selected executioncontext.Context
	for index := 0; index < directory.ContextCount(); index++ {
		row, rowOK := directory.ContextAt(index)
		if !rowOK || !row.Available() {
			t.Fatalf("context[%d] unavailable", index)
		}
		if row.ModuleKey() == key {
			if selected.Available() {
				t.Fatal("library Link unexpectedly published more than one context")
			}
			selected = row
		}
	}
	if !selected.Available() {
		t.Fatalf("missing context for module %q", name)
	}
	return selected
}

func libraryPlacementClasses(t testing.TB, plan *Plan, result *result.Result, linked *link.Link, name string) map[identity.ContentID]map[placement.Placement]bool {
	t.Helper()
	publication, publicationOK := placementpublication.Open(result)
	if !publicationOK || publication.QueryCount() == 0 {
		t.Fatal("detached result did not publish one placement family")
	}
	placementSchema, schemaOK := plan.PlacementSchema()
	if !schemaOK || !placementSchema.Valid() {
		t.Fatal("placement schema unavailable")
	}
	key := moduleKeyByName(t, linked, name)
	expectedContext := libraryContext(t, linked, name).ID()
	classes := make(map[identity.ContentID]map[placement.Placement]bool)
	for index := 0; index < publication.QueryCount(); index++ {
		query, queryOK := publication.QueryAt(index)
		if !queryOK {
			t.Fatalf("placement query[%d] unavailable", index)
		}
		contextID, contextOK := query.ContextID()
		mountID, mountOK := query.MountID()
		if !contextOK || !mountOK {
			t.Fatalf("placement query[%d] lost its detached context geometry", index)
		}
		if mountID != key {
			continue
		}
		if contextID != expectedContext {
			t.Fatalf("library placement query[%d] context = %v, want %v", index, contextID, expectedContext)
		}
		pointID, pointOK := query.PointID()
		if !pointOK || !pointID.Available() {
			t.Fatalf("library placement query[%d] point unavailable", index)
		}
		decoded, decodedOK := query.Placement(placementSchema)
		if !decodedOK {
			t.Fatalf("library placement query[%d] did not decode under its exact schema", index)
		}
		rows := decoded.Allocations()
		for {
			allocation, allocationOK := rows.Next()
			if !allocationOK {
				break
			}
			allocationKey, keyOK := placementSchema.Heap().KeyForID(allocation.AllocationID())
			if !keyOK {
				t.Fatalf("placement allocation has no Heap key")
			}
			module, _, _, kind, _, originOK := placementSchema.Heap().AllocationOriginForKey(allocationKey)
			if !originOK || module != key || kind != heapdomain.AllocationTable {
				continue
			}
			class, classOK := allocation.Placement()
			if classOK {
				pointClasses := classes[pointID]
				if pointClasses == nil {
					pointClasses = make(map[placement.Placement]bool)
					classes[pointID] = pointClasses
				}
				pointClasses[class] = true
			}
		}
	}
	if len(classes) == 0 {
		t.Fatal("library placement family had no table allocation row")
	}
	return classes
}

func placementClassSetsEqual(left, right map[placement.Placement]bool) bool {
	if len(left) != len(right) {
		return false
	}
	for class := range left {
		if !right[class] {
			return false
		}
	}
	return true
}

func moduleKeyByName(t testing.TB, linked *link.Link, name string) identity.ContentID {
	t.Helper()
	mounts := linked.Project().Mounts()
	for index := 0; index < mounts.Count(); index++ {
		shard, shardOK := mounts.At(index)
		mountName, nameOK := mounts.Name(shard)
		key, keyOK := linked.Project().ModuleKey(shard)
		if shardOK && nameOK && keyOK && mountName == name {
			return key
		}
	}
	t.Fatalf("missing module key %q", name)
	return identity.ContentID{}
}
