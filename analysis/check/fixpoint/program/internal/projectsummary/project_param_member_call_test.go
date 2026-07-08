package projectsummary

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/type/subtype"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

type memberCallSiteOnlyResult struct {
	graph *cfg.CFG
	point cfg.Point
	site  factflow.CallSiteView
	ks    *keyspace.KeySpace

	signature  *typ.Function
	outcome    callpayload.CallOutcome
	hasOutcome bool
}

func (r memberCallSiteOnlyResult) Registry() *axis.Registry { return standard.Registry() }
func (r memberCallSiteOnlyResult) Graph() cfg.Graph         { return r.graph }
func (r memberCallSiteOnlyResult) ExitState() (state.State, bool) {
	return state.State{}, true
}
func (r memberCallSiteOnlyResult) ReturnPoints() []cfg.Point    { return nil }
func (r memberCallSiteOnlyResult) KeySpace() *keyspace.KeySpace { return r.ks }
func (r memberCallSiteOnlyResult) ParameterValueSlots() []key.Value {
	return []key.Value{key.SymbolValue(symbol.ID(1)), key.SymbolValue(symbol.ID(2))}
}
func (r memberCallSiteOnlyResult) EntryState() (state.State, bool) { return state.State{}, true }
func (r memberCallSiteOnlyResult) StateAt(cfg.Point) (state.State, bool) {
	return state.State{}, true
}
func (r memberCallSiteOnlyResult) CallSiteView(point cfg.Point) (factflow.CallSiteView, bool) {
	return r.site, point == r.point
}
func (r memberCallSiteOnlyResult) CallSiteViewSignatureType(factflow.CallSiteView) (*typ.Function, bool) {
	return r.signature, r.signature != nil
}
func (r memberCallSiteOnlyResult) CallOutcomeAt(point cfg.Point) (callpayload.CallOutcome, bool) {
	return r.outcome, r.hasOutcome && point == r.point
}
func (r memberCallSiteOnlyResult) ExpressionPathRef(factflow.ExprRef) (pathdom.Path, bool) {
	return pathdom.Path{}, false
}

func TestParamMemberCallObligationsUseCallSiteSourcesWithoutSemanticCallFact(t *testing.T) {
	graph := cfg.New()
	call := graph.AddNode(cfg.NodeCall)
	graph.AddEdge(graph.Entry(), call, true)
	graph.AddEdge(call, graph.Exit(), true)

	client := pathdom.NewPath(symbol.ID(1), "client")
	modelID := pathdom.NewPath(symbol.ID(2), "model_id")
	source, ok := factflow.NewPathValueSource(modelID.Key(), 0, 0, 0, factflow.ValueSourceShape{Final: true, Adjusted: true})
	if !ok {
		t.Fatal("NewPathValueSource failed")
	}
	result := memberCallSiteOnlyResult{
		graph: graph,
		point: call,
		site: factflow.NewCallSite(factflow.CallSiteConfig{
			CalleePath:         client.Field("invoke"),
			CalleeMemberAccess: true,
			ArgumentSources:    []factflow.ValueSource{source},
		}).View(),
		ks:        keyspace.New(),
		signature: typ.Func().Param("model_id", typ.String).Build(),
	}

	got := projectParamMemberCallObligations(standard.Registry(), result, nil)
	if len(got) != 1 {
		t.Fatalf("member call obligations = %#v, want one from call-site source", got)
	}
	if got[0].ReceiverParam != 0 || got[0].ArgParam != 1 || got[0].MemberParamIndex != 0 {
		t.Fatalf("member call obligation = %#v, want receiver param 0, arg param 1, member param 0", got[0])
	}
}

func TestParamObligationsUseTypedCallSiteSourcesWithoutSemanticCallFact(t *testing.T) {
	graph := cfg.New()
	call := graph.AddNode(cfg.NodeCall)
	graph.AddEdge(graph.Entry(), call, true)
	graph.AddEdge(call, graph.Exit(), true)

	payload := pathdom.NewPath(symbol.ID(2), "payload")
	source, ok := factflow.NewPathValueSource(payload.Key(), 0, 0, 0, factflow.ValueSourceShape{Final: true, Adjusted: true})
	if !ok {
		t.Fatal("NewPathValueSource failed")
	}
	result := memberCallSiteOnlyResult{
		graph: graph,
		point: call,
		site: factflow.NewCallSite(factflow.CallSiteConfig{
			CalleeSymbol:    symbol.ID(1),
			ArgumentSources: []factflow.ValueSource{source},
		}).View(),
		ks:        keyspace.New(),
		signature: typ.Func().Param("payload", typ.String).Build(),
	}

	reg := standard.Registry()
	got := projectParamObligations(reg, result, nil)
	if len(got) != 2 {
		t.Fatalf("param obligations = %#v, want two parameter slots", got)
	}
	if product.Equal(reg, got[1], product.Top()) {
		t.Fatalf("payload obligation = top, want string requirement from call-site signature")
	}
	gotType, ok := typevalue.TypeOf(reg, got[1])
	if !ok || !subtype.IsSubtype(gotType, typ.String) {
		t.Fatalf("payload obligation type = %v/%v, want string", gotType, ok)
	}
}

func TestParamObligationsUseCallOutcomeSourcesWithoutSemanticCallFact(t *testing.T) {
	graph := cfg.New()
	call := graph.AddNode(cfg.NodeCall)
	graph.AddEdge(graph.Entry(), call, true)
	graph.AddEdge(call, graph.Exit(), true)

	payload := pathdom.NewPath(symbol.ID(2), "payload")
	source, ok := factflow.NewPathValueSource(payload.Key(), 0, 0, 0, factflow.ValueSourceShape{Final: true, Adjusted: true})
	if !ok {
		t.Fatal("NewPathValueSource failed")
	}
	reg := standard.Registry()
	value, ok := obligationValueFromType(reg, typ.String)
	if !ok {
		t.Fatal("obligationValueFromType(string) failed")
	}
	result := memberCallSiteOnlyResult{
		graph: graph,
		point: call,
		site: factflow.NewCallSite(factflow.CallSiteConfig{
			CalleeSymbol:    symbol.ID(1),
			ArgumentSources: []factflow.ValueSource{source},
		}).View(),
		ks: keyspace.New(),
		outcome: callpayload.CallOutcome{
			ParamObligations: []callpayload.CallParamObligation{
				{ParamIndex: 0, Value: value},
			},
		},
		hasOutcome: true,
	}

	got := projectParamObligations(reg, result, nil)
	if len(got) != 2 {
		t.Fatalf("param obligations = %#v, want two parameter slots", got)
	}
	if product.Equal(reg, got[1], product.Top()) {
		t.Fatalf("payload obligation = top, want string requirement from call outcome")
	}
	gotType, ok := typevalue.TypeOf(reg, got[1])
	if !ok || !subtype.IsSubtype(gotType, typ.String) {
		t.Fatalf("payload obligation type = %v/%v, want string", gotType, ok)
	}
}
