package engine_test

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/engine"
	"github.com/wippyai/go-lua/analysis/check/lint"
	"github.com/wippyai/go-lua/analysis/domain/placement"
)

func TestCheckPublishesClosedPlacementFacts(t *testing.T) {
	result, err := engine.Check(`local scratch = { a = 1, b = 2 }
local total = scratch.a + scratch.b
return total`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if result.Placement == nil || !result.Placement.Complete || len(result.Placement.Allocations) != 1 {
		t.Fatalf("placement = %#v, want one complete allocation", result.Placement)
	}
	item := result.Placement.Allocations[0]
	if item.Kind != "lua.table" || item.Placement != placement.Stack || !item.Decomposable || !item.FrameLocal || !item.DiesBeforeSuspension {
		t.Fatalf("allocation = %#v, want closed frame-local table", item)
	}
}

func TestCheckPlacementUsesPublishedExternalOwnershipSend(t *testing.T) {
	result, err := engine.Check(`local payload = { meta = { route = "worker" } }
coroutine.resume(nil, payload)`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if result.Placement == nil || len(result.Placement.Allocations) != 2 {
		t.Fatalf("placement = %#v, want the two payload allocations", result.Placement)
	}
	for _, item := range result.Placement.Allocations {
		if item.Placement != placement.SharedHeap || len(item.Blockers) != 0 {
			t.Fatalf("allocation = %#v, want the externally contracted shared graph", item)
		}
	}
}

func TestCheckPlacementUsesPublishedExternalCoroutineSend(t *testing.T) {
	result, err := engine.Check(`local callback = function() end
coroutine.create(callback)`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if result.Placement == nil || len(result.Placement.Allocations) != 1 {
		t.Fatalf("placement = %#v, want one callback allocation", result.Placement)
	}
	item := result.Placement.Allocations[0]
	if item.Placement != placement.SharedHeap || len(item.Blockers) != 0 {
		t.Fatalf("allocation = %#v, want the callback transferred to the coroutine", item)
	}
}

func TestCheckPlacementUsesPublishedExternalOwnershipRetain(t *testing.T) {
	result, err := engine.Check(`local object = {}
local meta = {}
setmetatable(object, meta)`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if result.Placement == nil || len(result.Placement.Allocations) != 2 {
		t.Fatalf("placement = %#v, want object and metatable allocations", result.Placement)
	}
	for _, item := range result.Placement.Allocations {
		if item.Placement != placement.OwnedHeap {
			continue
		}
		if !item.OwnerIdentity || len(item.Blockers) != 0 {
			t.Fatalf("allocation = %#v, want the externally retained metatable", item)
		}
		return
	}
	t.Fatal("placement omitted the metatable allocation")
}

func TestCheckAcceptsSealedEmptyMapAtDeclaredBoundary(t *testing.T) {
	result, err := engine.Check(`local registry: {[string]: {value: string}} = {}`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want the sealed empty map to prove the declaration", result.Diagnostics)
	}
}

func TestCheckPlacementFailsClosedAtOpaqueCall(t *testing.T) {
	result, err := engine.Check(`local value = { a = 1 }
unknown(value)`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if result.Placement == nil || len(result.Placement.Allocations) != 1 {
		t.Fatalf("placement = %#v, want one allocation", result.Placement)
	}
	item := result.Placement.Allocations[0]
	if item.Placement != placement.Unknown || len(item.Blockers) != 1 || item.Blockers[0] != "opaque-call" {
		t.Fatalf("allocation = %#v, want opaque-call blocker", item)
	}
}

func TestCheckPlacementPromotesReturnedAllocation(t *testing.T) {
	result, err := engine.Check(`local value = { a = 1 }
return value`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if result.Placement == nil || len(result.Placement.Allocations) != 1 {
		t.Fatalf("placement = %#v, want one allocation", result.Placement)
	}
	item := result.Placement.Allocations[0]
	if item.Placement != placement.OwnedHeap || !item.OwnerIdentity || item.Decomposable || item.FrameLocal {
		t.Fatalf("allocation = %#v, want returned owned allocation", item)
	}
}

func TestCheckPublishesDeclaredClosureReturnWitnesses(t *testing.T) {
	result, err := engine.Check(`type Upload = { size: number }
type View = { human_size: string }
local function materialize(upload: Upload): View
  local kilobytes: number = upload.size / 1024
  return { human_size = tostring(kilobytes) }
end
return materialize`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if result.Placement == nil {
		t.Fatal("placement = nil, want declared closure return witnesses")
	}
	ownedTable, stackScalar := false, false
	for _, item := range result.Placement.Allocations {
		ownedTable = ownedTable || (item.Kind == "lua.table" && item.Placement == placement.OwnedHeap && item.OwnerIdentity)
		stackScalar = stackScalar || (item.Kind == "lua.scalar" && item.Placement == placement.Stack && item.FrameLocal)
	}
	if !ownedTable || !stackScalar {
		t.Fatalf("placement = %#v, want owned table and stack scalar witnesses", result.Placement)
	}
}

func TestCheckPublishesReturnedClosureStoreWitnesses(t *testing.T) {
	result, err := engine.Check(`type Event = { key: string }
local pending: {[string]: Event} = {}
local function retain(event: Event)
  pending[event.key] = event
end
return retain`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if result.Placement == nil {
		t.Fatal("placement = nil, want returned closure store witnesses")
	}
	owned, shared := false, false
	for _, item := range result.Placement.Allocations {
		owned = owned || (item.Kind == "lua.table" && item.Placement == placement.OwnedHeap && item.OwnerIdentity)
		shared = shared || (item.Kind == "lua.table" && item.Placement == placement.SharedHeap)
	}
	if !owned || !shared {
		t.Fatalf("placement = %#v, want owned capture and shared formal witnesses", result.Placement)
	}
}

func TestCheckPublishesCyclicStackPlacementWitness(t *testing.T) {
	result, err := engine.Check(`type Message = { id: string }
local cache: {[string]: Message} = {}
local function collect(batch: {Message})
  for _, message in ipairs(batch) do
    local scratch = { id = message.id }
    cache[message.id] = message
    print(scratch.id)
  end
end
return collect`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if result.Placement == nil {
		t.Fatal("placement = nil, want cyclic stack witness")
	}
	for _, item := range result.Placement.Allocations {
		if item.Kind == "lua.table" && item.Placement == placement.Stack && item.Complete {
			return
		}
	}
	t.Fatalf("placement = %#v, want a complete stack table from the cyclic child", result.Placement)
}

func TestCheckPlacementRetainsLexicalClosureCapture(t *testing.T) {
	result, err := engine.Check(`local state = { value = 1 }
local read = function() return state.value end
local value = read()`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if result.Placement == nil || len(result.Placement.Allocations) != 2 {
		t.Fatalf("placement = %#v, want closure and captured allocation", result.Placement)
	}
	for _, item := range result.Placement.Allocations {
		if item.Placement != placement.OwnedHeap || !item.OwnerIdentity || item.FrameLocal {
			t.Fatalf("allocation = %#v, want retained lexical closure graph", item)
		}
	}
}

// A closure that only its own frame invokes is heap resident for that
// invocation alone, while a closure stored into a returned graph is retained by
// it. Both are owned; only the first is a live environment.
func TestCheckPlacementSeparatesLiveEnvironmentFromRetainedClosure(t *testing.T) {
	invoked, err := engine.Check(`local state = { value = 1 }
local read = function() return state.value end
local value = read()`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	for _, item := range invoked.Placement.Allocations {
		if (item.Kind == "lua.closure") != item.LiveEnvironment {
			t.Fatalf("allocation = %#v, want live environment on the invoked closure only", item)
		}
	}
	retained, err := engine.Check(`local state = { value = 1 }
local read = function() return state.value end
local holder = { read = read }
return holder`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	closures := 0
	for _, item := range retained.Placement.Allocations {
		if item.Kind != "lua.closure" {
			continue
		}
		closures++
		if item.Placement != placement.OwnedHeap || item.LiveEnvironment {
			t.Fatalf("allocation = %#v, want a retained closure without live-environment residency", item)
		}
	}
	if closures == 0 {
		t.Fatalf("placement = %#v, want the stored closure allocation", retained.Placement)
	}
}

// The declared-return fast path publishes no child value facts, but the graph a
// local body allocates and stores into an escaping container still joins upward
// through containment at its caller.
func TestCheckPlacementLiftsDeclaredLocalBodyAllocationGraph(t *testing.T) {
	result, err := engine.Check(`type Row = { id: string, meta: { source: string } }
local cache: {[string]: Row} = {}
process.send("worker-1", "cache.ready", cache)
local function build(id: string): Row
    local row: Row = { id = id, meta = { source = "builder" } }
    cache[row.id] = row
    return row
end
local row = build("x")
print(row.id)`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if result.Placement == nil {
		t.Fatal("placement = nil, want the callee allocation graph")
	}
	shared, depth := 0, 0
	for _, item := range result.Placement.Allocations {
		if item.Kind != "lua.table" || item.Placement != placement.SharedHeap {
			continue
		}
		shared++
		if item.Depth > depth {
			depth = item.Depth
		}
	}
	if shared != 3 || depth < 3 {
		t.Fatalf("placement = %#v, want the cache, row and meta tables shared through containment", result.Placement)
	}
}

func TestCheckPlacementCarriesAliasIntoSharedContainer(t *testing.T) {
	result, err := engine.Check(`local shared = { label = "shared" }
process.send("worker", "ready", shared)
local payload = { id = "payload" }
local alias = payload
shared.payload = alias`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if result.Placement == nil || len(result.Placement.Allocations) != 2 {
		t.Fatalf("placement = %#v, want two allocations", result.Placement)
	}
	for _, item := range result.Placement.Allocations {
		if item.Placement != placement.SharedHeap || !item.SealBeforeShare {
			t.Fatalf("allocation = %#v, want sealed shared placement", item)
		}
	}
}

// TestCheckPlacementOwnershipStoreRetainsOwnerAndStoredGraph pins the two
// halves of the ownership.store contract apart. The owner graph is three
// tables and the stored graph is two, so the counts identify each side. Both
// sides are retained -- neither keeps the opaque-call blocker -- but only the
// stored graph outlives the caller frame and acquires an owner obligation.
func TestCheckPlacementOwnershipStoreRetainsOwnerAndStoredGraph(t *testing.T) {
	result, err := engine.Check(`local box = { items = {}, index = {} }
local item = { child = { route = "owned" } }
ownership.store(item, box)`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if result.Placement == nil || !result.Placement.Complete || len(result.Placement.Allocations) != 5 {
		t.Fatalf("placement = %#v, want the complete five-allocation plan", result.Placement)
	}
	owner, stored, ownedDepth := 0, 0, 0
	for _, item := range result.Placement.Allocations {
		if !item.Complete || len(item.Blockers) != 0 {
			t.Fatalf("allocation = %#v, want a complete allocation with no blocker", item)
		}
		switch item.Placement {
		case placement.Stack:
			if item.OwnerIdentity {
				t.Fatalf("allocation = %#v, want the owner graph to keep its frame placement", item)
			}
			owner++
		case placement.OwnedHeap:
			if !item.OwnerIdentity {
				t.Fatalf("allocation = %#v, want an owner obligation on the stored graph", item)
			}
			if item.Depth > ownedDepth {
				ownedDepth = item.Depth
			}
			stored++
		default:
			t.Fatalf("allocation = %#v, want stack or owned-heap placement", item)
		}
	}
	if owner != 3 || stored != 2 || ownedDepth != 2 {
		t.Fatalf("owner=%d stored=%d owned depth=%d, want the three-table owner and the two-table stored graph", owner, stored, ownedDepth)
	}
}

// checkModulesPlacement runs the project checker over modules given in
// dependency order and returns the published placement plan. The last module is
// the target, matching the fixture corpus layout.
func checkModulesPlacement(t *testing.T, order []string, sources map[string]string, packages ...string) *engine.PlacementPlan {
	t.Helper()
	entries := make([]lint.Entry, 0, len(order))
	for _, name := range order {
		entries = append(entries, lint.Entry{Path: name + ".lua", ModulePath: name, Source: sources[name]})
	}
	input := lint.ProjectInput{Entries: entries, Targets: []string{order[len(order)-1]}}
	for _, pkg := range packages {
		input.Manifests = append(input.Manifests, fixtureHostManifest(pkg))
	}
	result, err := lint.CheckProject(context.Background(), input)
	if err != nil {
		t.Fatalf("CheckProject: %v", err)
	}
	return result.Placement
}

const placementBoxLibraryModule = `
type Boxed = {
    tag: string,
    body: string,
}

local M = {}
M.Boxed = Boxed

function M.wrap(payload: string): M.Boxed
    local box: M.Boxed = { tag = "boxed", body = payload }
    return box
end

return M
`

// placementSharedTables counts the sent graph of the shared library: a table
// site placed on the shared heap with its sealing proof already established.
func placementSharedTables(plan *engine.PlacementPlan) int {
	count := 0
	for _, item := range plan.Allocations {
		if item.Placement == placement.SharedHeap && item.Kind == "lua.table" && item.SealBeforeShare {
			count++
		}
	}
	return count
}

// TestCheckPlacementDivergesPerConsumerOfSharedLibrary proves one library
// allocation site reaches a different placement in each consumer that uses it.
// The consumer bodies are never invoked inside their own module, so the graph
// each one materializes is established from the imported relation alone; the
// send is the only boundary that promotes it past actor-local ownership.
func TestCheckPlacementDivergesPerConsumerOfSharedLibrary(t *testing.T) {
	plan := checkModulesPlacement(t, []string{"lib", "reader", "sender", "main"}, map[string]string{
		"lib": placementBoxLibraryModule,
		"reader": `local lib = require("lib")

local M = {}

function M.run(): string
    local box: lib.Boxed = lib.wrap("payload-read")
    return box.body
end

return M
`,
		"sender": `local lib = require("lib")

local M = {}

function M.run()
    local box: lib.Boxed = lib.wrap("payload-send")
    process.send("worker", "topic", box)
end

return M
`,
		"main": `local reader = require("reader")
local sender = require("sender")

sender.run()
print(reader.run())
`,
	}, "process")
	if plan == nil || !plan.Complete {
		t.Fatalf("placement = %#v, want a complete plan", plan)
	}
	shared, owned := 0, 0
	for _, item := range plan.Allocations {
		switch {
		case item.Placement == placement.SharedHeap:
			if !item.SealBeforeShare || item.Kind != "lua.table" || len(item.Obligations) != 0 {
				t.Fatalf("allocation = %#v, want the sent table sealed before it is shared", item)
			}
			shared++
		case item.Placement == placement.OwnedHeap && item.Kind == "lua.table" && item.Target == "":
			// The library's published return template: the graph every consumer
			// materializes, before any consumer-specific boundary applies.
			owned++
		}
		if item.Placement == placement.Unknown {
			t.Fatalf("allocation = %#v, want no unknown placement", item)
		}
	}
	if shared != 1 || owned == 0 {
		t.Fatalf("shared=%d library templates=%d in %#v, want exactly the sender's graph shared", shared, owned, plan.Allocations)
	}
}

// TestCheckPlacementSeparatesSiblingImportedReturnGraphs proves two bodies of
// the same module each materialize their own graph from the same library call.
// Both calls occupy the same operation slot inside their own body, so an
// identity keyed by the application alone would alias them and hand the read-only
// graph the sender's sharing and sealing proofs.
func TestCheckPlacementSeparatesSiblingImportedReturnGraphs(t *testing.T) {
	plan := checkModulesPlacement(t, []string{"lib", "consumer", "main"}, map[string]string{
		"lib": placementBoxLibraryModule,
		"consumer": `local lib = require("lib")

local M = {}

function M.emit()
    local sent: lib.Boxed = lib.wrap("sent")
    process.send("worker", "topic", sent)
end

function M.read(): string
    local kept: lib.Boxed = lib.wrap("kept")
    return kept.body
end

return M
`,
		"main": `local consumer = require("consumer")

consumer.emit()
print(consumer.read())
`,
	}, "process")
	if plan == nil || !plan.Complete {
		t.Fatalf("placement = %#v, want a complete plan", plan)
	}
	owned := 0
	for _, item := range plan.Allocations {
		if item.Placement == placement.OwnedHeap && item.Kind == "lua.table" {
			owned++
		}
	}
	// The sent graph, the read graph, the consumer module table, the library
	// module table, and the library return template are five distinct sites.
	// An aliased identity would merge the two library graphs into one.
	if shared, want := placementSharedTables(plan), 4; shared != 1 || owned != want {
		t.Fatalf("shared=%d owned tables=%d (want 1 and %d) in %#v", shared, owned, want, plan.Allocations)
	}
}

// TestCheckPlacementWithholdsForwardedGraphFromUncalledBody proves a body that
// returns the graph of an imported call publishes no site of its own for it.
// The forwarder's frame carries the graph in transit only; the caller's own call
// site and the module's published return relation already place it, so a second
// caller-less site would count one runtime object twice.
func TestCheckPlacementWithholdsForwardedGraphFromUncalledBody(t *testing.T) {
	plan := checkModulesPlacement(t, []string{"lib", "pass", "main"}, map[string]string{
		"lib": placementBoxLibraryModule,
		"pass": `local lib = require("lib")

type Boxed = lib.Boxed

local M = {}
M.Boxed = Boxed

function M.build(payload: string): Boxed
    return lib.wrap(payload)
end

return M
`,
		"main": `local pass = require("pass")

local box: pass.Boxed = pass.build("payload")
print(box.body)
`,
	})
	if plan == nil || !plan.Complete {
		t.Fatalf("placement = %#v, want a complete plan", plan)
	}
	for _, item := range plan.Allocations {
		if item.Placement == placement.Unknown {
			t.Fatalf("allocation = %#v, want no unknown placement", item)
		}
	}
	// The whole plan is the library module table and its published return
	// relation, the forwarder module table and its own published relation, and
	// the entry module's instantiation of that relation. The forwarder body
	// carries the graph in transit only and contributes no sixth site.
	if len(plan.Allocations) != 5 {
		t.Fatalf("allocations = %d in %#v, want no site for the graph in transit", len(plan.Allocations), plan.Allocations)
	}
}
