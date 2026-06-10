package cfg

import "testing"

func TestNew(t *testing.T) {
	c := New()

	t.Run("initial size", func(t *testing.T) {
		if c.Size() != 2 {
			t.Errorf("expected 2 nodes (entry+exit), got %d", c.Size())
		}
	})

	t.Run("entry node", func(t *testing.T) {
		n := c.Node(c.Entry())

		if n == nil {
			t.Fatal("entry node is nil")
		}

		if n.Kind != NodeEntry {
			t.Errorf("entry kind = %d, want %d", n.Kind, NodeEntry)
		}

		if n.Point != c.Entry() {
			t.Errorf("entry point = %d, want %d", n.Point, c.Entry())
		}
	})

	t.Run("exit node", func(t *testing.T) {
		n := c.Node(c.Exit())

		if n == nil {
			t.Fatal("exit node is nil")
		}

		if n.Kind != NodeExit {
			t.Errorf("exit kind = %d, want %d", n.Kind, NodeExit)
		}
	})

	t.Run("entry before exit", func(t *testing.T) {
		if c.Entry() >= c.Exit() {
			t.Errorf("entry (%d) should be before exit (%d)", c.Entry(), c.Exit())
		}
	})

	t.Run("no initial edges", func(t *testing.T) {
		if len(c.Edges()) != 0 {
			t.Errorf("expected no edges, got %d", len(c.Edges()))
		}
	})
}

func TestAddNode(t *testing.T) {
	tests := []struct {
		name string
		kind NodeKind
	}{
		{"entry", NodeEntry},
		{"exit", NodeExit},
		{"assign", NodeAssign},
		{"call", NodeCall},
		{"branch", NodeBranch},
		{"join", NodeJoin},
		{"return", NodeReturn},
		{"scope enter", NodeScopeEnter},
		{"scope exit", NodeScopeExit},
		{"noop", NodeNoop},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := New()
			initialSize := c.Size()
			p := c.AddNode(tt.kind)

			if c.Size() != initialSize+1 {
				t.Errorf("size = %d, want %d", c.Size(), initialSize+1)
			}

			n := c.Node(p)
			if n == nil {
				t.Fatal("node is nil")
			}

			if n.Kind != tt.kind {
				t.Errorf("kind = %d, want %d", n.Kind, tt.kind)
			}

			if n.Point != p {
				t.Errorf("point = %d, want %d", n.Point, p)
			}
		})
	}
}

func TestNode(t *testing.T) {
	c := New()
	p := c.AddNode(NodeAssign)

	tests := []struct {
		name  string
		point Point
		isNil bool
	}{
		{"valid entry", c.Entry(), false},
		{"valid exit", c.Exit(), false},
		{"valid added", p, false},
		{"out of bounds", Point(100), true},
		{"just past end", Point(c.Size()), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := c.Node(tt.point)

			if tt.isNil && n != nil {
				t.Errorf("expected nil, got %+v", n)
			}

			if !tt.isNil && n == nil {
				t.Error("expected non-nil node")
			}
		})
	}
}

func TestAddEdge(t *testing.T) {
	t.Run("single edge", func(t *testing.T) {
		c := New()
		p := c.AddNode(NodeAssign)
		c.AddEdge(c.Entry(), p, false)

		edges := c.Edges()
		if len(edges) != 1 {
			t.Fatalf("edges = %d, want 1", len(edges))
		}

		if edges[0].From != c.Entry() || edges[0].To != p {
			t.Errorf("edge = %+v, want {%d, %d, false}", edges[0], c.Entry(), p)
		}
	})

	t.Run("conditional edges", func(t *testing.T) {
		c := New()
		branch := c.AddNode(NodeBranch)
		then := c.AddNode(NodeAssign)
		els := c.AddNode(NodeAssign)

		c.AddEdge(branch, then, true)
		c.AddEdge(branch, els, false)

		edges := c.Edges()

		if len(edges) != 2 {
			t.Fatalf("edges = %d, want 2", len(edges))
		}

		if !edges[0].Cond {
			t.Error("first edge should be conditional")
		}

		if edges[1].Cond {
			t.Error("second edge should not be conditional")
		}
	})

	t.Run("updates predecessors", func(t *testing.T) {
		c := New()
		p := c.AddNode(NodeAssign)
		c.AddEdge(c.Entry(), p, false)

		preds := c.Predecessors(p)
		if len(preds) != 1 || preds[0] != c.Entry() {
			t.Errorf("predecessors = %v, want [%d]", preds, c.Entry())
		}
	})

	t.Run("updates successors", func(t *testing.T) {
		c := New()
		p := c.AddNode(NodeAssign)
		c.AddEdge(c.Entry(), p, false)

		succs := c.Successors(c.Entry())
		if len(succs) != 1 || succs[0] != p {
			t.Errorf("successors = %v, want [%d]", succs, p)
		}
	})
}

