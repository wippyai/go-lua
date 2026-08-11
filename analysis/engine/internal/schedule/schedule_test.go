package schedule

import (
	"bytes"
	"errors"
	"fmt"
	"reflect"
	"testing"
)

func TestPrepareIsCanonicalAcrossEdgePermutations(t *testing.T) {
	edges := []Edge{{0, 1}, {1, 0}, {1, 2}, {2, 3}, {3, 2}, {3, 4}, {5, 6}}
	want, err := Prepare(7, edges)
	if err != nil {
		t.Fatal(err)
	}
	wantBytes := scheduleBytes(want)
	for _, spelling := range [][]Edge{
		append([]Edge(nil), edges...),
		{{5, 6}, {3, 4}, {3, 2}, {2, 3}, {1, 2}, {1, 0}, {0, 1}},
		{{1, 2}, {3, 2}, {0, 1}, {5, 6}, {2, 3}, {1, 0}, {3, 4}, {0, 1}},
	} {
		got, err := Prepare(7, spelling)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, want) || !bytes.Equal(scheduleBytes(got), wantBytes) {
			t.Fatalf("edge spelling changed schedule:\n got %s\nwant %s", scheduleBytes(got), wantBytes)
		}
		assertScheduleLaws(t, got, 7, edges)
	}
}

func TestPrepareSelectsLowestReadyNodeAfterUnlock(t *testing.T) {
	// At the root frontier, 1 and 2 are initially ready. Processing 1 unlocks
	// 0, which is lower than the still-ready 2. Canonical order is therefore
	// 1, 0, 2; a FIFO ready queue incorrectly publishes 1, 2, 0.
	schedule, err := Prepare(3, []Edge{{1, 0}})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := nodeEvents(schedule), []Node{1, 0, 2}; !reflect.DeepEqual(got, want) {
		t.Fatalf("root node order = %v, want %v", got, want)
	}
}

func TestPrepareCanonicalReadyOrderIgnoresEdgeSpelling(t *testing.T) {
	// The two independent release edges make the spelling nontrivial. Every
	// spelling must retain the same exact min-ready linear extension.
	want := []Node{1, 0, 2, 4, 3}
	for _, edges := range [][]Edge{
		{{1, 0}, {4, 3}},
		{{4, 3}, {1, 0}},
		{{1, 0}, {4, 3}, {1, 0}, {4, 3}},
	} {
		schedule, err := Prepare(5, edges)
		if err != nil {
			t.Fatal(err)
		}
		if got := nodeEvents(schedule); !reflect.DeepEqual(got, want) {
			t.Fatalf("edge spelling %#v gives node order %v, want %v", edges, got, want)
		}
	}
}

func TestPrepareIngressDoesNotInventResidualMinimumHead(t *testing.T) {
	// The acyclic ingress 0 -> 2 canonically discovers the 2 <-> 1 cycle at
	// 2. Under the graph-derived DFS/LNF contract, 2 is therefore the unique
	// operational head. This is deliberately not a second, residual-SCC-minimum
	// head policy; no Program/Link Mu or external head enters Prepare.
	wantEvents := []Event{
		{EventNode, 0, NoRegion},
		{EventEnter, 2, 0}, {EventNode, 2, 0}, {EventNode, 1, 0}, {EventExit, 2, 0},
	}
	wantRegion := Region{Head: 2, Parent: NoRegion, Enter: 1, Exit: 4}
	var want []byte
	for _, edges := range [][]Edge{
		{{0, 2}, {2, 1}, {1, 2}},
		{{1, 2}, {0, 2}, {2, 1}},
		{{2, 1}, {1, 2}, {0, 2}, {2, 1}, {1, 2}},
	} {
		schedule, err := Prepare(3, edges)
		if err != nil {
			t.Fatal(err)
		}
		if got := scheduleEvents(schedule); !reflect.DeepEqual(got, wantEvents) {
			t.Fatalf("edges %#v give events %#v, want %#v", edges, got, wantEvents)
		}
		region, ok := schedule.RegionAt(0)
		if !ok || region != wantRegion {
			t.Fatalf("edges %#v give region %#v/%t, want %#v", edges, region, ok, wantRegion)
		}
		if got := scheduleBytes(schedule); want == nil {
			want = got
		} else if !bytes.Equal(got, want) {
			t.Fatalf("edge spelling %#v changed exact schedule: got %s, want %s", edges, got, want)
		}
		assertScheduleLaws(t, schedule, 3, edges)
	}
}

