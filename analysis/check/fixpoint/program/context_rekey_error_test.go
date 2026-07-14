package program

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestCollectCallContextKeysPropagatesDefinitionCaptureRekeyFailure(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
local captured = {}
local function callback() return captured end
return callback
`)
	bindings := bind.BindChunk(stmts, bind.Options{})
	keys := collectKeys(bindings, rootKey(summary.SummaryKey{}), reg, nil, body.Config{}.ModuleExports, stmts)
	if len(keys.functions) != 1 {
		t.Fatalf("function census = %d, want one callback", len(keys.functions))
	}
	invalidValue := *keyspace.New()
	keys.functions[0].entryState = state.State{}
	keys.functions[0].entryKeys = &invalidValue
	keys.functions[0].hasEntryState = true
	prepared, err := prepareBoundChunkBodies(stmts, bindings, body.Config{Registry: reg}, keys)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := collectCallContextKeys(&keys, stmts, bindings, body.Config{Registry: reg}, nil, prepared); err == nil {
		t.Fatal("definition-capture rekey failure was swallowed by context collection")
	}
}
