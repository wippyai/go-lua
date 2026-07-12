package readmodel

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/compiler/parse"
)

func TestColdBuildHoistableLoadBodyIDIsStable(t *testing.T) {
	const source = `
type Config = { limit: number }
local config: Config = { limit = 3 }
local total = 0
local i = 0
while i < 3 do
	total = total + config.limit
	i = i + 1
end
return total
`

	firstBodyID, firstGraphID := coldBuildHoistableLoad(t, source)
	for range 7 {
		_ = cfg.New()
	}
	secondBodyID, secondGraphID := coldBuildHoistableLoad(t, source)

	if firstGraphID == secondGraphID {
		t.Fatalf("independent builds reused graph ID %d; test cannot detect graph-order leakage", firstGraphID)
	}
	if firstBodyID != secondBodyID {
		t.Fatalf("codegen-facing hoistable-load BodyID changed across cold builds: first=%x second=%x (graph IDs %d and %d)",
			firstBodyID, secondBodyID, firstGraphID, secondGraphID)
	}
}

func coldBuildHoistableLoad(t *testing.T, source string) (lexicalidentity.StableLexicalBodyID, uint64) {
	t.Helper()
	stmts, err := parse.ParseString(source, "hoistable_load_identity_stability.lua")
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
		t.Fatalf("hoistable loads = %d, want one: %#v", len(loads), loads)
	}
	return loads[0].BodyID, result.Graph().ID()
}