func TestPrepareDenseRenamingPreservesMappedSchedule(t *testing.T) {
	// The action compiler canonically renumbers independent action keys. This
	// test performs that renumbering separately, then compares the schedules
	// after mapping the second spelling back to the same action identities.
	keys := []string{"emit", "read", "merge", "publish", "check"}
	edgesByKey := [][2]string{{"emit", "read"}, {"read", "merge"}, {"merge", "read"}, {"merge", "publish"}, {"check", "publish"}}
	first, firstMap := canonicalGraph(keys, edgesByKey)
	second, secondMap := canonicalGraph([]string{"publish", "check", "merge", "emit", "read"}, edgesByKey)
	left, err := Prepare(len(keys), first)
	if err != nil {
		t.Fatal(err)
	}
	right, err := Prepare(len(keys), second)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := mappedSchedule(left, firstMap), mappedSchedule(right, secondMap); !reflect.DeepEqual(got, want) {
		t.Fatalf("canonical action renaming changed schedule:\n got %#v\nwant %#v", got, want)
	}
}

func TestPrepareOrderedPermutationPreservesSemanticSchedule(t *testing.T) {
	// semanticRanks is deliberately unrelated to the dense IDs. Relabeling
	// those IDs while carrying the same rank for each semantic node must leave
	// the complete WTO/event artifact unchanged after mapping nodes back.
	const (
		source = "source"
		left   = "left"
		right  = "right"
		sink   = "sink"
		other  = "other"
	)
	names := []string{source, left, right, sink, other}
	edges := []Edge{
		{0, 1}, {1, 2}, {2, 1}, {2, 3}, {4, 3},
	}
	ranks := []int{2, 0, 1, 4, 3}
	permutation := []Node{4, 2, 0, 3, 1} // old dense ID -> new dense ID
	relabelled := make([]Edge, len(edges))
	for index, edge := range edges {
		relabelled[index] = Edge{permutation[edge.From], permutation[edge.To]}
	}
	relabelledRanks := make([]int, len(ranks))
	relabelledNames := make(map[Node]string, len(names))
	for old, newNode := range permutation {
		relabelledRanks[newNode] = ranks[old]
		relabelledNames[newNode] = names[old]
	}
	originalNames := make(map[Node]string, len(names))
	for node, name := range names {
		originalNames[Node(node)] = name
	}

	leftSchedule, err := PrepareOrdered(len(names), edges, ranks)
	if err != nil {
		t.Fatal(err)
	}
	rightSchedule, err := PrepareOrdered(len(names), relabelled, relabelledRanks)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := mappedSchedule(leftSchedule, originalNames), mappedSchedule(rightSchedule, relabelledNames); !reflect.DeepEqual(got, want) {
		t.Fatalf("dense relabeling changed semantic schedule:\n got %#v\nwant %#v", got, want)
	}
	assertScheduleLaws(t, leftSchedule, len(names), edges)
	assertScheduleLaws(t, rightSchedule, len(names), relabelled)
}

