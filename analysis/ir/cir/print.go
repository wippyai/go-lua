package cir

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
)

// Print renders a Body as deterministic text, one instruction per line, in the
// CFG's reverse post-order (entry first, exit last, unreachable points omitted).
// The output is the golden-test currency: stable operand spelling, no address
// or pointer identity. Points are labelled b0, b1, ... in traversal order and
// branch lines name their successor labels, so control flow is verifiable
// without inspecting the CFG separately.
func Print(b *Body, graph cfg.Graph) string {
	order := cfg.RPOReadOnly(graph)
	label := make(map[cfg.Point]string, len(order))
	for i, p := range order {
		label[p] = "b" + strconv.Itoa(i)
	}

	var sb strings.Builder
	if b.Name != "" {
		sb.WriteString("body ")
		sb.WriteString(b.Name)
		sb.WriteByte('\n')
	}
	for _, p := range order {
		insts := b.PointInstructions(p)
		if len(insts) == 0 {
			// A reachable point with no instruction window still gets a line so the
			// topology reads cleanly.
			writeLine(&sb, label[p], "noop")
			continue
		}
		for i, inst := range insts {
			text := b.spellInstruction(inst)
			if inst.Op == OpBranch {
				text += branchSuccessors(graph, label, p)
			}
			if i == 0 {
				writeLine(&sb, label[p], text)
			} else {
				writeLine(&sb, "", text)
			}
		}
	}
	// Nested function protos print as their own labelled body blocks, appended in
	// definition order so a closure line and its proto read together.
	for _, proto := range b.Protos() {
		if proto.Body == nil || proto.Graph == nil {
			continue
		}
		sb.WriteByte('\n')
		sb.WriteString(Print(proto.Body, proto.Graph))
	}
	return sb.String()
}

func writeLine(sb *strings.Builder, label, text string) {
	if label == "" {
		sb.WriteString("    ")
	} else {
		sb.WriteString(label)
		sb.WriteString(": ")
	}
	sb.WriteString(text)
	sb.WriteByte('\n')
}

// branchSuccessors renders a branch point's successor edges as
// "  then <label> else <label>", naming the true and false CFG edges.
func branchSuccessors(graph cfg.Graph, label map[cfg.Point]string, p cfg.Point) string {
	var thenL, elseL string
	for _, succ := range cfg.SuccessorsReadOnly(graph, p) {
		cond, ok := graph.EdgeCond(p, succ)
		if !ok {
			continue
		}
		if cond {
			thenL = label[succ]
		} else {
			elseL = label[succ]
		}
	}
	return "  then " + fallback(thenL, "-") + " else " + fallback(elseL, "-")
}

func fallback(s, d string) string {
	if s == "" {
		return d
	}
	return s
}

func (b *Body) spellInstruction(inst Instruction) string {
	switch inst.Op {
	case OpNoop:
		return "noop"
	case OpEntry:
		return "entry"
	case OpExit:
		return "exit"
	case OpAssign:
		return b.spellOperand(inst.Dst) + " = " + b.spellOperand(inst.A)
	case OpStaticMemberWrite:
		return "store.field " + b.spellOperand(inst.Dst) + " = " + b.spellOperand(inst.A)
	case OpDynamicIndexWrite:
		return "store.index " + b.spellOperand(inst.Dst) + "[" + b.spellOperand(inst.A) + "] = " + b.spellOperand(inst.B)
	case OpMakeTable:
		return b.spellOperand(inst.Dst) + " = table [" + b.spellOperandList(inst.List) + "]"
	case OpBinOp:
		return b.spellOperand(inst.Dst) + " = " + operatorMnemonic(inst.Operator) + " " + b.spellOperand(inst.A) + " " + b.spellOperand(inst.B)
	case OpUnOp:
		return b.spellOperand(inst.Dst) + " = " + operatorMnemonic(inst.Operator) + " " + b.spellOperand(inst.A)
	case OpConcat:
		return b.spellOperand(inst.Dst) + " = concat [" + b.spellOperandList(inst.List) + "]"
	case OpCall:
		return b.spellCall(inst)
	case OpReturn:
		return b.spellReturn(inst)
	case OpBranch:
		return "branch " + b.spellCheck(inst)
	case OpIterate:
		return b.spellIterate(inst)
	case OpClaim:
		return b.spellClaim(inst)
	case OpSelect:
		s := b.spellOperand(inst.Dst) + " = select [" + b.spellOperandList(inst.List) + "]"
		if inst.SelectDefault {
			s += " default"
		}
		return s
	case OpLogical:
		return b.spellOperand(inst.Dst) + " = " + operatorMnemonic(inst.Operator) + " " + b.spellOperand(inst.A) + " " + b.spellOperand(inst.B)
	case OpClosure:
		s := b.spellOperand(inst.Dst) + " = closure " + b.Proto(inst.Func).Name
		if inst.List.Len > 0 {
			s += " [" + b.spellOperandList(inst.List) + "]"
		}
		return s
	default:
		return "?" + strconv.Itoa(int(inst.Op))
	}
}

