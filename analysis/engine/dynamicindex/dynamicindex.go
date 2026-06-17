// Package dynamicindex owns resolved dynamic-index write facts and admission.
package dynamicindex

import (
	"github.com/wippyai/go-lua/analysis/domain/lattice"
	"github.com/wippyai/go-lua/analysis/domain/lattice/lift"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
)

type Site string

type Key struct {
	Table pathdom.PathKey
	Site  Site
}

type Admission uint8

const (
	AdmissionBottom Admission = iota
	AdmissionAdmitted
	AdmissionRejected
	AdmissionUnknown
)

type Fact struct {
	KeyPresence presence.Value
	KeyValue    product.Value
	Value       product.Value
	Admission   Admission
}

func Domain(reg *axis.Registry) lattice.Lattice[Fact] {
	valueDomain := product.Domain(reg)
	return lattice.Lattice[Fact]{
		Bottom: func() Fact { return Bottom(reg) },
		Top:    Top,
		Equal: func(a, b Fact) bool {
			return presence.Equal(a.KeyPresence, b.KeyPresence) &&
				valueDomain.Equal(a.KeyValue, b.KeyValue) &&
				valueDomain.Equal(a.Value, b.Value) &&
				a.Admission == b.Admission
		},
		LessOrEq: func(a, b Fact) bool {
			return presenceLessOrEq(a.KeyPresence, b.KeyPresence) &&
				valueDomain.LessOrEq(a.KeyValue, b.KeyValue) &&
				valueDomain.LessOrEq(a.Value, b.Value) &&
				admissionLessOrEq(a.Admission, b.Admission)
		},
		Join: func(a, b Fact) Fact {
			return Fact{
				KeyPresence: presence.Join(a.KeyPresence, b.KeyPresence),
				KeyValue:    valueDomain.Join(a.KeyValue, b.KeyValue),
				Value:       valueDomain.Join(a.Value, b.Value),
				Admission:   admissionJoin(a.Admission, b.Admission),
			}
		},
		Widen: func(prev, next Fact) Fact {
			return Fact{
				KeyPresence: presence.Widen(prev.KeyPresence, next.KeyPresence),
				KeyValue:    valueDomain.Widen(prev.KeyValue, next.KeyValue),
				Value:       valueDomain.Widen(prev.Value, next.Value),
				Admission:   admissionJoin(prev.Admission, next.Admission),
			}
		},
	}
}

func MapDomain(reg *axis.Registry) lattice.Lattice[map[Key]Fact] {
	return lift.Map[Key, Fact](Domain(reg))
}

func Bottom(reg *axis.Registry) Fact {
	return Fact{
		KeyPresence: presence.Bottom(),
		KeyValue:    product.Bottom(reg),
		Value:       product.Bottom(reg),
		Admission:   AdmissionBottom,
	}
}

func Top() Fact {
	return Fact{
		KeyPresence: presence.Top(),
		KeyValue:    product.Top(),
		Value:       product.Top(),
		Admission:   AdmissionUnknown,
	}
}

func CloneMap(in map[Key]Fact) map[Key]Fact {
	if len(in) == 0 {
		return nil
	}
	out := make(map[Key]Fact, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func presenceLessOrEq(a, b presence.Value) bool {
	return presence.Join(a, b) == b
}

func admissionLessOrEq(a, b Admission) bool {
	return a == b || a == AdmissionBottom || b == AdmissionUnknown
}

func admissionJoin(a, b Admission) Admission {
	if a == b {
		return a
	}
	if a == AdmissionBottom {
		return b
	}
	if b == AdmissionBottom {
		return a
	}
	return AdmissionUnknown
}
