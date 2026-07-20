package transformer

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

func TestSymbolicWTOTapeNestedComponentsAndEdgeClasses(t *testing.T) {
	graph := cfg.New()
	outerHead := graph.AddNode(cfg.NodeBranch)
	innerHead := graph.AddNode(cfg.NodeBranch)
	innerBody := graph.AddNode(cfg.NodeAssign)
	outerLatch := graph.AddNode(cfg.NodeAssign)
	graph.AddEdge(graph.Entry(), outerHead, false)
	graph.AddEdge(outerHead, innerHead, true)
	graph.AddEdge(outerHead, graph.Exit(), false)
	graph.AddEdge(innerHead, innerBody, true)
	graph.AddEdge(innerHead, outerLatch, false)
	graph.AddEdge(innerBody, innerHead, false)
	graph.AddEdge(outerLatch, outerHead, false)

	tape := mustCompileSymbolicWTOTape(t, graph)
	if len(tape.components) != 2 {
		t.Fatalf("components = %d, want outer and nested inner", len(tape.components))
	}
	outer := tape.points[tape.denseIndex(outerHead)].headComponent
	inner := tape.points[tape.denseIndex(innerHead)].headComponent
	if outer < 0 || inner < 0 || outer == inner {
		t.Fatalf("heads: outer=%d inner=%d", outer, inner)
	}
	if tape.components[inner].parent != outer {
		t.Fatalf("inner parent = %d, want outer %d", tape.components[inner].parent, outer)
	}
	for _, point := range []cfg.Point{outerHead, innerHead, innerBody, outerLatch} {
		if !tape.componentContains(outer, uint32(tape.denseIndex(point))) {
			t.Fatalf("outer component does not contain point %d", point)
		}
	}
	for _, point := range []cfg.Point{innerHead, innerBody} {
		if !tape.componentContains(inner, uint32(tape.denseIndex(point))) {
			t.Fatalf("inner component does not contain point %d", point)
		}
	}
	if tape.componentContains(inner, uint32(tape.denseIndex(outerLatch))) {
		t.Fatal("inner component contains outer latch")
	}

	assertSymbolicWTOEdge(t, tape, graph.Entry(), outerHead, symbolicWTOEdgeEntry, outer, 0, 1)
	assertSymbolicWTOEdge(t, tape, outerHead, innerHead, symbolicWTOEdgeEntry, inner, 0, 1)
	assertSymbolicWTOEdge(t, tape, innerHead, innerBody, symbolicWTOEdgeBody, inner, 0, 0)
	assertSymbolicWTOEdge(t, tape, innerBody, innerHead, symbolicWTOEdgeBackedge, inner, 0, 0)
	assertSymbolicWTOEdge(t, tape, innerHead, outerLatch, symbolicWTOEdgeExit, inner, 1, 0)
	assertSymbolicWTOEdge(t, tape, outerLatch, outerHead, symbolicWTOEdgeBackedge, outer, 0, 0)
	assertSymbolicWTOEdge(t, tape, outerHead, graph.Exit(), symbolicWTOEdgeExit, outer, 1, 0)
}

func TestSymbolicWTOTapeRejectsNilGraph(t *testing.T) {
	if tape, err := compileSymbolicWTOTape(nil); err == nil || tape != nil {
		t.Fatalf("compile nil = (%#v, %v), want atomic error", tape, err)
	}
}

func TestSymbolicWTOTapeClassifiesSiblingTransition(t *testing.T) {
	graph := cfg.New()
	left := graph.AddNode(cfg.NodeBranch)
	right := graph.AddNode(cfg.NodeBranch)
	graph.AddEdge(graph.Entry(), left, false)
	graph.AddEdge(left, left, true)
	graph.AddEdge(left, right, false)
	graph.AddEdge(right, right, true)
	graph.AddEdge(right, graph.Exit(), false)

	tape := mustCompileSymbolicWTOTape(t, graph)
	leftComponent := tape.points[tape.denseIndex(left)].headComponent
	rightComponent := tape.points[tape.denseIndex(right)].headComponent
	if leftComponent < 0 || rightComponent < 0 || leftComponent == rightComponent {
		t.Fatalf("sibling components = (%d,%d)", leftComponent, rightComponent)
	}
	assertSymbolicWTOEdge(t, tape, left, right, symbolicWTOEdgeTransition, -1, 1, 1)
}

