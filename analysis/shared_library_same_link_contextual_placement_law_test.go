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
	"github.com/wippyai/go-lua/domain/composite"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	"github.com/wippyai/go-lua/domain/placement"
	placementpublication "github.com/wippyai/go-lua/domain/placement/publication"
)

// TestSameLinkSharedLibraryAcrossActorContextsKeepsOneArtifactPlan proves
// the core contextual placement contract in one Link. The immutable library
// Program is mounted once; two actor roots address that same mount and the
// one runtime plan expands its exact Context rows. The synthetic library has
// no global Cell so this law isolates the contextual runtime contract while
// the Host global migration remains a separate acceptance law.
//
// This law is intentionally red until the current contextual placement-formal
// solve execution blocker is repaired. Its assertions must remain strict: a
// fallback context, a second library mount, a collapsed publication key, or
// equal placement classes is a failure rather than a reason to skip a lane.
func TestSameLinkSharedLibraryAcrossActorContextsKeepsOneArtifactPlan(t *testing.T) {
	linked, libProgram := sameLinkSharedLibraryFixture(t, fixtureContract(t))
	directory := linked.ContextDirectory()
	// The Directory owns one canonical reflexive transition per Context;
	// authored module-composition edges contribute the four cross-context
	// transitions in this fixture.
	wantTransitions := 6 + 4
	if !directory.Available() || directory.ContextCount() != 6 || directory.RootCount() != 6 || directory.TransitionCount() != wantTransitions {
		t.Fatalf("same-Link context geometry contexts=%d roots=%d transitions=%d, want transitions=%d", directory.ContextCount(), directory.RootCount(), directory.TransitionCount(), wantTransitions)
	}

	libModule := sameLinkModuleKey(t, linked, "lib")
	libMounts := 0
	for index := 0; index < linked.Project().Mounts().Count(); index++ {
		shard, shardOK := linked.Project().Mounts().At(index)
		name, nameOK := linked.Project().Mounts().Name(shard)
		mounted, programOK := linked.Project().Mounts().Program(shard)
		if !shardOK || !nameOK || !programOK {
			t.Fatalf("same-Link mount[%d] unavailable", index)
		}
		if name != "lib" {
			continue
		}
		libMounts++
		if mounted != libProgram {
			t.Fatal("same-Link library mount changed its immutable Program owner")
		}
	}
	if libMounts != 1 {
		t.Fatalf("same-Link library mount count=%d, want exactly one", libMounts)
	}

	libContexts := sameLinkContextsForModule(t, directory, libModule)
	if len(libContexts) != 2 || libContexts[0].ID() == libContexts[1].ID() || libContexts[0].ActorID() == libContexts[1].ActorID() {
		t.Fatalf("same-Link library contexts=%v, want two distinct actor-qualified rows", libContexts)
	}

	workspace := NewWorkspace()
	t.Cleanup(func() { _ = workspace.Close() })
	plan, compileStatus, compileDiagnostics := workspace.CompileWithDiagnostics(linked)
	if compileStatus != CompileComplete || plan == nil || plan.state == nil || plan.state.artifacts == nil {
		t.Fatalf("compile same-Link shared library=%v/%v diagnostics=%+v", compileStatus, plan, compileDiagnostics)
	}
	t.Cleanup(func() { _ = plan.Close() })
	product, productOK := plan.state.artifacts.products[libProgram.ContentID()]
	if !productOK || product.Artifact == nil || !product.Artifact.Available() || product.Template == nil || !product.Template.Available() || product.Template.ProgramID() != libProgram.ContentID() {
		t.Fatal("same-Link library did not publish one immutable Program/Artifact product")
	}
	artifactID := product.Artifact.ID()
	programID := libProgram.ContentID()

	stateResult, solveStatus, solveDiagnostics := plan.SolveWithDiagnostics(context.Background(), fixtureSolveOptions())
	if solveStatus != AnalyzeComplete || stateResult == nil {
		refusal, stage, rule, runtimeOK := plan.state.instantiateRuntimeTopology()
		constructionRow, constructionRowOK := refusal.ConstructionRow()
		t.Fatalf("solve same-Link shared library=%v/%v diagnostics=%+v runtime=%v stage=%v rule=%v refusal-stage=%v commit=%v row=%v/%v", solveStatus, stateResult, solveDiagnostics, runtimeOK, stage, rule, refusal.Stage(), refusal.Commit(), constructionRow, constructionRowOK)
	}
	if plan.state.committed == nil {
		t.Fatal("same-Link solve did not retain one committed program")
	}
	libSites := sameLinkLibrarySites(t, plan, libModule)
	if len(libSites) < 2 {
		t.Fatalf("same-Link library query sites=%d, want at least one per actor Context", len(libSites))
	}
	byContext := make(map[identity.ContentID]int, len(libContexts))
	for _, site := range libSites {
		if !site.Context.Available() || site.Context.ModuleKey() != libModule || !site.ID.Available() || !site.Point.Available() {
			t.Fatal("same-Link library query site lost its exact Context/Point")
		}
		byContext[site.Context.ID()]++
	}
	for _, contextRow := range libContexts {
		if byContext[contextRow.ID()] == 0 {
			t.Fatalf("same-Link library query sites omitted Context %v", contextRow.ID())
		}
	}

	// Every library query is resolved through the same committed engine plane.
	// ContextID, StateOrdinal, and the owner-issued publication key are read
	// from the query handle itself; none is reconstructed from an opaque ID.
	seenStateByPoint := make(map[identity.ContentID]map[uint64]identity.ContentID)
	seenPublicationByPoint := make(map[identity.ContentID]map[identity.ContentID]identity.ContentID)
	for _, site := range libSites {
		query, queryOK := plan.state.committed.Query(site.ID)
		if !queryOK {
			t.Fatalf("same-Link committed query missing SiteID %v", site.ID)
		}
		if query.ContextID() != site.Context.ID() {
			t.Fatalf("same-Link query ContextID=%v, want site ContextID=%v", query.ContextID(), site.Context.ID())
		}
		stateOrdinal, stateOK := query.StateOrdinal()
		if !stateOK {
			t.Fatalf("same-Link query %v lost its compact StateOrdinal", site.ID)
		}
		publicationKey, publicationOK := query.PublicationKey()
		if !publicationOK {
			t.Fatalf("same-Link query %v lost its publication key", site.ID)
		}
		states := seenStateByPoint[site.Point]
		if states == nil {
			states = make(map[uint64]identity.ContentID)
			seenStateByPoint[site.Point] = states
		}
		if previous, duplicate := states[stateOrdinal]; duplicate && previous != site.Context.ID() {
			t.Fatalf("same-Link library Point %v collapsed Contexts %v and %v onto StateOrdinal %d", site.Point, previous, site.Context.ID(), stateOrdinal)
		}
		states[stateOrdinal] = site.Context.ID()
		publications := seenPublicationByPoint[site.Point]
		if publications == nil {
			publications = make(map[identity.ContentID]identity.ContentID)
			seenPublicationByPoint[site.Point] = publications
		}
		if previous, duplicate := publications[publicationKey]; duplicate && previous != site.Context.ID() {
			t.Fatalf("same-Link library Point %v collapsed Contexts %v and %v onto publication key %v", site.Point, previous, site.Context.ID(), publicationKey)
		}
		publications[publicationKey] = site.Context.ID()
	}
	sharedPoints := 0
	for point, states := range seenStateByPoint {
		if len(states) < 2 {
			continue
		}
		if publications := seenPublicationByPoint[point]; len(publications) < 2 {
			t.Fatalf("same-Link library Point %v has distinct StateOrdinals but not distinct publication keys", point)
		}
		sharedPoints++
	}
	if sharedPoints == 0 {
		t.Fatal("same-Link library query sites did not publish a common Point in both actor ContextIDs")
	}

	classes := sameLinkLibraryPlacementClasses(t, plan, stateResult, libModule)
	sendContext := sameLinkContextForModule(t, directory, sameLinkModuleKey(t, linked, "consumer_send")).ActorID()
	storeContext := sameLinkContextForModule(t, directory, sameLinkModuleKey(t, linked, "consumer_store")).ActorID()
	if sendContext == storeContext {
		t.Fatal("same-Link send/store consumers collapsed their actor contexts")
	}
	var divergent bool
	for point, sendClasses := range classes {
		storeClasses, storeOK := classes[sameLinkPlacementPoint{context: sameLinkContextIDForActor(t, libContexts, storeContext), point: point.point}]
		if !storeOK || point.context != sameLinkContextIDForActor(t, libContexts, sendContext) {
			continue
		}
		if sendClasses[placement.SharedHeap] && storeClasses[placement.OwnedHeap] && !sameLinkPlacementClassSetsEqual(sendClasses, storeClasses) {
			divergent = true
			break
		}
	}
	if !divergent {
		t.Fatalf("same-Link shared library placement did not diverge for send/store contexts: %v", classes)
	}
	if libProgram.ContentID() != programID || product.Artifact.ID() != artifactID {
		t.Fatal("same-Link solve mutated the immutable library Program/Artifact identity")
	}
}

