package project

import "testing"

func TestProjectQueryViewsRejectForeignHandles(t *testing.T) {
	p := projectProgram(t, `return 1`)
	left, err := projectDraft(t, []Module{{Name: "main", Program: p}}).Finalize()
	if err != nil {
		t.Fatal(err)
	}
	right, err := projectDraft(t, []Module{{Name: "main", Program: p}}).Finalize()
	if err != nil {
		t.Fatal(err)
	}
	leftShard, ok := left.Mounts().At(0)
	if !ok {
		t.Fatal("left shard unavailable")
	}
	if _, ok := left.Mounts().Index(leftShard); !ok {
		t.Fatal("left shard was not indexed")
	}
	if _, ok := left.Mounts().Index(rightShard(t, right)); ok {
		t.Fatal("foreign shard crossed Project owner fence")
	}
	if left.MatchesTarget(nil) || right.MatchesTarget(nil) {
		t.Fatal("nil Target matched Project")
	}
}

func rightShard(t *testing.T, component *Component) Shard {
	t.Helper()
	shard, ok := component.Mounts().At(0)
	if !ok {
		t.Fatal("right shard unavailable")
	}
	return shard
}
