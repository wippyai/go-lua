package typ

import "testing"

func TestJoinReturnSlot_PreservesUnknownOverNil(t *testing.T) {
	if got := JoinReturnSlot(Unknown, Nil); !TypeEquals(got, Unknown) {
		t.Fatalf("JoinReturnSlot(unknown, nil) = %v, want unknown", got)
	}
	if got := JoinReturnSlot(Nil, Unknown); !TypeEquals(got, Unknown) {
		t.Fatalf("JoinReturnSlot(nil, unknown) = %v, want unknown", got)
	}
}
