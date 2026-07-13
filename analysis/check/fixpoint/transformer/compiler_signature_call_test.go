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
	if relation.ObservationCoverageComplete() {
		t.Fatal("return-target signature call incorrectly claimed complete diagnostic observation coverage")
	}
	cursor, _ := NewBindingCursor(Shape{}, nil, nil)
	got, exact := relation.Specialize(cursor, nil, nil)
	returns, accepted := effectlowering.StaticScalarSignatureReturns(reg, nil, sig)
	want := summary.Normalize(reg, summary.Summary{Returns: returns, MaySuspend: true})
	if !accepted || !exact || !summary.Equal(reg, got, want) {
		t.Fatalf("pure signature relation accepted/exact=%v/%v\n got=%#v\nwant=%#v", accepted, exact, got, want)
	}
}

func TestPlanCompilerAdjustedMultiReturnSignatureSelectsFirstWithoutSourceExpr(t *testing.T) {
	reg := standard.Registry()
	sig := signature.Function{Type: typ.Func().Returns(typ.String, typ.Integer).Build(), Effect: effect.Row{}}
	op, ok := operationplan.NewSignatureCallOperation(sig)
	if !ok {
		t.Fatal("signature descriptor rejected")
	}
	for _, test := range []struct {
		name           string
		sourceAdjusted bool
		sourceExpanded bool
		sourceOpenTail bool
		sourceResult   int
		siteAdjusted   bool
		siteExpanded   bool
		siteOpenTail   bool
		wantExact      bool
	}{
		{name: "adjusted", sourceAdjusted: true, siteAdjusted: true, wantExact: true},
		{name: "source expanded but site adjusted", sourceExpanded: true, siteAdjusted: true},
		{name: "source open tail but site adjusted", sourceExpanded: true, sourceOpenTail: true, siteAdjusted: true},
		{name: "source adjusted but site expanded", sourceAdjusted: true, siteExpanded: true},
		{name: "adjusted second result", sourceAdjusted: true, sourceResult: 1, siteAdjusted: true},
		{name: "both expanded", sourceExpanded: true, siteExpanded: true},
		{name: "both open tail", sourceExpanded: true, sourceOpenTail: true, siteExpanded: true, siteOpenTail: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			graph := cfg.New()
			call := graph.AddNode(cfg.NodeCall)
			ret := graph.AddNode(cfg.NodeReturn)
			graph.AddEdge(graph.Entry(), call, false)
			graph.AddEdge(call, ret, false)
			graph.AddEdge(ret, graph.Exit(), false)
			ref := factflow.ExprRef(1)
			returnShape, shapeOK := factflow.NewValueSourceShape(true, test.sourceExpanded, test.sourceAdjusted, test.sourceOpenTail)
			if !shapeOK {
				t.Fatal("return source shape rejected")
			}
			source, sourceOK := factflow.NewCallValueSource(0, 0, 0, test.sourceResult, call, returnShape)
			if !sourceOK {
				t.Fatal("no-expr call result source rejected")
			}
			site := factflow.NewCallSite(factflow.CallSiteConfig{
				Context: factflow.CallSiteContextReturnSource, Point: call, HasPoint: true,
				ExprRef: ref, HasExpr: true, Final: true,
				Adjusted: test.siteAdjusted, Expanded: test.siteExpanded, OpenTail: test.siteOpenTail,
				ResultTargets: []factflow.CallResultTarget{factflow.NewCallResultTarget(factflow.CallResultTargetReturn, 0, 0, 0, pathdom.Path{})},
			})
			plan := operationplan.New(graph, factflow.FactsInput{
				CallSites: map[cfg.Point]factflow.CallSite{call: site},
				Returns:   map[cfg.Point]factflow.Return{ret: factflow.NewReturn([]factflow.ValueSource{source})},
			}).WithSignatureCalls(map[cfg.Point]operationplan.SignatureCallOperation{call: op})
			relation := NewPlanCompiler().Compile(reg, graph, plan, Shape{})
			if !test.wantExact {
				if relation.ContextualReason() == "" {
					t.Fatal("non-adjusted multi-return call compiled as scalar")
				}
				return
			}
			if reason := relation.ContextualReason(); reason != "" {
				t.Fatalf("adjusted multi-return call compiled contextually: %s", reason)
			}
			cursor, _ := NewBindingCursor(Shape{}, nil, nil)
			got, exact := relation.Specialize(cursor, nil, nil)
			returns, accepted := effectlowering.StaticScalarSignatureReturns(reg, nil, sig)
			want := summary.Normalize(reg, summary.Summary{Returns: returns[:1], MaySuspend: true})
			if !accepted || !exact || !summary.Equal(reg, got, want) {
				t.Fatalf("adjusted signature relation accepted/exact=%v/%v got=%#v want=%#v", accepted, exact, got, want)
			}
		})
	}
}