func TestSymbolicWTOTapeExcludesUnreachablePointsAndIsImmutable(t *testing.T) {
	graph := cfg.New()
	live := graph.AddNode(cfg.NodeAssign)
	dead := graph.AddNode(cfg.NodeAssign)
	graph.AddEdge(graph.Entry(), live, false)
	graph.AddEdge(live, graph.Exit(), false)
	graph.AddEdge(dead, dead, false)
	graph.AddEdge(dead, live, false)

	tape := mustCompileSymbolicWTOTape(t, graph)
	if got, want := len(tape.points), len(graph.RPO()); got != want {
		t.Fatalf("points = %d, reachable RPO = %d", got, want)
	}
	if got := tape.denseIndex(dead); got != -1 {
		t.Fatalf("unreachable dense index = %d, want -1", got)
	}
	for _, edge := range tape.edges {
		if tape.points[edge.from].point == dead || tape.points[edge.to].point == dead {
			t.Fatalf("unreachable edge escaped into tape: %#v", edge)
		}
	}

	before := symbolicWTOTapeDigest(tape)
	later := graph.AddNode(cfg.NodeAssign)
	graph.AddEdge(live, later, false)
	graph.AddEdge(later, graph.Exit(), false)
	if after := symbolicWTOTapeDigest(tape); after != before {
		t.Fatalf("compiled tape changed after CFG mutation:\nbefore %s\nafter  %s", before, after)
	}
	if tape.denseIndex(later) != -1 {
		t.Fatal("compiled tape acquired a point added after construction")
	}
}

func TestSymbolicWTOTapeExhaustiveTwoVertexTopologies(t *testing.T) {
	for mask := 0; mask < 16; mask++ {
		t.Run(fmt.Sprintf("edges_%04b", mask), func(t *testing.T) {
			graph := cfg.New()
			a := graph.AddNode(cfg.NodeAssign)
			b := graph.AddNode(cfg.NodeAssign)
			graph.AddEdge(graph.Entry(), a, false)
			graph.AddEdge(graph.Entry(), b, false)
			graph.AddEdge(a, graph.Exit(), false)
			graph.AddEdge(b, graph.Exit(), false)
			vertices := []cfg.Point{a, b}
			variableEdges := 0
			for bit := 0; bit < 4; bit++ {
				if mask&(1<<bit) == 0 {
					continue
				}
				graph.AddEdge(vertices[bit/2], vertices[bit%2], false)
				variableEdges++
			}

			tape := mustCompileSymbolicWTOTape(t, graph)
			if got := len(tape.points); got != 4 {
				t.Fatalf("point count = %d, want 4", got)
			}
			if got := len(tape.edges); got != 4+variableEdges {
				t.Fatalf("edge count = %d, want %d", got, 4+variableEdges)
			}
			seen := make([]bool, graph.Size())
			for dense, point := range tape.points {
				if seen[point.point] {
					t.Fatalf("point %d appears twice", point.point)
				}
				seen[point.point] = true
				if got := tape.denseIndex(point.point); got != int32(dense) {
					t.Fatalf("point %d index = %d, want %d", point.point, got, dense)
				}
			}
			for _, edge := range tape.edges {
				if edge.from >= uint32(len(tape.points)) || edge.to >= uint32(len(tape.points)) {
					t.Fatalf("non-dense edge = %#v", edge)
				}
				if edge.exitCount == 0 && edge.enterCount == 0 &&
					edge.kind != symbolicWTOEdgeOutside && edge.kind != symbolicWTOEdgeBody && edge.kind != symbolicWTOEdgeBackedge {
					t.Fatalf("zero-crossing edge has kind %d", edge.kind)
				}
			}
		})
	}
}

