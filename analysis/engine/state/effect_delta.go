package state

import (
	"github.com/wippyai/go-lua/analysis/domain/lattice"
	"github.com/wippyai/go-lua/analysis/domain/lattice/lift"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
)

type EffectSite string

type EffectDeltaKind uint8

const (
	EffectDeltaMutation EffectDeltaKind = iota + 1
	EffectDeltaEscape
	EffectDeltaCall
)

type EffectDeltaChange uint8

const (
	EffectDeltaChangeBottom EffectDeltaChange = iota
	EffectDeltaChangeNone
	EffectDeltaChangeChanged
	EffectDeltaChangeUnknown
)

type EffectDeltaKey struct {
	Target pathdom.PathKey
	Site   EffectSite
	Kind   EffectDeltaKind
}

type EffectDelta struct {
	Before product.Value
	After  product.Value
	Change EffectDeltaChange
}

func (s State) ReadEffectDelta(reg *axis.Registry, key EffectDeltaKey) EffectDelta {
	if key.Target == "" {
		return effectDeltaBottom(reg)
	}
	if s.effectDeltasTop {
		return effectDeltaTop()
	}
	if delta, ok := s.effectDeltas[key]; ok {
		return delta
	}
	return effectDeltaBottom(reg)
}

func (s State) WriteEffectDelta(reg *axis.Registry, key EffectDeltaKey, delta EffectDelta) State {
	if key.Target == "" {
		return s
	}
	if s.effectDeltasTop {
		panic("state: cannot finite-write effect delta into top effect-delta lane")
	}
	domain := effectDeltaDomain(reg)
	if domain.Equal(delta, domain.Bottom()) {
		deltas, changed := deleteEffectDeltaEntry(s.effectDeltas, key)
		if !changed {
			return s
		}
		out := s.reachable()
		out.effectDeltas = deltas
		return out
	}
	deltas := cloneEffectDeltaMap(s.effectDeltas)
	if deltas == nil {
		deltas = make(map[EffectDeltaKey]EffectDelta, 1)
	}
	deltas[key] = delta
	out := s.reachable()
	out.effectDeltas = deltas
	return out
}

func effectDeltaMapDomain(reg *axis.Registry) lattice.Lattice[map[EffectDeltaKey]EffectDelta] {
	return lift.Map[EffectDeltaKey, EffectDelta](effectDeltaDomain(reg))
}

func effectDeltaDomain(reg *axis.Registry) lattice.Lattice[EffectDelta] {
	valueDomain := product.Domain(reg)
	return lattice.Lattice[EffectDelta]{
		Bottom: func() EffectDelta { return effectDeltaBottom(reg) },
		Top:    effectDeltaTop,
		Equal: func(a, b EffectDelta) bool {
			return valueDomain.Equal(a.Before, b.Before) &&
				valueDomain.Equal(a.After, b.After) &&
				a.Change == b.Change
		},
		LessOrEq: func(a, b EffectDelta) bool {
			return valueDomain.LessOrEq(a.Before, b.Before) &&
				valueDomain.LessOrEq(a.After, b.After) &&
				effectDeltaChangeLessOrEq(a.Change, b.Change)
		},
		Join: func(a, b EffectDelta) EffectDelta {
			return EffectDelta{
				Before: valueDomain.Join(a.Before, b.Before),
				After:  valueDomain.Join(a.After, b.After),
				Change: effectDeltaChangeJoin(a.Change, b.Change),
			}
		},
		Widen: func(prev, next EffectDelta) EffectDelta {
			return EffectDelta{
				Before: valueDomain.Widen(prev.Before, next.Before),
				After:  valueDomain.Widen(prev.After, next.After),
				Change: effectDeltaChangeJoin(prev.Change, next.Change),
			}
		},
	}
}

func effectDeltaBottom(reg *axis.Registry) EffectDelta {
	return EffectDelta{
		Before: product.Bottom(reg),
		After:  product.Bottom(reg),
		Change: EffectDeltaChangeBottom,
	}
}

func effectDeltaTop() EffectDelta {
	return EffectDelta{
		Before: product.Top(),
		After:  product.Top(),
		Change: EffectDeltaChangeUnknown,
	}
}

func effectDeltaChangeLessOrEq(a, b EffectDeltaChange) bool {
	return a == b || a == EffectDeltaChangeBottom || b == EffectDeltaChangeUnknown
}

func effectDeltaChangeJoin(a, b EffectDeltaChange) EffectDeltaChange {
	if a == b {
		return a
	}
	if a == EffectDeltaChangeBottom {
		return b
	}
	if b == EffectDeltaChangeBottom {
		return a
	}
	return EffectDeltaChangeUnknown
}

func cloneEffectDeltaMap(in map[EffectDeltaKey]EffectDelta) map[EffectDeltaKey]EffectDelta {
	if len(in) == 0 {
		return nil
	}
	out := make(map[EffectDeltaKey]EffectDelta, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func deleteEffectDeltaEntry(
	in map[EffectDeltaKey]EffectDelta,
	key EffectDeltaKey,
) (map[EffectDeltaKey]EffectDelta, bool) {
	if _, ok := in[key]; !ok {
		return in, false
	}
	out := make(map[EffectDeltaKey]EffectDelta, len(in)-1)
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
