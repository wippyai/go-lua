package lexicalidentity

import (
	"encoding/json"
	"testing"
)

func TestBodyIdentityDomains(t *testing.T) {
	ns := UnitNamespaceFromContent([]byte("unit"))
	if ns == (UnitNamespace{}) || ns != UnitNamespaceFromContent([]byte("unit")) {
		t.Fatal("unit namespace is empty or unstable")
	}
	root := RootBody(ns)
	fn := FunctionBody(ns, 1)
	if root == (StableLexicalBodyID{}) || fn == (StableLexicalBodyID{}) || root == fn {
		t.Fatalf("body domains alias: root=%x fn=%x", root, fn)
	}
	if fn == FunctionBody(ns, 2) || fn == FunctionBody(UnitNamespaceFromContent([]byte("other")), 1) {
		t.Fatal("function body lost owner or unit namespace")
	}
	if FunctionBody(ns, 0) != (StableLexicalBodyID{}) || RootBody(UnitNamespace{}) != (StableLexicalBodyID{}) {
		t.Fatal("invalid inputs did not fail closed")
	}
}

func TestBodyIdentityTextEncodingIsCanonicalHex(t *testing.T) {
	id := RootBody(UnitNamespaceFromContent([]byte("unit")))
	if got := len(id.String()); got != 64 {
		t.Fatalf("hex length = %d, want 64", got)
	}
	encoded, err := json.Marshal(id)
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != `"`+id.String()+`"` {
		t.Fatalf("JSON = %s, want canonical hex string", encoded)
	}
}
