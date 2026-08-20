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

// deriveCellTermPaths joins the closed Source Cell roles to Flow's sealed
// Binding roles. It has no fallback relation: every Cell is claimed once by
// one typed definition role, and local identities fold both lexical Body and
// definition host paths without using a Cell Term or global ordinal.
func deriveCellTermPaths(sourceView source.View, catalog source.CellRoles, view authored.View, bindings binding.Result, bodies *body.Result, forest *containment.Result, bodyPaths []identity.ContentID, paths *[keyspace.FamilyCount][]identity.ContentID) error {
	if paths == nil || bodies == nil || forest == nil || !catalog.Matches(sourceView) || !binding.Matches(&bindings, sourceView.Identity().ContentID(), view.ContentID()) {
		return errors.New("semanticpath: Cell role join owners are unavailable")
	}
	cells := view.Storage().Cells()
	if catalog.CellCount() != cells.Count() || bindings.CellCount() != cells.Count() || len(paths[keyspace.FamilyCell]) != cells.Count()+1 {
		return errors.New("semanticpath: Cell role join denominator disagrees")
	}
	claimed := make([]bool, cells.Count()+1)
	claim := func(cell keyspace.Term, want kind.CellRole, host keyspace.Term, bodyTerm keyspace.Term, slot uint32, qualifier uint32) error {
		ordinal := keyspace.TermOrdinal(cell)
		if keyspace.TermFamily(cell) != keyspace.FamilyCell || ordinal == 0 || uint64(ordinal) >= uint64(len(claimed)) || claimed[ordinal] {
			return errors.New("semanticpath: duplicate or invalid Cell role claim")
		}
		role, roleOK := bindings.Role(cell)
		gotHost, hostOK := bindings.Host(cell)
		cellKind, gotBody, key, cellOK := cells.Get(cell)
		if !roleOK || !hostOK || !cellOK || role != want || gotHost != host || cellKind != authored.CellLocal || key != 0 || gotBody != bodyTerm {
			return errors.New("semanticpath: Cell role claim disagrees with Binding or authored row")
		}
		parent, parentOK := forest.Parent(cell)
		if !parentOK || parent != host {
			return errors.New("semanticpath: Cell containment parent disagrees with Binding host")
		}
		bodyOrdinal := keyspace.TermOrdinal(bodyTerm)
		if keyspace.TermFamily(bodyTerm) != keyspace.FamilyBody || bodyOrdinal == 0 || uint64(bodyOrdinal) > uint64(len(bodyPaths)) || !bodyPaths[bodyOrdinal-1].Available() {
			return errors.New("semanticpath: Cell role Body path is unavailable")
		}
		var hostPath identity.ContentID
		if keyspace.TermFamily(host) == keyspace.FamilyBody {
			hostOrdinal := keyspace.TermOrdinal(host)
			if hostOrdinal == 0 || uint64(hostOrdinal) >= uint64(len(paths[keyspace.FamilyBody])) {
				return errors.New("semanticpath: Cell Body host is invalid")
			}
			hostPath = paths[keyspace.FamilyBody][hostOrdinal]
		} else {
			hostFamily, hostOrdinal := keyspace.TermFamily(host), keyspace.TermOrdinal(host)
			if hostFamily <= keyspace.FamilyInvalid || hostFamily >= keyspace.FamilyCount || hostOrdinal == 0 || uint64(hostOrdinal) >= uint64(len(paths[hostFamily])) {
				return errors.New("semanticpath: Cell definition host is invalid")
			}
			hostPath = paths[hostFamily][hostOrdinal]
		}
		if !hostPath.Available() {
			return errors.New("semanticpath: Cell definition host path is unavailable")
		}
		descriptor := digestPath3("semantic-cell-definition-v2", bodyPaths[bodyOrdinal-1], uint32(want), slot, qualifier, source.Span{})
		paths[keyspace.FamilyCell][ordinal] = digestBytes("semantic-cell-occurrence-v2", hostPath, descriptor)
		claimed[ordinal] = true
		return nil
	}
	for ordinal := 1; ordinal <= cells.Count(); ordinal++ {
		cell := keyspace.MakeTerm(keyspace.FamilyCell, uint32(ordinal))
		role, roleOK := bindings.Role(cell)
		host, hostOK := bindings.Host(cell)
		cellKind, bodyTerm, key, cellOK := cells.Get(cell)
		if !roleOK || !hostOK || !cellOK {
			return errors.New("semanticpath: Cell role row is unavailable")
		}
		switch role {
		case kind.CellGlobal:
			if host != 0 || cellKind != authored.CellGlobal || bodyTerm != 0 || key == 0 {
				return errors.New("semanticpath: global Cell role is invalid")
			}
			if _, hasParent := forest.Parent(cell); hasParent {
				return errors.New("semanticpath: global Cell has a containment parent")
			}
			atomID, atomOK := catalog.ExactIDForKey(key)
			if !atomOK {
				return errors.New("semanticpath: global Cell exact atom is unavailable")
			}
			if claimed[ordinal] {
				return errors.New("semanticpath: duplicate global Cell claim")
			}
			paths[keyspace.FamilyCell][ordinal] = digestPath("semantic-global-cell-v2", atomID, uint32(kind.CellGlobal), 0, source.Span{})
			claimed[ordinal] = true
		case kind.CellLocal:
			sourceRole, sourceOK := catalog.BindRoleForCell(host, cell)
			position, positionOK := sourceRole.Position()
			if !sourceOK || !catalog.Owns(sourceRole) || sourceRole.Kind() != source.CellRoleBind || !sourceRole.MatchesCell(cell) || !positionOK {
				return errors.New("semanticpath: Bind Cell role is invalid")
			}
			if err := claim(cell, kind.CellLocal, host, bodyTerm, uint32(position+1), uint32(source.CellRoleBind)); err != nil {
				return err
			}
		case kind.CellFormal:
			sourceRole, sourceOK := catalog.FormalRoleForCell(host, cell)
			position, positionOK := sourceRole.Position()
			if !sourceOK || !catalog.Owns(sourceRole) || sourceRole.Kind() != source.CellRoleFormal || !sourceRole.MatchesCell(cell) || !positionOK {
				return errors.New("semanticpath: Formal Cell role is invalid")
			}
			if err := claim(cell, kind.CellFormal, host, bodyTerm, uint32(position+1), uint32(source.CellRoleFormal)); err != nil {
				return err
			}
		case kind.CellFunctionVararg:
			functionBody, vararg, ok := functionCellRole(view.Functions(), host)
			if !ok || vararg != cell {
				return errors.New("semanticpath: Function Vararg Cell role is invalid")
			}
			if err := claim(cell, kind.CellFunctionVararg, host, functionBody, 0, 0); err != nil {
				return err
			}
		case kind.CellCapture:
			function, outer, position, inverseOK := bindings.CaptureRoleForCell(cell)
			outerRole, outerOK := bindings.Role(outer)
			if !inverseOK || function != host || outer == cell || !outerOK || outerRole < kind.CellGlobal || outerRole > kind.CellChunkVararg {
				return errors.New("semanticpath: Function Capture inverse is invalid")
			}
			if err := claim(cell, kind.CellCapture, function, bodyTerm, uint32(position+1), 0); err != nil {
				return err
			}
		case kind.CellLoop:
			loop, loopKind, position, inverseOK := bindings.LoopRoleForCell(cell)
			if !inverseOK || loop != host {
				return errors.New("semanticpath: Loop Cell inverse is invalid")
			}
			if err := claim(cell, kind.CellLoop, loop, bodyTerm, uint32(position+1), uint32(loopKind)); err != nil {
				return err
			}
		case kind.CellChunkVararg:
			entry, entryOK := bodies.Entry()
			chunk, chunkOK := bindings.ChunkVararg()
			if !entryOK || !chunkOK || chunk != cell || host != entry {
				return errors.New("semanticpath: chunk Vararg Cell role is invalid")
			}
			if err := claim(cell, kind.CellChunkVararg, entry, entry, 0, 0); err != nil {
				return err
			}
		default:
			return errors.New("semanticpath: Cell role is invalid")
		}
	}
	for ordinal := 1; ordinal <= cells.Count(); ordinal++ {
		if !claimed[ordinal] || !paths[keyspace.FamilyCell][ordinal].Available() {
			return errors.New("semanticpath: Cell role is uncovered")
		}
	}
	return nil
}

func functionCellRole(functions authored.Functions, function keyspace.Term) (keyspace.Term, keyspace.Term, bool) {
	_, bodyTerm, vararg, ok := functions.Get(function)
	return bodyTerm, vararg, ok
}
