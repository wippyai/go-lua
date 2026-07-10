package dominance

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

type straightLineFixture struct {
	g          *cfg.CFG
	n1, n2, n3 cfg.Point
}

func buildStraightLine() straightLineFixture {
	g := cfg.New()
	n1 := g.AddNode(cfg.NodeAssign)
	n2 := g.AddNode(cfg.NodeAssign)
	n3 := g.AddNode(cfg.NodeAssign)

	g.AddEdge(g.Entry(), n1, false)
	g.AddEdge(n1, n2, false)
	g.AddEdge(n2, n3, false)
	g.AddEdge(n3, g.Exit(), false)

	return straightLineFixture{g: g, n1: n1, n2: n2, n3: n3}
}

type branchJoinFixture struct {
	g                          *cfg.CFG
	branch, thenN, elseN, join cfg.Point
}

func buildBranchJoin() branchJoinFixture {
	g := cfg.New()
	branch := g.AddNode(cfg.NodeBranch)
	thenN := g.AddNode(cfg.NodeAssign)
	elseN := g.AddNode(cfg.NodeAssign)
	join := g.AddNode(cfg.NodeJoin)

	g.AddEdge(g.Entry(), branch, false)
	g.AddEdge(branch, thenN, true)
	g.AddEdge(branch, elseN, false)
	g.AddEdge(thenN, join, false)
	g.AddEdge(elseN, join, false)
	g.AddEdge(join, g.Exit(), false)

	return branchJoinFixture{
		g:      g,
		branch: branch,
		thenN:  thenN,
		elseN:  elseN,
		join:   join,
	}
}

type loopFixture struct {
	g            *cfg.CFG
	header, body cfg.Point
}

func buildSimpleLoop() loopFixture {
	g := cfg.New()
	header := g.AddNode(cfg.NodeBranch)
	body := g.AddNode(cfg.NodeAssign)

	g.AddEdge(g.Entry(), header, false)
	g.AddEdge(header, body, true)
	g.AddEdge(header, g.Exit(), false)
	g.AddEdge(body, header, false)

	return loopFixture{g: g, header: header, body: body}
}

type nestedIfFixture struct {
	g                        *cfg.CFG
	branch1, branch2         cfg.Point
	n1, n2, n3, join1, join2 cfg.Point
}

func buildNestedIf() nestedIfFixture {
	g := cfg.New()
	branch1 := g.AddNode(cfg.NodeBranch)
	branch2 := g.AddNode(cfg.NodeBranch)
	n1 := g.AddNode(cfg.NodeAssign)
	n2 := g.AddNode(cfg.NodeAssign)
	n3 := g.AddNode(cfg.NodeAssign)
	join1 := g.AddNode(cfg.NodeJoin)
	join2 := g.AddNode(cfg.NodeJoin)

	g.AddEdge(g.Entry(), branch1, false)
	g.AddEdge(branch1, branch2, true)
	g.AddEdge(branch1, n3, false)
	g.AddEdge(branch2, n1, true)
	g.AddEdge(branch2, n2, false)
	g.AddEdge(n1, join1, false)
	g.AddEdge(n2, join1, false)
	g.AddEdge(join1, join2, false)
	g.AddEdge(n3, join2, false)
	g.AddEdge(join2, g.Exit(), false)

	return nestedIfFixture{
		g:       g,
		branch1: branch1,
		branch2: branch2,
		n1:      n1,
		n2:      n2,
		n3:      n3,
		join1:   join1,
		join2:   join2,
	}
}

func TestComputeDominatorsStraightLine(t *testing.T) {
	f := buildStraightLine()
	idom, domTree := ComputeDominators(f.g)

	want := map[cfg.Point]cfg.Point{
		f.g.Entry(): f.g.Entry(),
		f.n1:        f.g.Entry(),
		f.n2:        f.n1,
		f.n3:        f.n2,
		f.g.Exit():  f.n3,
	}
	assertImmediateDominators(t, idom, want)

	if got := domTree[f.g.Entry()]; len(got) != 1 || got[0] != f.n1 {
		t.Fatalf("domTree[entry] = %v, want [%d]", got, f.n1)
	}
	if got := domTree[f.g.Exit()]; len(got) != 0 {
		t.Fatalf("domTree[exit] = %v, want empty", got)
	}
}

