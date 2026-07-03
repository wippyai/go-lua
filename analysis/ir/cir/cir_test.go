package cir

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

func TestInternPoolsAreOneBasedAndDeduped(t *testing.T) {
	b := NewBody("t")

	if ref := b.InternPath(path.Path{}); ref != 0 {
		t.Fatalf("empty path must intern to none, got %d", ref)
	}
	p := path.Path{Root: "x", Symbol: 5}
	r1 := b.InternPath(p)
	r2 := b.InternPath(p)
	if r1 == 0 || r1 != r2 {
		t.Fatalf("path interning not stable/one-based: %d %d", r1, r2)
	}

	c := Const{Kind: ConstNumber, Number: "42"}
	if b.InternConst(c) != b.InternConst(c) {
		t.Fatalf("const interning not deduped")
	}
	if b.InternType("string") != b.InternType("string") {
		t.Fatalf("type interning not deduped")
	}
	if b.InternType("") != 0 {
		t.Fatalf("empty type must intern to none")
	}
	// Distinct checks are never deduped: each branch owns one.
	if b.InternCheck(b.Check(0)) == b.InternCheck(b.Check(0)) {
		t.Fatalf("checks must not dedupe")
	}
}

func TestPrintHandBuiltBody(t *testing.T) {
	b := NewBody("hand")
	g := cfg.New()
	entry, exit := g.Entry(), g.Exit()

	p := g.AddNode(cfg.NodeAssign)
	g.AddEdge(entry, p, false)
	g.AddEdge(p, exit, false)

	dst := Operand{Kind: OperandPath, Ref: uint32(b.InternPath(path.Path{Root: "c", Symbol: 1}))}
	a := Operand{Kind: OperandPath, Ref: uint32(b.InternPath(path.Path{Root: "a", Symbol: 2}))}
	bb := Operand{Kind: OperandPath, Ref: uint32(b.InternPath(path.Path{Root: "b", Symbol: 3}))}

	start := b.Len()
	b.Emit(Instruction{Op: OpEntry, Point: entry})
	b.SetPointRange(entry, start, b.Len())

	start = b.Len()
	b.Emit(Instruction{Op: OpBinOp, Point: p, Dst: dst, A: a, B: bb, Operator: BinAdd})
	b.SetPointRange(p, start, b.Len())

	start = b.Len()
	b.Emit(Instruction{Op: OpExit, Point: exit})
	b.SetPointRange(exit, start, b.Len())

	want := "body hand\nb0: entry\nb1: c = add a b\nb2: exit\n"
	if got := Print(b, g); got != want {
		t.Fatalf("print mismatch\n got: %q\nwant: %q", got, want)
	}
}
