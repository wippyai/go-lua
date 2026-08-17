package target

import "testing"

func TestContentIDProtocolEncodingTracksStateFinality(t *testing.T) {
	left := mustSeal(t, deltaProtocolFinal(false)).ContentID()
	right := mustSeal(t, deltaProtocolFinal(true)).ContentID()
	if left == right {
		t.Fatal("protocol state finality mutation was omitted from ContentID")
	}
}
