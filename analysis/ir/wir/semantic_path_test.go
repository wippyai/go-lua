package wir

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
)

func TestForEachValuePathCoversSemanticCarriersAndOnlyOmitsDirectCalleeRoot(t *testing.T) {
	body := NewBody("semantic-paths")
	makePath := func(id symbol.ID) path.Path { return path.Path{Symbol: id} }
	operand := func(id symbol.ID) Operand {
		return Operand{Kind: OperandPath, Ref: uint32(body.InternPath(makePath(id)))}
	}
	point := cfg.Point(3)
	directCallee := operand(90)
	body.Emit(Instruction{
		Op:      OpCall,
		Point:   point,
		Dst:     operand(1),
		A:       operand(2),
		B:       operand(3),
		List:    body.AppendOperands([]Operand{operand(4)}),
		Results: body.AppendOperands([]Operand{operand(5)}),
		Call:    CallInfo{Callee: directCallee, Receiver: operand(6)},
		Check: body.InternCheck(Check{
			Kind: CheckPathEqual, Path: makePath(7), OtherPath: makePath(8),
		}),
		ImpliedChecks:    body.AppendImpliedChecks([]ImpliedCheck{{Check: Check{Kind: CheckTruthy, Path: makePath(9)}}}),
		SufficientChecks: body.AppendImpliedChecks([]ImpliedCheck{{Check: Check{Kind: CheckTruthy, Path: makePath(10)}}}),
		DiffConstraints: body.AppendBranchDiffConstraints([]BranchDiffConstraint{{
			HiPath: makePath(11), HasHi2: true, Hi2Path: makePath(12), LoPath: makePath(13),
		}}),
		TableEntries: body.AppendTableEntries([]TableEntry{{
			Suffix: makePath(14), Value: operand(15),
		}}),
	})
	// A descendant callee is a value lookup and is therefore not excluded.
	descendant := makePath(91).Field("method")
	body.Emit(Instruction{Op: OpCall, Point: point, Call: CallInfo{
		Callee: Operand{Kind: OperandPath, Ref: uint32(body.InternPath(descendant))},
	}})
	body.SetCallResultTarget(point, CallResultTarget{Kind: CallResultTargetOrdinaryAssignment, Path: makePath(16)})

	seen := make(map[path.PathKey]int)
	if !body.ForEachValuePath(func(p path.Path) bool {
		seen[p.Key()]++
		return true
	}) {
		t.Fatal("semantic path traversal stopped")
	}
	for id := symbol.ID(1); id <= 16; id++ {
		if seen[makePath(id).Key()] == 0 {
			t.Fatalf("semantic path carrier %d was omitted: %v", id, seen)
		}
	}
	if seen[makePath(90).Key()] != 0 {
		t.Fatal("root-only direct callee leaked into value paths")
	}
	if seen[descendant.Key()] == 0 {
		t.Fatal("descendant callee value path was omitted")
	}

	visited := 0
	if body.ForEachValuePath(func(path.Path) bool { visited++; return false }) || visited != 1 {
		t.Fatalf("early stop = %v visits, want one", visited)
	}
}