type sameLinkPlacementPoint struct {
	context identity.ContentID
	point   identity.ContentID
}

func sameLinkLibraryPlacementClasses(t testing.TB, plan *Plan, stateResult *result.Result, module identity.ContentID) map[sameLinkPlacementPoint]map[placement.Placement]bool {
	t.Helper()
	publication, publicationOK := placementpublication.Open(stateResult)
	if !publicationOK || publication.QueryCount() == 0 {
		t.Fatal("same-Link result did not publish placement queries")
	}
	placementSchema, schemaOK := plan.PlacementSchema()
	if !schemaOK || !placementSchema.Valid() {
		t.Fatal("same-Link placement schema unavailable")
	}
	classes := make(map[sameLinkPlacementPoint]map[placement.Placement]bool)
	for index := 0; index < publication.QueryCount(); index++ {
		query, queryOK := publication.QueryAt(index)
		if !queryOK {
			t.Fatalf("same-Link placement query[%d] unavailable", index)
		}
		contextID, contextOK := query.ContextID()
		mountID, mountOK := query.MountID()
		pointID, pointOK := query.PointID()
		if !contextOK || !mountOK || !pointOK || mountID != module || !contextID.Available() || !pointID.Available() {
			continue
		}
		decoded, decodedOK := query.Placement(placementSchema)
		if !decodedOK {
			t.Fatalf("same-Link placement query[%d] failed exact schema decode", index)
		}
		rows := decoded.Allocations()
		for {
			allocation, allocationOK := rows.Next()
			if !allocationOK {
				break
			}
			allocationKey, keyOK := placementSchema.Heap().KeyForID(allocation.AllocationID())
			if !keyOK {
				t.Fatal("same-Link placement allocation has no Heap key")
			}
			allocationModule, _, _, kind, _, originOK := placementSchema.Heap().AllocationOriginForKey(allocationKey)
			if !originOK || allocationModule != module || kind != heapdomain.AllocationTable {
				continue
			}
			class, classOK := allocation.Placement()
			if !classOK {
				continue
			}
			key := sameLinkPlacementPoint{context: contextID, point: pointID}
			pointClasses := classes[key]
			if pointClasses == nil {
				pointClasses = make(map[placement.Placement]bool)
				classes[key] = pointClasses
			}
			pointClasses[class] = true
		}
	}
	if len(classes) == 0 {
		t.Fatal("same-Link library placement family had no table allocation rows")
	}
	return classes
}

