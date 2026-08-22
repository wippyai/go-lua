package exactscalar

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
)

func TestCompileEmptyInputOwnsCanonicalEmptyBundle(t *testing.T) {
	bundle, fault := Compile(Input{})
	if fault.Available() || bundle == nil {
		t.Fatalf("empty compile fault=%v bundle=%v", fault, bundle)
	}
	if rows := bundle.Rows(); len(rows) != 0 {
		t.Fatalf("empty rows = %d, want 0", len(rows))
	}
	if _, ok := bundle.Exact(identity.ContentID{}); ok {
		t.Fatal("zero identity unexpectedly had an exact scalar")
	}
	bundle.ReleaseFacts()
	if rows := bundle.Rows(); len(rows) != 0 {
		t.Fatalf("released bundle rows = %d, want 0", len(rows))
	}
}
