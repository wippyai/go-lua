package axis

import (
	"strings"
	"testing"
)

type testExtension struct {
	kind string
	id   string
}

func (e testExtension) ExtensionKind() string { return e.kind }
func (e testExtension) ExtensionID() string   { return e.id }

func TestRegistryExtensionsValidateAndCopy(t *testing.T) {
	reg := NewRegistry()
	first := testExtension{kind: "test.extension", id: "first"}
	if err := reg.RegisterExtension(first); err != nil {
		t.Fatalf("RegisterExtension(first) error = %v", err)
	}
	if err := reg.RegisterExtension(first); err == nil || !strings.Contains(err.Error(), "duplicate extension") {
		t.Fatalf("RegisterExtension(duplicate) error = %v, want duplicate", err)
	}
	got := reg.Extensions("test.extension")
	if len(got) != 1 || got[0].ExtensionID() != "first" {
		t.Fatalf("Extensions = %#v, want first", got)
	}
	got[0] = testExtension{kind: "test.extension", id: "mutated"}
	if ext, ok := reg.LookupExtension("test.extension", "first"); !ok || ext.ExtensionID() != "first" {
		t.Fatalf("LookupExtension after caller mutation = %#v/%v, want first/true", ext, ok)
	}

	reg.Freeze()
	err := reg.RegisterExtension(testExtension{kind: "test.extension", id: "second"})
	if err == nil || !strings.Contains(err.Error(), "registry is frozen") {
		t.Fatalf("RegisterExtension(frozen) error = %v, want frozen", err)
	}
}
