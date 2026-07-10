// Package dynamicindex owns resolved dynamic-index write facts and admission.
package dynamicindex

import (
	"strconv"
	"strings"

	"github.com/wippyai/go-lua/analysis/domain/lattice"
	"github.com/wippyai/go-lua/analysis/domain/lattice/lift"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/internal/mapedit"
	"github.com/wippyai/go-lua/analysis/internal/registrycache"
)

type Site string

const factflowSitePrefix = "factflow.dynamic_index_write@"

// SiteForPoint returns the stable site name used for dynamic-index facts
// produced directly from a factflow write point.
func SiteForPoint(point int) Site {
	return Site(factflowSitePrefix + strconv.Itoa(point))
}

// PointFromSite decodes a SiteForPoint value.
func PointFromSite(site Site) (int, bool) {
	raw := string(site)
	if !strings.HasPrefix(raw, factflowSitePrefix) {
		return 0, false
	}
	point, err := strconv.Atoi(strings.TrimPrefix(raw, factflowSitePrefix))
	return point, err == nil
}

type Key struct {
	Table keyspace.Key
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

type FactConfig struct {
	KeyValue    product.Value
	HasKeyValue bool
	Value       product.Value
	HasValue    bool
	Admission   Admission
}

func NewFact(reg *axis.Registry, config FactConfig) Fact {
	out := Bottom(reg)
	out.Admission = config.Admission
	if config.HasKeyValue {
		out.KeyValue = config.KeyValue
		out.KeyPresence = product.PresenceOf(config.KeyValue)
	}
	if config.HasValue {
		out.Value = config.Value
	}
	return out
}

var factDomainCache registrycache.Cache[lattice.Lattice[Fact]]
var mapDomainCache registrycache.Cache[lattice.Lattice[map[Key]Fact]]

func Domain(reg *axis.Registry) lattice.Lattice[Fact] {
	return factDomainCache.GetFor(reg, factDomainForRegistry)
}

func factDomainForRegistry(reg *axis.Registry) lattice.Lattice[Fact] {
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
	return mapDomainCache.GetFor(reg, factMapDomainForRegistry)
}

func factMapDomainForRegistry(reg *axis.Registry) lattice.Lattice[map[Key]Fact] {
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
	return mapedit.Clone(in)
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