func TestPrepareOrderedIdentityMatchesPrepareWTO(t *testing.T) {
	for _, test := range []struct {
		name  string
		nodes int
		edges []Edge
	}{
		{name: "acyclic", nodes: 4, edges: []Edge{{2, 0}, {1, 3}, {0, 3}}},
		{name: "nested cycles", nodes: 5, edges: []Edge{{0, 1}, {1, 0}, {1, 2}, {2, 3}, {3, 2}, {3, 4}}},
		{name: "cross tree", nodes: 6, edges: []Edge{{0, 1}, {1, 0}, {3, 4}, {4, 3}, {2, 0}, {5, 4}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			identity, err := Prepare(test.nodes, test.edges)
			if err != nil {
				t.Fatal(err)
			}
			ordered, err := PrepareOrdered(test.nodes, test.edges, identityRanks(test.nodes))
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(identity, ordered) {
				t.Fatalf("identity Prepare and PrepareOrdered differ:\n got %s\nwant %s", scheduleBytes(ordered), scheduleBytes(identity))
			}
			assertScheduleLaws(t, ordered, test.nodes, test.edges)
		})
	}
}

func TestPrepareOrderedRejectsDuplicateAndMissingSemanticRanks(t *testing.T) {
	for _, test := range []struct {
		name  string
		ranks []int
	}{
		{name: "duplicate rank", ranks: []int{0, 0, 2}},
		{name: "missing rank", ranks: []int{0, 2, 2}},
		{name: "short rank list", ranks: []int{0, 1}},
		{name: "out of range", ranks: []int{0, 1, 3}},
		{name: "negative", ranks: []int{0, -1, 2}},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := PrepareOrdered(3, nil, test.ranks)
			if !errors.Is(err, ErrInvalidOrder) || got != nil {
				t.Fatalf("PrepareOrdered ranks %#v = %#v, %v; want nil invalid order", test.ranks, got, err)
			}
		})
	}
}

func TestPrepareCoreCycleShapes(t *testing.T) {
	for _, test := range []struct {
		name  string
		nodes int
		edges []Edge
	}{
		{"self", 1, []Edge{{0, 0}}},
		{"mutual", 2, []Edge{{0, 1}, {1, 0}}},
		// This graph is the schedule's domain-abstract analogue of a
		// cross-factor reduction: source 0 feeds a recurrence and one
		// result escapes afterwards. No factor vocabulary reaches schedule.
		{"cross-factor-like", 4, []Edge{{0, 1}, {1, 2}, {2, 1}, {2, 3}}},
		{"nested", 4, []Edge{{0, 1}, {1, 0}, {1, 2}, {2, 1}, {2, 3}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			schedule, err := Prepare(test.nodes, test.edges)
			if err != nil {
				t.Fatal(err)
			}
			if schedule.RegionCount() == 0 {
				t.Fatal("cyclic graph published no feedback region")
			}
			assertScheduleLaws(t, schedule, test.nodes, test.edges)
		})
	}
}

func TestPrepareFeedbackHeadsCoverEveryCycle(t *testing.T) {
	// Overlapping cycles require distinct nested cuts; this is a semantic WTO
	// property, not a test of how a caller chooses a recurrence head.
	edges := []Edge{{0, 1}, {1, 0}, {1, 2}, {2, 1}, {2, 3}, {3, 4}, {4, 3}}
	schedule, err := Prepare(5, edges)
	if err != nil {
		t.Fatal(err)
	}
	if schedule.RegionCount() < 2 {
		t.Fatalf("RegionCount = %d, want nested feedback coverage", schedule.RegionCount())
	}
	assertScheduleLaws(t, schedule, 5, edges)
}

func TestPrepareRejectsMalformedEdges(t *testing.T) {
	for _, test := range []struct {
		name      string
		nodeCount int
		edges     []Edge
	}{
		{"negative count", -1, nil}, {"negative from", 1, []Edge{{-1, 0}}},
		{"negative to", 1, []Edge{{0, -1}}}, {"from past end", 1, []Edge{{1, 0}}},
		{"to past end", 1, []Edge{{0, 1}}}, {"edge in empty graph", 0, []Edge{{0, 0}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := Prepare(test.nodeCount, test.edges)
			if !errors.Is(err, ErrInvalidEdge) || got != nil {
				t.Fatalf("Prepare(%d, %#v) = %#v, %v; want nil invalid edge", test.nodeCount, test.edges, got, err)
			}
		})
	}
}

