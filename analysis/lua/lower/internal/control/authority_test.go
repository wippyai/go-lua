package control

import "testing"

func TestControlAuthorityRequiresConcreteCrossings(t *testing.T) {
	writer := New(nil, nil, nil, nil, nil, nil, nil, "control.lua")
	if writer.Clean() != true {
		t.Fatal("new control writer retained pending state")
	}
	if err := writer.ready(); err == nil {
		t.Fatal("control writer accepted incomplete dependencies")
	}
}
