package recursivefamily

import "testing"

func TestKeyString(t *testing.T) {
	if got := (Key{Owner: "node"}).String(); got != "node" {
		t.Fatalf("owner-only key string = %q, want node", got)
	}
	if got := (Key{Namespace: "fn", Owner: "node"}).String(); got != "fn:node" {
		t.Fatalf("namespaced key string = %q, want fn:node", got)
	}
}

func TestKeyIsZero(t *testing.T) {
	if !(Key{}).IsZero() {
		t.Fatal("empty key should be zero")
	}
	if (Key{Owner: "node"}).IsZero() {
		t.Fatal("owner key should not be zero")
	}
}

func TestKeyHash(t *testing.T) {
	left := Key{Namespace: "fn", Owner: "node"}
	right := Key{Namespace: "fn", Owner: "node"}
	other := Key{Namespace: "fn", Owner: "edge"}
	if left.Hash() != right.Hash() {
		t.Fatal("equal keys should have equal hashes")
	}
	if left.Hash() == other.Hash() {
		t.Fatal("different keys should not collide in this regression case")
	}
}
