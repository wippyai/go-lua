package program

import (
	"reflect"
	"sort"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestResultVersionsTrackBodyAndConsumedSummaryInputs(t *testing.T) {
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

local function caller(flag)
	return leaf(flag)
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
	assertVersionChanged(t, before, after, "caller")
	assertVersionChanged(t, before, after, "chunk")
	assertVersionUnchanged(t, before, after, "unrelated")
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
				t.Fatalf("duplicate function result name %q", name)
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
	left, right := requireVersion(t, before, after, name)
	if left == right {
		t.Fatalf("%s version did not change: %d", name, left)
	}
}

func assertVersionUnchanged(t *testing.T, before, after map[string]uint64, name string) {
	t.Helper()
	left, right := requireVersion(t, before, after, name)
	if left != right {
		t.Fatalf("%s version changed: before=%d after=%d", name, left, right)
	}
}

func requireVersion(t *testing.T, before, after map[string]uint64, name string) (uint64, uint64) {
	t.Helper()
	left, ok := before[name]
	if !ok {
		t.Fatalf("before version missing %q; have %v", name, sortedVersionNames(before))
	}
	right, ok := after[name]
	if !ok {
		t.Fatalf("after version missing %q; have %v", name, sortedVersionNames(after))
	}
	return left, right
}

func sortedVersionNames(versions map[string]uint64) []string {
	names := make([]string, 0, len(versions))
	for name := range versions {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
