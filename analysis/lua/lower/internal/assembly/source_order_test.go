package assembly

import "testing"

func TestAssemblyBodyOrderAndEntryAreSingleAssignment(t *testing.T) {
	c := newAssemblyCollector()
	body := c.Body(assemblyTestSpan())
	if body == 0 || !c.SetEntry(body) || c.Entry() != body {
		t.Fatalf("Body/Entry setup failed: body=%d entry=%d", body, c.Entry())
	}
	if !c.SetBody(body) {
		t.Fatal("SetBody rejected an empty authored Body sequence")
	}
	if c.SetBody(body) {
		t.Fatal("SetBody accepted a second fill")
	}
}
