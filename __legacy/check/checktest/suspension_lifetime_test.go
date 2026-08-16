package checktest

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/placementplan"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
	"github.com/wippyai/go-lua/analysis/module/manifest"
	"github.com/wippyai/go-lua/analysis/module/signature"
)

func TestDiesBeforeSuspensionPureHelperChain(t *testing.T) {
	result := Check(`
local function helper(v: { value: integer }): integer
	return v.value
end

local function wrapper(v: { value: integer }): integer
	return helper(v)
end

local data = { value = 1 }
local out: integer = wrapper(data)
`)
	requireCleanSuspensionLifetimeCheck(t, result)
	assertDiesBeforeSuspensionCount(t, result.PlacementPlan(), 1)
}

func TestDiesBeforeSuspensionChannelReceiveBetweenBirthAndUse(t *testing.T) {
	result := Check(`
type Message = { value: integer }

local function route(ch: Channel<Message>): integer
	local data = { value = 1 }
	local received, ok = ch:receive()
	return data.value
end

local out: integer = route(nil :: Channel<Message>)
`, WithStdlib())
	requireCleanSuspensionLifetimeCheck(t, result)
	assertDiesBeforeSuspensionCount(t, result.PlacementPlan(), 0)
}

func TestDiesBeforeSuspensionTransitiveSuspendingHelper(t *testing.T) {
	result := Check(`
local function helper()
	coroutine.yield()
end

local function run(): integer
	local data = { value = 1 }
	helper()
	return data.value
end

local out: integer = run()
`, WithStdlib())
	requireCleanSuspensionLifetimeCheck(t, result)
	assertDiesBeforeSuspensionCount(t, result.PlacementPlan(), 0)
}

func TestDiesBeforeSuspensionSuspensionAfterLastUse(t *testing.T) {
	result := Check(`
local function run(): integer
	local data = { value = 1 }
	local out: integer = data.value
	coroutine.yield()
	return out
end

local out: integer = run()
`, WithStdlib())
	requireCleanSuspensionLifetimeCheck(t, result)
	assertDiesBeforeSuspensionCount(t, result.PlacementPlan(), 1)
}

func TestDiesBeforeSuspensionCoroutineWrapBetweenBirthAndUse(t *testing.T) {
	result := Check(`
local function gen()
	coroutine.yield(1)
end

local function run(): integer
	local data = { value = 1 }
	local iter = coroutine.wrap(gen)
	return data.value
end

local out: integer = run()
`, WithStdlib())
	requireCleanSuspensionLifetimeCheck(t, result)
	assertDiesBeforeSuspensionCount(t, result.PlacementPlan(), 0)
}

func TestDiesBeforeSuspensionUncertifiedHostCallBetweenBirthAndUse(t *testing.T) {
	host := manifest.New("host")
	host.DefineGlobalType("host", typ.Func().Build())
	// This is the legacy manifest shape: it has a resolved signature but no
	// operational suspension certification.
	host.DefineFunctionSignature("host", signature.Function{Type: typ.Func().Build()})
	result := Check(`
local function run(): integer
	local data = { value = 1 }
	host()
	return data.value
end

local out: integer = run()
	`, WithManifest("host", host))
	requireCleanSuspensionLifetimeCheck(t, result)
	assertDiesBeforeSuspensionCount(t, result.PlacementPlan(), 0)
	assertNoFrameLocalAllocation(t, result.PlacementPlan())
}

func TestDiesBeforeSuspensionValueUsedAfterSelectProbe(t *testing.T) {
	result := Check(`
local channel = require("channel")

type Message = { value: integer }

local function route(ch: Channel<Message>): integer
	local data = { value = 1 }
	local selected = channel.select {
		ch:case_receive(),
	}
	local out: integer = data.value
	return out
end

local out: integer = route(nil :: Channel<Message>)
`, WithStdlib(), WithManifest("channel", ChannelManifest()))
	requireCleanSuspensionLifetimeCheck(t, result)
	assertDiesBeforeSuspensionCount(t, result.PlacementPlan(), 0)
}

func TestMaySuspendExportsThroughFunctionManifest(t *testing.T) {
	mod := CheckAndExport(`
local M = {}

function M.run()
	coroutine.yield()
end

return M
`, "suspension", WithStdlib())
	if len(mod.Errors) != 0 {
		t.Fatalf("module errors = %#v, want clean", mod.Errors)
	}
	sig, ok := mod.Manifest.FunctionSignatures["suspension.run"]
	if !ok {
		t.Fatalf("missing suspension.run function signature: %#v", mod.Manifest.FunctionSignatures)
	}
	if sig.OperationalEffects == nil || !sig.OperationalEffects.SuspensionKnown || !sig.OperationalEffects.MaySuspend {
		t.Fatalf("suspension export = %#v, want certified may-suspend", sig.OperationalEffects)
	}
	data, err := manifest.Encode(mod.Manifest)
	if err != nil {
		t.Fatalf("encode manifest: %v", err)
	}
	decoded, err := manifest.Decode(data)
	if err != nil {
		t.Fatalf("decode manifest: %v", err)
	}
	decodedSig, ok := decoded.FunctionSignatures["suspension.run"]
	if !ok || decodedSig.OperationalEffects == nil || !decodedSig.OperationalEffects.SuspensionKnown || !decodedSig.OperationalEffects.MaySuspend {
		t.Fatalf("decoded suspension export = %#v, want certified may-suspend", decodedSig.OperationalEffects)
	}
}

func requireCleanSuspensionLifetimeCheck(t *testing.T, result Result) {
	t.Helper()
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want clean", result.Diagnostics)
	}
}

func assertDiesBeforeSuspensionCount(t *testing.T, plan placementplan.Plan, want int) {
	t.Helper()
	got := 0
	for _, entry := range plan.Entries {
		if entry.HasDiesBeforeSuspension && entry.DiesBeforeSuspension {
			got++
		}
	}
	if got != want {
		t.Fatalf("dies-before-suspension count = %d, want %d; entries=%#v", got, want, plan.Entries)
	}
}

func assertNoFrameLocalAllocation(t *testing.T, plan placementplan.Plan) {
	t.Helper()
	for _, entry := range plan.Entries {
		if entry.AllocationSite && entry.FrameLocal {
			t.Fatalf("uncertified call licensed frame-local allocation: %#v", entry)
		}
	}
}
