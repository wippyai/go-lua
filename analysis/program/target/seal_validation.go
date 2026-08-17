package target

import (
	"bytes"

	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
)

func (c *Contract) bindingEqual(row bindingRange, input BindingSpec) bool {
	if row.namespace != input.Namespace || row.owner.len() != len(input.Owner) || row.member.len() != len(input.Member) {
		return false
	}
	for index := range input.Owner {
		if c.segments[row.owner.start+uint32(index)] != input.Owner[index] {
			return false
		}
	}
	for index := range input.Member {
		if c.segments[row.member.start+uint32(index)] != input.Member[index] {
			return false
		}
	}
	return true
}

func (c *Contract) compareBindingRows(left, right uint32) int {
	return compareBindingRanges(c.bindings[left], c.bindings[right], c.segments)
}

func compareBindingRanges(left, right bindingRange, segments []string) int {
	if left.namespace < right.namespace {
		return -1
	}
	if left.namespace > right.namespace {
		return 1
	}
	if order := compareSegments(segments[left.owner.start:left.owner.end], segments[right.owner.start:right.owner.end]); order != 0 {
		return order
	}
	return compareSegments(segments[left.member.start:left.member.end], segments[right.member.start:right.member.end])
}

func compareBindingRangeSpec(left bindingRange, right BindingSpec, segments []string) int {
	if left.namespace < right.Namespace {
		return -1
	}
	if left.namespace > right.Namespace {
		return 1
	}
	if order := compareSegments(segments[left.owner.start:left.owner.end], right.Owner); order != 0 {
		return order
	}
	return compareSegments(segments[left.member.start:left.member.end], right.Member)
}

func validOperationOutcome(kind flowkind.OutcomeKind) bool {
	switch kind {
	case flowkind.OutcomeNormal, flowkind.OutcomeThrow, flowkind.OutcomeYield, flowkind.OutcomeCancel:
		return true
	default:
		return false
	}
}

func validValuesTail(tail ValuesTail, variable ValuesVar, count uint32, opaque bool) bool {
	switch tail {
	case ValuesClosed:
		return variable == 0
	case ValuesVariable:
		return uint64(variable) < uint64(count)
	case ValuesUnknown:
		return opaque && variable == 0
	default:
		return false
	}
}

func validBinding(binding BindingSpec) bool {
	if len(binding.Member) == 0 || !validSegments(binding.Member) || !bindingLengthsFit(binding) {
		return false
	}
	switch binding.Namespace {
	case BindingBuiltin:
		return len(binding.Owner) == 0
	case BindingModule, BindingProvider:
		return len(binding.Owner) != 0 && validSegments(binding.Owner)
	default:
		return false
	}
}

func validSegments(parts []string) bool {
	for _, part := range parts {
		if part == "" {
			return false
		}
		if _, err := checkedStoredLength("binding segment bytes", len(part)); err != nil {
			return false
		}
	}
	return true
}

func bindingLengthsFit(binding BindingSpec) bool {
	if _, err := checkedStoredLength("binding owner length", len(binding.Owner)); err != nil {
		return false
	}
	if _, err := checkedStoredLength("binding member length", len(binding.Member)); err != nil {
		return false
	}
	return true
}

func cloneBinding(input BindingSpec) BindingSpec {
	return BindingSpec{
		Namespace: input.Namespace,
		Owner:     append([]string(nil), input.Owner...), Member: append([]string(nil), input.Member...),
	}
}

func compareBinding(left, right BindingSpec) int {
	if left.Namespace != right.Namespace {
		if left.Namespace < right.Namespace {
			return -1
		}
		return 1
	}
	if order := compareSegments(left.Owner, right.Owner); order != 0 {
		return order
	}
	if order := compareSegments(left.Member, right.Member); order != 0 {
		return order
	}
	return 0
}

func compareSegments(left, right []string) int {
	limit := len(left)
	if len(right) < limit {
		limit = len(right)
	}
	for index := 0; index < limit; index++ {
		if left[index] < right[index] {
			return -1
		}
		if left[index] > right[index] {
			return 1
		}
	}
	if len(left) < len(right) {
		return -1
	}
	if len(left) > len(right) {
		return 1
	}
	return 0
}

func compareOutcome(left, right outcomeDraft) int {
	if left.kind < right.kind {
		return -1
	}
	if left.kind > right.kind {
		return 1
	}
	if compared := compareValues(left.values, right.values); compared != 0 {
		return compared
	}
	return compareFreshResults(left.fresh, right.fresh)
}

func compareFreshResults(left, right []freshResultDraft) int {
	limit := len(left)
	if len(right) < limit {
		limit = len(right)
	}
	for index := 0; index < limit; index++ {
		if left[index].result < right[index].result {
			return -1
		}
		if left[index].result > right[index].result {
			return 1
		}
		if left[index].kind < right[index].kind {
			return -1
		}
		if left[index].kind > right[index].kind {
			return 1
		}
	}
	if len(left) < len(right) {
		return -1
	}
	if len(left) > len(right) {
		return 1
	}
	return 0
}

