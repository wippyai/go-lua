// Package reachability owns the minimal may-reachability judgment for
// canonical Program occurrences and outcomes.
package reachability

import "github.com/wippyai/go-lua/analysis/lattice"

// Value is the two-point may-reachability lattice. Reachable means at least
// one concrete execution can reach the exact Program coordinate.
type Value uint8

const (
	Unreachable Value = iota
	Reachable
)

// Lattice is the complete finite-height reachability domain.
func Lattice() lattice.Lattice[Value] {
	return lattice.Lattice[Value]{
		Bottom:   func() Value { return Unreachable },
		Top:      func() Value { return Reachable },
		Equal:    func(left, right Value) bool { return left == right },
		LessOrEq: func(left, right Value) bool { return left <= right },
		Join:     join,
		Meet:     meet,
		Widen:    join,
	}
}

func join(left, right Value) Value {
	if left > right {
		return left
	}
	return right
}

func meet(left, right Value) Value {
	if left < right {
		return left
	}
	return right
}
