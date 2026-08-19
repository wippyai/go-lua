package evaluation

import "github.com/wippyai/go-lua/analysis/program/keyspace"

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

// TermCount returns the pre-Outcome Source denominator captured by this
// sealed port proof. It bounds a transitive Finish query without retaining or
// rebuilding a second term index.
func (ports *Ports) TermCount() uint32 {
	if !ports.available() {
		return 0
	}
	return ports.termCount
}
