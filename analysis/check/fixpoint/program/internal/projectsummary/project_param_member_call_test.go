package projectsummary

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/type/subtype"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

type memberCallSiteOnlyResult struct {
	graph *cfg.CFG
	point cfg.Point
	site  factflow.CallSiteView
	ks    *keyspace.KeySpace

	signature  *typ.Function
	outcome    callpayload.CallOutcome
	hasOutcome bool

	returnPoints  []cfg.Point
	returnSources map[cfg.Point][]factflow.ValueSource
	fn            *ast.FunctionExpr
	captures      []bind.Capture
}

func (r memberCallSiteOnlyResult) Registry() *axis.Registry { return standard.Registry() }
func (r memberCallSiteOnlyResult) Graph() cfg.Graph         { return r.graph }
func (r memberCallSiteOnlyResult) ExitState() (state.State, bool) {
	return state.State{}, true
}
func (r memberCallSiteOnlyResult) ReturnPoints() []cfg.Point {
	return append([]cfg.Point(nil), r.returnPoints...)
}
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
func (r memberCallSiteOnlyResult) ReturnValueSources(point cfg.Point) ([]factflow.ValueSource, bool) {
	sources, ok := r.returnSources[point]
	if !ok {
		return nil, false
	}
	return append([]factflow.ValueSource(nil), sources...), true
}
func (r memberCallSiteOnlyResult) ExpressionPathRef(factflow.ExprRef) (pathdom.Path, bool) {
	return pathdom.Path{}, false
}
func (r memberCallSiteOnlyResult) Function() *ast.FunctionExpr {
	return r.fn
}
func (r memberCallSiteOnlyResult) DirectCaptures(fn *ast.FunctionExpr) []bind.Capture {
	if fn == nil || fn != r.fn || len(r.captures) == 0 {
		return nil
	}
	return append([]bind.Capture(nil), r.captures...)
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

func TestParamMemberReturnSlotsUseCallSiteWithoutSemanticCallFact(t *testing.T) {
	graph := cfg.New()
	call := graph.AddNode(cfg.NodeCall)
	ret := graph.AddNode(cfg.NodeReturn)
	graph.AddEdge(graph.Entry(), call, true)
	graph.AddEdge(call, ret, true)
	graph.AddEdge(ret, graph.Exit(), true)

	client := pathdom.NewPath(symbol.ID(1), "client")
	callSource, ok := factflow.NewCallValueSource(0, 0, 0, 2, call, factflow.ValueSourceShape{
		Final:    true,
		Expanded: false,
		Adjusted: true,
	})
	if !ok {
		t.Fatal("NewCallValueSource failed")
	}
	result := memberCallSiteOnlyResult{
		graph: graph,
		point: call,
		site: factflow.NewCallSite(factflow.CallSiteConfig{
			CalleePath:         client.Field("invoke"),
			CalleeMemberAccess: true,
		}).View(),
		ks:            keyspace.New(),
		returnPoints:  []cfg.Point{ret},
		returnSources: map[cfg.Point][]factflow.ValueSource{ret: {callSource}},
	}

	got := projectParamMemberReturnSlots(standard.Registry(), result, nil)
	if len(got) != 1 {
		t.Fatalf("member return slots = %#v, want one slot from call-site receiver", got)
	}
	if got[0].ReceiverParam != 0 || got[0].ReturnIndex != 0 || got[0].MemberResultIndex != 2 {
		t.Fatalf("member return slot = %#v, want receiver param 0 return 0 from member result 2", got[0])
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

func TestCapturedPathObligationsUseCallSiteSourcesWithoutSemanticCallFact(t *testing.T) {
	graph := cfg.New()
	call := graph.AddNode(cfg.NodeCall)
	graph.AddEdge(graph.Entry(), call, true)
	graph.AddEdge(call, graph.Exit(), true)

	captured := symbol.ID(3)
	source, ok := factflow.NewPathValueSource(pathdom.NewPath(captured, "captured").Key(), 0, 0, 0, factflow.ValueSourceShape{Final: true, Adjusted: true})
	if !ok {
		t.Fatal("NewPathValueSource failed")
	}
	reg := standard.Registry()
	value, ok := obligationValueFromType(reg, typ.String)
	if !ok {
		t.Fatal("obligationValueFromType(string) failed")
	}
	fn := &ast.FunctionExpr{}
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
		fn:         fn,
		captures:   []bind.Capture{{Captured: captured, CapturedName: "captured"}},
	}

	got := projectCapturedPathObligations(reg, result, nil)
	if len(got) != 1 {
		t.Fatalf("captured obligations = %#v, want one from call-site argument source", got)
	}
	if want := pathaddr.SymbolStableKey(captured, nil); got[0].Path != want {
		t.Fatalf("captured path = %q, want %q", got[0].Path, want)
	}
	gotType, ok := typevalue.TypeOf(reg, got[0].Value)
	if !ok || !subtype.IsSubtype(gotType, typ.String) {
		t.Fatalf("captured obligation type = %v/%v, want string", gotType, ok)
	}
}
