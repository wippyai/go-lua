package engine

import "github.com/wippyai/go-lua/analysis/identity"

func installConstOperandResolver[K ~uint32 | ~uint64, V, O any](implementation *RuleImplementation[K, V, O], operand O) bool {
	if implementation == nil {
		return false
	}
	return implementation.InstallOperandResolver(func(OperandCoords) (O, bool) {
		return operand, true
	})
}

func installMemberOperandResolver[K ~uint32 | ~uint64, V, O any](implementation *RuleImplementation[K, V, O], operands map[identity.ContentID]O) bool {
	if implementation == nil || operands == nil {
		return false
	}
	return implementation.InstallOperandResolver(func(coords OperandCoords) (O, bool) {
		operand, ok := operands[coords.Member]
		return operand, ok
	})
}
