package transferfacts

import (
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
)

func (l *lowerer) branchEdgeReachabilityFromWIR(point cfg.Point) (factflow.BranchEdgeReachability, bool) {
	if l.wir == nil {
		return factflow.BranchEdgeReachability{}, false
	}
	for _, inst := range l.wir.PointInstructions(point) {
		if inst.Op != wir.OpBranch || l.wir.Check(inst.Check).Kind != wir.CheckNone {
			continue
		}
		truthy, ok := wirStaticLuaTruthiness(l.wir, inst.A, l.wirTempDefs())
		if !ok {
			return factflow.BranchEdgeReachability{}, false
		}
		return factflow.NewBranchEdgeReachability(!truthy, truthy), true
	}
	return factflow.BranchEdgeReachability{}, false
}

func (l *lowerer) wirTempDefs() map[uint32]wir.Instruction {
	if l.wirTempDefinitions != nil {
		return l.wirTempDefinitions
	}
	l.wirTempDefinitions = wirTempDefinitions(l.wir)
	return l.wirTempDefinitions
}

func wirStaticLuaTruthiness(body *wir.Body, operand wir.Operand, defs map[uint32]wir.Instruction) (bool, bool) {
	return wirStaticLuaTruthinessWithDefs(body, operand, defs, nil)
}

func wirTempDefinitions(body *wir.Body) map[uint32]wir.Instruction {
	if body == nil || body.Len() == 0 {
		return nil
	}
	defs := make(map[uint32]wir.Instruction)
	ambiguous := make(map[uint32]bool)
	for i := 0; i < body.Len(); i++ {
		inst := body.Instr(i)
		if inst.Dst.Kind == wir.OperandTemp {
			if ambiguous[inst.Dst.Ref] {
				continue
			}
			if _, exists := defs[inst.Dst.Ref]; exists {
				delete(defs, inst.Dst.Ref)
				ambiguous[inst.Dst.Ref] = true
				continue
			}
			defs[inst.Dst.Ref] = inst
		}
	}
	return defs
}

func wirStaticLuaTruthinessWithDefs(
	body *wir.Body,
	operand wir.Operand,
	defs map[uint32]wir.Instruction,
	seen map[uint32]bool,
) (bool, bool) {
	switch operand.Kind {
	case wir.OperandConst:
		return wirConstTruthiness(body.Const(wir.ConstRef(operand.Ref)))
	case wir.OperandTemp:
		if seen[operand.Ref] {
			return false, false
		}
		if seen == nil {
			seen = make(map[uint32]bool)
		}
		seen[operand.Ref] = true
		inst, ok := defs[operand.Ref]
		if !ok {
			return false, false
		}
		return wirInstructionTruthiness(body, inst, defs, seen)
	default:
		return false, false
	}
}

func wirInstructionTruthiness(
	body *wir.Body,
	inst wir.Instruction,
	defs map[uint32]wir.Instruction,
	seen map[uint32]bool,
) (bool, bool) {
	switch inst.Op {
	case wir.OpAssign:
		return wirStaticLuaTruthinessWithDefs(body, inst.A, defs, seen)
	case wir.OpUnOp:
		if inst.Operator != wir.UnNot {
			return false, false
		}
		truthy, ok := wirStaticLuaTruthinessWithDefs(body, inst.A, defs, seen)
		return !truthy, ok
	case wir.OpLogical:
		return wirLogicalTruthiness(body, inst, defs, seen)
	case wir.OpClaim:
		return wirStaticLuaTruthinessWithDefs(body, inst.A, defs, seen)
	case wir.OpMakeTable, wir.OpClosure:
		return true, true
	default:
		return false, false
	}
}

func wirLogicalTruthiness(
	body *wir.Body,
	inst wir.Instruction,
	defs map[uint32]wir.Instruction,
	seen map[uint32]bool,
) (bool, bool) {
	left, ok := wirStaticLuaTruthinessWithDefs(body, inst.A, defs, seen)
	if !ok {
		return false, false
	}
	switch inst.Operator {
	case wir.LogAnd:
		if !left {
			return false, true
		}
		return wirStaticLuaTruthinessWithDefs(body, inst.B, defs, seen)
	case wir.LogOr:
		if left {
			return true, true
		}
		return wirStaticLuaTruthinessWithDefs(body, inst.B, defs, seen)
	default:
		return false, false
	}
}

func wirConstTruthiness(c wir.Const) (bool, bool) {
	switch c.Kind {
	case wir.ConstNil:
		return false, true
	case wir.ConstBool:
		return c.Bool, true
	case wir.ConstNumber, wir.ConstString:
		return true, true
	default:
		return false, false
	}
}
