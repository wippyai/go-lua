package transformer

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestExactReturnCallResultTermAcceptsEnumeratedExpressionProducer(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	point := graph.AddNode(cfg.NodeCall)
	site := factflow.NewCallSite(factflow.CallSiteConfig{
		Context: factflow.CallSiteContextExpressionProducer, Point: point, HasPoint: true,
		Final: true, Expanded: true,
		ResultTargets: []factflow.CallResultTarget{
			factflow.NewCallResultTarget(factflow.CallResultTargetExpression, 0, 0, 0, pathdom.Path{}),
		},
	})
	plan := operationplan.New(graph, factflow.FactsInput{CallSites: map[cfg.Point]factflow.CallSite{point: site}})
	builder := NewBuilder(reg, Shape{}, DefaultOutputCapabilityRegistry(), plan)
	want := builder.Arena().Constant(typevalue.LiteralInt(reg, 1))
	ctx := planCompileContext{
		registry: reg, plan: plan, facts: plan.Facts(), builder: builder,
		resultRoots: map[ResultRoot]ValueTerm{{Point: point, Slot: 0}: want},
	}
	shape, ok := factflow.NewValueSourceShape(true, true, false, true)
	if !ok {
		t.Fatal("expression-consumer shape rejected")
	}
	source, ok := factflow.NewCallValueSource(0, 0, 0, 0, point, shape)
	if !ok {
		t.Fatal("expression-producer result source rejected")
	}
	got, err := exactReturnCallResultTerm(ctx, source)
	if err != nil || got != want {
		t.Fatalf("expression-producer result term = %d, err=%v, want %d", got, err, want)
	}
}

func TestExactCompilerSourceTermResolvesChannelSelectResultAuthority(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	point := graph.AddNode(cfg.NodeCall)
	facts := factflow.FactsInput{ChannelSelects: map[cfg.Point]factflow.ChannelSelectSet{
		point: factflow.NewChannelSelectSet(factflow.NewChannelSelect(factflow.ChannelSelectConfig{
			SelectID: "select-result", Kind: factflow.ChannelSelectSelect, Index: 0,
		})),
	}}
	plan := operationplan.New(graph, facts)
	builder := NewBuilder(reg, Shape{}, DefaultOutputCapabilityRegistry(), plan)
	ctx := planCompileContext{registry: reg, plan: plan, facts: plan.Facts(), builder: builder, expressions: make(map[factflow.ExprRef][]ValueTerm)}
	if err := bindChannelSelectResultTerms(&ctx); err != nil {
		t.Fatal(err)
	}
	shape, ok := factflow.NewValueSourceShape(true, false, false, false)
	if !ok {
		t.Fatal("scalar result shape rejected")
	}
	source, ok := factflow.NewCallValueSource(0, 0, 0, 0, point, shape)
	if !ok {
		t.Fatal("channel-select result source rejected")
	}
	got, err := exactCompilerSourceTerm(ctx, source)
	want, bound := builder.Arena().callResultValue(point, 0)
	if err != nil || !bound || got == 0 || got != want {
		t.Fatalf("channel-select result term = %d, err=%v, want %d", got, err, want)
	}
}
