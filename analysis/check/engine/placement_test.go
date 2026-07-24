package engine_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/engine"
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

func TestCheckPlacementOwnershipStoreRetainsOwnerAndStoredGraph(t *testing.T) {
	result, err := engine.Check(`local box = { items = {} }
local item = { child = { route = "owned" } }
ownership.store(item, box)`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if result.Placement == nil || len(result.Placement.Allocations) != 4 {
		t.Fatalf("placement = %#v, want four allocations", result.Placement)
	}
	for _, item := range result.Placement.Allocations {
		if item.Placement != placement.OwnedHeap || !item.OwnerIdentity {
			t.Fatalf("allocation = %#v, want owned placement", item)
		}
	}
}
