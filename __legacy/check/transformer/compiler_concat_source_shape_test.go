package transformer

import (
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestExactStringConcatSourceAdmitsScalarAdjustedSlotZero(t *testing.T) {
	reg := standard.Registry()
	shape, ok := factflow.NewValueSourceShape(true, false, true, false)
	if !ok {
		t.Fatal("scalar-adjusted source shape rejected")
	}
	source, ok := factflow.NewStringLiteralValueSource("base", -1, -1, 0, shape)
	if !ok {
		t.Fatal("scalar-adjusted literal source rejected")
	}
	plan := operationplan.New(nil, factflow.FactsInput{})
	ctx := planCompileContext{
		registry: reg,
		plan:     plan,
		facts:    plan.Facts(),
		builder:  NewBuilder(reg, Shape{}, DefaultOutputCapabilityRegistry(), plan),
	}
	term, err := exactStringConcatSourceTerm(ctx, source, nil)
	if err != nil || term == 0 {
		t.Fatalf("scalar-adjusted slot zero: term=%d err=%v", term, err)
	}
	value, exact := ctx.builder.Arena().evalValue(term, BindingCursor{}, SpecializationContext{})
	valueType, typed := typevalue.TypeOf(reg, value)
	if !exact || !typed || !typ.TypeEquals(valueType, typ.LiteralString("base")) {
		t.Fatalf("scalar-adjusted value = %v exact=%t typed=%t, want literal string", valueType, exact, typed)
	}
}

func TestExactStringConcatSourcePreservesAdjustedCallProducerAuthority(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	point := graph.AddNode(cfg.NodeCall)
	shape, ok := factflow.NewValueSourceShape(true, false, true, false)
	if !ok {
		t.Fatal("scalar-adjusted call shape rejected")
	}
	site := factflow.NewCallSite(factflow.CallSiteConfig{
		Context: factflow.CallSiteContextExpressionProducer,
		Point:   point, HasPoint: true,
		Final: true, Adjusted: true,
		ResultTargets: []factflow.CallResultTarget{
			factflow.NewCallResultTarget(factflow.CallResultTargetExpression, 0, 0, 0, pathdom.Path{}),
		},
	})
	plan := operationplan.New(graph, factflow.FactsInput{CallSites: map[cfg.Point]factflow.CallSite{point: site}})
	builder := NewBuilder(reg, Shape{}, DefaultOutputCapabilityRegistry(), plan)
	if builder.Arena().bindCallResult(point, 0) == 0 {
		t.Fatal("call result slot did not bind")
	}
	ctx := planCompileContext{registry: reg, plan: plan, facts: plan.Facts(), builder: builder}
	source, ok := factflow.NewCallValueSource(0, 0, 0, 0, point, shape)
	if !ok {
		t.Fatal("scalar-adjusted call source rejected")
	}

	got, err := exactStringConcatSourceTerm(ctx, source, nil)
	want, bound := builder.Arena().callResultValue(point, 0)
	if err != nil || !bound || got == 0 || got != want {
		t.Fatalf("adjusted call operand term = %d, err=%v, want producer slot %d", got, err, want)
	}
}
