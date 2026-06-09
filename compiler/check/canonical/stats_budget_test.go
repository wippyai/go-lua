package canonical

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/compiler/check/canonical/summary"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/compiler/parse"
	"github.com/wippyai/go-lua/types/diag"
)

func TestCanonicalStatsBudgetRecursiveContextFamily(t *testing.T) {
	diags, stats := runCanonicalStatsBudget(t, "stats-recursive-context.lua", `
local function is_odd(n)
    if n <= 0 then
        return false
    end
    return is_even(n - 1)
end

function is_even(n)
    if n <= 0 then
        return true
    end
    return is_odd(n - 1)
end

local a = is_even(4)
local b = is_odd(5)
return a, b
`)
	requireNoBudgetDiagnostics(t, diags, stats)
	requireStatsBudget(t, statsBudget{
		Name:                         "recursive context family",
		Stats:                        stats,
		MaxUniqueSummaryKeyDemands:   14,
		MaxDiagnosticObservedStates:  12,
		MaxSnapshotExactKeyMisses:    48,
		MaxObserveIntraWithKeyCalls:  12,
		MaxNestedCallProductMisses:   4,
		MaxNestedCallProductCacheGap: 4,
	})
}

func TestCanonicalStatsBudgetDynamicReceiverIndexBoundary(t *testing.T) {
	diags, stats := runCanonicalStatsBudget(t, "stats-dynamic-receiver-boundary.lua", `
local FlowGraph = {}
local flow_graph_mt = { __index = FlowGraph }

function FlowGraph.new()
    return setmetatable({
        nodes = table.create(0, 4),
        node_order = table.create(4, 0),
        edges = table.create(0, 4),
    }, flow_graph_mt)
end

function FlowGraph:create_node(node_id, node_type)
    self.nodes[node_id] = {
        node_id = node_id,
        node_type = node_type,
        parent_node_id = nil,
        status = "pending",
    }
    table.insert(self.node_order, node_id)
    self.edges[node_id] = {
        targets = table.create(2, 0),
    }
    return node_id, nil
end

function FlowGraph:compute_auto_chain()
    for i = 1, #self.node_order - 1 do
        local current_node_id = self.node_order[i]
        local next_node_id = self.node_order[i + 1]
        local current_node = self.nodes[current_node_id]
        local next_node = self.nodes[next_node_id]
        if not current_node.parent_node_id and not next_node.parent_node_id then
            local current_edges = self.edges[current_node_id]
            table.insert(current_edges.targets, {
                target_node_id = next_node_id,
            })
        end
    end
end

local graph = FlowGraph.new()
local _, err = graph:create_node("a", "start")
if err then
    return nil, err
end
local _, err2 = graph:create_node("b", "finish")
if err2 then
    return nil, err2
end
graph:compute_auto_chain()
return graph
`)
	requireNoBudgetDiagnostics(t, diags, stats)
	requireStatsBudget(t, statsBudget{
		Name:                         "dynamic receiver/index boundary",
		Stats:                        stats,
		MaxUniqueSummaryKeyDemands:   24,
		MaxDiagnosticObservedStates:  16,
		MaxSnapshotExactKeyMisses:    8,
		MaxObserveIntraWithKeyCalls:  20,
		MaxNestedCallProductMisses:   8,
		MaxNestedCallProductCacheGap: 8,
	})
}

func TestCanonicalStatsBudgetDiagnosticObservationDoesNotDemandExtraSummaryKeys(t *testing.T) {
	_, stats := runCanonicalStatsBudget(t, "stats-diagnostics-heavy.lua", `
type QueryResult = {[string]: any}

local function read(result: {QueryResult})
    if result[1] then
        local a: string = result[1]["ok"]
        local b: string = result[3]["missing"]
        local c: number = result[4]["missing"]
        return a, b, c
    end
    local d: string = result[5]["missing"]
    return nil, d
end

local function caller(result)
    return read(result)
end

return caller
`)
	requireStatsBudget(t, statsBudget{
		Name:                         "diagnostic observation optional/index shape",
		Stats:                        stats,
		MaxUniqueSummaryKeyDemands:   8,
		MaxDiagnosticObservedStates:  8,
		MaxSnapshotExactKeyMisses:    6,
		MaxObserveIntraWithKeyCalls:  8,
		MaxNestedCallProductMisses:   4,
		MaxNestedCallProductCacheGap: 4,
	})
	if stats.DiagnosticObservedStates < 6 {
		t.Fatalf("diagnostic observation did not exercise enough exact states\n%s", formatStatsSnapshot(stats))
	}
	if stats.UniqueSummaryKeyDemands >= stats.DiagnosticObservedStates {
		t.Fatalf("diagnostic observation appears to create one summary-key demand per observed state\n%s", formatStatsSnapshot(stats))
	}
}

