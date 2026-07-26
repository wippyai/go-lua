package wirprint

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
)

func TestPrintHandBuiltBody(t *testing.T) {
	body := wir.NewBody("hand")
	graph := cfg.New()
	entry, exit := graph.Entry(), graph.Exit()

	point := graph.AddNode(cfg.NodeAssign)
	graph.AddEdge(entry, point, false)
	graph.AddEdge(point, exit, false)

	dst := wir.Operand{Kind: wir.OperandPath, Ref: uint32(body.InternPath(path.Path{Root: "c", Symbol: 1}))}
	left := wir.Operand{Kind: wir.OperandPath, Ref: uint32(body.InternPath(path.Path{Root: "a", Symbol: 2}))}
	right := wir.Operand{Kind: wir.OperandPath, Ref: uint32(body.InternPath(path.Path{Root: "b", Symbol: 3}))}

	start := body.Len()
	body.Emit(wir.Instruction{Op: wir.OpEntry, Point: entry})
	body.SetPointRange(entry, start, body.Len())

	start = body.Len()
	body.Emit(wir.Instruction{Op: wir.OpBinOp, Point: point, Dst: dst, A: left, B: right, Operator: wir.BinAdd})
	body.SetPointRange(point, start, body.Len())

	start = body.Len()
	body.Emit(wir.Instruction{Op: wir.OpExit, Point: exit})
	body.SetPointRange(exit, start, body.Len())

	want := "body hand\nb0: entry\nb1: c = add a b\nb2: exit\n"
	if got := Print(body, graph); got != want {
		t.Fatalf("print mismatch\n got: %q\nwant: %q", got, want)
	}
}
