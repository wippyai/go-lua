package runtimekind

import "github.com/wippyai/go-lua/analysis/domain/value/axis"

var Key = axis.NewKey[Value]("runtimekind")

func Spec() axis.Spec[Value] {
	return axis.Spec[Value]{
		Key:      Key,
		Bottom:   Bottom,
		Top:      Top,
		Equal:    Equal,
		LessOrEq: LessOrEq,
		Join:     Join,
		Meet:     Meet,
		Widen:    Widen,
		Hash:     Hash,
	}
}
