package readmodel

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/body"
	readapi "github.com/wippyai/go-lua/analysis/check/readmodel"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/compiler/parse"
)

func TestForEachHoistableLoadProjectsCleanReadAndRejectsConditionalAliasWrite(t *testing.T) {
	stmts, err := parse.ParseString(`
type Config = { limit: number }

local clean: Config = { limit = 3 }
local clean_total = 0
local i = 0
while i < 3 do
	clean_total = clean_total + clean.limit
	i = i + 1
end

local changed: Config = { limit = 3 }
local alias = changed
local changed_total = 0
local j = 0
while j < 3 do
	changed_total = changed_total + changed.limit
	if j == 1 then
		alias.limit = 9
	end
	j = j + 1
end
return clean_total + changed_total
`, "hoistable_load_readmodel.lua")
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}
	result, err := body.CheckChunk(stmts, body.Config{Registry: standard.Registry()})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}

	var loads []HoistableLoad
	New(result).ForEachHoistableLoad(func(load HoistableLoad) bool {
		loads = append(loads, load)
		return true
	})
	if len(loads) != 1 {
		t.Fatalf("hoistable loads = %d, want only clean.limit: %#v", len(loads), loads)
	}
	load := loads[0]
	if load.SchemaVersion != readapi.HoistableLoadSchemaVersion {
		t.Fatalf("schema version = %d, want %d", load.SchemaVersion, readapi.HoistableLoadSchemaVersion)
	}
	if got := load.ReadPath.String(); got != "clean.limit" {
		t.Fatalf("read path = %q, want clean.limit", got)
	}
	if load.BodyID == (lexicalidentity.StableLexicalBodyID{}) || load.Point == 0 || load.LoopHead == 0 {
		t.Fatalf("machine site/scope identifiers are incomplete: %#v", load)
	}
	if load.Point == load.LoopHead {
		t.Fatalf("read point equals loop head: %#v", load)
	}
	if !load.LoopSpan.Valid() {
		t.Fatalf("invariance witness scope has no source span: %#v", load)
	}
}

// A typed cast does not make a metatable-backed member read a raw table load:
// __index can return a different value and have side effects on every access.
// Keep the advice stream and its codegen projection in lockstep on this
// soundness boundary.
func TestInvariantLoopReadAndHoistableLoadRejectStatefulMetatableRead(t *testing.T) {
	stmts, err := parse.ParseString(`
type Config = { limit: number }

local ticks = 0
local clean = setmetatable({}, {
	__index = function(_self, _key): number
		ticks = ticks + 1
		return ticks
	end,
}) :: Config
local total = 0
local i = 0
while i < 3 do
	total = total + clean.limit
	i = i + 1
end
return total
`, "metamethod-hoist-soundness.lua")
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}
	result, err := body.CheckChunk(stmts, body.Config{Registry: standard.Registry()})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}
	var advice []InvariantLoopRead
	New(result).ForEachInvariantLoopRead(func(read InvariantLoopRead) bool {
		advice = append(advice, read)
		return true
	})
	if len(advice) != 0 {
		t.Fatalf("invariant loop reads = %#v, want none for metatable-backed clean.limit", advice)
	}

	var loads []HoistableLoad
	New(result).ForEachHoistableLoad(func(load HoistableLoad) bool {
		loads = append(loads, load)
		return true
	})
	if len(loads) != 0 {
		t.Fatalf("hoistable loads = %#v, want none for metatable-backed clean.limit", loads)
	}
}

func TestInvariantLoopReadAndHoistableLoadRequireNoIndexMetatableWitness(t *testing.T) {
	stmts, err := parse.ParseString(`
type Config = { limit: number }

local clean = setmetatable({ limit = 3 }, {
	__index = function(_self, _key): number
		return 0
	end,
}) :: Config
local total = 0
local i = 0
while i < 3 do
	total = total + clean.limit
	i = i + 1
end
return total
`, "metatable-member-hoist-soundness.lua")
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}
	result, err := body.CheckChunk(stmts, body.Config{Registry: standard.Registry()})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}
	var loads []HoistableLoad
	New(result).ForEachHoistableLoad(func(load HoistableLoad) bool {
		loads = append(loads, load)
		return true
	})
	if len(loads) != 0 {
		t.Fatalf("hoistable loads = %#v, want none without a no-__index witness", loads)
	}
}
