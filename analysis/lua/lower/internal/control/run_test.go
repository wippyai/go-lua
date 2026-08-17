package control

import "testing"

func TestControlRunRequiresAReadyContinuation(t *testing.T) {
	var writer Writer
	if err := writer.Run(); err == nil {
		t.Fatal("Run accepted an uninitialized control writer")
	}
}
