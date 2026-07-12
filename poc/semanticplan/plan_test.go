package semanticplan

import (
	"math/rand"
	"reflect"
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	sourcevalue "github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

type fixedSources struct {
	value product.Value
	ok    bool
}

func (s fixedSources) ValueOfSource(cfg.Point, factflow.ValueSource, state.State, func(cfg.Point) state.State) (product.Value, bool) {
	return s.value, s.ok
}

type lazyPathSources struct {
	reg      *axis.Registry
	resolver *visibility.Resolver
	path     pathdom.Path
}

func (s lazyPathSources) ValueOfSource(point cfg.Point, _ factflow.ValueSource, in state.State, read func(cfg.Point) state.State) (product.Value, bool) {
	current := in
	if read != nil {
		current = read(point)
	}
	key := s.resolver.KeyAt(point, s.path)
	value := current.ReadPathKey(s.reg, s.resolver.KeySpace(), key)
	return value, !product.Equal(s.reg, value, product.Bottom(s.reg))
}

type pathFixture struct {
	point      cfg.Point
	graph      *cfg.CFG
	input      factflow.FactsInput
	resolver   *visibility.Resolver
	target     pathdom.Path
	sourcePath pathdom.Path
	source     factflow.ValueSource
}

func newPathFixture() pathFixture {
	graph := cfg.New()
	point := graph.AddNode(cfg.NodeAssign)
	graph.AddEdge(graph.Entry(), point, false)
	graph.AddEdge(point, graph.Exit(), false)
	targetSym, sourceSym := symbol.ID(101), symbol.ID(202)
	target := pathdom.NewPath(targetSym, "target").Field("field")
	sourcePath := pathdom.NewPath(sourceSym, "source").Field("value")
	source := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: 77, HasExpr: true}
	builder := visibility.NewBuilder()
	builder.Define(point, targetSym, "target")
	builder.Define(point, sourceSym, "source")
	return pathFixture{
		point: point, graph: graph, resolver: visibility.NewResolver(builder.Build()), target: target, sourcePath: sourcePath, source: source,
		input: factflow.FactsInput{
			PathAssignments: map[cfg.Point]factflow.PathAssignment{point: factflow.NewPathAssignment(target, source)},
			ExpressionPaths: map[factflow.ExprRef]pathdom.Path{source.ExprRef: sourcePath},
		},
	}
}

func TestPathAssignmentPlanConcreteMatchesProductionAcrossAllLanesRandomized(t *testing.T) {
	fixture := newPathFixture()
	plan, err := CompilePathAssignments(fixture.input)
	if err != nil {
		t.Fatal(err)
	}
	reg := standard.Registry()
	rng := rand.New(rand.NewSource(17))
	values := []product.Value{
		product.Top(),
		product.Absent(reg),
		typevalue.LiteralString(reg, "value"),
		typevalue.LiteralInt(reg, 7),
		product.NewWithPresence(reg, product.ShapeTop, presence.Present()),
	}
	ctx := transfer.NodeContext{Graph: fixture.graph, Registry: reg, Point: fixture.point, Node: fixture.graph.Node(fixture.point)}

	for _, lane := range state.DefaultLanes() {
		lane := lane
		t.Run(string(lane), func(t *testing.T) {
			domain := state.DomainWithLanes(reg, []state.LaneID{lane})
			for i := 0; i < 128; i++ {
				assigned := values[rng.Intn(len(values))]
				base := domain.Bottom()
				if lane == state.LaneValues {
					base = base.WriteValue(reg, key.SymbolValue(fixture.target.Symbol), values[rng.Intn(len(values))])
				}
				config := factapply.FactsNodeTransferConfig{
					Facts: factflow.NewFacts(fixture.input), Sources: fixedSources{value: assigned, ok: true}, Visibility: fixture.resolver,
				}
				oracle := factapply.NewFactsNodeTransfer(config)(ctx, base)
				config.Facts = factflow.Facts{}
				got := plan.BindConcrete(config)(ctx, base)
				if !domain.Equal(oracle, got) {
					t.Fatalf("case %d lane %s differs", i, lane)
				}
			}
		})
	}
}

