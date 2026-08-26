package delta

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/relation/apply"
	inputop "github.com/wippyai/go-lua/analysis/engine/relation/operator/input"
	joinop "github.com/wippyai/go-lua/analysis/engine/relation/operator/join"
	mergeop "github.com/wippyai/go-lua/analysis/engine/relation/operator/merge"
	"github.com/wippyai/go-lua/analysis/engine/relation/solve/fixpoint"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/read"
	"github.com/wippyai/go-lua/analysis/engine/relation/tuple"
	"github.com/wippyai/go-lua/analysis/engine/testdata/relationfixture"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement/derivation"
	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
)

func TestApplyValuePreservesAuthenticatedEmptyExtent(t *testing.T) {
	node, ok := identity.DeriveContentID("analysis/engine/relation/eval/delta/empty-apply-law/v1", []byte("node"))
	if !ok {
		t.Fatal("apply node identity")
	}
	value, ok := applyValue(node, []apply.Results{})
	if !ok || !value.availableNoMount() || value.applications == nil || len(value.applications) != 0 {
		t.Fatal("authenticated empty Apply extent collapsed into unavailable nil")
	}
}

func joinRoot(t testing.TB, fixture testfixture.Fixture) (arrangement.Node, derivation.Plan, bool) {
	t.Helper()
	execution := fixture.Mounted().Arrangement().Execution()
	root, ok := fixture.JoinNode()
	if !ok || !root.Available() || root.Kind() != algebra.KindJoin {
		return arrangement.Node{}, derivation.Plan{}, false
	}
	for _, expression := range execution.ExpressionIDs() {
		candidate, candidateOK := execution.Entry(expression)
		if !candidateOK || candidate.Digest() != root.Digest() {
			continue
		}
		paths, pathsOK := execution.Derivation(expression)
		if pathsOK {
			return root, paths, true
		}
	}
	return arrangement.Node{}, derivation.Plan{}, false
}

func fullJoinRows(t testing.TB, fixture testfixture.Fixture) int {
	t.Helper()
	root, ok := fixture.JoinNode()
	if !ok || !root.Available() || root.Kind() != algebra.KindJoin {
		t.Fatal("full join root")
	}
	children := root.Children()
	if len(children) != 2 {
		t.Fatal("full join children")
	}
	binding, ok := root.Join()
	if !ok {
		t.Fatal("full join binding")
	}
	values := make([][]tuple.Batch, len(children))
	for index, child := range children {
		input, inputOK := child.Input()
		if !inputOK {
			t.Fatal("full join input")
		}
		reader, readerOK := read.Bind(fixture.BothRoot(), input.Values(), fixture.Geometry(), fixture.Scratch())
		if !readerOK {
			t.Fatal("full join reader")
		}
		values[index], ok = inputop.Execute(input, fixture.Mounted(), reader)
		if !ok {
			t.Fatal("full join input execution")
		}
	}
	rows := 0
	for _, left := range values[0] {
		for _, right := range values[1] {
			batch, batchOK := joinop.Join(binding, fixture.Mounted(), fixture.Geometry(), left, right)
			if !batchOK {
				t.Fatal("full join execution")
			}
			rows += batch.Len()
		}
	}
	return rows
}

// A right-side positive insertion reaches the changed occurrence exactly once.
// Derive promotes the sealed correspondence vector to a physical index while
// retaining its unkeyed logical Access, so the stable sibling is redeemed by
// Lookup rather than a full Reader/index scan.
func TestLaterJoinPositiveInsertMatchesIndexedSibling(t *testing.T) {
	fixture := testfixture.New(t, 0xD7)
	dbDelta, ok := fixture.LeftToBothDelta()
	if !ok {
		t.Fatal("right publication delta")
	}
	root, ok := fixpoint.Later(dbDelta)
	if !ok {
		t.Fatal("later root")
	}
	rootNode, paths, ok := joinRoot(t, fixture)
	if !ok {
		t.Fatal("sealed join root")
	}
	differential, ok := New(fixture.Mounted(), root, fixture.Geometry())
	if !ok || !differential.Available() {
		t.Fatal("later session")
	}
	activePaths, gotRows := 0, 0
	for index := 0; index < paths.Len(); index++ {
		path, pathOK := paths.PathAt(index)
		if !pathOK {
			t.Fatal("sealed join path")
		}
		value, active, executeOK := differential.executePath(rootNode, path, paths)
		if !executeOK {
			t.Fatalf("join path %d refused", index)
		}
		if !active {
			continue
		}
		activePaths++
		for _, batch := range value.batches {
			gotRows += batch.Len()
		}
	}
	wantRows := fullJoinRows(t, fixture)
	if activePaths != 1 || gotRows != wantRows || gotRows != 1 {
		t.Fatalf("later/full join active paths/rows=%d/%d rows=%d", activePaths, gotRows, wantRows)
	}
}

