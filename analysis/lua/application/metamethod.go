package application

import (
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typecall"
)

func binaryMetamethodReturn(first typ.Type, slot MetaSlot, second typ.Type) (typ.Type, bool) {
	if result, found, ok := metamethodReturn(first, slot); found {
		return result, ok
	}
	if result, found, ok := metamethodReturn(second, slot); found {
		return result, ok
	}
	return nil, false
}

func unaryMetamethodReturn(operand typ.Type, slot MetaSlot) (typ.Type, bool, bool) {
	return metamethodReturn(operand, slot)
}

func metamethodReturn(t typ.Type, slot MetaSlot) (typ.Type, bool, bool) {
	name, ok := metamethodName(slot)
	if !ok {
		return nil, false, false
	}
	mt, found := typecall.GetMetamethod(t, name)
	if !found {
		return nil, false, false
	}
	result, ok := typecall.CallableReturn(mt)
	if !ok {
		return nil, true, false
	}
	return result, true, true
}

// metamethodName is the single boundary from the closed semantic MetaSlot
// vocabulary to the table spelling understood by the retained type model.
func metamethodName(slot MetaSlot) (string, bool) {
	switch slot {
	case MetaUnm:
		return "__unm", true
	case MetaBNot:
		return "__bnot", true
	case MetaLen:
		return "__len", true
	case MetaAdd:
		return "__add", true
	case MetaSub:
		return "__sub", true
	case MetaMul:
		return "__mul", true
	case MetaDiv:
		return "__div", true
	case MetaIDiv:
		return "__idiv", true
	case MetaMod:
		return "__mod", true
	case MetaPow:
		return "__pow", true
	case MetaConcat:
		return "__concat", true
	case MetaBand:
		return "__band", true
	case MetaBor:
		return "__bor", true
	case MetaBxor:
		return "__bxor", true
	case MetaShl:
		return "__shl", true
	case MetaShr:
		return "__shr", true
	case MetaEq:
		return "__eq", true
	case MetaLt:
		return "__lt", true
	case MetaLe:
		return "__le", true
	case MetaIndex:
		return "__index", true
	case MetaNewIndex:
		return "__newindex", true
	case MetaCall:
		return "__call", true
	default:
		return "", false
	}
}
