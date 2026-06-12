package state

import (
	"github.com/wippyai/go-lua/analysis/domain/lattice"
	"github.com/wippyai/go-lua/analysis/domain/lattice/lift"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
)

type DynamicIndexSite string

type DynamicIndexKey struct {
	Table pathdom.PathKey
	Site  DynamicIndexSite
}

type DynamicIndexAdmission uint8

const (
	DynamicIndexAdmissionBottom DynamicIndexAdmission = iota
	DynamicIndexAdmissionAdmitted
	DynamicIndexAdmissionRejected
	DynamicIndexAdmissionUnknown
)

type DynamicIndexFact struct {
	KeyPresence presence.Value
	KeyValue    product.Value
	Value       product.Value
	Admission   DynamicIndexAdmission
}

func (s State) ReadDynamicIndexFact(reg *axis.Registry, key DynamicIndexKey) DynamicIndexFact {
	if key.Table == "" {
		return dynamicIndexFactBottom(reg)
	}
	if s.dynamicIndexTop {
		return dynamicIndexFactTop()
	}
	if fact, ok := s.dynamicIndex[key]; ok {
		return fact
	}
	return dynamicIndexFactBottom(reg)
}

func (s State) WriteDynamicIndexFact(reg *axis.Registry, key DynamicIndexKey, fact DynamicIndexFact) State {
	if key.Table == "" {
		return s
	}
	if s.dynamicIndexTop {
		panic("state: cannot finite-write dynamic index fact into top dynamic-index lane")
	}
	domain := dynamicIndexFactDomain(reg)
	if domain.Equal(fact, domain.Bottom()) {
		facts, changed := deleteDynamicIndexEntry(s.dynamicIndex, key)
		if !changed {
			return s
		}
		out := s.reachable()
		out.dynamicIndex = facts
		return out
	}
	facts := cloneDynamicIndexMap(s.dynamicIndex)
	if facts == nil {
		facts = make(map[DynamicIndexKey]DynamicIndexFact, 1)
	}
	facts[key] = fact
	out := s.reachable()
	out.dynamicIndex = facts
	return out
}

func dynamicIndexMapDomain(reg *axis.Registry) lattice.Lattice[map[DynamicIndexKey]DynamicIndexFact] {
	return lift.Map[DynamicIndexKey, DynamicIndexFact](dynamicIndexFactDomain(reg))
}

func dynamicIndexFactDomain(reg *axis.Registry) lattice.Lattice[DynamicIndexFact] {
	valueDomain := product.Domain(reg)
	return lattice.Lattice[DynamicIndexFact]{
		Bottom: func() DynamicIndexFact { return dynamicIndexFactBottom(reg) },
		Top:    dynamicIndexFactTop,
		Equal: func(a, b DynamicIndexFact) bool {
			return presence.Equal(a.KeyPresence, b.KeyPresence) &&
				valueDomain.Equal(a.KeyValue, b.KeyValue) &&
				valueDomain.Equal(a.Value, b.Value) &&
				a.Admission == b.Admission
		},
		LessOrEq: func(a, b DynamicIndexFact) bool {
			return presenceLessOrEq(a.KeyPresence, b.KeyPresence) &&
				valueDomain.LessOrEq(a.KeyValue, b.KeyValue) &&
				valueDomain.LessOrEq(a.Value, b.Value) &&
				dynamicIndexAdmissionLessOrEq(a.Admission, b.Admission)
		},
		Join: func(a, b DynamicIndexFact) DynamicIndexFact {
			return DynamicIndexFact{
				KeyPresence: presence.Join(a.KeyPresence, b.KeyPresence),
				KeyValue:    valueDomain.Join(a.KeyValue, b.KeyValue),
				Value:       valueDomain.Join(a.Value, b.Value),
				Admission:   dynamicIndexAdmissionJoin(a.Admission, b.Admission),
			}
		},
		Widen: func(prev, next DynamicIndexFact) DynamicIndexFact {
			return DynamicIndexFact{
				KeyPresence: presence.Widen(prev.KeyPresence, next.KeyPresence),
				KeyValue:    valueDomain.Widen(prev.KeyValue, next.KeyValue),
				Value:       valueDomain.Widen(prev.Value, next.Value),
				Admission:   dynamicIndexAdmissionJoin(prev.Admission, next.Admission),
			}
		},
	}
}

func dynamicIndexFactBottom(reg *axis.Registry) DynamicIndexFact {
	return DynamicIndexFact{
		KeyPresence: presence.Bottom(),
		KeyValue:    product.Bottom(reg),
		Value:       product.Bottom(reg),
		Admission:   DynamicIndexAdmissionBottom,
	}
}

func dynamicIndexFactTop() DynamicIndexFact {
	return DynamicIndexFact{
		KeyPresence: presence.Top(),
		KeyValue:    product.Top(),
		Value:       product.Top(),
		Admission:   DynamicIndexAdmissionUnknown,
	}
}

func presenceLessOrEq(a, b presence.Value) bool {
	return presence.Join(a, b) == b
}

func dynamicIndexAdmissionLessOrEq(a, b DynamicIndexAdmission) bool {
	return a == b || a == DynamicIndexAdmissionBottom || b == DynamicIndexAdmissionUnknown
}

func dynamicIndexAdmissionJoin(a, b DynamicIndexAdmission) DynamicIndexAdmission {
	if a == b {
		return a
	}
	if a == DynamicIndexAdmissionBottom {
		return b
	}
	if b == DynamicIndexAdmissionBottom {
		return a
	}
	return DynamicIndexAdmissionUnknown
}

func cloneDynamicIndexMap(in map[DynamicIndexKey]DynamicIndexFact) map[DynamicIndexKey]DynamicIndexFact {
	if len(in) == 0 {
		return nil
	}
	out := make(map[DynamicIndexKey]DynamicIndexFact, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func deleteDynamicIndexEntry(
	in map[DynamicIndexKey]DynamicIndexFact,
	key DynamicIndexKey,
) (map[DynamicIndexKey]DynamicIndexFact, bool) {
	if _, ok := in[key]; !ok {
		return in, false
	}
	out := make(map[DynamicIndexKey]DynamicIndexFact, len(in)-1)
	for k, v := range in {
		if k != key {
			out[k] = v
		}
	}
	if len(out) == 0 {
		return nil, true
	}
	return out, true
}
