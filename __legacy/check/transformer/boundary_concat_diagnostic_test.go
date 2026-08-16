package transformer

import (
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/domain/type/subtype"
	typetable "github.com/wippyai/go-lua/analysis/domain/type/table"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
)

func TestBoundaryConcatOperandObligationsReadCompiledValueDAG(t *testing.T) {
	reg := standard.Registry()
	endpoint := symbol.ID(9201)
	config := symbol.ID(9202)
	configType := typetable.NewRecord().Field("base_url", typ.String).Build()
	configValue := typevalue.WithWitness(reg, typevalue.FromType(reg, configType), configType)

	tests := []struct {
		name      string
		params    []symbol.ID
		contracts []product.Value
		build     func(*Arena) ValueTerm
		want      int
	}{
		{
			name:      "literal prefix and unresolved endpoint",
			params:    []symbol.ID{endpoint},
			contracts: []product.Value{product.Top()},
			build: func(arena *Arena) ValueTerm {
				return arena.StringConcatValue(
					arena.Constant(typevalue.LiteralString(reg, "https://api.example.test")),
					arena.Root(Root{Kind: RootParam, Index: 0}),
				)
			},
			want: 0,
		},
		{
			name:      "declared string field and unresolved endpoint",
			params:    []symbol.ID{config, endpoint},
			contracts: []product.Value{configValue, product.Top()},
			build: func(arena *Arena) ValueTerm {
				baseURL := arena.StaticIndexValue(
					arena.Root(Root{Kind: RootParam, Index: 0}),
					segment.Segment{Kind: segment.SegmentField, Name: "base_url"},
				)
				return arena.StringConcatValue(baseURL, arena.Root(Root{Kind: RootParam, Index: 1}))
			},
			want: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			graph := cfg.New()
			plan := operationplan.New(graph, factflow.FactsInput{}).
				WithBoundaryParams(tc.params).
				WithBoundaryParamContracts(tc.contracts)
			builder := NewBuilder(reg, Shape{Params: uint32(len(tc.params))}, DefaultOutputCapabilityRegistry(), plan)
			ctx := planCompileContext{registry: reg, graph: graph, plan: plan, facts: plan.Facts(), builder: builder}
			got := boundaryConcatOperandParamIndices(ctx, tc.build(builder.Arena()))
			if len(got) != 1 || got[0] != tc.want {
				t.Fatalf("concat operand params = %v, want [%d]", got, tc.want)
			}
		})
	}
}

