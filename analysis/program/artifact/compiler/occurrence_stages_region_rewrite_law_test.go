package compiler

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
)

func regionRewriteID(value byte) identity.ContentID {
	var id identity.ContentID
	id[0] = value
	return id
}

func TestRewriteRegionMembersIgnoresUnrelatedGlobalStages(t *testing.T) {
	first, second := regionRewriteID(1), regionRewriteID(2)
	firstStage := regionRewriteID(3)
	stageFor := map[identity.ContentID][]identity.ContentID{
		first: {firstStage},
	}
	for value := byte(4); value < 100; value++ {
		unrelated := regionRewriteID(value)
		stageFor[unrelated] = []identity.ContentID{regionRewriteID(value + 100)}
	}

	rewritten, injected, ok := rewriteRegionMembers([]identity.ContentID{first, second}, nil, stageFor)
	if !ok {
		t.Fatal("rewrite failed")
	}
	want := []identity.ContentID{first, firstStage, second}
	if len(rewritten) != len(want) || cap(rewritten) != len(want) {
		t.Fatalf("rewritten len/cap=%d/%d, want %d/%d", len(rewritten), cap(rewritten), len(want), len(want))
	}
	for index := range want {
		if rewritten[index] != want[index] {
			t.Fatalf("rewritten[%d]=%s, want %s", index, rewritten[index], want[index])
		}
	}
	if len(injected) != 1 || injected[0] != first {
		t.Fatalf("injected=%v, want [%s]", injected, first)
	}
}
