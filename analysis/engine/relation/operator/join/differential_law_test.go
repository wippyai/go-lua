package join_test

import (
	"testing"

	physicaljoin "github.com/wippyai/go-lua/analysis/engine/relation/operator/join"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/database"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/read"
	"github.com/wippyai/go-lua/analysis/engine/relation/tuple"
	testfixture "github.com/wippyai/go-lua/analysis/engine/testdata/relationfixture"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	bindingpkg "github.com/wippyai/go-lua/analysis/relation/semantic/binding"
	"github.com/wippyai/go-lua/internal/relationoracle"
)

// inputSourceAtRoot replays one sealed Input producer against a public
// committed root. It is intentionally local to this law file because the
// fixture has no delta-to-Batch adapter: the test compares the immutable
// root replay with the full root rather than inventing one.
func inputSourceAtRoot(t testing.TB, fixture testfixture.Fixture, left bool, root database.Version) tuple.Batch {
	t.Helper()
	mounted := fixture.Mounted()
	var node arrangement.Node
	var reader read.Reader
	var ok bool
	var scope witness.Scope
	if left {
		node, ok = fixture.LeftInputNode()
		reader, ok = fixture.ReaderLeftPayload(root)
		scope, _ = fixture.OverlapScopes()
	} else {
		node, ok = fixture.RightInputNode()
		reader, ok = fixture.ReaderRightPayload(root)
		_, scope = fixture.OverlapScopes()
	}
	if !ok || !node.Available() || !reader.Available() {
		t.Fatal("sealed root input source")
	}
	inputBinding, bindingOK := node.Input()
	rangeBinding, rangeOK := inputBinding.Range()
	if !bindingOK || !inputBinding.Available() || !rangeOK || !rangeBinding.Available() {
		t.Fatal("sealed root input binding")
	}
	values := make([]tuple.Tuple, 0, 2)
	completed, valid := reader.Scan(func(row read.Row) bool {
		value, valueOK := tuple.Input(mounted, reader, row)
		if !valueOK {
			return false
		}
		if len(values) == 0 {
			scope = value.Scope()
		} else if !scope.Same(value.Scope()) {
			return false
		}
		values = append(values, value)
		return true
	})
	if !completed || !valid || !scope.ValidFor(mounted.RuntimeFence()) {
		t.Fatalf("root input scan=(%v,%v)", completed, valid)
	}
	batch, batchOK := tuple.NewRangeBatch(mounted, rangeBinding, scope, values, bindingpkg.DenominatorWitness{})
	if !batchOK || !batch.ValidFor(mounted) {
		t.Fatal("root input range batch")
	}
	return batch
}

