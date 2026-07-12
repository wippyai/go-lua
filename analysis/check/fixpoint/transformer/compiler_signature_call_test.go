package transformer

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/domain/effect"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/engine/effectlowering"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/module/signature"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestPlanCompilerPureSignatureCallFeedsReturnExactly(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	call := graph.AddNode(cfg.NodeCall)
	ret := graph.AddNode(cfg.NodeReturn)
	graph.AddEdge(graph.Entry(), call, false)
	graph.AddEdge(call, ret, false)
	graph.AddEdge(ret, graph.Exit(), false)

	ref := factflow.ExprRef(1)
	shape, _ := factflow.NewValueSourceShape(true, false, true, false)
	source, ok := factflow.NewCallValueSource(ref, 0, 0, 0, call, shape)
	if !ok {
		t.Fatal("call result source rejected")
	}
	site := factflow.NewCallSite(factflow.CallSiteConfig{
		Context: factflow.CallSiteContextReturnSource, Point: call, HasPoint: true,
		ExprRef: ref, HasExpr: true,
		ResultTargets: []factflow.CallResultTarget{
			factflow.NewCallResultTarget(factflow.CallResultTargetReturn, 0, 0, 0, pathdom.Path{}),
		},
		Final: true, Adjusted: true,
	})
	sig := signature.Function{Type: typ.Func().Returns(typ.String).Build(), Effect: effect.Row{}}
	op, ok := operationplan.NewSignatureCallOperation(sig)
	if !ok {
		t.Fatal("signature descriptor rejected")
	}
	plan := operationplan.New(graph, factflow.FactsInput{
		CallSites: map[cfg.Point]factflow.CallSite{call: site},
		Returns:   map[cfg.Point]factflow.Return{ret: factflow.NewReturn([]factflow.ValueSource{source})},
	}).WithSignatureCalls(map[cfg.Point]operationplan.SignatureCallOperation{call: op})

	relation := NewPlanCompiler().Compile(reg, graph, plan, Shape{})
	if reason := relation.ContextualReason(); reason != "" {
		t.Fatalf("pure signature call compiled contextually: %s", reason)
	}
	cursor, _ := NewBindingCursor(Shape{}, nil, nil)
	got, exact := relation.Specialize(cursor, nil, nil)
	returns, accepted := effectlowering.StaticScalarSignatureReturns(reg, nil, sig)
	want := summary.Normalize(reg, summary.Summary{Returns: returns})
	if !accepted || !exact || !summary.Equal(reg, got, want) {
		t.Fatalf("pure signature relation accepted/exact=%v/%v\n got=%#v\nwant=%#v", accepted, exact, got, want)
	}
}
