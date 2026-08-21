package compiler

import (
	"errors"
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
)

// freezeFormalEffects validates and canonically seals the operation-owned
// ownership metadata row. Formal effects are a set: authored permutations have
// one sealed order, while exact duplicates are rejected as ambiguous input.
func freezeFormalEffects(input vocabulary.FormalEffectRow) ([]vocabulary.FormalEffectSpec, vocabulary.RowTail, error) {
	// A zero FormalEffectRow is the omitted optional declaration and therefore
	// has the same canonical empty row as an explicitly closed row. A populated
	// row must state its admitted tail explicitly; no row variable or other
	// tail can enter this plane.
	tail := input.Tail
	if tail == 0 {
		if len(input.Occurrences) != 0 {
			return nil, 0, errors.New("target: populated formal effect row has no tail")
		}
		tail = vocabulary.RowClosed
	}
	if tail != vocabulary.RowClosed && tail != vocabulary.RowUnknownOpen {
		return nil, 0, errors.New("target: formal effect row has invalid tail")
	}
	if _, err := vocabulary.CheckedStoredLength("formal effect table", len(input.Occurrences)); err != nil {
		return nil, 0, err
	}
	if len(input.Occurrences) == 0 {
		return nil, tail, nil
	}
	out := make([]vocabulary.FormalEffectSpec, len(input.Occurrences))
	for index, item := range input.Occurrences {
		canonical, err := canonicalFormalEffect(item)
		if err != nil {
			return nil, 0, fmt.Errorf("target: formal effect %d: %w", index, err)
		}
		out[index] = canonical
	}
	sort.Slice(out, func(left, right int) bool { return compareFormalEffect(out[left], out[right]) < 0 })
	for index := 1; index < len(out); index++ {
		if compareFormalEffect(out[index-1], out[index]) == 0 {
			return nil, 0, fmt.Errorf("target: duplicate formal effect %d", index)
		}
	}
	return out, tail, nil
}

func canonicalFormalEffect(input vocabulary.FormalEffectSpec) (vocabulary.FormalEffectSpec, error) {
	out := input
	switch input.Kind {
	case vocabulary.FormalEffectBorrow, vocabulary.FormalEffectRetain,
		vocabulary.FormalEffectSendParam, vocabulary.FormalEffectExport,
		vocabulary.FormalEffectOpaque, vocabulary.FormalEffectFreeze:
		if input.Param < -1 {
			return vocabulary.FormalEffectSpec{}, errors.New("parameter is less than -1")
		}
		if input.Into != 0 || input.HasInto || input.FromParam != 0 {
			return vocabulary.FormalEffectSpec{}, errors.New("kind carries unrelated operands")
		}
	case vocabulary.FormalEffectStore:
		if input.Param < -1 {
			return vocabulary.FormalEffectSpec{}, errors.New("parameter is less than -1")
		}
		if input.FromParam != 0 {
			return vocabulary.FormalEffectSpec{}, errors.New("kind carries unrelated operands")
		}
		if !input.HasInto {
			// The bool is the presence authority. Canonicalizing the payload
			// removes the second representation of an absent destination.
			out.Into = -1
		} else if input.Into < 0 {
			return vocabulary.FormalEffectSpec{}, errors.New("present Store Into is less than zero")
		}
	case vocabulary.FormalEffectBorrowAll:
		if input.Param != 0 || input.Into != 0 || input.HasInto || input.FromParam != 0 {
			return vocabulary.FormalEffectSpec{}, errors.New("kind carries unrelated operands")
		}
	case vocabulary.FormalEffectSendSuffix:
		if input.FromParam < 0 {
			return vocabulary.FormalEffectSpec{}, errors.New("suffix boundary is negative")
		}
		if input.Param != 0 || input.Into != 0 || input.HasInto {
			return vocabulary.FormalEffectSpec{}, errors.New("kind carries unrelated operands")
		}
	default:
		return vocabulary.FormalEffectSpec{}, errors.New("invalid kind")
	}
	return out, nil
}

func compareFormalEffect(left, right vocabulary.FormalEffectSpec) int {
	if left.Kind != right.Kind {
		if left.Kind < right.Kind {
			return -1
		}
		return 1
	}
	if left.Param != right.Param {
		if left.Param < right.Param {
			return -1
		}
		return 1
	}
	if left.HasInto != right.HasInto {
		if !left.HasInto {
			return -1
		}
		return 1
	}
	if left.Into != right.Into {
		if left.Into < right.Into {
			return -1
		}
		return 1
	}
	if left.FromParam < right.FromParam {
		return -1
	}
	if left.FromParam > right.FromParam {
		return 1
	}
	return 0
}
