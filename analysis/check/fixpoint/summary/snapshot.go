package summary

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/value/axis"
)

// Reader reads exact summary keys.
type Reader interface {
	Read(SummaryKey) (Summary, bool)
}

// OwnedNormalizedReader reads summaries that are already normalized and owned
// by the producer. Returned summaries share backing storage with the reader;
// callers must treat them as immutable or clone before mutation.
type OwnedNormalizedReader interface {
	ReadOwnedNormalized(SummaryKey) (Summary, bool)
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

// NewSnapshotOwnedNormalized returns a snapshot that takes ownership of entries
// that are already normalized. Callers must not mutate the provided summaries
// after construction. Use NewSnapshot at public or arbitrary input boundaries.
func NewSnapshotOwnedNormalized(reg *axis.Registry, entries ...EntrySummary) Snapshot {
	if len(entries) == 0 {
		return Snapshot{reg: reg}
	}
	out := Snapshot{
		reg:     reg,
		entries: make(map[SummaryKey]Summary, len(entries)),
	}
	for _, entry := range entries {
		out.entries[entry.Key] = entry.Summary
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

// ReadOwnedNormalized returns the summary for k without cloning. Snapshot
// entries are normalized at construction, and this method is for internal
// fixpoint paths that preserve the immutable-summary contract.
func (s Snapshot) ReadOwnedNormalized(k SummaryKey) (Summary, bool) {
	if len(s.entries) == 0 {
		return Summary{}, false
	}
	got, ok := s.entries[k]
	if !ok {
		return Summary{}, false
	}
	return got, true
}

// Entries returns every exact-key summary in deterministic key order.
func (s Snapshot) Entries() []EntrySummary {
	if len(s.entries) == 0 {
		return nil
	}
	out := make([]EntrySummary, 0, len(s.entries))
	for key, got := range s.entries {
		out = append(out, EntrySummary{Key: key, Summary: got.Clone()})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key.Less(out[j].Key) })
	return out
}

// EntriesOwnedNormalized returns every exact-key summary in deterministic key
// order without cloning summary payloads. Callers must not mutate returned
// summaries unless they clone first.
func (s Snapshot) EntriesOwnedNormalized() []EntrySummary {
	if len(s.entries) == 0 {
		return nil
	}
	out := make([]EntrySummary, 0, len(s.entries))
	for key, got := range s.entries {
		out = append(out, EntrySummary{Key: key, Summary: got})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key.Less(out[j].Key) })
	return out
}
