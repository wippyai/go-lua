package compiler

import (
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
)

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
	return comparePublicationEffectSpec(left.publication, right.publication)
}

// comparePublicationEffectSpec orders authored publication declarations. It is
// the ordering the sealed projection used to carry, and it is the same ordering:
// freezePublicationEffect copies the seven fields one-for-one with no
// transformation, so comparing the declaration compares the projection.
func comparePublicationEffectSpec(left, right vocabulary.PublicationEffectSpec) int {
	if left.Kind != right.Kind {
		if left.Kind < right.Kind {
			return -1
		}
		return 1
	}
	if left.Subject != right.Subject {
		if left.Subject < right.Subject {
			return -1
		}
		return 1
	}
	if left.Destination != right.Destination {
		if left.Destination < right.Destination {
			return -1
		}
		return 1
	}
	if left.Context != right.Context {
		if left.Context < right.Context {
			return -1
		}
		return 1
	}
	if left.Escape != right.Escape {
		if left.Escape < right.Escape {
			return -1
		}
		return 1
	}
	if left.Mutability != right.Mutability {
		if left.Mutability < right.Mutability {
			return -1
		}
		return 1
	}
	if left.Lifetime < right.Lifetime {
		return -1
	}
	if left.Lifetime > right.Lifetime {
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
	return vocabulary.CheckedStoredTotal(what, parts...)
}