func TestPathAssignmentPlanPreservesNoopCases(t *testing.T) {
	fixture := newPathFixture()
	plan, err := CompilePathAssignments(fixture.input)
	if err != nil {
		t.Fatal(err)
	}
	reg := standard.Registry()
	base := state.Domain(reg).Bottom().WriteValue(reg, key.SymbolValue(fixture.target.Symbol), typevalue.LiteralString(reg, "old"))
	ctx := transfer.NodeContext{Graph: fixture.graph, Registry: reg, Point: fixture.point, Node: fixture.graph.Node(fixture.point)}
	for _, tc := range []struct {
		name     string
		resolver *visibility.Resolver
		sources  sourcevalue.SourceValues
	}{
		{"missing-source", fixture.resolver, fixedSources{}},
		{"missing-visibility", nil, fixedSources{value: product.Top(), ok: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			config := factapply.FactsNodeTransferConfig{Facts: factflow.NewFacts(fixture.input), Sources: tc.sources, Visibility: tc.resolver}
			want := factapply.NewFactsNodeTransfer(config)(ctx, base)
			got := plan.BindConcrete(factapply.FactsNodeTransferConfig{Sources: tc.sources, Visibility: tc.resolver})(ctx, base)
			if !state.Domain(reg).Equal(want, got) {
				t.Fatal("noop behavior differs")
			}
		})
	}
}

func TestPathAssignmentPlanMatchesRandomSubtreesAliasesAndLazyReads(t *testing.T) {
	fixture := newPathFixture()
	plan, err := CompilePathAssignments(fixture.input)
	if err != nil {
		t.Fatal(err)
	}
	reg := standard.Registry()
	domain := state.Domain(reg)
	rng := rand.New(rand.NewSource(29))
	ctx := transfer.NodeContext{Graph: fixture.graph, Registry: reg, Point: fixture.point, Node: fixture.graph.Node(fixture.point)}
	sources := lazyPathSources{reg: reg, resolver: fixture.resolver, path: fixture.sourcePath}
	config := factapply.FactsNodeTransferConfig{Facts: factflow.NewFacts(fixture.input), Sources: sources, Visibility: fixture.resolver}
	oracle := factapply.NewFactsNodeTransfer(config)
	config.Facts = factflow.Facts{}
	planned := plan.BindConcrete(config)
	values := []product.Value{product.Top(), product.Absent(reg), typevalue.LiteralString(reg, "s"), typevalue.LiteralInt(reg, 9)}
	for i := 0; i < 512; i++ {
		base := domain.Bottom()
		// Seed the lazy source, the target subtree, and a sibling. Equality
		// closure and subtree invalidation see different shapes across cases.
		base = base.WritePathKey(reg, fixture.resolver.KeySpace(), fixture.resolver.KeyAt(fixture.point, fixture.sourcePath), values[rng.Intn(len(values))])
		for depth := 0; depth < 1+rng.Intn(4); depth++ {
			child := fixture.target
			for n := 0; n <= depth; n++ {
				child = child.Field("child")
			}
			base = base.WritePathKey(reg, fixture.resolver.KeySpace(), fixture.resolver.KeyAt(fixture.point, child), values[rng.Intn(len(values))])
		}
		sibling := fixture.target.Parent().Field("sibling")
		base = base.WritePathKey(reg, fixture.resolver.KeySpace(), fixture.resolver.KeyAt(fixture.point, sibling), values[rng.Intn(len(values))])
		want := oracle(ctx, base)
		got := planned(ctx, base)
		if !domain.Equal(want, got) {
			t.Fatalf("random case %d differs", i)
		}
	}
}

func TestPlanCopiesFactsAndPaths(t *testing.T) {
	fixture := newPathFixture()
	plan, err := CompilePathAssignments(fixture.input)
	if err != nil {
		t.Fatal(err)
	}
	mutated := factflow.NewPathAssignment(pathdom.NewPath(999, "mutated").Field("x"), fixture.source)
	fixture.input.PathAssignments[fixture.point] = mutated
	fixture.input.ExpressionPaths[fixture.source.ExprRef] = pathdom.NewPath(998, "mutated-source")
	op, ok := plan.Operation(fixture.point)
	if !ok || !op.Target.Equal(fixture.target) || !op.SourcePath.Equal(fixture.sourcePath) {
		t.Fatalf("compiled operation changed with caller maps: %#v", op)
	}
}

func TestCompanionsAreExplicitAndSymbolicFallbackIsAtomic(t *testing.T) {
	fixture := newPathFixture()
	fixture.input.PathStaticMemberWrites = map[cfg.Point]factflow.PathStaticMemberWrite{
		fixture.point: factflow.NewPathStaticMemberWrite(fixture.target, fixture.source),
	}
	plan, err := CompilePathAssignments(fixture.input)
	if err != nil {
		t.Fatal(err)
	}
	op, _ := plan.Operation(fixture.point)
	if !op.HasStaticMemberWrite {
		t.Fatal("same-point static-member companion missing from operation metadata")
	}
	transformer := DefaultPathAssignmentRegistry().Lift(op)
	if !transformer.Contextual() || len(transformer.FallbackLanes()) != len(pathAssignmentAccesses) {
		t.Fatalf("companion did not force atomic fallback: %v", transformer.FallbackLanes())
	}
}

