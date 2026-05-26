package generics_test

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// TestCrossCompilationGenericIdentity guards that a named generic type declared
// in one compilation does not leak into an independent compilation that declares
// a same-named type with a different body. Named-generic identity is scoped by a
// per-compilation epoch, so the process-global product-value interner cannot
// merge `Box<T> = {value: T}` from one compilation with `Box<T> = interface{...}`
// from another.
func TestCrossCompilationGenericIdentity(t *testing.T) {
	recordBoxes := []string{
		`type Box<T> = {value: T}
		local function wrap<T>(x: T): Box<T> return {value = x} end
		local box = wrap(42)
		local n: integer = box.value`,
		`type Box<T> = {value: T}
		local function unwrap<T>(box: Box<T>): T return box.value end
		local box: Box<string> = {value = "hello"}
		local s: string = unwrap(box)`,
	}
	interfaceBox := `type Box<T> = interface { unwrap: fun(self: self): T }
		local function process<T>(box: Box<T>): T
			return box:unwrap()
		end`

	for _, src := range recordBoxes {
		_ = testutil.Check(src, testutil.WithStdlib())
	}
	r := testutil.Check(interfaceBox, testutil.WithStdlib())
	if r.HasError() {
		t.Fatalf("interface Box<T> leaked record-Box identity from a prior compilation: %v",
			testutil.ErrorMessages(r.Diagnostics))
	}
}
