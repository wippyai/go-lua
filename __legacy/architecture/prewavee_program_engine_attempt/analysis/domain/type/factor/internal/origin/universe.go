package origin

import (
	"github.com/wippyai/go-lua/analysis/program"
	"github.com/wippyai/go-lua/analysis/program/link"
)

// Universe is the closed provenance vocabulary of the Type Factor installed
// for one exact Link.  It is derived from the Program forms for which the
// current Type rules can construct a Pack; it is not a solver capacity,
// heuristic, or open registry.
//
// An Origin's Link Value is only an ordinal. The universe is therefore the
// closed enumeration which makes that ordinal meaningful for this installed
// Factor and validates every finite provenance set before it enters a carrier.
// It retains no Link pointer or content identity in the hot carrier.
type Universe struct {
	entries []Origin
}

// Build closes the finite source-position vocabulary that Cell/Read rules may
// later attach to data-only literal and Values results:
//
//   - literals expose one scalar (Fixed(0)) position;
//   - Values expose each statically fixed result position and the one
//     statically represented variadic-tail position when present.
//
// Literal and Values transfers themselves remain Data-only. Other Program
// forms contribute no provenance positions rather than silently creating a
// broad "any value/any index" universe. When a future Cell/Read rule adds a
// source form, it must extend this exact enumeration atomically.
func Build(source *link.Link) (*Universe, bool) {
	if source == nil || !source.ContentID().Available() {
		return nil, false
	}
	entries := make([]Origin, 0)
	for shardIndex := 0; shardIndex < source.ShardCount(); shardIndex++ {
		shard, ok := source.ShardAt(shardIndex)
		if !ok {
			return nil, false
		}
		p, ok := source.Program(shard)
		if !ok || p == nil || !appendProgram(source, shard, p, &entries) {
			return nil, false
		}
	}
	set := New(entries...)
	return &Universe{entries: set.entries}, true
}

// Count reports the exact number of provenance witnesses admitted by the
// installed rule surface.
func (universe *Universe) Count() int {
	if universe == nil {
		return 0
	}
	return len(universe.entries)
}

// At returns one witness in canonical order.
func (universe *Universe) At(index int) (Origin, bool) {
	if universe == nil || index < 0 || index >= len(universe.entries) {
		return Origin{}, false
	}
	return universe.entries[index], true
}

// Contains reports whether origin belongs to this Link-derived finite
// vocabulary. Origin intentionally carries no Link pointer or content ID;
// factor installation, not a hot provenance witness, owns that authority.
func (universe *Universe) Contains(origin Origin) bool {
	if universe == nil {
		return false
	}
	_, found := search(universe.entries, origin)
	return found
}

// Valid reports whether every origin in set belongs to this finite Universe.
// It is intentionally all-or-nothing: provenance outside the declared Type
// rules is a construction error, not evidence that can be dropped or joined
// into a less precise fallback.
func (universe *Universe) Valid(set Set) bool {
	if universe == nil {
		return false
	}
	for index := 0; index < set.Count(); index++ {
		entry, ok := set.At(index)
		if !ok || !universe.Contains(entry) {
			return false
		}
	}
	return true
}

// Remaining is the fourth, finite widening-rank component. Union can only
// add admitted witnesses, so every proper origin widening strictly decreases
// this number without a cardinality cap.  Invalid sets have no rank.
func (universe *Universe) Remaining(set Set) (uint64, bool) {
	if !universe.Valid(set) || set.Count() > len(universe.entries) {
		return 0, false
	}
	return uint64(len(universe.entries) - set.Count()), true
}

func appendProgram(source *link.Link, shard link.Shard, p *program.Program, entries *[]Origin) bool {
	if !appendLiterals(source, shard, p, entries) {
		return false
	}
	for index := 0; index < p.ValuesCount(); index++ {
		term, ok := p.ValuesAt(index)
		if !ok || !appendValues(source, shard, p, term, entries) {
			return false
		}
	}
	return true
}

func appendLiterals(source *link.Link, shard link.Shard, p *program.Program, entries *[]Origin) bool {
	for _, family := range []struct {
		count int
		at    func(int) (program.Term, bool)
	}{
		{p.NilCount(), p.NilAt},
		{p.BoolCount(), p.BoolAt},
		{p.IntegerCount(), p.IntegerAt},
		{p.FloatCount(), p.FloatAt},
		{p.StringCount(), p.StringAt},
	} {
		for index := 0; index < family.count; index++ {
			term, ok := family.at(index)
			if !ok || !appendOrigin(source, shard, term, Fixed(0), entries) {
				return false
			}
		}
	}
	return true
}

func appendValues(source *link.Link, shard link.Shard, p *program.Program, term program.Term, entries *[]Origin) bool {
	fixed, ok := p.ValuesLen(term)
	if !ok {
		return false
	}
	for index := 0; index < fixed; index++ {
		if !appendOrigin(source, shard, term, Fixed(uint32(index)), entries) {
			return false
		}
	}
	_, tail, ok := p.Values(term)
	if !ok {
		return false
	}
	return tail == 0 || appendOrigin(source, shard, term, Tail(0), entries)
}

func appendOrigin(source *link.Link, shard link.Shard, term program.Term, position Position, entries *[]Origin) bool {
	value, ok := source.ValueOf(shard, term)
	if !ok {
		return false
	}
	*entries = append(*entries, At(value, position))
	return true
}

func search(entries []Origin, wanted Origin) (int, bool) {
	low, high := 0, len(entries)
	for low < high {
		middle := low + (high-low)/2
		comparison := Compare(entries[middle], wanted)
		if comparison < 0 {
			low = middle + 1
		} else {
			high = middle
		}
	}
	return low, low < len(entries) && Compare(entries[low], wanted) == 0
}
