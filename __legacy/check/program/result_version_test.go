package program

import (
	"reflect"
	"sort"
	"strconv"
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/lua/bind"
)

func TestResultVersionsTrackBodyAndConsumedSummaryInputs(t *testing.T) {
	before := resultVersionsByName(t, `
local function leaf(flag)
	if flag then
		return "ok"
	end
	return nil
end

local function middle(flag)
	return leaf(flag)
end

local function caller(flag)
	return middle(flag)
end

local function unrelated(flag)
	if flag then
		return 1
	end
	return 2
end

return caller(true), unrelated(false)
`)
	after := resultVersionsByName(t, `
local function leaf(flag)
	if flag then
		return 42
	end
	return nil
end

local function middle(flag)
	return leaf(flag)
end

local function caller(flag)
	return middle(flag)
end

local function unrelated(flag)
	if flag then
		return 1
	end
	return 2
end

return caller(true), unrelated(false)
`)

	assertVersionChanged(t, before, after, "leaf")
	assertVersionChanged(t, before, after, "middle")
	assertVersionChanged(t, before, after, "caller")
	assertVersionChanged(t, before, after, "chunk")
	assertVersionUnchanged(t, before, after, "unrelated")
}

func TestRecursiveSCCResultVersionsAreStableAcrossIndependentSolves(t *testing.T) {
	src := `
local function recurse(flag)
	if flag then
		return recurse(false)
	end
	return "ok"
end

return recurse(true)
`
	first := resultVersionsByName(t, src)
	second := resultVersionsByName(t, src)
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("recursive SCC result versions are unstable\nfirst:  %v\nsecond: %v", first, second)
	}
	if _, ok := first["recurse"]; !ok {
		t.Fatalf("recursive SCC did not publish its lexical body; have %v", sortedVersionNames(first))
	}
	if len(first) != 2 {
		t.Fatalf("recursive SCC published %d result versions, want exactly chunk and recurse", len(first))
	}
}

func TestResultVersionsIgnoreCommentsOutsideBodies(t *testing.T) {
	before := resultVersionsByName(t, `
local function leaf(flag)
	if flag then
		return "ok"
	end
	return nil
end

local function caller(flag)
	return leaf(flag)
end

return caller(true)
`)
	after := resultVersionsByName(t, `
local function leaf(flag)
	if flag then
		return "ok"
	end
	return nil
end

-- outside any function body; this must not invalidate body input digests.
local function caller(flag)
	return leaf(flag)
end

return caller(true)
`)
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("comment-only edit outside bodies changed result versions\nbefore: %v\nafter:  %v", before, after)
	}
}

func TestResultVersionsDeterministicAcrossIndependentSolves(t *testing.T) {
	src := `
local function leaf(flag)
	if flag then
		return "ok"
	end
	return nil
end

local function caller(flag)
	return leaf(flag)
end

return caller(true)
`
	first := resultVersionsByName(t, src)
	second := resultVersionsByName(t, src)
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("same inputs produced different result versions\nfirst:  %v\nsecond: %v", first, second)
	}
}

func TestCanonicalResultVersionSurvivesTypedLineage(t *testing.T) {
	src := `
local function leaf(flag)
	if flag then
		return "ok"
	end
	return nil
end

local function caller(flag)
	return leaf(flag)
end

	return caller(true)
	`
	first := resultVersionsByName(t, src)
	second := resultVersionsByName(t, src)
	wantNames := []string{"caller", "chunk", "leaf"}
	if got := sortedVersionNames(first); !reflect.DeepEqual(got, wantNames) {
		t.Fatalf("canonical ResultVersion bodies = %v, want %v", got, wantNames)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("typed lineage ResultVersions are unstable\nfirst:  %#v\nsecond: %#v", first, second)
	}
}

