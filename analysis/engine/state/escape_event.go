package state

import "github.com/wippyai/go-lua/analysis/engine/state/escapeevent"

type EscapeEvent = escapeevent.Fact

type EscapeEventsSnapshot struct {
	Bottom bool
	Top    bool
	Facts  []EscapeEvent
}

// EscapeEventsSnapshot returns finite must escape events in stable order.
// Bottom is explicit; Top means the reachable must lane contains no events.
func (s State) EscapeEventsSnapshot() EscapeEventsSnapshot {
	snapshot := s.escapeEvents.Snapshot()
	return EscapeEventsSnapshot{
		Bottom: snapshot.Bottom,
		Top:    snapshot.Top,
		Facts:  snapshot.Facts,
	}
}

func (s State) AddEscapeEvent(fact EscapeEvent) State {
	escapeEvents, changed := s.escapeEvents.Add(fact)
	if !changed {
		return s
	}
	out := s.reachable()
	out.escapeEvents = escapeEvents
	return out
}

func (s State) HasEscapeEvent(fact EscapeEvent) bool {
	return s.escapeEvents.Has(fact)
}
