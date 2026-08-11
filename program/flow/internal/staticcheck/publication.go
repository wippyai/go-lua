package staticcheck

import (
	"errors"

	"github.com/wippyai/go-lua/program/flow/internal/authored"
	"github.com/wippyai/go-lua/program/flow/internal/binding"
	"github.com/wippyai/go-lua/program/flow/internal/directbinding"
	"github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/source"
	"github.com/wippyai/go-lua/program/static"
	staticrole "github.com/wippyai/go-lua/program/static/role"
)

func validatePublications(
	sourceView source.View,
	flowView authored.View,
	staticView static.View,
	bindings binding.Result,
	direct *directbinding.Result,
	tree *contextTree,
) error {
	publications := staticView.Publications()
	assigns := flowView.Storage().Assigns()
	writes := flowView.Storage().Writes()
	values := flowView.Values()
	cells := flowView.Storage().Cells()
	paths := direct.PublicationPaths()
	references := staticView.References()
	if paths.Count() != publications.Count() {
		return errors.New("program/flow/staticcheck: Publication path denominator mismatch")
	}
	for ordinal := 1; ordinal <= publications.Count(); ordinal++ {
		publication := keyspace.MakeTerm(keyspace.FamilyTypePublication, uint32(ordinal))
		at, ok := publications.At(ordinal - 1)
		if !ok || at != publication {
			return errors.New("program/flow/staticcheck: noncanonical Publication ordinal")
		}
		assign, pair, target, ok := publications.Get(publication)
		if !ok || keyspace.TermFamily(assign) != keyspace.FamilyAssign || keyspace.TermOrdinal(assign) == 0 ||
			keyspace.TermFamily(target) != keyspace.FamilyTypeRef || keyspace.TermOrdinal(target) == 0 ||
			keyspace.TermOrdinal(target) > uint32(staticView.References().Count()) {
			return errors.New("program/flow/staticcheck: malformed Publication row")
		}
		resolution, targetDeclaration, targetRoot, targetOK := references.Get(target)
		if !targetOK {
			return errors.New("program/flow/staticcheck: Publication target is unavailable")
		}
		canonicalTarget := false
		switch resolution {
		case static.TypeRefDeclaration:
			if keyspace.TermOrdinal(targetDeclaration) == 0 ||
				!staticrole.TypeReferenceTargetFamily(keyspace.TermFamily(targetDeclaration)) {
				return errors.New("program/flow/staticcheck: Publication target is unavailable")
			}
		case static.TypeRefCanonicalPath:
			canonicalTarget = true
		default:
			return errors.New("program/flow/staticcheck: Publication target is unavailable")
		}
		root, pathOwner, depth, pathOK := paths.Get(publication)
		if !pathOK || depth <= 0 || keyspace.TermFamily(root) != keyspace.FamilyCell || keyspace.TermOrdinal(root) == 0 {
			return errors.New("program/flow/staticcheck: Publication path is unavailable")
		}
		if canonicalTarget {
			if targetRoot != root {
				return errors.New("program/flow/staticcheck: Publication target root disagrees")
			}
			canonicalCount, canonicalOK := references.CanonicalCount(target)
			if !canonicalOK || canonicalCount != depth {
				return errors.New("program/flow/staticcheck: Publication target path depth disagrees")
			}
		}
		pathCursor, cursorOK := paths.PathCursor(publication)
		if !cursorOK {
			return errors.New("program/flow/staticcheck: Publication path cursor is unavailable")
		}
		for index := 0; index < depth; index++ {
			key, nextCursor, keyOK := pathCursor.Segment()
			if !keyOK || key == 0 {
				return errors.New("program/flow/staticcheck: Publication path segment is unavailable")
			}
			if canonicalTarget {
				canonical, canonicalOK := references.CanonicalAt(target, depth-1-index)
				if !canonicalOK || key != canonical {
					return errors.New("program/flow/staticcheck: Publication path key disagrees")
				}
			}
			pathCursor = nextCursor
		}
		if _, _, extra := pathCursor.Segment(); extra {
			return errors.New("program/flow/staticcheck: Publication path depth is noncanonical")
		}
		assignOwner, assignValues, assignOK := assigns.Get(assign)
		if !assignOK || assignOwner != pathOwner || keyspace.TermFamily(assignOwner) != keyspace.FamilyBody || keyspace.TermOrdinal(assignOwner) == 0 {
			return errors.New("program/flow/staticcheck: Publication Assign owner disagrees")
		}
		valuesOwner, _, valuesOK := values.Get(assignValues)
		if !valuesOK || valuesOwner != assignOwner {
			return errors.New("program/flow/staticcheck: Publication Values owner disagrees")
		}
		valuePosition, valuePositionOK := values.Position(assignValues, int(pair))
		if !valuePositionOK || valuePosition.NilFill || (valuePosition.Fixed == 0 && valuePosition.Tail == 0) ||
			(valuePosition.Fixed != 0 && valuePosition.Tail != 0) || valuePosition.TailOffset < 0 {
			return errors.New("program/flow/staticcheck: Publication pair is an adjusted nil fill")
		}
		write, writeOK := assigns.WriteAt(assign, int(pair))
		if !writeOK {
			return errors.New("program/flow/staticcheck: Publication pair has no Write")
		}
		writeAssign, writeTarget, writeRowOK := writes.Get(write)
		if !writeRowOK || writeAssign != assign || keyspace.TermFamily(writeTarget) != keyspace.FamilyLensExact || keyspace.TermOrdinal(writeTarget) == 0 {
			return errors.New("program/flow/staticcheck: Publication Write disagrees")
		}
		position, positionOK := sourcePosition(sourceView, assign)
		if !positionOK || position.Body != assignOwner {
			return errors.New("program/flow/staticcheck: Publication Assign Position disagrees")
		}
		point, pointOK := tree.pointForTerm(sourceView, assign)
		if !pointOK {
			return errors.New("program/flow/staticcheck: Publication Assign has no lexical point")
		}
		role, roleOK := bindings.Role(root)
		if !roleOK || !validPublicationRoot(cells, bindings, tree, root, role, point) {
			return errors.New("program/flow/staticcheck: Publication root is not visible")
		}
	}
	return nil
}

func validPublicationRoot(
	cells authored.Cells,
	bindings binding.Result,
	tree *contextTree,
	root keyspace.Term,
	role kind.CellRole,
	point int,
) bool {
	cellKind, body, key, ok := cells.Get(root)
	if !ok {
		return false
	}
	switch role {
	case kind.CellGlobal:
		return cellKind == authored.CellGlobal && body == 0 && key != 0
	case kind.CellLocal, kind.CellFormal, kind.CellFunctionVararg, kind.CellLoop, kind.CellCapture, kind.CellChunkVararg:
		return cellKind == authored.CellLocal && key == 0 && body != 0 && tree.cellVisible(point, root)
	default:
		return false
	}
}

func sourcePosition(view source.View, term keyspace.Term) (source.Position, bool) {
	body, _, cursor, ok := view.Index().Position(term)
	if !ok || keyspace.TermFamily(body) != keyspace.FamilyBody || cursor < 0 {
		return source.Position{}, false
	}
	root, rootOK := view.Index().Root(term)
	if !rootOK || root == 0 {
		return source.Position{}, false
	}
	return source.Position{Term: term, Root: root, Body: body, Cursor: uint32(cursor)}, true
}
