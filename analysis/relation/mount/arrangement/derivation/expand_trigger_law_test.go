package derivation_test

import (
	"testing"

	expandfixture "github.com/wippyai/go-lua/analysis/engine/relation/runtime/testdata/targetfixture/expand"
	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
)

// The R-only source is a sealed descriptor, not an evaluator-created Input.
// This law checks that the real mounted Expand plan names exactly one C path,
// one Expand frame, and the full authored R vector access.
func TestExpandReaderTriggerSealsActualCandidatePath(t *testing.T) {
	fixture := expandfixture.New(t)
	execution := fixture.World().Mounted().Arrangement().Execution()
	entry, ok := execution.Dependency(fixture.Dependency())
	if !ok || !entry.Available() || entry.Node().Kind() != algebra.KindExpand {
		t.Fatal("Expand dependency entry")
	}
	plan, ok := execution.Derivation(entry.Expression())
	if !ok || !plan.Available() {
		t.Fatal("Expand derivation")
	}
	triggers := plan.ExpandReaderTriggers()
	if len(triggers) != 1 {
		t.Fatalf("Expand triggers=%d, want one", len(triggers))
	}
	trigger := triggers[0]
	logical, logicalOK := execution.LogicalNode(trigger.Node())
	if !trigger.Available() || !logicalOK || !logical.Available() || logical.Kind() != algebra.KindExpand {
		t.Fatal("trigger node was not the sealed Expand node")
	}
	path, ok := plan.Path(trigger.PathOccurrence())
	if !ok || path.LeafRelation() != fixture.Contract().Candidate() {
		t.Fatal("trigger did not name the candidate Input path")
	}
	frame, ok := path.FrameAt(int(trigger.FrameOrdinal()))
	if !ok || frame.Kind() != algebra.KindExpand || frame.Node() != trigger.Node() {
		t.Fatal("trigger frame was not the actual Expand zipper frame")
	}
	reader := trigger.Reader().Access()
	if !reader.Available() || reader.Relation() != fixture.Contract().Reader() || len(reader.Columns()) == 0 || reader.Key().Available() {
		t.Fatal("trigger reader access was not the full unkeyed R vector")
	}
	if sibling, siblingOK := frame.SiblingAt(1); !siblingOK || sibling.Physical() != trigger.Reader().Physical() {
		t.Fatal("trigger reader did not match the sealed frame sibling")
	}
	replay := trigger.Replay()
	if !replay.Available() || replay.EmitOccurrence() != trigger.PathOccurrence() || replay.WatcherCount() != 1 {
		t.Fatalf("replay metadata available=%t emit=%d watchers=%d", replay.Available(), replay.EmitOccurrence(), replay.WatcherCount())
	}
	anchor := replay.Anchor()
	if !anchor.Available() || anchor.PathOccurrence() != trigger.PathOccurrence() || anchor.Access().Physical() != path.Leaf().Physical() {
		t.Fatal("replay did not seal the exact candidate anchor")
	}
	if scan := anchor.Range(); !scan.Available() || scan.Access().Relation() != fixture.Contract().Candidate() || scan.Access().Key().Available() || len(scan.Access().Columns()) != 0 {
		t.Fatal("replay did not seal the candidate relation range")
	}
	watcher, watcherOK := replay.WatcherAt(0)
	if !watcherOK || !watcher.Available() || watcher.PathOccurrence() != trigger.PathOccurrence() || watcher.StopFrame() != trigger.FrameOrdinal() || watcher.StopFrameDigest() != trigger.Node() || watcher.Leaf().Physical() != path.Leaf().Physical() {
		t.Fatal("replay watcher did not retain the exact stop boundary")
	}
}
