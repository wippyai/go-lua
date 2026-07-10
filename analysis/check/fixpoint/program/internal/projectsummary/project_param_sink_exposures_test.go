package projectsummary

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

func TestFromResultProjectsParamSinkExposureFromPathFactWithoutSemanticAssignment(t *testing.T) {
	reg := standard.Registry()
	param := symbol.ID(9501)
	sink := symbol.ID(9502)
	graph := cfg.New()
	assign := graph.AddNode(cfg.NodeAssign)
	graph.AddEdge(graph.Entry(), assign, false)
	graph.AddEdge(assign, graph.Exit(), false)
	shape, ok := factflow.NewValueSourceShape(true, false, false, false)
	if !ok {
		t.Fatal("NewValueSourceShape returned false")
	}
	source, ok := factflow.NewExpressionValueSource(factflow.ExprRef(1), 0, factflow.NoValueSourceIndex, 0, shape)
	if !ok {
		t.Fatal("NewExpressionValueSource returned false")
	}
	contract := typevalue.FromType(reg, typ.String)
	stub := normalReturnFactProjectAssignmentStub{
		normalReturnFactProjectResultStub: normalReturnFactProjectResultStub{
			reg:   reg,
			graph: graph,
			exit:  state.State{},
			slots: []key.Value{key.SymbolValue(param)},
			exprPaths: map[factflow.ExprRef]pathdom.Path{
				factflow.ExprRef(1): pathdom.NewPath(param, ""),
			},
			entryState:    state.State{}.WriteValue(reg, key.SymbolValue(param), contract),
			hasEntryState: true,
		},
		fn: &ast.FunctionExpr{},
		kinds: map[symbol.ID]symbol.Kind{
			sink: symbol.Global,
		},
		pathAssignments: map[cfg.Point]factflow.PathAssignment{
			assign: factflow.NewPathAssignment(pathdom.NewPath(sink, "G").Field("slot"), source),
		},
	}

	got := FromResult(stub).ParamSinkExposures
	sourceKey, ok := pathaddr.RootPlaceholderKeyFromPath(pathdom.NewPlaceholder(0))
	if !ok {
		t.Fatal("RootPlaceholderKeyFromPath failed")
	}
	gotType, gotTypeOK := typ.Nil, false
	if len(got) == 1 {
		gotType, gotTypeOK = typevalue.TypeOf(reg, got[0].Contract)
	}
	if len(got) != 1 || got[0].Source != sourceKey || !gotTypeOK || !typ.TypeEquals(gotType, typ.String) {
		t.Fatalf("ParamSinkExposures = %#v, want source placeholder 0 with string contract", got)
	}
}

func TestFromResultProjectsParamSinkExposureFromPathValueSource(t *testing.T) {
	reg := standard.Registry()
	param := symbol.ID(9503)
	sink := symbol.ID(9504)
	graph := cfg.New()
	assign := graph.AddNode(cfg.NodeAssign)
	graph.AddEdge(graph.Entry(), assign, false)
	graph.AddEdge(assign, graph.Exit(), false)
	source, ok := factflow.NewPathValueSource(pathdom.NewPath(param, "").Key(), 0, factflow.NoValueSourceIndex, 0, factflow.ValueSourceShape{Final: true, Adjusted: true})
	if !ok {
		t.Fatal("NewPathValueSource returned false")
	}
	contract := typevalue.FromType(reg, typ.String)
	stub := normalReturnFactProjectAssignmentStub{
		normalReturnFactProjectResultStub: normalReturnFactProjectResultStub{
			reg:           reg,
			graph:         graph,
			exit:          state.State{},
			slots:         []key.Value{key.SymbolValue(param)},
			entryState:    state.State{}.WriteValue(reg, key.SymbolValue(param), contract),
			hasEntryState: true,
		},
		fn: &ast.FunctionExpr{},
		kinds: map[symbol.ID]symbol.Kind{
			sink: symbol.Global,
		},
		pathAssignments: map[cfg.Point]factflow.PathAssignment{
			assign: factflow.NewPathAssignment(pathdom.NewPath(sink, "G").Field("slot"), source),
		},
	}

	got := FromResult(stub).ParamSinkExposures
	sourceKey, ok := pathaddr.RootPlaceholderKeyFromPath(pathdom.NewPlaceholder(0))
	if !ok {
		t.Fatal("RootPlaceholderKeyFromPath failed")
	}
	if len(got) != 1 || got[0].Source != sourceKey {
		t.Fatalf("ParamSinkExposures = %#v, want source placeholder 0 from path value source", got)
	}
}

func TestFromResultProjectsCapturedParamSinkExposureFromPathFact(t *testing.T) {
	reg := standard.Registry()
	param := symbol.ID(9511)
	captured := symbol.ID(9512)
	graph := cfg.New()
	assign := graph.AddNode(cfg.NodeAssign)
	graph.AddEdge(graph.Entry(), assign, false)
	graph.AddEdge(assign, graph.Exit(), false)
	shape, ok := factflow.NewValueSourceShape(true, false, false, false)
	if !ok {
		t.Fatal("NewValueSourceShape returned false")
	}
	source, ok := factflow.NewExpressionValueSource(factflow.ExprRef(1), 0, factflow.NoValueSourceIndex, 0, shape)
	if !ok {
		t.Fatal("NewExpressionValueSource returned false")
	}
	contract := typevalue.FromType(reg, typ.String)
	fn := &ast.FunctionExpr{}
	stub := normalReturnFactProjectAssignmentStub{
		normalReturnFactProjectResultStub: normalReturnFactProjectResultStub{
			reg:   reg,
			graph: graph,
			exit:  state.State{},
			slots: []key.Value{key.SymbolValue(param)},
			exprPaths: map[factflow.ExprRef]pathdom.Path{
				factflow.ExprRef(1): pathdom.NewPath(param, ""),
			},
			entryState:    state.State{}.WriteValue(reg, key.SymbolValue(param), contract),
			hasEntryState: true,
		},
		fn: fn,
		captures: []bind.Capture{{
			Captured:     captured,
			CapturedName: "captured",
		}},
		kinds: map[symbol.ID]symbol.Kind{
			captured: symbol.Local,
		},
		pathAssignments: map[cfg.Point]factflow.PathAssignment{
			assign: factflow.NewPathAssignment(pathdom.NewPath(captured, "captured").Field("slot"), source),
		},
	}

	got := FromResult(stub).ParamSinkExposures
	sourceKey, ok := pathaddr.RootPlaceholderKeyFromPath(pathdom.NewPlaceholder(0))
	if !ok {
		t.Fatal("RootPlaceholderKeyFromPath failed")
	}
	if len(got) != 1 || got[0].Source != sourceKey {
		t.Fatalf("ParamSinkExposures = %#v, want captured sink exposure for source placeholder 0", got)
	}
}