// oracleBatch is deliberately a test projection, not a second physical
// implementation.  The source tuple has already redeemed the mounted row,
// scope, lineage, cells, and value identities; the neutral oracle receives
// only those immutable logical facts.
func oracleBatch(t testing.TB, relation model.RelationID, batch tuple.Batch, label string) relationoracle.Relation {
	t.Helper()
	scopeID, ok := identity.DeriveContentID("analysis/engine/relation/operator/join/law/scope/v1", []byte(label))
	if !ok {
		t.Fatal("oracle scope identity")
	}
	scope, ok := relationoracle.NewScope(scopeID)
	if !ok {
		t.Fatal("oracle scope")
	}
	rows := make([]relationoracle.Row, 0, batch.Len())
	for index := 0; index < batch.Len(); index++ {
		value, valueOK := batch.At(index)
		if !valueOK || value.SourceLen() != 1 {
			t.Fatalf("oracle source tuple %d", index)
		}
		rowID, sourceOK := value.SourceAt(0)
		if !sourceOK || rowID.Relation() != relation {
			t.Fatalf("oracle source row %d", index)
		}
		cells := make([]relationoracle.Cell, 0, value.Len())
		for cellIndex := 0; cellIndex < value.Len(); cellIndex++ {
			cell, cellOK := value.At(cellIndex)
			if !cellOK || cell.Source() != 0 || !cell.Presence().Available() {
				t.Fatalf("oracle source cell %d/%d", index, cellIndex)
			}
			var oracleCell relationoracle.Cell
			if cell.Presence().Is(model.Present) || cell.Presence().Is(model.AuthenticatedOpaque) {
				if !cell.Value().Available() {
					t.Fatalf("oracle present value %d/%d", index, cellIndex)
				}
				oracleValue, valueOK := relationoracle.NewValueToken(cell.Type(), cell.Value().Opaque())
				if !valueOK {
					t.Fatalf("oracle value %d/%d", index, cellIndex)
				}
				if cell.Presence().Is(model.Present) {
					oracleCell, valueOK = relationoracle.PresentCell(cell.Column(), cell.Type(), oracleValue)
				} else {
					oracleCell, valueOK = relationoracle.OpaqueCell(cell.Column(), cell.Type(), oracleValue)
				}
				if !valueOK {
					t.Fatalf("oracle populated cell %d/%d", index, cellIndex)
				}
			} else {
				var valueOK bool
				switch {
				case cell.Presence().Is(model.ProvenAbsent):
					oracleCell, valueOK = relationoracle.AbsentCell(cell.Column(), cell.Type())
				case cell.Presence().Is(model.UnprovenMissing):
					oracleCell, valueOK = relationoracle.MissingCell(cell.Column(), cell.Type())
				default:
					t.Fatalf("oracle unsupported presence %d/%d: %v", index, cellIndex, cell.Presence().Kind())
				}
				if !valueOK {
					t.Fatalf("oracle empty cell %d/%d", index, cellIndex)
				}
			}
			cells = append(cells, oracleCell)
		}
		var row relationoracle.Row
		var rowValid bool
		if lineage := value.Lineage(); lineage.Available() {
			row, rowValid = relationoracle.NewRow(rowID, scope, cells, lineage)
		} else {
			row, rowValid = relationoracle.NewRow(rowID, scope, cells)
		}
		if !rowValid {
			t.Fatalf("oracle row %d", index)
		}
		rows = append(rows, row)
	}
	relationValue, relationOK := relationoracle.NewRelation(relation, rows)
	if !relationOK {
		t.Fatal("oracle relation")
	}
	return relationValue
}

func oracleJoin(t testing.TB, fixture testfixture.Fixture, left, right tuple.Batch) relationoracle.Relation {
	t.Helper()
	leftRelation := oracleBatch(t, fixture.RelationLeft(), left, "left")
	rightRelation := oracleBatch(t, fixture.RelationRight(), right, "right")
	leftColumns := fixture.PayloadColumnsLeft()
	rightColumns := fixture.PayloadColumnsRight()
	typeID := firstTupleType(t, left)
	entry, ok := relationoracle.NewAlgebraEntry(typeID, relationoracle.IdentityAlgebra{})
	if !ok {
		t.Fatal("oracle algebra entry")
	}
	registry, ok := relationoracle.NewAlgebraRegistry([]relationoracle.AlgebraEntry{entry})
	if !ok {
		t.Fatal("oracle algebra registry")
	}
	joined := relationoracle.Join(leftRelation, rightRelation, relationoracle.NewJoinSpecWithLineage(fixture.RelationLeft(), []model.ColumnID{leftColumns[0], leftColumns[1]}, []model.ColumnID{rightColumns[0], rightColumns[1]}, nil, nil, registry, relationoracle.ExactScope{}, fixtureLineage(t, fixture)))
	return joined
}

// firstTupleType keeps the oracle registry tied to the sealed fixture's
// declared key type.  The fixture has one scalar type for both key columns.
func firstTupleType(t testing.TB, batch tuple.Batch) model.TypeID {
	t.Helper()
	value, ok := batch.At(0)
	if !ok {
		t.Fatal("oracle type tuple")
	}
	cell, ok := value.At(0)
	if !ok || !cell.Type().Available() {
		t.Fatal("oracle type cell")
	}
	return cell.Type()
}

func fixtureLineage(t testing.TB, fixture testfixture.Fixture) relationoracle.LineageAlgebra {
	t.Helper()
	authority, ok := fixture.Mounted().Lineage()
	if !ok || authority == nil {
		t.Fatal("fixture lineage authority")
	}
	return authority
}