func (b *Body) spellCall(inst Instruction) string {
	var sb strings.Builder
	if inst.Results.Len > 0 {
		sb.WriteString(b.spellOperandList(inst.Results))
		sb.WriteString(" = ")
	}
	sb.WriteString("call ")
	if inst.Call.Method != 0 {
		sb.WriteString(b.spellOperand(inst.Call.Receiver))
		sb.WriteByte(':')
		sb.WriteString(b.Const(inst.Call.Method).Str)
	} else {
		sb.WriteString(b.spellOperand(inst.Call.Callee))
	}
	sb.WriteByte('(')
	sb.WriteString(b.spellOperandList(inst.List))
	if inst.ListSpread {
		sb.WriteString("...")
	}
	sb.WriteByte(')')
	if inst.ResultSpread {
		sb.WriteString(" multret")
	}
	return sb.String()
}

func (b *Body) spellReturn(inst Instruction) string {
	if inst.List.Len == 0 {
		return "return"
	}
	s := "return " + b.spellOperandList(inst.List)
	if inst.ListSpread {
		s += "..."
	}
	return s
}

func (b *Body) spellIterate(inst Instruction) string {
	mode := "generic"
	if inst.Iter == IterNumeric {
		mode = "numeric"
	}
	var sb strings.Builder
	if inst.Results.Len > 0 {
		sb.WriteString(b.spellOperandList(inst.Results))
		sb.WriteString(" = ")
	}
	sb.WriteString("iterate.")
	sb.WriteString(mode)
	sb.WriteString(" [")
	sb.WriteString(b.spellOperandList(inst.List))
	sb.WriteByte(']')
	return sb.String()
}

func (b *Body) spellClaim(inst Instruction) string {
	kind := claimMnemonic(inst.Claim)
	s := b.spellOperand(inst.Dst) + " = claim." + kind + " " + b.spellOperand(inst.A)
	if inst.Type != 0 {
		s += " : " + b.TypeSpelling(inst.Type)
	}
	return s
}

func (b *Body) spellOperandList(r OperandRange) string {
	ops := b.Operands(r)
	if len(ops) == 0 {
		return ""
	}
	parts := make([]string, len(ops))
	for i, op := range ops {
		parts[i] = b.spellOperand(op)
	}
	return strings.Join(parts, ", ")
}

func (b *Body) spellOperand(op Operand) string {
	switch op.Kind {
	case OperandNone:
		return "_"
	case OperandPath:
		return b.Path(PathRef(op.Ref)).String()
	case OperandConst:
		return spellConst(b.Const(ConstRef(op.Ref)))
	case OperandType:
		return b.TypeSpelling(TypeRef(op.Ref))
	case OperandTemp:
		return "%" + strconv.FormatUint(uint64(op.Ref), 10)
	case OperandVararg:
		return "..."
	default:
		return "?"
	}
}

