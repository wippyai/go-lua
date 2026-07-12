package boundaryeffects

import (
	"errors"
	"fmt"
	"math/rand"
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

const (
	lexLeft   symbol.ID = 101
	lexRight  symbol.ID = 102
	lexResult symbol.ID = 103
)

type fixture struct {
	graph  *cfg.CFG
	points Points
	facts  factflow.FactsInput
}

func makeFixture() fixture {
	g := cfg.New()
	branch := g.AddNode(cfg.NodeBranch)
	thenPoint := g.AddNode(cfg.NodeAssign)
	elsePoint := g.AddNode(cfg.NodeAssign)
	join := g.AddNode(cfg.NodeJoin)
	final := g.AddNode(cfg.NodeAssign)
	g.AddEdge(g.Entry(), branch, false)
	g.AddEdge(branch, thenPoint, true)
	g.AddEdge(branch, elsePoint, false)
	g.AddEdge(thenPoint, join, false)
	g.AddEdge(elsePoint, join, false)
	g.AddEdge(join, final, false)
	g.AddEdge(final, g.Exit(), false)
	left := pathdom.NewPath(lexLeft, "left").Field("value")
	right := pathdom.NewPath(lexRight, "right").Field("value")
	chosen := pathdom.NewPath(lexResult, "result").Field("chosen")
	last := pathdom.NewPath(lexResult, "result").Field("final")
	s1 := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: 1, HasExpr: true}
	s2 := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: 2, HasExpr: true}
	s3 := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: 3, HasExpr: true}
	facts := factflow.FactsInput{
		PathAssignments: map[cfg.Point]factflow.PathAssignment{
			thenPoint: factflow.NewPathAssignment(chosen, s1), elsePoint: factflow.NewPathAssignment(chosen, s2),
			final: factflow.NewPathAssignment(last, s3),
		},
		ExpressionPaths: map[factflow.ExprRef]pathdom.Path{1: left, 2: right, 3: chosen},
		BranchPathRelations: map[cfg.Point]factflow.BranchPathRelationSet{
			branch: factflow.NewBranchPathRelationSet(
				factflow.NewBranchPathEquality(left, right, true, false),
				factflow.NewBranchPathInequality(left, right, false, true),
			),
		},
	}
	return fixture{graph: g, points: Points{g.Entry(), branch, thenPoint, elsePoint, join, final, g.Exit()}, facts: facts}
}

type boundFixture struct {
	bound       Bound
	resolver    *visibility.Resolver
	facts       factflow.Facts
	left, right pathdom.Path
}

func bindCase(t testing.TB, fixture fixture, serial int) boundFixture {
	t.Helper()
	leftRoot := pathdom.NewPath(symbol.ID(1000+serial*3), fmt.Sprintf("callerLeft%d", serial))
	rightRoot := pathdom.NewPath(symbol.ID(1001+serial*3), fmt.Sprintf("callerRight%d", serial))
	resultRoot := pathdom.NewPath(symbol.ID(1002+serial*3), fmt.Sprintf("callerResult%d", serial))
	defs := []visibility.Definition{
		{Point: fixture.graph.Entry(), Symbol: leftRoot.Symbol, Root: leftRoot.Root},
		{Point: fixture.graph.Entry(), Symbol: rightRoot.Symbol, Root: rightRoot.Root},
		{Point: fixture.graph.Entry(), Symbol: resultRoot.Symbol, Root: resultRoot.Root},
	}
	resolver := visibility.NewResolver(visibility.BuildForward(visibility.BuildConfig{Graph: fixture.graph, Definitions: defs}))
	bindings, err := PackBindings(RootBinding{Left, leftRoot}, RootBinding{Right, rightRoot}, RootBinding{Output, resultRoot})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := CompileDiamond(fixture.points)
	if err != nil {
		t.Fatal(err)
	}
	bound, err := plan.Bind(bindings, resolver)
	if err != nil {
		t.Fatal(err)
	}
	left, right := bound.paths[0].path, bound.paths[1].path
	chosen, last := bound.paths[2].path, bound.paths[3].path
	s1 := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: 1, HasExpr: true}
	s2 := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: 2, HasExpr: true}
	s3 := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: 3, HasExpr: true}
	facts := factflow.NewFacts(factflow.FactsInput{
		PathAssignments: map[cfg.Point]factflow.PathAssignment{
			fixture.points.Then: factflow.NewPathAssignment(chosen, s1), fixture.points.Else: factflow.NewPathAssignment(chosen, s2),
			fixture.points.Final: factflow.NewPathAssignment(last, s3),
		},
		ExpressionPaths: map[factflow.ExprRef]pathdom.Path{1: left, 2: right, 3: chosen},
		BranchPathRelations: map[cfg.Point]factflow.BranchPathRelationSet{
			fixture.points.Branch: factflow.NewBranchPathRelationSet(
				factflow.NewBranchPathEquality(left, right, true, false),
				factflow.NewBranchPathInequality(left, right, false, true),
			),
		},
	})
	return boundFixture{bound: bound, resolver: resolver, facts: facts, left: left, right: right}
}

