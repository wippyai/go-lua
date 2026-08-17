package lexical

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// sourceItem retains the one authored Body order until its Body closes. A zero
// term with a nonzero cell is typed evidence for one source turn whose Cell is
// declared later; lexical never decides which semantic Term fills that turn.
type sourceItem struct {
	term keyspace.Term
	cell bind.Symbol
}

// CellEvidence identifies one reserved source turn and the later lexical Cell
// it requires. Its representation is private so only Bodies can resolve or
// fill it; semantic children retain the typed value without reconstructing
// lexical coordinates.
type CellEvidence struct {
	owner  keyspace.Term
	source int
	cell   bind.Symbol
}

// Append publishes one existing statement, Label, TypeAlias, or Interface
// Term in exact authored order. Seal derives root and frontier projections.
func (b *Bodies) Append(term keyspace.Term) error {
	if term == 0 {
		return fmt.Errorf("lualower: could not append Body source Term")
	}
	b.source = append(b.source, sourceItem{term: term})
	return nil
}

// ReserveCell appends one authored source turn whose semantic Term depends on
// a Cell declared later in the same Body. It returns lexical evidence only;
// the owning semantic child must resolve and fill it explicitly before Finish.
func (b *Bodies) ReserveCell(cell bind.Symbol) (CellEvidence, error) {
	if cell == 0 || len(b.frames) == 0 {
		return CellEvidence{}, fmt.Errorf("lualower: invalid Body source Cell evidence")
	}
	evidence := CellEvidence{owner: b.Owner(), source: len(b.source), cell: cell}
	b.source = append(b.source, sourceItem{cell: cell})
	return evidence, nil
}

// ResolveCell returns the exact currently visible Cell required by evidence.
func (b *Bodies) ResolveCell(evidence CellEvidence) (keyspace.Term, error) {
	if !b.validEvidence(evidence) {
		return 0, fmt.Errorf("lualower: invalid Body source Cell evidence")
	}
	cell, ok := b.Resolve(evidence.cell)
	if !ok || cell == 0 {
		return 0, fmt.Errorf("lualower: Body source Cell is absent")
	}
	return cell, nil
}

// Fill replaces one reserved source turn with the semantic Term created by
// its owning child. It cannot change source order or be applied twice.
func (b *Bodies) Fill(evidence CellEvidence, term keyspace.Term) error {
	if term == 0 || !b.validEvidence(evidence) {
		return fmt.Errorf("lualower: invalid Body source evidence fill")
	}
	item := &b.source[evidence.source]
	if item.term != 0 || item.cell != evidence.cell {
		return fmt.Errorf("lualower: Body source evidence already filled")
	}
	item.term = term
	item.cell = 0
	return nil
}

func (b *Bodies) validEvidence(evidence CellEvidence) bool {
	if len(b.frames) == 0 || evidence.owner == 0 || evidence.owner != b.Owner() ||
		evidence.source < b.frames[len(b.frames)-1].sourceMark ||
		evidence.source < 0 || evidence.source >= len(b.source) || evidence.cell == 0 {
		return false
	}
	item := b.source[evidence.source]
	return item.term == 0 && item.cell == evidence.cell
}