func TestPredecessors(t *testing.T) {
	t.Run("no predecessors", func(t *testing.T) {
		c := New()
		preds := c.Predecessors(c.Entry())

		if len(preds) != 0 {
			t.Errorf("entry should have no predecessors, got %v", preds)
		}
	})

	t.Run("single predecessor", func(t *testing.T) {
		c := New()
		p := c.AddNode(NodeAssign)
		c.AddEdge(c.Entry(), p, false)

		preds := c.Predecessors(p)
		if len(preds) != 1 {
			t.Fatalf("predecessors = %d, want 1", len(preds))
		}

		if preds[0] != c.Entry() {
			t.Errorf("predecessor = %d, want %d", preds[0], c.Entry())
		}
	})

	t.Run("multiple predecessors", func(t *testing.T) {
		c := New()
		left := c.AddNode(NodeAssign)
		right := c.AddNode(NodeAssign)
		join := c.AddNode(NodeJoin)

		c.AddEdge(left, join, false)
		c.AddEdge(right, join, false)

		preds := c.Predecessors(join)
		if len(preds) != 2 {
			t.Fatalf("predecessors = %d, want 2", len(preds))
		}
	})
}

func TestSuccessors(t *testing.T) {
	t.Run("no successors", func(t *testing.T) {
		c := New()
		succs := c.Successors(c.Exit())

		if len(succs) != 0 {
			t.Errorf("exit should have no successors, got %v", succs)
		}
	})

	t.Run("single successor", func(t *testing.T) {
		c := New()
		p := c.AddNode(NodeAssign)
		c.AddEdge(c.Entry(), p, false)

		succs := c.Successors(c.Entry())
		if len(succs) != 1 || succs[0] != p {
			t.Errorf("successors = %v, want [%d]", succs, p)
		}
	})

	t.Run("multiple successors", func(t *testing.T) {
		c := New()
		branch := c.AddNode(NodeBranch)
		left := c.AddNode(NodeAssign)
		right := c.AddNode(NodeAssign)

		c.AddEdge(branch, left, true)
		c.AddEdge(branch, right, false)

		succs := c.Successors(branch)
		if len(succs) != 2 {
			t.Fatalf("successors = %d, want 2", len(succs))
		}
	})
}

func TestIsJoin(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(*CFG) Point
		expected bool
	}{
		{
			"no predecessors",
			func(c *CFG) Point { return c.Entry() },
			false,
		},
		{
			"single predecessor",
			func(c *CFG) Point {
				p := c.AddNode(NodeAssign)
				c.AddEdge(c.Entry(), p, false)
				return p
			},
			false,
		},
		{
			"two predecessors",
			func(c *CFG) Point {
				left := c.AddNode(NodeAssign)
				right := c.AddNode(NodeAssign)
				join := c.AddNode(NodeJoin)
				c.AddEdge(left, join, false)
				c.AddEdge(right, join, false)
				return join
			},
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := New()
			p := tt.setup(c)

			if c.IsJoin(p) != tt.expected {
				t.Errorf("IsJoin = %v, want %v", c.IsJoin(p), tt.expected)
			}
		})
	}
}

func TestIsBranch(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(*CFG) Point
		expected bool
	}{
		{
			"no successors",
			func(c *CFG) Point { return c.Exit() },
			false,
		},
		{
			"single successor",
			func(c *CFG) Point {
				p := c.AddNode(NodeAssign)
				c.AddEdge(c.Entry(), p, false)
				return c.Entry()
			},
			false,
		},
		{
			"two successors",
			func(c *CFG) Point {
				branch := c.AddNode(NodeBranch)
				left := c.AddNode(NodeAssign)
				right := c.AddNode(NodeAssign)
				c.AddEdge(branch, left, true)
				c.AddEdge(branch, right, false)
				return branch
			},
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := New()
			p := tt.setup(c)

			if c.IsBranch(p) != tt.expected {
				t.Errorf("IsBranch = %v, want %v", c.IsBranch(p), tt.expected)
			}
		})
	}
}