func TestHighFanoutLexicalBodiesHaveStableKeysOrderAndVersions(t *testing.T) {
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

return first() + second() + third()
`)
	bindings := bind.BindChunk(stmts, bind.Options{})
	stats := &Stats{}
	config := Config{Check: body.Config{Registry: standard.Registry()}, Stats: stats}

	var wantKeys []summary.SummaryKey
	var wantBodies []materializedBodyVersion
	for run := 0; run < 12; run++ {
		result, err := RunBoundChunk(stmts, bindings, config)
		if err != nil {
			t.Fatalf("run %d RunBoundChunk: %v", run, err)
		}
		gotKeys := resultSummaryKeys(result)
		gotBodies := materializedBodyVersions(result.RootResult())
		if len(gotKeys) != 7 || len(gotBodies) != 7 {
			t.Fatalf("run %d semantic high-fanout publication = %d summary keys, %d bodies; want one result for each of 7 lexical bodies", run, len(gotKeys), len(gotBodies))
		}
		if stats.FunctionalSummary.LexicalBodies != 7 || stats.FunctionalSummary.FormalEquations <= 0 {
			t.Fatalf("run %d formal execution = %d lexical bodies / %d equations; want 7 bodies and non-empty equations", run, stats.FunctionalSummary.LexicalBodies, stats.FunctionalSummary.FormalEquations)
		}
		if run == 0 {
			wantKeys, wantBodies = gotKeys, gotBodies
			continue
		}
		if !reflect.DeepEqual(gotKeys, wantKeys) {
			t.Fatalf("run %d summary keys changed\nwant: %#v\n got: %#v", run, wantKeys, gotKeys)
		}
		if !reflect.DeepEqual(gotBodies, wantBodies) {
			t.Fatalf("run %d body ordering or ResultVersions changed\nwant: %#v\n got: %#v", run, wantBodies, gotBodies)
		}
	}
}

func resultSummaryKeys(result Result) []summary.SummaryKey {
	entries := result.Snapshot().Entries()
	keys := make([]summary.SummaryKey, 0, len(entries))
	for _, entry := range entries {
		keys = append(keys, entry.Key)
	}
	return keys
}

type materializedBodyVersion struct {
	ordinal       string
	resultVersion uint64
}

func materializedBodyVersions(root *body.Result) []materializedBodyVersion {
	var out []materializedBodyVersion
	var walk func(*body.Result, string)
	walk = func(result *body.Result, ordinal string) {
		if result == nil {
			return
		}
		out = append(out, materializedBodyVersion{
			ordinal:       ordinal,
			resultVersion: result.ResultVersion(),
		})
		for index, child := range result.FunctionResults() {
			walk(child, ordinal+"/"+strconv.Itoa(index))
		}
	}
	walk(root, "root")
	return out
}

func resultVersionsByName(t *testing.T, src string) map[string]uint64 {
	t.Helper()
	result, err := RunChunk(parseChunk(t, src), Config{Check: body.Config{Registry: standard.Registry()}})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	root := result.RootResult()
	if root == nil {
		t.Fatal("RootResult missing")
	}
	out := map[string]uint64{"chunk": root.ResultVersion()}
	var walk func(*body.Result)
	walk = func(parent *body.Result) {
		for _, child := range parent.FunctionResults() {
			name := childResultName(root, child)
			if name == "" {
				t.Fatalf("missing stable name for child result %#v", child.Function())
			}
			if _, exists := out[name]; exists {
				t.Fatalf("lexical body %q published more than one result", name)
			}
			out[name] = child.ResultVersion()
			walk(child)
		}
	}
	walk(root)
	if len(out) == 0 {
		t.Fatal("no result versions collected")
	}
	return out
}

func childResultName(root, child *body.Result) string {
	if root == nil || child == nil || child.Function() == nil {
		return ""
	}
	origin, ok := root.FunctionOrigin(child.Function())
	if ok && origin.HasTargetSymbol {
		if name := root.SymbolName(origin.TargetSymbol); name != "" {
			return name
		}
	}
	if ok && origin.Method != "" {
		return origin.Method
	}
	return ""
}

func assertVersionChanged(t *testing.T, before, after map[string]uint64, name string) {
	t.Helper()
	left, leftOK := before[name]
	right, rightOK := after[name]
	if !leftOK || !rightOK {
		t.Fatalf("version missing %q: before=%v after=%v", name, sortedVersionNames(before), sortedVersionNames(after))
	}
	if left != right {
		return
	}
	t.Fatalf("%s lexical-body version did not change: %x", name, left)
}

func assertVersionUnchanged(t *testing.T, before, after map[string]uint64, name string) {
	t.Helper()
	left, leftOK := before[name]
	right, rightOK := after[name]
	if !leftOK || !rightOK {
		t.Fatalf("version missing %q: before=%v after=%v", name, sortedVersionNames(before), sortedVersionNames(after))
	}
	if left != right {
		t.Fatalf("%s lexical-body version changed: before=%x after=%x", name, left, right)
	}
}

func sortedVersionNames(versions map[string]uint64) []string {
	names := make([]string, 0, len(versions))
	for name := range versions {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