func TestUnsupportedCompanionFailsPlanCompilation(t *testing.T) {
	fixture := newPathFixture()
	fixture.input.PathDescendantInvalidations = map[cfg.Point]factflow.PathDescendantInvalidation{
		fixture.point: factflow.NewPathDescendantInvalidation(fixture.target.Parent()),
	}
	if _, err := CompilePathAssignments(fixture.input); err == nil {
		t.Fatal("unmodeled companion compiled")
	}
}

func TestSymbolicPathAssignmentGuardCorrelationRebasingAndOrder(t *testing.T) {
	fixture := newPathFixture()
	plan, err := CompilePathAssignments(fixture.input)
	if err != nil {
		t.Fatal(err)
	}
	op, _ := plan.Operation(fixture.point)
	transformer := DefaultPathAssignmentRegistry().Lift(op)
	if !transformer.Contextual() || !transformer.TermComplete() {
		t.Fatalf("term coverage/execution verdict = contextual %v complete %v fallback %v", transformer.Contextual(), transformer.TermComplete(), transformer.FallbackLanes())
	}
	reg := standard.Registry()
	value := typevalue.LiteralString(reg, "bound")
	callerTarget := pathdom.NewPath(301, "caller-target")
	callerSource := pathdom.NewPath(302, "caller-source")
	rows, ok := transformer.SubstituteTerms(Bindings{
		Roots:  map[symbol.ID]pathdom.Path{fixture.target.Symbol: callerTarget, fixture.sourcePath.Symbol: callerSource},
		Values: map[pathdom.PathKey]product.Value{fixture.sourcePath.Key(): value},
	})
	if !ok || len(rows) != 1 || len(rows[0].Effects) == 0 {
		t.Fatalf("instantiation=%v/%v", rows, ok)
	}
	for i, effect := range rows[0].Effects {
		if i > 0 && rows[0].Effects[i-1].Phase > effect.Phase {
			t.Fatalf("semantic order regressed at %d: %v", i, rows[0].Effects)
		}
		if effect.Target.Symbol != callerTarget.Symbol || effect.Source.Symbol != callerSource.Symbol {
			t.Fatalf("effect not rebound: %#v", effect)
		}
		if !product.Equal(reg, effect.Value, value) {
			t.Fatal("correlated source value lost")
		}
	}
	// The guard makes a missing source binding an infeasible row, never a row
	// with a guessed Top value.
	rows, ok = transformer.SubstituteTerms(Bindings{Roots: map[symbol.ID]pathdom.Path{
		fixture.target.Symbol: callerTarget, fixture.sourcePath.Symbol: callerSource,
	}})
	if !ok || len(rows) != 0 {
		t.Fatalf("missing guarded source produced effects: %v/%v", rows, ok)
	}
}

func TestMissingLaneAdapterFallsBackWithoutPartialTransformer(t *testing.T) {
	fixture := newPathFixture()
	plan, _ := CompilePathAssignments(fixture.input)
	op, _ := plan.Operation(fixture.point)
	registry, err := NewSymbolicRegistry(pathAssignmentLaneAdapter{lane: state.LaneValues, effects: []phasedEffect{{EffectWriteValue, 40}}})
	if err != nil {
		t.Fatal(err)
	}
	transformer := registry.Lift(op)
	if !transformer.Contextual() || len(transformer.FallbackLanes()) != len(state.DefaultLanes())-1 {
		t.Fatalf("fallback=%v", transformer.FallbackLanes())
	}
	if rows, ok := transformer.SubstituteTerms(Bindings{}); ok || rows != nil {
		t.Fatalf("contextual transformer instantiated partially: %v/%v", rows, ok)
	}
}

func TestPathAssignmentMetadataIsStable(t *testing.T) {
	op := PathAssignmentOp{}
	wantLanes := []state.LaneID{
		state.LaneValues, state.LanePathEvidence, state.LaneHeapTableIdentity,
		state.LaneDynamicIndex, state.LaneKeyMemberships, state.LaneLenFloors, state.LaneUserLattices,
	}
	if got := accessesToLanes(op.Accesses()); !reflect.DeepEqual(got, wantLanes) {
		t.Fatalf("lane dependencies=%v want=%v", got, wantLanes)
	}
	if len(op.Ownership()) != 3 || len(op.Rebasing()) != 2 {
		t.Fatalf("ownership/rebasing=%v/%v", op.Ownership(), op.Rebasing())
	}
}
