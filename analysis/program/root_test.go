package program

import "testing"

func TestRootQueriesDoNotInventSourceRoots(t *testing.T) {
	published, err := Publish(rootAssembly(t, "program-root-law.lua"))
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	body, ok := published.BodyAt(0)
	if !ok {
		t.Fatal("BodyAt(0) unavailable")
	}
	if count, ok := body.RootCount(); !ok || count != 0 {
		t.Fatalf("RootCount = %d/%v; want an explicit empty Source root range", count, ok)
	}
	if _, ok := body.RootAt(-1); ok {
		t.Fatal("RootAt accepted a negative index")
	}
	if _, ok := body.RootAt(0); ok {
		t.Fatal("RootAt fabricated a Source root absent from the sealed range")
	}
	if (Root{}).Available() {
		t.Fatal("zero Root reported available")
	}
}
