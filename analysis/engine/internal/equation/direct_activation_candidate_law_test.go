package equation

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
)

// TestDirectActivationTransportArityFollowsDeclaredVector proves the transport
// denominator carries whatever import arity its declaration names: the expanded
// edge census is entries*imports + exits at every arity, each entry receives one
// edge per declared import Factor, and every exit returns the one exported
// Factor.
func TestDirectActivationTransportArityFollowsDeclaredVector(t *testing.T) {
	factors := []composition.Key{boundaryKey(60), boundaryKey(61), boundaryKey(62), boundaryKey(63), boundaryKey(64)}
	rows := make([]composition.Factor, len(factors))
	for index, key := range factors {
		rows[index] = composition.Factor{Key: key}
	}
	source, sourceOK := composition.Seal(composition.Candidate{Factors: rows})
	if !sourceOK || source == nil {
		t.Fatal("cold factor rows")
	}
	base := NewBatch()
	if _, siteOK := base.AdmitSite(boundaryKey(80), EmptyScope(), TrueExpr(), InitPresent); !siteOK || !base.Seal() {
		t.Fatal("sealed base batch")
	}
	entries, exits, trigger := []PointRef{7, 9}, []PointRef{11}, PointRef(5)
	export := factors[len(factors)-1]
	origin := MaterializationOrigin{Family: boundaryKey(70), Application: boundaryKey(71), Target: boundaryKey(72), Endpoint: boundaryKey(73)}
	for arity := 1; arity < len(factors); arity++ {
		imports := factors[:arity]
		set, setOK := NewDirectActivationTransportSet(source, base, entries, exits, imports, export)
		if !setOK || !set.Available() || !set.OwnedBy(source, base) {
			t.Fatalf("arity %d transport set refused", arity)
		}
		candidate, candidateOK := NewDirectActivationCandidate(source, base, origin, trigger, set)
		if !candidateOK || !candidate.Available() {
			t.Fatalf("arity %d candidate refused", arity)
		}
		want := len(entries)*arity + len(exits)
		if candidate.TransportCount() != want {
			t.Fatalf("arity %d expanded %d edges, declared census is %d", arity, candidate.TransportCount(), want)
		}
		forward := make(map[PointRef]map[composition.Key]int, len(entries))
		reverse := make(map[PointRef]int, len(exits))
		for index := 0; index < want; index++ {
			transport, transportOK := candidate.TransportAt(index)
			if !transportOK {
				t.Fatalf("arity %d edge %d", arity, index)
			}
			switch {
			case transport.Source == trigger:
				if transport.Factor == export {
					t.Fatalf("arity %d imported the exported Factor", arity)
				}
				if forward[transport.Target] == nil {
					forward[transport.Target] = map[composition.Key]int{}
				}
				forward[transport.Target][transport.Factor]++
			case transport.Target == trigger:
				if transport.Factor != export {
					t.Fatalf("arity %d exported a Factor outside the declared vector", arity)
				}
				reverse[transport.Source]++
			default:
				t.Fatalf("arity %d edge %d touched neither side of the trigger", arity, index)
			}
		}
		if len(forward) != len(entries) || len(reverse) != len(exits) {
			t.Fatalf("arity %d reached %d entries and %d exits", arity, len(forward), len(reverse))
		}
		for _, entry := range entries {
			carried := forward[entry]
			if len(carried) != arity {
				t.Fatalf("arity %d entry %d received %d import Factors", arity, entry, len(carried))
			}
			for _, factor := range imports {
				if carried[factor] != 1 {
					t.Fatalf("arity %d entry %d received declared import %v %d times", arity, entry, factor, carried[factor])
				}
			}
		}
		for _, exit := range exits {
			if reverse[exit] != 1 {
				t.Fatalf("arity %d exit %d returned %d exported edges", arity, exit, reverse[exit])
			}
		}
	}
	if _, ok := NewDirectActivationTransportSet(source, base, entries, exits, nil, export); ok {
		t.Fatal("an empty import vector was admitted")
	}
	if _, ok := NewDirectActivationTransportSet(source, base, entries, exits, []composition.Key{factors[0], factors[0]}, export); ok {
		t.Fatal("one Factor was admitted twice in an import vector")
	}
	if _, ok := NewDirectActivationTransportSet(source, base, entries, exits, []composition.Key{factors[0]}, factors[0]); ok {
		t.Fatal("an export repeating an import was admitted")
	}
	if _, ok := NewDirectActivationTransportSet(source, base, entries, exits, []composition.Key{boundaryKey(90)}, export); ok {
		t.Fatal("an import Factor unknown to the composition was admitted")
	}
}