type sources struct {
	reg      *axis.Registry
	resolver *visibility.Resolver
	facts    factflow.Facts
}

func (s sources) ValueOfSource(point cfg.Point, source factflow.ValueSource, in state.State, read func(cfg.Point) state.State) (product.Value, bool) {
	path, ok := s.facts.ExpressionPath(source.ExprRef)
	if !ok {
		return product.Value{}, false
	}
	current := in
	if read != nil {
		current = read(point)
	}
	value := current.ReadPathKey(s.reg, s.resolver.KeySpace(), s.resolver.KeyAt(point, path))
	return value, !product.Equal(s.reg, value, product.Bottom(s.reg))
}

func oracle(f fixture, b boundFixture, reg *axis.Registry, entry state.State, lanes []state.LaneID) transfer.Result {
	s := sources{reg, b.resolver, b.facts}
	return transfer.Run(transfer.Config{
		Graph: f.graph, Registry: reg, EntryState: entry, StateLanes: lanes,
		NodeTransfer: factapply.NewFactsNodeTransfer(factapply.FactsNodeTransferConfig{Facts: b.facts, Sources: s, Visibility: b.resolver}),
		EdgeTransfer: factapply.NewFactsEdgeTransfer(factapply.FactsEdgeTransferConfig{Facts: b.facts, Sources: s, Visibility: b.resolver}),
	})
}

func entryState(reg *axis.Registry, b boundFixture, bottom state.State, left, right product.Value) state.State {
	return bottom.WritePathKey(reg, b.resolver.KeySpace(), b.bound.paths[0].key, left).
		WritePathKey(reg, b.resolver.KeySpace(), b.bound.paths[1].key, right)
}

func TestGuardedBoundaryEffectsDifferentialEveryPointRandomBindings(t *testing.T) {
	f := makeFixture()
	reg := standard.Registry()
	domain := state.Domain(reg)
	rng := rand.New(rand.NewSource(0xb017d))
	values := []product.Value{typevalue.LiteralString(reg, "same"), typevalue.LiteralString(reg, "other"), typevalue.LiteralInt(reg, 1), typevalue.LiteralInt(reg, 2)}
	for bindingCase := 0; bindingCase < 128; bindingCase++ {
		b := bindCase(t, f, bindingCase+1)
		for valueCase := 0; valueCase < 32; valueCase++ {
			entry := entryState(reg, b, domain.Bottom(), values[rng.Intn(len(values))], values[rng.Intn(len(values))])
			want := oracle(f, b, reg, entry, nil)
			got, err := b.bound.Execute(Config{Registry: reg, Resolver: b.resolver, Entry: entry})
			if err != nil {
				t.Fatalf("binding=%d values=%d: %v", bindingCase, valueCase, err)
			}
			for _, point := range f.graph.RPO() {
				if !domain.Equal(want[point], got[point]) {
					var differing []state.LaneID
					for _, lane := range state.DefaultLanes() {
						if !state.DomainWithLanes(reg, []state.LaneID{lane}).Equal(want[point], got[point]) {
							differing = append(differing, lane)
						}
					}
					t.Fatalf("binding=%d values=%d point=%d differs lanes=%v\nwant refs=%#v proofs=%#v\ngot refs=%#v proofs=%#v", bindingCase, valueCase, point, differing,
						want[point].PathRefinementsSnapshot(b.resolver.KeySpace()), want[point].BranchProofsSnapshot(b.resolver.KeySpace()),
						got[point].PathRefinementsSnapshot(b.resolver.KeySpace()), got[point].BranchProofsSnapshot(b.resolver.KeySpace()))
				}
			}
		}
	}
}

