package inspect

import "github.com/wippyai/go-lua/analysis/type/typ"

// BoolJoin describes one monotone Boolean equation over referenced type nodes.
type BoolJoin uint8

const (
	BoolConstant BoolJoin = iota
	BoolAll
	BoolAny
)

// BoolEquation is one node in a finite monotone Boolean equation system.
// Constant is used only by BoolConstant. Callers must spell empty conjunctions
// and disjunctions explicitly as constants so their semantic intent is clear.
type BoolEquation struct {
	Join     BoolJoin
	Constant bool
	Inputs   []typ.Type
}

// Constant returns a Boolean equation with no dependencies.
func Constant(value bool) BoolEquation {
	return BoolEquation{Join: BoolConstant, Constant: value}
}

// All returns the conjunction of inputs.
func All(inputs ...typ.Type) BoolEquation {
	return BoolEquation{Join: BoolAll, Inputs: inputs}
}

// Any returns the disjunction of inputs.
func Any(inputs ...typ.Type) BoolEquation {
	return BoolEquation{Join: BoolAny, Inputs: inputs}
}

// LeastBoolFixedPoint returns the least solution of the finite equation graph
// reachable from root.
func LeastBoolFixedPoint(root typ.Type, equation func(typ.Type) BoolEquation) bool {
	return solveBoolFixedPoint(root, equation, false)
}

// GreatestBoolFixedPoint returns the greatest solution of the finite equation
// graph reachable from root.
func GreatestBoolFixedPoint(root typ.Type, equation func(typ.Type) BoolEquation) bool {
	return solveBoolFixedPoint(root, equation, true)
}

func solveBoolFixedPoint(root typ.Type, equation func(typ.Type) BoolEquation, greatest bool) bool {
	if root == nil || equation == nil {
		return false
	}
	types := []typ.Type{root}
	index := map[typ.Type]int{root: 0}
	equations := make([]BoolEquation, 0, 1)
	for read := 0; read < len(types); read++ {
		eq := equation(types[read])
		for _, input := range eq.Inputs {
			if input == nil {
				continue
			}
			if _, exists := index[input]; !exists {
				index[input] = len(types)
				types = append(types, input)
			}
		}
		equations = append(equations, eq)
	}

	values := make([]bool, len(types))
	if greatest {
		for i := range values {
			values[i] = true
		}
	}
	dependents := make([][]int, len(types))
	for owner, eq := range equations {
		for _, input := range eq.Inputs {
			if inputIndex, exists := index[input]; exists {
				dependents[inputIndex] = append(dependents[inputIndex], owner)
			}
		}
	}
	queue := make([]int, len(types))
	queued := make([]bool, len(types))
	for i := range queue {
		queue[i], queued[i] = i, true
	}
	for read := 0; read < len(queue); read++ {
		i := queue[read]
		queued[i] = false
		value := evalBoolEquation(equations[i], index, values)
		if values[i] == value {
			continue
		}
		values[i] = value
		for _, dependent := range dependents[i] {
			if !queued[dependent] {
				queued[dependent] = true
				queue = append(queue, dependent)
			}
		}
	}
	return values[0]
}

func evalBoolEquation(eq BoolEquation, index map[typ.Type]int, values []bool) bool {
	switch eq.Join {
	case BoolAll:
		for _, input := range eq.Inputs {
			inputIndex, exists := index[input]
			if !exists || !values[inputIndex] {
				return false
			}
		}
		return true
	case BoolAny:
		for _, input := range eq.Inputs {
			if inputIndex, exists := index[input]; exists && values[inputIndex] {
				return true
			}
		}
		return false
	default:
		return eq.Constant
	}
}
