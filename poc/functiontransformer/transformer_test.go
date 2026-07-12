package functiontransformer

import (
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
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

const (
	lexicalLeft   symbol.ID = 101
	lexicalRight  symbol.ID = 102
	lexicalResult symbol.ID = 103
)

type fixture struct {
	graph                   *cfg.CFG
	branch, then, otherwise cfg.Point
	join, finalPoint        cfg.Point
	input                   Input
	left, right, chosen     pathdom.Path
	final                   pathdom.Path
}

func newFixture() fixture {
	graph := cfg.New()
	branch := graph.AddNode(cfg.NodeBranch)
	thenPoint := graph.AddNode(cfg.NodeAssign)
	elsePoint := graph.AddNode(cfg.NodeAssign)
	join := graph.AddNode(cfg.NodeJoin)
	finalPoint := graph.AddNode(cfg.NodeAssign)
	graph.AddEdge(graph.Entry(), branch, false)
	graph.AddEdge(branch, thenPoint, true)
	graph.AddEdge(branch, elsePoint, false)
	graph.AddEdge(thenPoint, join, false)
	graph.AddEdge(elsePoint, join, false)
	graph.AddEdge(join, finalPoint, false)
	graph.AddEdge(finalPoint, graph.Exit(), false)

	left := pathdom.NewPath(lexicalLeft, "left").Field("value")
	right := pathdom.NewPath(lexicalRight, "right").Field("value")
	chosen := pathdom.NewPath(lexicalResult, "result").Field("chosen")
	final := pathdom.NewPath(lexicalResult, "result").Field("final")
	leftSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: 1, HasExpr: true}
	rightSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: 2, HasExpr: true}
	chosenSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: 3, HasExpr: true}
	facts := factflow.FactsInput{
		PathAssignments: map[cfg.Point]factflow.PathAssignment{
			thenPoint:  factflow.NewPathAssignment(chosen, leftSource),
			elsePoint:  factflow.NewPathAssignment(chosen, rightSource),
			finalPoint: factflow.NewPathAssignment(final, chosenSource),
		},
		ExpressionPaths: map[factflow.ExprRef]pathdom.Path{1: left, 2: right, 3: chosen},
		BranchPathRelations: map[cfg.Point]factflow.BranchPathRelationSet{
			branch: factflow.NewBranchPathRelationSet(
				factflow.NewBranchPathEquality(left, right, true, false),
				factflow.NewBranchPathInequality(left, right, false, true),
			),
		},
	}
	return fixture{
		graph: graph, branch: branch, then: thenPoint, otherwise: elsePoint, join: join, finalPoint: finalPoint,
		input: Input{Graph: graph, Facts: facts}, left: left, right: right, chosen: chosen, final: final,
	}
}

type concreteFixture struct {
	bindings PackedBindings
	resolver *visibility.Resolver
	facts    factflow.Facts
	left     pathdom.Path
	right    pathdom.Path
	chosen   pathdom.Path
	final    pathdom.Path
}

func bindFixture(t testing.TB, fixture fixture, serial int) concreteFixture {
	t.Helper()
	leftRoot := pathdom.NewPath(symbol.ID(1000+serial*10), fmt.Sprintf("callerLeft%d", serial)).Field("payload")
	rightRoot := pathdom.NewPath(symbol.ID(1001+serial*10), fmt.Sprintf("callerRight%d", serial)).Field("payload")
	resultRoot := pathdom.NewPath(symbol.ID(1002+serial*10), fmt.Sprintf("callerResult%d", serial)).Field("payload")
	bindings, err := PackBindings([]RootBinding{
		{Lexical: lexicalResult, Caller: resultRoot},
		{Lexical: lexicalLeft, Caller: leftRoot},
		{Lexical: lexicalRight, Caller: rightRoot},
	})
	if err != nil {
		t.Fatal(err)
	}
	left, _ := bindings.path(fixture.left)
	right, _ := bindings.path(fixture.right)
	chosen, _ := bindings.path(fixture.chosen)
	final, _ := bindings.path(fixture.final)

	definitions := make([]visibility.Definition, 0, 3)
	for _, path := range []pathdom.Path{leftRoot, rightRoot, resultRoot} {
		definitions = append(definitions, visibility.Definition{Point: fixture.graph.Entry(), Symbol: path.Symbol, Root: path.Root})
	}
	resolver := visibility.NewResolver(visibility.BuildForward(visibility.BuildConfig{Graph: fixture.graph, Definitions: definitions}))
	leftSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: 1, HasExpr: true}
	rightSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: 2, HasExpr: true}
	chosenSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: 3, HasExpr: true}
	facts := factflow.NewFacts(factflow.FactsInput{
		PathAssignments: map[cfg.Point]factflow.PathAssignment{
			fixture.then:       factflow.NewPathAssignment(chosen, leftSource),
			fixture.otherwise:  factflow.NewPathAssignment(chosen, rightSource),
			fixture.finalPoint: factflow.NewPathAssignment(final, chosenSource),
		},
		ExpressionPaths: map[factflow.ExprRef]pathdom.Path{1: left, 2: right, 3: chosen},
		BranchPathRelations: map[cfg.Point]factflow.BranchPathRelationSet{
			fixture.branch: factflow.NewBranchPathRelationSet(
				factflow.NewBranchPathEquality(left, right, true, false),
				factflow.NewBranchPathInequality(left, right, false, true),
			),
		},
	})
	return concreteFixture{bindings: bindings, resolver: resolver, facts: facts, left: left, right: right, chosen: chosen, final: final}
}

