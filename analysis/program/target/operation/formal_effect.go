package operation

import (
	"errors"
	"sort"

	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
)

// canonicalFormalEffects is the final operation-owner fence for formal
// ownership rows. Compiler normally supplies an already frozen row, but the
// operation query builder also validates and canonicalizes its construction
// input so no alternate append path can publish a non-deterministic row.
func canonicalFormalEffects(input []vocabulary.FormalEffectSpec, tail vocabulary.RowTail) ([]vocabulary.FormalEffectSpec, vocabulary.RowTail, error) {
	if tail == 0 {
		if len(input) != 0 {
			return nil, 0, errors.New("populated formal effect row has no tail")
		}
		tail = vocabulary.RowClosed
	}
	if tail != vocabulary.RowClosed && tail != vocabulary.RowUnknownOpen {
		return nil, 0, errors.New("formal effect row has invalid tail")
	}
	out := append([]vocabulary.FormalEffectSpec(nil), input...)
	for index := range out {
		item := out[index]
		switch item.Kind {
		case vocabulary.FormalEffectBorrow, vocabulary.FormalEffectRetain,
			vocabulary.FormalEffectSendParam, vocabulary.FormalEffectExport,
			vocabulary.FormalEffectOpaque, vocabulary.FormalEffectFreeze:
			if item.Param < -1 {
				return nil, 0, errors.New("formal effect parameter is less than -1")
			}
			if item.Into != 0 || item.HasInto || item.FromParam != 0 {
				return nil, 0, errors.New("formal effect carries unrelated operands")
			}
		case vocabulary.FormalEffectStore:
			if item.Param < -1 {
				return nil, 0, errors.New("formal effect parameter is less than -1")
			}
			if item.FromParam != 0 {
				return nil, 0, errors.New("formal effect carries unrelated operands")
			}
			if !item.HasInto {
				item.Into = -1
			} else if item.Into < 0 {
				return nil, 0, errors.New("present Store Into is less than zero")
			}
		case vocabulary.FormalEffectBorrowAll:
			if item.Param != 0 || item.Into != 0 || item.HasInto || item.FromParam != 0 {
				return nil, 0, errors.New("formal effect carries unrelated operands")
			}
		case vocabulary.FormalEffectSendSuffix:
			if item.FromParam < 0 {
				return nil, 0, errors.New("formal effect suffix boundary is negative")
			}
			if item.Param != 0 || item.Into != 0 || item.HasInto {
				return nil, 0, errors.New("formal effect carries unrelated operands")
			}
		default:
			return nil, 0, errors.New("formal effect has invalid kind")
		}
		out[index] = item
	}
	sort.Slice(out, func(left, right int) bool { return compareFormalEffects(out[left], out[right]) < 0 })
	for index := 1; index < len(out); index++ {
		if compareFormalEffects(out[index-1], out[index]) == 0 {
			return nil, 0, errors.New("duplicate formal effect")
		}
	}
	return out, tail, nil
}

func compareFormalEffects(left, right vocabulary.FormalEffectSpec) int {
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
