package derivation

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
)

// This is a sealed-shape law rather than a fixture-local evaluator test. It
// proves the exact production grammar Expand(Join(C,Select(X))) is C-major:
// the first watcher is the sole C anchor and the second watcher is the one
// bounded selected sibling. Reversing occurrence order is refused because it
// would make emission order ambiguous.
func TestExpandReplaySealsCmajorCompoundShape(t *testing.T) {
	shape := newReplayShape(t)
	cPath, cOK := shape.path(0, true)
	xPath, xOK := shape.path(1, false)
	if !cOK || !xOK {
		t.Fatal("compound paths")
	}
	cShape, cJoin, cShapeOK := expandWatcherShape(cPath, 0, shape.contract)
	xShape, xJoin, xShapeOK := expandWatcherShape(xPath, 0, shape.contract)
	if !cShapeOK || !xShapeOK || cShape != 2 || xShape != 2 || cJoin != shape.joinNode || xJoin != shape.joinNode {
		t.Fatalf("compound shape c=(%d,%v,%t) x=(%d,%v,%t)", cShape, cJoin, cShapeOK, xShape, xJoin, xShapeOK)
	}
	cWatcher, cWatcherOK := newExpandWatcher(cPath.Occurrence(), 0, shape.expandNode, cPath.leaf, shape.cRange)
	xWatcher, xWatcherOK := newExpandWatcher(xPath.Occurrence(), 0, shape.expandNode, xPath.leaf, shape.xRange)
	if !cWatcherOK || !xWatcherOK {
		t.Fatal("watchers")
	}
	anchor, anchorOK := newExpandAnchor(cWatcher.PathOccurrence(), cWatcher.leaf, cWatcher.range_)
	if !anchorOK {
		t.Fatal("anchor")
	}
	replay, replayOK := newExpandReplay(cWatcher.PathOccurrence(), anchor, []ExpandWatcher{cWatcher, xWatcher})
	if !replayOK || !replay.Available() || replay.WatcherCount() != 2 || replay.EmitOccurrence() != cWatcher.PathOccurrence() {
		t.Fatal("compound replay")
	}
	if reverse, reverseOK := newExpandReplay(xWatcher.PathOccurrence(), anchor, []ExpandWatcher{xWatcher, cWatcher}); reverseOK || reverse.Available() {
		t.Fatal("non-canonical watcher order accepted")
	}
	wrongAnchor, wrongAnchorOK := newExpandAnchor(cWatcher.PathOccurrence(), xWatcher.leaf, xWatcher.range_)
	if !wrongAnchorOK {
		t.Fatal("wrong anchor specimen")
	}
	if mismatched, mismatchedOK := newExpandReplay(cWatcher.PathOccurrence(), wrongAnchor, []ExpandWatcher{cWatcher, xWatcher}); mismatchedOK || mismatched.Available() {
		t.Fatal("anchor access was not tied to its watcher")
	}
}

type replayShape struct {
	contract   model.ExpandContract
	expandNode identity.ContentID
	joinNode   identity.ContentID
	cRelation  model.RelationID
	xRelation  model.RelationID
	cColumn    model.ColumnID
	xColumn    model.ColumnID
	rKeyID     model.KeyID
	cRange     sealedAccess
	xRange     sealedAccess
}

func newReplayShape(t *testing.T) replayShape {
	t.Helper()
	owner, ok := model.IssueOwnerID(identity.ContentID{1})
	if !ok {
		t.Fatal("owner")
	}
	cRelation, ok := model.IssueRelationID(owner, identity.ContentID{2})
	if !ok {
		t.Fatal("candidate relation")
	}
	pRelation, ok := model.IssueRelationID(owner, identity.ContentID{3})
	if !ok {
		t.Fatal("publisher relation")
	}
	rRelation, ok := model.IssueRelationID(owner, identity.ContentID{4})
	if !ok {
		t.Fatal("reader relation")
	}
	xRelation, ok := model.IssueRelationID(owner, identity.ContentID{5})
	if !ok {
		t.Fatal("sibling relation")
	}
	cColumn, ok := model.IssueColumnID(cRelation, identity.ContentID{6})
	if !ok {
		t.Fatal("candidate column")
	}
	xColumn, ok := model.IssueColumnID(xRelation, identity.ContentID{7})
	if !ok {
		t.Fatal("sibling column")
	}
	rKey, ok := model.IssueColumnID(rRelation, identity.ContentID{8})
	if !ok {
		t.Fatal("reader key")
	}
	rKeyID, ok := model.IssueKeyID(rRelation, identity.ContentID{9})
	if !ok {
		t.Fatal("reader key id")
	}
	contract := model.DefineExpandContract(cRelation, pRelation, rRelation, rKey, pRelation).WithScope(mustScope(t, owner))
	return replayShape{
		contract: contract, expandNode: identity.ContentID{10}, joinNode: identity.ContentID{11}, rKeyID: rKeyID,
		cRelation: cRelation, xRelation: xRelation, cColumn: cColumn, xColumn: xColumn,
		cRange: mustSealedAccess(t, cRelation, nil, identity.ContentID{12}),
		xRange: mustSealedAccess(t, xRelation, nil, identity.ContentID{13}),
	}
}

