package evidence

import "github.com/wippyai/go-lua/analysis/domain/value/axis"

var Key = axis.NewKey[Value]("evidence")

func Spec() axis.Spec[Value] {
	return axis.Spec[Value]{
		Key:      Key,
		Bottom:   Bottom,
		Top:      Top,
		Equal:    Equal,
		LessOrEq: func(a, b Value) bool { return b.Covers(a) },
		Join:     Join,
		Widen:    Widen,
		Hash:     Value.Hash,
	}
}