func sameLinkPlacementClassSetsEqual(left, right map[placement.Placement]bool) bool {
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

func sameLinkLibrarySites(t testing.TB, plan *Plan, module identity.ContentID) []composite.QuerySite {
	t.Helper()
	if plan == nil || plan.state == nil {
		t.Fatal("same-Link Plan unavailable")
	}
	sites := make([]composite.QuerySite, 0)
	for index := 0; index < plan.state.querySites.Count(); index++ {
		site, siteOK := plan.state.querySites.At(index)
		if !siteOK {
			t.Fatalf("same-Link query site %d is unavailable", index)
		}
		if site.Context.ModuleKey() == module {
			sites = append(sites, site)
		}
	}
	return sites
}

func sameLinkContextsForModule(t testing.TB, directory executioncontext.Directory, module identity.ContentID) []executioncontext.Context {
	t.Helper()
	contexts := make([]executioncontext.Context, 0)
	for index := 0; index < directory.ContextCount(); index++ {
		contextRow, ok := directory.ContextAt(index)
		if !ok || !contextRow.Available() {
			t.Fatalf("same-Link Context[%d] unavailable", index)
		}
		if contextRow.ModuleKey() == module {
			contexts = append(contexts, contextRow)
		}
	}
	return contexts
}

func sameLinkContextForModule(t testing.TB, directory executioncontext.Directory, module identity.ContentID) executioncontext.Context {
	t.Helper()
	contexts := sameLinkContextsForModule(t, directory, module)
	if len(contexts) != 1 {
		t.Fatalf("same-Link module %v contexts=%d, want one", module, len(contexts))
	}
	return contexts[0]
}

func sameLinkContextIDForActor(t testing.TB, contexts []executioncontext.Context, actor identity.ContentID) identity.ContentID {
	t.Helper()
	for _, contextRow := range contexts {
		if contextRow.ActorID() == actor {
			return contextRow.ID()
		}
	}
	t.Fatalf("same-Link library context missing actor %v", actor)
	return identity.ContentID{}
}

func sameLinkModuleKey(t testing.TB, linked *link.Link, name string) identity.ContentID {
	t.Helper()
	mounts := linked.Project().Mounts()
	for index := 0; index < mounts.Count(); index++ {
		shard, shardOK := mounts.At(index)
		mountName, nameOK := mounts.Name(shard)
		module, moduleOK := linked.Project().ModuleKey(shard)
		if shardOK && nameOK && moduleOK && mountName == name {
			return module
		}
	}
	t.Fatalf("same-Link module key missing %q", name)
	return identity.ContentID{}
}

func sameLinkSharedLibraryFixture(t testing.TB, target *contract.Contract) (*link.Link, *program.Program) {
	t.Helper()
	sources := map[string]string{
		"lib": `type Boxed = {
	tag: string,
	body: string,
}

local M = {}
M.Boxed = Boxed

function M.wrap(payload: string): M.Boxed
	local box: M.Boxed = { tag = "boxed", body = payload }
	return box
end
return M`,
		"consumer_send": `local lib = require("lib")
local M = {}
function M.run()
	local box: lib.Boxed = lib.wrap("payload-send")
  process.send("worker", "topic", box)
end
return M`,
		"consumer_store": `local lib = require("lib")
local M = {}
local registry = {}
function M.run()
	local box: lib.Boxed = lib.wrap("payload-store")
  table.insert(registry, box)
end
return M`,
		"main-a": `local send = require("consumer_send")
send.run()
return true`,
		"main-b": `local store = require("consumer_store")
store.run()
return true`,
	}
	programs := make(map[string]*program.Program, len(sources))
	for name, source := range sources {
		lowered, err := lower.Lower(lower.Source{Name: name + ".lua", Text: []byte(source)})
		if err != nil {
			t.Fatalf("lower same-Link %s: %v", name, err)
		}
		programs[name] = lowered
	}
	names := []string{"lib", "consumer_send", "consumer_store", "main-a", "main-b"}
	actorFor := map[string]string{
		"lib":            "both",
		"consumer_send":  "actor-a",
		"consumer_store": "actor-b",
		"main-a":         "actor-a",
		"main-b":         "actor-b",
	}
	actors := []string{"actor-a", "actor-b"}
	modules := make([]linkproject.Module, 0, len(names))
	for _, name := range names {
		modules = append(modules, linkproject.Module{Name: name, Program: programs[name]})
	}
	aliases := make([]linkmodule.ModuleCacheAliasClassSpec, 0, 8)
	roots := make([]linkmodule.AnalysisRootSpec, 0, 8)
	rootFor := make(map[string]string, 8)
	for _, name := range names {
		wantActor := actorFor[name]
		for _, actor := range actors {
			if wantActor != "both" && wantActor != actor {
				continue
			}
			instance := "instance:" + actor + ":" + name
			root := "root:" + actor + ":" + name
			aliases = append(aliases, linkmodule.ModuleCacheAliasClassSpec{Actor: actor, Instances: []string{instance}, Representative: instance})
			roots = append(roots, linkmodule.AnalysisRootSpec{Name: root, Module: name, Actor: actor, Instance: instance})
			rootFor[actor+":"+name] = root
		}
	}
	entries := make([]linkmodule.ModuleCacheEntrySpec, 0, 4)
	for _, name := range names {
		actor := actorFor[name]
		if actor == "both" {
			continue
		}
		imports := programs[name].Flow().Authored().Imports()
		for index := 0; index < imports.Count(); index++ {
			item, itemOK := imports.ImportAt(index)
			if !itemOK || item.Call == 0 || item.Request == 0 || !programs[name].Flow().Executable().Contains(item.Call) {
				continue
			}
			_, _, requested, requestedOK := programs[name].Source().Literals().Strings().At(int(keyspace.TermOrdinal(item.Request) - 1))
			if !requestedOK {
				t.Fatalf("same-Link %s import request unavailable", name)
			}
			from, fromOK := rootFor[actor+":"+name]
			to, toOK := rootFor[actor+":"+requested]
			if !fromOK || !toOK {
				t.Fatalf("same-Link %s import %q has no actor-local roots", name, requested)
			}
			entries = append(entries, linkmodule.ModuleCacheEntrySpec{Module: name, Import: item.Term, FromRoot: from, ToRoot: to})
		}
	}
	linked, err := link.Seal(&link.Spec{
		Target:  target,
		Modules: modules,
		Module: linkmodule.Spec{
			Actors:             []linkmodule.ActorSpec{{Name: actors[0]}, {Name: actors[1]}},
			ModuleCacheAliases: aliases,
			AnalysisRoots:      roots,
			ModuleCacheEntries: entries,
		},
	})
	if err != nil {
		t.Fatalf("seal same-Link shared library: %v", err)
	}
	return linked, programs["lib"]
}