type statsBudget struct {
	Name                         string
	Stats                        summary.StatsSnapshot
	MaxUniqueSummaryKeyDemands   int
	MaxDiagnosticObservedStates  int
	MaxSnapshotExactKeyMisses    int
	MaxObserveIntraWithKeyCalls  int
	MaxNestedCallProductMisses   int
	MaxNestedCallProductCacheGap int
}

func runCanonicalStatsBudget(t *testing.T, name, src string) ([]diag.Diagnostic, summary.StatsSnapshot) {
	t.Helper()
	chunk, err := parse.ParseString(src, name)
	if err != nil {
		t.Fatalf("parse %s failed: %v", name, err)
	}
	driver := NewDriver(Config{Stdlib: scope.NewWithBuiltins()})
	sess := newCanonicalTestSession(name)
	driver.Run(sess, chunk)
	if driver.stats == nil {
		t.Fatal("canonical driver did not initialize stats")
	}
	return sess.DiagnosticsSlice(), driver.stats.Snapshot()
}

func requireNoBudgetDiagnostics(t *testing.T, diags []diag.Diagnostic, stats summary.StatsSnapshot) {
	t.Helper()
	if errors := errorDiagnosticsOnly(diags); len(errors) != 0 {
		t.Fatalf("expected clean check, got diagnostics: %v\n%s", diagnosticMessages(errors), formatStatsSnapshot(stats))
	}
}

func requireStatsBudget(t *testing.T, b statsBudget) {
	t.Helper()
	if b.Stats.UniqueSummaryKeyDemands > b.MaxUniqueSummaryKeyDemands {
		t.Fatalf("%s UniqueSummaryKeyDemands=%d, budget=%d\n%s", b.Name, b.Stats.UniqueSummaryKeyDemands, b.MaxUniqueSummaryKeyDemands, formatStatsSnapshot(b.Stats))
	}
	if b.Stats.DiagnosticObservedStates > b.MaxDiagnosticObservedStates {
		t.Fatalf("%s DiagnosticObservedStates=%d, budget=%d\n%s", b.Name, b.Stats.DiagnosticObservedStates, b.MaxDiagnosticObservedStates, formatStatsSnapshot(b.Stats))
	}
	if b.Stats.SnapshotExactKeyMisses > b.MaxSnapshotExactKeyMisses {
		t.Fatalf("%s SnapshotExactKeyMisses=%d, budget=%d\n%s", b.Name, b.Stats.SnapshotExactKeyMisses, b.MaxSnapshotExactKeyMisses, formatStatsSnapshot(b.Stats))
	}
	if b.Stats.ObserveIntraWithKeyCalls > b.MaxObserveIntraWithKeyCalls {
		t.Fatalf("%s ObserveIntraWithKeyCalls=%d, budget=%d\n%s", b.Name, b.Stats.ObserveIntraWithKeyCalls, b.MaxObserveIntraWithKeyCalls, formatStatsSnapshot(b.Stats))
	}
	if b.Stats.NestedCallProductCacheMisses > b.MaxNestedCallProductMisses {
		t.Fatalf("%s NestedCallProductCacheMisses=%d, budget=%d\n%s", b.Name, b.Stats.NestedCallProductCacheMisses, b.MaxNestedCallProductMisses, formatStatsSnapshot(b.Stats))
	}
	cacheGap := b.Stats.NestedCallProductCacheMisses - b.Stats.NestedCallProductCacheHits
	if cacheGap > b.MaxNestedCallProductCacheGap {
		t.Fatalf("%s nested call product cache gap=%d, budget=%d\n%s", b.Name, cacheGap, b.MaxNestedCallProductCacheGap, formatStatsSnapshot(b.Stats))
	}
}

