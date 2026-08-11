package evaluation

import "github.com/wippyai/go-lua/program/keyspace"

// Entry returns the first evaluated runtime term for one authored term.
// The query performs no allocation and fails closed for a foreign, unsupported,
// or unsealed term.
func (ports *Ports) Entry(term keyspace.Term) (keyspace.Term, bool) {
	if !ports.available() {
		return 0, false
	}
	return ports.plane(keyspace.TermFamily(term), keyspace.TermOrdinal(term), &ports.entry)
}

// Finish returns the final evaluation/commit port for one authored term. The
// query is allocation free and has no dependency on source or authored state.
func (ports *Ports) Finish(term keyspace.Term) (keyspace.Term, bool) {
	if !ports.available() {
		return 0, false
	}
	return ports.plane(keyspace.TermFamily(term), keyspace.TermOrdinal(term), &ports.finish)
}