func TestEdges(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		c := New()
		if len(c.Edges()) != 0 {
			t.Errorf("edges = %d, want 0", len(c.Edges()))
		}
	})

	t.Run("returns all edges", func(t *testing.T) {
		c := New()
		p1 := c.AddNode(NodeAssign)
		p2 := c.AddNode(NodeAssign)

		c.AddEdge(c.Entry(), p1, false)
		c.AddEdge(p1, p2, false)
		c.AddEdge(p2, c.Exit(), false)

		edges := c.Edges()
		if len(edges) != 3 {
			t.Errorf("edges = %d, want 3", len(edges))
		}
	})
}

func TestSize(t *testing.T) {
	c := New()
	if c.Size() != 2 {
		t.Errorf("initial size = %d, want 2", c.Size())
	}

	for i := 0; i < 5; i++ {
		c.AddNode(NodeAssign)

		if c.Size() != 3+i {
			t.Errorf("after %d adds, size = %d, want %d", i+1, c.Size(), 3+i)
		}
	}
}

func TestLinearCFG(t *testing.T) {
	c := New()
	nodes := make([]Point, 5)

	for i := range nodes {
		nodes[i] = c.AddNode(NodeAssign)
	}

	c.AddEdge(c.Entry(), nodes[0], false)

	for i := 0; i < len(nodes)-1; i++ {
		c.AddEdge(nodes[i], nodes[i+1], false)
	}

	c.AddEdge(nodes[len(nodes)-1], c.Exit(), false)

	t.Run("size", func(t *testing.T) {
		if c.Size() != 7 {
			t.Errorf("size = %d, want 7", c.Size())
		}
	})

	t.Run("each node has one pred/succ", func(t *testing.T) {
		for i, n := range nodes {
			if len(c.Predecessors(n)) != 1 {
				t.Errorf("node %d: predecessors = %d, want 1", i, len(c.Predecessors(n)))
			}

			if len(c.Successors(n)) != 1 {
				t.Errorf("node %d: successors = %d, want 1", i, len(c.Successors(n)))
			}
		}
	})

	t.Run("no joins or branches", func(t *testing.T) {
		for i, n := range nodes {
			if c.IsJoin(n) {
				t.Errorf("node %d is join", i)
			}

			if c.IsBranch(n) {
				t.Errorf("node %d is branch", i)
			}
		}
	})
}

func TestDiamondCFG(t *testing.T) {
	c := New()
	branch := c.AddNode(NodeBranch)
	left := c.AddNode(NodeAssign)
	right := c.AddNode(NodeAssign)
	join := c.AddNode(NodeJoin)

	c.AddEdge(c.Entry(), branch, false)
	c.AddEdge(branch, left, true)
	c.AddEdge(branch, right, false)
	c.AddEdge(left, join, false)
	c.AddEdge(right, join, false)
	c.AddEdge(join, c.Exit(), false)

	t.Run("branch point", func(t *testing.T) {
		if !c.IsBranch(branch) {
			t.Error("branch not detected")
		}

		succs := c.Successors(branch)

		if len(succs) != 2 {
			t.Errorf("branch successors = %d, want 2", len(succs))
		}
	})

	t.Run("join point", func(t *testing.T) {
		if !c.IsJoin(join) {
			t.Error("join not detected")
		}

		preds := c.Predecessors(join)

		if len(preds) != 2 {
			t.Errorf("join predecessors = %d, want 2", len(preds))
		}
	})

	t.Run("conditional edges", func(t *testing.T) {
		edges := c.Edges()

		var foundThen, foundElse bool

		for _, e := range edges {
			if e.From == branch && e.To == left && e.Cond {
				foundThen = true
			}

			if e.From == branch && e.To == right && !e.Cond {
				foundElse = true
			}
		}

		if !foundThen {
			t.Error("then edge not found")
		}

		if !foundElse {
			t.Error("else edge not found")
		}
	})
}