func formatStatsSnapshot(s summary.StatsSnapshot) string {
	var b strings.Builder
	fmt.Fprintf(&b, "stats{uniqueKeys=%d summarizeWithKey=%d intra=%d observeIntraWithKey=%d snapshotHit=%d snapshotMiss=%d diagnosticStates=%d callEntryProjectionRuns=%d nestedCacheHit=%d nestedCacheMiss=%d}",
		s.UniqueSummaryKeyDemands,
		s.SummarizeWithKeyCalls,
		s.IntraObserverCalls,
		s.ObserveIntraWithKeyCalls,
		s.SnapshotExactKeyHits,
		s.SnapshotExactKeyMisses,
		s.DiagnosticObservedStates,
		s.CallEntryProjectionRuns,
		s.NestedCallProductCacheHits,
		s.NestedCallProductCacheMisses,
	)
	if len(s.SummaryKeyDemandsByRef) != 0 {
		fmt.Fprintf(&b, "\nbyRef=%s", formatIntByRef(s.SummaryKeyDemandsByRef))
	}
	if len(s.SummaryKeyDemandFamiliesByRef) != 0 {
		fmt.Fprintf(&b, "\nfamilies=%s", formatFamiliesByRef(s.SummaryKeyDemandFamiliesByRef))
	}
	return b.String()
}

func formatIntByRef(in map[summary.FuncRef]int) string {
	refs := sortedIntStatsRefs(in)
	parts := make([]string, 0, len(refs))
	for _, ref := range refs {
		parts = append(parts, fmt.Sprintf("%s:%d", formatStatsRef(ref), in[ref]))
	}
	return "{" + strings.Join(parts, ", ") + "}"
}

func formatFamiliesByRef(in map[summary.FuncRef]summary.SummaryKeyFamilyCounts) string {
	refs := sortedFamilyStatsRefs(in)
	parts := make([]string, 0, len(refs))
	for _, ref := range refs {
		c := in[ref]
		parts = append(parts, fmt.Sprintf("%s:{default=%d values=%d refs=%d facts=%d multi=%d}",
			formatStatsRef(ref), c.Default, c.WithValues, c.WithReferences, c.WithFacts, c.WithMultipleAxes))
	}
	return "{" + strings.Join(parts, ", ") + "}"
}

func sortedIntStatsRefs(in map[summary.FuncRef]int) []summary.FuncRef {
	refs := make([]summary.FuncRef, 0, len(in))
	for ref := range in {
		refs = append(refs, ref)
	}
	sort.Slice(refs, func(i, j int) bool {
		if refs[i].GraphID != refs[j].GraphID {
			return refs[i].GraphID < refs[j].GraphID
		}
		return refs[i].ParentHash < refs[j].ParentHash
	})
	return refs
}

func sortedFamilyStatsRefs(in map[summary.FuncRef]summary.SummaryKeyFamilyCounts) []summary.FuncRef {
	refs := make([]summary.FuncRef, 0, len(in))
	for ref := range in {
		refs = append(refs, ref)
	}
	sort.Slice(refs, func(i, j int) bool {
		if refs[i].GraphID != refs[j].GraphID {
			return refs[i].GraphID < refs[j].GraphID
		}
		return refs[i].ParentHash < refs[j].ParentHash
	})
	return refs
}

func formatStatsRef(ref summary.FuncRef) string {
	return fmt.Sprintf("g%d/h%d", ref.GraphID, ref.ParentHash)
}

func errorDiagnosticsOnly(diags []diag.Diagnostic) []diag.Diagnostic {
	out := make([]diag.Diagnostic, 0, len(diags))
	for _, d := range diags {
		if d.Severity == diag.SeverityError {
			out = append(out, d)
		}
	}
	return out
}

func diagnosticMessages(diags []diag.Diagnostic) []string {
	out := make([]string, 0, len(diags))
	for _, d := range diags {
		out = append(out, d.Message)
	}
	return out
}