func TestLaterJoinBothSidesUseDisjointPivotEpochs(t *testing.T) {
	fixture := testfixture.New(t, 0xD8)
	dbDelta, ok := fixture.LeftToBothDelta()
	if !ok {
		t.Fatal("right publication delta")
	}
	later, ok := fixpoint.Later(dbDelta)
	if !ok {
		t.Fatal("later root")
	}
	session, ok := New(fixture.Mounted(), later, fixture.Geometry())
	if !ok || !session.Available() {
		t.Fatal("later session")
	}
	_, paths, ok := joinRoot(t, fixture)
	if !ok {
		t.Fatal("sealed join root")
	}
	var left, right derivation.Frame
	for index := 0; index < paths.Len(); index++ {
		path, pathOK := paths.PathAt(index)
		if !pathOK {
			t.Fatal("sealed join path")
		}
		for frameIndex := 0; frameIndex < path.FrameCount(); frameIndex++ {
			frame, frameOK := path.FrameAt(frameIndex)
			if !frameOK || frame.Kind() != algebra.KindJoin {
				continue
			}
			if frame.Orientation() == derivation.OrientationLeft {
				left = frame
			} else if frame.Orientation() == derivation.OrientationRight {
				right = frame
			}
		}
	}
	leftEpoch, leftOK := session.stableEpoch(left, 0)
	rightEpoch, rightOK := session.stableEpoch(right, 0)
	if !leftOK || !rightOK || !leftEpoch.Same(dbDelta.Base()) || !rightEpoch.Same(dbDelta.Next()) {
		t.Fatalf("join pivot epochs left=%t/%t right=%t/%t", leftOK, leftEpoch.Same(dbDelta.Base()), rightOK, rightEpoch.Same(dbDelta.Next()))
	}
	// The left expansion is (ΔL, BaseR), while the right expansion is
	// (NextL, ΔR). Their changed/stable sides are disjoint by construction;
	// a ΔL⋈ΔR term cannot be emitted twice by path enumeration.
}

func TestLaterIrrelevantChangedRelationReturnsEmptyInput(t *testing.T) {
	fixture := testfixture.New(t, 0xDA)
	dbDelta, ok := fixture.LeftToBothDelta()
	if !ok {
		t.Fatal("right publication delta")
	}
	later, ok := fixpoint.Later(dbDelta)
	if !ok {
		t.Fatal("later root")
	}
	session, ok := New(fixture.Mounted(), later, fixture.Geometry())
	if !ok || !session.Available() {
		t.Fatal("later session")
	}
	entry, ok := fixture.Mounted().Arrangement().Execution().Dependency(fixture.DependencyLeft())
	if !ok {
		t.Fatal("left entry")
	}
	result, ok := session.Evaluate(entry)
	if !ok || !result.Available() || result.Kind() != algebra.KindInput || len(result.Batches()) != 0 || len(result.Applications()) != 0 || len(result.Settlements()) != 0 {
		t.Fatalf("irrelevant delta result ok=%t available=%t kind=%v batches=%d applications=%d settlements=%d", ok, result.Available(), result.Kind(), len(result.Batches()), len(result.Applications()), len(result.Settlements()))
	}
}

func TestLaterForeignEntryRefused(t *testing.T) {
	fixture := testfixture.New(t, 0xDB)
	foreign := testfixture.New(t, 0xDC)
	dbDelta, ok := fixture.LeftToBothDelta()
	if !ok {
		t.Fatal("right publication delta")
	}
	later, ok := fixpoint.Later(dbDelta)
	if !ok {
		t.Fatal("later root")
	}
	session, ok := New(fixture.Mounted(), later, fixture.Geometry())
	if !ok || !session.Available() {
		t.Fatal("later session")
	}
	entry, ok := foreign.Mounted().Arrangement().Execution().Dependency(foreign.DependencyLeft())
	if !ok {
		t.Fatal("foreign entry")
	}
	if result, evaluateOK := session.Evaluate(entry); evaluateOK || result.Available() {
		t.Fatal("foreign schedule entry accepted")
	}
}

func TestLaterSessionRefusesFullRoot(t *testing.T) {
	fixture := testfixture.New(t, 0xDD)
	full, ok := fixpoint.Full(fixture.BothRoot())
	if !ok {
		t.Fatal("full root")
	}
	if session, sessionOK := New(fixture.Mounted(), full, fixture.Geometry()); sessionOK || session.Available() {
		t.Fatal("full root entered delta evaluator")
	}
}

