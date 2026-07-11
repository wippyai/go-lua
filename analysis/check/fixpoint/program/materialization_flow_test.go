package program

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/ref"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/module/importlookup"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/compiler/ast"
)

func TestCanceledMaterializationReturnsPromptly(t *testing.T) {
	stmts := parseChunk(t, `local value = 1`)
	reg := standard.Registry()
	bindings := bind.BindChunk(stmts, bind.Options{})
	config := body.Config{Registry: reg}
	keys := collectKeys(bindings, summary.DefaultSummaryKey(ref.Root()), reg, nil, importlookup.Source{}, stmts)
	prepared, err := prepareBoundChunkBodies(stmts, bindings, config, keys)
	if err != nil {
		t.Fatalf("prepareBoundChunkBodies: %v", err)
	}
	root, err := solvePrepared(prepared.root, config)
	if err != nil {
		t.Fatalf("solvePrepared: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	config.Context = ctx
	started := time.Now()
	_, _, err = materializeFunctionTree(root, nil, prepared, bindings, config, nil, nil, nil, nil, keys, nil, nil, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("materializeFunctionTree error = %v, want context cancellation", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("canceled materialization took %s, want prompt return", elapsed)
	}
}

func TestCollectMaterializedCallContextKeysStableAcrossBaseResultMapOrder(t *testing.T) {
	stmts := parseChunk(t, `
local function alpha(value: number): number
	return value + 1
end

local function beta(value: number): number
	return value * 2
end

local function gamma(value: number): number
	return value - 1
end

local function first(): number
	return alpha(1) + alpha(2) + beta(3) + beta(4) + gamma(5) + gamma(6)
end

local function second(): number
	return alpha(7) + alpha(8) + beta(9) + beta(10) + gamma(11) + gamma(12)
end

local function third(): number
	return alpha(13) + alpha(14) + beta(15) + beta(16) + gamma(17) + gamma(18)
end
`)
	reg := standard.Registry()
	bindings := bind.BindChunk(stmts, bind.Options{})
	config := body.Config{Registry: reg}

	var want []summary.SummaryKey
	for run := 0; run < 12; run++ {
		keys := collectKeys(bindings, summary.DefaultSummaryKey(ref.Root()), reg, nil, importlookup.Source{}, stmts)
		prepared, err := prepareBoundChunkBodies(stmts, bindings, config, keys)
		if err != nil {
			t.Fatalf("run %d prepareBoundChunkBodies: %v", run, err)
		}
		baseResults := make(map[*ast.FunctionExpr]*body.Result, len(keys.functions))
		for offset := range keys.functions {
			index := offset
			if run%2 != 0 {
				index = len(keys.functions) - 1 - offset
			}
			origin := keys.functions[index]
			result, err := solvePrepared(prepared.function(origin.funcExpr), config)
			if err != nil {
				t.Fatalf("run %d solve %v: %v", run, origin.key, err)
			}
			baseResults[origin.funcExpr] = result
		}
		if _, err := collectMaterializedCallContextKeys(&keys, nil, baseResults, config); err != nil {
			t.Fatalf("run %d collectMaterializedCallContextKeys: %v", run, err)
		}
		got := materializedContextKeys(keys.contexts)
		if len(got) != 3 {
			t.Fatalf("run %d semantic contexts = %d, want 3 (alpha/beta/gamma)", run, len(got))
		}
		if run == 0 {
			want = got
			continue
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("run %d materialized context keys changed\nwant: %#v\n got: %#v", run, want, got)
		}
	}
}

func materializedContextKeys(contexts contextIndex) []summary.SummaryKey {
	entries := contexts.Entries()
	keys := make([]summary.SummaryKey, 0, len(entries))
	for _, entry := range entries {
		keys = append(keys, entry.key)
	}
	return keys
}