func TestBoundaryMemberCallConcatObligationKeepsExactMemberOrigin(t *testing.T) {
	reg := standard.Registry()
	provider, endpoint, fullURL := symbol.ID(9301), symbol.ID(9302), symbol.ID(9303)
	point := cfg.Point(2)
	providerType := typetable.NewRecord().
		Field("get", typ.Func().Param("url", typ.String).Build()).
		Build()
	providerValue := typevalue.WithWitness(reg, typevalue.FromType(reg, providerType), providerType)
	providerPath := pathdom.NewPath(provider, "http")
	shape, _ := factflow.NewValueSourceShape(true, false, false, false)
	argument, ok := factflow.NewPathValueSource(pathdom.NewPath(fullURL, "full_url").Key(), 0, 0, 0, shape)
	if !ok {
		t.Fatal("argument source rejected")
	}
	facts := factflow.FactsInput{
		CallSites: map[cfg.Point]factflow.CallSite{point: factflow.NewCallSite(factflow.CallSiteConfig{
			Context: factflow.CallSiteContextReturnSource, Point: point, HasPoint: true,
			CalleeSymbol: provider, CalleePath: providerPath.Field("get"), CalleeMemberAccess: true,
			ArgumentSources: []factflow.ValueSource{argument}, Final: true, Adjusted: true,
		})},
	}
	graph := cfg.New()
	call := graph.AddNode(cfg.NodeCall)
	if call != point {
		t.Fatalf("call point = %d, want %d", call, point)
	}
	graph.AddEdge(graph.Entry(), call, false)
	graph.AddEdge(call, graph.Exit(), false)
	plan := operationplan.New(graph, facts).
		WithBoundaryParams([]symbol.ID{provider, endpoint}).
		WithBoundaryParamContracts([]product.Value{providerValue, product.Top()})
	builder := NewBuilder(reg, Shape{Params: 2}, DefaultOutputCapabilityRegistry(), plan)
	arena := builder.Arena()
	ctx := planCompileContext{
		registry: reg, graph: graph, plan: plan, facts: plan.Facts(), builder: builder,
		locals: map[symbol.ID]ValueTerm{
			provider: arena.Root(Root{Kind: RootParam, Index: 0}),
			endpoint: arena.Root(Root{Kind: RootParam, Index: 1}),
		},
	}
	ctx.locals[fullURL] = arena.StringConcatValue(
		arena.Constant(typevalue.LiteralString(reg, "https://api.example.test")),
		ctx.locals[endpoint],
	)
	term, _, exact := boundaryMemberCallFromSite(ctx, point)
	if !exact || len(term.obligations) != 1 {
		t.Fatalf("member concat term = %#v/%t, want one obligation", term, exact)
	}
	obligation := term.obligations[0]
	if obligation.ParamIndex != 1 || !obligation.Origin.HasOrigin || obligation.Origin.ReceiverParam != 0 ||
		obligation.Origin.Member.Kind != segment.SegmentField || obligation.Origin.Member.Name != "get" ||
		obligation.Origin.ArgParam != 1 || obligation.Origin.MemberParamIndex != 0 {
		t.Fatalf("member concat obligation = %#v, want exact http.get endpoint origin", obligation)
	}
	want := boundaryConcatOperandObligationType()
	got, ok := typevalue.TypeOf(reg, obligation.Value)
	if !ok || !subtype.IsSubtype(got, want) || !subtype.IsSubtype(want, got) {
		t.Fatalf("member concat obligation type = %v/%t, want %v", got, ok, want)
	}
}

func TestStringConcatTermPublishesStringOnlyOnPossibleNormalContinuation(t *testing.T) {
	reg := standard.Registry()
	arena := NewArena(reg)
	term := arena.StringConcatValue(
		arena.Constant(typevalue.LiteralString(reg, "prefix")),
		arena.Root(Root{Kind: RootParam, Index: 0}),
	)
	evaluate := func(argument product.Value) (product.Value, bool) {
		t.Helper()
		cursor, err := NewBindingCursor(Shape{Params: 1}, []product.Value{argument}, nil)
		if err != nil {
			t.Fatal(err)
		}
		return arena.evalValue(term, cursor, SpecializationContext{})
	}
	value, exact := evaluate(product.Top())
	got, typed := typevalue.TypeOf(reg, value)
	if !exact || !typed || !typ.TypeEquals(got, typ.String) {
		t.Fatalf("unresolved normal concat = %v/%t type %v/%t, want exact string", value, exact, got, typed)
	}
	if _, exact := evaluate(typevalue.LiteralBool(reg, true)); exact {
		t.Fatal("definitely invalid boolean concat published a normal result")
	}
}

func TestBoundaryConcatObligationMapsStructuralEnvironmentBackToParam(t *testing.T) {
	reg := standard.Registry()
	endpoint := symbol.ID(9401)
	graph := cfg.New()
	plan := operationplan.New(graph, factflow.FactsInput{}).
		WithBoundaryParams([]symbol.ID{endpoint}).
		WithBoundaryParamContracts([]product.Value{product.Top()})
	builder := NewBuilder(reg, Shape{Params: 1}, DefaultOutputCapabilityRegistry(), plan)
	arena := builder.Arena()
	environment := arena.bindEnvironmentSymbol(endpoint)
	term := arena.StringConcatValue(arena.Constant(typevalue.LiteralString(reg, "prefix")), environment)
	ctx := planCompileContext{registry: reg, graph: graph, plan: plan, facts: plan.Facts(), builder: builder}
	got := boundaryConcatParamObligations(ctx, term)
	if len(got) != 1 || got[0].ParamIndex != 0 {
		t.Fatalf("structural environment concat obligations = %#v, want param 0", got)
	}
}
