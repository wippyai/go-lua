package transferfacts

import (
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/lua/typeoperator"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/type/subtype"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/parse/numparse"
)

func numericForLoopVariableTypeFromWIR(
	body *wir.Body,
	symbolTypes map[symbol.ID]typ.Type,
	tempDefs map[uint32]wir.Instruction,
	inst wir.Instruction,
) typ.Type {
	if body == nil || inst.Iter != wir.IterNumeric {
		return nil
	}
	bounds := body.Operands(inst.List)
	if len(bounds) == 0 {
		return typ.Number
	}
	for _, bound := range bounds {
		t, ok := numericForBoundTypeFromWIR(body, symbolTypes, tempDefs, bound, nil)
		if !ok || t == nil || typ.IsAny(t) || typ.IsUnknown(t) || !subtype.IsSubtype(t, typ.Integer) {
			return typ.Number
		}
	}
	return typ.Integer
}

func numericForBoundTypeFromWIR(
	body *wir.Body,
	symbolTypes map[symbol.ID]typ.Type,
	tempDefs map[uint32]wir.Instruction,
	op wir.Operand,
	seen map[uint32]bool,
) (typ.Type, bool) {
	if body == nil {
		return nil, false
	}
	if t, ok := wirConstructorValueTypeFromSymbols(symbolTypes, body, tempDefs, op, seen); ok {
		return t, true
	}
	switch op.Kind {
	case wir.OperandConst:
		c := body.Const(wir.ConstRef(op.Ref))
		if c.Kind != wir.ConstNumber {
			return nil, false
		}
		if _, ok := numparse.ParseIntegralLiteral(c.Number); ok {
			return typ.Integer, true
		}
		return typ.Number, true
	case wir.OperandTemp:
		if seen == nil {
			seen = make(map[uint32]bool)
		}
		if seen[op.Ref] {
			return nil, false
		}
		def, ok := tempDefs[op.Ref]
		if !ok {
			return nil, false
		}
		seen[op.Ref] = true
		defer delete(seen, op.Ref)
		if def.Type != 0 {
			if t := body.Type(def.Type); t != nil {
				return t, true
			}
		}
		switch def.Op {
		case wir.OpAssign, wir.OpClaim:
			return numericForBoundTypeFromWIR(body, symbolTypes, tempDefs, def.A, seen)
		case wir.OpUnOp:
			opText, ok := wirExpressionOperator(def)
			if !ok {
				return nil, false
			}
			operand, ok := numericForBoundTypeFromWIR(body, symbolTypes, tempDefs, def.A, seen)
			if !ok {
				if def.Operator == wir.UnLen {
					return typ.Integer, true
				}
				return nil, false
			}
			return typeoperator.UnaryOp(opText, operand)
		case wir.OpBinOp, wir.OpConcat, wir.OpLogical:
			opText, ok := wirExpressionOperator(def)
			if !ok {
				return nil, false
			}
			leftOp, rightOp, ok := wirBinaryExpressionOperands(body, def)
			if !ok {
				return nil, false
			}
			left, ok := numericForBoundTypeFromWIR(body, symbolTypes, tempDefs, leftOp, seen)
			if !ok {
				return nil, false
			}
			right, ok := numericForBoundTypeFromWIR(body, symbolTypes, tempDefs, rightOp, seen)
			if !ok {
				return nil, false
			}
			return typeoperator.BinaryOp(left, opText, right)
		}
	}
	return nil, false
}

func numericForIntegralLiteralFromWIR(body *wir.Body, tempDefs map[uint32]wir.Instruction, op wir.Operand) (int64, bool) {
	if body == nil {
		return 0, false
	}
	switch op.Kind {
	case wir.OperandConst:
		c := body.Const(wir.ConstRef(op.Ref))
		if c.Kind != wir.ConstNumber {
			return 0, false
		}
		return numparse.ParseIntegralLiteral(c.Number)
	case wir.OperandTemp:
		inst, ok := tempDefs[op.Ref]
		if !ok || inst.Op != wir.OpUnOp || inst.Operator != wir.UnNeg {
			return 0, false
		}
		value, ok := numericForIntegralLiteralFromWIR(body, tempDefs, inst.A)
		if !ok {
			return 0, false
		}
		return -value, true
	default:
		return 0, false
	}
}