func compareValues(left, right valuesDraft) int {
	limit := len(left.types)
	if len(right.types) < limit {
		limit = len(right.types)
	}
	for index := 0; index < limit; index++ {
		if order := bytes.Compare([]byte(left.types[index]), []byte(right.types[index])); order != 0 {
			return order
		}
	}
	if len(left.types) < len(right.types) {
		return -1
	}
	if len(left.types) > len(right.types) {
		return 1
	}
	if left.tail < right.tail {
		return -1
	}
	if left.tail > right.tail {
		return 1
	}
	if left.varID < right.varID {
		return -1
	}
	if left.varID > right.varID {
		return 1
	}
	if order := bytes.Compare([]byte(left.tailType), []byte(right.tailType)); order != 0 {
		return order
	}
	limit = len(left.suffix)
	if len(right.suffix) < limit {
		limit = len(right.suffix)
	}
	for index := 0; index < limit; index++ {
		if order := bytes.Compare([]byte(left.suffix[index]), []byte(right.suffix[index])); order != 0 {
			return order
		}
	}
	if len(left.suffix) < len(right.suffix) {
		return -1
	}
	if len(left.suffix) > len(right.suffix) {
		return 1
	}
	return 0
}

func (values valuesDraft) key() (string, error) {
	// Framed components prevent any concatenation ambiguity; this is cold seal
	// bookkeeping only and never a public binding or effect identity.
	parts := make([]int, 0)
	for _, typ := range values.types {
		if _, err := checkedStoredLength("Values key type bytes", len(typ)); err != nil {
			return "", err
		}
		parts = append(parts, 4, len(typ))
	}
	for _, typ := range values.suffix {
		if _, err := checkedStoredLength("Values key suffix type bytes", len(typ)); err != nil {
			return "", err
		}
		parts = append(parts, 4, len(typ))
	}
	if _, err := checkedStoredLength("Values key tail type bytes", len(values.tailType)); err != nil {
		return "", err
	}
	parts = append(parts, 4, 1, 4, 4, len(values.tailType))
	total, err := checkedStoredTotal("Values key", parts...)
	if err != nil {
		return "", err
	}
	out := make([]byte, 0, total)
	for _, typ := range values.types {
		length, lengthErr := checkedStoredLength("Values key type bytes", len(typ))
		if lengthErr != nil {
			return "", lengthErr
		}
		out = appendUint32(out, length)
		out = append(out, typ...)
	}
	out = appendUint32(out, ^uint32(0))
	out = append(out, byte(values.tail))
	out = appendUint32(out, uint32(values.varID))
	out = appendUint32(out, uint32(len(values.tailType)))
	out = append(out, values.tailType...)
	for _, typ := range values.suffix {
		length, lengthErr := checkedStoredLength("Values key suffix type bytes", len(typ))
		if lengthErr != nil {
			return "", lengthErr
		}
		out = appendUint32(out, length)
		out = append(out, typ...)
	}
	return string(out), nil
}

func compareEffect(left, right effectDraft) int {
	if left.target < right.target {
		return -1
	}
	if left.target > right.target {
		return 1
	}
	if order := compareUint32Slice(left.values, right.values); order != 0 {
		return order
	}
	if order := compareUint32Slice(left.types, right.types); order != 0 {
		return order
	}
	if order := compareUint32Slice(left.valuesVar, right.valuesVar); order != 0 {
		return order
	}
	if order := compareUint32Slice(left.rows, right.rows); order != 0 {
		return order
	}
	if left.hasPublication != right.hasPublication {
		if !left.hasPublication {
			return -1
		}
		return 1
	}
	if !left.hasPublication {
		return 0
	}
	return comparePublicationEffectDescriptor(left.publication, right.publication)
}

func comparePublicationEffectDescriptor(left, right PublicationEffectDescriptor) int {
	if left.kind != right.kind {
		if left.kind < right.kind {
			return -1
		}
		return 1
	}
	if left.subject != right.subject {
		if left.subject < right.subject {
			return -1
		}
		return 1
	}
	if left.destination != right.destination {
		if left.destination < right.destination {
			return -1
		}
		return 1
	}
	if left.context != right.context {
		if left.context < right.context {
			return -1
		}
		return 1
	}
	if left.escape != right.escape {
		if left.escape < right.escape {
			return -1
		}
		return 1
	}
	if left.mutability != right.mutability {
		if left.mutability < right.mutability {
			return -1
		}
		return 1
	}
	if left.lifetime < right.lifetime {
		return -1
	}
	if left.lifetime > right.lifetime {
		return 1
	}
	return 0
}

func compareUint32Slice[T ~uint32](left, right []T) int {
	limit := len(left)
	if len(right) < limit {
		limit = len(right)
	}
	for index := 0; index < limit; index++ {
		if left[index] < right[index] {
			return -1
		}
		if left[index] > right[index] {
			return 1
		}
	}
	if len(left) < len(right) {
		return -1
	}
	if len(left) > len(right) {
		return 1
	}
	return 0
}

func totalEffectArguments(input []effectDraft, count func(effectDraft) int, what string) (int, error) {
	parts := make([]int, len(input))
	for index, effect := range input {
		parts[index] = count(effect)
	}
	return checkedStoredTotal(what, parts...)
}

func appendUint32(out []byte, value uint32) []byte {
	return append(out, byte(value>>24), byte(value>>16), byte(value>>8), byte(value))
}