func TestPrepareEmptyAndDuplicateEdgesAreStructuralNoOps(t *testing.T) {
	empty, err := Prepare(0, nil)
	if err != nil {
		t.Fatal(err)
	}
	if empty.NodeCount() != 0 || empty.EventCount() != 0 || empty.RegionCount() != 0 {
		t.Fatalf("empty schedule = nodes/events/regions %d/%d/%d, want 0/0/0", empty.NodeCount(), empty.EventCount(), empty.RegionCount())
	}
	plain := []Edge{{0, 1}, {1, 0}, {1, 2}, {2, 3}}
	duplicates := []Edge{{1, 2}, {0, 1}, {1, 0}, {1, 2}, {0, 1}, {2, 3}, {1, 0}}
	want, err := Prepare(4, plain)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Prepare(4, duplicates)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("duplicate edges changed schedule:\n got %s\nwant %s", scheduleBytes(got), scheduleBytes(want))
	}
}

func TestPrepareExhaustiveFourNodeGraphsSatisfyWTO(t *testing.T) {
	// A complete small-graph sweep catches schedule defects without depending
	// on Program, factors, or a caller-specified recurrence partition.
	const nodes = 4
	for bits := uint32(0); bits < 1<<(nodes*nodes); bits++ {
		edges := make([]Edge, 0, nodes*nodes)
		for from := 0; from < nodes; from++ {
			for to := 0; to < nodes; to++ {
				if bits&(1<<uint(from*nodes+to)) != 0 {
					edges = append(edges, Edge{Node(from), Node(to)})
				}
			}
		}
		schedule, err := Prepare(nodes, edges)
		if err != nil {
			t.Fatalf("graph %#x: %v", bits, err)
		}
		assertScheduleLaws(t, schedule, nodes, edges)
	}
}

func TestPrepareDeepGraphsUseNoHostRecursion(t *testing.T) {
	const nodes = 100_000
	chain := make([]Edge, 0, nodes-1)
	for node := 0; node < nodes-1; node++ {
		chain = append(chain, Edge{Node(node), Node(node + 1)})
	}
	schedule, err := Prepare(nodes, chain)
	if err != nil {
		t.Fatal(err)
	}
	if schedule.RegionCount() != 0 || schedule.EventCount() != nodes {
		t.Fatalf("deep chain regions/events = %d/%d, want 0/%d", schedule.RegionCount(), schedule.EventCount(), nodes)
	}

	const nested = 20_000
	edges := make([]Edge, 0, (nested-1)*2)
	for node := 0; node < nested-1; node++ {
		edges = append(edges, Edge{Node(node), Node(node + 1)}, Edge{Node(node + 1), Node(node)})
	}
	schedule, err = Prepare(nested, edges)
	if err != nil {
		t.Fatal(err)
	}
	assertScheduleLaws(t, schedule, nested, edges)
}

func BenchmarkPrepareSparseChain(b *testing.B) { benchmarkPrepareChain(b, false) }
func BenchmarkPrepareSparseRing(b *testing.B)  { benchmarkPrepareChain(b, true) }

