package typewitness

import "github.com/wippyai/go-lua/analysis/domain/value/axis"

var Key = axis.NewKey[Value]("typewitness")

func Spec() axis.Spec[Value] {
	return axis.Spec[Value]{
		Key:      Key,
		Bottom:   Bottom,
		Top:      Top,
		Equal:    Equal,
		LessOrEq: func(a, b Value) bool { return Equal(Join(a, b), b) },
		Join:     Join,
		Meet:     Meet,
		Widen:    Join,
		Hash:     Value.Hash,
	}
}
