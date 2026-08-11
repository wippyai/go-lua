package support

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
)

func TestMaskIdentityForwardsCanonicalGuardFormula(t *testing.T) {
	first, err := guard.New([]guard.Atom{3, 7, 11})
	if err != nil {
		t.Fatal(err)
	}
	second, err := guard.New([]guard.Atom{3, 7, 11})
	if err != nil {
		t.Fatal(err)
	}
	firstMask := identityMask(t, first, false)
	secondMask := identityMask(t, second, true)

	firstID, firstOK := firstMask.Identity()
	secondID, secondOK := secondMask.Identity()
	raw, rawOK := firstMask.Guard()
	rawID, rawIdentityOK := guard.Identity(raw)
	if !firstOK || !secondOK || !rawOK || !rawIdentityOK || !firstID.Available() || firstID != secondID || firstID != rawID {
		t.Fatalf("support identity forwarding = %x/%t %x/%t raw:%x/%t", firstID, firstOK, secondID, secondOK, rawID, rawIdentityOK)
	}
	if _, ok := (Mask{}).Identity(); ok {
		t.Fatal("invalid support mask received an identity")
	}
	polls := 0
	if cancelled, ok := firstMask.IdentityWithCheckpoint(func() bool {
		polls++
		return false
	}); ok || cancelled.Available() || polls == 0 {
		t.Fatalf("cancelled support identity = %x/%t after %d polls", cancelled, ok, polls)
	}
}

func identityMask(t testing.TB, manager *guard.Manager, reverse bool) Mask {
	t.Helper()
	work := New(manager)
	if work == nil {
		t.Fatal("support work")
	}
	var value Mask
	var ok bool
	if reverse {
		value, ok = work.Conjoin(work.True(), 11, true)
		if ok {
			value, ok = work.Conjoin(value, 3, false)
		}
	} else {
		value, ok = work.Conjoin(work.True(), 3, false)
		if ok {
			value, ok = work.Conjoin(value, 11, true)
		}
	}
	if !ok || !work.Seal() || !value.Valid() {
		t.Fatal("support formula")
	}
	return value
}