func TestDifferentialCoversEveryStateLane(t *testing.T) {
	f := makeFixture()
	reg := standard.Registry()
	b := bindCase(t, f, 1)
	full := state.Domain(reg)
	entry := entryState(reg, b, full.Bottom(), typevalue.LiteralString(reg, "same"), typevalue.LiteralString(reg, "same"))
	if len(state.DefaultLanes()) != 17 {
		t.Fatalf("default lane count=%d, want 17", len(state.DefaultLanes()))
	}
	for _, lane := range state.DefaultLanes() {
		lanes := []state.LaneID{lane}
		laneDomain := state.DomainWithLanes(reg, lanes)
		want := oracle(f, b, reg, entry, lanes)
		got, err := b.bound.Execute(Config{Registry: reg, Resolver: b.resolver, Entry: entry, StateLanes: lanes})
		if err != nil {
			t.Fatalf("lane %s: %v", lane, err)
		}
		for _, point := range f.graph.RPO() {
			if !laneDomain.Equal(want[point], got[point]) {
				t.Fatalf("lane %s point %d differs", lane, point)
			}
		}
	}
}

func TestStateDependentHeapAndProofInputsFailClosed(t *testing.T) {
	f := makeFixture()
	reg := standard.Registry()
	b := bindCase(t, f, 1)
	domain := state.Domain(reg)
	entry := entryState(reg, b, domain.Bottom(), typevalue.LiteralInt(reg, 1), typevalue.LiteralInt(reg, 1))
	entry = entry.AddBranchProof(pathevidence.BranchProof{Kind: pathevidence.BranchProofPathEqual, Path: b.bound.paths[0].local, Other: b.bound.paths[1].local})
	if _, err := b.bound.Execute(Config{Registry: reg, Resolver: b.resolver, Entry: entry}); !errors.Is(err, ErrStateDependent) {
		t.Fatalf("Execute error=%v, want ErrStateDependent", err)
	}
}

func TestBoundaryOnlyAndSparseObservationsMatchFullResult(t *testing.T) {
	f := makeFixture()
	reg := standard.Registry()
	b := bindCase(t, f, 7)
	domain := state.Domain(reg)
	entry := entryState(reg, b, domain.Bottom(), typevalue.LiteralString(reg, "same"), typevalue.LiteralString(reg, "other"))
	config := Config{Registry: reg, Resolver: b.resolver, Entry: entry}
	full, err := b.bound.Execute(config)
	if err != nil {
		t.Fatal(err)
	}
	exit, err := b.bound.ExecuteExit(config)
	if err != nil {
		t.Fatal(err)
	}
	if !domain.Equal(exit, full[f.points.Exit]) {
		t.Fatal("boundary-only exit differs from full result")
	}
	var sparse Observations
	plan := Observe(ObserveThen, ObserveJoin, ObserveExit)
	gotExit, err := b.bound.ExecuteObserved(config, plan, &sparse)
	if err != nil {
		t.Fatal(err)
	}
	if !domain.Equal(gotExit, exit) {
		t.Fatal("sparse execution changed exit")
	}
	checks := []struct {
		observation Observation
		point       cfg.Point
	}{{ObserveThen, f.points.Then}, {ObserveJoin, f.points.Join}, {ObserveExit, f.points.Exit}}
	for _, check := range checks {
		got, ok := sparse.Get(check.observation)
		if !ok || !domain.Equal(got, full[check.point]) {
			t.Fatalf("observation %d differs", check.observation)
		}
	}
	if _, ok := sparse.Get(ObserveBranch); ok {
		t.Fatal("unrequested branch observation was retained")
	}
}