func mustScope(t *testing.T, owner model.OwnerID) model.ScopeID {
	t.Helper()
	value, ok := model.IssueScopeID(owner, identity.ContentID{14})
	if !ok {
		t.Fatal("scope")
	}
	return value
}

func mustSealedAccess(t *testing.T, relation model.RelationID, columns []model.ColumnID, physical identity.ContentID) sealedAccess {
	t.Helper()
	access, ok := NewAccess(relation, model.KeyID{}, columns)
	if !ok {
		t.Fatal("access")
	}
	return sealedAccess{access: access, physical: physical}
}

func (shape replayShape) path(occurrence uint32, candidate bool) (Path, bool) {
	leafRelation, column, leafPhysical := shape.xRelation, shape.xColumn, identity.ContentID{21}
	frames := []Frame{
		{kind: algebra.KindExpand, orientation: OrientationNone, node: shape.expandNode,
			siblings: []sealedAccess{
				mustSealedAccessNoTest(shape.cRelation, []model.ColumnID{shape.cColumn}, identity.ContentID{22}),
				mustSealedAccessNoTest(shape.contract.Reader(), []model.ColumnID{shape.contract.Key()}, identity.ContentID{23}),
				{access: mustAccessNoTest(shape.contract.Reader(), shape.rKeyID, nil), physical: identity.ContentID{24}},
			}, expandContract: shape.contract, expandEvidence: identity.ContentID{25}},
	}
	if candidate {
		leafRelation, column, leafPhysical = shape.cRelation, shape.cColumn, identity.ContentID{20}
		frames = append(frames, Frame{kind: algebra.KindJoin, orientation: OrientationLeft, node: shape.joinNode, siblings: []sealedAccess{mustSealedAccessNoTest(shape.xRelation, []model.ColumnID{shape.xColumn}, identity.ContentID{26})}, columns: []model.ColumnID{shape.cColumn}})
	} else {
		frames = append(frames, Frame{kind: algebra.KindJoin, orientation: OrientationRight, node: shape.joinNode, siblings: []sealedAccess{mustSealedAccessNoTest(shape.cRelation, []model.ColumnID{shape.cColumn}, identity.ContentID{27})}, columns: []model.ColumnID{shape.xColumn}})
		scope := shape.contract.Scope()
		frames = append(frames, Frame{kind: algebra.KindSelect, orientation: OrientationNone, node: identity.ContentID{28}, siblings: []sealedAccess{}, scope: scope})
	}
	leaf := mustSealedAccessNoTest(leafRelation, []model.ColumnID{column}, leafPhysical)
	path := Path{root: mustExpressionIDNoTest(), occurrence: occurrence, node: identity.ContentID{byte(30 + occurrence)}, leafRelation: leafRelation, readColumns: []model.ColumnID{column}, leaf: leaf, frames: frames}
	path.digest, _ = digestPath(path)
	return path, path.Available()
}

func mustExpressionIDNoTest() model.ExpressionID {
	owner, _ := model.IssueOwnerID(identity.ContentID{40})
	value, _ := model.IssueExpressionID(owner, identity.ContentID{41})
	return value
}

func mustSealedAccessNoTest(relation model.RelationID, columns []model.ColumnID, physical identity.ContentID) sealedAccess {
	access, _ := NewAccess(relation, model.KeyID{}, columns)
	return sealedAccess{access: access, physical: physical}
}

func mustAccessNoTest(relation model.RelationID, key model.KeyID, columns []model.ColumnID) Access {
	access, _ := NewAccess(relation, key, columns)
	return access
}