// assertOracleJoin compares the logical result, not physical row ordinals.
// The oracle keeps the oriented left RowID as its one representable identity,
// while the physical tuple preserves both owner-issued source RowIDs. Columns,
// typed values, presence, and lineage are the common semantic boundary.
func assertOracleJoin(t testing.TB, physical tuple.Batch, expected relationoracle.Relation) {
	t.Helper()
	if !expected.Available() {
		t.Fatal("oracle join refused unexpectedly")
	}
	rows := expected.Rows()
	if physical.Len() != len(rows) {
		t.Fatalf("join cardinality physical=%d oracle=%d", physical.Len(), len(rows))
	}
	used := make([]bool, len(rows))
	for physicalIndex := 0; physicalIndex < physical.Len(); physicalIndex++ {
		value, valueOK := physical.At(physicalIndex)
		if !valueOK {
			t.Fatalf("physical joined tuple %d", physicalIndex)
		}
		found := -1
		for oracleIndex, row := range rows {
			if used[oracleIndex] || !oracleRowMatchesTuple(row, value) {
				continue
			}
			found = oracleIndex
			break
		}
		if found < 0 {
			t.Fatalf("physical tuple %d has no oracle row", physicalIndex)
		}
		used[found] = true
		if rows[found].Lineage() != value.Lineage() {
			t.Fatalf("joined lineage physical=%v oracle=%v", value.Lineage(), rows[found].Lineage())
		}
	}
}

func oracleRowMatchesTuple(row relationoracle.Row, value tuple.Tuple) bool {
	if !row.Available() || !value.Available() || len(row.Cells()) != value.Len() {
		return false
	}
	for _, oracleCell := range row.Cells() {
		physicalCell, ok := value.CellFor(oracleCell.Column())
		if !ok || physicalCell.Type() != oracleCell.Type() || physicalCell.Presence().Kind() != oracleCell.Presence().Kind() {
			return false
		}
		oracleValue, oracleHasValue := oracleCell.Value()
		physicalHasValue := physicalCell.Value().Available()
		if oracleHasValue != physicalHasValue {
			return false
		}
		if oracleHasValue && oracleValue.Content() != physicalCell.Value().Opaque() {
			return false
		}
	}
	return true
}

func TestTupleJoinDifferentialPositiveAndNoMatch(t *testing.T) {
	fixture := testfixture.New(t, 0x71)
	left := inputSource(t, fixture, true)
	right := inputSource(t, fixture, false)
	binding := bindingFor(t, fixture)
	physical, ok := physicaljoin.Join(binding, fixture.Mounted(), fixture.Geometry(), left, right)
	if !ok || !physical.ValidFor(fixture.Mounted()) {
		t.Fatal("physical positive join")
	}
	assertOracleJoin(t, physical, oracleJoin(t, fixture, left, right))

	matchingLeft, _ := matchingPair(t, fixture.Mounted(), binding, left, right)
	var unmatchedRight tuple.Tuple
	for index := 0; index < right.Len(); index++ {
		candidate, candidateOK := right.At(index)
		if candidateOK && !correspondenceMatch(fixture.Mounted(), binding, matchingLeft, candidate) {
			unmatchedRight = candidate
			break
		}
	}
	if !unmatchedRight.Available() {
		t.Fatal("fixture unmatched right tuple")
	}
	leftOnly := exactRangeBatch(t, fixture.Mounted(), left, []tuple.Tuple{matchingLeft})
	rightOnly := exactRangeBatch(t, fixture.Mounted(), right, []tuple.Tuple{unmatchedRight})
	physical, ok = physicaljoin.Join(binding, fixture.Mounted(), fixture.Geometry(), leftOnly, rightOnly)
	if !ok || physical.Len() != 0 {
		t.Fatal("physical no-match join")
	}
	assertOracleJoin(t, physical, oracleJoin(t, fixture, leftOnly, rightOnly))
}

