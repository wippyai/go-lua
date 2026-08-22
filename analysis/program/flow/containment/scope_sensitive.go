package containment

import (
	staticquery "github.com/wippyai/go-lua/analysis/program/static/query"

	"github.com/wippyai/go-lua/analysis/program/flow/authored"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// sealScopeSensitiveBodies materializes the one dense ownership projection
// used by consumers of lexical-scope facts. The relation is owned by the canonical
// Flow containment proof because its inputs are the sealed authored Cell and
// Label rows plus Static's Alias and Interface declarations. Consumers ask
// only whether a Body owns such a child; rewrite policy stays with its owner.
//
// The boolean return deliberately preserves the old all-or-nothing predicate:
// a malformed Cell/Label/Alias/Interface row makes the relation unavailable,
// so every branch rewrite is refused.  No partial owner set is published as a
// valid relation.
func sealScopeSensitiveBodies(
	view authored.View,
	staticView staticquery.View,
	counts [keyspace.FamilyCount]uint32,
) ([]bool, bool) {
	if !view.ContentID().Available() || !staticView.Available() {
		return nil, false
	}
	owners := make([]bool, int(counts[keyspace.FamilyBody]))
	mark := func(term keyspace.Term) {
		if keyspace.TermFamily(term) != keyspace.FamilyBody {
			return
		}
		ordinal := keyspace.TermOrdinal(term)
		if ordinal == 0 || uint64(ordinal) > uint64(len(owners)) {
			return
		}
		owners[ordinal-1] = true
	}

	cells := view.Storage().Cells()
	for index := 0; index < cells.Count(); index++ {
		term, termOK := cells.At(index)
		kind, body, key, rowOK := cells.Get(term)
		if !termOK || !rowOK {
			return nil, false
		}
		switch kind {
		case authored.CellLocal:
			if key != 0 || keyspace.TermFamily(body) != keyspace.FamilyBody || keyspace.TermOrdinal(body) == 0 {
				return nil, false
			}
			mark(body)
		case authored.CellGlobal:
			if body != 0 || key == 0 {
				return nil, false
			}
		default:
			return nil, false
		}
	}

	labels := view.Control().Labels()
	for index := 0; index < labels.Count(); index++ {
		term, termOK := labels.At(index)
		owner, rowOK := labels.Get(term)
		if !termOK || !rowOK {
			return nil, false
		}
		// Keep the historical predicate exact: the row is structurally valid
		// when it is readable; only valid Body owners can affect a valid arm.
		mark(owner)
	}

	aliases := staticView.Declarations().Aliases()
	for index := 0; index < aliases.Count(); index++ {
		term, termOK := aliases.At(index)
		owner, _, _, _, rowOK := aliases.Get(term)
		if !termOK || !rowOK {
			return nil, false
		}
		mark(owner)
	}

	interfaces := staticView.Declarations().Interfaces()
	for index := 0; index < interfaces.Count(); index++ {
		term, termOK := interfaces.At(index)
		owner, _, _, rowOK := interfaces.Get(term)
		if !termOK || !rowOK {
			return nil, false
		}
		mark(owner)
	}
	return owners, true
}

// ScopeSensitiveBody reports whether Body owns a Cell, Label, Alias, or
// Interface whose lexical scope a source transformation must preserve. It is
// a sealed dense fact query; it does not decide any consumer's rewrite policy.
func (r *Result) ScopeSensitiveBody(body keyspace.Term) (bool, bool) {
	if r == nil || !r.available() || !r.scopeSensitiveAvailable ||
		keyspace.TermFamily(body) != keyspace.FamilyBody || keyspace.TermOrdinal(body) == 0 {
		return false, false
	}
	ordinal := keyspace.TermOrdinal(body)
	if uint64(ordinal) > uint64(len(r.scopeSensitiveBodies)) {
		return false, false
	}
	return r.scopeSensitiveBodies[ordinal-1], true
}
