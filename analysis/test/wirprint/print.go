// Package wirprint contains WIR presentation helpers used only by tests.
package wirprint

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
)

// Print renders a Body as deterministic text, one instruction per line, in the
// CFG's reverse post-order (entry first, exit last, unreachable points omitted).
// The output is the golden-test currency: stable operand spelling, no address
// or pointer identity. Points are labelled b0, b1, ... in traversal order and
// branch lines name their successor labels, so control flow is verifiable
// without inspecting the CFG separately.
func Print(b *wir.Body, graph cfg.Graph) string {
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
			text := spellInstruction(b, inst)
			if inst.Op == wir.OpBranch {
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

func spellInstruction(b *wir.Body, inst wir.Instruction) string {
	switch inst.Op {
	case wir.OpNoop:
		return "noop"
	case wir.OpEntry:
		return "entry"
	case wir.OpExit:
		return "exit"
	case wir.OpAssign:
		return spellOperand(b, inst.Dst) + " = " + spellOperand(b, inst.A)
	case wir.OpStaticMemberWrite:
		return "store.field " + spellOperand(b, inst.Dst) + " = " + spellOperand(b, inst.A)
	case wir.OpDynamicIndexWrite:
		s := "store.index " + spellOperand(b, inst.Dst) + "[" + spellOperand(b, inst.A) + "]"
		if suffix := b.Segments(inst.DynamicSuffix); len(suffix) != 0 {
			s += segment.FormatSegments(suffix)
		}
		return s + " = " + spellOperand(b, inst.B)
	case wir.OpDynamicIndexRead:
		return spellOperand(b, inst.Dst) + " = index " + spellOperand(b, inst.A) + "[" + spellOperand(b, inst.B) + "]"
	case wir.OpMakeTable:
		return spellOperand(b, inst.Dst) + " = table [" + spellOperandList(b, inst.List) + "]"
	case wir.OpBinOp:
		s := spellOperand(b, inst.Dst) + " = " + operatorMnemonic(inst.Operator) + " " + spellOperand(b, inst.A) + " " + spellOperand(b, inst.B)
		if inst.Check != 0 {
			s += " check[" + spellCheck(b, inst) + "]"
		}
		return s
	case wir.OpUnOp:
		return spellOperand(b, inst.Dst) + " = " + operatorMnemonic(inst.Operator) + " " + spellOperand(b, inst.A)
	case wir.OpConcat:
		return spellOperand(b, inst.Dst) + " = concat [" + spellOperandList(b, inst.List) + "]"
	case wir.OpCall:
		return spellCall(b, inst)
	case wir.OpReturn:
		return spellReturn(b, inst)
	case wir.OpBranch:
		return "branch " + spellCheck(b, inst)
	case wir.OpIterate:
		return spellIterate(b, inst)
	case wir.OpClaim:
		return spellClaim(b, inst)
	case wir.OpSelect:
		s := spellOperand(b, inst.Dst) + " = select [" + spellOperandList(b, inst.List) + "]"
		if inst.SelectDefault {
			s += " default"
		}
		return s
	case wir.OpLogical:
		return spellOperand(b, inst.Dst) + " = " + operatorMnemonic(inst.Operator) + " " + spellOperand(b, inst.A) + " " + spellOperand(b, inst.B)
	case wir.OpClosure:
		s := spellOperand(b, inst.Dst) + " = closure " + b.Proto(inst.Func).Name
		if inst.List.Len > 0 {
			s += " [" + spellOperandList(b, inst.List) + "]"
		}
		return s
	default:
		return "?" + strconv.Itoa(int(inst.Op))
	}
}

func spellCall(b *wir.Body, inst wir.Instruction) string {
	var sb strings.Builder
	if inst.Results.Len > 0 {
		sb.WriteString(spellOperandList(b, inst.Results))
		sb.WriteString(" = ")
	}
	sb.WriteString("call ")
	if inst.Call.Method != 0 {
		sb.WriteString(spellOperand(b, inst.Call.Receiver))
		sb.WriteByte(':')
		sb.WriteString(b.Const(inst.Call.Method).Str)
	} else {
		sb.WriteString(spellOperand(b, inst.Call.Callee))
	}
	if inst.CallTypeArgs.Len > 0 {
		sb.WriteByte('<')
		for i, ref := range b.TypeRefs(inst.CallTypeArgs) {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(b.TypeDisplay(ref))
		}
		sb.WriteByte('>')
	}
	sb.WriteByte('(')
	sb.WriteString(spellOperandList(b, inst.List))
	if inst.ListSpread {
		sb.WriteString("...")
	}
	sb.WriteByte(')')
	if inst.ResultSpread {
		sb.WriteString(" multret")
	}
	if inst.Check != 0 {
		sb.WriteString(" check[")
		sb.WriteString(spellCheck(b, inst))
		sb.WriteByte(']')
	}
	return sb.String()
}

func spellReturn(b *wir.Body, inst wir.Instruction) string {
	if inst.List.Len == 0 {
		return "return"
	}
	s := "return " + spellOperandList(b, inst.List)
	if inst.ListSpread {
		s += "..."
	}
	return s
}

func spellIterate(b *wir.Body, inst wir.Instruction) string {
	mode := "generic"
	if inst.Iter == wir.IterNumeric {
		mode = "numeric"
	}
	var sb strings.Builder
	if inst.Results.Len > 0 {
		sb.WriteString(spellOperandList(b, inst.Results))
		sb.WriteString(" = ")
	}
	sb.WriteString("iterate.")
	sb.WriteString(mode)
	sb.WriteString(" [")
	sb.WriteString(spellOperandList(b, inst.List))
	sb.WriteByte(']')
	return sb.String()
}

func spellClaim(b *wir.Body, inst wir.Instruction) string {
	kind := claimMnemonic(inst.Claim)
	s := spellOperand(b, inst.Dst) + " = claim." + kind + " " + spellOperand(b, inst.A)
	if inst.Type != 0 {
		s += " : " + b.TypeDisplay(inst.Type)
	}
	return s
}

func spellOperandList(b *wir.Body, r wir.OperandRange) string {
	ops := b.Operands(r)
	if len(ops) == 0 {
		return ""
	}
	parts := make([]string, len(ops))
	for i, op := range ops {
		parts[i] = spellOperand(b, op)
	}
	return strings.Join(parts, ", ")
}

func spellOperand(b *wir.Body, op wir.Operand) string {
	switch op.Kind {
	case wir.OperandNone:
		return "_"
	case wir.OperandPath:
		return b.Path(wir.PathRef(op.Ref)).String()
	case wir.OperandConst:
		return spellConst(b.Const(wir.ConstRef(op.Ref)))
	case wir.OperandType:
		return b.TypeDisplay(wir.TypeRef(op.Ref))
	case wir.OperandTemp:
		return "%" + strconv.FormatUint(uint64(op.Ref), 10)
	case wir.OperandVararg:
		return "..."
	default:
		return "?"
	}
}

func spellConst(c wir.Const) string {
	switch c.Kind {
	case wir.ConstNil:
		return "nil"
	case wir.ConstBool:
		if c.Bool {
			return "true"
		}
		return "false"
	case wir.ConstNumber:
		return c.Number
	case wir.ConstString:
		return strconv.Quote(c.Str)
	default:
		return "?"
	}
}

func spellCheck(b *wir.Body, inst wir.Instruction) string {
	c := b.Check(inst.Check)
	subject := c.Path.String()
	neg := ""
	if c.Negated {
		neg = " (neg)"
	}
	switch c.Kind {
	case wir.CheckNone:
		// A condition that did not normalize to a path check: name the value.
		return "cond " + spellOperand(b, inst.A)
	case wir.CheckTruthy:
		return "truthy " + subject
	case wir.CheckFalsy:
		return "falsy " + subject
	case wir.CheckNil:
		return "nil " + subject
	case wir.CheckNotNil:
		return "notnil " + subject
	case wir.CheckTypeEqual:
		return "type_eq " + subject + " " + typeCheckOperand(c)
	case wir.CheckTypeNot:
		return "type_ne " + subject + " " + typeCheckOperand(c)
	case wir.CheckLiteralEqual:
		return "lit_eq " + subject + " " + literalCheckOperand(c)
	case wir.CheckLiteralNot:
		return "lit_ne " + subject + " " + literalCheckOperand(c)
	case wir.CheckPathEqual:
		return "path_eq " + subject + " " + c.OtherPath.String()
	case wir.CheckPathNot:
		return "path_ne " + subject + " " + c.OtherPath.String()
	case wir.CheckLenGe:
		return "len_ge " + subject + " " + strconv.FormatInt(c.LenFloor, 10) + neg
	case wir.CheckIndexInRange:
		return "in_range " + subject + " " + c.OtherPath.String() + neg
	case wir.CheckNumGe:
		return "num_ge " + subject + " " + strconv.FormatInt(c.NumFloor, 10) + neg
	case wir.CheckNumLe:
		return "num_le " + subject + " " + strconv.FormatInt(c.NumCeil, 10) + neg
	case wir.CheckFrozenTable:
		return "frozen " + subject
	case wir.CheckModResidue:
		return "mod_residue " + subject + " " + strconv.FormatInt(c.Modulus, 10) + " " + strconv.FormatInt(c.Residue, 10) + neg
	default:
		return "cond?"
	}
}

func typeCheckOperand(c wir.Check) string {
	if c.TypeName != "" {
		return strconv.Quote(c.TypeName)
	}
	return "== " + c.OtherPath.String()
}

func literalCheckOperand(c wir.Check) string {
	if c.Literal != nil {
		return c.Literal.String()
	}
	if c.LiteralString != "" {
		return strconv.Quote(c.LiteralString)
	}
	return "?"
}

func operatorMnemonic(op wir.Operator) string {
	switch op {
	case wir.BinAdd:
		return "add"
	case wir.BinSub:
		return "sub"
	case wir.BinMul:
		return "mul"
	case wir.BinDiv:
		return "div"
	case wir.BinIDiv:
		return "idiv"
	case wir.BinMod:
		return "mod"
	case wir.BinPow:
		return "pow"
	case wir.BinBAnd:
		return "band"
	case wir.BinBOr:
		return "bor"
	case wir.BinBXor:
		return "bxor"
	case wir.BinShl:
		return "shl"
	case wir.BinShr:
		return "shr"
	case wir.BinEq:
		return "eq"
	case wir.BinNe:
		return "ne"
	case wir.BinLt:
		return "lt"
	case wir.BinLe:
		return "le"
	case wir.BinGt:
		return "gt"
	case wir.BinGe:
		return "ge"
	case wir.UnNeg:
		return "neg"
	case wir.UnNot:
		return "not"
	case wir.UnLen:
		return "len"
	case wir.UnBNot:
		return "bnot"
	case wir.LogAnd:
		return "and"
	case wir.LogOr:
		return "or"
	default:
		return fmt.Sprintf("op%d", op)
	}
}

func claimMnemonic(k wir.ClaimKind) string {
	switch k {
	case wir.ClaimCast:
		return "cast"
	case wir.ClaimAssert:
		return "assert"
	case wir.ClaimAnnotation:
		return "annotation"
	case wir.ClaimAssertsPredicate:
		return "asserts"
	default:
		return "none"
	}
}
