package readmodel

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/body"
	readapi "github.com/wippyai/go-lua/analysis/check/readmodel"
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
	if load.BodyID == 0 || load.Point == 0 || load.LoopHead == 0 {
		t.Fatalf("machine site/scope identifiers are incomplete: %#v", load)
	}
	if load.Point == load.LoopHead {
		t.Fatalf("read point equals loop head: %#v", load)
	}
	if !load.LoopSpan.Valid() {
		t.Fatalf("invariance witness scope has no source span: %#v", load)
	}
}
