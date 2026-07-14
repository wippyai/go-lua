package program

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestPhaseCallbackSummaryPropagatesEntryRekeyFailure(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `local function callback() return "ok" end`)
	bindings := bind.BindChunk(stmts, bind.Options{})
	keys := collectKeys(bindings, rootKey(summary.SummaryKey{}), reg, nil, body.Config{}.ModuleExports, stmts)
	if len(keys.functions) != 1 || keys.functions[0].funcExpr == nil {
		t.Fatalf("function census = %#v, want one callback", keys.functions)
	}
	prepared, err := prepareBoundChunkBodies(stmts, bindings, body.Config{Registry: reg}, keys)
	if err != nil {
		t.Fatal(err)
	}
	prepass, err := solvePrepared(prepared.root, body.Config{Registry: reg})
	if err != nil {
		t.Fatal(err)
	}
	tracker := &callbackPhaseTracker{
		keys:     &keys,
		prepass:  prepass,
		config:   body.Config{Registry: reg},
		prepared: prepared,
	}
	invalidValue := *keyspace.New()
	invalid := &invalidValue
	if invalid.Valid() {
		t.Fatal("shallow keyspace copy unexpectedly retained authority")
	}

	_, ok, err := tracker.phaseCallbackSummary(
		cfg.Point(1),
		registeredPhaseCallback{fn: keys.functions[0].funcExpr},
		state.State{},
		invalid,
	)
	if err == nil {
		t.Fatal("callback entry rekey failure was reported as an absent summary")
	}
	if ok {
		t.Fatal("failed callback solve was reported as a present summary")
	}
}
