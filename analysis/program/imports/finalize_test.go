package imports

import "testing"

func TestFinalizerRejectsNilAndClosedCapabilities(t *testing.T) {
	var nilDraft *Draft
	if _, err := nilDraft.Finalizer(); err == nil {
		t.Fatal("nil Draft acquired a Finalizer")
	}
	var nilFinalizer Finalizer
	if _, err := nilFinalizer.Commit(CommitInput{}); err == nil {
		t.Fatal("nil Finalizer committed a Component")
	}
	if nilFinalizer.Abort() {
		t.Fatal("nil Finalizer aborted successfully")
	}
}