func TestNestedLoopCFG(t *testing.T) {
	c := New()

	outerHeader := c.AddNode(NodeBranch)
	innerHeader := c.AddNode(NodeBranch)
	body := c.AddNode(NodeAssign)
	innerJoin := c.AddNode(NodeJoin)
	outerJoin := c.AddNode(NodeJoin)

	c.AddEdge(c.Entry(), outerHeader, false)
	c.AddEdge(outerHeader, innerHeader, true)
	c.AddEdge(outerHeader, outerJoin, false)
	c.AddEdge(innerHeader, body, true)
	c.AddEdge(innerHeader, innerJoin, false)
	c.AddEdge(body, innerHeader, false)
	c.AddEdge(innerJoin, outerHeader, false)
	c.AddEdge(outerJoin, c.Exit(), false)

	t.Run("outer header is branch", func(t *testing.T) {
		if !c.IsBranch(outerHeader) {
			t.Error("outer header should be branch")
		}
	})

	t.Run("inner header is branch", func(t *testing.T) {
		if !c.IsBranch(innerHeader) {
			t.Error("inner header should be branch")
		}
	})

	t.Run("back edges exist", func(t *testing.T) {
		edges := c.Edges()

		var hasInnerBack, hasOuterBack bool

		for _, e := range edges {
			if e.From == body && e.To == innerHeader {
				hasInnerBack = true
			}

			if e.From == innerJoin && e.To == outerHeader {
				hasOuterBack = true
			}
		}

		if !hasInnerBack {
			t.Error("inner loop back edge not found")
		}

		if !hasOuterBack {
			t.Error("outer loop back edge not found")
		}
	})
}

func TestMultipleExits(t *testing.T) {
	c := New()

	check := c.AddNode(NodeBranch)
	earlyReturn := c.AddNode(NodeReturn)
	normal := c.AddNode(NodeAssign)
	normalReturn := c.AddNode(NodeReturn)

	c.AddEdge(c.Entry(), check, false)
	c.AddEdge(check, earlyReturn, true)
	c.AddEdge(check, normal, false)
	c.AddEdge(earlyReturn, c.Exit(), false)
	c.AddEdge(normal, normalReturn, false)
	c.AddEdge(normalReturn, c.Exit(), false)

	preds := c.Predecessors(c.Exit())
	if len(preds) != 2 {
		t.Errorf("exit predecessors = %d, want 2", len(preds))
	}

	if !c.IsJoin(c.Exit()) {
		t.Error("exit should be join point with multiple returns")
	}
}

func TestEdgeCond(t *testing.T) {
	c := New()

	branch := c.AddNode(NodeBranch)
	thenBlock := c.AddNode(NodeAssign)
	elseBlock := c.AddNode(NodeAssign)

	c.AddEdge(c.Entry(), branch, false)
	c.AddEdge(branch, thenBlock, true)  // then-branch
	c.AddEdge(branch, elseBlock, false) // else-branch
	c.AddEdge(thenBlock, c.Exit(), false)
	c.AddEdge(elseBlock, c.Exit(), false)

	t.Run("then edge", func(t *testing.T) {
		cond, ok := c.EdgeCond(branch, thenBlock)

		if !ok {
			t.Fatal("edge not found")
		}

		if !cond {
			t.Error("expected true for then-branch")
		}
	})

	t.Run("else edge", func(t *testing.T) {
		cond, ok := c.EdgeCond(branch, elseBlock)

		if !ok {
			t.Fatal("edge not found")
		}

		if cond {
			t.Error("expected false for else-branch")
		}
	})

	t.Run("non-existent edge", func(t *testing.T) {
		_, ok := c.EdgeCond(thenBlock, branch)
		if ok {
			t.Error("expected not found for non-existent edge")
		}
	})
}

func TestAddBranch(t *testing.T) {
	c := New()

	branch := c.AddBranch()

	t.Run("returns valid point", func(t *testing.T) {
		n := c.Node(branch)
		if n == nil {
			t.Error("expected valid point with corresponding node")
		}
	})

	t.Run("node is branch", func(t *testing.T) {
		n := c.Node(branch)
		if n.Kind != NodeBranch {
			t.Errorf("expected NodeBranch, got %d", n.Kind)
		}
	})

	t.Run("increments size", func(t *testing.T) {
		before := c.Size()
		c.AddBranch()
		after := c.Size()

		if after != before+1 {
			t.Errorf("size = %d, want %d", after, before+1)
		}
	})
}