func benchmarkPrepareChain(b *testing.B, ring bool) {
	const nodes = 4_096
	edges := make([]Edge, 0, nodes)
	for node := 0; node < nodes-1; node++ {
		edges = append(edges, Edge{Node(node), Node(node + 1)})
	}
	if ring {
		edges = append(edges, Edge{Node(nodes - 1), 0})
	}
	b.ReportAllocs()
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		if _, err := Prepare(nodes, edges); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkPrepareNested(b *testing.B) {
	const nodes = 1_024
	edges := make([]Edge, 0, (nodes-1)*2)
	for node := 0; node < nodes-1; node++ {
		edges = append(edges, Edge{Node(node), Node(node + 1)}, Edge{Node(node + 1), Node(node)})
	}
	b.ReportAllocs()
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		if _, err := Prepare(nodes, edges); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkPrepareDense(b *testing.B) {
	for _, nodes := range []int{128, 256, 512} {
		b.Run(fmt.Sprintf("nodes=%d", nodes), func(b *testing.B) {
			edges := denseEdges(nodes)
			b.ReportAllocs()
			b.ResetTimer()
			for iteration := 0; iteration < b.N; iteration++ {
				if _, err := Prepare(nodes, edges); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func canonicalGraph(keys []string, edges [][2]string) ([]Edge, map[Node]string) {
	ordered := append([]string(nil), keys...)
	// Independently canonicalize source spelling. Keys are intentionally only a
	// test oracle: production receives their resulting dense order, not keys.
	for left := range ordered {
		for right := left + 1; right < len(ordered); right++ {
			if ordered[right] < ordered[left] {
				ordered[left], ordered[right] = ordered[right], ordered[left]
			}
		}
	}
	index := make(map[string]Node, len(ordered))
	back := make(map[Node]string, len(ordered))
	for position, key := range ordered {
		index[key], back[Node(position)] = Node(position), key
	}
	graph := make([]Edge, len(edges))
	for position, edge := range edges {
		graph[position] = Edge{index[edge[0]], index[edge[1]]}
	}
	return graph, back
}

type namedEvent struct {
	kind   EventKind
	node   string
	region int
}
type namedRegion struct {
	head                string
	parent, enter, exit int
}
type namedSchedule struct {
	events  []namedEvent
	regions []namedRegion
}

func mappedSchedule(schedule *Schedule, names map[Node]string) namedSchedule {
	result := namedSchedule{events: make([]namedEvent, schedule.EventCount()), regions: make([]namedRegion, schedule.RegionCount())}
	for index := range result.events {
		event, _ := schedule.EventAt(index)
		result.events[index] = namedEvent{event.Kind, names[event.Node], event.Region}
	}
	for index := range result.regions {
		region, _ := schedule.RegionAt(index)
		result.regions[index] = namedRegion{names[region.Head], region.Parent, region.Enter, region.Exit}
	}
	return result
}

func assertScheduleLaws(t *testing.T, schedule *Schedule, nodes int, edges []Edge) {
	t.Helper()
	assertEventRegionBijection(t, schedule)
	head := make([]bool, nodes)
	for index := 0; index < schedule.RegionCount(); index++ {
		region, _ := schedule.RegionAt(index)
		if head[region.Head] {
			t.Fatalf("duplicate published head %d", region.Head)
		}
		head[region.Head] = true
	}
	if remainingCycle(nodes, edges, head) {
		t.Fatal("removing published feedback heads left a cyclic subgraph")
	}
	position := make([]int, nodes)
	for node := range position {
		position[node] = -1
	}
	for index := 0; index < schedule.EventCount(); index++ {
		event, _ := schedule.EventAt(index)
		if event.Kind == EventNode {
			if position[event.Node] != -1 {
				t.Fatalf("Node %d has multiple transfer events", event.Node)
			}
			position[event.Node] = index
		}
	}
	for node, at := range position {
		if at < 0 {
			t.Fatalf("Node %d has no transfer event", node)
		}
	}
	for _, edge := range edges {
		if position[edge.From] < position[edge.To] {
			continue
		}
		covered := false
		for index := 0; index < schedule.RegionCount(); index++ {
			region, _ := schedule.RegionAt(index)
			if region.Head == edge.To && region.Enter < position[edge.From] && region.Exit > position[edge.From] {
				covered = true
				break
			}
		}
		if !covered {
			t.Fatalf("backward influence %d -> %d has no active head", edge.From, edge.To)
		}
	}
}

func assertEventRegionBijection(t *testing.T, schedule *Schedule) {
	t.Helper()
	entered, exited := make([]bool, schedule.RegionCount()), make([]bool, schedule.RegionCount())
	stack := make([]int, 0, schedule.RegionCount())
	for index := 0; index < schedule.EventCount(); index++ {
		event, _ := schedule.EventAt(index)
		switch event.Kind {
		case EventEnter:
			region, ok := schedule.RegionAt(event.Region)
			if !ok || entered[event.Region] || region.Enter != index || region.Head != event.Node {
				t.Fatalf("invalid enter %#v", event)
			}
			parent := NoRegion
			if len(stack) != 0 {
				parent = stack[len(stack)-1]
			}
			if region.Parent != parent {
				t.Fatalf("region %d parent = %d, want %d", event.Region, region.Parent, parent)
			}
			entered[event.Region] = true
			stack = append(stack, event.Region)
		case EventNode:
			if event.Region == NoRegion {
				if len(stack) != 0 {
					t.Fatalf("root node inside region")
				}
				continue
			}
			if len(stack) == 0 || stack[len(stack)-1] != event.Region {
				t.Fatalf("node %#v outside current region", event)
			}
		case EventExit:
			region, ok := schedule.RegionAt(event.Region)
			if !ok || len(stack) == 0 || stack[len(stack)-1] != event.Region || exited[event.Region] || region.Exit != index || region.Head != event.Node {
				t.Fatalf("invalid exit %#v", event)
			}
			exited[event.Region] = true
			stack = stack[:len(stack)-1]
		default:
			t.Fatalf("unknown event kind %d", event.Kind)
		}
	}
	if len(stack) != 0 {
		t.Fatalf("unterminated regions %#v", stack)
	}
	for index := range entered {
		if !entered[index] || !exited[index] {
			t.Fatalf("region %d unbalanced", index)
		}
	}
}

func remainingCycle(nodes int, edges []Edge, removed []bool) bool {
	indegree, successors := make([]int, nodes), make([][]Node, nodes)
	remaining := 0
	for node := 0; node < nodes; node++ {
		if !removed[node] {
			remaining++
		}
	}
	for _, edge := range edges {
		if !removed[edge.From] && !removed[edge.To] {
			successors[edge.From] = append(successors[edge.From], edge.To)
			indegree[edge.To]++
		}
	}
	ready := make([]Node, 0, remaining)
	for node := 0; node < nodes; node++ {
		if !removed[node] && indegree[node] == 0 {
			ready = append(ready, Node(node))
		}
	}
	seen := 0
	for len(ready) != 0 {
		node := ready[len(ready)-1]
		ready = ready[:len(ready)-1]
		seen++
		for _, next := range successors[node] {
			indegree[next]--
			if indegree[next] == 0 {
				ready = append(ready, next)
			}
		}
	}
	return seen != remaining
}

func denseEdges(nodes int) []Edge {
	edges := make([]Edge, 0, nodes*nodes)
	for from := 0; from < nodes; from++ {
		for to := 0; to < nodes; to++ {
			edges = append(edges, Edge{Node(from), Node(to)})
		}
	}
	return edges
}

func scheduleBytes(schedule *Schedule) []byte {
	var out bytes.Buffer
	for index := 0; index < schedule.EventCount(); index++ {
		event, _ := schedule.EventAt(index)
		fmt.Fprintf(&out, "%d:%d:%d;", event.Kind, event.Node, event.Region)
	}
	for index := 0; index < schedule.RegionCount(); index++ {
		region, _ := schedule.RegionAt(index)
		fmt.Fprintf(&out, "r%d:%d:%d:%d:%d;", index, region.Head, region.Parent, region.Enter, region.Exit)
	}
	return out.Bytes()
}

func scheduleEvents(schedule *Schedule) []Event {
	events := make([]Event, schedule.EventCount())
	for index := range events {
		events[index], _ = schedule.EventAt(index)
	}
	return events
}

func nodeEvents(schedule *Schedule) []Node {
	nodes := make([]Node, 0, schedule.NodeCount())
	for index := 0; index < schedule.EventCount(); index++ {
		event, _ := schedule.EventAt(index)
		if event.Kind == EventNode {
			nodes = append(nodes, event.Node)
		}
	}
	return nodes
}
