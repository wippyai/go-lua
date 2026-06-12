package state

import (
	"github.com/wippyai/go-lua/analysis/domain/lattice"
	"github.com/wippyai/go-lua/analysis/domain/lattice/lift"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
)

type EscapePlacement uint8

const (
	EscapePlacementBottom EscapePlacement = iota
	EscapePlacementStack
	EscapePlacementOwnedHeap
	EscapePlacementEscaped
	EscapePlacementUnknown
)

func (s State) ReadEscapePlacement(id identity.ID) EscapePlacement {
	if id == (identity.ID{}) {
		return EscapePlacementBottom
	}
	if s.escapePlacementTop {
		return EscapePlacementUnknown
	}
	if placement, ok := s.escapePlacement[id]; ok {
		return placement
	}
	return EscapePlacementBottom
}

func (s State) WriteEscapePlacement(id identity.ID, placement EscapePlacement) State {
	if id == (identity.ID{}) {
		return s
	}
	if s.escapePlacementTop {
		panic("state: cannot finite-write escape placement into top escape-placement lane")
	}
	if placement == EscapePlacementBottom {
		placements, changed := deleteEscapePlacementEntry(s.escapePlacement, id)
		if !changed {
			return s
		}
		out := s.reachable()
		out.escapePlacement = placements
		return out
	}
	placements := cloneEscapePlacementMap(s.escapePlacement)
	if placements == nil {
		placements = make(map[identity.ID]EscapePlacement, 1)
	}
	placements[id] = placement
	out := s.reachable()
	out.escapePlacement = placements
	return out
}

func escapePlacementMapDomain() lattice.Lattice[map[identity.ID]EscapePlacement] {
	return lift.Map[identity.ID, EscapePlacement](escapePlacementDomain())
}

func escapePlacementDomain() lattice.Lattice[EscapePlacement] {
	return lattice.Lattice[EscapePlacement]{
		Bottom: func() EscapePlacement { return EscapePlacementBottom },
		Top:    func() EscapePlacement { return EscapePlacementUnknown },
		Equal: func(a, b EscapePlacement) bool {
			return a == b
		},
		LessOrEq: func(a, b EscapePlacement) bool {
			return a <= b
		},
		Join: func(a, b EscapePlacement) EscapePlacement {
			if a > b {
				return a
			}
			return b
		},
		Widen: func(prev, next EscapePlacement) EscapePlacement {
			if prev > next {
				return prev
			}
			return next
		},
	}
}

func cloneEscapePlacementMap(in map[identity.ID]EscapePlacement) map[identity.ID]EscapePlacement {
	if len(in) == 0 {
		return nil
	}
	out := make(map[identity.ID]EscapePlacement, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func deleteEscapePlacementEntry(
	in map[identity.ID]EscapePlacement,
	id identity.ID,
) (map[identity.ID]EscapePlacement, bool) {
	if _, ok := in[id]; !ok {
		return in, false
	}
	out := make(map[identity.ID]EscapePlacement, len(in)-1)
	for k, v := range in {
		if k != id {
			out[k] = v
		}
	}
	if len(out) == 0 {
		return nil, true
	}
	return out, true
}