func TestComputeDominatorsBranchJoin(t *testing.T) {
	f := buildBranchJoin()
	idom, _ := ComputeDominators(f.g)

	want := map[cfg.Point]cfg.Point{
		f.g.Entry(): f.g.Entry(),
		f.branch:    f.g.Entry(),
		f.thenN:     f.branch,
		f.elseN:     f.branch,
		f.join:      f.branch,
		f.g.Exit():  f.join,
	}
	assertImmediateDominators(t, idom, want)
}

func TestComputeDominanceFrontierBranchJoin(t *testing.T) {
	f := buildBranchJoin()
	idom, _ := ComputeDominators(f.g)
	df := ComputeDominanceFrontier(f.g, idom)

	assertFrontier(t, df, f.thenN, []cfg.Point{f.join})
	assertFrontier(t, df, f.elseN, []cfg.Point{f.join})
	assertFrontier(t, df, f.g.Entry(), nil)
	assertFrontier(t, df, f.branch, nil)
}

func TestComputeDominatorsSimpleLoop(t *testing.T) {
	f := buildSimpleLoop()
	idom, _ := ComputeDominators(f.g)

	want := map[cfg.Point]cfg.Point{
		f.g.Entry(): f.g.Entry(),
		f.header:    f.g.Entry(),
		f.body:      f.header,
		f.g.Exit():  f.header,
	}
	assertImmediateDominators(t, idom, want)
}

func TestComputeDominanceFrontierSimpleLoop(t *testing.T) {
	f := buildSimpleLoop()
	idom, _ := ComputeDominators(f.g)
	df := ComputeDominanceFrontier(f.g, idom)

	assertFrontier(t, df, f.body, []cfg.Point{f.header})
	assertFrontier(t, df, f.header, []cfg.Point{f.header})
}

func TestComputeDominatorsNestedIf(t *testing.T) {
	f := buildNestedIf()
	idom, _ := ComputeDominators(f.g)

	want := map[cfg.Point]cfg.Point{
		f.g.Entry(): f.g.Entry(),
		f.branch1:   f.g.Entry(),
		f.branch2:   f.branch1,
		f.n1:        f.branch2,
		f.n2:        f.branch2,
		f.n3:        f.branch1,
		f.join1:     f.branch2,
		f.join2:     f.branch1,
		f.g.Exit():  f.join2,
	}
	assertImmediateDominators(t, idom, want)
}

func TestDeterminism(t *testing.T) {
	for run := range 5 {
		f := buildNestedIf()
		idom1, domTree1 := ComputeDominators(f.g)
		df1 := ComputeDominanceFrontier(f.g, idom1)

		idom2, domTree2 := ComputeDominators(f.g)
		df2 := ComputeDominanceFrontier(f.g, idom2)

		if len(idom1) != len(idom2) {
			t.Fatalf("run %d: idom lengths differ: %d vs %d", run, len(idom1), len(idom2))
		}
		for point, dom := range idom1 {
			if idom2[point] != dom {
				t.Fatalf("run %d: idom[%d] = %d, want %d", run, point, idom2[point], dom)
			}
		}

		for point, children1 := range domTree1 {
			assertPointSlice(t, domTree2[point], children1, "domTree")
		}
		for point, frontier1 := range df1 {
			assertPointSlice(t, df2[point], frontier1, "dominance frontier")
		}
	}
}

func TestDominates(t *testing.T) {
	f := buildBranchJoin()
	idom, _ := ComputeDominators(f.g)

	for _, point := range f.g.RPO() {
		if !Dominates(idom, f.g.Entry(), point) {
			t.Fatalf("entry should dominate %d", point)
		}
	}
	if !Dominates(idom, f.branch, f.join) {
		t.Fatal("branch should dominate join")
	}
	if Dominates(idom, f.join, f.branch) {
		t.Fatal("join should not dominate branch")
	}
	if Dominates(idom, f.g.Exit(), f.g.Entry()) {
		t.Fatal("exit should not dominate entry")
	}
}

func TestStrictlyDominates(t *testing.T) {
	f := buildStraightLine()
	idom, _ := ComputeDominators(f.g)

	for _, point := range f.g.RPO() {
		if point == f.g.Entry() {
			if StrictlyDominates(idom, f.g.Entry(), point) {
				t.Fatal("entry should not strictly dominate itself")
			}
			continue
		}
		if !StrictlyDominates(idom, f.g.Entry(), point) {
			t.Fatalf("entry should strictly dominate %d", point)
		}
	}
}