// A relation-only Merge redeems each authored child through its sealed
// physical key vector, but the occurrence pivot emits the affected fold only
// once. This compares the Later path directly with the full Merge operator;
// no output deduplication is used by the differential evaluator.
func TestLaterMergeAffectedMatchesFullFoldOnce(t *testing.T) {
	fixture := testfixture.New(t, 0xDE)
	dbDelta, ok := fixture.BaseToLeftDelta()
	if !ok {
		t.Fatal("merge delta")
	}
	later, ok := fixpoint.Later(dbDelta)
	if !ok {
		t.Fatal("later root")
	}
	session, ok := New(fixture.Mounted(), later, fixture.Geometry())
	if !ok || !session.Available() {
		t.Fatal("later session")
	}
	mergeRoot, ok := fixture.MergeNode()
	if !ok || !mergeRoot.Available() {
		t.Fatal("sealed merge root")
	}
	execution := fixture.Mounted().Arrangement().Execution()
	var paths derivation.Plan
	for _, expression := range execution.ExpressionIDs() {
		candidate, candidateOK := execution.Entry(expression)
		if candidateOK && candidate.Digest() == mergeRoot.Digest() {
			paths, ok = execution.Derivation(expression)
			break
		}
	}
	if !ok || !paths.Available() {
		t.Fatal("sealed merge paths")
	}
	binding, bindingOK := mergeRoot.Merge()
	if !bindingOK || !binding.Available() {
		t.Fatal("merge binding")
	}
	gotRows := 0
	for index := 0; index < paths.Len(); index++ {
		path, pathOK := paths.PathAt(index)
		if !pathOK {
			t.Fatal("merge path")
		}
		value, active, executeOK := session.executePath(mergeRoot, path, paths)
		if !executeOK {
			t.Fatalf("merge path %d refused", index)
		}
		if !active {
			continue
		}
		for _, batch := range value.batches {
			gotRows += batch.Len()
		}
	}
	inputs := make([]tuple.Batch, 0)
	for _, child := range mergeRoot.Children() {
		input, inputOK := child.Input()
		if !inputOK {
			t.Fatal("merge input")
		}
		reader, readerOK := read.Bind(fixture.LeftRoot(), input.Values(), fixture.Geometry(), fixture.Scratch())
		if !readerOK {
			t.Fatal("merge full reader")
		}
		batches, batchesOK := inputop.Execute(input, fixture.Mounted(), reader)
		if !batchesOK {
			t.Fatal("merge full input")
		}
		inputs = append(inputs, batches...)
	}
	full, fullOK := mergeop.Execute(binding, fixture.Mounted(), inputs)
	if !fullOK {
		t.Fatal("merge full execution")
	}
	wantRows := 0
	for _, batch := range full {
		wantRows += batch.Len()
	}
	if gotRows != wantRows || gotRows != len(fixture.RowsLeft()) {
		t.Fatalf("merge Later/full rows=%d/%d", gotRows, wantRows)
	}
}

// The mounted production census admits only unary Apply children (or the
// explicitly authored zero-input seed). Keep the non-pivot indexed-redemption
// gap from becoming reachable silently if a future compiler declaration adds
// another production shape.
func TestLaterProductionApplyShapeIsUnaryOrSeed(t *testing.T) {
	fixture := testfixture.New(t, 0xDF)
	execution := fixture.Mounted().Arrangement().Execution()
	seen := make(map[identity.ContentID]struct{})
	var visit func(arrangement.Node)
	visit = func(node arrangement.Node) {
		if !node.Available() {
			return
		}
		digest := node.Digest()
		if _, duplicate := seen[digest]; duplicate {
			return
		}
		seen[digest] = struct{}{}
		if node.Kind() == algebra.KindApply {
			binding, ok := node.Apply()
			if !ok || !binding.Available() {
				t.Fatal("unavailable production Apply binding")
			}
			if binding.ChildCount() > 1 || len(node.Children()) > 1 {
				t.Fatalf("production Apply has %d sealed children", binding.ChildCount())
			}
			if binding.ChildCount() == 0 && len(binding.Deliveries()) != 0 {
				t.Fatal("childless production Apply carries deliveries")
			}
		}
		for _, child := range node.Children() {
			visit(child)
		}
	}
	for _, expression := range execution.ExpressionIDs() {
		node, ok := execution.Entry(expression)
		if !ok {
			t.Fatal("production expression entry")
		}
		if node.Kind() != algebra.KindPublish {
			continue
		}
		visit(node)
	}
}

// Complete is not distributive over a Later frontier.  Until the mount
// supplies a sealed full-successor child extent, the delta evaluator must
// refuse this path rather than passing changed-only rows to complete.Execute
// and turning unchanged denominator members into ProvenAbsent cells.
func TestLaterRefusesPartialCompleteFrontier(t *testing.T) {
	fixture := testfixture.New(t, 0xE0)
	delta, ok := fixture.BaseToLeftDelta()
	if !ok {
		t.Fatal("complete delta")
	}
	later, ok := fixpoint.Later(delta)
	if !ok {
		t.Fatal("later root")
	}
	session, ok := New(fixture.Mounted(), later, fixture.Geometry())
	if !ok || !session.Available() {
		t.Fatal("later session")
	}
	entry, ok := fixture.Mounted().Arrangement().Execution().Dependency(fixture.DependencyComplete())
	if !ok {
		t.Fatal("complete entry")
	}
	if result, evaluateOK := session.Evaluate(entry); evaluateOK || result.Available() {
		t.Fatal("partial Complete frontier was accepted")
	}
}
