package project

import "testing"

func TestProjectDraftViewsExpireWithSharedFinalizationFence(t *testing.T) {
	p := projectProgram(t, `return 1`)
	draft := projectDraft(t, []Module{{Name: "main", Program: p}})
	views := draft.Mounts()
	if views.Count() != 1 {
		t.Fatalf("draft mount count = %d, want 1", views.Count())
	}
	copyDraft := *draft
	if _, err := draft.Finalize(); err != nil {
		t.Fatal(err)
	}
	if views.Count() != 0 || copyDraft.Mounts().Count() != 0 {
		t.Fatal("finalization left a live construction view")
	}
	if _, err := copyDraft.Finalize(); err == nil {
		t.Fatal("copied Project Draft finalized twice")
	}
}
