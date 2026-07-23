package front

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/symbol"
)

func TestCompileWIRLowersOrderedChannelSelectCases(t *testing.T) {
	graph := cfg.New()
	point := graph.AddNode(cfg.NodeCall)
	graph.AddEdge(graph.Entry(), point, false)
	graph.AddEdge(point, graph.Exit(), false)

	body := wir.NewBody("select")
	first := body.InternPath(pathdom.NewPath(symbol.ID(1), "first"))
	second := body.InternPath(pathdom.NewPath(symbol.ID(2), "second"))
	body.Emit(wir.Instruction{Op: wir.OpEntry, Point: graph.Entry()})
	body.Emit(wir.Instruction{
		Op:            wir.OpSelect,
		Point:         point,
		Dst:           wir.Operand{Kind: wir.OperandTemp, Ref: 4},
		List:          body.AppendOperands([]wir.Operand{{Kind: wir.OperandPath, Ref: uint32(first)}, {Kind: wir.OperandPath, Ref: uint32(second)}}),
		SelectDefault: true,
	})
	body.Emit(wir.Instruction{Op: wir.OpExit, Point: graph.Exit()})

	artifact, err := compileWIR("ordered channel select", body, graph, map[cfg.Point]cfg.Point{})
	if err != nil {
		t.Fatalf("compileWIR: %v", err)
	}
	if len(artifact.Equations) != 2 {
		t.Fatalf("equation count = %d, want 2", len(artifact.Equations))
	}
	selectEquation := artifact.Equations[1]
	if selectEquation.Occurrence.Kind != "channel-select" || selectEquation.KernelID != selectKernel {
		t.Fatalf("select equation = %#v", selectEquation)
	}
	roles := make(map[string]string, len(selectEquation.Operands))
	for _, operand := range selectEquation.Operands {
		roles[operand.Role] = string(operand.Term.Encoding)
	}
	if roles["default"] != "select/default/true" {
		t.Fatalf("select default = %q, want true marker", roles["default"])
	}
	if roles["case-00000000"] != "path/"+string(body.Path(first).Key()) || roles["case-00000001"] != "path/"+string(body.Path(second).Key()) {
		t.Fatalf("ordered select cases = %#v", roles)
	}
	if _, payload := roles["payload"]; payload {
		t.Fatalf("select invented payload operand %#v", roles)
	}
}
