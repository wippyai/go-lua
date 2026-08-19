package boot

import (
	"errors"
	"sort"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/scalar"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
)

func validAggregate(value vocabulary.BootAggregate) bool {
	return value == vocabulary.BootAggregateTable || value == vocabulary.BootAggregateMetatable
}

func validMutability(value vocabulary.InitialMutability) bool {
	return value == vocabulary.InitialMutable || value == vocabulary.InitialFrozen
}

func emptyBinding(value vocabulary.BindingSpec) bool {
	return value.Namespace == 0 && len(value.Owner) == 0 && len(value.Member) == 0
}

func initialBindingClass(kind vocabulary.InitialValueKind) vocabulary.InitialBindingClass {
	switch kind {
	case vocabulary.InitialValueOperation:
		return vocabulary.InitialBindingAdmitted
	case vocabulary.InitialValueDeniedOperation:
		return vocabulary.InitialBindingDenied
	case vocabulary.InitialValueNil, vocabulary.InitialValueBoolean, vocabulary.InitialValueInteger, vocabulary.InitialValueFloat, vocabulary.InitialValueString, vocabulary.InitialValueRoot, vocabulary.InitialValueAbsent:
		return vocabulary.InitialBindingOrdinary
	default:
		return vocabulary.InitialBindingInvalid
	}
}

func normalizeKey(value keyspace.LiteralValue) (keyspace.LiteralValue, error) {
	normalized, ok := scalar.Normalize(value)
	if !ok || normalized != value {
		return keyspace.LiteralValue{}, errors.New("target/boot: invalid exact key")
	}
	return normalized, nil
}

func compareLiteral(left, right keyspace.LiteralValue) int {
	order, ok := scalar.Compare(left, right)
	if !ok {
		return 0
	}
	return order
}

func compareEntry(left, right entryDraft) int {
	if left.root < right.root {
		return -1
	}
	if left.root > right.root {
		return 1
	}
	return compareLiteral(left.key, right.key)
}

func lookupEntry(entries []entryDraft, root vocabulary.InitialRoot, key keyspace.LiteralValue) (entryDraft, bool) {
	index := sort.Search(len(entries), func(index int) bool {
		entry := entries[index]
		return entry.root > root || (entry.root == root && compareLiteral(entry.key, key) >= 0)
	})
	if index == len(entries) || entries[index].root != root || compareLiteral(entries[index].key, key) != 0 {
		return entryDraft{}, false
	}
	return entries[index], true
}

func compareBinding(left, right vocabulary.BindingSpec) int {
	if left.Namespace < right.Namespace {
		return -1
	}
	if left.Namespace > right.Namespace {
		return 1
	}
	if order := vocabulary.CompareSegments(left.Owner, right.Owner); order != 0 {
		return order
	}
	return vocabulary.CompareSegments(left.Member, right.Member)
}

func compareValue(left, right valueDraft) int {
	if left.kind < right.kind {
		return -1
	}
	if left.kind > right.kind {
		return 1
	}
	switch left.kind {
	case vocabulary.InitialValueBoolean:
		if !left.boolean && right.boolean {
			return -1
		}
		if left.boolean && !right.boolean {
			return 1
		}
	case vocabulary.InitialValueInteger:
		if left.integer < right.integer {
			return -1
		}
		if left.integer > right.integer {
			return 1
		}
	case vocabulary.InitialValueFloat:
		if left.floatBits < right.floatBits {
			return -1
		}
		if left.floatBits > right.floatBits {
			return 1
		}
	case vocabulary.InitialValueString:
		if left.string < right.string {
			return -1
		}
		if left.string > right.string {
			return 1
		}
	case vocabulary.InitialValueRoot:
		if left.root < right.root {
			return -1
		}
		if left.root > right.root {
			return 1
		}
	case vocabulary.InitialValueOperation:
		if left.operation < right.operation {
			return -1
		}
		if left.operation > right.operation {
			return 1
		}
	case vocabulary.InitialValueDeniedOperation:
		return compareBinding(left.binding, right.binding)
	}
	return 0
}

func valueHandle(values []valueDraft, want valueDraft) (vocabulary.InitialValue, bool) {
	index := sort.Search(len(values), func(index int) bool { return compareValue(values[index], want) >= 0 })
	if index == len(values) || compareValue(values[index], want) != 0 {
		return 0, false
	}
	return vocabulary.InitialValue(index + 1), true
}

func exactString(value keyspace.LiteralValue) (string, bool) {
	if value.Kind != keyspace.LiteralString {
		return "", false
	}
	return value.String, true
}
