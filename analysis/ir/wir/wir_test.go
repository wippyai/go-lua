package wir

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/type/typ"
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
	if b.InternType(typ.String) != b.InternType(typ.String) {
		t.Fatalf("type interning not deduped")
	}
	if b.InternType(typ.String) == b.InternType(typ.Number) {
		t.Fatalf("distinct resolved types must intern distinctly")
	}
	if b.InternType(nil) != 0 {
		t.Fatalf("nil (unresolved) type must intern to none")
	}
	// Distinct checks are never deduped: each branch owns one.
	if b.InternCheck(b.Check(0)) == b.InternCheck(b.Check(0)) {
		t.Fatalf("checks must not dedupe")
	}
}

func TestBranchChecksExposePointBranchesOnly(t *testing.T) {
	b := NewBody("branches")
	g := cfg.New()
	entry, exit := g.Entry(), g.Exit()
	p := g.AddNode(cfg.NodeBranch)
	g.AddEdge(entry, p, false)
	g.AddEdge(p, exit, true)
	g.AddEdge(p, exit, false)

	x := path.Path{Root: "x", Symbol: 1}
	y := path.Path{Root: "y", Symbol: 2}
	xCheck := Check{Kind: CheckTruthy, Path: x}
	yCheck := Check{Kind: CheckNil, Path: y}

	start := b.Len()
	b.Emit(Instruction{Op: OpNoop, Point: p})
	b.Emit(Instruction{Op: OpBranch, Point: p, Check: b.InternCheck(xCheck)})
	b.Emit(Instruction{Op: OpBranch, Point: p, Check: b.InternCheck(yCheck)})
	b.SetPointRange(p, start, b.Len())

	got := b.BranchChecks(p)
	if len(got) != 2 {
		t.Fatalf("BranchChecks returned %d checks, want 2: %#v", len(got), got)
	}
	if got[0].Kind != CheckTruthy || !got[0].Path.Equal(x) {
		t.Fatalf("first check = %#v, want truthy x", got[0])
	}
	if got[1].Kind != CheckNil || !got[1].Path.Equal(y) {
		t.Fatalf("second check = %#v, want nil y", got[1])
	}
	if missing := b.BranchChecks(exit); len(missing) != 0 {
		t.Fatalf("BranchChecks(exit) = %#v, want none", missing)
	}
	if !b.HasInstruction(p, OpBranch) {
		t.Fatalf("HasInstruction(point, OpBranch) = false, want true")
	}
	if b.HasInstruction(p, OpReturn) {
		t.Fatalf("HasInstruction(point, OpReturn) = true, want false")
	}

	var visited int
	b.ForEachBranchCheck(p, func(Check) bool {
		visited++
		return false
	})
	if visited != 1 {
		t.Fatalf("ForEachBranchCheck visited %d checks after stop, want 1", visited)
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
