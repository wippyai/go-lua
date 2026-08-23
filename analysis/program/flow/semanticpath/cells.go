package semanticpath

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/binding"
	"github.com/wippyai/go-lua/analysis/program/flow/body"
	"github.com/wippyai/go-lua/analysis/program/flow/containment"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

// deriveCellTermPaths reads Flow's sealed Binding column - role, definition
// host, and host-order slot - and folds one definition identity per Cell.
// Binding owns the classification and the slot; this pass joins them to the
// Body and host paths and never re-walks the authored Bind, formal, capture,
// or Loop order that produced them.
func deriveCellTermPaths(sourceView source.View, catalog source.CellRoles, view authored.View, bindings binding.Result, bodies *body.Result, forest *containment.Result, bodyPaths []identity.ContentID, paths *[keyspace.FamilyCount][]identity.ContentID) error {
	if paths == nil || bodies == nil || forest == nil || !catalog.Matches(sourceView) || !binding.Matches(&bindings, sourceView.Identity().ContentID(), view.ContentID()) {
		return errors.New("semanticpath: Cell role join owners are unavailable")
	}
	cells := view.Storage().Cells()
	if catalog.CellCount() != cells.Count() || bindings.CellCount() != cells.Count() || len(paths[keyspace.FamilyCell]) != cells.Count()+1 {
		return errors.New("semanticpath: Cell role join denominator disagrees")
	}
	loops := view.Control().Loops()
	for ordinal := 1; ordinal <= cells.Count(); ordinal++ {
		cell := keyspace.MakeTerm(keyspace.FamilyCell, uint32(ordinal))
		role, roleOK := bindings.Role(cell)
		host, hostOK := bindings.Host(cell)
		slot, slotOK := bindings.Slot(cell)
		cellKind, bodyTerm, key, cellOK := cells.Get(cell)
		if !roleOK || !hostOK || !slotOK || !cellOK {
			return errors.New("semanticpath: Cell role row is unavailable")
		}
		if role == kind.CellGlobal {
			if host != 0 || slot != 0 || cellKind != authored.CellGlobal || bodyTerm != 0 || key == 0 {
				return errors.New("semanticpath: global Cell role is invalid")
			}
			if _, hasParent := forest.Parent(cell); hasParent {
				return errors.New("semanticpath: global Cell has a containment parent")
			}
			atomID, atomOK := catalog.ExactIDForKey(key)
			if !atomOK {
				return errors.New("semanticpath: global Cell exact atom is unavailable")
			}
			paths[keyspace.FamilyCell][ordinal] = digestPath("semantic-global-cell-v2", atomID, uint32(kind.CellGlobal), 0, source.Span{})
			continue
		}
		qualifier, qualifierOK := definitionQualifier(role, loops, host)
		if !qualifierOK {
			return errors.New("semanticpath: Cell role is invalid")
		}
		if cellKind != authored.CellLocal || key != 0 {
			return errors.New("semanticpath: lexical Cell authored row disagrees with Binding")
		}
		if parent, parentOK := forest.Parent(cell); !parentOK || parent != host {
			return errors.New("semanticpath: Cell containment parent disagrees with Binding host")
		}
		bodyOrdinal := keyspace.TermOrdinal(bodyTerm)
		if keyspace.TermFamily(bodyTerm) != keyspace.FamilyBody || bodyOrdinal == 0 || uint64(bodyOrdinal) > uint64(len(bodyPaths)) || !bodyPaths[bodyOrdinal-1].Available() {
			return errors.New("semanticpath: Cell role Body path is unavailable")
		}
		hostFamily, hostOrdinal := keyspace.TermFamily(host), keyspace.TermOrdinal(host)
		if hostFamily <= keyspace.FamilyInvalid || hostFamily >= keyspace.FamilyCount || hostOrdinal == 0 || uint64(hostOrdinal) >= uint64(len(paths[hostFamily])) {
			return errors.New("semanticpath: Cell definition host is invalid")
		}
		hostPath := paths[hostFamily][hostOrdinal]
		if !hostPath.Available() {
			return errors.New("semanticpath: Cell definition host path is unavailable")
		}
		descriptor := digestPath3("semantic-cell-definition-v2", bodyPaths[bodyOrdinal-1], uint32(role), slot, qualifier, source.Span{})
		paths[keyspace.FamilyCell][ordinal] = digestBytes("semantic-cell-occurrence-v2", hostPath, descriptor)
	}
	for ordinal := 1; ordinal <= cells.Count(); ordinal++ {
		if !paths[keyspace.FamilyCell][ordinal].Available() {
			return errors.New("semanticpath: Cell role is uncovered")
		}
	}
	return nil
}

// definitionQualifier is the authored-order discriminator folded into a
// lexical Cell's definition descriptor beside its role and slot. A Bind and a
// Function formal slot are positions in Source's two distinct authored
// orders, so each carries that order's kind; a Loop header slot carries the
// Loop kind that shaped the header; every remaining lexical role owns its
// slot outright and qualifies it with zero.
func definitionQualifier(role kind.CellRole, loops authored.Loops, host keyspace.Term) (uint32, bool) {
	switch role {
	case kind.CellLocal:
		return uint32(source.CellRoleBind), true
	case kind.CellFormal:
		return uint32(source.CellRoleFormal), true
	case kind.CellLoop:
		_, _, loopKind, _, ok := loops.Get(host)
		if !ok || loopKind != kind.LoopNumericFor && loopKind != kind.LoopGenericFor {
			return 0, false
		}
		return uint32(loopKind), true
	case kind.CellCapture, kind.CellFunctionVararg, kind.CellChunkVararg:
		return 0, true
	default:
		return 0, false
	}
}
