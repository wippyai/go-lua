package persistentcalls

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/lattice"
	"github.com/wippyai/go-lua/analysis/engine/state"
)

type approximation[S any] struct {
	valid       bool
	summary     S
	callsTop    bool
	callEntries map[FunctionID]state.State
}

func approximationDomain[S any](states lattice.Lattice[state.State], summaries lattice.Lattice[S]) lattice.Lattice[approximation[S]] {
	return lattice.Lattice[approximation[S]]{
		Bottom: func() approximation[S] { return approximation[S]{} },
		Top: func() approximation[S] {
			return approximation[S]{valid: true, summary: summaries.Top(), callsTop: true}
		},
		Equal: func(a, b approximation[S]) bool {
			if a.valid != b.valid {
				return false
			}
			if !a.valid {
				return true
			}
			if !summaries.Equal(a.summary, b.summary) || a.callsTop != b.callsTop {
				return false
			}
			return a.callsTop || equalCallEntries(states, a.callEntries, b.callEntries)
		},
		LessOrEq: func(a, b approximation[S]) bool {
			if !a.valid {
				return true
			}
			if !b.valid || !summaries.LessOrEq(a.summary, b.summary) || a.callsTop && !b.callsTop {
				return false
			}
			if b.callsTop {
				return true
			}
			for id, entry := range a.callEntries {
				other := states.Bottom()
				if value, ok := b.callEntries[id]; ok {
					other = value
				}
				if !states.LessOrEq(entry, other) {
					return false
				}
			}
			return true
		},
		Join: func(a, b approximation[S]) approximation[S] {
			return joinApproximation(states, summaries, a, b, false)
		},
		Widen: func(prev, next approximation[S]) approximation[S] {
			return joinApproximation(states, summaries, prev, next, true)
		},
	}
}

func joinApproximation[S any](states lattice.Lattice[state.State], summaries lattice.Lattice[S], a, b approximation[S], widen bool) approximation[S] {
	if !a.valid {
		return cloneApproximation(b)
	}
	if !b.valid {
		return cloneApproximation(a)
	}
	joinSummary := summaries.Join
	joinState := states.Join
	if widen {
		joinSummary = summaries.Widen
		joinState = states.Widen
	}
	out := approximation[S]{valid: true, summary: joinSummary(a.summary, b.summary), callEntries: cloneCallEntries(a.callEntries)}
	if a.callsTop || b.callsTop {
		out.callsTop = true
		out.callEntries = nil
		return out
	}
	if out.callEntries == nil && len(b.callEntries) != 0 {
		out.callEntries = make(map[FunctionID]state.State, len(b.callEntries))
	}
	for id, entry := range b.callEntries {
		if prior, ok := out.callEntries[id]; ok {
			out.callEntries[id] = joinState(prior, entry)
		} else {
			out.callEntries[id] = entry
		}
	}
	return out
}

func equalCallEntries(domain lattice.Lattice[state.State], a, b map[FunctionID]state.State) bool {
	if len(a) != len(b) {
		return false
	}
	for id, left := range a {
		right, ok := b[id]
		if !ok || !domain.Equal(left, right) {
			return false
		}
	}
	return true
}

func cloneApproximation[S any](in approximation[S]) approximation[S] {
	if !in.valid {
		return approximation[S]{}
	}
	return approximation[S]{valid: true, summary: in.summary, callsTop: in.callsTop, callEntries: cloneCallEntries(in.callEntries)}
}

func cloneCallEntries(in map[FunctionID]state.State) map[FunctionID]state.State {
	if len(in) == 0 {
		return nil
	}
	out := make(map[FunctionID]state.State, len(in))
	for id, entry := range in {
		out[id] = entry
	}
	return out
}

type revisionTracker[S any] struct {
	domain    lattice.Lattice[S]
	summaries map[FunctionID]Summary[S]
}

func newRevisionTracker[S any](domain lattice.Lattice[S], published map[FunctionID]Summary[S]) *revisionTracker[S] {
	return &revisionTracker[S]{domain: domain, summaries: cloneSummaries(published)}
}

func (r *revisionTracker[S]) currentRevision(id FunctionID, summary S) Revision {
	current := r.summaries[id]
	if !r.domain.Equal(current.Value, summary) {
		current.Value = summary
		current.Revision++
		r.summaries[id] = current
	}
	return current.Revision
}

type trackingReader[S any] struct {
	domain  lattice.Lattice[S]
	tracker *revisionTracker[S]
	read    func(FunctionID) approximation[S]

	observed map[FunctionID]Revision
}

func (r *trackingReader[S]) begin() {
	r.observed = make(map[FunctionID]Revision)
}

func (r *trackingReader[S]) Read(id FunctionID) (Summary[S], bool) {
	approx := r.read(id)
	if !approx.valid {
		summary := r.domain.Bottom()
		revision := r.tracker.currentRevision(id, summary)
		r.observed[id] = revision
		return Summary[S]{Value: summary, Revision: revision}, false
	}
	revision := r.tracker.currentRevision(id, approx.summary)
	r.observed[id] = revision
	return Summary[S]{Value: approx.summary, Revision: revision}, true
}

func (r *trackingReader[S]) dependencies() map[FunctionID]Revision {
	if len(r.observed) == 0 {
		return nil
	}
	out := make(map[FunctionID]Revision, len(r.observed))
	for id, revision := range r.observed {
		out[id] = revision
	}
	return out
}

func (r *trackingReader[S]) revalidate() error {
	for id, revision := range r.observed {
		approx := r.read(id)
		if !approx.valid {
			if current := r.tracker.currentRevision(id, r.domain.Bottom()); current != revision {
				return fmt.Errorf("persistentcalls: dependency %q changed during solve: %d -> %d", id, revision, current)
			}
			continue
		}
		if current := r.tracker.currentRevision(id, approx.summary); current != revision {
			return fmt.Errorf("persistentcalls: dependency %q changed during solve: %d -> %d", id, revision, current)
		}
	}
	return nil
}
