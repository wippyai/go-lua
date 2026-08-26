package differential

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/engine/relation/apply"
	"github.com/wippyai/go-lua/analysis/relation/semantic/invocation"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
)

// Results is an immutable zipper over two Apply result extents.  Its entries
// are paired by the exact structural InvocationAddress carried by each
// Application, never by an evaluation ordinal or by position in the input
// slices.  A missing side is sparse transport: it becomes a Before-only or
// After-only Differential, not a fabricated empty Application.
//
// Results may be empty while remaining available.  In that case Operation
// retains the authenticated operation identity for the closed extent even
// though there are no application values to carry it.
type Results struct {
	before    apply.Results
	after     apply.Results
	beforeOK  bool
	afterOK   bool
	operation signature.Identity
	values    []Differential
	sealed    bool
}

// Pair seals two Apply result extents into a canonical address zipper.
// Either input may be the zero apply.Results value, but both may not be
// omitted.  Present extents must name the same operation when both exist.
// Every side is checked for duplicate InvocationAddress values; duplicate
// addresses are ambiguous transport and are rejected rather than silently
// overwritten in a map.
//
// The output order is structural InvocationAddress order, so permuting either
// input extent does not change the sealed zipper.  The original apply.Results
// values are retained so their proposal leases remain live authorities for
// the resulting Differential entries.
func Pair(before, after apply.Results) (Results, bool) {
	beforeOK := before.Available()
	afterOK := after.Available()
	if !beforeOK && !afterOK {
		return Results{}, false
	}

	operation := before.Operation()
	if !beforeOK {
		operation = after.Operation()
	}
	if !operation.Available() {
		return Results{}, false
	}
	if beforeOK && afterOK && before.Operation() != after.Operation() {
		return Results{}, false
	}

	beforeValues, beforeAddresses, ok := collect(before, beforeOK)
	if !ok {
		return Results{}, false
	}
	afterValues, afterAddresses, ok := collect(after, afterOK)
	if !ok {
		return Results{}, false
	}

	entries := make([]paired, 0, len(beforeValues)+len(afterValues))
	matchedAfter := make([]bool, len(afterValues))
	for beforeIndex, value := range beforeValues {
		match := -1
		for afterIndex, address := range afterAddresses {
			if address.Same(beforeAddresses[beforeIndex]) {
				match = afterIndex
				break
			}
		}
		if match < 0 {
			entry, entryOK := New(value, apply.Application{})
			if !entryOK {
				return Results{}, false
			}
			entries = append(entries, paired{address: beforeAddresses[beforeIndex], value: entry})
			continue
		}
		matchedAfter[match] = true
		entry, entryOK := New(value, afterValues[match])
		if !entryOK {
			return Results{}, false
		}
		entries = append(entries, paired{address: beforeAddresses[beforeIndex], value: entry})
	}
	for afterIndex, value := range afterValues {
		if matchedAfter[afterIndex] {
			continue
		}
		entry, entryOK := New(apply.Application{}, value)
		if !entryOK {
			return Results{}, false
		}
		entries = append(entries, paired{address: afterAddresses[afterIndex], value: entry})
	}

	sort.SliceStable(entries, func(left, right int) bool {
		return entries[left].address.Compare(entries[right].address) < 0
	})
	values := make([]Differential, len(entries))
	for index, entry := range entries {
		values[index] = entry.value
	}
	result := Results{
		before:    before,
		after:     after,
		beforeOK:  beforeOK,
		afterOK:   afterOK,
		operation: operation,
		values:    values,
		sealed:    true,
	}
	if !result.Available() {
		return Results{}, false
	}
	return result, true
}

type paired struct {
	address invocation.InvocationAddress
	value   Differential
}

func collect(values apply.Results, present bool) ([]apply.Application, []invocation.InvocationAddress, bool) {
	if !present {
		return nil, nil, true
	}
	applications := make([]apply.Application, values.Len())
	addresses := make([]invocation.InvocationAddress, values.Len())
	for index := 0; index < values.Len(); index++ {
		application, ok := values.At(index)
		if !ok || !application.Available() || !application.Operation().Available() {
			return nil, nil, false
		}
		address := application.Invocation()
		if !address.Available() {
			return nil, nil, false
		}
		for _, prior := range addresses[:index] {
			if prior.Same(address) {
				return nil, nil, false
			}
		}
		applications[index] = application
		addresses[index] = address
	}
	return applications, addresses, true
}

// Available reports whether the zipper still retains at least one valid
// extent and every sealed Differential still retains its original live side
// leases.  A reset of any proposal buffer therefore propagates through the
// zipper instead of leaving a stale reconstructed application behind.
func (value Results) Available() bool {
	if !value.sealed || (!value.beforeOK && !value.afterOK) || !value.operation.Available() || value.values == nil {
		return false
	}
	if value.beforeOK && (!value.before.Available() || value.before.Operation() != value.operation) {
		return false
	}
	if value.afterOK && (!value.after.Available() || value.after.Operation() != value.operation) {
		return false
	}
	for _, entry := range value.values {
		if !entry.Available() || entry.Operation() != value.operation {
			return false
		}
	}
	return true
}

// Operation returns the exact operation identity retained by this extent,
// including when the extent has zero entries.
func (value Results) Operation() signature.Identity {
	if !value.Available() {
		return signature.Identity{}
	}
	return value.operation
}

// Len reports the number of address-paired differential entries.  Zero may
// be an authenticated empty extent; callers use Available to distinguish it
// from an unavailable zipper.
func (value Results) Len() int {
	if !value.Available() {
		return 0
	}
	return len(value.values)
}

// At returns one differential in canonical InvocationAddress order.
func (value Results) At(index int) (Differential, bool) {
	if !value.Available() || index < 0 || index >= len(value.values) {
		return Differential{}, false
	}
	return value.values[index], true
}

// Values returns a defensive copy of the sealed differential vector.
func (value Results) Values() []Differential {
	if !value.Available() {
		return nil
	}
	return append([]Differential(nil), value.values...)
}