func TestSymbolicWTOTapeDeterministicAndMapFree(t *testing.T) {
	graph := cfg.New()
	head := graph.AddNode(cfg.NodeBranch)
	body := graph.AddNode(cfg.NodeAssign)
	graph.AddEdge(graph.Entry(), head, false)
	graph.AddEdge(head, body, true)
	graph.AddEdge(head, graph.Exit(), false)
	graph.AddEdge(body, head, false)

	want := symbolicWTOTapeDigest(mustCompileSymbolicWTOTape(t, graph))
	for i := 0; i < 100; i++ {
		got := symbolicWTOTapeDigest(mustCompileSymbolicWTOTape(t, graph))
		if got != want {
			t.Fatalf("compile %d differs:\nwant %s\ngot  %s", i, want, got)
		}
	}
	assertNoMapFields(t, reflect.TypeOf(symbolicWTOTape{}), map[reflect.Type]bool{})
}

func mustCompileSymbolicWTOTape(t *testing.T, graph cfg.Graph) *symbolicWTOTape {
	t.Helper()
	tape, err := compileSymbolicWTOTape(graph)
	if err != nil {
		t.Fatal(err)
	}
	return tape
}

func assertSymbolicWTOEdge(t *testing.T, tape *symbolicWTOTape, from, to cfg.Point, kind symbolicWTOEdgeKind, component int32, exits, enters int) {
	t.Helper()
	fromIndex := tape.denseIndex(from)
	toIndex := tape.denseIndex(to)
	if fromIndex < 0 || toIndex < 0 {
		t.Fatalf("edge %d -> %d has missing endpoint", from, to)
	}
	point := tape.points[fromIndex]
	for _, edge := range tape.edges[point.edgeBegin:point.edgeEnd] {
		if edge.to != uint32(toIndex) {
			continue
		}
		if edge.kind != kind || edge.component != component || edge.exitCount != exits || edge.enterCount != enters {
			t.Fatalf("edge %d -> %d = %#v, want kind=%d component=%d exit=%d enter=%d", from, to, edge, kind, component, exits, enters)
		}
		return
	}
	t.Fatalf("edge %d -> %d absent", from, to)
}

func symbolicWTOTapeDigest(tape *symbolicWTOTape) string {
	var out strings.Builder
	for i, point := range tape.points {
		fmt.Fprintf(&out, "p%d=%d/c%d/h%d/e%d:%d;", i, point.point, point.component, point.headComponent, point.edgeBegin, point.edgeEnd)
	}
	for i, component := range tape.components {
		fmt.Fprintf(&out, "c%d=h%d/p%d/r%d:%d/d%d;", i, component.head, component.parent, component.begin, component.end, component.depth)
	}
	for _, edge := range tape.edges {
		fmt.Fprintf(&out, "%d>%d/k%d/c%d/x%d/n%d;", edge.from, edge.to, edge.kind, edge.component, edge.exitCount, edge.enterCount)
	}
	return out.String()
}

func assertNoMapFields(t *testing.T, typ reflect.Type, seen map[reflect.Type]bool) {
	t.Helper()
	if seen[typ] {
		return
	}
	seen[typ] = true
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i).Type
		for field.Kind() == reflect.Slice || field.Kind() == reflect.Array || field.Kind() == reflect.Pointer {
			field = field.Elem()
		}
		if field.Kind() == reflect.Map {
			t.Fatalf("%s.%s retains an execution-time map", typ, typ.Field(i).Name)
		}
		if field.Kind() == reflect.Struct {
			assertNoMapFields(t, field, seen)
		}
	}
}