func TestImmediateDominatorInfoMatchesMapPredicates(t *testing.T) {
	f := buildNestedIf()
	info := ComputeImmediateDominatorInfo(f.g)
	idom := ComputeImmediateDominators(f.g)

	for _, a := range f.g.RPO() {
		for _, b := range f.g.RPO() {
			if got, want := info.Dominates(a, b), Dominates(idom, a, b); got != want {
				t.Fatalf("Dominates(%d, %d) = %v, want %v", a, b, got, want)
			}
			if got, want := info.StrictlyDominates(a, b), StrictlyDominates(idom, a, b); got != want {
				t.Fatalf("StrictlyDominates(%d, %d) = %v, want %v", a, b, got, want)
			}
		}
	}
}

func TestComputePostDominatorsLinear(t *testing.T) {
	f := buildStraightLine()
	postIDom, _ := ComputePostDominators(f.g)
	rpo := f.g.RPO()

	for _, point := range rpo {
		if !PostDominates(postIDom, f.g.Exit(), point) {
			t.Fatalf("exit should post-dominate %d", point)
		}
	}
	for i := 0; i < len(rpo)-1; i++ {
		if !PostDominates(postIDom, rpo[i+1], rpo[i]) {
			t.Fatalf("%d should post-dominate %d", rpo[i+1], rpo[i])
		}
	}
}

func TestComputePostDominatorsBranchJoin(t *testing.T) {
	f := buildBranchJoin()
	postIDom, _ := ComputePostDominators(f.g)

	if !PostDominates(postIDom, f.join, f.thenN) {
		t.Fatal("join should post-dominate then branch")
	}
	if !PostDominates(postIDom, f.join, f.elseN) {
		t.Fatal("join should post-dominate else branch")
	}
	if !PostDominates(postIDom, f.join, f.branch) {
		t.Fatal("join should post-dominate branch")
	}
	for _, point := range f.g.RPO() {
		if !PostDominates(postIDom, f.g.Exit(), point) {
			t.Fatalf("exit should post-dominate %d", point)
		}
	}
	if PostDominates(postIDom, f.branch, f.join) {
		t.Fatal("branch should not post-dominate join")
	}
	if PostDominates(postIDom, f.g.Entry(), f.g.Exit()) {
		t.Fatal("entry should not post-dominate exit")
	}
}

func TestComputeDomInfo(t *testing.T) {
	f := buildBranchJoin()
	info := ComputeDomInfo(f.g)

	if info == nil {
		t.Fatal("ComputeDomInfo returned nil")
	}
	if info.ImmediateDominators == nil {
		t.Fatal("ImmediateDominators is nil")
	}
	if info.DominatorTree == nil {
		t.Fatal("DominatorTree is nil")
	}
	if info.DominanceFrontier == nil {
		t.Fatal("DominanceFrontier is nil")
	}
}

func TestEmptyGraph(t *testing.T) {
	g := cfg.New()
	idom, domTree := ComputeDominators(g)
	df := ComputeDominanceFrontier(g, idom)

	if idom[g.Entry()] != g.Entry() {
		t.Fatalf("idom[entry] = %d, want %d", idom[g.Entry()], g.Entry())
	}
	if len(domTree) != 0 {
		t.Fatalf("domTree = %v, want empty", domTree)
	}
	if len(df) != 0 {
		t.Fatalf("dominance frontier = %v, want empty", df)
	}
}

func assertImmediateDominators(t *testing.T, got, want map[cfg.Point]cfg.Point) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("idom length = %d, want %d; got %v", len(got), len(want), got)
	}
	for point, wantDom := range want {
		if got[point] != wantDom {
			t.Fatalf("idom[%d] = %d, want %d; got %v", point, got[point], wantDom, got)
		}
	}
}

func assertFrontier(t *testing.T, df map[cfg.Point][]cfg.Point, point cfg.Point, want []cfg.Point) {
	t.Helper()
	assertPointSlice(t, df[point], want, "dominance frontier")
}

func assertPointSlice(t *testing.T, got, want []cfg.Point, label string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s length = %d, want %d; got %v want %v", label, len(got), len(want), got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("%s[%d] = %d, want %d; got %v want %v", label, i, got[i], want[i], got, want)
		}
	}
}
