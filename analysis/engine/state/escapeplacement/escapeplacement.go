package escapeplacement

import (
	"github.com/wippyai/go-lua/analysis/domain/lattice"
	"github.com/wippyai/go-lua/analysis/domain/lattice/lift"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/placement"
)

type Value = placement.Value

const (
	Bottom    = placement.Bottom
	Stack     = placement.Stack
	OwnedHeap = placement.OwnedHeap
	Escaped   = placement.SharedHeap
	Unknown   = placement.Unknown
)

func Domain() lattice.Lattice[Value] {
	return lattice.Lattice[Value]{
		Bottom: func() Value { return placement.Bottom },
		Top:    func() Value { return placement.Unknown },
		Equal:  placement.Equal,
		LessOrEq: func(a, b Value) bool {
			return placement.LessOrEq(a, b)
		},
		Join:  placement.Join,
		Widen: placement.Widen,
	}
}

func MapDomain() lattice.Lattice[map[identity.ID]Value] {
	return lift.Map[identity.ID, Value](Domain())
}

func CloneMap(in map[identity.ID]Value) map[identity.ID]Value {
	if len(in) == 0 {
		return nil
	}
	out := make(map[identity.ID]Value, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func DeleteEntry(in map[identity.ID]Value, id identity.ID) (map[identity.ID]Value, bool) {
	if _, ok := in[id]; !ok {
		return in, false
	}
	out := make(map[identity.ID]Value, len(in)-1)
	for k, v := range in {
		if k != id {
			out[k] = v
		}
	}
	if len(out) == 0 {
		return nil, true
	}
	return out, true
}
