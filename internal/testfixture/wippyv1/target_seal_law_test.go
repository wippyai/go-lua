package wippyv1

import (
	"sync"
	"testing"
)

// TestTargetIsSealedOncePerProcess states the compute-once law for the
// transcribed v1 host surface. The whole input is the compiled-in provider
// set, so the seal has one content identity and every caller must receive that
// one sealed value rather than an equal re-derivation.
func TestTargetIsSealedOncePerProcess(t *testing.T) {
	first, err := Target()
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 16; index++ {
		again, err := Target()
		if err != nil {
			t.Fatal(err)
		}
		if again != first {
			t.Fatalf("call %d re-sealed the v1 target instead of sharing the sealed value", index)
		}
	}
}

// TestTargetIsSharedByConcurrentCallers states that concurrent callers share
// one seal rather than racing several.
func TestTargetIsSharedByConcurrentCallers(t *testing.T) {
	expected, err := Target()
	if err != nil {
		t.Fatal(err)
	}
	const callers = 32
	var group sync.WaitGroup
	shared := make([]bool, callers)
	group.Add(callers)
	for index := 0; index < callers; index++ {
		go func(slot int) {
			defer group.Done()
			answer, err := Target()
			shared[slot] = err == nil && answer == expected
		}(index)
	}
	group.Wait()
	for slot, ok := range shared {
		if !ok {
			t.Fatalf("concurrent caller %d did not receive the shared seal", slot)
		}
	}
}

// TestTargetIdentityIsStable states that the one seal answers one identity.
func TestTargetIdentityIsStable(t *testing.T) {
	target, err := Target()
	if err != nil {
		t.Fatal(err)
	}
	id := target.ContentID()
	if !id.Available() {
		t.Fatal("sealed v1 target has no content identity")
	}
	for index := 0; index < 100; index++ {
		if target.ContentID() != id {
			t.Fatal("v1 target identity changed between reads")
		}
	}
}
