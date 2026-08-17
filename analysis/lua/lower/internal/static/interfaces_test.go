package static

import "testing"

func TestStaticInterfaceRequiresPredeclaredDefinition(t *testing.T) {
	var w Writer
	if _, err := w.BeginInterface(nil); err == nil {
		t.Fatal("BeginInterface accepted a missing definition")
	}
	if err := w.FinishInterface(nil, 0, 0); err == nil {
		t.Fatal("FinishInterface accepted a missing definition")
	}
}
