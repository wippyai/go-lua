package operation

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
)

// BindingCount and BindingAt expose copied neutral binding values. The final
// operation owner retains only rows and segment pools.
func (core Core) BindingCount(op vocabulary.Operation) int {
	row, ok := core.operation(op)
	if !ok {
		return 0
	}
	return core.geometry.bindings.Count(row.bindings)
}

// BindingTotal returns the number of binding rows owned by Core across the
// canonical operation catalogue. It is a cold aggregate count; callers that
// need one operation use BindingCount for an O(1) span query.
func (core Core) BindingTotal() int {
	total := 0
	for operation := 1; operation <= core.OperationCount(); operation++ {
		total += core.BindingCount(vocabulary.Operation(operation))
	}
	return total
}

func (core Core) binding(op vocabulary.Operation, index int) (bindingRow, bool) {
	row, ok := core.operation(op)
	if !ok {
		return bindingRow{}, false
	}
	return core.geometry.bindings.At(row.bindings, index)
}

func (core Core) BindingNamespaceAt(op vocabulary.Operation, index int) (vocabulary.BindingNamespace, bool) {
	binding, ok := core.binding(op, index)
	if !ok {
		return 0, false
	}
	return binding.namespace, true
}

func (core Core) BindingOwnerCountAt(op vocabulary.Operation, index int) int {
	binding, ok := core.binding(op, index)
	if !ok {
		return 0
	}
	return core.geometry.segments.Count(binding.owner)
}

func (core Core) BindingOwnerAt(op vocabulary.Operation, binding, index int) (string, bool) {
	row, ok := core.binding(op, binding)
	if !ok {
		return "", false
	}
	return core.geometry.segments.At(row.owner, index)
}

func (core Core) BindingMemberCountAt(op vocabulary.Operation, index int) int {
	binding, ok := core.binding(op, index)
	if !ok {
		return 0
	}
	return core.geometry.segments.Count(binding.member)
}

func (core Core) BindingMemberAt(op vocabulary.Operation, binding, index int) (string, bool) {
	row, ok := core.binding(op, binding)
	if !ok {
		return "", false
	}
	return core.geometry.segments.At(row.member, index)
}

// CompareBinding compares one owner-issued binding row with a neutral spec
// without materializing Owner or Member slices. It is used by Target's sealed
// lookup index and therefore must remain allocation-free.
func (core Core) CompareBinding(op vocabulary.Operation, index int, right vocabulary.BindingSpec) (int, bool) {
	leftNamespace, ok := core.BindingNamespaceAt(op, index)
	if !ok {
		return 0, false
	}
	if leftNamespace < right.Namespace {
		return -1, true
	}
	if leftNamespace > right.Namespace {
		return 1, true
	}
	if order, ok := core.compareBindingSpecSegments(op, index, true, right.Owner); !ok || order != 0 {
		return order, ok
	}
	return core.compareBindingSpecSegments(op, index, false, right.Member)
}

// CompareBindings compares two owner-issued rows directly from Core's sealed
// segment pools. The fallback bool reports malformed coordinates.
func (core Core) CompareBindings(leftOp vocabulary.Operation, leftIndex int, rightOp vocabulary.Operation, rightIndex int) (int, bool) {
	leftNamespace, leftOK := core.BindingNamespaceAt(leftOp, leftIndex)
	rightNamespace, rightOK := core.BindingNamespaceAt(rightOp, rightIndex)
	if !leftOK || !rightOK {
		return 0, false
	}
	if leftNamespace < rightNamespace {
		return -1, true
	}
	if leftNamespace > rightNamespace {
		return 1, true
	}
	if order, ok := core.compareBindingRowSegments(leftOp, leftIndex, rightOp, rightIndex, true); !ok || order != 0 {
		return order, ok
	}
	return core.compareBindingRowSegments(leftOp, leftIndex, rightOp, rightIndex, false)
}

func (core Core) compareBindingSpecSegments(op vocabulary.Operation, index int, owner bool, right []string) (int, bool) {
	count := core.BindingOwnerCountAt(op, index)
	if !owner {
		count = core.BindingMemberCountAt(op, index)
	}
	limit := count
	if len(right) < limit {
		limit = len(right)
	}
	for segment := 0; segment < limit; segment++ {
		var left string
		var ok bool
		if owner {
			left, ok = core.BindingOwnerAt(op, index, segment)
		} else {
			left, ok = core.BindingMemberAt(op, index, segment)
		}
		if !ok {
			return 0, false
		}
		if left < right[segment] {
			return -1, true
		}
		if left > right[segment] {
			return 1, true
		}
	}
	if count < len(right) {
		return -1, true
	}
	if count > len(right) {
		return 1, true
	}
	return 0, true
}

func (core Core) compareBindingRowSegments(leftOp vocabulary.Operation, leftIndex int, rightOp vocabulary.Operation, rightIndex int, owner bool) (int, bool) {
	leftCount := core.BindingOwnerCountAt(leftOp, leftIndex)
	rightCount := core.BindingOwnerCountAt(rightOp, rightIndex)
	if !owner {
		leftCount = core.BindingMemberCountAt(leftOp, leftIndex)
		rightCount = core.BindingMemberCountAt(rightOp, rightIndex)
	}
	limit := leftCount
	if rightCount < limit {
		limit = rightCount
	}
	for segment := 0; segment < limit; segment++ {
		var left, right string
		var leftOK, rightOK bool
		if owner {
			left, leftOK = core.BindingOwnerAt(leftOp, leftIndex, segment)
			right, rightOK = core.BindingOwnerAt(rightOp, rightIndex, segment)
		} else {
			left, leftOK = core.BindingMemberAt(leftOp, leftIndex, segment)
			right, rightOK = core.BindingMemberAt(rightOp, rightIndex, segment)
		}
		if !leftOK || !rightOK {
			return 0, false
		}
		if left < right {
			return -1, true
		}
		if left > right {
			return 1, true
		}
	}
	if leftCount < rightCount {
		return -1, true
	}
	if leftCount > rightCount {
		return 1, true
	}
	return 0, true
}

func (core Core) BindingAt(op vocabulary.Operation, index int) (vocabulary.BindingSpec, bool) {
	binding, ok := core.binding(op, index)
	if !ok {
		return vocabulary.BindingSpec{}, false
	}
	owner := make([]string, core.geometry.segments.Count(binding.owner))
	for index := range owner {
		value, valueOK := core.geometry.segments.At(binding.owner, index)
		if !valueOK {
			return vocabulary.BindingSpec{}, false
		}
		owner[index] = value
	}
	member := make([]string, core.geometry.segments.Count(binding.member))
	for index := range member {
		value, valueOK := core.geometry.segments.At(binding.member, index)
		if !valueOK {
			return vocabulary.BindingSpec{}, false
		}
		member[index] = value
	}
	return vocabulary.BindingSpec{Namespace: binding.namespace, Owner: owner, Member: member}, true
}

func (core Core) bindingSpecs(op vocabulary.Operation) ([]vocabulary.BindingSpec, error) {
	count := core.BindingCount(op)
	bindings := make([]vocabulary.BindingSpec, count)
	for index := range bindings {
		binding, ok := core.BindingAt(op, index)
		if !ok {
			return nil, errors.New("target/operation: malformed binding projection")
		}
		bindings[index] = binding
	}
	return bindings, nil
}
