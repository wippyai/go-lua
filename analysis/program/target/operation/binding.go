package operation

import (
	"errors"
	"sort"

	"github.com/wippyai/go-lua/analysis/program/internal/rows"
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

func (core Core) binding(op vocabulary.Operation, index int) (bindingRow, bool) {
	row, ok := core.operation(op)
	if !ok {
		return bindingRow{}, false
	}
	return core.geometry.bindings.At(row.bindings, index)
}

func (core Core) bindingKey(op vocabulary.Operation, index int) (bindingKeyRow, bool) {
	if op == 0 {
		return bindingKeyRow{}, false
	}
	rangeRow, ok := core.bindingRanges.At(int(op) - 1)
	if !ok {
		return bindingKeyRow{}, false
	}
	return core.bindingKeyRows.At(rangeRow.bindings, index)
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

// BindingOwnerKeyAt returns the exact-key handle projected for one owner
// segment by the immutable operation anchor value.
func (core Core) BindingOwnerKeyAt(op vocabulary.Operation, binding, index int) (vocabulary.ExactKey, bool) {
	row, ok := core.bindingKey(op, binding)
	if !ok {
		return 0, false
	}
	return core.bindingKeys.At(row.owner, index)
}

// BindingMemberKeyAt returns the exact-key handle projected for one member
// segment by the immutable operation anchor value.
func (core Core) BindingMemberKeyAt(op vocabulary.Operation, binding, index int) (vocabulary.ExactKey, bool) {
	row, ok := core.bindingKey(op, binding)
	if !ok {
		return 0, false
	}
	return core.bindingKeys.At(row.member, index)
}

// compareBindingSpec compares one owner-issued binding row with a neutral
// spec without materializing Owner or Member slices. It is used while sealing
// and querying the operation-owned lookup index.
func (core Core) compareBindingSpec(op vocabulary.Operation, index int, right vocabulary.BindingSpec) (int, bool) {
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

// compareBindingRows compares two owner-issued rows directly from Core's
// sealed segment pools. The fallback bool reports malformed coordinates.
func (core Core) compareBindingRows(leftOp vocabulary.Operation, leftIndex int, rightOp vocabulary.Operation, rightIndex int) (int, bool) {
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

func (core Core) compareBindingIndex(left, right bindingIndexRow) (int, bool) {
	order, ok := core.compareBindingRows(left.operation, int(left.binding), right.operation, int(right.binding))
	if !ok || order != 0 {
		return order, ok
	}
	if left.operation < right.operation {
		return -1, true
	}
	if left.operation > right.operation {
		return 1, true
	}
	if left.binding < right.binding {
		return -1, true
	}
	if left.binding > right.binding {
		return 1, true
	}
	return 0, true
}

func compileBindingLookup(geometry Geometry) (rows.Rows[bindingIndexRow], error) {
	core := Core{geometry: geometry}
	lookup := make([]bindingIndexRow, 0, geometry.bindings.Len())
	for operationIndex := 0; operationIndex+1 < geometry.operations.Count(); operationIndex++ {
		operation := vocabulary.Operation(operationIndex + 1)
		row, ok := geometry.operations.At(operationIndex)
		if !ok {
			return rows.Rows[bindingIndexRow]{}, errors.New("target/operation: malformed binding geometry")
		}
		count := geometry.bindings.Count(row.bindings)
		if count != row.bindings.Len() {
			return rows.Rows[bindingIndexRow]{}, errors.New("target/operation: malformed binding geometry")
		}
		for index := 0; index < count; index++ {
			lookup = append(lookup, bindingIndexRow{binding: uint32(index), operation: operation})
		}
	}
	sort.Slice(lookup, func(left, right int) bool {
		order, ok := core.compareBindingIndex(lookup[left], lookup[right])
		return ok && order < 0
	})
	for index := 1; index < len(lookup); index++ {
		order, ok := core.compareBindingIndex(lookup[index-1], lookup[index])
		if !ok {
			return rows.Rows[bindingIndexRow]{}, errors.New("target/operation: malformed binding geometry")
		}
		if order == 0 {
			return rows.Rows[bindingIndexRow]{}, errors.New("target/operation: duplicate sealed binding")
		}
	}
	return rows.NewRows(lookup), nil
}

// Lookup finds an exact binding in the operation owner's sealed index without
// joining, hashing, parser fallback, or allocation.
func (core Core) Lookup(binding vocabulary.BindingSpec) (vocabulary.Operation, bool) {
	if !vocabulary.ValidBinding(binding) {
		return 0, false
	}
	left, right := 0, core.geometry.lookup.Count()
	for left < right {
		middle := left + (right-left)/2
		row, ok := core.geometry.lookup.At(middle)
		if !ok {
			return 0, false
		}
		order, ok := core.compareBindingSpec(row.operation, int(row.binding), binding)
		if !ok {
			return 0, false
		}
		if order < 0 {
			left = middle + 1
		} else {
			right = middle
		}
	}
	if left >= core.geometry.lookup.Count() {
		return 0, false
	}
	row, ok := core.geometry.lookup.At(left)
	if !ok {
		return 0, false
	}
	order, ok := core.compareBindingSpec(row.operation, int(row.binding), binding)
	if !ok || order != 0 {
		return 0, false
	}
	return row.operation, true
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
