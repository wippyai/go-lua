package transformer

import "fmt"

func collectRelationGuardAtoms(arena *Arena, guard Guard, atoms map[ValueTerm]struct{}, state map[Guard]uint8) error {
	if arena == nil || atoms == nil || state == nil || guard == 0 || int(guard) >= len(arena.guards) {
		return fmt.Errorf("transformer: relation guard is outside its term owner")
	}
	switch state[guard] {
	case 1:
		return fmt.Errorf("transformer: relation guard contains a cycle")
	case 2:
		return nil
	}
	state[guard] = 1
	node := arena.guards[guard]
	switch node.op {
	case guardTrue, guardFalse:
	case guardTruthy, guardFalsy:
		if node.value == 0 || int(node.value) >= len(arena.values) {
			return fmt.Errorf("transformer: relation guard atom is outside its term owner")
		}
		atoms[node.value] = struct{}{}
	case guardAnd, guardOr:
		for _, child := range node.args {
			if err := collectRelationGuardAtoms(arena, child, atoms, state); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("transformer: relation guard has invalid syntax")
	}
	state[guard] = 2
	return nil
}