func initialState(registry *axis.Registry, resolver *visibility.Resolver, graph cfg.Graph, left, right pathdom.Path, leftValue, rightValue product.Value) state.State {
	entry := graph.Entry()
	return state.Domain(registry).Bottom().
		WritePathKey(registry, resolver.KeySpace(), resolver.KeyAt(entry, left), leftValue).
		WritePathKey(registry, resolver.KeySpace(), resolver.KeyAt(entry, right), rightValue)
}

func solveConcrete(fixture fixture, bound concreteFixture, registry *axis.Registry, entry state.State) transfer.Result {
	sources := pathSources{registry: registry, resolver: bound.resolver, facts: bound.facts}
	return transfer.Run(transfer.Config{
		Graph: fixture.graph, Registry: registry, EntryState: entry,
		NodeTransfer: factapply.NewFactsNodeTransfer(factapply.FactsNodeTransferConfig{Facts: bound.facts, Sources: sources, Visibility: bound.resolver}),
		EdgeTransfer: factapply.NewFactsEdgeTransfer(factapply.FactsEdgeTransferConfig{Facts: bound.facts, Sources: sources, Visibility: bound.resolver}),
	})
}

func TestLexicalTransformerMatchesConcreteSolveAtEveryPointAndExit(t *testing.T) {
	fixture := newFixture()
	transformer, err := Compile(fixture.input)
	if err != nil {
		t.Fatal(err)
	}
	registry := standard.Registry()
	domain := state.Domain(registry)
	rng := rand.New(rand.NewSource(418))
	values := []product.Value{
		product.Top(), typevalue.LiteralString(registry, "same"), typevalue.LiteralString(registry, "different"),
		typevalue.LiteralInt(registry, 1), typevalue.LiteralInt(registry, 2),
	}
	for bindingCase := 0; bindingCase < 64; bindingCase++ {
		concrete := bindFixture(t, fixture, bindingCase+1)
		bound, err := transformer.Bind(concrete.bindings)
		if err != nil {
			t.Fatalf("binding %d: %v", bindingCase, err)
		}
		for valueCase := 0; valueCase < 16; valueCase++ {
			leftValue := values[rng.Intn(len(values))]
			rightValue := values[rng.Intn(len(values))]
			entry := initialState(registry, concrete.resolver, fixture.graph, concrete.left, concrete.right, leftValue, rightValue)
			want := solveConcrete(fixture, concrete, registry, entry)
			got, err := bound.Execute(Config{Registry: registry, Resolver: concrete.resolver, EntryState: entry})
			if err != nil {
				t.Fatalf("binding %d values %d: %v", bindingCase, valueCase, err)
			}
			for _, point := range fixture.graph.RPO() {
				if !domain.Equal(want[point], got[point]) {
					t.Fatalf("binding %d values %d point %d differs", bindingCase, valueCase, point)
				}
			}
		}
	}
}

func TestCompileFailsClosedForCyclesUnsupportedFactsAndMissingBindings(t *testing.T) {
	fixture := newFixture()
	unsupported := fixture.input
	unsupported.Facts.NoNormalReturns = map[cfg.Point]struct{}{fixture.then: {}}
	if _, err := Compile(unsupported); err == nil {
		t.Fatal("unsupported fact family compiled")
	}

	cycleGraph := cfg.New()
	loop := cycleGraph.AddNode(cfg.NodeNoop)
	cycleGraph.AddEdge(cycleGraph.Entry(), loop, false)
	cycleGraph.AddEdge(loop, loop, false)
	cycleGraph.AddEdge(loop, cycleGraph.Exit(), false)
	if _, err := Compile(Input{Graph: cycleGraph}); err == nil {
		t.Fatal("recursive CFG compiled")
	}

	transformer, err := Compile(fixture.input)
	if err != nil {
		t.Fatal(err)
	}
	bindings, _ := PackBindings([]RootBinding{{Lexical: lexicalLeft, Caller: pathdom.NewPath(9001, "left")}})
	if _, err := transformer.Bind(bindings); err == nil {
		t.Fatal("partial root binding succeeded")
	}
}
