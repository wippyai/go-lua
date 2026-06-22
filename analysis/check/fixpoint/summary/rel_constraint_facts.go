package summary

import (
	"sort"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
)

type relConstraintFactKey struct {
	CoA     int64
	A       pathdom.PathKey
	ALength bool
	CoB     int64
	B       pathdom.PathKey
	BLength bool
	C       pathdom.PathKey
	CLength bool
	K       int64
}

type relConstraintFactLane struct{}

var relConstraintLane relConstraintFactLane

func (relConstraintFactLane) Normalize(in []callboundary.RelConstraintFact) []callboundary.RelConstraintFact {
	if len(in) == 0 {
		return nil
	}
	byKey := make(map[relConstraintFactKey]callboundary.RelConstraintFact, len(in))
	for _, fact := range in {
		fact, ok := normalizeRelConstraintFact(fact)
		if !ok {
			continue
		}
		byKey[relConstraintKeyOf(fact)] = fact
	}
	if len(byKey) == 0 {
		return nil
	}
	out := make([]callboundary.RelConstraintFact, 0, len(byKey))
	for _, fact := range byKey {
		out = append(out, fact)
	}
	sort.Slice(out, func(i, j int) bool {
		return relConstraintKeyLess(relConstraintKeyOf(out[i]), relConstraintKeyOf(out[j]))
	})
	return out
}

func (lane relConstraintFactLane) Clone(in []callboundary.RelConstraintFact) []callboundary.RelConstraintFact {
	in = lane.Normalize(in)
	if len(in) == 0 {
		return nil
	}
	out := make([]callboundary.RelConstraintFact, len(in))
	for i, fact := range in {
		out[i] = cloneRelConstraintFact(fact)
	}
	return out
}

func (lane relConstraintFactLane) Equal(a, b []callboundary.RelConstraintFact) bool {
	a = lane.Normalize(a)
	b = lane.Normalize(b)
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if relConstraintKeyOf(a[i]) != relConstraintKeyOf(b[i]) {
			return false
		}
	}
	return true
}

func (lane relConstraintFactLane) LessOrEq(a, b []callboundary.RelConstraintFact) bool {
	a = lane.Normalize(a)
	b = lane.Normalize(b)
	if len(b) == 0 {
		return true
	}
	left := make(map[relConstraintFactKey]struct{}, len(a))
	for _, fact := range a {
		left[relConstraintKeyOf(fact)] = struct{}{}
	}
	for _, fact := range b {
		if _, ok := left[relConstraintKeyOf(fact)]; !ok {
			return false
		}
	}
	return true
}

func (lane relConstraintFactLane) Join(a, b []callboundary.RelConstraintFact) []callboundary.RelConstraintFact {
	a = lane.Normalize(a)
	b = lane.Normalize(b)
	if len(a) == 0 || len(b) == 0 {
		return nil
	}
	right := make(map[relConstraintFactKey]callboundary.RelConstraintFact, len(b))
	for _, fact := range b {
		right[relConstraintKeyOf(fact)] = fact
	}
	out := make([]callboundary.RelConstraintFact, 0, len(a))
	for _, left := range a {
		if _, ok := right[relConstraintKeyOf(left)]; ok {
			out = append(out, cloneRelConstraintFact(left))
		}
	}
	return lane.Normalize(out)
}

func (lane relConstraintFactLane) Widen(prev, next []callboundary.RelConstraintFact) []callboundary.RelConstraintFact {
	return lane.Join(prev, next)
}

func normalizeRelConstraintFact(fact callboundary.RelConstraintFact) (callboundary.RelConstraintFact, bool) {
	if fact.A.Path.IsEmpty() || fact.C.Path.IsEmpty() || !fact.A.Path.IsPlaceholder() || !fact.C.Path.IsPlaceholder() {
		return callboundary.RelConstraintFact{}, false
	}
	fact.A.Path = fact.A.Path.Clone()
	fact.C.Path = fact.C.Path.Clone()
	if fact.B.Path.IsEmpty() || fact.CoB == 0 {
		fact.CoB = 0
		fact.B = callboundary.RelOperand{}
		return fact, true
	}
	if !fact.B.Path.IsPlaceholder() {
		return callboundary.RelConstraintFact{}, false
	}
	fact.B.Path = fact.B.Path.Clone()
	if relOperandKeyLess(fact.B, fact.CoB, fact.A, fact.CoA) {
		fact.A, fact.B = fact.B, fact.A
		fact.CoA, fact.CoB = fact.CoB, fact.CoA
	}
	return fact, true
}

func cloneRelConstraintFact(fact callboundary.RelConstraintFact) callboundary.RelConstraintFact {
	fact.A.Path = fact.A.Path.Clone()
	fact.B.Path = fact.B.Path.Clone()
	fact.C.Path = fact.C.Path.Clone()
	return fact
}

func relConstraintKeyOf(fact callboundary.RelConstraintFact) relConstraintFactKey {
	return relConstraintFactKey{
		CoA:     fact.CoA,
		A:       fact.A.Path.Key(),
		ALength: fact.A.IsLength,
		CoB:     fact.CoB,
		B:       fact.B.Path.Key(),
		BLength: fact.B.IsLength,
		C:       fact.C.Path.Key(),
		CLength: fact.C.IsLength,
		K:       fact.K,
	}
}

func relOperandKeyLess(left callboundary.RelOperand, leftCoeff int64, right callboundary.RelOperand, rightCoeff int64) bool {
	if left.IsLength != right.IsLength {
		return left.IsLength && !right.IsLength
	}
	if left.Path.Key() != right.Path.Key() {
		return left.Path.Key() < right.Path.Key()
	}
	return leftCoeff < rightCoeff
}

func relConstraintKeyLess(a, b relConstraintFactKey) bool {
	switch {
	case a.ALength != b.ALength:
		return a.ALength && !b.ALength
	case a.A != b.A:
		return a.A < b.A
	case a.CoA != b.CoA:
		return a.CoA < b.CoA
	case a.BLength != b.BLength:
		return a.BLength && !b.BLength
	case a.B != b.B:
		return a.B < b.B
	case a.CoB != b.CoB:
		return a.CoB < b.CoB
	case a.CLength != b.CLength:
		return a.CLength && !b.CLength
	case a.C != b.C:
		return a.C < b.C
	case a.K != b.K:
		return a.K < b.K
	default:
		return false
	}
}