func spellConst(c Const) string {
	switch c.Kind {
	case ConstNil:
		return "nil"
	case ConstBool:
		if c.Bool {
			return "true"
		}
		return "false"
	case ConstNumber:
		return c.Number
	case ConstString:
		return strconv.Quote(c.Str)
	default:
		return "?"
	}
}

func (b *Body) spellCheck(inst Instruction) string {
	c := b.Check(inst.Check)
	subject := c.Path.String()
	neg := ""
	if c.Negated {
		neg = " (neg)"
	}
	switch c.Kind {
	case branchcond.CheckNone:
		// A condition that did not normalize to a path check: name the value.
		return "cond " + b.spellOperand(inst.A)
	case branchcond.CheckTruthy:
		return "truthy " + subject
	case branchcond.CheckFalsy:
		return "falsy " + subject
	case branchcond.CheckNil:
		return "nil " + subject
	case branchcond.CheckNotNil:
		return "notnil " + subject
	case branchcond.CheckTypeEqual:
		return "type_eq " + subject + " " + typeCheckOperand(c)
	case branchcond.CheckTypeNot:
		return "type_ne " + subject + " " + typeCheckOperand(c)
	case branchcond.CheckLiteralEqual:
		return "lit_eq " + subject + " " + literalCheckOperand(c)
	case branchcond.CheckLiteralNot:
		return "lit_ne " + subject + " " + literalCheckOperand(c)
	case branchcond.CheckPathEqual:
		return "path_eq " + subject + " " + c.OtherPath.String()
	case branchcond.CheckPathNot:
		return "path_ne " + subject + " " + c.OtherPath.String()
	case branchcond.CheckLenGe:
		return "len_ge " + subject + " " + strconv.FormatInt(c.LenFloor, 10) + neg
	case branchcond.CheckIndexInRange:
		return "in_range " + subject + " " + c.OtherPath.String() + neg
	case branchcond.CheckNumGe:
		return "num_ge " + subject + " " + strconv.FormatInt(c.NumFloor, 10) + neg
	default:
		return "cond?"
	}
}

func typeCheckOperand(c branchcond.Check) string {
	if c.TypeName != "" {
		return strconv.Quote(c.TypeName)
	}
	return "== " + c.OtherPath.String()
}

func literalCheckOperand(c branchcond.Check) string {
	if lit, ok := c.LiteralValue(); ok && lit != nil {
		return lit.String()
	}
	if c.LiteralString != "" {
		return strconv.Quote(c.LiteralString)
	}
	return "?"
}

func operatorMnemonic(op Operator) string {
	switch op {
	case BinAdd:
		return "add"
	case BinSub:
		return "sub"
	case BinMul:
		return "mul"
	case BinDiv:
		return "div"
	case BinIDiv:
		return "idiv"
	case BinMod:
		return "mod"
	case BinPow:
		return "pow"
	case BinBAnd:
		return "band"
	case BinBOr:
		return "bor"
	case BinBXor:
		return "bxor"
	case BinShl:
		return "shl"
	case BinShr:
		return "shr"
	case BinEq:
		return "eq"
	case BinNe:
		return "ne"
	case BinLt:
		return "lt"
	case BinLe:
		return "le"
	case BinGt:
		return "gt"
	case BinGe:
		return "ge"
	case UnNeg:
		return "neg"
	case UnNot:
		return "not"
	case UnLen:
		return "len"
	case UnBNot:
		return "bnot"
	case LogAnd:
		return "and"
	case LogOr:
		return "or"
	default:
		return fmt.Sprintf("op%d", op)
	}
}

func claimMnemonic(k ClaimKind) string {
	switch k {
	case ClaimCast:
		return "cast"
	case ClaimAssert:
		return "assert"
	case ClaimAnnotation:
		return "annotation"
	case ClaimAssertsPredicate:
		return "asserts"
	default:
		return "none"
	}
}
