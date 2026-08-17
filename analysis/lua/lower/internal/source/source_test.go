package source

import "testing"

func TestSourceOwnerRejectsIncompleteAuthorityBeforeDispatch(t *testing.T) {
	var owner *Owner
	if _, err := owner.Begin(nil); err == nil {
		t.Fatal("nil source owner accepted Begin")
	}
	if owner != nil && owner.Clean() {
		t.Fatal("nil source owner unexpectedly reported clean")
	}
}
