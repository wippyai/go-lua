package compiler

import (
	"bytes"

	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
)

func compareSuspension(left, right suspensionDraft) int {
	if order := compareSuspensionKey(left, right); order != 0 {
		return order
	}
	if left.multiplicity < right.multiplicity {
		return -1
	}
	if left.multiplicity > right.multiplicity {
		return 1
	}
	return 0
}

func compareSuspensionKey(left, right suspensionDraft) int {
	if left.yield != right.yield {
		if left.yield < right.yield {
			return -1
		}
		return 1
	}
	if left.reentry != right.reentry {
		if left.reentry < right.reentry {
			return -1
		}
		return 1
	}
	if left.source != right.source {
		if left.source < right.source {
			return -1
		}
		return 1
	}
	return 0
}

func compareResume(left, right resumeDraft) int {
	if left.source != right.source {
		if left.source < right.source {
			return -1
		}
		return 1
	}
	if left.carrier < right.carrier {
		return -1
	}
	if left.carrier > right.carrier {
		return 1
	}
	if compared := compareValues(left.arguments, right.arguments); compared != 0 {
		return compared
	}
	return 0
}

func compareInputSource(left, right vocabulary.InputSource) int {
	if left.Kind < right.Kind {
		return -1
	}
	if left.Kind > right.Kind {
		return 1
	}
	if left.Ordinal < right.Ordinal {
		return -1
	}
	if left.Ordinal > right.Ordinal {
		return 1
	}
	return 0
}

func compareTransfer(left, right transferDraft) int {
	if compared := compareTransferIdentity(left, right); compared != 0 {
		return compared
	}
	if left.identity < right.identity {
		return -1
	}
	if left.identity > right.identity {
		return 1
	}
	if left.capabilities < right.capabilities {
		return -1
	}
	if left.capabilities > right.capabilities {
		return 1
	}
	return 0
}

func compareTransferIdentity(left, right transferDraft) int {
	if left.endpoint.Kind < right.endpoint.Kind {
		return -1
	}
	if left.endpoint.Kind > right.endpoint.Kind {
		return 1
	}
	if left.endpoint.Input < right.endpoint.Input {
		return -1
	}
	if left.endpoint.Input > right.endpoint.Input {
		return 1
	}
	if compared := compareInputSource(left.payload, right.payload); compared != 0 {
		return compared
	}
	return compareInputSource(left.alias, right.alias)
}

func compareCallback(left, right callbackDraft) int {
	if compared := compareCallbackIdentity(left, right); compared != 0 {
		return compared
	}
	if left.admission < right.admission {
		return -1
	}
	if left.admission > right.admission {
		return 1
	}
	if left.lifecycle < right.lifecycle {
		return -1
	}
	if left.lifecycle > right.lifecycle {
		return 1
	}
	return 0
}

func compareCallbackIdentity(left, right callbackDraft) int {
	if left.function.Kind < right.function.Kind {
		return -1
	}
	if left.function.Kind > right.function.Kind {
		return 1
	}
	if left.function.Ordinal < right.function.Ordinal {
		return -1
	}
	if left.function.Ordinal > right.function.Ordinal {
		return 1
	}
	if compared := compareValues(left.arguments, right.arguments); compared != 0 {
		return compared
	}
	for index := range left.outcomes {
		if compared := compareValues(left.outcomes[index], right.outcomes[index]); compared != 0 {
			return compared
		}
	}
	return 0
}

func compareBinding(left, right vocabulary.BindingSpec) int {
	if left.Namespace != right.Namespace {
		if left.Namespace < right.Namespace {
			return -1
		}
		return 1
	}
	if order := vocabulary.CompareSegments(left.Owner, right.Owner); order != 0 {
		return order
	}
	if order := vocabulary.CompareSegments(left.Member, right.Member); order != 0 {
		return order
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
