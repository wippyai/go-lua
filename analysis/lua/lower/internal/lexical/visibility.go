package lexical

import (
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

type activeUndo struct {
	id      bind.Symbol
	prior   keyspace.Term
	existed bool
}

// Resolve returns the currently visible Cell for one binder identity.
func (b *Bodies) Resolve(id bind.Symbol) (keyspace.Term, bool) {
	term, ok := b.active[id]
	return term, ok && term != 0
}

// Has reports whether a binder identity already has an active Cell.
func (b *Bodies) Has(id bind.Symbol) bool {
	_, ok := b.active[id]
	return ok
}

func (b *Bodies) install(id bind.Symbol, term keyspace.Term) {
	if b.active == nil {
		b.active = make(map[bind.Symbol]keyspace.Term)
	}
	prior, existed := b.active[id]
	b.undo = append(b.undo, activeUndo{id: id, prior: prior, existed: existed})
	b.active[id] = term
}

func (b *Bodies) restore(mark int) {
	for i := len(b.undo) - 1; i >= mark; i-- {
		undo := b.undo[i]
		if undo.existed {
			b.active[undo.id] = undo.prior
		} else {
			delete(b.active, undo.id)
		}
	}
	b.undo = b.undo[:mark]
}
