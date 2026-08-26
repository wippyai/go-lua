package arithmetic

import "testing"

func TestWorldBuildsSeededRoot(t *testing.T) {
	fixture := New(t)
	if !fixture.Mounted().Available() || !fixture.View().Available() || !fixture.Base().Available() {
		t.Fatal("arithmetic world unavailable")
	}
	if _, ok := fixture.Output(fixture.Base()); !ok {
		t.Fatal("arithmetic output reader")
	}
	if _, ok := fixture.OutputPayload(fixture.Base()); !ok {
		t.Fatal("arithmetic output row vector reader")
	}
}
