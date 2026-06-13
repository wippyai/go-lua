package typeoperator

import (
	"github.com/wippyai/go-lua/analysis/lua/typecall"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func binaryMetamethodReturn(first typ.Type, name string, second typ.Type) (typ.Type, bool) {
	if result, found, ok := metamethodReturn(first, name); found {
		return result, ok
	}
	if result, found, ok := metamethodReturn(second, name); found {
		return result, ok
	}
	return nil, false
}

func unaryMetamethodReturn(operand typ.Type, name string) (typ.Type, bool, bool) {
	return metamethodReturn(operand, name)
}

func metamethodReturn(t typ.Type, name string) (typ.Type, bool, bool) {
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