func TestTupleJoinDifferentialScopeDisjointAndMalformedRefusal(t *testing.T) {
	fixture := testfixture.New(t, 0x71)
	left := inputSource(t, fixture, true)
	right := inputSource(t, fixture, false)
	_, disjoint := fixture.DisjointScopes()
	empty, ok := tuple.NewRangeBatch(fixture.Mounted(), right.Range(), disjoint, []tuple.Tuple{}, bindingpkg.DenominatorWitness{})
	if !ok {
		t.Fatal("disjoint empty range")
	}
	physical, ok := physicaljoin.Join(bindingFor(t, fixture), fixture.Mounted(), fixture.Geometry(), left, empty)
	if !ok || !physical.ValidFor(fixture.Mounted()) || physical.Len() != 0 {
		t.Fatal("physical disjoint join")
	}
	assertOracleJoin(t, physical, oracleJoin(t, fixture, left, empty))

	var unavailable tuple.Batch
	if physical, ok := physicaljoin.Join(bindingFor(t, fixture), fixture.Mounted(), fixture.Geometry(), unavailable, right); ok || physical.Available() {
		t.Fatal("physical malformed batch was accepted")
	}
	if physical, ok := physicaljoin.Join(arrangement.JoinBinding{}, fixture.Mounted(), fixture.Geometry(), left, right); ok || physical.Available() {
		t.Fatal("physical unavailable binding was accepted")
	}
}

func TestTupleJoinDifferentialPermutationAndPublishedRootReplay(t *testing.T) {
	fixture := testfixture.New(t, 0x71)
	left := inputSource(t, fixture, true)
	right := inputSource(t, fixture, false)
	binding := bindingFor(t, fixture)
	forward, ok := physicaljoin.Join(binding, fixture.Mounted(), fixture.Geometry(), left, right)
	if !ok {
		t.Fatal("forward physical join")
	}

	leftValues, rightValues := left.Tuples(), right.Tuples()
	for first, last := 0, len(leftValues)-1; first < last; first, last = first+1, last-1 {
		leftValues[first], leftValues[last] = leftValues[last], leftValues[first]
	}
	for first, last := 0, len(rightValues)-1; first < last; first, last = first+1, last-1 {
		rightValues[first], rightValues[last] = rightValues[last], rightValues[first]
	}
	permutedLeft := exactRangeBatch(t, fixture.Mounted(), left, leftValues)
	permutedRight := exactRangeBatch(t, fixture.Mounted(), right, rightValues)
	permuted, ok := physicaljoin.Join(binding, fixture.Mounted(), fixture.Geometry(), permutedLeft, permutedRight)
	if !ok {
		t.Fatal("permuted physical join")
	}
	assertOracleJoin(t, forward, oracleJoin(t, fixture, left, right))
	assertOracleJoin(t, permuted, oracleJoin(t, fixture, permutedLeft, permutedRight))
	if permuted.Len() != forward.Len() {
		t.Fatal("permutation changed join cardinality")
	}

	leftDelta, leftDeltaOK := fixture.BaseToLeftDelta()
	rightDelta, rightDeltaOK := fixture.LeftToBothDelta()
	if !leftDeltaOK || !rightDeltaOK || !leftDelta.Next().Same(fixture.LeftRoot()) || !rightDelta.Next().Same(fixture.BothRoot()) {
		t.Fatal("published fixture roots are not a valid delta chain")
	}
	// The public fixture exposes immutable committed roots and deltas, but no
	// delta-to-Batch operator. Replay the exact post-left and post-right roots
	// through the sealed Input boundary and compare against the full root; do
	// not fabricate a delta adapter in this law.
	leftAtPostLeft := inputSourceAtRoot(t, fixture, true, fixture.LeftRoot())
	rightAtPostRight := inputSourceAtRoot(t, fixture, false, fixture.BothRoot())
	replayed, ok := physicaljoin.Join(binding, fixture.Mounted(), fixture.Geometry(), leftAtPostLeft, rightAtPostRight)
	if !ok || !replayed.ValidFor(fixture.Mounted()) || replayed.Len() != forward.Len() {
		t.Fatal("published-root replay changed join")
	}
	assertOracleJoin(t, replayed, oracleJoin(t, fixture, leftAtPostLeft, rightAtPostRight))
	if replayed.Len() != forward.Len() {
		t.Fatal("delta/root replay cardinality changed")
	}
}
