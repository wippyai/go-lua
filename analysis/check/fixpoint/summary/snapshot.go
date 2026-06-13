package summary

import "github.com/wippyai/go-lua/analysis/domain/value/axis"

// Reader reads exact summary keys.
type Reader interface {
	Read(SummaryKey) (Summary, bool)
}

// EntrySummary binds a key to a summary for snapshot construction.
type EntrySummary struct {
	Key     SummaryKey
	Summary Summary
}

// Snapshot is an immutable exact-key summary reader.
type Snapshot struct {
	reg     *axis.Registry
	entries map[SummaryKey]Summary
}

// NewSnapshot returns a snapshot containing entries.
func NewSnapshot(reg *axis.Registry, entries ...EntrySummary) Snapshot {
	if len(entries) == 0 {
		return Snapshot{reg: reg}
	}
	out := Snapshot{
		reg:     reg,
		entries: make(map[SummaryKey]Summary, len(entries)),
	}
	for _, entry := range entries {
		out.entries[entry.Key] = Normalize(reg, entry.Summary)
	}
	return out
}

// Read returns the summary for k. It never falls back to other entries for the
// same function reference.
func (s Snapshot) Read(k SummaryKey) (Summary, bool) {
	if len(s.entries) == 0 {
		return Summary{}, false
	}
	got, ok := s.entries[k]
	if !ok {
		return Summary{}, false
	}
	return got.Clone(), true
}
