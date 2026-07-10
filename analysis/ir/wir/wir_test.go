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
	constRef1, constRef2 := b.InternConst(c), b.InternConst(c)
	if constRef1 != constRef2 {
		t.Fatalf("const interning not deduped")
	}
	stringRef1, stringRef2 := b.InternType(typ.String), b.InternType(typ.String)
	if stringRef1 != stringRef2 {
		t.Fatalf("type interning not deduped")
	}
	if b.InternType(typ.String) == b.InternType(typ.Number) {
		t.Fatalf("distinct resolved types must intern distinctly")
	}
	if b.InternType(nil) != 0 {
		t.Fatalf("nil (unresolved) type must intern to none")
	}
	// Distinct checks are never deduped: each branch owns one.
	checkRef1, checkRef2 := b.InternCheck(b.Check(0)), b.InternCheck(b.Check(0))
	if checkRef1 == checkRef2 {
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

func TestBranchImpliedChecksUseFlatPool(t *testing.T) {
	b := NewBody("branch-implied")
	p := cfg.Point(3)
	x := path.Path{Root: "x", Symbol: 1}
	y := path.Path{Root: "y", Symbol: 2}
	checks := []ImpliedCheck{
		{Check: Check{Kind: CheckTruthy, Path: x}, Edge: true, Polarity: true},
		{Check: Check{Kind: CheckNil, Path: y}, Edge: false, Polarity: true},
	}
	start := b.Len()
	b.Emit(Instruction{
		Op:            OpBranch,
		Point:         p,
		Check:         b.InternCheck(Check{}),
		ImpliedChecks: b.AppendImpliedChecks(checks),
	})
	b.SetPointRange(p, start, b.Len())

	insts := b.PointInstructions(p)
	if len(insts) != 1 {
		t.Fatalf("point instructions = %d, want 1", len(insts))
	}
	got := b.ImpliedChecks(insts[0].ImpliedChecks)
	if len(got) != len(checks) {
		t.Fatalf("implied checks = %d, want %d: %#v", len(got), len(checks), got)
	}
	if got[0].Check.Kind != CheckTruthy || !got[0].Check.Path.Equal(x) || !got[0].Edge || !got[0].Polarity {
		t.Fatalf("first implied check = %#v, want truthy x on true edge", got[0])
	}
	if got[1].Check.Kind != CheckNil || !got[1].Check.Path.Equal(y) || got[1].Edge || !got[1].Polarity {
		t.Fatalf("second implied check = %#v, want nil y on false edge", got[1])
	}
}

func TestBranchDiffConstraintsUseFlatPool(t *testing.T) {
	b := NewBody("branch-diff")
	p := cfg.Point(4)
	i := path.Path{Root: "i", Symbol: 1}
	xs := path.Path{Root: "xs", Symbol: 2}
	diffs := []BranchDiffConstraint{
		{CoHi: 1, HiPath: i, LoPath: xs, LoIsLen: true, C: -1, Edge: true},
	}
	start := b.Len()
	b.Emit(Instruction{
		Op:              OpBranch,
		Point:           p,
		Check:           b.InternCheck(Check{}),
		DiffConstraints: b.AppendBranchDiffConstraints(diffs),
	})
	b.SetPointRange(p, start, b.Len())

	insts := b.PointInstructions(p)
	if len(insts) != 1 {
		t.Fatalf("point instructions = %d, want 1", len(insts))
	}
	got := b.BranchDiffConstraints(insts[0].DiffConstraints)
	if len(got) != len(diffs) {
		t.Fatalf("diff constraints = %d, want %d: %#v", len(got), len(diffs), got)
	}
	if got[0].CoHi != 1 || !got[0].HiPath.Equal(i) || !got[0].LoPath.Equal(xs) || !got[0].LoIsLen || got[0].C != -1 || !got[0].Edge {
		t.Fatalf("first diff constraint = %#v, want i - #xs <= -1 on true edge", got[0])
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

func TestInstructionAssignmentSourceOperand(t *testing.T) {
	a := Operand{Kind: OperandPath, Ref: 1}
	b := Operand{Kind: OperandPath, Ref: 2}

	tests := []struct {
		name string
		inst Instruction
		want Operand
		ok   bool
	}{
		{name: "assign", inst: Instruction{Op: OpAssign, A: a}, want: a, ok: true},
		{name: "static member write", inst: Instruction{Op: OpStaticMemberWrite, A: a}, want: a, ok: true},
		{name: "dynamic index write", inst: Instruction{Op: OpDynamicIndexWrite, A: a, B: b}, want: b, ok: true},
		{name: "dynamic index write missing value", inst: Instruction{Op: OpDynamicIndexWrite, A: a}, ok: false},
		{name: "call", inst: Instruction{Op: OpCall, A: a}, ok: false},
	}
	for _, tc := range tests {
		got, ok := tc.inst.AssignmentSourceOperand()
		if ok != tc.ok || got != tc.want {
			t.Fatalf("%s: AssignmentSourceOperand = %#v/%v, want %#v/%v", tc.name, got, ok, tc.want, tc.ok)
		}
	}
}

func TestInstructionWritesAssignmentPoint(t *testing.T) {
	dst := Operand{Kind: OperandPath, Ref: 1}

	writes := []Op{OpAssign, OpDynamicIndexRead, OpMakeTable, OpBinOp, OpUnOp, OpConcat, OpClaim, OpSelect, OpLogical, OpClosure}
	for _, op := range writes {
		if !((Instruction{Op: op, Dst: dst}).WritesAssignmentPoint()) {
			t.Fatalf("%v with destination must write assignment point", op)
		}
		if (Instruction{Op: op}).WritesAssignmentPoint() {
			t.Fatalf("%v without destination must not write assignment point", op)
		}
	}
	for _, op := range []Op{OpStaticMemberWrite, OpDynamicIndexWrite} {
		if !(Instruction{Op: op}).WritesAssignmentPoint() {
			t.Fatalf("%v must write assignment point", op)
		}
	}
	if (Instruction{Op: OpCall, Dst: dst}).WritesAssignmentPoint() {
		t.Fatalf("call must not be classified as assignment write by destination slot")
	}
}

func TestTableConstructorByExpressionID(t *testing.T) {
	body := NewBody("tables")
	first := ExpressionID(101)
	second := ExpressionID(202)
	body.Emit(Instruction{Op: OpMakeTable, ExprID: first, Dst: Operand{Kind: OperandTemp, Ref: 1}})
	body.Emit(Instruction{Op: OpMakeTable, ExprID: second, Dst: Operand{Kind: OperandTemp, Ref: 2}})

	got, ok := body.TableConstructorByExpressionID(second)
	if !ok || got.ExprID != second || got.Dst.Ref != 2 {
		t.Fatalf("TableConstructorByExpressionID(second) = %#v/%v", got, ok)
	}
	if got, ok := body.TableConstructorByExpressionID(303); ok || got.Op != OpNoop {
		t.Fatalf("TableConstructorByExpressionID(missing) = %#v/%v, want none", got, ok)
	}
	if got, ok := body.TableConstructorByExpressionID(0); ok || got.Op != OpNoop {
		t.Fatalf("TableConstructorByExpressionID(0) = %#v/%v, want none", got, ok)
	}
}
