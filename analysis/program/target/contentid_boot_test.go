package target

import "testing"

func TestContentIDBootEncodingTracksWholeObjectShape(t *testing.T) {
	left := completeBootSpec("Lua 5.3", InitialMutable)
	right := completeBootSpec("Lua 5.3", InitialMutable)
	right.InitialRoots[0].Shape.Immutable = !right.InitialRoots[0].Shape.Immutable
	leftID := mustSeal(t, left).ContentID()
	rightID := mustSeal(t, right).ContentID()
	if !leftID.Available() || !rightID.Available() {
		t.Fatal("boot contracts did not receive ContentIDs")
	}
	if leftID == rightID {
		t.Fatal("boot shape mutation was omitted from ContentID")
	}
}
