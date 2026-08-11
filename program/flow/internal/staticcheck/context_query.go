package staticcheck

import (
	"errors"

	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/source"
)

func contextPosition(view source.View, term keyspace.Term) (source.Position, bool) {
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

func (tree *contextTree) pointAt(body keyspace.Term, gap uint32) (int, bool) {
	if tree == nil || keyspace.TermFamily(body) != keyspace.FamilyBody || keyspace.TermOrdinal(body) == 0 || int(keyspace.TermOrdinal(body)) >= len(tree.bodies) {
		return 0, false
	}
	node := tree.bodies[keyspace.TermOrdinal(body)]
	if node.gapCount == 0 || uint64(gap) >= uint64(node.gapCount) {
		return 0, false
	}
	return node.gapStart + int(gap), true
}

func (tree *contextTree) pointBody(point int) (keyspace.Term, bool) {
	if tree == nil || point <= 0 || point >= len(tree.points) {
		return 0, false
	}
	return tree.points[point].body, true
}

func (tree *contextTree) pointForTerm(view source.View, term keyspace.Term) (int, bool) {
	body, _, cursor, ok := view.Index().Position(term)
	if !ok || cursor < 0 {
		return 0, false
	}
	return tree.pointAt(body, uint32(cursor))
}

func (tree *contextTree) addCell(point int, cell keyspace.Term) error {
	if tree == nil || point <= 0 || point >= len(tree.points) || keyspace.TermFamily(cell) != keyspace.FamilyCell || keyspace.TermOrdinal(cell) == 0 || int(keyspace.TermOrdinal(cell)) >= len(tree.cellPoint) {
		return errors.New("program/flow/staticcheck: invalid Cell introduction")
	}
	ordinal := int(keyspace.TermOrdinal(cell))
	if tree.cellPoint[ordinal] != 0 {
		return errors.New("program/flow/staticcheck: duplicate Cell introduction")
	}
	tree.cellPoint[ordinal] = point
	return nil
}

func (tree *contextTree) addParam(point int, param keyspace.Term) error {
	if tree == nil || point <= 0 || point >= len(tree.points) || keyspace.TermFamily(param) != keyspace.FamilyTypeParam || keyspace.TermOrdinal(param) == 0 || int(keyspace.TermOrdinal(param)) >= len(tree.generic) {
		return errors.New("program/flow/staticcheck: invalid generic introduction")
	}
	ordinal := int(keyspace.TermOrdinal(param))
	if tree.generic[ordinal] != 0 {
		return errors.New("program/flow/staticcheck: duplicate generic introduction")
	}
	tree.generic[ordinal] = point
	return nil
}

func (tree *contextTree) cellVisible(point int, cell keyspace.Term) bool {
	if tree == nil || point <= 0 || point >= len(tree.points) || keyspace.TermFamily(cell) != keyspace.FamilyCell || keyspace.TermOrdinal(cell) == 0 || int(keyspace.TermOrdinal(cell)) >= len(tree.cellPoint) {
		return false
	}
	introduction := tree.cellPoint[keyspace.TermOrdinal(cell)]
	return introduction != 0 && tree.tin[introduction] <= tree.tin[point] && tree.tout[point] <= tree.tout[introduction]
}

func (tree *contextTree) paramVisible(point int, param keyspace.Term) bool {
	if tree == nil || point <= 0 || point >= len(tree.points) || keyspace.TermFamily(param) != keyspace.FamilyTypeParam || keyspace.TermOrdinal(param) == 0 || int(keyspace.TermOrdinal(param)) >= len(tree.generic) {
		return false
	}
	introduction := tree.generic[keyspace.TermOrdinal(param)]
	return introduction != 0 && tree.tin[introduction] <= tree.tin[point] && tree.tout[point] <= tree.tout[introduction]
}
