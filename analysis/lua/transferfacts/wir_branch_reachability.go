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
		truthy, ok := l.wirStaticLuaTruthinessAt(point, inst.A)
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
		for _, temp := range wirInstructionDefinedTemps(body, inst) {
			if ambiguous[temp] {
				continue
			}
			if _, exists := defs[temp]; exists {
				delete(defs, temp)
				ambiguous[temp] = true
				continue
			}
			defs[temp] = inst
		}
	}
	return defs
}

func (l *lowerer) wirTempDefSets() map[uint32][]wir.Instruction {
	if l.wirTempDefinitionSets != nil {
		return l.wirTempDefinitionSets
	}
	l.wirTempDefinitionSets = wirTempDefinitionSets(l.wir)
	return l.wirTempDefinitionSets
}

func wirTempDefinitionSets(body *wir.Body) map[uint32][]wir.Instruction {
	if body == nil || body.Len() == 0 {
		return nil
	}
	defs := make(map[uint32][]wir.Instruction)
	for i := 0; i < body.Len(); i++ {
		inst := body.Instr(i)
		for _, temp := range wirInstructionDefinedTemps(body, inst) {
			defs[temp] = append(defs[temp], inst)
		}
	}
	return defs
}

func wirInstructionDefinedTemps(body *wir.Body, inst wir.Instruction) []uint32 {
	if inst.Dst.Kind == wir.OperandTemp {
		return []uint32{inst.Dst.Ref}
	}
	if inst.Results.Len == 0 {
		return nil
	}
	var out []uint32
	for _, result := range body.Operands(inst.Results) {
		if result.Kind == wir.OperandTemp {
			out = append(out, result.Ref)
		}
	}
	return out
}

func (l *lowerer) wirStaticLuaTruthinessAt(point cfg.Point, operand wir.Operand) (bool, bool) {
	if truthy, ok := wirStaticLuaTruthiness(l.wir, operand, l.wirTempDefs()); ok {
		return truthy, true
	}
	if operand.Kind != wir.OperandTemp || l.graph == nil {
		return false, false
	}
	defs := l.wirTempDefSets()[operand.Ref]
	if len(defs) == 0 {
		return false, false
	}
	var out bool
	var have bool
	for _, def := range defs {
		if !l.wirPointStaticallyReachable(def.Point) {
			continue
		}
		if def.Point != point {
			if l.wirReachability == nil {
				l.wirReachability = cfg.NewReachability(l.graph)
			}
			if !l.wirReachability.CanReach(def.Point, point) {
				continue
			}
		}
		truthy, ok := wirInstructionTruthiness(l.wir, def, l.wirTempDefs(), nil)
		if !ok {
			return false, false
		}
		if have && truthy != out {
			return false, false
		}
		out = truthy
		have = true
	}
	return out, have
}

func (l *lowerer) wirPointStaticallyReachable(point cfg.Point) bool {
	if l.graph == nil {
		return true
	}
	reachable := l.wirStaticReachablePoints()
	return reachable[point]
}

func (l *lowerer) wirStaticReachablePoints() map[cfg.Point]bool {
	if l.wirStaticReachable != nil {
		return l.wirStaticReachable
	}
	reachable := make(map[cfg.Point]bool)
	if l.graph == nil {
		l.wirStaticReachable = reachable
		return reachable
	}
	stack := []cfg.Point{l.graph.Entry()}
	for len(stack) != 0 {
		point := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if reachable[point] {
			continue
		}
		reachable[point] = true
		edgeReachability, hasEdgeReachability := l.staticWIRBranchEdgeReachability(point)
		successors := cfg.SuccessorsReadOnly(l.graph, point)
		conditions := cfg.SuccessorConditionsReadOnly(l.graph, point)
		for index, succ := range successors {
			if hasEdgeReachability {
				if len(conditions) == len(successors) && edgeReachability.EdgeUnreachable(conditions[index]) {
					continue
				}
			}
			stack = append(stack, succ)
		}
	}
	l.wirStaticReachable = reachable
	return reachable
}

func (l *lowerer) staticWIRBranchEdgeReachability(point cfg.Point) (factflow.BranchEdgeReachability, bool) {
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