func TestSuccessor(t *testing.T) {
	c := New()
	n1 := c.AddNode(NodeAssign)
	n2 := c.AddNode(NodeAssign)
	c.AddEdge(n1, n2, false)

	t.Run("returns single successor", func(t *testing.T) {
		succ := c.Successor(n1)
		if succ != n2 {
			t.Errorf("Successor(%d) = %d, want %d", n1, succ, n2)
		}
	})

	t.Run("returns self when no successors", func(t *testing.T) {
		succ := c.Successor(n2)
		if succ != n2 {
			t.Errorf("Successor(%d) = %d, want %d (self)", n2, succ, n2)
		}
	})
}

func TestID(t *testing.T) {
	t.Run("nil CFG returns 0", func(t *testing.T) {
		var c *CFG
		if c.ID() != 0 {
			t.Errorf("nil CFG.ID() = %d, want 0", c.ID())
		}
	})

	t.Run("each CFG has unique ID", func(t *testing.T) {
		c1 := New()
		c2 := New()
		if c1.ID() == c2.ID() {
			t.Errorf("two CFGs should have different IDs: %d == %d", c1.ID(), c2.ID())
		}
	})

	t.Run("ID is stable", func(t *testing.T) {
		c := New()
		id1 := c.ID()
		id2 := c.ID()
		if id1 != id2 {
			t.Errorf("ID should be stable: %d != %d", id1, id2)
		}
	})
}

func TestNilCFG(t *testing.T) {
	var c *CFG

	t.Run("Entry returns 0", func(t *testing.T) {
		if c.Entry() != 0 {
			t.Errorf("nil.Entry() = %d, want 0", c.Entry())
		}
	})

	t.Run("Exit returns 0", func(t *testing.T) {
		if c.Exit() != 0 {
			t.Errorf("nil.Exit() = %d, want 0", c.Exit())
		}
	})

	t.Run("ID returns 0", func(t *testing.T) {
		if c.ID() != 0 {
			t.Errorf("nil.ID() = %d, want 0", c.ID())
		}
	})

}

func TestRPO(t *testing.T) {
	t.Run("simple linear CFG", func(t *testing.T) {
		c := New()
		n1 := c.AddNode(NodeAssign)
		n2 := c.AddNode(NodeAssign)
		c.AddEdge(c.Entry(), n1, false)
		c.AddEdge(n1, n2, false)
		c.AddEdge(n2, c.Exit(), false)

		rpo := c.RPO()
		if len(rpo) == 0 {
			t.Fatal("RPO should not be empty")
		}

		// Entry should be first in RPO
		if rpo[0] != c.Entry() {
			t.Errorf("first node in RPO should be entry, got %d", rpo[0])
		}
	})

	t.Run("branch CFG", func(t *testing.T) {
		c := New()
		branch := c.AddBranch()
		thenN := c.AddNode(NodeAssign)
		elseN := c.AddNode(NodeAssign)
		join := c.AddNode(NodeJoin)

		c.AddEdge(c.Entry(), branch, false)
		c.AddEdge(branch, thenN, true)
		c.AddEdge(branch, elseN, false)
		c.AddEdge(thenN, join, false)
		c.AddEdge(elseN, join, false)
		c.AddEdge(join, c.Exit(), false)

		rpo := c.RPO()

		// Should include all nodes
		if len(rpo) != 6 {
			t.Errorf("RPO should have 6 nodes, got %d", len(rpo))
		}

		// Entry first
		if rpo[0] != c.Entry() {
			t.Errorf("entry should be first in RPO")
		}
	})
}

func TestRPO_ReturnValueIsIndependentSlice(t *testing.T) {
	c := New()
	n1 := c.AddNode(NodeAssign)
	n2 := c.AddNode(NodeAssign)
	c.AddEdge(c.Entry(), n1, false)
	c.AddEdge(n1, n2, false)
	c.AddEdge(n2, c.Exit(), false)

	first := c.RPO()
	if len(first) == 0 {
		t.Fatal("expected non-empty RPO")
	}
	origHead := first[0]
	first[0] = c.Exit()

	second := c.RPO()
	if len(second) == 0 {
		t.Fatal("expected non-empty RPO on second call")
	}
	if second[0] != origHead {
		t.Fatalf("expected second RPO head %d, got %d", origHead, second[0])
	}
}

func TestRPO_InvalidatesOnGraphMutation(t *testing.T) {
	c := New()
	n1 := c.AddNode(NodeAssign)
	c.AddEdge(c.Entry(), n1, false)
	c.AddEdge(n1, c.Exit(), false)

	before := c.RPO()
	if len(before) != 3 {
		t.Fatalf("expected 3 points before mutation, got %d", len(before))
	}

	n2 := c.AddNode(NodeAssign)
	c.AddEdge(n1, n2, false)
	c.AddEdge(n2, c.Exit(), false)

	after := c.RPO()
	found := false
	for _, p := range after {
		if p == n2 {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected mutated graph RPO to include node %d", n2)
	}
}

// Benchmarks

func BenchmarkNew(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = New()
	}
}

func BenchmarkAddNode(b *testing.B) {
	c := New()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.AddNode(NodeAssign)
	}
}

func BenchmarkAddEdge(b *testing.B) {
	c := New()
	nodes := make([]Point, b.N+1)
	for i := range nodes {
		nodes[i] = c.AddNode(NodeAssign)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.AddEdge(nodes[i], nodes[i+1], false)
	}
}

func BenchmarkPredecessors(b *testing.B) {
	c := New()
	n1 := c.AddNode(NodeAssign)
	n2 := c.AddNode(NodeAssign)
	c.AddEdge(c.Entry(), n1, false)
	c.AddEdge(n1, n2, false)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = c.Predecessors(n2)
	}
}

func BenchmarkSuccessors(b *testing.B) {
	c := New()
	n1 := c.AddNode(NodeAssign)
	n2 := c.AddNode(NodeAssign)
	c.AddEdge(c.Entry(), n1, false)
	c.AddEdge(n1, n2, false)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = c.Successors(n1)
	}
}

func BenchmarkRPO(b *testing.B) {
	c := buildLargeCFG(100)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = c.RPO()
	}
}

func BenchmarkBuildLinearCFG(b *testing.B) {
	for i := 0; i < b.N; i++ {
		c := New()
		prev := c.Entry()
		for j := 0; j < 50; j++ {
			n := c.AddNode(NodeAssign)
			c.AddEdge(prev, n, false)
			prev = n
		}
		c.AddEdge(prev, c.Exit(), false)
	}
}

func BenchmarkBuildDiamondCFG(b *testing.B) {
	for i := 0; i < b.N; i++ {
		c := New()
		prev := c.Entry()
		for j := 0; j < 20; j++ {
			branch := c.AddBranch()
			thenN := c.AddNode(NodeAssign)
			elseN := c.AddNode(NodeAssign)
			join := c.AddNode(NodeJoin)
			c.AddEdge(prev, branch, false)
			c.AddEdge(branch, thenN, true)
			c.AddEdge(branch, elseN, false)
			c.AddEdge(thenN, join, false)
			c.AddEdge(elseN, join, false)
			prev = join
		}
		c.AddEdge(prev, c.Exit(), false)
	}
}

func BenchmarkLargeCFGBuild(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = buildLargeCFG(200)
	}
}

func buildLargeCFG(n int) *CFG {
	c := New()
	prev := c.Entry()
	for i := 0; i < n; i++ {
		if i%5 == 0 {
			branch := c.AddBranch()
			thenN := c.AddNode(NodeAssign)
			elseN := c.AddNode(NodeAssign)
			join := c.AddNode(NodeJoin)
			c.AddEdge(prev, branch, false)
			c.AddEdge(branch, thenN, true)
			c.AddEdge(branch, elseN, false)
			c.AddEdge(thenN, join, false)
			c.AddEdge(elseN, join, false)
			prev = join
		} else {
			node := c.AddNode(NodeAssign)
			c.AddEdge(prev, node, false)
			prev = node
		}
	}
	c.AddEdge(prev, c.Exit(), false)
	return c
}
